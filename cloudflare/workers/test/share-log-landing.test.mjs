// 分享记录（/share/list、/share/visit）与 OG 落地页（/s/<token>）在 Worker 上的行为。
//
// 三件事必须钉住，它们都是「做错了会静默泄漏或静默失效」的地方：
//   1. 记录列表按房间隔离、不回 token（token 是 bearer 凭据）；
//   2. 落地页给抓取程序的摘要里**不放**带密码的分享内容，也不因为爬虫访问而计数；
//   3. /s/<token> 这条路由真的被 Worker 接住了（没被静态资源的兜底吃掉）。
import { makeEnv, makeChecker } from './harness.mjs';
import { issueRoomSessionToken } from './.build/auth.mjs';
import { ShareHandler, validateShareToken } from './.build/share.mjs';
import { handleShareLanding } from './.build/share-landing.mjs';
import worker from './.build/index.mjs';

const { check, summary } = makeChecker();
const now = Math.floor(Date.now() / 1000);

function seed(db) {
  const insert = db.prepare(
    `INSERT INTO messages (id, type, content, name, size, room, timestamp, senderIP, uuid, expireTime)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  );
  insert.run(1, 'text', '招行 APP 登录密码\n账号 6225********1234', null, null, 'default', now, '127.0.0.1', null, null);
  insert.run(2, 'file', null, 'photo.png', 2048, 'default', now, '127.0.0.1', 'uuid-1', now + 600);
  insert.run(3, 'text', '私密房间的正文', null, null, 'private', now, '127.0.0.1', null, null);
}

async function postShare(env, body, query = '', { auth = 'Bearer 123' } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth) headers.Authorization = auth;
  const req = new Request(`http://worker.local/share${query}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
  const res = await ShareHandler.create(req, env);
  return { status: res.status, json: await res.json() };
}

async function postVisit(env, body, { ip = '203.0.113.9' } = {}) {
  const req = new Request('http://worker.local/share/visit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'CF-Connecting-IP': ip },
    body: JSON.stringify(body),
  });
  const res = await ShareHandler.visit(req, env);
  return { status: res.status, json: await res.json() };
}

async function getList(env, query, { auth = 'Bearer 123' } = {}) {
  const headers = {};
  if (auth) headers.Authorization = auth;
  const req = new Request(`http://worker.local/share/list${query}`, { method: 'GET', headers });
  const res = await ShareHandler.list(req, env);
  return { status: res.status, json: await res.json() };
}

async function getLanding(env, token, query = '') {
  const req = new Request(`http://worker.local/s/${encodeURIComponent(token)}${query}`);
  const res = await handleShareLanding(req, env);
  return { status: res.status, html: await res.text(), headers: res.headers };
}

// 「注入了卡片的外壳」：有挂载点 + 有 `<base>`。
// 这是正常路径的样子，和「纯卡片兜底」以及「被静态资源原样回出来的 index.html」都不同。
function shellInjected(html) {
  return html.includes('<div id="app">') && html.includes('<base href="/">');
}

function ogField(html, prop) {
  const needle = `property="${prop}" content="`;
  const idx = html.indexOf(needle);
  if (idx < 0) return null;
  return html.slice(idx + needle.length).split('"')[0];
}

// ── 签发即入档 ──────────────────────────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);

  const created = await postShare(env, { type: 'content', id: '1', ttl: 600 });
  check('签发：HTTP 200', created.status, 200);
  check('签发：不限次数的分享也有 jti', typeof created.json.jti === 'string' && created.json.jti.length > 0, true);
  check('签发：链接就是 /s/<token>', created.json.url.endsWith(`/s/${encodeURIComponent(created.json.token)}`), true);
  check('签发：url 与 pageUrl 同值（单一地址）', created.json.url === created.json.pageUrl, true);
  check('签发：地址里没有 #', created.json.url.includes('#'), false);
  check('签发：初始打开次数为 0', [created.json.visits, created.json.scans], [0, 0]);

  const row = db.prepare('SELECT * FROM share_log WHERE jti = ?').get(created.json.jti);
  check('入档：记下类型', row.type, 'content');
  check('入档：记下内容类型', row.kind, 'text');
  check('入档：名称取首行摘要', row.name, '招行 APP 登录密码');
  check('入档：房间', row.room, 'default');
}

