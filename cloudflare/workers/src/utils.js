export function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c == 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}

function extractVersion(uaString, pattern) {
  const match = uaString.match(pattern);
  return match ? match[1] : '';
}

export function parseUserAgent(uaString = '') {
  const ua = String(uaString || '');

  const isTablet = /iPad|Tablet|PlayBook|Silk|Kindle/i.test(ua);
  const isMobile = !isTablet && /Mobile|iPhone|Android/i.test(ua);
  const isBot = /bot|crawler|spider|curl|wget/i.test(ua);

  let type = 'Desktop';
  if (isBot) type = 'Bot';
  else if (isTablet) type = 'Tablet';
  else if (isMobile) type = 'Mobile';

  let os = 'Unknown';
  if (/Windows NT 10\.0/i.test(ua)) os = 'Windows 10';
  else if (/Windows NT 6\.3/i.test(ua)) os = 'Windows 8.1';
  else if (/Windows NT 6\.2/i.test(ua)) os = 'Windows 8';
  else if (/Windows NT 6\.1/i.test(ua)) os = 'Windows 7';
  else if (/Android\s([\d.]+)/i.test(ua)) os = `Android ${extractVersion(ua, /Android\s([\d.]+)/i)}`.trim();
  else if (/iPhone OS\s([\d_]+)/i.test(ua)) os = `iOS ${extractVersion(ua, /iPhone OS\s([\d_]+)/i).replace(/_/g, '.')}`.trim();
  else if (/iPad; CPU OS\s([\d_]+)/i.test(ua)) os = `iPadOS ${extractVersion(ua, /iPad; CPU OS\s([\d_]+)/i).replace(/_/g, '.')}`.trim();
  else if (/Mac OS X\s([\d_]+)/i.test(ua)) os = `macOS ${extractVersion(ua, /Mac OS X\s([\d_]+)/i).replace(/_/g, '.')}`.trim();
  else if (/Linux/i.test(ua)) os = 'Linux';

  let browser = 'Unknown';
  if (/Edg\/(\d+)/i.test(ua)) browser = `Edge ${extractVersion(ua, /Edg\/(\d+)/i)}`.trim();
  else if (/Chrome\/(\d+)/i.test(ua)) browser = `Chrome ${extractVersion(ua, /Chrome\/(\d+)/i)}`.trim();
  else if (/Firefox\/(\d+)/i.test(ua)) browser = `Firefox ${extractVersion(ua, /Firefox\/(\d+)/i)}`.trim();
  else if (/Version\/(\d+).+Safari/i.test(ua)) browser = `Safari ${extractVersion(ua, /Version\/(\d+)/i)}`.trim();
  else if (/Safari/i.test(ua)) browser = 'Safari';

  let device = type;
  if (/iPhone/i.test(ua)) device = 'iPhone';
  else if (/iPad/i.test(ua)) device = 'iPad';
  else if (/Android/i.test(ua)) device = isTablet ? 'Android Tablet' : 'Android Phone';
  else if (/Macintosh|Mac OS X/i.test(ua)) device = 'Mac';
  else if (/Windows/i.test(ua)) device = 'PC';
  else if (/Linux/i.test(ua)) device = 'Linux PC';

  return { type, device, os, browser };
}

export function buildSenderDevice(uaString = '') {
  const deviceInfo = parseUserAgent(uaString);
  return {
    type: deviceInfo.type,
    os: deviceInfo.os,
    browser: deviceInfo.browser,
  };
}

function normalizeRoomName(room = '') {
  const normalized = String(room || '').trim();
  return normalized === '' || normalized === 'default' ? 'default' : normalized;
}

