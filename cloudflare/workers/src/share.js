import { corsHeaders } from './cors';
import {
  canAccessRoomAsync,
  extractAuthToken,
  normalizeAuthValue,
  normalizeRoomName,
  parseRoomAuth,
  resolveRoomAuth,
} from './auth';
import { errorResponse } from './errors';
import {
  DEFAULT_SHARE_LIST_LIMIT,
  listShareRecords,
  markShareVisit,
  recordShareInLog,
  shareLogLimitFromQuery,
} from './share-log';
import { SHARE_NAME_LIMIT, firstSummaryLine } from './share-summary';

export const SHARE_TOKEN_QUERY_KEY = 't';

// 分享密码走的请求头。**不要放 URL** —— query 会进浏览器历史和访问日志。
const SHARE_PASSWORD_HEADER = 'X-Share-Password';
// 存进 token 的是 HMAC(签名密钥, "share-password:"+密码) 的十六进制前 16 位。
// 用密钥而不是裸 SHA256：token 在 URL 里，裸哈希能被离线爆破。
const SHARE_PASSWORD_HASH_LEN = 16;
export const DEFAULT_SHARE_TTL_SECONDS = 15 * 60;
export const MIN_SHARE_TTL_SECONDS = 60;
export const MAX_SHARE_TTL_SECONDS = 24 * 60 * 60;
export const MAX_SHARE_MAX_USES = 1000;

function normalizeShareTTL(ttl) {
  const value = Number(ttl);
  if (!Number.isFinite(value) || value <= 0) {
    return DEFAULT_SHARE_TTL_SECONDS;
  }
  if (value < MIN_SHARE_TTL_SECONDS) {
    return MIN_SHARE_TTL_SECONDS;
  }
  if (value > MAX_SHARE_TTL_SECONDS) {
    return MAX_SHARE_TTL_SECONDS;
  }
  return Math.floor(value);
}

function normalizeShareMaxUses(maxUses) {
  const value = Number(maxUses);
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  if (value > MAX_SHARE_MAX_USES) {
    return MAX_SHARE_MAX_USES;
  }
  return Math.floor(value);
}

