import { normalizeRoomName } from './auth';

// 分享记录（share log）：签发分享时落一条，分享页被真人打开时加一个计数。
// 与 Go 侧 share_log.go 一一对应，行为必须一致（那边文件头有完整的背景说明）。
//
// 为什么需要它：分享 token 是**无状态 HMAC 签名**，服务端不留痕迹 —— 于是
// 「我最近分享过什么」「这条链接被打开了几次」都答不上来。这里只补最小的事实。
//
// ⚠️ 谁能看到这份记录：GET /share/list 用**房间凭据**鉴权（与 POST /share 同一套规则）。
// 房间没配密码时 canAccessRoomAsync 恒为真 —— 也就是**开放房间的记录列表对所有能访问
// 服务器的人可读**。有意为之：列表里的信息在开放房间里本来就公开，而且列表**不回
// token**，拿到它也取不到正文、无法重新取用别人的分享。放敏感内容请给房间设密码。
const MAX_SHARE_LOG_ROWS = 500;
export const DEFAULT_SHARE_LIST_LIMIT = 50;
export const MAX_SHARE_LIST_LIMIT = 200;
// 同一访客对同一条分享的重复上报窗口（秒）。
const SHARE_VISIT_DEDUPE_WINDOW = 600;

// 建表只做一次，但要**按 DB 记**（WeakSet 而不是一个布尔）。
// 一个布尔在进程内是全局的：测试里每个用例都新建一个内存 D1，第二次进来时
// 标志位已经是 true，于是新库上根本没建表，"no such table" 在很后面才炸出来。
const ensuredShareLogDbs = new WeakSet();

export async function ensureShareLogTables(env) {
  if (!env?.DB) {
    return false;
  }
  if (ensuredShareLogDbs.has(env.DB)) {
    return true;
  }
  await env.DB.prepare(`
    CREATE TABLE IF NOT EXISTS share_log (
      jti TEXT PRIMARY KEY,
      type TEXT NOT NULL,
      id TEXT NOT NULL,
      room TEXT NOT NULL DEFAULT 'default',
      kind TEXT,
      name TEXT,
      size INTEGER,
      createdAt INTEGER NOT NULL,
      exp INTEGER NOT NULL,
      maxUses INTEGER NOT NULL DEFAULT 0,
      password INTEGER NOT NULL DEFAULT 0,
      visits INTEGER NOT NULL DEFAULT 0,
      scans INTEGER NOT NULL DEFAULT 0
    )
  `).run();
  await env.DB.prepare('CREATE INDEX IF NOT EXISTS idx_share_log_room ON share_log(room, createdAt)').run();
  await env.DB.prepare(`
    CREATE TABLE IF NOT EXISTS share_visit_marks (
      jti TEXT NOT NULL,
      visitor TEXT NOT NULL,
      ts INTEGER NOT NULL,
      PRIMARY KEY (jti, visitor)
    )
  `).run();
  ensuredShareLogDbs.add(env.DB);
  return true;
}

// 没有 D1 时的进程内兜底（本地 wrangler dev 无绑定、或测试环境）。
const memoryShareLog = new Map();
const memoryVisitMarks = new Map();

function trimMemoryLog() {
  while (memoryShareLog.size > MAX_SHARE_LOG_ROWS) {
    const oldest = memoryShareLog.keys().next().value;
    if (oldest === undefined) {
      break;
    }
    memoryShareLog.delete(oldest);
  }
}

function normalizeListLimit(limit) {
  const value = Math.floor(Number(limit) || 0);
  if (value <= 0) {
    return DEFAULT_SHARE_LIST_LIMIT;
  }
  return Math.min(value, MAX_SHARE_LIST_LIMIT);
}

export function shareLogLimitFromQuery(raw) {
  return normalizeListLimit(raw);
}

/**
 * 记一条新分享。必须在 token 签发成功后调用。
 * meta: { kind, name, size } —— 记录列表里要能一眼认出「分享的是哪条」。
 */
