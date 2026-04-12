import { corsHeaders } from '../cors';
import { canAccessRoom, extractAuthTokens, hasRoomAuthEntry, normalizeRoomName, parseRoomAuth } from '../auth';

function toDisplayRoom(room) {
  return normalizeRoomName(room) === 'default' ? '' : normalizeRoomName(room);
}

function compareRooms(left, right) {
  if (Boolean(left.isActive) !== Boolean(right.isActive)) {
    return left.isActive ? -1 : 1;
  }
  return (right.lastActive || 0) - (left.lastActive || 0);
}

export class RoomsHandler {
  static async list(request, env) {
    try {
      const enabled = String(env.ROOM_LIST || '').toLowerCase();
      if (!['1', 'true', 'yes', 'on'].includes(enabled)) {
        return new Response(JSON.stringify({ rooms: [] }), {
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      if (!env.DB) {
        return new Response(JSON.stringify({
          error: 'Database not available',
          message: '数据库服务不可用',
        }), {
          status: 503,
          headers: { 'Content-Type': 'application/json', ...corsHeaders },
        });
      }

      const tokens = extractAuthTokens(request);
      const roomAuth = parseRoomAuth(env);
      const messageResult = await env.DB.prepare(`
        SELECT room, COUNT(*) AS messageCount, MAX(timestamp) AS lastActive
        FROM messages
        GROUP BY room
      `).all();
      const presenceResult = await env.DB.prepare(`
        SELECT room, COUNT(*) AS deviceCount, MAX(connectedAt) AS lastActive
        FROM room_presence
        GROUP BY room
      `).all();

      const roomMap = new Map();
      for (const row of messageResult.results || []) {
        const normalizedRoom = normalizeRoomName(row.room);
        roomMap.set(normalizedRoom, {
          room: normalizedRoom,
          messageCount: Number(row.messageCount || 0),
          messageLastActive: Number(row.lastActive || 0),
          deviceCount: 0,
          presenceLastActive: 0,
        });
      }

      for (const row of presenceResult.results || []) {
        const normalizedRoom = normalizeRoomName(row.room);
        const existing = roomMap.get(normalizedRoom) || {
          room: normalizedRoom,
          messageCount: 0,
          messageLastActive: 0,
          deviceCount: 0,
          presenceLastActive: 0,
        };
        existing.deviceCount = Number(row.deviceCount || 0);
        existing.presenceLastActive = Number(row.lastActive || 0);
        roomMap.set(normalizedRoom, existing);
      }

      for (const room of Object.keys(roomAuth)) {
        const normalizedRoom = normalizeRoomName(room);
        if (!roomMap.has(normalizedRoom)) {
          roomMap.set(normalizedRoom, {
            room: normalizedRoom,
            messageCount: 0,
            messageLastActive: 0,
            deviceCount: 0,
            presenceLastActive: 0,
          });
        }
      }

      const roomEntries = await Promise.all(Array.from(roomMap.values())
        .map(async row => {
          const normalizedRoom = row.room;
          if (!canAccessRoom(env, normalizedRoom, '') && !tokens.some(token => canAccessRoom(env, normalizedRoom, token))) {
            return null;
          }
          const messageLastActive = Number(row.messageLastActive || 0);
          const presenceLastActive = Number(row.presenceLastActive || 0);
          const deviceCount = Number(row.deviceCount || 0);

          return {
            name: toDisplayRoom(normalizedRoom),
            messageCount: Number(row.messageCount || 0),
            deviceCount,
            lastActive: Math.max(messageLastActive, presenceLastActive),
            isActive: deviceCount > 0,
            isProtected: hasRoomAuthEntry(env, normalizedRoom),
          };
        }));

      const rooms = roomEntries
        .filter(Boolean)
        .sort(compareRooms);

      return new Response(JSON.stringify({ rooms }), {
        headers: { 'Content-Type': 'application/json', ...corsHeaders },
      });
    } catch (error) {
      console.error('Rooms handler error:', error);
      return new Response(JSON.stringify({
        error: 'Internal Server Error',
        message: '获取房间列表时发生错误',
      }), {
        status: 500,
        headers: { 'Content-Type': 'application/json', ...corsHeaders },
      });
    }
  }
}