function bytesToBase64Url(bytes) {
  let binary = '';
  const view = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  for (let i = 0; i < view.length; i += 1) {
    binary += String.fromCharCode(view[i]);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function base64UrlToBytes(value) {
  const normalized = String(value || '').replace(/-/g, '+').replace(/_/g, '/');
  const padding = normalized.length % 4 === 0 ? '' : '='.repeat(4 - (normalized.length % 4));
  const binary = atob(normalized + padding);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function textToBytes(text) {
  return new TextEncoder().encode(String(text || ''));
}

async function sha256(bytes) {
  return new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
}

function newShareJTI() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
}

async function getShareSigningKey(env) {
  const material = ['cloud-clipboard-share-v1', normalizeAuthValue(env.AUTH_PASSWORD)];
  const roomAuth = parseRoomAuth(env);
  Object.keys(roomAuth).sort().forEach(room => {
    material.push(room, normalizeAuthValue(roomAuth[room].password));
  });
  if (env.SHARE_SIGNING_SECRET) {
    material.push(String(env.SHARE_SIGNING_SECRET));
  }

  const digest = await sha256(textToBytes(material.join('\0')));
  return crypto.subtle.importKey(
    'raw',
    digest,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign', 'verify'],
  );
}

export function extractShareToken(request) {
  return new URL(request.url).searchParams.get(SHARE_TOKEN_QUERY_KEY) || '';
}

// 把分享密码算成可存进 token 的短串；空密码返回空串（不需要密码）。
async function sharePasswordHash(env, password) {
  const normalized = String(password || '').trim();
  if (!normalized) {
    return '';
  }
  const key = await getShareSigningKey(env);
  const signature = await crypto.subtle.sign('HMAC', key, textToBytes(`share-password:${normalized}`));
  const hex = [...new Uint8Array(signature)].map((b) => b.toString(16).padStart(2, '0')).join('');
  return hex.slice(0, SHARE_PASSWORD_HASH_LEN);
}

// 常数时间比较：逐字节异或累加，不要短路返回。
// 用 `===` 会在第一个不同的字节就返回，能按时间差逐字节猜出哈希。
function constantTimeEqual(a, b) {
  const left = String(a || '');
  const right = String(b || '');
  if (left.length !== right.length) {
    return false;
  }
  let diff = 0;
  for (let i = 0; i < left.length; i += 1) {
    diff |= left.charCodeAt(i) ^ right.charCodeAt(i);
  }
  return diff === 0;
}

export async function signShareClaims(env, claims) {
  const key = await getShareSigningKey(env);
  const payload = bytesToBase64Url(textToBytes(JSON.stringify(claims)));
  const signature = await crypto.subtle.sign('HMAC', key, textToBytes(payload));
  return `${payload}.${bytesToBase64Url(new Uint8Array(signature))}`;
}

export async function parseShareToken(env, token) {
  const normalized = String(token || '').trim();
  if (!normalized) {
    return null;
  }

  const parts = normalized.split('.');
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    return null;
  }

  try {
    const key = await getShareSigningKey(env);
    const valid = await crypto.subtle.verify(
      'HMAC',
      key,
      base64UrlToBytes(parts[1]),
      textToBytes(parts[0]),
    );
    if (!valid) {
      return null;
    }

    const claims = JSON.parse(new TextDecoder().decode(base64UrlToBytes(parts[0])));
    const type = String(claims?.typ || '').trim();
    const id = String(claims?.id || '').trim();
    const room = normalizeRoomName(claims?.room);
    const exp = Number(claims?.exp || 0);
    const jti = String(claims?.jti || '').trim();
    const maxUses = Number(claims?.mu || 0);
    if (!type || !id || !exp) {
      return null;
    }
    if (Math.floor(Date.now() / 1000) > exp) {
      return null;
    }
    if (maxUses > 0 && !jti) {
      return null;
    }
    return {
      type,
      id,
      room,
      exp,
      jti,
      maxUses: Number.isFinite(maxUses) && maxUses > 0 ? Math.floor(maxUses) : 0,
      // 非空表示这条分享需要密码；值是 HMAC(签名密钥, 密码) 的前若干位
      pwdHash: String(claims?.p || '').trim(),
    };
  } catch {
    return null;
  }
}

/**
 * Range 续传 / HEAD 不计入次数，避免视频拖动进度条把次数耗尽。
 */
export function shouldConsumeShareUse(request) {
  if (!request) {
    return false;
  }
  const method = String(request.method || 'GET').toUpperCase();
  if (method === 'HEAD') {
    return false;
  }
  if (method !== 'GET') {
    return false;
  }
  const rangeHeader = String(request.headers?.get?.('Range') || '').trim();
  if (!rangeHeader) {
    return true;
  }
  const lower = rangeHeader.toLowerCase();
  if (!lower.startsWith('bytes=')) {
    return true;
  }
  const spec = rangeHeader.slice('bytes='.length).trim();
  if (!spec || spec.includes(',')) {
    return true;
  }
  const startPart = spec.split('-')[0].trim();
  return startPart === '' || startPart === '0';
}

// 按 DB 记（WeakSet 而不是布尔）：布尔在进程内是全局的，测试里每个用例都新建一个
// 内存 D1，第二次进来时标志位已经是 true，新库上就根本没建表。
const ensuredShareUsageDbs = new WeakSet();

async function ensureShareUsageTable(env) {
  if (!env?.DB) {
    return false;
  }
  if (ensuredShareUsageDbs.has(env.DB)) {
    return true;
  }
  await env.DB.prepare(`
    CREATE TABLE IF NOT EXISTS share_token_usage (
      jti TEXT PRIMARY KEY,
      used INTEGER NOT NULL DEFAULT 0,
      maxUses INTEGER NOT NULL,
      exp INTEGER NOT NULL
    )
  `).run();
  await env.DB.prepare(`
    CREATE INDEX IF NOT EXISTS idx_share_token_usage_exp ON share_token_usage(exp)
  `).run();
  ensuredShareUsageDbs.add(env.DB);
  return true;
}

/**
 * 进程/边缘内存兜底（无 D1 时）；有 D1 时以 D1 为准。
 */
const memoryShareUsage = new Map();

async function consumeShareUse(env, claims) {
  if (!claims || !claims.maxUses || claims.maxUses <= 0) {
    return true;
  }
  if (!claims.jti) {
    return false;
  }

  const now = Math.floor(Date.now() / 1000);
  if (claims.exp > 0 && claims.exp <= now) {
    return false;
  }

  if (await ensureShareUsageTable(env)) {
    // 清理少量过期记录
    try {
      await env.DB.prepare('DELETE FROM share_token_usage WHERE exp > 0 AND exp <= ?')
        .bind(now)
        .run();
    } catch {
      // ignore cleanup errors
    }

    const row = await env.DB.prepare('SELECT used, maxUses, exp FROM share_token_usage WHERE jti = ?')
      .bind(claims.jti)
      .first();

    if (!row) {
      await env.DB.prepare(
        'INSERT INTO share_token_usage (jti, used, maxUses, exp) VALUES (?, 1, ?, ?)',
      ).bind(claims.jti, claims.maxUses, claims.exp).run();
      return true;
    }

    const used = Number(row.used || 0);
    const maxUses = Math.max(Number(row.maxUses || 0), claims.maxUses);
    const exp = Math.max(Number(row.exp || 0), claims.exp);
    if (exp > 0 && exp <= now) {
      await env.DB.prepare('DELETE FROM share_token_usage WHERE jti = ?').bind(claims.jti).run();
      return false;
    }
    if (used >= maxUses) {
      return false;
    }
    await env.DB.prepare(
      'UPDATE share_token_usage SET used = ?, maxUses = ?, exp = ? WHERE jti = ?',
    ).bind(used + 1, maxUses, exp, claims.jti).run();
    return true;
  }

  // memory fallback
  let entry = memoryShareUsage.get(claims.jti);
  if (!entry) {
    entry = { used: 0, maxUses: claims.maxUses, exp: claims.exp };
    memoryShareUsage.set(claims.jti, entry);
  } else {
    entry.exp = Math.max(entry.exp || 0, claims.exp);
    entry.maxUses = Math.max(entry.maxUses || 0, claims.maxUses);
  }
  if (entry.exp > 0 && entry.exp <= now) {
    memoryShareUsage.delete(claims.jti);
    return false;
  }
  if (entry.used >= entry.maxUses) {
    return false;
  }
  entry.used += 1;
  return true;
}

export async function validateShareToken(env, request, expectedType, expectedId, expectedRoom) {
  const claims = await parseShareToken(env, extractShareToken(request));
  if (!claims) {
    return false;
  }
  if (claims.type !== expectedType
    || claims.id !== String(expectedId || '')
    || claims.room !== normalizeRoomName(expectedRoom)) {
    return false;
  }

  // 需要密码的分享：请求头里必须带对（常数时间比较）
  if (claims.pwdHash) {
    const supplied = await sharePasswordHash(env, request.headers.get(SHARE_PASSWORD_HEADER));
    if (!constantTimeEqual(supplied, claims.pwdHash)) {
      return false;
    }
  }

  if (claims.maxUses > 0 && shouldConsumeShareUse(request)) {
    const ok = await consumeShareUse(env, claims);
    if (!ok) {
      return false;
    }
  }
  return true;
}

export async function ensureRoomOrShareAccess(request, env, room, {
  shareType = '',
  shareId = '',
} = {}) {
  const normalizedRoom = normalizeRoomName(room);
  const requirement = resolveRoomAuth(env, normalizedRoom);
  const token = extractAuthToken(request);

  if (!requirement.required) {
    return { ok: true, room: normalizedRoom, token, requirement };
  }

  if (token && await canAccessRoomAsync(env, normalizedRoom, token)) {
    return { ok: true, room: normalizedRoom, token, requirement };
  }

  if (shareType && shareId) {
    const shareOk = await validateShareToken(env, request, shareType, shareId, normalizedRoom);
    if (shareOk) {
      return { ok: true, room: normalizedRoom, token, requirement, viaShare: true };
    }
  }

  if (!token && !extractShareToken(request)) {
    return {
      ok: false,
      room: normalizedRoom,
      token,
      requirement,
      response: errorResponse(401, 'unauthorized', 'Unauthorized', '需要认证令牌'),
    };
  }

  return {
    ok: false,
    room: normalizedRoom,
    token,
    requirement,
    response: errorResponse(401, 'unauthorized_invalid_token', 'Unauthorized', '无效的认证令牌'),
  };
}

export async function findContentById(env, contentId, preferredRoom, hasPreferredRoom) {
  if (!env.DB) {
    return null;
  }

  if (hasPreferredRoom) {
    return env.DB.prepare('SELECT * FROM messages WHERE id = ? AND room = ? LIMIT 1')
      .bind(contentId, preferredRoom)
      .first();
  }

  return env.DB.prepare('SELECT * FROM messages WHERE id = ? ORDER BY id DESC LIMIT 1')
    .bind(contentId)
    .first();
}

export async function findFileMeta(env, uuid) {
  if (env.R2_BUCKET) {
    const object = await env.R2_BUCKET.head(`files/${uuid}`);
    if (object) {
      return {
        uuid,
        name: object.customMetadata?.originalName || 'file',
        size: Number(object.size || 0),
        room: normalizeRoomName(object.customMetadata?.room || 'default'),
        expireTime: Number(object.customMetadata?.expireTime || 0),
      };
    }
  }

  if (env.DB) {
    const row = await env.DB.prepare('SELECT uuid, name, size, room, expireTime FROM messages WHERE uuid = ? ORDER BY id DESC LIMIT 1')
      .bind(uuid)
      .first();
    if (row) {
      return {
        uuid: row.uuid,
        name: row.name || 'file',
        size: Number(row.size || 0),
        room: normalizeRoomName(row.room || 'default'),
        expireTime: Number(row.expireTime || 0),
      };
    }
  }

  return null;
}

// 分享链接**就是落地页地址本身**（见 share-landing.js）：服务端把 OG 卡片注入 SPA 外壳后
// 返回同一份 HTML，真人由前端 history 路由 `/s/:token` 接管。没有第二跳、没有第二个地址。
//
// 曾经它是前端 hash 路由地址（`/#/s?t=…`）：那时前端用 hash 路由，抓取程序读不到 `#` 之后
// 的部分，于是另有一个 `/s/<token>` 落地页专给预览用 —— 同一个分享因此有两个地址（贴出去的
// 和真人看的），平台的点击统计、书签、二维码各认各的。现在收敛成一个。
//
// 仍然保留这个函数名：名字说的是「给收件人的地址」，与 `pageUrl`/`url` 两个响应字段对应。
export function buildSharePageURL(request, token) {
  return buildShareLandingURL(request, token);
}

// 分享地址：token 在**路径**里。不能放 `#` 之后 —— 浏览器不把 fragment 发给服务器，
// 而 OG 标签必须由服务端注入。base64url 里没有 `/`，encodeURIComponent 只是兜住边界字符。
function buildShareLandingURL(request, token) {
  const origin = new URL(request.url).origin;
  return `${origin}/s/${encodeURIComponent(token)}`;
}

// 已用次数，只读。表可能还没建起来（没消费过就没有行），出错按 0 处理。
export async function readShareUsed(env, claims) {
  if (!claims?.jti || !claims?.maxUses) {
    return 0;
  }
  if (env.DB) {
    try {
      const row = await env.DB.prepare('SELECT used FROM share_token_usage WHERE jti = ?')
        .bind(claims.jti)
        .first();
      return Number(row?.used || 0);
    } catch {
      return 0;
    }
  }
  return Number(memoryShareUsage.get(claims.jti)?.used || 0);
}

async function issueShareToken(env, { type, id, room, ttl, maxUses, password }) {
  const expiresAt = Math.floor(Date.now() / 1000) + ttl;
  const claims = {
    typ: type,
    id,
    room,
    exp: expiresAt,
    // 无条件分配 jti。以前只在限次（maxUses > 0）时才发 —— 于是不限次的分享
    // 既入不了档（分享记录）也计不了数（打开次数）。现在 jti 还兼作档案号。
    jti: newShareJTI(),
  };
  if (maxUses > 0) {
    claims.mu = maxUses;
  }
  const pwdHash = await sharePasswordHash(env, password);
  if (pwdHash) {
    claims.p = pwdHash;
  }
  const token = await signShareClaims(env, claims);
  return {
    token,
    expiresAt,
    jti: claims.jti,
    claims: {
      type,
      id,
      room,
      exp: expiresAt,
      jti: claims.jti,
      maxUses,
      pwdHash,
    },
  };
}

export class ShareHandler {
  static async create(request, env) {
    try {
      const url = new URL(request.url);
      const hasRequestedRoom = url.searchParams.has('room');
      const requestedRoom = normalizeRoomName(url.searchParams.get('room'));
      const body = await request.json().catch(() => ({}));
      const shareType = String(body?.type || '').trim().toLowerCase();
      const ttl = normalizeShareTTL(body?.ttl);
      const maxUses = normalizeShareMaxUses(body?.maxUses);
      const authToken = extractAuthToken(request);

      if (!shareType) {
        return errorResponse(400, 'missing_type', 'Bad Request', '缺少 type');
      }

      if (shareType === 'content') {
        const id = String(body?.id || '').trim();
        if (!id) {
          return errorResponse(400, 'missing_id', 'Bad Request', '缺少 id');
        }

        const row = await findContentById(env, Number(id), requestedRoom, hasRequestedRoom);
        if (!row) {
          return errorResponse(404, 'content_not_found', 'Not Found', '内容未找到');
        }

        const room = normalizeRoomName(row.room || 'default');
        if (!await canAccessRoomAsync(env, room, authToken)) {
          return errorResponse(401, 'room_forbidden', 'Unauthorized', '无权访问该房间');
        }

        // 一律签发 token：TTL / 次数限制 / 密码都由它承载，房间是否需要鉴权不再影响这件事。
        // 曾经只在 requirement.required 时才发 —— 结果是开放房间的分享链接永不过期、不限次数，
        // 弹窗里让用户设的值被静默丢弃，而响应里却照样回 ttl/maxUses，会骗到调用方。
        const issued = await issueShareToken(env, {
          type: 'content',
          id,
          room,
          ttl,
          maxUses,
          password: body?.password,
        });

        // 入档：记录列表要能回答「我最近分享过什么」。名称取文件名 / 文本首行摘要。
        const kind = String(row.type || 'text');
        await recordShareInLog(env, issued.claims, {
          kind,
          name: kind === 'file' ? String(row.name || '') : firstSummaryLine(row.content, SHARE_NAME_LIMIT),
          size: kind === 'file' ? Number(row.size || 0) : 0,
        });

        // rawUrl 是「直接拿正文」的地址（带同一个 token），给分享页的下载按钮
        // 和前端自己的下载链路用 —— 分享页地址是 hash 路由，取不了正文。
        const rawUrl = new URL(`${url.origin}/content/${id}`);
        if (room !== 'default') {
          rawUrl.searchParams.set('room', room);
        }
        rawUrl.searchParams.set(SHARE_TOKEN_QUERY_KEY, issued.token);

        return new Response(JSON.stringify({
          type: 'content',
          id,
          room,
          ttl,
          expiresAt: issued.expiresAt,
          maxUses,
          token: issued.token,
          jti: issued.jti,
          url: buildSharePageURL(request, issued.token),
          pageUrl: buildShareLandingURL(request, issued.token),
          rawUrl: rawUrl.toString(),
          visits: 0,
          scans: 0,
        }), {
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      if (shareType === 'file') {
        const uuid = String(body?.uuid || body?.id || '').trim();
        if (!uuid) {
          return errorResponse(400, 'missing_uuid', 'Bad Request', '缺少 uuid');
        }

        const fileMeta = await findFileMeta(env, uuid);
        if (!fileMeta) {
          return errorResponse(404, 'file_not_found', 'Not Found', '文件未找到或已过期');
        }

        const now = Math.floor(Date.now() / 1000);
        if (fileMeta.expireTime > 0 && fileMeta.expireTime < now) {
          return errorResponse(404, 'file_expired', 'Not Found', '文件已过期');
        }

        const room = normalizeRoomName(fileMeta.room);
        if (hasRequestedRoom && room !== requestedRoom) {
          return errorResponse(404, 'file_not_found', 'Not Found', '文件未找到或已过期');
        }
        if (!await canAccessRoomAsync(env, room, authToken)) {
          return errorResponse(401, 'room_forbidden', 'Unauthorized', '无权访问该房间');
        }

        // 同 content 分支：一律签发 token，文件名不再进 URL ——
        // 分享页会先问一次 GET /share 拿到它，再拼 /file/<uuid>/<name>。
        const issued = await issueShareToken(env, {
          type: 'file',
          id: uuid,
          room,
          ttl,
          maxUses,
          password: body?.password,
        });

        await recordShareInLog(env, issued.claims, {
          kind: 'file',
          name: String(fileMeta.name || ''),
          size: Number(fileMeta.size || 0),
        });

        const rawUrl = new URL(`${url.origin}/file/${uuid}/${encodeURIComponent(fileMeta.name || 'file')}`);
        rawUrl.searchParams.set(SHARE_TOKEN_QUERY_KEY, issued.token);

        return new Response(JSON.stringify({
          type: 'file',
          uuid,
          room,
          ttl,
          expiresAt: issued.expiresAt,
          maxUses,
          token: issued.token,
          jti: issued.jti,
          url: buildSharePageURL(request, issued.token),
          pageUrl: buildShareLandingURL(request, issued.token),
          rawUrl: rawUrl.toString(),
          visits: 0,
          scans: 0,
        }), {
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      return errorResponse(400, 'unsupported_type', 'Bad Request', '不支持的 type');
    } catch (error) {
      console.error('Share create error:', error);
      return errorResponse(500, 'share_token_failed', 'Internal Server Error', '生成分享链接失败');
    }
  }

  /**
   * GET /share?t=... —— 分享页在取正文之前先问一次这里。
   *
   * 为什么不直接让分享页去调 /content 或 /file：
   *   - 文件场景必须先知道**文件名**才能拼出 /file/<uuid>/<name>，而 token 里没有这个名字；
   *   - 分享页要在取正文之前就把「类型 / 大小 / 剩余有效期 / 剩余次数」渲染出来；
   *   - 「token 无效」「已过期」「需要密码」三种情况要能分开报，取正文的接口分不出来。
   *
   * **不消耗使用次数**：打开页面本身不该烧掉一次，真正取正文时才消耗（见 validateShareToken）。
   */
  static async info(request, env) {
    try {
      const claims = await parseShareToken(env, extractShareToken(request));
      if (!claims) {
        return errorResponse(401, 'share_token_invalid', 'Unauthorized', '分享链接无效或已过期');
      }

      if (claims.pwdHash) {
        const supplied = await sharePasswordHash(env, request.headers.get(SHARE_PASSWORD_HEADER));
        if (!constantTimeEqual(supplied, claims.pwdHash)) {
          return errorResponse(401, 'share_password_required', 'Unauthorized', '需要分享密码');
        }
      }

      const now = Math.floor(Date.now() / 1000);
      const response = {
        type: claims.type,
        room: claims.room,
        expiresAt: claims.exp,
        maxUses: claims.maxUses || 0,
        used: await readShareUsed(env, claims),
        needsPassword: Boolean(claims.pwdHash),
      };

      if (claims.type === 'content') {
        const row = await findContentById(env, Number(claims.id), claims.room, true);
        if (!row) {
          return errorResponse(404, 'content_not_found', 'Not Found', '内容未找到');
        }
        response.id = claims.id;
        response.room = normalizeRoomName(row.room || claims.room);
        response.kind = String(row.type || 'text');

        if (response.kind === 'file') {
          const meta = await findFileMeta(env, row.uuid);
          if (!meta) {
            return errorResponse(404, 'file_not_found', 'Not Found', '文件未找到或已过期');
          }
          if (meta.expireTime > 0 && meta.expireTime < now) {
            return errorResponse(404, 'file_expired', 'Not Found', '文件已过期');
          }
          response.uuid = meta.uuid;
          response.name = meta.name;
          response.size = meta.size;
        }
      } else if (claims.type === 'file') {
        const meta = await findFileMeta(env, claims.id);
        if (!meta) {
          return errorResponse(404, 'file_not_found', 'Not Found', '文件未找到或已过期');
        }
        if (meta.expireTime > 0 && meta.expireTime < now) {
          return errorResponse(404, 'file_expired', 'Not Found', '文件已过期');
        }
        response.kind = 'file';
        response.uuid = meta.uuid;
        response.name = meta.name;
        response.size = meta.size;
      } else {
        return errorResponse(400, 'unsupported_type', 'Bad Request', '不支持的分享类型');
      }

      return new Response(JSON.stringify(response), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders },
      });
    } catch (error) {
      console.error('Share info error:', error);
      return errorResponse(500, 'share_info_failed', 'Internal Server Error', '读取分享信息失败');
    }
  }

  /**
   * GET /share/list?room=<room>&limit=<n> —— 某个房间最近的分享记录。
   *
   * 鉴权刻意与「在该房间签发分享」完全一致（canAccessRoomAsync），于是规则只有一条，
   * 不会出现「能建分享但看不到自己建的分享」。房间没配密码时鉴权恒为通过 ——
   * 也就是开放房间的记录列表是公开可读的，理由见 share-log.js 文件头。
   *
   * **不回 token**：token 是 bearer 凭据，列表只是给创建者看统计的。
   */
  static async list(request, env) {
    try {
      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      if (!await canAccessRoomAsync(env, room, extractAuthToken(request))) {
        return errorResponse(401, 'room_forbidden', 'Unauthorized', '无权访问该房间');
      }

      const limit = url.searchParams.get('limit')
        ? shareLogLimitFromQuery(url.searchParams.get('limit'))
        : DEFAULT_SHARE_LIST_LIMIT;

      const { records, total } = await listShareRecords(env, room, limit);
      // 已用次数住在 share_token_usage（只有限次分享才有行），和记录表合并不了 SQL，
      // 数量又有上限（默认 50、最多 200），逐条查可以接受。
      const withUsed = [];
      for (const rec of records) {
        withUsed.push({ ...rec, used: await readShareUsed(env, { jti: rec.jti, maxUses: rec.maxUses }) });
      }

      return new Response(JSON.stringify({ room, total, limit, records: withUsed }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders },
      });
    } catch (error) {
      console.error('Share list error:', error);
      return errorResponse(500, 'share_list_failed', 'Internal Server Error', '读取分享记录失败');
    }
  }

  /**
   * POST /share/visit —— 分享页被**真人**打开时上报一次。
   *
   * 为什么由前端上报、而不是在 /s/<token> 落地页里计数：落地页是给社交平台的**抓取
   * 程序**看的（贴一次链接，微信/Telegram/Slack 都会去抓，且平台侧会按自己的节奏重抓）。
   * 在那里计数会把「机器抓取」算成「有人打开」。只有执行了 JS 的分享页能证明是真人。
   *
   * 鉴权：只需要 token 本身（未认证接口）—— 你拿着链接才能上报，接口也只回**这一条**
   * 分享的计数。同一访客十分钟内重复上报会被去重（见 markShareVisit）。
   */
  static async visit(request, env) {
    try {
      const url = new URL(request.url);
      const body = await request.json().catch(() => ({}));
      const token = String(body?.token || url.searchParams.get(SHARE_TOKEN_QUERY_KEY) || '').trim();
      if (!token) {
        return errorResponse(400, 'missing_token', 'Bad Request', '缺少 token');
      }

      const claims = await parseShareToken(env, token);
      if (!claims) {
        return errorResponse(401, 'share_token_invalid', 'Unauthorized', '分享链接无效或已过期');
      }

      const result = await markShareVisit(env, claims, {
        viaQR: Boolean(body?.qr) || url.searchParams.get('q') === '1',
        visitorKey: request.headers.get('CF-Connecting-IP') || 'unknown',
      });

      return new Response(JSON.stringify({ ok: true, ...result }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders },
      });
    } catch (error) {
      console.error('Share visit error:', error);
      return errorResponse(500, 'share_visit_failed', 'Internal Server Error', '分享访问上报失败');
    }
  }
}
