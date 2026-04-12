import { normalizeRoomName, resolveRoomAuth } from '../auth';
import { buildSenderDevice, parseUserAgent } from '../utils';

function isRoomListEnabled(env) {
  return ['1', 'true', 'yes', 'on'].includes(String(env.ROOM_LIST || '').toLowerCase());
}

export class WebSocketRoom {
  constructor(state, env) {
    this.state = state;
    this.env = env;
    this.sessions = new Map();
    console.log('WebSocketRoom 实例创建');
  }

  async fetch(request) {
    const url = new URL(request.url);
    
    console.log(`WebSocketRoom fetch: ${url.pathname}`);
    
    // 处理广播消息的内部请求
    if (url.pathname === '/broadcast') {
      try {
        const message = await request.json();
        this.broadcast(message);
        return new Response('OK');
      } catch (error) {
        console.error('广播消息处理错误:', error);
        return new Response('Broadcast Error', { status: 500 });
      }
    }

    if (url.pathname === '/stats') {
      return Response.json(this.getRoomStats());
    }

    // 处理 WebSocket 升级请求
    const upgradeHeader = request.headers.get('Upgrade');
    if (upgradeHeader && upgradeHeader.toLowerCase() === 'websocket') {
      return this.handleWebSocket(request);
    }

    return new Response('Expected WebSocket', { status: 400 });
  }

  async handleWebSocket(request) {
    try {
      console.log('开始处理 WebSocket 升级');
      
      // 创建 WebSocket 对
      const webSocketPair = new WebSocketPair();
      const [client, server] = Object.values(webSocketPair);
      
      const url = new URL(request.url);
      const sessionId = this.generateSessionId();
      const userAgent = request.headers.get('User-Agent') || '';
      const ip = request.headers.get('CF-Connecting-IP') || 'unknown';
      const room = normalizeRoomName(url.searchParams.get('room'));
      
      console.log(`创建 WebSocket 会话: ${sessionId}, room: ${room}, ip: ${ip}`);
      
      const session = {
        webSocket: server,
        sessionId,
        userAgent,
        ip,
        room: room,
        connectedAt: Date.now()
      };

      this.sessions.set(sessionId, session);
      await this.persistSessionPresence(session);

      // 接受 WebSocket 连接
      server.accept();
      
      console.log(`WebSocket 连接已建立: ${sessionId}`);
      
      // 设置事件监听器
      server.addEventListener('message', (event) => {
        this.handleMessage(sessionId, event);
      });

      server.addEventListener('close', (event) => {
        this.handleClose(sessionId, event);
      });

      server.addEventListener('error', (event) => {
        this.handleError(sessionId, event);
      });

      // 与 Go 后端保持一致：先发送历史，再发送配置，再同步设备。
      await this.sendHistoryMessages(server, room);
      await this.sendConfigMessage(server, room);
      await this.sendExistingDevices(server, room, sessionId);

      // 广播新设备连接
      this.broadcastDeviceConnect(sessionId, userAgent, room);

      console.log(`WebSocket 会话 ${sessionId} 初始化完成`);

      // 返回 WebSocket 响应
      return new Response(null, {
        status: 101,
        webSocket: client,
      });

    } catch (error) {
      console.error('WebSocket 升级失败:', error);
      return new Response('WebSocket Upgrade Failed', { 
        status: 500 
      });
    }
  }

  async sendConfigMessage(webSocket, room) {
    try {
      const fileLimit = parseInt(this.env.FILE_LIMIT) || 104857600;
      const configMessage = {
        event: 'config',
        data: {
          version: 'cloudflare-worker-v1.0.0',
          server: {
            history: parseInt(this.env.HISTORY_LIMIT) || 10,
            prefix: '',
            roomList: isRoomListEnabled(this.env)
          },
          text: {
            limit: parseInt(this.env.TEXT_LIMIT) || 4096
          },
          file: {
            expire: parseInt(this.env.FILE_EXPIRE) || 3600,
            chunk: fileLimit + 1,
            limit: fileLimit
          },
          auth: resolveRoomAuth(this.env, room).required
        }
      };
      
      if (webSocket.readyState === WebSocket.OPEN) {
        webSocket.send(JSON.stringify(configMessage));
        console.log(`配置消息已发送`);
      }
      
    } catch (error) {
      console.error('发送配置消息失败:', error);
    }
  }

