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

let shareUsageTableReady = false;

async function ensureShareUsageTable(env) {
  if (!env?.DB || shareUsageTableReady) {
    return Boolean(env?.DB);
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
  shareUsageTableReady = true;
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

async function findContentById(env, contentId, preferredRoom, hasPreferredRoom) {
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

async function findFileMeta(env, uuid) {
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

// 分享链接指向前端分享页（hash 路由），不再是裸接口地址。
// `#` 必须保留字面量 —— 交给 URL 对象拼会被转义成 %23，hash 路由当场失效。
function buildSharePageURL(request, token) {
  const origin = new URL(request.url).origin;
  return `${origin}/#/s?${SHARE_TOKEN_QUERY_KEY}=${encodeURIComponent(token)}`;
}

// 已用次数，只读。表可能还没建起来（没消费过就没有行），出错按 0 处理。
async function readShareUsed(env, claims) {
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
  };
  if (maxUses > 0) {
    claims.jti = newShareJTI();
    claims.mu = maxUses;
  }
  const pwdHash = await sharePasswordHash(env, password);
  if (pwdHash) {
    claims.p = pwdHash;
  }
  const token = await signShareClaims(env, claims);
  return { token, expiresAt };
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
          url: buildSharePageURL(request, issued.token),
          rawUrl: rawUrl.toString(),
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
          url: buildSharePageURL(request, issued.token),
          rawUrl: rawUrl.toString(),
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
}
