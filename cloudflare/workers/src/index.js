import { Router } from 'itty-router';
import { corsHeaders, handleCors } from './cors';
import { TextHandler } from './handlers/text';
import { FileHandler } from './handlers/file';
import { ContentHandler } from './handlers/content';
import { WebSocketHandler } from './handlers/websocket';

// 导入 Durable Objects
export { WebSocketRoom } from './durable-objects/websocket-room';

const router = Router();

// CORS 预检请求
router.options('*', handleCors);

// API 路由
router.get('/api/server', handleServer);
router.post('/api/text', TextHandler.create);
//router.get('/api/content/latest', ContentHandler.getLatest);
//router.get('/api/content/:id', ContentHandler.getById);
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

// 处理 /server 端点
async function handleServer(request, env) {
  const url = new URL(request.url);
  const wsProtocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  
  return new Response(JSON.stringify({
    server: `${wsProtocol}//${url.host}/api/push`,
    auth: !!env.AUTH_PASSWORD,
    version: "cloudflare-worker-v1.0.0"
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
      return new Response('Internal Server Error', { 
        status: 500,
        headers: corsHeaders
      });
    }
  }
};