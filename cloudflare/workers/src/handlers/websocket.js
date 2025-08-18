import { corsHeaders } from '../cors';

export class WebSocketHandler {
  static async connect(request, env) {
    try {
      const url = new URL(request.url);
      const room = url.searchParams.get('room') || 'default';
      
      console.log(`WebSocket 连接请求: room=${room}, url=${url.toString()}`);
      
      // 认证检查 - 匹配 Go 版本的逻辑
      let authNeeded = false;
      let expectedPassword = '';
      
      if (env.AUTH_PASSWORD) {
        authNeeded = true;
        expectedPassword = env.AUTH_PASSWORD;
      }

      if (authNeeded) {
        const token = url.searchParams.get('auth');
        if (!token || token !== expectedPassword) {
          console.log('WebSocket 认证失败');
          return new Response('Unauthorized', {
            status: 401,
            headers: corsHeaders
          });
        }
        console.log('WebSocket 认证成功');
      }

      // 检查是否为 WebSocket 升级请求
      const upgradeHeader = request.headers.get('Upgrade');
      if (!upgradeHeader || upgradeHeader.toLowerCase() !== 'websocket') {
        console.log('不是 WebSocket 升级请求');
        return new Response('Expected WebSocket', { 
          status: 400,
          headers: corsHeaders
        });
      }

      // 检查必要的 WebSocket 头部
      const connectionHeader = request.headers.get('Connection');
      const wsKeyHeader = request.headers.get('Sec-WebSocket-Key');
      const wsVersionHeader = request.headers.get('Sec-WebSocket-Version');
      
      if (!connectionHeader || !wsKeyHeader || !wsVersionHeader) {
        console.log('缺少必要的 WebSocket 头部');
        return new Response('Invalid WebSocket headers', {
          status: 400,
          headers: corsHeaders
        });
      }

      console.log('准备创建 Durable Object 连接');

      // 确保 WEBSOCKET_ROOM binding 存在
      if (!env.WEBSOCKET_ROOM) {
        console.error('WEBSOCKET_ROOM binding 不存在');
        return new Response('WebSocket service unavailable', {
          status: 503,
          headers: corsHeaders
        });
      }

      // 使用 Durable Objects 处理 WebSocket 连接
      const durableObjectId = env.WEBSOCKET_ROOM.idFromName(room);
      const durableObject = env.WEBSOCKET_ROOM.get(durableObjectId);
      
      console.log(`转发到 Durable Object, room: ${room}`);
      
      // 转发请求到 Durable Object，保持原始头部和查询参数
      return await durableObject.fetch(request);

    } catch (error) {
      console.error('WebSocket handler error:', error);
      console.error('Error stack:', error.stack);
      
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: error.message,
        stack: error.stack
      }), { 
        status: 500,
        headers: {
          'Content-Type': 'application/json',
          ...corsHeaders
        }
      });
    }
  }
}