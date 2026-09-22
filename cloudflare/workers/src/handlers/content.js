import { corsHeaders } from '../cors';
import { broadcastMessage, buildSenderDevice, ensureBoardColumn } from '../utils';
import { ensureRoomAccess, normalizeRoomName } from '../auth';
import { ensureRoomOrShareAccess } from '../share';
import { errorResponse } from '../errors';

// 看板的列：**固定三列**（todo / doing / done）。空串/缺失归一成 todo（新条目默认落待办）。
// 返回 null 表示不认识这个值 —— 调用方必须报错，不能静默回落，否则客户端以为挪成功了。
// 与 Go 侧 normalizeBoardColumn 同一份契约，改一边记得改另一边。
function normalizeBoardColumn(raw) {
  const value = String(raw ?? '').trim().toLowerCase();
  if (value === '') {
    return 'todo';
  }
  return ['todo', 'doing', 'done'].includes(value) ? value : null;
}

function normalizeExpire(expireTime) {
  const numericExpire = Number(expireTime || 0);
  if (!numericExpire) {
    return 0;
  }

  return String(numericExpire).length === 10 ? numericExpire : Math.floor(numericExpire / 1000);
}

// 读**显式**的格式信号：?format= > .json 后缀 > ?json=1。
//
// 与 Go 侧的 resolveContentFormat 是同一份契约 —— 改一边必须改另一边。
// 已发布的 Android 捷径走的是 .json 后缀，一个字都不能改。
//
// 返回 '' 表示调用方没显式要格式，由分支自己决定（文本分支会再看 Accept 头，
// 文件分支不看 —— 见 wantsJSON）。返回 null 表示 format 给了不认识的值
// （比如 ?format=html）：调用方必须报错、不能回落，否则客户端以为拿到 HTML、
// 实际拿到原文。
function resolveContentFormat(request, url) {
  const explicit = String(url.searchParams.get('format') || '').trim().toLowerCase();
  if (explicit) {
    if (explicit === 'json') {
      return 'json';
    }
    if (explicit === 'raw' || explicit === 'text' || explicit === 'plain') {
      return 'raw';
    }
    return null;
  }

  if (url.pathname.endsWith('.json')) {
    return 'json';
  }

  const jsonParam = String(url.searchParams.get('json') || '').toLowerCase();
  if (jsonParam === 'true' || jsonParam === '1') {
    return 'json';
  }

  return '';
}

// 决定**文本**响应给不给 JSON：显式格式优先，没显式时才看 Accept 头。
//
// 文件分支不走这里。下载链路上的 Accept 头太不可靠（浏览器、下载器、脚本五花八门），
// 所以文件分支历来只认显式信号 —— 别为了「统一」合并掉：合并的后果是
// 「浏览器直接点开文件链接」会突然收到一坨 JSON。
function wantsJSON(explicitFormat, request) {
  if (explicitFormat === 'json') {
    return true;
  }
  if (explicitFormat === 'raw') {
    return false;
  }
  return request.headers.get('Accept')?.includes('application/json') === true;
}

function buildJsonContentPayload(row) {
  const base = {
    id: row.id.toString(),
    timestamp: row.timestamp,
    room: row.room || 'default',
    senderIP: row.senderIP || 'unknown',
    senderClientID: row.senderClientID || '',
    senderDevice: buildSenderDevice(row.userAgent || 'unknown', row.deviceName),
    // 看板的列，空串 = 待办。数据库列叫 boardColumn（`column` 是 SQL 关键字），
    // 对外一律叫 `column` —— 和 Go 侧、和 WebSocket 载荷保持一致。
    column: row.boardColumn || '',
  };

  if (row.type === 'text') {
    return {
      ...base,
      type: 'text',
      content: row.content,
    };
  }

  return {
    ...base,
    type: ContentHandler.determineFileType(row.name),
    name: row.name,
    size: row.size,
    uuid: row.uuid,
    url: row.url,
    cache: row.uuid,
    expire: normalizeExpire(row.expireTime),
  };
}

