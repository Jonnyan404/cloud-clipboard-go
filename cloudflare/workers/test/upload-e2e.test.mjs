// 端到端验证 Cloudflare Worker 的 /upload/raw 与 /upload/base64 胶水层：
// 请求包装 → 嗅探 → 委托给 TextHandler/FileHandler → D1/R2 落库 → 响应 JSON。
// 用 esbuild 打包后的处理器 + mock env（D1 用 node:sqlite，R2 用 Map）。
import { RawUploadHandler } from './.build/raw-upload.mjs';
import { DatabaseSync } from 'node:sqlite';

function makeEnv() {
  const db = new DatabaseSync(':memory:');
  db.exec(`CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT, content TEXT, name TEXT, size INTEGER,
    room TEXT, timestamp INTEGER, senderIP TEXT, senderClientID TEXT, userAgent TEXT,
    uuid TEXT, expireTime INTEGER, url TEXT)`);
  const DB = {
    prepare(sql) {
      const stmt = db.prepare(sql);
      let args = [];
      const api = {
        bind(...a) { args = a; return api; },
        async run() { const r = stmt.run(...args); return { meta: { last_row_id: Number(r.lastInsertRowid) } }; },
        async first() { return stmt.get(...args) ?? null; },
        async all() { return { results: stmt.all(...args) }; },
      };
      return api;
    },
  };
  const r2 = new Map();
  const R2_BUCKET = {
    async put(key, body, opts) { r2.set(key, { body, opts }); },
    async delete(key) { r2.delete(key); },
  };
  // Durable Object 桩：链式调用不断，且必须让 then 为 undefined ——
  // 否则 await 一个 Proxy 会无限递归（Proxy 对 then 也返回自身，成为永不 resolve 的 thenable）。
  const makeStub = () => new Proxy(function () {}, {
    get: (t, prop) => (prop === 'then' ? undefined : makeStub()),
    apply: () => Promise.resolve(makeStub()),
  });
  const anyStub = makeStub();
  const env = {
    DB, R2_BUCKET, WEBSOCKET_ROOM: anyStub,
    AUTH_PASSWORD: '123', ROOM_AUTH_JSON: '{}', HISTORY_LIMIT: '50',
    TEXT_LIMIT: '40960', FILE_LIMIT: '204857600', FILE_EXPIRE: '3600',
  };
  return { env, db, r2 };
}

let pass = 0, fail = 0;
const failures = [];
function check(name, got, want) {
  const g = JSON.stringify(got), w = JSON.stringify(want);
  if (g === w) { pass++; console.log(`  ✓ ${name}`); }
  else { fail++; failures.push(`${name}\n      实际: ${g}\n      期望: ${w}`); console.log(`  ✗ ${name}  实际=${g} 期望=${w}`); }
}

async function call(env, path, body, { base64 = false, auth = 'Bearer 123' } = {}) {
  const headers = {};
  if (auth) headers.Authorization = auth;
  const req = new Request(`http://worker.local${path}`, { method: 'POST', headers, body });
  const res = base64
    ? await RawUploadHandler.uploadBase64(req, env)
    : await RawUploadHandler.upload(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text };
}

const lastMessage = (db) => db.prepare('SELECT * FROM messages ORDER BY id DESC LIMIT 1').get();

console.log('\n── 1. 纯文本走文本消息 ──');
{
  const { env, db } = makeEnv();
  const r = await call(env, '/upload/raw?room=default&client=ios-shortcuts', 'hello world');
  check('HTTP 200', r.status, 200);
  check('响应 type=text', r.json.type, 'text');
  check('响应带 id', typeof r.json.id, 'string');
  const m = lastMessage(db);
  check('D1 type=text', m.type, 'text');
  check('D1 content 正确', m.content, 'hello world');
  check('D1 room', m.room, 'default');
  check('D1 senderClientID', m.senderClientID, 'ios-shortcuts');
}

console.log('\n── 2. 中文文本 ──');
{
  const { env, db } = makeEnv();
  await call(env, '/upload/raw?room=default', '中文内容 你好世界');
  check('D1 content 中文完好', lastMessage(db).content, '中文内容 你好世界');
}

