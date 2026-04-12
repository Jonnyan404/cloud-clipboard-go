import { corsHeaders } from './cors';

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

  return String(value);
}

export function parseRoomAuth(env) {
  const roomAuth = env.ROOM_AUTH;
  if (!roomAuth) {
    return {};
  }

  if (typeof roomAuth === 'object') {
    return Object.entries(roomAuth).reduce((acc, [room, password]) => {
      acc[normalizeRoomName(room)] = normalizeAuthValue(password);
      return acc;
    }, {});
  }

  try {
    const parsed = JSON.parse(roomAuth);
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }

    return Object.entries(parsed).reduce((acc, [room, password]) => {
      acc[normalizeRoomName(room)] = normalizeAuthValue(password);
      return acc;
    }, {});
  } catch (error) {
    console.error('ROOM_AUTH 解析失败:', error);
    return {};
  }
}

export function resolveRoomAuth(env, room) {
  const normalizedRoom = normalizeRoomName(room);
  const globalPassword = normalizeAuthValue(env.AUTH_PASSWORD);
  const roomAuth = parseRoomAuth(env);
  const hasRoomPassword = Object.prototype.hasOwnProperty.call(roomAuth, normalizedRoom);
  const roomPassword = hasRoomPassword ? normalizeAuthValue(roomAuth[normalizedRoom]) : '';

  if (roomPassword) {
    return { room: normalizedRoom, required: true, password: roomPassword };
  }

  if (globalPassword) {
    return { room: normalizedRoom, required: true, password: globalPassword };
  }

  if (hasRoomPassword) {
    return { room: normalizedRoom, required: false, password: '' };
  }

  return { room: normalizedRoom, required: false, password: '' };
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
    ? normalizeAuthValue(roomAuth[normalizedRoom])
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

export function ensureRoomAccess(request, env, room) {
  const normalizedRoom = normalizeRoomName(room);
  const requirement = resolveRoomAuth(env, normalizedRoom);
  const token = extractAuthToken(request);

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

  if (!tokenMatchesRoom(env, normalizedRoom, token)) {
    return {
      ok: false,
      room: normalizedRoom,
      token,
      requirement,
      response: jsonError(401, '无效的认证令牌', 'Unauthorized'),
    };
  }

  return { ok: true, room: normalizedRoom, token, requirement };
}
