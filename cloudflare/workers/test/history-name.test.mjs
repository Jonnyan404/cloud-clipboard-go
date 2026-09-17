// 历史消息里的文件名必须是原始文件名。
//
// 为什么需要这个测试：Durable Object 发历史消息时曾经把图标拼进 name
// （`${icon} ${row.name}`），而前端本来就按扩展名自己渲染图标 —— 于是每个文件名
// 前面都多出一个图标（多数文件是 📄），并且前端下载时 anchor.download 会拿这个
// 被污染的名字当文件名。实时消息和 HTTP 的 /content/latest 都是原始文件名，
// 只有历史这条路径拼过，所以症状是「刷新后才多出来」，很容易被当成前端问题。
import { WebSocketRoom } from './.build/websocket-room.mjs';
import { makeEnv, makeChecker } from './harness.mjs';

const check = makeChecker();
const { env, db } = makeEnv();

// 三条历史：两个文件 + 一条文本。文件名故意用中英文各一个、且带扩展名，
// 这样如果服务端又去拼图标，断言会直接抓到。
db.exec(`INSERT INTO messages (type, content, name, size, room, timestamp, senderIP, userAgent, uuid, expireTime, url) VALUES
  ('file', NULL, '报告 2026.pdf', 123, 'default', 1789000000, '1.2.3.4', 'UA', 'uuid-1', 0, 'http://x/file/uuid-1/a'),
  ('file', NULL, 'photo.png', 456, 'default', 1789000001, '1.2.3.4', 'UA', 'uuid-2', 0, 'http://x/file/uuid-2/b'),
  ('text', 'hi', NULL, NULL, 'default', 1789000002, '1.2.3.4', 'UA', NULL, NULL, NULL)`);

// 直接借原型造实例，省掉 Durable Object 的 state/ctx 依赖。
const room = Object.create(WebSocketRoom.prototype);
room.env = env;
room.sessions = new Map();

const sent = [];
const fakeSocket = {
  readyState: WebSocket.OPEN,
  send: payload => sent.push(JSON.parse(payload)),
};

await room.sendHistoryMessages(fakeSocket, 'default');

const fileMessages = sent.filter(m => m.event === 'receive' && m.data?.type === 'file');

check.check(
  '历史里的文件名原样透出（没有被拼上图标）',
  fileMessages.map(m => m.data.name),
  ['报告 2026.pdf', 'photo.png'],
);

check.check(
  '历史里没有任何名字以图标开头',
  fileMessages.some(m => /^[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]\uFE0F?\s/u.test(String(m.data.name || ''))),
  false,
);

check.check(
  '历史里的 file 消息仍带 uuid / cache / size',
  fileMessages.map(m => [m.data.uuid, m.data.cache, m.data.size]),
  [['uuid-1', 'uuid-1', 123], ['uuid-2', 'uuid-2', 456]],
);

check.check('文本消息不受影响', sent.some(m => m.data?.type === 'text' && m.data.content === 'hi'), true);

check.summary('历史消息文件名');
