// `GET /content`（历史分页）+ WS `?history=0` + `config.latestId` —— W2 的三件事。
//
// 规格：`docs/specs/ws-live-only.md`（三边同一份契约）。断言照 §5 的验收条目来，
// 不做「读代码觉得对」那类检查。
//
// 为什么 Worker 侧必须自己测一遍：Rust 那边靠「对着 Go 的 fixture 比投影 + 双跑比对」，
// 而 Worker 既跑不进 `tools/compare-with-go.mjs`（那要起 Go/Rust 两个进程），
// 又没有 Go 的 fixture 可读（它是 D1，不是 redb）—— 所以这几条只能在
// 「mock 的 D1 + 真的处理器」上验。这正是 `test/run.sh` 一贯的做法。
import { TextHandler } from './.build/text.mjs';
import { WebSocketRoom } from './.build/websocket-room.mjs';
import worker from './.build/index.mjs';
import { makeEnv, makeChecker, postJson } from './harness.mjs';

const { check, summary } = makeChecker();

// 走**真 router**（`worker.fetch`）而不是直接调处理器：这样能一并钉住
// 「`/content` 这条路由真的注册了」—— 漏注册的后果是请求落到 SPA 兜底、
// 拿回一份 HTML（spec §0.3 第 3 条记的就是这个坑），而直接调处理器**测不出来**。
async function fetchJson(env, path, { auth = 'Bearer 123' } = {}) {
  const headers = {};
  if (auth) headers.Authorization = auth;
  const res = await worker.fetch(new Request(`http://worker.local${path}`, { headers }), env, {});
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text };
}

// 一边发一边收 id：`/text` 的响应里带 `id`，比事后猜 autoincrement 稳。
async function seed(env, texts, room = 'default') {
  const ids = [];
  for (const t of texts) {
    const r = await postJson(TextHandler.create, env, `/text?room=${room}`, t);
    ids.push(String(r.json.id));
  }
  return ids;
}

// 造一个 Durable Object 实例（借原型，省掉 state/ctx 依赖）—— 与 history-name.test.mjs 同一招。
// ⚠️ 握手那一段之所以不能走 router：它需要一个真的 `WebSocketPair`，Node 里造不出来，
// 所以 `sendHandshake` 被抽成了「只吃一个能 send 的对象 + 一个 Request」的形状。
function makeRoom(env) {
  const room = Object.create(WebSocketRoom.prototype);
  room.env = env;
  room.sessions = new Map();
  return room;
}

function collect() {
  const sent = [];
  const socket = {
    readyState: WebSocket.OPEN,
    send: payload => sent.push(JSON.parse(payload)),
  };
  return { sent, socket };
}

console.log('\n── A. 基本形状：最新 limit 条、正序（旧的在前）──');
{
  const { env } = makeEnv();
  await seed(env, ['第一条', '第二条', '第三条']);

  const r = await fetchJson(env, '/content?room=default&limit=2');
  check('HTTP 200（路由注册了、没落到 SPA 兜底）', r.status, 200);
  check('响应就是 {messages:[...]}（没有 hasMore / nextBefore）', Object.keys(r.json), ['messages']);
  check('条数被 limit 收住', r.json.messages.length, 2);
  // ⚠️ 正序：客户端是一条条 append 渲染的，倒过来不会报错、只是列表顺序不对。
  check('正序（旧的在前）', r.json.messages.map(m => m.content), ['第二条', '第三条']);
  check('id 是字符串（与 /content/<id> 同形）', typeof r.json.messages[0].id, 'string');
}

console.log('\n── B. 游标是 id：拿 messages[0].id 往回翻一页，且不重复 ──');
{
  const { env } = makeEnv();
  await seed(env, ['第一条', '第二条', '第三条']);

  const page1 = await fetchJson(env, '/content?room=default&limit=2');
  const cursor = page1.json.messages[0].id;
  const page2 = await fetchJson(env, `/content?room=default&limit=2&before=${cursor}`);

  check('第二页拿到更早那条', page2.json.messages.map(m => m.content), ['第一条']);
  const ids = [...page1.json.messages, ...page2.json.messages].map(m => m.id);
  check('两页之间不重复', new Set(ids).size, ids.length);
}

