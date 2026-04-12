import { corsHeaders } from '../cors';
import { buildSenderDevice, saveToD1, broadcastMessage, generateUUID } from '../utils';
import { ensureRoomAccess, normalizeRoomName } from '../auth';

export class FileHandler {
  static async upload(request, env) {
    try {
      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      const authResult = ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }

      if (!env.R2_BUCKET) {
        return new Response(JSON.stringify({
          error: 'Storage not available',
          message: '文件存储服务不可用'
        }), {
          status: 503,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      const formData = await request.formData();
      const file = formData.get('file');

      if (!file) {
        return new Response(JSON.stringify({
          error: 'No file provided',
          message: '未提供文件'
        }), {
          status: 400,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 检查文件大小限制
      const fileLimit = env.FILE_LIMIT ? parseInt(env.FILE_LIMIT) : 104857600; // 100MB
      if (file.size > fileLimit) {
        return new Response(JSON.stringify({
          error: 'File too large',
          message: `文件大小超出限制 (最大 ${Math.floor(fileLimit / 1024 / 1024)}MB)`
        }), {
          status: 413,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      const uuid = generateUUID();
      const currentTime = Math.floor(Date.now() / 1000);
      const expireSeconds = env.FILE_EXPIRE ? parseInt(env.FILE_EXPIRE) : 3600;
      const expireTime = currentTime + expireSeconds;

      // 上传文件到 R2
      await env.R2_BUCKET.put(`files/${uuid}`, file.stream(), {
        httpMetadata: {
          contentType: file.type || 'application/octet-stream',
          contentDisposition: `inline; filename="${file.name}"`
        },
        customMetadata: {
          originalName: file.name,
          uploadTime: Date.now().toString(),
          expireTime: expireTime.toString(),
          room: room
        }
      });

      const fileUrl = `${url.origin}/api/file/${uuid}/${encodeURIComponent(file.name)}`;

      // 创建消息记录
      const messageData = {
        type: 'file',
        name: file.name,
        size: file.size,
        room,
        timestamp: Math.floor(Date.now() / 1000), // 使用毫秒时间戳
        senderIP: request.headers.get('CF-Connecting-IP') || 'unknown',
        userAgent: request.headers.get('User-Agent') || 'unknown',
        uuid,
        expireTime,
        url: fileUrl
      };
      const senderDevice = buildSenderDevice(messageData.userAgent);

      // 保存到 D1 并获取清理结果
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
          expire: expireTime,
          cache: uuid,
          senderDevice,
        }
      });

      const contentURL = `${url.origin}/api/content/${messageId}${room !== 'default' ? `?room=${room}` : ''}`;

      return new Response(JSON.stringify({
        id: messageId.toString(),
        type: this.determineFileType(file.name),
        url: contentURL
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });

    } catch (error) {
      console.error('File upload error:', error);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '上传文件时发生错误'
      }), {
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }

  static async download(request, env) {
    try {
      const { uuid, filename } = request.params;
      console.log(`文件下载请求: UUID ${uuid}, filename: ${filename}`);

      const object = env.R2_BUCKET ? await env.R2_BUCKET.get(`files/${uuid}`) : null;
      const room = normalizeRoomName(object?.customMetadata?.room || 'default');
      const authResult = ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }
      
      if (!env.R2_BUCKET) {
        return new Response('Storage not available', { 
          status: 503,
          headers: corsHeaders 
        });
      }
      
      if (!object) {
        console.log(`文件未找到: ${uuid}`);
        return new Response('File not found', { 
          status: 404,
          headers: corsHeaders 
        });
      }

      // 检查文件是否过期
      const expireTime = parseInt(object.customMetadata?.expireTime || '0');
      const currentTime = Math.floor(Date.now() / 1000);
      if (expireTime > 0 && currentTime > expireTime) {
        console.log(`文件已过期: ${uuid}, expireTime: ${expireTime}, currentTime: ${currentTime}`);
        // 删除过期文件
        await env.R2_BUCKET.delete(`files/${uuid}`);
        return new Response('File expired', { 
          status: 404,
          headers: corsHeaders 
        });
      }

      console.log(`文件下载成功: ${uuid}, size: ${object.size}, type: ${object.httpMetadata?.contentType}`);

      const headers = {
        'Content-Type': object.httpMetadata?.contentType || 'application/octet-stream',
        'Content-Length': object.size.toString(),
        'Cache-Control': 'public, max-age=3600',
        'Last-Modified': new Date(parseInt(object.customMetadata?.uploadTime || Date.now())).toUTCString(),
        ...corsHeaders
      };

      // 如果请求包含 download=true，设置为附件下载
      const url = new URL(request.url);
      const originalName = object.customMetadata?.originalName || filename || 'file';
      
      if (url.searchParams.get('download') === 'true') {
        headers['Content-Disposition'] = `attachment; filename="${encodeURIComponent(originalName)}"`;
      } else {
        headers['Content-Disposition'] = `inline; filename="${encodeURIComponent(originalName)}"`;
      }

      return new Response(object.body, { headers });

    } catch (error) {
      console.error('File download error:', error);
      console.error('Error stack:', error.stack);
      return new Response('Internal Server Error', { 
        status: 500,
        headers: corsHeaders
      });
    }
  }

  static async delete(request, env) {
    try {
      let room = 'default';
      if (env.R2_BUCKET) {
        const object = await env.R2_BUCKET.head(`files/${request.params.uuid}`);
        if (object?.customMetadata?.room) {
          room = normalizeRoomName(object.customMetadata.room);
        }
      }

      if (room === 'default' && env.DB) {
        const fileRecord = await env.DB.prepare('SELECT room FROM messages WHERE uuid = ? ORDER BY id DESC LIMIT 1')
          .bind(request.params.uuid)
          .first();
        if (fileRecord?.room) {
          room = normalizeRoomName(fileRecord.room);
        }
      }

      const authResult = ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }

      const { uuid } = request.params;
      console.log(`删除文件请求: UUID ${uuid}`);
      
      if (env.R2_BUCKET) {
        // 从 R2 删除文件
        await env.R2_BUCKET.delete(`files/${uuid}`);
        console.log(`文件已从 R2 删除: ${uuid}`);
      }
      
      if (env.DB) {
        // 从 D1 删除相关记录
        await env.DB.prepare('DELETE FROM messages WHERE uuid = ?').bind(uuid).run();
        console.log(`文件记录已从 D1 删除: ${uuid}`);
      }

      return new Response(JSON.stringify({
        status: '文件删除成功'
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });

    } catch (error) {
      console.error('File delete error:', error);
      console.error('Error stack:', error.stack);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '删除文件时发生错误'
      }), {
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }

  static determineFileType(filename) {
    if (!filename) return 'file';
    
    const ext = filename.split('.').pop()?.toLowerCase();
    const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico'];
    const videoExts = ['mp4', 'webm', 'ogg', 'mov', 'avi', 'mkv', 'm4v'];
    const audioExts = ['mp3', 'wav', 'ogg', 'm4a', 'aac', 'flac'];
    
    if (imageExts.includes(ext)) return 'image';
    if (videoExts.includes(ext)) return 'video';
    if (audioExts.includes(ext)) return 'audio';
    return 'file';
  }

  // 添加获取文件类型图标的方法
  static getFileTypeIcon(filename) {
    const type = FileHandler.determineFileType(filename);
    const iconMap = {
      'image': '🖼️',
      'video': '🎬',
      'audio': '🎵',
      'file': '📄'
    };
    return iconMap[type] || '📄';
  }

  // 添加获取文件大小格式化的方法
  static formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
}