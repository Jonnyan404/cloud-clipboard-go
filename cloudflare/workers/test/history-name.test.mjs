// 历史里的文件名必须是原始文件名。
//
// 为什么需要这个测试：历史消息曾经由 Durable Object 在**握手里**推，而那条路径把图标
// 拼进 name（`${icon} ${row.name}`）—— 前端本来就按扩展名自己渲染图标，于是每个文件名
// 前面都多出一个图标（多数文件是 📄），而且前端下载时 anchor.download 会拿这个被污染的
// 名字当文件名。实时消息和 HTTP 投影都是原始文件名，只有握手那条路径拼过，
// 所以症状是「刷新后才多出来」，很容易被当成前端问题。
//
// ⚠️★ **握手已经不推历史了**（历史一律走 `GET /content`），所以这条契约现在钉在
// `/content` 上 —— 那已经是客户端看到历史文件名的**唯一**路径。测试跟着搬过来，
// 否则「握手那条路径没了」会连带把这条契约一起丢掉（那才是真正危险的地方）。
//
// 走**真 router**（`worker.fetch`）而不是直接调处理器：这样连「`/content` 这条路由
// 真的注册了」一起钉住 —— 漏注册的后果是请求落到 SPA 兜底、拿回一份 HTML。
import worker from './.build/index.mjs';
import { makeEnv, makeChecker } from './harness.mjs';

const check = makeChecker();
const { env, db } = makeEnv();

// 三条历史：两个文件 + 一条文本。文件名故意用中英文各一个、且带扩展名，
// 这样如果服务端又去拼图标，断言会直接抓到。
db.exec(`INSERT INTO messages (type, content, name, size, room, timestamp, senderIP, userAgent, uuid, expireTime, url) VALUES
  ('file', NULL, '报告 2026.pdf', 123, 'default', 1789000000, '1.2.3.4', 'UA', 'uuid-1', 0, 'http://x/file/uuid-1/a'),
  ('file', NULL, 'photo.png', 456, 'default', 1789000001, '1.2.3.4', 'UA', 'uuid-2', 0, 'http://x/file/uuid-2/b'),
  ('text', 'hi', NULL, NULL, 'default', 1789000002, '1.2.3.4', 'UA', NULL, NULL, NULL)`);

const res = await worker.fetch(
  new Request('http://worker.local/content?room=default', { headers: { Authorization: 'Bearer 123' } }),
  env,
  {},
);
const body = await res.json();
const messages = body.messages || [];
// 只有文件条目才带 `name`（文本条目没有）—— 用这个过滤，不依赖 `type` 的具体取值
// （`/content` 的 `type` 是 `determineFileType(name)` 的产物，可能不是字面的 "file"）。
const fileMessages = messages.filter(m => m.name);

check.check('HTTP 200（路由注册了、没落到 SPA 兜底）', res.status, 200);

check.check(
  '历史里的文件名原样透出（没有被拼上图标）',
  fileMessages.map(m => m.name),
  ['报告 2026.pdf', 'photo.png'],
);

check.check(
  '历史里没有任何名字以图标开头',
  fileMessages.some(m => /^[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]\uFE0F?\s/u.test(String(m.name || ''))),
  false,
);

check.check(
  '历史里的 file 条目仍带 uuid / cache / size',
  fileMessages.map(m => [m.uuid, m.cache, m.size]),
  [['uuid-1', 'uuid-1', 123], ['uuid-2', 'uuid-2', 456]],
);

check.check('文本消息不受影响', messages.some(m => m.type === 'text' && m.content === 'hi'), true);

check.summary('历史消息文件名（走 /content）');