export class ContentHandler {
  static async buildFileResponse(env, result, { forceDownload = false } = {}) {
    if (!env.R2_BUCKET) {
      return new Response('Storage not available', {
        status: 503,
        headers: corsHeaders,
      });
    }

    const object = await env.R2_BUCKET.get(`files/${result.uuid}`);
    if (!object) {
      return new Response('File not found', {
        status: 404,
        headers: corsHeaders,
      });
    }

    const expireTime = parseInt(object.customMetadata?.expireTime || '0', 10);
    const currentTime = Math.floor(Date.now() / 1000);
    if (expireTime > 0 && currentTime > expireTime) {
      return new Response('File expired', {
        status: 404,
        headers: corsHeaders,
      });
    }

    const contentType = object.httpMetadata?.contentType || 'application/octet-stream';
    const fileType = ContentHandler.determineFileType(result.name);
    const headers = {
      'Content-Type': contentType,
      'Content-Length': object.size.toString(),
      'Last-Modified': new Date(parseInt(object.customMetadata?.uploadTime || Date.now(), 10)).toUTCString(),
      'X-Content-ID': result.id.toString(),
      'X-Content-Type': 'file',
      'X-Content-Room': result.room || 'default',
      'X-File-UUID': result.uuid,
      'X-File-Type': fileType,
      ...corsHeaders,
    };

    if (forceDownload) {
      headers['Content-Disposition'] = `attachment; filename="${encodeURIComponent(result.name)}"`;
    } else if (
      fileType === 'image' ||
      fileType === 'video' ||
      fileType === 'audio' ||
      contentType.startsWith('text/') ||
      contentType === 'application/pdf'
    ) {
      headers['Content-Disposition'] = `inline; filename="${encodeURIComponent(result.name)}"`;
      headers['Cache-Control'] = 'public, max-age=300';
    } else {
      headers['Content-Disposition'] = `attachment; filename="${encodeURIComponent(result.name)}"`;
    }

    return new Response(object.body, { headers });
  }