// ── 计数与去重、扫码 ────────────────────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);
  const share = await postShare(env, { type: 'content', id: '1', ttl: 600 });

  const first = await postVisit(env, { token: share.json.token });
  check('上报：HTTP 200', first.status, 200);
  check('上报：第一次被计入', [first.json.tracked, first.json.visits], [true, 1]);

  const again = await postVisit(env, { token: share.json.token });
  check('上报：同一访客重复上报被去重', [again.json.tracked, again.json.visits], [false, 1]);

  const other = await postVisit(env, { token: share.json.token }, { ip: '203.0.113.10' });
  check('上报：换个访客算一次新打开', other.json.visits, 2);

  const qr = await postVisit(env, { token: share.json.token, qr: true }, { ip: '203.0.113.11' });
  check('上报：二维码额外计一次扫码', [qr.json.visits, qr.json.scans], [3, 1]);

  const bad = await postVisit(env, { token: 'not-a-token' });
  check('上报：坏 token → 401', bad.status, 401);
}

// ── 记录列表：房间隔离 + 不回 token ─────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);
  // 全局密码留空 = default 房间真的是开放的；private 房间单独设密码
  env.AUTH_PASSWORD = '';
  env.ROOM_AUTH_JSON = JSON.stringify({ private: 'private-pass' });

  const privateToken = await issueRoomSessionToken(env, 'private', 3600, '');
  const open = await postShare(env, { type: 'content', id: '1', ttl: 600 });
  const privateShare = await postShare(
    env,
    { type: 'content', id: '3', ttl: 600 },
    '?room=private',
    { auth: `Bearer ${privateToken}` },
  );
  check('私密房间：带凭据能签发', privateShare.status, 200);
  await postVisit(env, { token: open.json.token });
  await postVisit(env, { token: open.json.token, qr: true }, { ip: '203.0.113.11' });

  const noAuth = await getList(env, '?room=private', { auth: null });
  check('列表：没凭据读私密房间 → 401', noAuth.status, 401);

  const openList = await getList(env, '?room=default', { auth: null });
  check('列表：开放房间不需要凭据（与签发同一套规则）', openList.status, 200);
  check('列表：只含本房间的记录', openList.json.records.length, 1);
  check('列表：带上打开次数', openList.json.records[0].visits, 2);
  check('列表：带上扫码次数', openList.json.records[0].scans, 1);
  check('列表：不回 token', JSON.stringify(openList.json).includes(open.json.token), false);
  check('列表：不含其它房间的正文摘要', JSON.stringify(openList.json).includes('私密房间的正文'), false);

  const privateList = await getList(env, '?room=private', { auth: `Bearer ${privateToken}` });
  check('列表：带凭据能读私密房间', privateList.status, 200);
  check('列表：私密房间看到的是自己的记录', privateList.json.records[0].name, '私密房间的正文');

  const limited = await getList(env, '?room=default&limit=99999', { auth: null });
  check('列表：limit 被夹到上限', limited.json.limit, 200);
}

