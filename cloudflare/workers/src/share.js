import { corsHeaders } from './cors';
import {
  canAccessRoomAsync,
  extractAuthToken,
  jsonError,
  normalizeAuthValue,
  normalizeRoomName,
  parseRoomAuth,
  resolveRoomAuth,
} from './auth';

export const SHARE_TOKEN_QUERY_KEY = 't';
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
      response: jsonError(401, '需要认证令牌', 'Unauthorized'),
    };
  }

  return {
    ok: false,
    room: normalizedRoom,
    token,
    requirement,
    response: jsonError(401, '无效的认证令牌', 'Unauthorized'),
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
        room: normalizeRoomName(object.customMetadata?.room || 'default'),
        expireTime: Number(object.customMetadata?.expireTime || 0),
      };
    }
  }

  if (env.DB) {
    const row = await env.DB.prepare('SELECT uuid, name, room, expireTime FROM messages WHERE uuid = ? ORDER BY id DESC LIMIT 1')
      .bind(uuid)
      .first();
    if (row) {
      return {
        uuid: row.uuid,
        name: row.name || 'file',
        room: normalizeRoomName(row.room || 'default'),
        expireTime: Number(row.expireTime || 0),
      };
    }
  }

  return null;
}

async function issueShareToken(env, { type, id, room, ttl, maxUses }) {
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
      let expiresAt = Math.floor(Date.now() / 1000) + ttl;

      if (!shareType) {
        return jsonError(400, '缺少 type', 'Bad Request');
      }

      if (shareType === 'content') {
        const id = String(body?.id || '').trim();
        if (!id) {
          return jsonError(400, '缺少 id', 'Bad Request');
        }

        const row = await findContentById(env, Number(id), requestedRoom, hasRequestedRoom);
        if (!row) {
          return jsonError(404, '内容未找到', 'Not Found');
        }

        const room = normalizeRoomName(row.room || 'default');
        if (!await canAccessRoomAsync(env, room, authToken)) {
          return jsonError(401, '无权访问该房间', 'Unauthorized');
        }

        const requirement = resolveRoomAuth(env, room);
        const target = new URL(`${url.origin}/api/content/${id}`);
        if (room !== 'default') {
          target.searchParams.set('room', room);
        }

        const response = {
          type: 'content',
          id,
          room,
          ttl,
          expiresAt,
          maxUses,
          url: target.toString(),
        };

        if (requirement.required) {
          const issued = await issueShareToken(env, {
            type: 'content',
            id,
            room,
            ttl,
            maxUses,
          });
          expiresAt = issued.expiresAt;
          response.expiresAt = expiresAt;
          target.searchParams.set(SHARE_TOKEN_QUERY_KEY, issued.token);
          response.token = issued.token;
          response.url = target.toString();
        }

        return new Response(JSON.stringify(response), {
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      if (shareType === 'file') {
        const uuid = String(body?.uuid || body?.id || '').trim();
        if (!uuid) {
          return jsonError(400, '缺少 uuid', 'Bad Request');
        }

        const fileMeta = await findFileMeta(env, uuid);
        if (!fileMeta) {
          return jsonError(404, '文件未找到或已过期', 'Not Found');
        }

        const now = Math.floor(Date.now() / 1000);
        if (fileMeta.expireTime > 0 && fileMeta.expireTime < now) {
          return jsonError(404, '文件已过期', 'Not Found');
        }

        const room = normalizeRoomName(fileMeta.room);
        if (hasRequestedRoom && room !== requestedRoom) {
          return jsonError(404, '文件未找到或已过期', 'Not Found');
        }
        if (!await canAccessRoomAsync(env, room, authToken)) {
          return jsonError(401, '无权访问该房间', 'Unauthorized');
        }

        const requirement = resolveRoomAuth(env, room);
        const filename = encodeURIComponent(fileMeta.name || 'file');
        const target = new URL(`${url.origin}/api/file/${uuid}/${filename}`);
        const response = {
          type: 'file',
          uuid,
          room,
          ttl,
          expiresAt,
          maxUses,
          url: target.toString(),
        };

        if (requirement.required) {
          const issued = await issueShareToken(env, {
            type: 'file',
            id: uuid,
            room,
            ttl,
            maxUses,
          });
          expiresAt = issued.expiresAt;
          response.expiresAt = expiresAt;
          target.searchParams.set(SHARE_TOKEN_QUERY_KEY, issued.token);
          response.token = issued.token;
          response.url = target.toString();
        }

        return new Response(JSON.stringify(response), {
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      return jsonError(400, '不支持的 type', 'Bad Request');
    } catch (error) {
      console.error('Share create error:', error);
      return jsonError(500, '生成分享链接失败', 'Internal Server Error');
    }
  }
}