  static async getLatest(request, env) {
    try {
      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      const explicitFormat = resolveContentFormat(request, url);
      if (explicitFormat === null) {
        return errorResponse(400, 'unsupported_format', 'Unsupported format', '不支持的格式（只支持 raw / json）');
      }
      // 文件分支只看显式信号；文本分支还会看 Accept（见下方 wantsJSON）
      const isJSON = explicitFormat === 'json';
      const forceDownload = url.searchParams.get('download') === 'true';
      const authResult = await ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }

      console.log(`获取最新内容: room=${room}, isJSON=${isJSON}, forceDownload=${forceDownload}`);

      if (!env.DB) {
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库不可用');
      }

      // 从 D1 获取最新消息
      //
      // 二级排序键 id DESC 不是装饰：timestamp 是**秒级**，同一秒里发两条（先文本后文件）
      // 时 timestamp 完全相等，SQLite 在并列中返回哪一条是未定义的 —— 实测会返回**先插入**的
      // 那条，于是「接收最新」拿到的是上一条消息。加上 id DESC 后与 Go 侧一致
      // （Go 是倒着遍历队列，天然取最后插入的）。回归测试见 test/receive-path.test.mjs 的 H 段。
      let query = 'SELECT * FROM messages WHERE room = ?';
      const params = [room];
      query += ' ORDER BY timestamp DESC, id DESC LIMIT 1';

      console.log(`最新内容查询: ${query}, 参数:`, params);

      const result = await env.DB.prepare(query).bind(...params).first();
      
      console.log(`最新内容查询结果:`, result);
      
      if (!result) {
        return errorResponse(404, 'no_content', 'No content available', '没有可用的内容');
      }

      // 处理文本内容
      // 文件过期检查必须放在返回记录之前：否则客户端会拿着一条已过期的记录去下载，
      // 拿到 R2 缺失/404 的错误文本，然后存成一个顶着原文件名的假文件。
      // （Apple 快捷指令的「获取URL内容」不暴露 HTTP 状态码，只能靠响应体里的 error 字段。）
      const expireAt = result.type === 'file' ? normalizeExpire(result.expireTime) : 0;
      if (expireAt > 0 && expireAt < Math.floor(Date.now() / 1000)) {
        return errorResponse(404, 'file_expired', 'File expired', '文件已过期');
      }

      if (result.type === 'text') {
        if (wantsJSON(explicitFormat, request)) {
          return new Response(JSON.stringify(buildJsonContentPayload(result)), {
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
          return new Response(JSON.stringify(buildJsonContentPayload(result)), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          return await ContentHandler.buildFileResponse(env, result, { forceDownload });
        }
      }

    } catch (error) {
      console.error('Latest content handler error:', error);
      console.error('Error stack:', error.stack);
      return errorResponse(500, 'internal_error', 'Internal Server Error', '获取最新内容时发生错误');
    }
  }

  // POST /content/:id/column —— 看板把卡片挪到另一列。
  //
  // 与 Go 侧 handleContentColumn 同一份契约：固定三列、卡片就是条目本身、不建新表，
  // 且**不动 timestamp**（挪个位置不该让卡片在时间流里跳到最前面）。
  //
  // ⚠️ 鉴权用 ensureRoomAccess（**房间密码**），不用 getById 那条 ensureRoomOrShareAccess ——
  // 分享 token 是给「只读一条」用的，不能拿来改东西。
  static async setColumn(request, env) {
    try {
      const { id } = request.params;
      const url = new URL(request.url);
      const numericId = parseInt(id, 10);
      if (!Number.isInteger(numericId) || numericId <= 0) {
        return errorResponse(400, 'invalid_content_id', 'Invalid content id', '无效的内容 ID');
      }

      let body;
      try {
        body = await request.json();
      } catch {
        return errorResponse(400, 'invalid_body', 'Invalid JSON body', '请求体不是合法的 JSON');
      }
      const column = normalizeBoardColumn(body && body.column);
      if (column === null) {
        return errorResponse(400, 'invalid_column', 'Unknown board column', '未知的看板列（只支持 todo / doing / done）');
      }

      if (!env.DB) {
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库不可用');
      }

      const hasRequestedRoom = url.searchParams.has('room');
      const requestedRoom = normalizeRoomName(url.searchParams.get('room'));
      // 先按条目**自己记录的房间**取，再鉴权 —— 和 getById 同一条思路：不信客户端传的 ?room=
      const row = hasRequestedRoom
        ? await env.DB.prepare('SELECT * FROM messages WHERE id = ? AND room = ?').bind(numericId, requestedRoom).first()
        : await env.DB.prepare('SELECT * FROM messages WHERE id = ? ORDER BY id DESC LIMIT 1').bind(numericId).first();

      if (!row) {
        return errorResponse(404, 'content_not_found', 'Content not found', '内容未找到');
      }

      const contentRoom = normalizeRoomName(row.room || 'default');
      const authResult = await ensureRoomAccess(request, env, contentRoom);
      if (!authResult.ok) {
        return authResult.response;
      }

      if (!await ensureBoardColumn(env.DB)) {
        return errorResponse(503, 'database_unavailable', 'Board column unavailable', '看板列不可用');
      }

      await env.DB.prepare('UPDATE messages SET boardColumn = ? WHERE id = ? AND room = ?')
        .bind(column, numericId, contentRoom).run();

      // 广播载荷和 getById 的形状一致：客户端 `case 'update'` 是原地合并，所以给整条。
      await broadcastMessage(env, contentRoom, {
        event: 'update',
        data: buildJsonContentPayload({ ...row, boardColumn: column }),
      });

      return new Response(JSON.stringify({
        id: numericId.toString(),
        type: row.type,
        column,
      }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders }
      });
    } catch (error) {
      console.error('Board column handler error:', error);
      return errorResponse(500, 'internal_error', 'Internal Server Error', '设置看板列时发生错误');
    }
  }

  static async getById(request, env) {
    try {
      const { id } = request.params;
      const url = new URL(request.url);
      const hasRequestedRoom = url.searchParams.has('room');
      const room = normalizeRoomName(url.searchParams.get('room'));
      const explicitFormat = resolveContentFormat(request, url);
      if (explicitFormat === null) {
        return errorResponse(400, 'unsupported_format', 'Unsupported format', '不支持的格式（只支持 raw / json）');
      }
      // 文件分支只看显式信号；文本分支还会看 Accept（见下方 wantsJSON）
      const isJSON = explicitFormat === 'json';
      const forceDownload = url.searchParams.get('download') === 'true';

      console.log(`获取内容: ID ${id}, room: ${room}, isJSON: ${isJSON}`);

      if (!env.DB) {
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库不可用');
      }

      // 先取内容，再按内容所在房间做「密码 OR 分享 token」鉴权
      let result;
      if (hasRequestedRoom) {
        result = await env.DB.prepare('SELECT * FROM messages WHERE id = ? AND room = ?')
          .bind(parseInt(id, 10), room)
          .first();
      } else {
        result = await env.DB.prepare('SELECT * FROM messages WHERE id = ? ORDER BY id DESC LIMIT 1')
          .bind(parseInt(id, 10))
          .first();
      }

      console.log(`查询结果:`, result);
      
      if (!result) {
        return errorResponse(404, 'content_not_found', 'Content not found', '内容未找到');
      }

      const contentRoom = normalizeRoomName(result.room || 'default');
      const authResult = await ensureRoomOrShareAccess(request, env, contentRoom, {
        shareType: 'content',
        shareId: String(result.id),
      });
      if (!authResult.ok) {
        return authResult.response;
      }

      // 文件过期检查必须放在返回记录之前：否则客户端会拿着一条已过期的记录去下载，
      // 拿到 R2 缺失/404 的错误文本，然后存成一个顶着原文件名的假文件。
      // （Apple 快捷指令的「获取URL内容」不暴露 HTTP 状态码，只能靠响应体里的 error 字段。）
      const expireAt = result.type === 'file' ? normalizeExpire(result.expireTime) : 0;
      if (expireAt > 0 && expireAt < Math.floor(Date.now() / 1000)) {
        return errorResponse(404, 'file_expired', 'File expired', '文件已过期');
      }

      if (result.type === 'text') {
        if (wantsJSON(explicitFormat, request)) {
          return new Response(JSON.stringify(buildJsonContentPayload(result)), {
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
          return new Response(JSON.stringify(buildJsonContentPayload(result)), {
            headers: { 'Content-Type': 'application/json', ...corsHeaders }
          });
        } else {
          return await ContentHandler.buildFileResponse(env, result, { forceDownload });
        }
      }

    } catch (error) {
      console.error('Content handler error:', error);
      console.error('Error stack:', error.stack);
      return errorResponse(500, 'internal_error', 'Internal Server Error', '获取内容时发生错误');
    }
  }

  static async revoke(request, env) {
    try {
      const { id } = request.params;
      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      const authResult = await ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }

      console.log(`删除消息请求: ID ${id}, room: ${room}`);

      if (!env.DB) {
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库不可用');
      }

      // 查找要删除的消息
      const query = 'SELECT * FROM messages WHERE id = ? AND room = ?';
      const params = [parseInt(id), room];

      const message = await env.DB.prepare(query).bind(...params).first();

      if (!message) {
        return errorResponse(404, 'message_not_found', 'Message not found', '消息未找到');
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
      return errorResponse(500, 'internal_error', 'Internal Server Error', '删除消息时发生错误');
    }
  }

  static async revokeAll(request, env) {
    try {
      const url = new URL(request.url);
      const room = normalizeRoomName(url.searchParams.get('room'));
      const authResult = await ensureRoomAccess(request, env, room);
      if (!authResult.ok) {
        return authResult.response;
      }

      console.log(`清空所有消息请求: room: ${room}`);

      if (!env.DB) {
        return errorResponse(503, 'database_unavailable', 'Database not available', '数据库不可用');
      }

      // 获取要删除的文件UUID列表
      let fileQuery = 'SELECT uuid FROM messages WHERE type = ? AND uuid IS NOT NULL AND room = ?';
      const fileParams = ['file', room];

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
      const deleteQuery = 'DELETE FROM messages WHERE room = ?';
      const deleteParams = [room];

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
      return errorResponse(500, 'internal_error', 'Internal Server Error', '清空消息时发生错误');
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