console.log('\n── C. 游标失效（那条已被撤销 / 认不出）不报错 ──');
{
  const { env, db } = makeEnv();
  const ids = await seed(env, ['第一条', '第二条', '第三条']);
  // 直接删掉中间那条：客户端手里的游标指向一条**已经不在了**的消息，这很正常。
  db.exec(`DELETE FROM messages WHERE id = ${Number(ids[1])}`);

  const r = await fetchJson(env, `/content?room=default&limit=2&before=${ids[1]}`);
  check('指向已删除的 id → 不是 5xx', r.status, 200);
  check('拿到的是它之前那条', r.json.messages.map(m => m.content), ['第一条']);

  const bad = await fetchJson(env, '/content?room=default&limit=2&before=abc');
  check('认不出的游标 → 不是 5xx', bad.status, 200);
  check('退化成「最近 limit 条」', bad.json.messages.map(m => m.content), ['第一条', '第三条']);
}

console.log('\n── D. limit 被 history 上限夹住（缺省值与上限是同一个数）──');
{
  const { env } = makeEnv();
  env.HISTORY_LIMIT = '3';
  await seed(env, ['一', '二', '三', '四', '五']);

  const huge = await fetchJson(env, '/content?room=default&limit=999999');
  check('limit=999999 被夹住（不是想给多大给多大）', huge.json.messages.length, 3);

  const dflt = await fetchJson(env, '/content?room=default');
  check('不传 limit 与传上限拿到同一个量', dflt.json.messages.length, 3);

  const bad = await fetchJson(env, '/content?room=default&limit=-1');
  check('非正的 limit 也取上限', bad.json.messages.length, 3);
}

console.log('\n── D2. 硬上限：配置再大，一次也不会返回一整间房 ──');
{
  // ⚠️★ 这条防的是「配置设成 10000，于是 /content 一次返回 10000 条」——
  // 那正是这个变更要消灭的「一次推 2MB」，只是从 WS 挪到了 HTTP，**等于没改**。
  const { env, db } = makeEnv();
  env.HISTORY_LIMIT = '10000';

  const values = [];
  for (let i = 1; i <= 101; i += 1) {
    values.push(`('text', 'm${i}', 'default', ${1789000000 + i}, 'unknown', '', 'UA')`);
  }
  db.exec(`INSERT INTO messages (type, content, room, timestamp, senderIP, senderClientID, userAgent)
    VALUES ${values.join(',')}`);

  const huge = await fetchJson(env, '/content?room=default&limit=999999');
  check('配置 10000 时，一次最多 100 条', huge.json.messages.length, 100);
  check('取的是**最近**那 100 条', huge.json.messages[huge.json.messages.length - 1].content, 'm101');

  const dflt = await fetchJson(env, '/content?room=default');
  check('缺省也取有效上限 min(10000,100)，而不是 10000', dflt.json.messages.length, 100);

  // 硬上限不等于「看不到更早的」—— 那正是 `before` 游标存在的意义。
  const older = await fetchJson(
    env, `/content?room=default&before=${huge.json.messages[0].id}`,
  );
  check('用 before 仍然能翻到更早的', older.json.messages.map(m => m.content), ['m1']);
}

console.log('\n── E. 鉴权与 /content/latest 同一套（同一错误码、同一文案）──');
{
  const { env } = makeEnv();
  await seed(env, ['一条']);

  const list = await fetchJson(env, '/content?room=default', { auth: null });
  const latest = await fetchJson(env, '/content/latest?room=default', { auth: null });

  check('两个端点的状态码一致', list.status, latest.status);
  check('错误码一致', list.json.code, latest.json.code);
  check('英文文案一致', list.json.error, latest.json.error);
  check('中文文案一致', list.json.message, latest.json.message);
  check('确实是 401', list.status, 401);

  // 带上正确凭据就该通（否则上面那条可能只是因为「两个都恰好 401」）。
  const ok = await fetchJson(env, '/content?room=default');
  check('带凭据 → 200', ok.status, 200);
}

