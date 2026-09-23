// 分享页链路：POST /share 一律签发 token 且链接指向 /s/<token>，GET /share 提供元信息、且不消耗使用次数。
//
// 为什么单独一个文件：share.js 此前完全没有测试覆盖 —— 而它是「开放房间不发 token」
// 那个静默丢弃 TTL / 次数限制的 bug 的所在地。这里把修好之后的形状钉住。
import { makeEnv, makeChecker } from './harness.mjs';
import { ShareHandler, parseShareToken, validateShareToken } from './.build/share.mjs';

const { check, summary } = makeChecker();
const now = Math.floor(Date.now() / 1000);

function seed(db) {
  const insert = db.prepare(
    `INSERT INTO messages (id, type, content, name, size, room, timestamp, senderIP, uuid, expireTime)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  );
  insert.run(1, 'text', 'hello from the share page', null, null, 'default', now, '127.0.0.1', null, null);
  insert.run(2, 'file', null, 'photo.png', 4096, 'default', now, '127.0.0.1', 'uuid-1', now + 600);
}

async function postShare(env, body) {
  const req = new Request('http://worker.local/share', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer 123' },
    body: JSON.stringify(body),
  });
  const res = await ShareHandler.create(req, env);
  return { status: res.status, json: await res.json() };
}

async function getShare(env, token, password) {
  const headers = {};
  if (password) {
    headers['X-Share-Password'] = password;
  }
  const req = new Request(`http://worker.local/share?t=${encodeURIComponent(token)}`, { method: 'GET', headers });
  const res = await ShareHandler.info(req, env);
  return { status: res.status, json: await res.json() };
}

// ── 开放房间也必须签发 token ────────────────────────────────────────────
// 曾经只在房间需要鉴权时才发，于是开放房间的链接是裸接口地址：
// 弹窗里让用户设的 TTL / 次数限制被静默丢弃，而响应里照样回 ttl/maxUses。
{
  const { env, db } = makeEnv();
  seed(db);
  env.AUTH_PASSWORD = '';
  env.ROOM_AUTH_JSON = '{}';

  const open = await postShare(env, { type: 'content', id: '1', ttl: 60, maxUses: 2 });
  check('开放房间：HTTP 200', open.status, 200);
  check('开放房间：仍签发 token', typeof open.json.token === 'string' && open.json.token.length > 0, true);
  check('开放房间：链接指向 /s/<token>', open.json.url.endsWith(`/s/${encodeURIComponent(open.json.token)}`), true);
  // 单一地址：url 与 pageUrl 同值（pageUrl 保留只为兼容老客户端）
  check('开放房间：url 与 pageUrl 同值', open.json.url === open.json.pageUrl, true);
  check('开放房间：地址里没有 #', open.json.url.includes('#'), false);
  check('开放房间：链接不是裸接口地址', open.json.url.includes('/content/'), false);
  check('开放房间：rawUrl 指向正文接口', open.json.rawUrl.includes('/content/1'), true);
  check('开放房间：rawUrl 带同一个 token', open.json.rawUrl.includes(`t=${encodeURIComponent(open.json.token)}`), true);
  check('开放房间：expiresAt 在将来', open.json.expiresAt > now, true);
  check('开放房间：maxUses 原样回传', open.json.maxUses, 2);

  const info = await getShare(env, open.json.token);
  check('元信息：HTTP 200', info.status, 200);
  check('元信息：type', info.json.type, 'content');
  check('元信息：kind', info.json.kind, 'text');
  check('元信息：room', info.json.room, 'default');
  check('元信息：needsPassword', info.json.needsPassword, false);
  check('元信息：maxUses', info.json.maxUses, 2);

  // 连着看三次，然后确认两次取正文都还能过 —— 证明「看一眼」没吃掉次数
  await getShare(env, open.json.token);
  await getShare(env, open.json.token);
  await getShare(env, open.json.token);

  const contentReq = () => new Request(`http://worker.local/content/1?t=${encodeURIComponent(open.json.token)}`);
  check('取正文第 1 次成功', await validateShareToken(env, contentReq(), 'content', '1', 'default'), true);
  check('取正文第 2 次成功', await validateShareToken(env, contentReq(), 'content', '1', 'default'), true);
  check('取正文第 3 次被 maxUses=2 拦下', await validateShareToken(env, contentReq(), 'content', '1', 'default'), false);
}

// ── 文件分享：元信息必须带 uuid / 文件名 / 大小 ─────────────────────────
// 文件名不在 token 里，分享页只能从这里拿到它才能拼出 /file/<uuid>/<name>。
{
  const { env, db } = makeEnv();
  seed(db);

  const fileShare = await postShare(env, { type: 'file', uuid: 'uuid-1', ttl: 60, maxUses: 0 });
  check('文件分享：链接指向 /s/<token>', fileShare.json.url.endsWith(`/s/${encodeURIComponent(fileShare.json.token)}`), true);
  check('文件分享：url 与 pageUrl 同值', fileShare.json.url === fileShare.json.pageUrl, true);
  check('文件分享：rawUrl 指向文件接口', fileShare.json.rawUrl.includes('/file/uuid-1/photo.png'), true);
  check('文件分享：rawUrl 带 token', fileShare.json.rawUrl.includes(`t=${encodeURIComponent(fileShare.json.token)}`), true);

  const info = await getShare(env, fileShare.json.token);
  check('文件元信息：kind', info.json.kind, 'file');
  check('文件元信息：uuid', info.json.uuid, 'uuid-1');
  check('文件元信息：文件名', info.json.name, 'photo.png');
  check('文件元信息：大小', info.json.size, 4096);
}

// ── 密码闸门 ────────────────────────────────────────────────────────────
{
  const { env, db } = makeEnv();
  seed(db);

  const locked = await postShare(env, { type: 'content', id: '1', ttl: 60, maxUses: 0, password: 'hunter2' });
  check('带密码的分享照样签发 token', typeof locked.json.token === 'string', true);
  check('明文密码不进链接', locked.json.url.includes('hunter2'), false);

  const noPwd = await getShare(env, locked.json.token);
  check('没带密码 → 401', noPwd.status, 401);
  check('没带密码 → 可区分的错误码', noPwd.json.code, 'share_password_required');

  const wrong = await getShare(env, locked.json.token, 'nope');
  check('密码错 → 401', wrong.status, 401);

  const right = await getShare(env, locked.json.token, 'hunter2');
  check('密码对 → 200', right.status, 200);
  check('密码对 → 标出需要密码', right.json.needsPassword, true);
}

// ── 带密码的分享：换发「预览令牌」──────────────────────────────────────
// 浏览器自己发的请求（<img> / <video> / <a download>）**加不了 X-Share-Password 头**，
// 所以有密码的分享必须换发一个短期、无密码的能力令牌给这些地址用 ——
// 否则配了密码的实例上图片/视频/下载一律 401，而文本却是好的（那条是 JS 发的）。
// 与 Go 侧 TestShareInfoIssuesPreviewTokenForPasswordShare 对应。
{
  const { env, db } = makeEnv();
  seed(db);

  const locked = await postShare(env, { type: 'file', uuid: 'uuid-1', ttl: 600, maxUses: 5, password: 'hunter2' });
  const info = await getShare(env, locked.json.token, 'hunter2');
  check('预览令牌：带密码的分享会换发', typeof info.json.previewToken === 'string' && info.json.previewToken.length > 0, true);
  check('预览令牌：有效期不晚于原分享', info.json.previewExpiresAt <= info.json.expiresAt, true);

  // ① 不带密码头也能取文件 —— 这正是 <img src> 的处境
  const previewReq = () => new Request(
    `http://worker.local/file/uuid-1/photo.png?t=${encodeURIComponent(info.json.previewToken)}`,
  );
  check('预览令牌：不带密码头也放行文件', await validateShareToken(env, previewReq(), 'file', 'uuid-1', 'default'), true);

  const claims = await parseShareToken(env, info.json.previewToken);
  // ② 不带配额：预览一张图不该烧掉 maxUses
  check('预览令牌：不带使用配额', [claims.maxUses, claims.jti], [0, '']);
  // ③ 不能再要求密码，否则等于没换
  check('预览令牌：不再要求密码', claims.pwdHash, '');
}

// 不带密码的分享**刻意不发**：原 token 本来就能进 URL，
// 而多发一个「不限次」的令牌会让 maxUses 形同虚设。
{
  const { env, db } = makeEnv();
  seed(db);
  const open = await postShare(env, { type: 'file', uuid: 'uuid-1', ttl: 60, maxUses: 2 });
  const info = await getShare(env, open.json.token);
  check('预览令牌：不带密码的分享不发（否则绕过 maxUses）', 'previewToken' in info.json, false);
}

// ── 坏 token ────────────────────────────────────────────────────────────
{
  const { env } = makeEnv();
  const bad = await getShare(env, 'not-a-token');
  check('坏 token → 401', bad.status, 401);
  check('坏 token → 错误码', bad.json.code, 'share_token_invalid');
}

summary('分享页链路在 Worker 上成立');