export async function saveToD1(db, messageData, env) { // 修复：添加 env 参数
  try {
    if (!db) {
      console.log('D1 database not available, skipping save');
      return { messageId: Math.floor(Math.random() * 1000000), filesToCleanup: [] };
    }

    // 先检查并清理超出限制的消息（在保存新消息之前）
    const room = normalizeRoomName(messageData.room);
    const filesToCleanup = await cleanupOldMessagesBeforeSave(db, room, env); // 修复：传递 env

    // 保存新消息
    const result = await db.prepare(`
      INSERT INTO messages (type, content, name, size, room, timestamp, senderIP, userAgent, uuid, expireTime, url)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).bind(
      messageData.type,
      messageData.content || null,
      messageData.name || null,
      messageData.size || null,
      room,
      messageData.timestamp,
      messageData.senderIP || 'unknown',
      messageData.userAgent || 'unknown',
      messageData.uuid || null,
      messageData.expireTime || null,
      messageData.url || null
    ).run();

    // 返回新消息ID和需要删除的文件列表
    return {
      messageId: result.meta.last_row_id,
      filesToCleanup: filesToCleanup
    };

  } catch (error) {
    console.error('D1 save error:', error);
    return {
      messageId: Math.floor(Math.random() * 1000000),
      filesToCleanup: []
    };
  }
}

// 在保存新消息前清理旧消息
async function cleanupOldMessagesBeforeSave(db, room = 'default', env) { // 修复：添加 env 参数
  try {
    if (!db) return [];

    const normalizedRoom = normalizeRoomName(room);

    // 修复：从 env 对象获取历史限制，默认 50 条
    const historyLimit = env.HISTORY_LIMIT ? parseInt(env.HISTORY_LIMIT) : 50;
    
    // 查询房间内的消息数量
    const countQuery = 'SELECT COUNT(*) as count FROM messages WHERE room = ?';
    const countParams = [normalizedRoom];

    const countResult = await db.prepare(countQuery).bind(...countParams).first();
    const messageCount = countResult.count;

    console.log(`房间 ${normalizedRoom} 当前消息数量: ${messageCount}, 限制: ${historyLimit}`);

    // 如果加上新消息会超过限制，需要删除旧消息
    if (messageCount >= historyLimit) {
      const excessCount = messageCount - historyLimit + 1; // +1 因为要保存新消息
      console.log(`需要删除 ${excessCount} 条旧消息为新消息腾出空间`);

      // 获取要删除的最旧消息（按时间戳排序，不是按ID）
      let selectQuery = 'SELECT id, type, uuid, timestamp FROM messages WHERE room = ?';
      const selectParams = [normalizedRoom];
      
      // 按时间戳升序排列，选择最旧的消息
      selectQuery += ` ORDER BY timestamp ASC, id ASC LIMIT ${excessCount}`;

      const oldMessages = await db.prepare(selectQuery).bind(...selectParams).all();
      
      if (oldMessages.results && oldMessages.results.length > 0) {
        console.log(`找到 ${oldMessages.results.length} 条旧消息需要删除`);

        // 收集要删除的文件 UUID 和消息 ID
        const fileUuidsToDelete = [];
        const messageIdsToDelete = [];

        for (const msg of oldMessages.results) {
          messageIdsToDelete.push(msg.id);
          if (msg.type === 'file' && msg.uuid) {
            fileUuidsToDelete.push(msg.uuid);
          }
          console.log(`将删除消息: ID=${msg.id}, 类型=${msg.type}, 时间戳=${msg.timestamp}`);
        }

        // 删除数据库记录
        if (messageIdsToDelete.length > 0) {
          const placeholders = messageIdsToDelete.map(() => '?').join(',');
          await db.prepare(`DELETE FROM messages WHERE id IN (${placeholders})`)
            .bind(...messageIdsToDelete).run();
          console.log(`从数据库删除了 ${messageIdsToDelete.length} 条消息记录`);
        }

        // 返回需要删除的文件 UUID 列表
        return fileUuidsToDelete;
      }
    }

    return [];
  } catch (error) {
    console.error('清理旧消息时出错:', error);
    return [];
  }
}

// 保留原有的清理函数，用于维护 API
export async function cleanupOldMessages(db, room = 'default', env) { // 修复：添加 env 参数
  return cleanupOldMessagesBeforeSave(db, room, env);
}

export async function broadcastMessage(env, room, message) {
  try {
    if (!env.WEBSOCKET_ROOM) {
      console.log('WEBSOCKET_ROOM binding not available, skipping broadcast');
      return;
    }

    const durableObjectId = env.WEBSOCKET_ROOM.idFromName(normalizeRoomName(room));
    const durableObject = env.WEBSOCKET_ROOM.get(durableObjectId);
    
    const broadcastRequest = new Request('https://internal/broadcast', {
      method: 'POST',
      body: JSON.stringify(message),
      headers: { 'Content-Type': 'application/json' }
    });

    await durableObject.fetch(broadcastRequest);
    console.log(`Broadcast message to room: ${normalizeRoomName(room)}`);
  } catch (error) {
    console.error('Broadcast error:', error);
  }
}