console.log('\n── 3. HTML 文档被降级为纯文本 ──');
{
  const { env, db } = makeEnv();
  const html = '<!DOCTYPE html>\n<html><head><title>T</title></head><body><h1>标题</h1><p>Hello &amp; 你好</p></body></html>';
  await call(env, '/upload/raw?room=default', html);
  check('D1 content 已剥标签', lastMessage(db).content, '标题 Hello & 你好');
}

console.log('\n── 4. as=file 强制存成文件 ──');
{
  const { env, db, r2 } = makeEnv();
  const r = await call(env, '/upload/raw?room=default&as=file', 'this is a text file');
  check('HTTP 200', r.status, 200);
  const m = lastMessage(db);
  check('D1 type=file', m.type, 'file');
  check('D1 name=clipboard.txt', m.name, 'clipboard.txt');
  check('D1 带 uuid', typeof m.uuid, 'string');
  check('R2 有一个对象', r2.size, 1);
  check('R2 对象名含 uuid', [...r2.keys()][0], `files/${m.uuid}`);
}

console.log('\n── 5. PNG 被嗅探为图片 ──');
{
  const { env, db } = makeEnv();
  const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]);
  const r = await call(env, '/upload/raw?room=default', png);
  const m = lastMessage(db);
  check('D1 type=file', m.type, 'file');
  check('D1 name=clipboard.png', m.name, 'clipboard.png');
  check('响应 type=image', r.json.type, 'image');
}

console.log('\n── 6. name 参数被保留 ──');
{
  const { env, db } = makeEnv();
  await call(env, '/upload/raw?room=default&name=my%20notes.txt', 'note body');
  check('D1 name 用传入的名字', lastMessage(db).name, 'my notes.txt');
}

console.log('\n── 7. base64 端点 ──');
{
  const { env, db } = makeEnv();
  const r = await call(env, '/upload/base64?room=default', Buffer.from('base64 payload').toString('base64'), { base64: true });
  check('HTTP 200', r.status, 200);
  check('D1 content 解码正确', lastMessage(db).content, 'base64 payload');
}
{
  const { env, db } = makeEnv();
  await call(env, '/upload/base64?room=default', 'data:text/plain;base64,' + Buffer.from('with prefix').toString('base64'), { base64: true });
  check('data: 前缀被剥离', lastMessage(db).content, 'with prefix');
}
{
  const { env } = makeEnv();
  const r = await call(env, '/upload/base64?room=default', '!!!not-base64!!!', { base64: true });
  check('非法 base64 → 400', r.status, 400);
  check('错误码', r.json.error, 'Invalid base64');
}

console.log('\n── 8. 认证失败被拦下 ──');
{
  const { env, db } = makeEnv();
  const r = await call(env, '/upload/raw?room=default', 'should not be stored', { auth: 'Bearer wrong-password' });
  check('非 200', r.status !== 200, true);
  check('D1 未写入', db.prepare('SELECT COUNT(*) AS c FROM messages').get().c, 0);
}

console.log('\n── 9. 空文本与超限文本 ──');
{
  const { env, db } = makeEnv();
  await call(env, '/upload/raw?room=default', '');
  const m = lastMessage(db);
  console.log(`  · 空体结果: HTTP 状态见上，D1 行数=${db.prepare('SELECT COUNT(*) AS c FROM messages').get().c}`);
}
{
  const { env, db } = makeEnv();
  const long = 'x'.repeat(50000); // 超过 TEXT_LIMIT=40960
  const r = await call(env, '/upload/raw?room=default', long);
  const m = lastMessage(db);
  check('超限文本转为文件', m.type, 'file');
  check('文件名为 clipboard.txt', m.name, 'clipboard.txt');
}

console.log(`\n通过 ${pass} 项，失败 ${fail} 项`);
if (failures.length) {
  console.log('\n失败明细：');
  for (const f of failures) console.log('  ✗ ' + f);
  process.exit(1);
}
console.log('端到端全绿 ✅');