// ── 落地页：文本摘要 ────────────────────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);
  const share = await postShare(env, { type: 'content', id: '1', ttl: 600 });

  const landing = await getLanding(env, share.json.token);
  check('落地页：HTTP 200', landing.status, 200);
  check('落地页：标题取首行', ogField(landing.html, 'og:title'), '招行 APP 登录密码');
  // 第二行（账号）刻意不进预览：摘要只取首行，别把整段正文搬进第三方缓存
  check('落地页：第二行不进预览', landing.html.includes('6225'), false);
  check('落地页：og:url 是落地页自己', ogField(landing.html, 'og:url'), `http://worker.local/s/${encodeURIComponent(share.json.token)}`);
  check('落地页：带 noindex', landing.headers.get('X-Robots-Tag').includes('noindex'), true);
  check('落地页：带 no-referrer（地址里有 token，别流到第三方）', landing.headers.get('Referrer-Policy'), 'no-referrer');
  check('落地页：带 nosniff', landing.headers.get('X-Content-Type-Options'), 'nosniff');
  // 正常路径 = 「SPA 外壳 + 注入的卡片」：真人由前端路由 /s/:token 接管，**没有第二跳**。
  check('落地页：回的是注入了卡片的外壳', shellInjected(landing.html), true);
  check('落地页：外壳的相对资源引用保留', landing.html.includes('./assets/'), true);
  check('落地页：标题被换成了分享标题', landing.html.includes('<title>招行 APP 登录密码</title>'), true);

  // 扫码那条地址带 ?q=1：前端直接从 location.search 读 route.query.q，落地页**不再转手**，
  // 所以 q 不该污染 og:url（它只影响前端上报）。
  const scanned = await getLanding(env, share.json.token, '?q=1');
  check('落地页：q=1 时仍是同一份外壳', shellInjected(scanned.html), true);
  check('落地页：og:url 不带扫码标记', ogField(scanned.html, 'og:url'), `http://worker.local/s/${encodeURIComponent(share.json.token)}`);

  // 抓取程序会反复访问：绝不能计数
  await getLanding(env, share.json.token);
  await getLanding(env, share.json.token);
  const row = db.prepare('SELECT visits, scans FROM share_log WHERE jti = ?').get(share.json.jti);
  check('落地页：爬虫访问不计入打开次数', [row.visits, row.scans], [0, 0]);

  // 也不能消耗使用次数：两次预取之后，maxUses=2 的额度必须还完整
  const limitedShare = await postShare(env, { type: 'content', id: '1', ttl: 600, maxUses: 2 });
  await getLanding(env, limitedShare.json.token);
  await getLanding(env, limitedShare.json.token);
  const takeContent = () => new Request(
    `http://worker.local/content/1?t=${encodeURIComponent(limitedShare.json.token)}`,
  );
  check('落地页：预取没吃掉第 1 次额度', await validateShareToken(env, takeContent(), 'content', '1', 'default'), true);
  check('落地页：预取没吃掉第 2 次额度', await validateShareToken(env, takeContent(), 'content', '1', 'default'), true);
  check('落地页：第 3 次才被 maxUses=2 拦下', await validateShareToken(env, takeContent(), 'content', '1', 'default'), false);
}

// ── 落地页：密码 / 失效 / 文件 ──────────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);

  const locked = await postShare(env, { type: 'content', id: '1', ttl: 600, password: 'hunter2' });
  const lockedLanding = await getLanding(env, locked.json.token);
  check('落地页：带密码 → 不放内容摘要', lockedLanding.html.includes('6225') || lockedLanding.html.includes('登录密码'), false);
  check('落地页：带密码 → 说清楚需要密码', ogField(lockedLanding.html, 'og:title'), '受密码保护的分享');

  const broken = await getLanding(env, 'garbage.token');
  check('落地页：坏 token → 通用卡片', ogField(broken.html, 'og:title'), '分享链接无效或已过期');
  // 有外壳时**仍然是外壳**（只是卡片换成通用文案）——真人照样跑起前端看到可读的错误页。
  // 纯卡片只在**没有外壳**时才用，别搞反。
  check('落地页：坏 token 仍回外壳', shellInjected(broken.html), true);

  const missing = await postShare(env, { type: 'content', id: '999', ttl: 600 });
  check('签发：内容不存在 → 404', missing.status, 404);

  // 文件分享：标题是文件名，描述里有大小；图片才给 og:image
  const fileShare = await postShare(env, { type: 'file', uuid: 'uuid-1', ttl: 600 });
  const fileLanding = await getLanding(env, fileShare.json.token);
  check('落地页：文件标题是文件名', ogField(fileLanding.html, 'og:title'), 'photo.png');
  check('落地页：描述带大小', ogField(fileLanding.html, 'og:description').includes('2.0KB'), true);
  check('落地页：图片给 og:image', String(ogField(fileLanding.html, 'og:image')).includes('/file/uuid-1/photo.png?t='), true);

  // 限次的分享**不给** og:image：抓取程序抓图会烧掉一次使用额度
  const limitedFile = await postShare(env, { type: 'file', uuid: 'uuid-1', ttl: 600, maxUses: 3 });
  const limitedLanding = await getLanding(env, limitedFile.json.token);
  check('落地页：限次分享不给 og:image', ogField(limitedLanding.html, 'og:image'), null);
}

