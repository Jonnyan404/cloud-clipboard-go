import { corsHeaders } from '../cors';
import { buildSenderDevice, saveToD1, broadcastMessage } from '../utils';
import { ensureRoomAccess, normalizeRoomName } from '../auth';
import { errorResponse } from '../errors';

/**
 * 从请求体里取正文。只认两种结构化形态，**其余一律当纯文本**：
 *
 *   · application/json    → {"content": "..."}
 *   · multipart/form-data → 表单字段 content
 *   · 其它（含不声明、含 urlencoded） → 整个请求体就是正文
 *
 * ⚠️ `application/x-www-form-urlencoded` **刻意不认**：它是 `curl --data-binary` 之类
 * 不带 `-H` 时的**默认** Content-Type，很多老调用方都这样发正文；当表单解析的话，
 * 一段没有 `=` 的正文会解析出空的 content —— 不是报错，是**静默存成空串**。
 *
 * 为什么要有前两条：快捷指令把**字符串变量**当请求体发出去时字节会变成 UTF-16，
 * 而结构化请求体（JSON / 表单）是按 UTF-8 序列化的。Go 侧 readTextBody 是同一套契约。
 */
async function readTextBody(request) {
  // 只取 media type，丢掉 charset 之类的参数
  const mediaType = (request.headers.get('Content-Type') || '').toLowerCase().split(';')[0].trim();

  if (mediaType === 'application/json') {
    const payload = await request.json().catch(() => null);
    if (!payload || typeof payload !== 'object') {
      throw new Error('JSON 正文解析失败');
    }
    return typeof payload.content === 'string' ? payload.content : '';
  }

  if (mediaType === 'multipart/form-data') {
    const form = await request.formData().catch(() => null);
    if (!form) {
      throw new Error('表单正文解析失败');
    }
    const value = form.get('content');
    return typeof value === 'string' ? value : '';
  }

  return request.text();
}

export class TextHandler {
  static async create(request, env) {
    try {
      console.log('处理文本创建请求');

      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      const targetMessageId = url.searchParams.get('id');
      const authResult = await ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        console.log('文本创建请求认证失败');
        return authResult.response;
      }
      
      console.log(`文本消息房间: ${room}`);
      
      // 正文可以是纯文本、JSON 或表单 —— 见 readTextBody 上面那段说明
      let content;
      try {
        content = await readTextBody(request);
      } catch (error) {
        console.log('解析文本请求体失败:', error.message);
        return errorResponse(400, 'invalid_body', 'Cannot parse request body', '请求体无法解析');
      }
      
      if (!content || content.trim() === '') {
        console.log('文本内容为空');
        return errorResponse(400, 'empty_content', 'Empty content', '内容不能为空');
      }

      console.log(`接收到文本内容: ${content.substring(0, 100)}...`);

      // 检查文本长度限制
      const textLimit = env.TEXT_LIMIT ? parseInt(env.TEXT_LIMIT) : 4096;
      if (content.length > textLimit) {
        console.log(`文本长度超限: ${content.length} > ${textLimit}`);
        return errorResponse(413, 'text_too_long', 'Text too long', `文本长度超出限制 (最大 ${textLimit} 字符)`);
      }

      if (targetMessageId) {
        const updated = await TextHandler.update(request, env, {
          id: targetMessageId,
          room,
          content,
          url,
        });

        return updated;
      }

      // 创建消息记录
      const messageData = {
        type: 'text',
        content: content,
        room,
        timestamp: Math.floor(Date.now() / 1000), // 使用毫秒时间戳
        senderIP: request.headers.get('CF-Connecting-IP') || 'unknown',
        senderClientID: String(url.searchParams.get('client') || '').trim(), // 前端每客户端持久ID
        userAgent: request.headers.get('User-Agent') || 'unknown',
        deviceName: String(url.searchParams.get('name') || '') // 客户端声明的设备名，空表示未声明
      };
      const senderDevice = buildSenderDevice(messageData.userAgent, messageData.deviceName);

      console.log('准备保存文本消息:', messageData);

      // 检查 DB binding
      if (!env.DB) {
        console.error('DB binding 不存在!');
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库服务不可用');
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
          id: messageId,
          senderDevice,
        }
      });

      const contentURL = `${url.origin}/content/${messageId}${room !== 'default' ? `?room=${room}` : ''}`;

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
      console.error('Text handler stack:', error?.stack || '(no stack)');
      const reason = String(error?.message || error || 'unknown');
      console.error('Text handler reason:', reason);
      return errorResponse(500, 'internal_error', 'Internal Server Error', `处理文本时发生错误: ${reason}`);
    }
  }

  static async update(request, env, { id, room, content, url }) {
    const numericId = parseInt(id, 10);
    if (!Number.isInteger(numericId) || numericId <= 0) {
      return errorResponse(400, 'invalid_id', 'Invalid message id', '无效的 ID 参数');
    }

    if (!env.DB) {
      return errorResponse(503, 'database_unavailable', 'Database not available', '数据库服务不可用');
    }

    const existingMessage = await env.DB.prepare(
      'SELECT id, type, content, room FROM messages WHERE id = ? AND room = ?'
    ).bind(numericId, room).first();

    if (!existingMessage || existingMessage.type !== 'text') {
      return errorResponse(404, 'message_not_updatable', 'Message not found', '消息未找到或无法更新');
    }

    const contentURL = `${url.origin}/content/${numericId}${room !== 'default' ? `?room=${room}` : ''}`;

    if (existingMessage.content === content) {
      return new Response(JSON.stringify({
        id: numericId.toString(),
        type: 'text',
        url: contentURL
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }

    const timestamp = Math.floor(Date.now() / 1000);
    const senderIP = request.headers.get('CF-Connecting-IP') || 'unknown';
    const senderClientID = String(url.searchParams.get('client') || '').trim();
    const userAgent = request.headers.get('User-Agent') || 'unknown';
    // 更新的是同一台设备自己发的消息，deviceName 列保持原值不动，这里只用于广播载荷
    const senderDevice = buildSenderDevice(userAgent, url.searchParams.get('name'));

    await env.DB.prepare(`
      UPDATE messages
      SET content = ?, timestamp = ?, senderIP = ?, senderClientID = ?, userAgent = ?
      WHERE id = ? AND room = ? AND type = 'text'
    `).bind(content, timestamp, senderIP, senderClientID, userAgent, numericId, room).run();

    await broadcastMessage(env, room, {
      event: 'update',
      data: {
        id: numericId,
        type: 'text',
        content,
        timestamp,
        room,
        senderIP,
        senderClientID,
        senderDevice,
      }
    });

    return new Response(JSON.stringify({
      id: numericId.toString(),
      type: 'text',
      url: contentURL
    }), {
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }
}