  async sendHistoryMessages(webSocket, room) {
    try {
      console.log(`获取房间 ${room} 的历史消息`);
      
      if (!this.env.DB) {
        console.log('DB binding 不可用，跳过历史消息');
        return;
      }

      if (webSocket.readyState !== WebSocket.OPEN) {
        console.log('WebSocket 未就绪，跳过历史消息');
        return;
      }

      // 获取历史消息限制，默认为 10
      const historyLimit = parseInt(this.env.HISTORY_LIMIT || '10');
      console.log(`历史消息限制: ${historyLimit}`);

      const query = `
        SELECT * FROM (
          SELECT * FROM messages
          WHERE room = ?
          ORDER BY timestamp DESC, id DESC
          LIMIT ?
        ) recent
        ORDER BY timestamp ASC, id ASC
      `;
      const params = [normalizeRoomName(room), historyLimit];
      
      console.log(`历史消息查询: ${query}, 参数:`, params, `限制: ${historyLimit}`);
      
      const results = await this.env.DB.prepare(query).bind(...params).all();
      
      if (!results.results || results.results.length === 0) {
        console.log(`房间 ${room} 没有历史消息`);
        return;
      }

      console.log(`找到 ${results.results.length} 条历史消息 (限制: ${historyLimit})`);

      // 发送历史消息
      for (const row of results.results) {
        if (webSocket.readyState !== WebSocket.OPEN) {
          console.log('WebSocket 已关闭，停止发送历史消息');
          break;
        }

        const historyMessage = {
          event: 'receive',
          data: {
            id: row.id,
            type: row.type,
            timestamp: row.timestamp,
            room: row.room || 'default',
            senderIP: row.senderIP || 'unknown',
            senderDevice: buildSenderDevice(row.userAgent || 'unknown')
          }
        };

        // 根据消息类型添加相应字段
        if (row.type === 'text') {
          historyMessage.data.content = row.content;
        } else if (row.type === 'file') {
          // 为历史文件消息添加图标
          const FileHandler = await import('../handlers/file.js');
          const fileIcon = FileHandler.FileHandler.getFileTypeIcon(row.name);
          const displayName = `${fileIcon} ${row.name}`;
          
          historyMessage.data.name = displayName;
          historyMessage.data.size = row.size;
          historyMessage.data.uuid = row.uuid;
          historyMessage.data.url = row.url;
          
          // 处理过期时间
          let expireTime = row.expireTime;
          if (expireTime && expireTime.toString().length === 10) {
            expireTime = expireTime * 1000;
          }
          historyMessage.data.expire = expireTime;
          historyMessage.data.cache = row.uuid;
        }
        
        console.log(`发送历史消息: ID ${row.id}, 类型 ${row.type}`);
        
        webSocket.send(JSON.stringify(historyMessage));
        
      }
      
      console.log(`历史消息发送完成，共发送 ${results.results.length} 条 (限制: ${historyLimit})`);
      
    } catch (error) {
      console.error('发送历史消息失败:', error);
      console.error('Error details:', error.stack);
    }
  }

  async sendExistingDevices(webSocket, room, excludeSessionId) {
    try {
      // 发送房间内现有设备信息
      const existingDevices = [];
      for (const [sessionId, session] of this.sessions) {
        if (sessionId !== excludeSessionId && session.room === room) {
          const deviceInfo = parseUserAgent(session.userAgent);
          existingDevices.push({
            id: sessionId,
            type: deviceInfo.type,
            device: deviceInfo.device,
            os: deviceInfo.os,
            browser: deviceInfo.browser
          });
        }
      }

      for (const deviceMeta of existingDevices) {
        if (webSocket.readyState === WebSocket.OPEN) {
          webSocket.send(JSON.stringify({
            event: 'connect',
            data: deviceMeta
          }));
        }
      }

      console.log(`发送了 ${existingDevices.length} 个现有设备信息`);

    } catch (error) {
      console.error('发送现有设备信息失败:', error);
    }
  }

