// Android 快捷指令（HTTP Shortcuts）**真正发出的请求形状**，逐条打一遍。
//
// 为什么值得单独一份：那几条捷径用的形状和网页端不一样，网页端全绿并不能说明捷径能用 ——
//   · 鉴权走**查询串** `?auth=`，不是 `Authorization` 头
//   · 「接收最新」走的是 `/content/latest.json` 路径后缀，不是 `?json=1`
//   · 「展示文件」直连 `/file/<uuid>/<name>`，**且不带 room**（服务端按文件记录的房间鉴权）
// 上午那个「文本正常、文件/图片 401」就是这么漏出去的：第二次请求（下载）没带凭据。
// 这条链路上任何一处形状变了，这里必须跟着红。
//
// 请求形状取自 shortcuts/android/shortcuts.json 的导出（那个文件带密码不入库，
// 所以这里只能照抄一份 —— **改捷径的 URL 时同步改这里**）：
//   发送文本   POST {url}/text?room={room}&auth={auth}&name={name}         body: text/plain
//   发送文件   POST {url}/upload?room={room}&auth={auth}&name={name}       multipart，字段名 file
//   接收最新   GET  {url}/content/latest.json?room={room}&auth={auth}
//   接收指定ID GET  {url}/content/{ID}?room={room}&json=true&auth={auth}
//   展示文件   GET  {url}/file/{uuid}/{name}?auth={auth}
import { TextHandler } from './.build/text.mjs';
import { FileHandler } from './.build/file.mjs';
import { ContentHandler } from './.build/content.mjs';
import { makeEnv, makeChecker, postJson, getJson } from './harness.mjs';

const { check, summary } = makeChecker();

const PNG = new Uint8Array([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
]);
const DEVICE = 'Android%E5%BF%AB%E6%8D%B7%E6%8C%87%E4%BB%A4';   // name 参数是编码后的
const FILE_NAME = '测试 图 1.png';                                // 中文 + 空格，顺带验编码

// 「接收指定ID」和「展示文件」都要 params —— 真路由会填，测试里自己填
async function getById(env, id, query, auth) {
  const req = new Request(`http://worker.local/content/${id}?${query}`);
  req.params = { id: String(id) };
  const res = await ContentHandler.getById(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text };
}

async function download(env, uuid, name, { auth = '123' } = {}) {
  const q = auth ? `?auth=${encodeURIComponent(auth)}` : '';
  const req = new Request(`http://worker.local/file/${uuid}/${encodeURIComponent(name)}${q}`);
  req.params = { uuid, filename: encodeURIComponent(name) };
  const res = await FileHandler.download(req, env);
  return { status: res.status, bytes: new Uint8Array(await res.arrayBuffer()) };
}

async function sendFile(env, { auth = '123', room = 'default' } = {}) {
  const form = new FormData();
  form.append('file', new Blob([PNG], { type: 'image/png' }), FILE_NAME);
  const res = await FileHandler.upload(
    new Request(`http://worker.local/upload?room=${room}&auth=${auth}&name=${DEVICE}`, {
      method: 'POST', body: form,
    }), env);
  return res.status;
}

// 文本那条链：发送 → 接收最新 → 接收指定ID。
// 每个场景用**各自的 env**：两条消息的 timestamp 都是秒级，同一秒里 text 和 file
// 谁在前是不确定的，混在一个库里测「最新」会测出假失败。
async function textFlow(label, env, auth) {
  const send = await postJson(TextHandler.create, env,
    `/text?room=default&auth=${auth}&name=${DEVICE}`, '捷径验收文本', { auth: null });
  check(`${label}｜发送文本 POST /text?auth=`, send.status, 200);

  const latest = await getJson(ContentHandler.getLatest, env,
    `/content/latest.json?room=default&auth=${auth}`, { auth: null });
  check(`${label}｜接收最新 GET /content/latest.json`, latest.status, 200);
  check(`${label}｜拿到文字`, latest.json?.content, '捷径验收文本');

  const byId = await getById(env, latest.json.id, `room=default&json=true&auth=${auth}`);
  check(`${label}｜接收指定ID GET /content/{id}?json=true`, byId.status, 200);
  check(`${label}｜按 ID 拿到同一份文字`, byId.json?.content, '捷径验收文本');
}

// 文件那条链：发送 → 接收最新（拿 uuid）→ 展示文件（第二次请求，要自己带 auth）
async function fileFlow(label, env, auth) {
  check(`${label}｜发送文件 POST /upload (multipart)`, await sendFile(env, { auth }), 200);

  const latest = await getJson(ContentHandler.getLatest, env,
    `/content/latest.json?room=default&auth=${auth}`, { auth: null });
  check(`${label}｜最新一条是文件`, latest.json?.type, 'image');
  check(`${label}｜文件名保真`, latest.json?.name, FILE_NAME);

  const dl = await download(env, latest.json.uuid, FILE_NAME, { auth });
  check(`${label}｜展示文件 GET /file/{uuid}/{name}?auth=`, dl.status, 200);
  check(`${label}｜下到的是原字节`, dl.bytes.length === PNG.length
    && dl.bytes.every((b, i) => b === PNG[i]), true);

  const byId = await getById(env, latest.json.id, `room=default&json=true&auth=${auth}`);
  check(`${label}｜按 ID 也能取到文件`, byId.json?.uuid, latest.json.uuid);
}

