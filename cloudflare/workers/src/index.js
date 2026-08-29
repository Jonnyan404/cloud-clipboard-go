import { Router } from 'itty-router';
import { corsHeaders, handleCors } from './cors';
import { canAccessRoom, canAccessRoomAsync, hasRoomAuthEntry, resolveRoomAuth, issueRoomSessionToken, validateRoomSessionToken, parseRoomSessionToken, extractAuthToken } from './auth';
import { TextHandler } from './handlers/text';
import { FileHandler } from './handlers/file';
import { ContentHandler } from './handlers/content';
import { RoomsHandler } from './handlers/rooms';
import { WebSocketHandler } from './handlers/websocket';
import { ShareHandler } from './share';

// 导入 Durable Objects
export { WebSocketRoom } from './durable-objects/websocket-room';

const router = Router();

function isRoomListEnabled(env) {
  return ['1', 'true', 'yes', 'on'].includes(String(env.ROOM_LIST || '').toLowerCase());
}

// CORS 预检请求
router.options('*', handleCors);

// API 路由
router.get('/api/server', handleServer);
router.post('/api/auth/token', handleAuthToken);
router.post('/api/auth/token/refresh', handleAuthTokenRefresh);
router.get('/api/rooms', RoomsHandler.list);
router.post('/api/text', TextHandler.create);
router.post('/api/share', ShareHandler.create);
router.get('/api/content/latest', ContentHandler.getLatest);
router.get('/api/content/latest.json', ContentHandler.getLatest);
router.get('/api/content/:id', ContentHandler.getById);
router.get('/api/content/:id.json', ContentHandler.getById);
router.post('/api/upload/multipart/create', FileHandler.createMultipart);
router.put('/api/upload/multipart/:partNumber', FileHandler.uploadMultipartPart);
router.post('/api/upload/multipart/complete', FileHandler.completeMultipart);
router.delete('/api/upload/multipart', FileHandler.abortMultipart);
router.post('/api/upload', FileHandler.upload);
router.get('/api/file/:uuid/:filename?', FileHandler.download);
router.delete('/api/file/:uuid', FileHandler.delete);

// 添加删除消息路由
router.delete('/api/revoke/all', ContentHandler.revokeAll);
router.delete('/api/revoke/:id', ContentHandler.revoke);

// WebSocket 连接
router.get('/api/push', WebSocketHandler.connect);

// 健康检查
router.get('/health', () => new Response('OK'));

// 房间会话令牌有效期，默认 1 小时
const ROOM_SESSION_TTL = 3600;

// 处理 /auth/token 端点
async function handleAuthToken(request, env) {
  if (request.method !== 'POST') {
    return new Response(JSON.stringify({ error: 'Method Not Allowed' }), {
      status: 405,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  const url = new URL(request.url);
  const room = url.searchParams.get('room') || 'default';

  try {
    const body = await request.json();
    const password = String(body.password || '').trim();

    if (!password) {
      return new Response(JSON.stringify({ error: '密码不能为空' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }

    if (!canAccessRoom(env, room, password)) {
      return new Response(JSON.stringify({ error: '密码不正确' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }

    // 使用全局密码登录时，签发对所有房间有效的全局会话令牌
    const globalPassword = String(env.AUTH_PASSWORD || '').trim();
    const scope = globalPassword && password === globalPassword ? 'global' : '';

    const token = await issueRoomSessionToken(env, room, ROOM_SESSION_TTL, scope);

    return new Response(JSON.stringify({
      token,
      expiresAt: Math.floor(Date.now() / 1000) + ROOM_SESSION_TTL,
      scope,
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  } catch (error) {
    console.error('Error in handleAuthToken:', error);
    return new Response(JSON.stringify({ error: '令牌签发失败' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }
}

// 处理 /auth/token/refresh 端点：使用仍有效的会话令牌续签，无需密码即可静默续期
async function handleAuthTokenRefresh(request, env) {
  if (request.method !== 'POST') {
    return new Response(JSON.stringify({ error: 'Method Not Allowed' }), {
      status: 405,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  const url = new URL(request.url);
  const room = url.searchParams.get('room') || 'default';
  const token = extractAuthToken(request);

  const claims = await parseRoomSessionToken(env, token);
  if (!claims || !(await validateRoomSessionToken(env, room, token))) {
    return new Response(JSON.stringify({ error: '会话令牌无效或已过期' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  try {
    // 续签时保留原令牌的 scope，避免全局会话降级为房间专属
    const scope = claims.scope === 'global' ? 'global' : '';
    const newToken = await issueRoomSessionToken(env, room, ROOM_SESSION_TTL, scope);
    return new Response(JSON.stringify({
      token: newToken,
      expiresAt: Math.floor(Date.now() / 1000) + ROOM_SESSION_TTL,
      scope,
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  } catch (error) {
    console.error('Error in handleAuthTokenRefresh:', error);
    return new Response(JSON.stringify({ error: '令牌续签失败' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }
}

// 处理 /server 端点
async function handleServer(request, env) {
  const url = new URL(request.url);
  const wsProtocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const requestedRoom = url.searchParams.has('room') ? url.searchParams.get('room') : null;
  const globalPassword = String(env.AUTH_PASSWORD || '').trim();
  const token = request.headers.get('Authorization')?.replace(/^Bearer\s+/i, '') || url.searchParams.get('auth') || '';

  let authRequired = false;
  let authorized = true;
  let roomProtected = false;
  if (requestedRoom !== null) {
    const requirement = resolveRoomAuth(env, requestedRoom);
    authRequired = requirement.required;
    authorized = !requirement.required || await canAccessRoomAsync(env, requestedRoom, token);
    roomProtected = hasRoomAuthEntry(env, requestedRoom);
  } else if (globalPassword) {
    authRequired = true;
    authorized = await canAccessRoomAsync(env, 'default', token);
  }
  
  return new Response(JSON.stringify({
    server: `${wsProtocol}//${url.host}/api/push`,
    auth: authRequired,
    authorized,
    roomProtected,
    version: "cloudflare-worker-v1.0.0",
    roomList: isRoomListEnabled(env),
    history: parseInt(env.HISTORY_LIMIT || '10', 10),
  }), {
    headers: {
      'Content-Type': 'application/json',
      ...corsHeaders
    }
  });
}

export default {
  async fetch(request, env, ctx) {
    try {
      return await router.handle(request, env, ctx);
    } catch (error) {
      console.error('Worker error:', error);
      console.error('Worker stack:', error?.stack || '(no stack)');
      const reason = String(error?.message || error || 'unknown');
      return new Response(`Internal Server Error: ${reason}`, { 
        status: 500,
        headers: corsHeaders
      });
    }
  }
};