console.log('\n── F. 列表里的条目与 /content/<id> 同一个形状 ──');
{
  const { env } = makeEnv();
  const ids = await seed(env, ['形状比对']);

  const list = await fetchJson(env, '/content?room=default');
  const one = await fetchJson(env, `/content/${ids[0]}?room=default&format=json`);

  check('单条端点也是 200', one.status, 200);
  check('列表里的那条 = 单条取出来的那条', list.json.messages[0], one.json);
  // §0.5：目标形状是 WS `receive` 的载荷本身（SPA 就是按那个形状渲染历史的）。
  check('带 senderIP', typeof list.json.messages[0].senderIP, 'string');
  check('带 senderDevice', typeof list.json.messages[0].senderDevice, 'object');
  check('带 room', list.json.messages[0].room, 'default');
  check('带 column', list.json.messages[0].column, '');
}

console.log('\n── G. WS `?history=0`：只收到 config，一条 receive 都没有 ──');
{
  const { env } = makeEnv();
  await seed(env, ['历史一', '历史二']);

  const withHistory = collect();
  await makeRoom(env).sendHandshake(
    withHistory.socket, 'default', new Request('https://x/push?room=default'), 'sid-a',
  );
  check('不带开关 → 默认仍然推历史（老客户端行为不变）',
    withHistory.sent.map(m => m.event), ['receive', 'receive', 'config']);
  check('config 在历史之后（顺序是契约）',
    withHistory.sent.findIndex(m => m.event === 'config'), 2);

  const skip = collect();
  await makeRoom(env).sendHandshake(
    skip.socket, 'default', new Request('https://x/push?room=default&history=0'), 'sid-b',
  );
  check('带 history=0 → 一条 receive 都没有', skip.sent.filter(m => m.event === 'receive'), []);
  check('其余不变（config 照发）', skip.sent.map(m => m.event), ['config']);
}

console.log('\n── H. `config.latestId`：有消息 = 该房间最大 id；空房间 = 0 ──');
{
  const { env } = makeEnv();
  const ids = await seed(env, ['一', '二', '三']);

  const sent = collect();
  await makeRoom(env).sendHandshake(
    sent.socket, 'default', new Request('https://x/push?room=default&history=0'), 'sid-c',
  );
  const cfg = sent.sent.find(m => m.event === 'config');
  check('latestId = 最大 id', cfg.data.latestId, Number(ids[ids.length - 1]));

  const empty = collect();
  await makeRoom(env).sendHandshake(
    empty.socket, '空房间', new Request('https://x/push?room=空房间&history=0'), 'sid-d',
  );
  check('空房间 = 0', empty.sent.find(m => m.event === 'config').data.latestId, 0);

  // ⚠️ 水印只该出现在**握手**载荷里：`/server` 上加一个永远不会被读到的字段，
  // 正是这个坑（`automation.enabled` / `prefix` 踩过两次）。
  const serverRes = await worker.fetch(new Request('http://worker.local/server'), env, {});
  const serverBody = await serverRes.json();
  check('/server 里没有 latestId', Object.prototype.hasOwnProperty.call(serverBody, 'latestId'), false);
  check('/server 里的 history 与握手 config 是同一个数（同一根旋钮）', serverBody.history, 50);
}

console.log('\n── I. 房间隔离：看不到别的房间的历史 ──');
{
  const { env } = makeEnv();
  await seed(env, ['default 里的'], 'default');
  await seed(env, ['work 里的'], 'work');

  const r = await fetchJson(env, '/content?room=work');
  check('只拿到本房间的', r.json.messages.map(m => m.content), ['work 里的']);
}

console.log('\n── J. 缺省就是 50（三端统一的那根旋钮）──');
{
  // 完全不配 HISTORY_LIMIT：有效值应当是 50，与 Go / Rust 的内置缺省一致。
  // 这条钉的是「Jonny 2026-09-26 定：三端统一 50」—— 以前 Worker 缺省是 10，
  // 于是同一个客户端换后端会少看一截历史。
  const { env } = makeEnv();
  delete env.HISTORY_LIMIT;

  const serverRes = await worker.fetch(new Request('http://worker.local/server'), env, {});
  check('/server 的 history 缺省 = 50', (await serverRes.json()).history, 50);

  const sent = collect();
  await makeRoom(env).sendHandshake(
    sent.socket, '默认房间', new Request('https://x/push?room=默认房间&history=0'), 'sid-e',
  );
  check('握手 config 的 history 缺省 = 50',
    sent.sent.find(m => m.event === 'config').data.server.history, 50);
}

summary('WS 只推实时 / 历史走 HTTP（Worker 侧）');