// ── 路由：/s/<token> 必须被 Worker 接住（不能被静态资源兜底成 index.html）──
{
  const { env, db } = makeEnv();
  seed(db);
  const share = await postShare(env, { type: 'content', id: '1', ttl: 600 });

  const res = await worker.fetch(new Request(`http://worker.local/s/${encodeURIComponent(share.json.token)}`), env, {});
  const html = await res.text();
  check('路由：/s/<token> 命中落地页', res.headers.get('Content-Type').startsWith('text/html'), true);
  check('路由：渲染出 OG 标题', ogField(html, 'og:title'), '招行 APP 登录密码');
  // 落地页**就是**外壳 —— 但必须是「注入了 base + OG 标签」的那一份，
  // 而不是被静态资源原样回出来的 index.html。区别就在注入上。
  check('路由：落地页就是外壳', html.includes('<div id="app">'), true);
  check('路由：外壳带了 base', html.includes('<base href="/">'), true);
  check('路由：外壳里注入了 noindex', html.includes('name="robots" content="noindex, nofollow"'), true);

  // 带全局密码：证明命中的是记录列表（而不是被 /share 抢走、也不是静态资源兜底）
  const listRes = await worker.fetch(new Request('http://worker.local/share/list?room=default', {
    headers: { Authorization: 'Bearer 123' },
  }), env, {});
  check('路由：/share/list 命中（未被 /share 抢先）', listRes.status, 200);
  check('路由：/share/list 返回记录', (await listRes.json()).records.length, 1);

  const noAuthList = await worker.fetch(new Request('http://worker.local/share/list?room=default'), env, {});
  check('路由：/share/list 受房间鉴权保护', noAuthList.status, 401);
  check('路由：/share/list 的 401 来自列表处理器', (await noAuthList.json()).code, 'room_forbidden');
}

// ── 没有资源层（前端没部署）→ 回落纯卡片 ────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);
  delete env.ASSETS;
  const share = await postShare(env, { type: 'content', id: '1', ttl: 600 });

  const landing = await getLanding(env, share.json.token);
  check('无外壳：HTTP 200', landing.status, 200);
  check('无外壳：仍给出 OG 标题', ogField(landing.html, 'og:title'), '招行 APP 登录密码');
  check('无外壳：退化成纯卡片', landing.html.includes('<div id="app">'), false);
  check('无外壳：没有 base 可注入', landing.html.includes('<base'), false);
  // 纯卡片里既没有按钮、也没有跳转脚本：真人那边本来就没有前端可以看。
  check('无外壳：没有跳转脚本', landing.html.includes('location.replace'), false);
  check('无外壳：没有「打开分享」按钮', landing.html.includes('share-open-link'), false);
  // 隐私头在两条路径上都要有
  check('无外壳：同样带 noindex', landing.headers.get('X-Robots-Tag').includes('noindex'), true);
}

// ── 兜底路由：深路径也要注入 base（否则 history 路由刷新白屏）───────────
{
  const { env, db } = makeEnv();
  seed(db);

  const res = await worker.fetch(new Request('http://worker.local/some/deep/path'), env, {});
  const html = await res.text();
  check('兜底：HTTP 200', res.status, 200);
  check('兜底：回的是外壳', html.includes('<div id="app">'), true);
  check('兜底：注入了 base', html.includes('<base href="/">'), true);
  // 兜底是**前端路由**的兜底，不该带分享的 OG 卡片。
  check('兜底：不带 og:title', ogField(html, 'og:title'), null);

  const posted = await worker.fetch(
    new Request('http://worker.local/some/deep/path', { method: 'POST' }),
    env,
    {},
  );
  check('兜底：非 GET/HEAD 明确 404', posted.status, 404);
}

summary('分享记录 / 计数 / OG 落地页在 Worker 上成立');
