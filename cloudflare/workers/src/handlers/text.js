import { corsHeaders } from '../cors';
import { saveToD1, broadcastMessage } from '../utils';

export class TextHandler {
  static async create(request, env) {
    try {
      console.log('处理文本创建请求');
      
      // 认证检查
      if (env.AUTH_PASSWORD) {
        const auth = request.headers.get('Authorization') || 
                    new URL(request.url).searchParams.get('auth');
        if (!auth || auth.replace('Bearer ', '') !== env.AUTH_PASSWORD) {
          console.log('文本创建请求认证失败');
          return new Response(JSON.stringify({
            error: 'Unauthorized',
            message: '需要认证令牌'
          }), {
            status: 401,
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        }
      }

      const url = new URL(request.url);
      const room = url.searchParams.get('room') || 'default';
      
      console.log(`文本消息房间: ${room}`);
      
      const content = await request.text();
      
      if (!content || content.trim() === '') {
        console.log('文本内容为空');
        return new Response(JSON.stringify({
          error: 'Empty content',
          message: '内容不能为空'
        }), {
          status: 400,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      console.log(`接收到文本内容: ${content.substring(0, 100)}...`);

      // 检查文本长度限制
      const textLimit = env.TEXT_LIMIT ? parseInt(env.TEXT_LIMIT) : 4096;
      if (content.length > textLimit) {
        console.log(`文本长度超限: ${content.length} > ${textLimit}`);
        return new Response(JSON.stringify({
          error: 'Text too long',
          message: `文本长度超出限制 (最大 ${textLimit} 字符)`
        }), {
          status: 413,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 创建消息记录
      const messageData = {
        type: 'text',
        content: content,
        room,
        timestamp: Math.floor(Date.now() / 1000), // 使用毫秒时间戳
        senderIP: request.headers.get('CF-Connecting-IP') || 'unknown',
        userAgent: request.headers.get('User-Agent') || 'unknown'
      };

      console.log('准备保存文本消息:', messageData);

      // 检查 DB binding
      if (!env.DB) {
        console.error('DB binding 不存在!');
        return new Response(JSON.stringify({
          error: 'Database not available',
          message: '数据库服务不可用'
        }), {
          status: 503,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 保存到 D1 (会自动清理旧消息)
      const saveResult = await saveToD1(env.DB, messageData, env); // 修复：传递 env
      const messageId = saveResult.messageId;
      const filesToCleanup = saveResult.filesToCleanup;

      // 清理被删除的旧文件
      if (filesToCleanup.length > 0 && env.R2_BUCKET) {
        console.log(`清理 ${filesToCleanup.length} 个旧文件`);
        for (const fileUuid of filesToCleanup) {
          try {
            await env.R2_BUCKET.delete(`files/${fileUuid}`);
            console.log(`已删除旧文件: ${fileUuid}`);
          } catch (deleteError) {
            console.error(`删除文件失败: ${fileUuid}`, deleteError);
          }
        }
      }

      // 广播到 WebSocket 连接
      await broadcastMessage(env, room, {
        event: 'receive',
        data: {
          ...messageData,
          id: messageId
        }
      });

      const contentURL = `${url.origin}/api/content/${messageId}${room !== 'default' ? `?room=${room}` : ''}`;

      console.log(`文本消息处理完成, ID: ${messageId}, URL: ${contentURL}`);

      return new Response(JSON.stringify({
        id: messageId.toString(),
        type: 'text',
        url: contentURL
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });

    } catch (error) {
      console.error('Text handler error:', error);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '处理文本时发生错误'
      }), {
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }
}