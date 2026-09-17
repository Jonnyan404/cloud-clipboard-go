import { corsHeaders } from '../cors';
import { TextHandler } from './text';
import { FileHandler } from './file';
import { ensureRoomAccess, normalizeRoomName } from '../auth';
import { sniffPayload, normalizeText, htmlDocumentToPlainText } from '../sniff';

// ─────────────────────────────────────────────────────────────
// POST /upload/raw —— 与 Go 版 cloud-clip/lib/handler.go 行为对齐。
//
// 快捷指令的「剪贴板路径」用它：剪贴板里可能是文字也可能是图片，客户端分不出
// （只能靠本地化的 typeOf），所以把原始字节发过来、由服务端按内容魔数分流。
// 分享路径不走这里 —— 文本走文本消息分支，文件走 /upload（multipart，part 自带真实文件名）。
// 缺了这个端点，Send 在 Cloudflare Worker 部署上会 404。
//
// 设计：**不复制存储逻辑**。嗅探出类型后把请求重新包装，委托给既有处理器：
//   · 文本 → TextHandler.create（复用长度限制、D1 写入、清理、广播、响应格式）
//   · 文件 → FileHandler.upload（复用 R2 写入、过期时间、D1 写入、清理、广播）
// 两个端点各自只多出「嗅探 + 包装」这一步，存储路径与既有端点完全一致。
//
// 嗅探与文本规范化的纯逻辑在 ../sniff.js（零依赖，可被 Node 直接单测）。
// ─────────────────────────────────────────────────────────────

// 把「原始字节」变成既有处理器认得的形状。
// 只保留认证与客户端标识相关头，避免把原请求的 Content-Type 等误导性头带过去。
function copyHeaders(request, extra = {}) {
  const headers = new Headers();
  for (const name of ['Authorization', 'Cookie', 'User-Agent', 'CF-Connecting-IP']) {
    const v = request.headers.get(name);
    if (v) headers.set(name, v);
  }
  for (const [k, v] of Object.entries(extra)) headers.set(k, v);
  return headers;
}

async function dispatchAsText(request, env, text) {
  const synth = new Request(request.url, {
    method: 'POST',
    headers: copyHeaders(request, { 'Content-Type': 'text/plain; charset=utf-8' }),
    // HTML 降级与 Go 版一样在入库前完成
    body: htmlDocumentToPlainText(text),
  });
  return TextHandler.create(synth, env);
}

async function dispatchAsFile(request, env, bytes, fileName) {
  const synth = new Request(request.url, {
    method: 'POST',
    headers: copyHeaders(request, {
      // FileHandler.upload 认这个头走「流式上传」分支，内部会 decodeURIComponent
      'X-File-Name': encodeURIComponent(fileName),
      'X-File-Size': String(bytes.length),
      'Content-Type': 'application/octet-stream',
    }),
    body: bytes,
  });
  return FileHandler.upload(synth, env);
}

// 决定文件的存储名：客户端没给名字时退回默认名。
// 曾经在这里按嗅探结果补扩展名，但快捷指令的文件分支改走 multipart
// （part 自带真实文件名，含扩展名）后，?name= 不再有人传，那段成了死代码。
function resolveFileName(name, ext) {
  return name || `clipboard.${ext}`;
}

async function handle(request, env) {
  try {
    const url = new URL(request.url);
    const room = normalizeRoomName(url.searchParams.get('room'));
    const requestedName = url.searchParams.get('name') || '';
    const asFile = url.searchParams.get('as') === 'file';

    const authResult = await ensureRoomAccess(request, env, room);
    if (!authResult.ok) return authResult.response;

    const bytes = new Uint8Array(await request.arrayBuffer());

    const { kind, ext } = sniffPayload(bytes);

    if (!asFile && kind === 'text' && !requestedName) {
      const textLimit = env.TEXT_LIMIT ? parseInt(env.TEXT_LIMIT) : 4096;
      if (textLimit <= 0 || bytes.length <= textLimit) {
        return await dispatchAsText(request, env, normalizeText(bytes));
      }
      console.log(`按文件处理: 文本超出文本消息限制 (${bytes.length} 字节), 转为文件存储`);
    }

    const fileName = resolveFileName(requestedName, ext);
    return await dispatchAsFile(request, env, bytes, fileName);
  } catch (error) {
    console.error('[raw-upload] error:', error);
    const reason = String(error?.message || error || 'unknown');
    return new Response(JSON.stringify({
      error: 'Internal Server Error',
      message: `处理上传时发生错误: ${reason}`,
    }), {
      status: 500,
      headers: { 'Content-Type': 'application/json', ...corsHeaders },
    });
  }
}

export class RawUploadHandler {
  // POST /upload/raw —— 请求体即原始字节，按内容嗅探自动分流文本/文件。
  // 快捷指令的剪贴板路径靠它：剪贴板里可能是文字也可能是图片，客户端分不出
  // （只能靠本地化的 typeOf），所以把分流交给服务端。
  static async upload(request, env) {
    return handle(request, env);
  }
}
