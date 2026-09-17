// 端到端验证 Cloudflare Worker 的 /upload/raw 与 /upload（multipart）胶水层：
// 请求包装 → 嗅探 → 委托给 TextHandler/FileHandler → D1/R2 落库 → 响应 JSON。
// 用 esbuild 打包后的处理器 + mock env（D1 用 node:sqlite，R2 用 Map）。
import { RawUploadHandler } from './.build/raw-upload.mjs';
import { FileHandler } from './.build/file.mjs';
import { makeEnv, makeChecker, postJson } from './harness.mjs';

const { check, summary } = makeChecker();

// PNG 头（够长以命中魔数表）
const PNG_BYTES = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]);

// 适配：统一走 RawUploadHandler.upload
async function call(env, path, body, { auth = 'Bearer 123' } = {}) {
  return postJson(RawUploadHandler.upload, env, path, body, { auth });
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

console.log('\n── 4. PNG 被嗅探为图片 ──');
{
  const { env, db } = makeEnv();
  const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]);
  const r = await call(env, '/upload/raw?room=default', png);
  const m = lastMessage(db);
  check('D1 type=file', m.type, 'file');
  check('D1 name=clipboard.png', m.name, 'clipboard.png');
  check('响应 type=image', r.json.type, 'image');
}

console.log('\n── 5. 认证失败被拦下 ──');
{
  const { env, db } = makeEnv();
  const r = await call(env, '/upload/raw?room=default', 'should not be stored', { auth: 'Bearer wrong-password' });
  check('非 200', r.status !== 200, true);
  check('D1 未写入', db.prepare('SELECT COUNT(*) AS c FROM messages').get().c, 0);
}

console.log('\n── 6. 空文本与超限文本 ──');
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

console.log('\n── 7. multipart 上传保留真实文件名（快捷指令文件分支走的就是这条）──');
{
  // 为什么重要：getName 拿不到扩展名、服务端按内容嗅探也只能给 txt，
  // 所以「文件名保真」只能靠 multipart 的 part 自带名字。
  const { env, db } = makeEnv();
  const form = new FormData();
  form.append('file', new Blob([Buffer.from('django==4.2.*')], { type: 'text/plain' }), 'requirements.md');
  const req = new Request('http://worker.local/upload?room=default&client=ios-shortcuts', {
    method: 'POST',
    headers: { Authorization: 'Bearer 123' },
    body: form,
  });
  const res = await FileHandler.upload(req, env);
  check('HTTP 200', res.status, 200);
  check('真实文件名保留（含扩展名）', lastMessage(db).name, 'requirements.md');
  check('存成文件', lastMessage(db).type, 'file');
}
{
  // 中文名与 .sh 也要保真 —— 这两个正是当初被嗅探成 txt 的场景
  const { env, db } = makeEnv();
  const form = new FormData();
  form.append('file', new Blob([Buffer.from('#!/bin/bash\n')], { type: 'text/plain' }), 'build.sh');
  const req = new Request('http://worker.local/upload?room=default', {
    method: 'POST',
    headers: { Authorization: 'Bearer 123' },
    body: form,
  });
  await FileHandler.upload(req, env);
  check('.sh 扩展名保留', lastMessage(db).name, 'build.sh');
}

summary('端到端全绿');