  handleMessage(sessionId, event) {
    try {
      if (event.data && event.data.trim()) {
        console.log(`WebSocket 消息 from ${sessionId}:`, event.data);
      }
    } catch (error) {
      console.error(`处理消息错误 (${sessionId}):`, error);
    }
  }

  handleClose(sessionId, event) {
    console.log(`WebSocket 会话关闭: ${sessionId}`);
    
    const session = this.sessions.get(sessionId);
    if (session) {
      this.sessions.delete(sessionId);
      void this.removeSessionPresence(sessionId);
      
      // 广播设备断开连接
      this.broadcast({
        event: 'disconnect',
        data: { id: sessionId }
      }, session.room);
    }
  }

  handleError(sessionId, event) {
    console.error(`WebSocket 错误 (${sessionId}):`, event);
    this.sessions.delete(sessionId);
    void this.removeSessionPresence(sessionId);
  }

  broadcastDeviceConnect(sessionId, userAgent, room) {
    try {
      const deviceInfo = parseUserAgent(userAgent);
      
      const connectMessage = {
        event: 'connect',
        data: {
          id: sessionId,
          type: deviceInfo.type,
          device: deviceInfo.device,
          os: deviceInfo.os,
          browser: deviceInfo.browser
        }
      };
      
      this.broadcast(connectMessage, room, sessionId);
      console.log(`设备连接广播: ${sessionId}`);
      
    } catch (error) {
      console.error('广播设备连接失败:', error);
    }
  }

  broadcast(message, room = null, excludeSessionId = null) {
    if (!message || typeof message !== 'object') {
      console.error('无效的广播消息:', message);
      return;
    }

    const messageString = JSON.stringify(message);
    const disconnectedSessions = [];
    const targetRoom = room ? normalizeRoomName(room) : null;
    
    console.log(`广播消息给 ${this.sessions.size} 个会话: ${message.event}`);
    
    for (const [sessionId, session] of this.sessions) {
      try {
        if (excludeSessionId && sessionId === excludeSessionId) {
          continue;
        }
        if (targetRoom && normalizeRoomName(session.room) !== targetRoom) {
          continue;
        }
        if (session.webSocket.readyState === WebSocket.OPEN) {
          session.webSocket.send(messageString);
        } else {
          console.log(`会话 ${sessionId} 已断开，标记清理`);
          disconnectedSessions.push(sessionId);
        }
      } catch (error) {
        console.error(`广播到会话 ${sessionId} 失败:`, error);
        disconnectedSessions.push(sessionId);
      }
    }
    
    // 清理断开的连接
    for (const sessionId of disconnectedSessions) {
      this.sessions.delete(sessionId);
      void this.removeSessionPresence(sessionId);
    }
    
    console.log(`广播完成，清理了 ${disconnectedSessions.length} 个断开的会话`);
  }

  generateSessionId() {
    return Math.random().toString(36).substr(2, 9);
  }

  async persistSessionPresence(session) {
    if (!this.env.DB) {
      return;
    }

    try {
      const connectedAt = Math.floor(session.connectedAt / 1000);
      await this.env.DB.prepare(`
        INSERT OR REPLACE INTO room_presence (sessionId, room, connectedAt, userAgent, updatedAt)
        VALUES (?, ?, ?, ?, ?)
      `).bind(
        session.sessionId,
        normalizeRoomName(session.room),
        connectedAt,
        session.userAgent || '',
        connectedAt,
      ).run();
    } catch (error) {
      console.error(`持久化房间在线状态失败 (${session.sessionId}):`, error);
    }
  }

  async removeSessionPresence(sessionId) {
    if (!this.env.DB) {
      return;
    }

    try {
      await this.env.DB.prepare('DELETE FROM room_presence WHERE sessionId = ?').bind(sessionId).run();
    } catch (error) {
      console.error(`删除房间在线状态失败 (${sessionId}):`, error);
    }
  }

  getRoomStats() {
    let latestConnectedAt = 0;

    for (const session of this.sessions.values()) {
      if (session.connectedAt > latestConnectedAt) {
        latestConnectedAt = session.connectedAt;
      }
    }

    return {
      deviceCount: this.sessions.size,
      isActive: this.sessions.size > 0,
      lastActive: latestConnectedAt ? Math.floor(latestConnectedAt / 1000) : 0,
    };
  }
}