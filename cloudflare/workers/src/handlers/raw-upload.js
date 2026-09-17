import { corsHeaders } from '../cors';
import { TextHandler } from './text';
import { FileHandler } from './file';
import { ensureRoomAccess, normalizeRoomName } from '../auth';
import { sniffPayload, normalizeText, htmlDocumentToPlainText } from '../sniff';

// ─────────────────────────────────────────────────────────────
// /upload/raw 与 /upload/base64 —— 与 Go 版 cloud-clip/lib/handler.go 行为对齐。
//
// 客户端（Apple 快捷指令）不判断类型，只发原始字节，由这里按内容魔数分流。
// 缺了这两个端点，Send 在 Cloudflare Worker 部署上会 404。
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

function badRequest(error, message) {
  return new Response(JSON.stringify({ error, message }), {
    status: 400,
    headers: { 'Content-Type': 'application/json', ...corsHeaders },
  });
}

// 决定文件的存储名。与 Go 版 resolveFileName 行为一致：
// 客户端只能给出「不含扩展名」的名字——Shortcuts 的 getName 会把扩展名剥掉
// （rightclick.txt → rightclick），所以按内容嗅探到的扩展名补回去，
// 让接收端拿到 requirements.txt 而不是一个没有后缀的 requirements。
// 判断依据与 Go 的 filepath.Ext 对齐：名字里只要有「.」就认为已有扩展名。
function resolveFileName(name, ext) {
  if (!name) return `clipboard.${ext}`;
  return name.includes('.') ? name : `${name}.${ext}`;
}

async function handle(request, env, { base64 }) {
  try {
    const url = new URL(request.url);
    const room = normalizeRoomName(url.searchParams.get('room'));
    const requestedName = url.searchParams.get('name') || '';
    const asFile = url.searchParams.get('as') === 'file';

    const authResult = await ensureRoomAccess(request, env, room);
    if (!authResult.ok) return authResult.response;

    const raw = new Uint8Array(await request.arrayBuffer());

    let bytes = raw;
    if (base64) {
      let encoded = new TextDecoder('utf-8').decode(raw).trim();
      const comma = encoded.indexOf(',');
      if (comma >= 0) {
        const prefix = encoded.slice(0, comma);
        if (prefix.startsWith('data:') && prefix.endsWith(';base64')) encoded = encoded.slice(comma + 1);
      }
      try {
        const binary = atob(encoded);
        bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      } catch {
        return badRequest('Invalid base64', '无效的 base64 数据');
      }
    }

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
  // POST /upload/raw —— 请求体即原始字节，按内容嗅探自动分流文本/文件
  static async upload(request, env) {
    return handle(request, env, { base64: false });
  }

  // POST /upload/base64 —— 请求体为 base64 文本（可带 data:*;base64, 前缀），解码后同样分流
  static async uploadBase64(request, env) {
    return handle(request, env, { base64: true });
  }
}
