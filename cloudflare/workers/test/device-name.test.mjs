// 验证设备名（?name=）在 Worker 后端上的完整链路：
//   /text、/upload 带 name → 落库 → /content/latest?json=1 回读 → 前端拿到 senderDevice.name
//
// 特别要盯住「自愈加列」这条路径：线上已有部署的 messages 表里没有 deviceName 列，
// 靠 saveToD1 里那次幂等 ALTER 补上。harness 的表结构刻意停留在加列之前，
// 就是为了让这段逻辑每次都真的被走到 —— 它坏了不会报错，只会静默丢名字。
import { TextHandler } from './.build/text.mjs';
import { FileHandler } from './.build/file.mjs';
import { ContentHandler } from './.build/content.mjs';
import { makeEnv, makeChecker, postJson, getJson } from './harness.mjs';

const { check, summary } = makeChecker();

console.log('\n── A. 带 name 发送文本 → 回读应当拿到该名字 ──');
{
  const { env } = makeEnv();
  await postJson(
    TextHandler.create,
    env,
    '/text?room=default&client=ios-shortcuts&name=iOS%20%E5%BF%AB%E6%8D%B7%E6%8C%87%E4%BB%A4',
    '来自快捷指令'
  );
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('HTTP 200', r.status, 200);
  check('senderDevice.name 保真（中文未被破坏）', r.json.senderDevice.name, 'iOS 快捷指令');
  check('type 仍然保留', typeof r.json.senderDevice.type, 'string');
}

console.log('\n── B. 不带 name → 不写 name 字段，载荷与改动前一致 ──');
{
  const { env } = makeEnv();
  await postJson(TextHandler.create, env, '/text?room=default&client=web-abc', '普通浏览器发送');
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('name 字段不存在', r.json.senderDevice.name, undefined);
  check('os 仍在', typeof r.json.senderDevice.os, 'string');
  check('browser 仍在', typeof r.json.senderDevice.browser, 'string');
}

console.log('\n── C. 带 name 上传文件 → 文件消息同样带名字 ──');
{
  const { env } = makeEnv();
  const form = new FormData();
  form.append('file', new Blob([new Uint8Array([1, 2, 3])], { type: 'image/png' }), 'shot.png');
  await FileHandler.upload(
    new Request('http://worker.local/upload?room=default&client=ios-shortcuts&name=%E6%88%91%E7%9A%84%E6%A0%91%E8%8E%93%E6%B4%BE', {
      method: 'POST',
      headers: { Authorization: 'Bearer 123' },
      body: form,
    }),
    env
  );
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('文件名保真', r.json.name, 'shot.png');
  check('senderDevice.name 保真', r.json.senderDevice.name, '我的树莓派');
}

console.log('\n── D. name 被清洗：控制字符剔除、超长按字符截断 ──');
{
  // 每条断言各用一个新 env：timestamp 只精确到秒，同一秒内多条消息的
  // ORDER BY timestamp DESC 排序不稳定，混在一起会取错行。
  const a = makeEnv();
  await postJson(TextHandler.create, a.env, '/text?room=default&client=x&name=%20a%0Ab%0Dc%20', 'x');
  const ra = await getJson(ContentHandler.getLatest, a.env, '/content/latest?room=default&json=1');
  check('控制字符被剔除、首尾空白被裁掉', ra.json.senderDevice.name, 'abc');

  const b = makeEnv();
  await postJson(TextHandler.create, b.env, '/text?room=default&client=x&name=' + '%E5%B2%9A'.repeat(40), 'y');
  const rb = await getJson(ContentHandler.getLatest, b.env, '/content/latest?room=default&json=1');
  check('超长名字被截到 32 字符', rb.json.senderDevice.name, '岚'.repeat(32));
}

console.log('\n── E. 同一个库连续写入不会因重复加列而失败 ──');
{
  const { env } = makeEnv();
  for (let i = 0; i < 3; i++) {
    const res = await postJson(TextHandler.create, env, '/text?room=default&client=x&name=n' + i, 'msg' + i);
    check(`第 ${i + 1} 次写入成功`, res.status, 200);
  }
  // 直接按 id 取最后一行，避开 timestamp 同秒导致的排序歧义
  const last = await env.DB.prepare('SELECT deviceName FROM messages ORDER BY id DESC LIMIT 1').first();
  check('最后一条名字正确', last.deviceName, 'n2');
  const count = await env.DB.prepare('SELECT COUNT(*) AS c FROM messages').first();
  check('三条都真的落库了', count.c, 3);
}

summary('Worker 设备名链路');