export async function recordShareInLog(env, claims, meta = {}) {
  if (!claims?.jti) {
    return;
  }

  const now = Math.floor(Date.now() / 1000);
  const row = {
    jti: String(claims.jti),
    type: String(claims.type || ''),
    id: String(claims.id || ''),
    room: normalizeRoomName(claims.room),
    kind: String(meta.kind || ''),
    name: String(meta.name || ''),
    size: Number(meta.size || 0),
    createdAt: now,
    exp: Number(claims.exp || 0),
    maxUses: Number(claims.maxUses || 0),
    password: claims.pwdHash ? 1 : 0,
  };

  if (await ensureShareLogTables(env)) {
    await env.DB.prepare(
      `INSERT OR REPLACE INTO share_log
         (jti, type, id, room, kind, name, size, createdAt, exp, maxUses, password)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    ).bind(
      row.jti, row.type, row.id, row.room, row.kind, row.name, row.size,
      row.createdAt, row.exp, row.maxUses, row.password,
    ).run();

    // 顺手裁一刀：超过上限就删最旧的，已过期的优先。
    // 这份日志只回答「最近分享过什么」，不是审计系统，不该无限长大。
    try {
      await env.DB.prepare(
        `DELETE FROM share_log WHERE jti IN (
           SELECT jti FROM share_log
           ORDER BY (exp > 0 AND exp <= ?) ASC, createdAt DESC
           LIMIT -1 OFFSET ?
         )`,
      ).bind(now, MAX_SHARE_LOG_ROWS).run();
    } catch {
      // 裁剪失败不影响签发
    }
    return;
  }

  memoryShareLog.set(row.jti, { ...row, visits: 0, scans: 0 });
  trimMemoryLog();
}

/**
 * 记一次「有人打开了这条分享」。viaQR：二维码链接带 ?q=1。
 * visitorKey 用于去重：同一条分享在同一窗口内的重复上报只算一次，
 * 否则拿着链接连点就能把数字刷上去。
 */
export async function markShareVisit(env, claims, { viaQR = false, visitorKey = 'unknown' } = {}) {
  const empty = { visits: 0, scans: 0, tracked: false };
  if (!claims?.jti) {
    return empty;
  }

  const now = Math.floor(Date.now() / 1000);
  const visitor = String(visitorKey || 'unknown');

  if (await ensureShareLogTables(env)) {
    const row = await env.DB.prepare('SELECT visits, scans FROM share_log WHERE jti = ?')
      .bind(claims.jti)
      .first();
    if (!row) {
      return empty;
    }

    const mark = await env.DB.prepare('SELECT ts FROM share_visit_marks WHERE jti = ? AND visitor = ?')
      .bind(claims.jti, visitor)
      .first();
    if (mark && now - Number(mark.ts || 0) < SHARE_VISIT_DEDUPE_WINDOW) {
      return { visits: Number(row.visits || 0), scans: Number(row.scans || 0), tracked: false };
    }

    await env.DB.prepare(
      'INSERT OR REPLACE INTO share_visit_marks (jti, visitor, ts) VALUES (?, ?, ?)',
    ).bind(claims.jti, visitor, now).run();
    try {
      await env.DB.prepare('DELETE FROM share_visit_marks WHERE ts < ?')
        .bind(now - SHARE_VISIT_DEDUPE_WINDOW)
        .run();
    } catch {
      // 清理失败不影响计数
    }

    await env.DB.prepare('UPDATE share_log SET visits = visits + 1, scans = scans + ? WHERE jti = ?')
      .bind(viaQR ? 1 : 0, claims.jti)
      .run();

    return {
      visits: Number(row.visits || 0) + 1,
      scans: Number(row.scans || 0) + (viaQR ? 1 : 0),
      tracked: true,
    };
  }

  const rec = memoryShareLog.get(claims.jti);
  if (!rec) {
    return empty;
  }
  const markKey = `${claims.jti}|${visitor}`;
  const last = memoryVisitMarks.get(markKey);
  if (last && now - last < SHARE_VISIT_DEDUPE_WINDOW) {
    return { visits: rec.visits, scans: rec.scans, tracked: false };
  }
  memoryVisitMarks.set(markKey, now);
  rec.visits += 1;
  if (viaQR) {
    rec.scans += 1;
  }
  return { visits: rec.visits, scans: rec.scans, tracked: true };
}

/**
 * 某个房间的记录（新→旧）与该房间的总条数。
 * `used`（已用次数）不在这里补 —— 它属于 share_token_usage，由 share.js 补。
 */
export async function listShareRecords(env, room, limit) {
  const normalizedRoom = normalizeRoomName(room);
  const normalizedLimit = normalizeListLimit(limit);
  const now = Math.floor(Date.now() / 1000);

  const toRecord = (row) => ({
    jti: String(row.jti || ''),
    type: String(row.type || ''),
    kind: String(row.kind || ''),
    id: String(row.id || ''),
    room: normalizeRoomName(row.room),
    name: String(row.name || ''),
    size: Number(row.size || 0),
    createdAt: Number(row.createdAt || 0),
    expiresAt: Number(row.exp || 0),
    maxUses: Number(row.maxUses || 0),
    visits: Number(row.visits || 0),
    scans: Number(row.scans || 0),
    password: Number(row.password || 0) === 1,
    expired: Number(row.exp || 0) > 0 && Number(row.exp || 0) <= now,
  });

  if (await ensureShareLogTables(env)) {
    const totalRow = await env.DB.prepare('SELECT COUNT(*) AS n FROM share_log WHERE room = ?')
      .bind(normalizedRoom)
      .first();
    const rows = await env.DB.prepare(
      'SELECT * FROM share_log WHERE room = ? ORDER BY createdAt DESC LIMIT ?',
    ).bind(normalizedRoom, normalizedLimit).all();

    return {
      total: Number(totalRow?.n || 0),
      records: (rows?.results || []).map(toRecord),
    };
  }

  const all = [...memoryShareLog.values()]
    .filter((rec) => normalizeRoomName(rec.room) === normalizedRoom)
    .sort((a, b) => b.createdAt - a.createdAt);

  return {
    total: all.length,
    records: all.slice(0, normalizedLimit).map(toRecord),
  };
}
