import { corsHeaders } from './cors';

function textToBytes(text) {
  return new TextEncoder().encode(text);
}

function bytesToBase64Url(bytes) {
  return btoa(String.fromCharCode.apply(null, bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

function base64UrlToBytes(str) {
  const padded = str + '==='.slice(0, (4 - str.length % 4) % 4);
  const binary = atob(padded.replace(/-/g, '+').replace(/_/g, '/'));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

async function sha256(data) {
  return new Uint8Array(await crypto.subtle.digest('SHA-256', data));
}

export function normalizeRoomName(room = '') {
  const normalized = String(room || '').trim();
  return normalized === '' || normalized === 'default' ? 'default' : normalized;
}

export function extractAuthToken(request) {
  const authHeader = request.headers.get('Authorization');
  if (authHeader) {
    const parts = authHeader.split(' ');
    if (parts.length === 2 && parts[0].toLowerCase() === 'bearer') {
      return parts[1];
    }
    return authHeader;
  }

  return new URL(request.url).searchParams.get('auth') || '';
}

// 提取 WebSocket 握手使用的 token。
// 优先取 Authorization / ?auth= 以兼容旧客户端，其次取 Sec-WebSocket-Protocol 子协议，
// 避免凭据出现在 URL 中泄漏到访问日志。
export function extractWebSocketToken(request) {
  const token = extractAuthToken(request);
  if (token) {
    return token;
  }

  const protocols = request.headers.get('Sec-WebSocket-Protocol') || '';
  for (const protocol of protocols.split(',')) {
    const normalized = protocol.trim();
    if (normalized) {
      return normalized;
    }
  }
  return '';
}

export function extractAuthTokens(request) {
  const tokens = [];
  const pushToken = value => {
    const normalized = normalizeAuthValue(value);
    if (normalized && !tokens.includes(normalized)) {
      tokens.push(normalized);
    }
  };

  pushToken(extractAuthToken(request));

  const extraHeader = request.headers.get('X-Room-Auth-Tokens');
  if (!extraHeader) {
    return tokens;
  }

  try {
    const parsed = JSON.parse(extraHeader);
    if (Array.isArray(parsed)) {
      parsed.forEach(pushToken);
    }
  } catch {
    extraHeader.split(',').forEach(pushToken);
  }

  return tokens;
}

export function normalizeAuthValue(value) {
  if (value === undefined || value === null || value === false) {
    return '';
  }

  if (typeof value === 'string') {
    return value.trim();
  }

  if (typeof value === 'number' && !Number.isFinite(value)) {
    return '';
  }

  return String(value);
}

// 解析房间配置里的 fileExpire 字段：
//   未定义 -> undefined（使用全局 FILE_EXPIRE）
//   0      -> 0（该房间文件永不过期）
//   >0     -> 覆盖全局过期秒数
//   其它    -> 视为非法配置，回退全局值并告警
function parseFileExpireValue(room, value) {
  if (value === undefined || value === null || value === false) {
    return undefined;
  }
  const numeric = typeof value === 'number' ? value : Number(String(value).trim());
  if (Number.isFinite(numeric) && numeric >= 0) {
    return Math.floor(numeric);
  }
  console.warn(`ROOM_AUTH_JSON 配置警告: 房间 '${room}' 的 fileExpire 非法 (${JSON.stringify(value)})，已回退为全局 FILE_EXPIRE`);
  return undefined;
}

// 单个房间的 ROOM_AUTH_JSON 值支持两种形式：
//   字符串/数字                 -> 仅密码（旧格式，完全兼容）
//   对象 {password, fileExpire} -> 密码 + 文件过期覆盖策略
function parseRoomAuthEntry(room, value) {
  if (value !== null && typeof value === 'object') {
    return {
      password: normalizeAuthValue(value.password),
      fileExpire: parseFileExpireValue(room, value.fileExpire),
    };
  }
  return { password: normalizeAuthValue(value), fileExpire: undefined };
}

export function parseRoomAuth(env) {
  const roomAuth = env.ROOM_AUTH_JSON;
  if (!roomAuth) {
    return {};
  }

  const reduce = entries => entries.reduce((acc, [room, value]) => {
    acc[normalizeRoomName(room)] = parseRoomAuthEntry(room, value);
    return acc;
  }, {});

  if (typeof roomAuth === 'object') {
    return reduce(Object.entries(roomAuth));
  }

  try {
    const parsed = JSON.parse(roomAuth);
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }

    return reduce(Object.entries(parsed));
  } catch (error) {
    console.error('ROOM_AUTH_JSON 解析失败:', error);
    return {};
  }
}

export function resolveRoomAuth(env, room) {
  const normalizedRoom = normalizeRoomName(room);
  const globalPassword = normalizeAuthValue(env.AUTH_PASSWORD);
  const roomAuth = parseRoomAuth(env);
  const hasRoomEntry = Object.prototype.hasOwnProperty.call(roomAuth, normalizedRoom);
  const entry = hasRoomEntry ? roomAuth[normalizedRoom] : { password: '', fileExpire: undefined };
  const roomPassword = entry.password;

  if (roomPassword) {
    return { room: normalizedRoom, required: true, password: roomPassword, fileExpire: entry.fileExpire };
  }

  if (globalPassword) {
    return { room: normalizedRoom, required: true, password: globalPassword, fileExpire: entry.fileExpire };
  }

  return { room: normalizedRoom, required: false, password: '', fileExpire: entry.fileExpire };
}

export function tokenMatchesRoom(env, room, token) {
  const normalizedToken = normalizeAuthValue(token);
  if (!normalizedToken) {
    return false;
  }

  const globalPassword = normalizeAuthValue(env.AUTH_PASSWORD);
  if (globalPassword && normalizedToken === globalPassword) {
    return true;
  }

  const normalizedRoom = normalizeRoomName(room);
  const roomAuth = parseRoomAuth(env);
  const roomPassword = Object.prototype.hasOwnProperty.call(roomAuth, normalizedRoom)
    ? roomAuth[normalizedRoom].password
    : '';

  return !!roomPassword && normalizedToken === roomPassword;
}

export function canAccessRoom(env, room, token) {
  const requirement = resolveRoomAuth(env, room);
  if (!requirement.required) {
    return true;
  }

  return tokenMatchesRoom(env, room, token);
}

export async function canAccessRoomAsync(env, room, token) {
  const requirement = resolveRoomAuth(env, room);
  if (!requirement.required) {
    return true;
  }

  // Try room session token first
  if (await validateRoomSessionToken(env, room, token)) {
    return true;
  }

  // Fall back to password/bearer token
  return tokenMatchesRoom(env, room, token);
}

export function hasRoomAuthEntry(env, room) {
  const normalizedRoom = normalizeRoomName(room);
  const roomAuth = parseRoomAuth(env);
  return Object.prototype.hasOwnProperty.call(roomAuth, normalizedRoom);
}

export function jsonError(status, message, error = null) {
  return new Response(JSON.stringify({
    error: error || (status === 401 ? 'Unauthorized' : 'Error'),
    message,
  }), {
    status,
    headers: { 'Content-Type': 'application/json', ...corsHeaders },
  });
}

export async function ensureRoomAccess(request, env, room, tokenOverride) {
  const normalizedRoom = normalizeRoomName(room);
  const requirement = resolveRoomAuth(env, normalizedRoom);
  const token = tokenOverride || extractAuthToken(request);

  if (!requirement.required) {
    return { ok: true, room: normalizedRoom, token, requirement };
  }

  if (!token) {
    return {
      ok: false,
      room: normalizedRoom,
      token,
      requirement,
      response: jsonError(401, '需要认证令牌', 'Unauthorized'),
    };
  }

  if (await validateRoomSessionToken(env, normalizedRoom, token)) {
    return { ok: true, room: normalizedRoom, token, requirement };
  }

  if (tokenMatchesRoom(env, normalizedRoom, token)) {
    return { ok: true, room: normalizedRoom, token, requirement };
  }

  return {
    ok: false,
    room: normalizedRoom,
    token,
    requirement,
    response: jsonError(401, '无效的认证令牌', 'Unauthorized'),
  };
}

async function getRoomSessionSigningKey(env) {
  const material = [];
  material.push('cloud-clipboard-room-session-v1');
  
  const globalPassword = normalizeAuthValue(env.AUTH_PASSWORD);
  if (globalPassword) {
    material.push(globalPassword);
  }

  const roomAuth = parseRoomAuth(env);
  const rooms = Object.keys(roomAuth).sort();
  for (const room of rooms) {
    material.push(room);
    material.push(normalizeAuthValue(roomAuth[room]));
  }

  if (env.ROOM_SESSION_SECRET) {
    material.push(String(env.ROOM_SESSION_SECRET));
  }

  const keyMaterial = textToBytes(material.join('\x00'));
  const keyHash = await sha256(keyMaterial);

  return crypto.subtle.importKey(
    'raw',
    keyHash,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign', 'verify'],
  );
}

export async function issueRoomSessionToken(env, room, ttlSeconds = 3600, scope = '') {
  const normalizedRoom = normalizeRoomName(room);
  
  // Clamp TTL to valid range
  let validTtl = Math.max(60, Math.min(ttlSeconds, 24 * 60 * 60));

  const now = Math.floor(Date.now() / 1000);
  const claims = {
    typ: 'room_session',
    room: normalizedRoom,
    exp: now + validTtl,
  };
  // scope: ""=房间专属, "global"=全局所有房间
  if (scope === 'global') {
    claims.scope = 'global';
  }

  const payload = bytesToBase64Url(textToBytes(JSON.stringify(claims)));
  const key = await getRoomSessionSigningKey(env);
  const signature = await crypto.subtle.sign('HMAC', key, textToBytes(payload));
  
  return `${payload}.${bytesToBase64Url(new Uint8Array(signature))}`;
}

// 解析并校验会话令牌（不含房间匹配），返回 claims；无效返回 null
export async function parseRoomSessionToken(env, token) {
  const normalized = String(token || '').trim();
  if (!normalized) {
    return null;
  }

  const parts = normalized.split('.');
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    return null;
  }

  try {
    const key = await getRoomSessionSigningKey(env);
    const valid = await crypto.subtle.verify(
      'HMAC',
      key,
      base64UrlToBytes(parts[1]),
      textToBytes(parts[0]),
    );

    if (!valid) {
      return null;
    }

    const claimsBytes = base64UrlToBytes(parts[0]);
    const claimsText = new TextDecoder().decode(claimsBytes);
    const claims = JSON.parse(claimsText);

    if (claims.typ !== 'room_session') {
      return null;
    }

    if (!claims.exp || claims.exp < Math.floor(Date.now() / 1000)) {
      return null;
    }

    return claims;
  } catch (error) {
    console.error('Failed to parse room session token:', error);
    return null;
  }
}

export async function validateRoomSessionToken(env, room, token) {
  const claims = await parseRoomSessionToken(env, token);
  if (!claims) {
    return false;
  }

  const normalizedRoom = normalizeRoomName(room);
  // 全局会话令牌对所有房间有效
  if (claims.scope === 'global') {
    return true;
  }

  if (!claims.room || normalizeRoomName(claims.room) !== normalizedRoom) {
    return false;
  }

  return true;
}
