import { corsHeaders } from '../cors';
import { broadcastMessage } from '../utils';

export class ContentHandler {
  static async getLatest(request, env) {
    try {
      const url = new URL(request.url);
      const room = url.searchParams.get('room') || 'default';
      const isJSON = url.pathname.endsWith('.json') || url.searchParams.get('json') === 'true';
      const forceDownload = url.searchParams.get('download') === 'true';

      console.log(`获取最新内容: room=${room}, isJSON=${isJSON}, forceDownload=${forceDownload}`);

      if (!env.DB) {
        const response = isJSON 
          ? JSON.stringify({ error: '数据库不可用' })
          : '数据库不可用';
        return new Response(response, {
          status: 503,
          headers: {
            'Content-Type': isJSON ? 'application/json' : 'text/plain',
            ...corsHeaders
          }
        });
      }

      // 从 D1 获取最新消息
      let query = 'SELECT * FROM messages WHERE 1=1';
      const params = [];
      
      if (room !== 'default') {
        query += ' AND room = ?';
        params.push(room);
      }
      
      query += ' ORDER BY timestamp DESC LIMIT 1';

      console.log(`最新内容查询: ${query}, 参数:`, params);

      const result = await env.DB.prepare(query).bind(...params).first();
      
      console.log(`最新内容查询结果:`, result);
      
      if (!result) {
        const response = isJSON 
          ? JSON.stringify({ error: '内容未找到' })
          : '没有可用的内容';
        return new Response(response, {
          status: 404,
          headers: {
            'Content-Type': isJSON ? 'application/json' : 'text/plain',
            ...corsHeaders
          }
        });
      }

      // 处理文本内容
      if (result.type === 'text') {
        if (isJSON) {
          return new Response(JSON.stringify({
            type: 'text',
            content: result.content,
            id: result.id.toString(),
            timestamp: result.timestamp,
            room: result.room || 'default'
          }), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          const content = result.content + (result.content.endsWith('\n') ? '' : '\n');
          return new Response(content, {
            headers: { 
              'Content-Type': 'text/plain; charset=utf-8',
              'X-Content-ID': result.id.toString(),
              'X-Content-Type': 'text',
              'X-Content-Room': result.room || 'default',
              ...corsHeaders 
            }
          });
        }
      } 
      // 处理文件内容
      else if (result.type === 'file') {
        if (isJSON) {
          return new Response(JSON.stringify({
            type: ContentHandler.determineFileType(result.name),
            name: result.name,
            size: result.size,
            uuid: result.uuid,
            url: result.url,
            id: result.id.toString(),
            timestamp: result.timestamp,
            room: result.room || 'default'
          }), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          console.log(`获取最新文件: ${result.uuid}`);
          
          if (!env.R2_BUCKET) {
            return new Response('Storage not available', { 
              status: 503,
              headers: corsHeaders 
            });
          }

          const object = await env.R2_BUCKET.get(`files/${result.uuid}`);
          
          if (!object) {
            console.log(`文件未找到: ${result.uuid}`);
            return new Response('File not found', { 
              status: 404,
              headers: corsHeaders 
            });
          }

          // 检查文件是否过期
          const expireTime = parseInt(object.customMetadata?.expireTime || '0');
          const currentTime = Math.floor(Date.now() / 1000);
          if (expireTime > 0 && currentTime > expireTime) {
            console.log(`文件已过期: ${result.uuid}`);
            return new Response('File expired', { 
              status: 404,
              headers: corsHeaders 
            });
          }

          console.log(`返回最新文件内容: ${result.uuid}, size: ${object.size}`);

          // 获取正确的 MIME 类型
          const contentType = object.httpMetadata?.contentType || 'application/octet-stream';
          const fileType = ContentHandler.determineFileType(result.name);
          
          // 构建响应头
          const headers = {
            'Content-Type': contentType,
            'Content-Length': object.size.toString(),
            'Last-Modified': new Date(parseInt(object.customMetadata?.uploadTime || Date.now())).toUTCString(),
            'X-Content-ID': result.id.toString(),
            'X-Content-Type': 'file',
            'X-Content-Room': result.room || 'default',
            'X-File-UUID': result.uuid,
            'X-File-Type': fileType,
            ...corsHeaders
          };

          // 根据文件类型和请求决定 Content-Disposition
          if (forceDownload) {
            headers['Content-Disposition'] = `attachment; filename="${encodeURIComponent(result.name)}"`;
          } else {
            // 对于可以在浏览器中显示的文件类型，使用 inline
            if (fileType === 'image' || fileType === 'video' || fileType === 'audio' || 
                contentType.startsWith('text/') || contentType === 'application/pdf') {
              headers['Content-Disposition'] = `inline; filename="${encodeURIComponent(result.name)}"`;
              headers['Cache-Control'] = 'public, max-age=300'; // 5分钟缓存
            } else {
              // 其他文件类型强制下载，避免浏览器尝试渲染
              headers['Content-Disposition'] = `attachment; filename="${encodeURIComponent(result.name)}"`;
            }
          }

          return new Response(object.body, { headers });
        }
      }

    } catch (error) {
      console.error('Latest content handler error:', error);
      console.error('Error stack:', error.stack);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '获取最新内容时发生错误',
        details: error.message
      }), { 
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }

  static async getById(request, env) {
    try {
      const { id } = request.params;
      const url = new URL(request.url);
      const room = url.searchParams.get('room');
      const isJSON = url.pathname.endsWith('.json') || url.searchParams.get('json') === 'true';

      console.log(`获取内容: ID ${id}, room: ${room}, isJSON: ${isJSON}`);

      if (!env.DB) {
        const response = isJSON 
          ? JSON.stringify({ error: '数据库不可用' })
          : '数据库不可用';
        return new Response(response, {
          status: 503,
          headers: {
            'Content-Type': isJSON ? 'application/json' : 'text/plain',
            ...corsHeaders
          }
        });
      }

      // 从 D1 获取消息
      let query = 'SELECT * FROM messages WHERE id = ?';
      const params = [parseInt(id)];
      
      if (room) {
        query += ' AND room = ?';
        params.push(room);
      }

      console.log(`查询 SQL: ${query}, 参数:`, params);

      const result = await env.DB.prepare(query).bind(...params).first();
      
      console.log(`查询结果:`, result);
      
      if (!result) {
        const response = isJSON 
          ? JSON.stringify({ error: '内容未找到' })
          : '内容未找到';
        return new Response(response, {
          status: 404,
          headers: {
            'Content-Type': isJSON ? 'application/json' : 'text/plain',
            ...corsHeaders
          }
        });
      }

      if (result.type === 'text') {
        if (isJSON) {
          return new Response(JSON.stringify({
            type: 'text',
            content: result.content,
            id: result.id.toString(),
            timestamp: result.timestamp
          }), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          const content = result.content + (result.content.endsWith('\n') ? '' : '\n');
          return new Response(content, {
            headers: { 'Content-Type': 'text/plain; charset=utf-8', ...corsHeaders }
          });
        }
      } else if (result.type === 'file') {
        if (isJSON) {
          return new Response(JSON.stringify({
            type: ContentHandler.determineFileType(result.name),
            name: result.name,
            size: result.size,
            uuid: result.uuid,
            url: result.url,
            id: result.id.toString(),
            timestamp: result.timestamp
          }), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          // 对于按 ID 查询的文件，保持重定向行为
          console.log(`重定向到文件: ${result.url}`);
          return Response.redirect(result.url, 302);
        }
      }

    } catch (error) {
      console.error('Content handler error:', error);
      console.error('Error stack:', error.stack);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '获取内容时发生错误',
        details: error.message
      }), { 
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }

  static async revoke(request, env) {
    try {
      // 认证检查
      if (env.AUTH_PASSWORD) {
        const auth = request.headers.get('Authorization') || 
                    new URL(request.url).searchParams.get('auth');
        if (!auth || auth.replace('Bearer ', '') !== env.AUTH_PASSWORD) {
          return new Response(JSON.stringify({
            error: 'Unauthorized',
            message: '需要认证令牌'
          }), {
            status: 401,
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        }
      }

      const { id } = request.params;
      const url = new URL(request.url);
      const room = url.searchParams.get('room') || 'default';

      console.log(`删除消息请求: ID ${id}, room: ${room}`);

      if (!env.DB) {
        return new Response(JSON.stringify({
          error: 'Database not available',
          message: '数据库不可用'
        }), {
          status: 503,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 查找要删除的消息
      let query = 'SELECT * FROM messages WHERE id = ?';
      const params = [parseInt(id)];
      
      if (room !== 'default') {
        query += ' AND room = ?';
        params.push(room);
      }

      const message = await env.DB.prepare(query).bind(...params).first();

      if (!message) {
        return new Response(JSON.stringify({
          error: 'Message not found',
          message: '消息未找到'
        }), {
          status: 404,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 如果是文件消息，删除文件
      if (message.type === 'file' && message.uuid && env.R2_BUCKET) {
        try {
          await env.R2_BUCKET.delete(`files/${message.uuid}`);
          console.log(`已删除文件: ${message.uuid}`);
        } catch (error) {
          console.error(`删除文件失败: ${message.uuid}`, error);
        }
      }

      // 从数据库删除消息
      await env.DB.prepare('DELETE FROM messages WHERE id = ?').bind(parseInt(id)).run();

      // 广播撤销消息
      await broadcastMessage(env, room, {
        event: 'revoke',
        data: { id: parseInt(id) }
      });

      return new Response(JSON.stringify({
        status: '消息删除成功',
        id: parseInt(id)
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });

    } catch (error) {
      console.error('Revoke handler error:', error);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '删除消息时发生错误'
      }), {
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    }
  }

  static async revokeAll(request, env) {
    try {
      // 认证检查
      if (env.AUTH_PASSWORD) {
        const auth = request.headers.get('Authorization') || 
                    new URL(request.url).searchParams.get('auth');
        if (!auth || auth.replace('Bearer ', '') !== env.AUTH_PASSWORD) {
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

      console.log(`清空所有消息请求: room: ${room}`);

      if (!env.DB) {
        return new Response(JSON.stringify({
          error: 'Database not available',
          message: '数据库不可用'
        }), {
          status: 503,
          headers: { 'Content-Type': 'application/json', ...corsHeaders }
        });
      }

      // 获取要删除的文件UUID列表
      let fileQuery = 'SELECT uuid FROM messages WHERE type = ? AND uuid IS NOT NULL';
      const fileParams = ['file'];
      
      if (room !== 'default') {
        fileQuery += ' AND room = ?';
        fileParams.push(room);
      }

      const fileResults = await env.DB.prepare(fileQuery).bind(...fileParams).all();

      // 删除文件
      if (env.R2_BUCKET && fileResults.results) {
        for (const fileRecord of fileResults.results) {
          if (fileRecord.uuid) {
            try {
              await env.R2_BUCKET.delete(`files/${fileRecord.uuid}`);
              console.log(`已删除文件: ${fileRecord.uuid}`);
            } catch (error) {
              console.error(`删除文件失败: ${fileRecord.uuid}`, error);
            }
          }
        }
      }

      // 删除消息记录
      let deleteQuery = 'DELETE FROM messages WHERE 1=1';
      const deleteParams = [];
      
      if (room !== 'default') {
        deleteQuery += ' AND room = ?';
        deleteParams.push(room);
      }

      await env.DB.prepare(deleteQuery).bind(...deleteParams).run();

      // 广播清空消息
      await broadcastMessage(env, room, {
        event: 'clearAll',
        data: { room }
      });

      return new Response(JSON.stringify({
        status: '所有消息已清除'
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });

    } catch (error) {
      console.error('Revoke all handler error:', error);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '清空消息时发生错误'
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
}