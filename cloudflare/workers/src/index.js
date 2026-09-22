import { Router } from 'itty-router';
import { corsHeaders, handleCors } from './cors';
import { canAccessRoom, canAccessRoomAsync, resolveRoomAuth, issueRoomSessionToken, validateRoomSessionToken, parseRoomSessionToken, extractAuthToken } from './auth';
import { TextHandler } from './handlers/text';
import { FileHandler } from './handlers/file';
import { ContentHandler } from './handlers/content';
import { RoomsHandler } from './handlers/rooms';
import { WebSocketHandler } from './handlers/websocket';
import { ShareHandler } from './share';
import { errorResponse } from './errors';

// 导入 Durable Objects
export { WebSocketRoom } from './durable-objects/websocket-room';

const router = Router();

function isRoomListEnabled(env) {
  return ['1', 'true', 'yes', 'on'].includes(String(env.ROOM_LIST || '').toLowerCase());
}

// CORS 预检请求
router.options('*', handleCors);

// API 路由（无 /api 前缀，与自托管 Go 后端路径对齐）
router.get('/server', handleServer);
router.post('/auth/token', handleAuthToken);
router.post('/auth/token/refresh', handleAuthTokenRefresh);
router.get('/rooms', RoomsHandler.list);
router.post('/text', TextHandler.create);
router.post('/share', ShareHandler.create);
// 分享页在取正文之前先问一次：类型 / 文件名 / 大小 / 剩余有效期 / 是否需要密码。
router.get('/share', ShareHandler.info);
router.get('/content/latest', ContentHandler.getLatest);
router.get('/content/latest.json', ContentHandler.getLatest);
router.get('/content/:id', ContentHandler.getById);
router.get('/content/:id.json', ContentHandler.getById);
// 看板：把卡片挪到某一列。`:id` 不跨 `/`，所以不会和上面两条抢。
router.post('/content/:id/column', ContentHandler.setColumn);
router.post('/upload/chunk', FileHandler.createChunk);
router.post('/upload/chunk/:uuid', FileHandler.uploadChunkPart);
router.post('/upload/finish/:uuid', FileHandler.finishChunk);
router.post('/upload/multipart/create', FileHandler.createMultipart);
router.put('/upload/multipart/:partNumber', FileHandler.uploadMultipartPart);
router.post('/upload/multipart/complete', FileHandler.completeMultipart);
router.delete('/upload/multipart', FileHandler.abortMultipart);
router.post('/upload', FileHandler.upload);
router.get('/file/:uuid/:filename?', FileHandler.download);
router.delete('/file/:uuid', FileHandler.delete);

// 添加删除消息路由
router.delete('/revoke/all', ContentHandler.revokeAll);
router.delete('/revoke/:id', ContentHandler.revoke);

// WebSocket 连接
router.get('/push', WebSocketHandler.connect);

// 健康检查
router.get('/health', () => new Response('OK'));

// 兜底路由：资源层没命中、上面也没命中时走这里。
//
// 为什么兜底要放在 Worker 而不是资源层（not_found_handling = "single-page-application"）：
// 资源层的 SPA 回退只认「导航请求」，而浏览器点下载链接、打开分享链接正好就是导航请求，
// 于是 /file/<uuid>/<name> 会被回成 index.html，文件下载就变成了「下载到一个 html」。
// 放在这里之后，导航请求和 XHR 走同一条路，不再有两套行为。
router.all('*', handleFallback);

// 房间会话令牌有效期，默认 1 小时
const ROOM_SESSION_TTL = 3600;

// 兜底处理：GET/HEAD 回前端首页（等价于原来的 SPA 回退），其余方法明确 404。
// 前端用的是 hash 路由，所以只有手输错地址之类的场景会走到这里。
async function handleFallback(request, env) {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    return errorResponse(404, 'route_not_found', 'Not Found', '接口不存在');
  }

  if (!env.ASSETS) {
    return errorResponse(404, 'assets_missing', 'Not Found', '前端资源未部署');
  }

  const indexUrl = new URL('/index.html', request.url);
  return env.ASSETS.fetch(new Request(indexUrl, { method: request.method }));
}

// 处理 /auth/token 端点
async function handleAuthToken(request, env) {
  if (request.method !== 'POST') {
    return errorResponse(405, 'method_not_allowed', 'Method Not Allowed', '方法不允许');
  }

  const url = new URL(request.url);
  const room = url.searchParams.get('room') || 'default';

  try {
    const body = await request.json();
    const password = String(body.password || '').trim();

    if (!password) {
      return errorResponse(401, 'password_required', 'Password required', '密码不能为空');
    }

    if (!canAccessRoom(env, room, password)) {
      return errorResponse(401, 'wrong_password', 'Wrong password', '密码不正确');
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
    return errorResponse(500, 'token_issue_failed', 'Failed to issue token', '令牌签发失败');
  }
}

// 处理 /auth/token/refresh 端点：使用仍有效的会话令牌续签，无需密码即可静默续期
async function handleAuthTokenRefresh(request, env) {
  if (request.method !== 'POST') {
    return errorResponse(405, 'method_not_allowed', 'Method Not Allowed', '方法不允许');
  }

  const url = new URL(request.url);
  const room = url.searchParams.get('room') || 'default';
  const token = extractAuthToken(request);

  const claims = await parseRoomSessionToken(env, token);
  if (!claims || !(await validateRoomSessionToken(env, room, token))) {
    return errorResponse(401, 'session_token_invalid', 'Session token invalid or expired', '会话令牌无效或已过期');
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
    return errorResponse(500, 'token_refresh_failed', 'Failed to refresh token', '令牌续签失败');
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
    roomProtected = requirement.required;
  } else if (globalPassword) {
    authRequired = true;
    authorized = await canAccessRoomAsync(env, 'default', token);
  }
  
  return new Response(JSON.stringify({
    server: `${wsProtocol}//${url.host}/push`,
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