console.log('\n── A. 加密房间（AUTH_PASSWORD 已设）：带对密码要全通 ──');
{
  await textFlow('加密', makeEnv().env, '123');   // harness 里 AUTH_PASSWORD = '123'
  await fileFlow('加密', makeEnv().env, '123');
}

console.log('\n── B. 加密房间：不带凭据 / 密码错，必须被拒 ──');
{
  const { env } = makeEnv();
  await postJson(TextHandler.create, env, '/text?room=default&auth=123', 'secret', { auth: null });

  const noAuth = await getJson(ContentHandler.getLatest, env,
    '/content/latest.json?room=default', { auth: null });
  check('无凭据读最新被拒', noAuth.status, 401);

  const wrong = await getJson(ContentHandler.getLatest, env,
    '/content/latest.json?room=default&auth=wrong', { auth: null });
  check('密码错读最新被拒', wrong.status, 401);

  // 下载这条最容易漏：它是**第二次**请求，第一次 URL 上的 auth 不会自动跟过来
  const { env: env2 } = makeEnv();
  await sendFile(env2, { auth: '123' });
  const latest = await getJson(ContentHandler.getLatest, env2,
    '/content/latest.json?room=default&auth=123', { auth: null });
  const uuid = latest.json.uuid;

  check('无凭据下载被拒', (await download(env2, uuid, FILE_NAME, { auth: '' })).status, 401);
  check('密码错下载被拒', (await download(env2, uuid, FILE_NAME, { auth: 'wrong' })).status, 401);
  check('密码对下载通过', (await download(env2, uuid, FILE_NAME, { auth: '123' })).status, 200);
}

console.log('\n── C. 不加密房间（AUTH_PASSWORD 为空）：不带凭据要全通 ──');
{
  // 开放实例：auth 变量留空，URL 里就是 `auth=`（空值），不是没有这个参数
  const { env } = makeEnv();
  env.AUTH_PASSWORD = '';
  env.ROOM_AUTH_JSON = '{}';
  await textFlow('开放', env, '');

  const { env: env2 } = makeEnv();
  env2.AUTH_PASSWORD = '';
  env2.ROOM_AUTH_JSON = '{}';
  await fileFlow('开放', env2, '');
}

console.log('\n── D. 房间级密码：只认本房间的密码 ──');
{
  const { env } = makeEnv();
  env.AUTH_PASSWORD = '';                                  // 全局不设，只有 vault 有密码
  env.ROOM_AUTH_JSON = JSON.stringify({ vault: { password: 'vault-pw' } });

  const ok = await postJson(TextHandler.create, env,
    `/text?room=vault&auth=vault-pw&name=${DEVICE}`, 'vault 里的文本', { auth: null });
  check('vault + 房间密码 → 200', ok.status, 200);

  const bad = await postJson(TextHandler.create, env,
    `/text?room=vault&auth=default-pw&name=${DEVICE}`, '不该进去', { auth: null });
  check('vault + 别的密码 → 401', bad.status, 401);

  const open = await postJson(TextHandler.create, env,
    `/text?room=default&auth=&name=${DEVICE}`, 'default 房间开着', { auth: null });
  check('default（没设密码）+ 空 auth → 200', open.status, 200);
}

console.log('\n── E. 错误响应形状：code / error / message 三字段，且恒为 JSON ──');
{
  // 锁死的是所有客户端共用的契约：Apple 快捷指令不暴露 HTTP 状态码、只能读响应体，
  // 前端则依赖 message 显示具体原因。这里曾经按 Accept 分叉（带 Accept 给 JSON、
  // 不带就给 text/plain），而捷径根本不发 Accept，于是「文本超限」被误报成
  // 「服务器未确认保存，请检查部署地址及服务器状态」。
  const { env } = makeEnv();
  env.TEXT_LIMIT = '100';

  const tooLong = await postJson(TextHandler.create, env,
    `/text?room=default&name=${DEVICE}`, 'A'.repeat(101));
  check('文本超限 → 413', tooLong.status, 413);
  check('  · code', tooLong.json?.code, 'text_too_long');
  check('  · error 是英文人话', tooLong.json?.error, 'Text too long');
  check('  · message 是中文人话', tooLong.json?.message, '文本长度超出限制 (最大 100 字符)');

  const empty = await postJson(TextHandler.create, env,
    `/text?room=default&name=${DEVICE}`, '   ');
  check('空内容 → 400', empty.status, 400);
  check('  · code', empty.json?.code, 'empty_content');
  check('  · message', empty.json?.message, '内容不能为空');

  // 取不存在的内容：Accept 是 text/html 也必须拿到 JSON（这条以前会回纯文本）
  const missing = await getById(env, 999999, 'room=default&auth=123', '123');
  check('内容不存在 → 404', missing.status, 404);
  check('  · code', missing.json?.code, 'content_not_found');
  check('  · message', missing.json?.message, '内容未找到');
}

summary('Android 快捷指令的请求形状在 Worker 上成立');
