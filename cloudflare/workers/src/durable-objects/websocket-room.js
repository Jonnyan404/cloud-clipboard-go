import { normalizeRoomName, resolveRoomAuth } from '../auth';
import { historyLimit, parseUserAgent, sanitizeDeviceName } from '../utils';

function isRoomListEnabled(env) {
  return ['1', 'true', 'yes', 'on'].includes(String(env.ROOM_LIST || '').toLowerCase());
}

// 在线会话视为活跃的最大间隔（超过则视为离开），与前端 30s 心跳配合
const PRESENCE_TTL_MS = 120 * 1000;
// 清理任务的调度间隔
const PRESENCE_CLEAN_INTERVAL_MS = 60 * 1000;
// 同一会话写 D1 的最小间隔（节流心跳 30s 带来的频繁写入）
const PRESENCE_WRITE_INTERVAL_MS = 120 * 1000;

export class WebSocketRoom {
  constructor(state, env) {
    this.state = state;
    this.env = env;
    this.sessions = new Map();
    console.log('WebSocketRoom 实例创建');
    // 安排周期性的残留在线状态清理
    this.state.blockConcurrencyWhile(() => this.schedulePresenceCleanup());
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
      this.roomName = this.roomName || room;
      
      console.log(`创建 WebSocket 会话: ${sessionId}, room: ${room}, ip: ${ip}`);
      
      const session = {
        webSocket: server,
        sessionId,
        userAgent,
        ip,
        room: room,
        connectedAt: Date.now(),
        lastPresenceTouchedAt: 0
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

      // 握手载荷的发送顺序抽在 sendHandshake 里（历史不再由 WS 推，见那里的注释）。
      await this.sendHandshake(server, room, sessionId);

      // 广播新设备连接
      this.broadcastDeviceConnect(sessionId, userAgent, room);

      console.log(`WebSocket 会话 ${sessionId} 初始化完成`);

      // 回显客户端通过 Sec-WebSocket-Protocol 子协议提供的 token，避免 token 出现在 URL/访问日志中。
      // 浏览器要求服务端必须回选一个子协议，否则握手会被判定失败。
      const response = new Response(null, {
        status: 101,
        webSocket: client,
      });
      const requestedProtocols = request.headers.get('Sec-WebSocket-Protocol');
      if (requestedProtocols) {
        const subprotocol = requestedProtocols.split(',')[0].trim();
        if (subprotocol) {
          response.headers.set('Sec-WebSocket-Protocol', subprotocol);
        }
      }
      return response;

    } catch (error) {
      console.error('WebSocket 升级失败:', error);
      return new Response('WebSocket Upgrade Failed', { 
        status: 500 
      });
    }
  }

  // 握手时依次发：config → 房间里已有的设备。
  //
  // ⚠️★ **顺序是契约，不是碰巧**（`docs/specs/ws-live-only.md` §2.2）：
  // `config` 事件**必须**在实时之前发 —— 客户端在拿到 `config.latestId` 之前
  // **一条都不该写剪贴板**（它要用那个 id 对齐「HTTP 取回的历史」与「WS 推来的实时」）。
  // 别为了首屏快一点把 config 挪到后面去。
  //
  // ⚠️★ **握手不再推历史**（Jonny 2026-09-26：「没有老客户端，不用考虑老客户端」）。
  // 历史改由客户端自己去 `GET /content` 拿；原来那个 `?history=0` 开关也一并删掉
  // （默认就是不推，它与默认同义，留着只会是一处会漂的冗余）。
  //
  // ⚠️ 抽成独立方法（而不是写在 handleWebSocket 里）是为了**能测**：handleWebSocket
  // 需要真的 `WebSocketPair`，Node 里造不出来；而这一段的全部输入只是
  // 「一个能 send 的对象 + 一个房间名」。
  async sendHandshake(webSocket, room, sessionId) {
    // ⚠️ 水印要在**发 config 之前**取：它是「连接时刻」的快照，客户端拿它对齐
    // 「HTTP 取回的历史」与「WS 推来的实时」的边界。晚于 config 就已经没意义了。
    const latestId = await this.latestMessageId(room);

    await this.sendConfigMessage(webSocket, room, latestId);
    await this.sendExistingDevices(webSocket, room, sessionId);
  }

  // 连接时刻该房间的**最大消息 id**（没有消息时 `0`）—— 客户端拿它对边界。
  //
  // ⚠️★ **它现在的职责**：客户端是**自己去 `GET /content` 拿历史**的，而「HTTP 取历史」
  // 与「WS 收实时」之间有个**时序窗口** —— 请求返回之前可能已经推来几条实时消息。
  // 有了 `latestId`，客户端才能对齐那条边界：`id <= latestId` 只当历史认领（**不写剪贴板**），
  // `id > latestId` 才是实时。少了它，边界上的消息会被处理两次（或把历史灌进剪贴板）。
  // 它**兼作能力探测信号**：老后端（没有 `/content`、也就没这个字段）不下发它，
  // 新客户端据此退回「靠 WS 推历史」的老路（`docs/specs/ws-live-only.md` §0.4）。
  //
  // ⚠️ 用 `MAX(id)`，不是「取最新那条的 id」：最新那条是按 `timestamp DESC, id DESC` 挑的，
  // 而 `POST /text?id=` **原地改正文**会把 timestamp 往前刷、id 不变 —— 于是它可能给出一个
  // **偏小**的值，而偏小是**不安全**的方向（历史消息会被当成实时、写进剪贴板）。
  async latestMessageId(room) {
    if (!this.env.DB) {
      return 0;
    }
    try {
      const row = await this.env.DB.prepare(
        'SELECT MAX(id) AS latestId FROM messages WHERE room = ?'
      ).bind(normalizeRoomName(room)).first();
      return Number(row?.latestId) || 0;
    } catch (error) {
      // 取不到水印**不是**致命错误：回 `0` 等于告诉客户端「这个后端没法区分历史与实时」，
      // 而客户端对那种后端的行为是**拒绝自动写剪贴板**（fail-safe，见 spec §0.4 / §7 第 11 条）。
      console.error('取水印失败:', error);
      return 0;
    }
  }

  async sendConfigMessage(webSocket, room, latestId = 0) {
    try {
      const fileLimit = parseInt(this.env.FILE_LIMIT) || 104857600;
      const multipartPartSize = fileLimit > 5 * 1024 * 1024
        ? Math.min(fileLimit, 8 * 1024 * 1024)
        : fileLimit + 1;
      const configMessage = {
        event: 'config',
        data: {
          version: 'cloudflare-worker-v1.0.0',
          server: {
            // ⚠️ 与 WS 推历史、落库裁剪、`GET /content` 的上限是**同一个数**
            // （utils.js 的 historyLimit 一处定义）—— 别在这里手写 parseInt(env.HISTORY_LIMIT)。
            history: historyLimit(this.env),
            prefix: '',
            roomList: isRoomListEnabled(this.env)
          },
          text: {
            limit: parseInt(this.env.TEXT_LIMIT) || 4096
          },
          file: {
            expire: parseInt(this.env.FILE_EXPIRE) || 3600,
            chunk: multipartPartSize,
            limit: fileLimit
          },
          auth: resolveRoomAuth(this.env, room).required,
          // 连接时刻该房间的**最大消息 id**（没有消息时 0）—— 客户端拿它对齐
          // 「HTTP 取回的历史」与「WS 推来的实时」的边界（理由见上面 latestMessageId 那段）。
          // ⚠️★ 它**必须在 WebSocket 握手载荷里**，**不是** `/server` 的 HTTP 响应 ——
          // 前端读的 `app.config` 就是**这条 `config` 事件**（这个坑踩过两次：
          // `automation.enabled`、`prefix`）。加错地方会得到一个永远 `undefined` 的字段。
          latestId,
          // 定时自动化：Worker 侧**没有实现**这一族接口（没有 /tasks、没有 /automation，
          // 也没有进程内调度器）。显式声明 enabled:false，而不是让这个字段干脆缺失 ——
          // SPA 工具栏那个入口正是按它决定渲不渲染（PageToolbar.vue 的 automationEnabled），
          // 而前端读的 `app.config` 就是**这条 config 事件**的载荷（不是 /server 的响应）。
          // 缺失的话，前端分不清「这个后端不支持」和「字段还没送到」。
          // ⚠️ 哪天 Worker 侧真的实现了，记得连同 docs/api*.md §8 的「仅 Go 实现」一起改。
          automation: { enabled: false }
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
      const session = this.sessions.get(sessionId);
      if (session) {
        // 前端每 30s 会发送空文本帧作为心跳；任何消息都视为该会话仍活跃。
        const now = Date.now();
        session.connectedAt = now;
        // 节流：仅当距上次写 D1 超过间隔才落库，避免心跳带来的频繁写入
        if (now - session.lastPresenceTouchedAt >= PRESENCE_WRITE_INTERVAL_MS) {
          session.lastPresenceTouchedAt = now;
          void this.touchSessionPresence(sessionId, now);
        }
      }
      if (event.data && event.data.trim()) {
        console.log(`WebSocket 消息 from ${sessionId}:`, event.data);

        // Web 端延迟测量: 客户端发送 {"event":"ping","data":<clientMs>},
        // 与 Go 后端行为一致，原样回显 data，客户端用 (Date.now()-data) 计算 RTT。
        let parsed;
        try {
          parsed = JSON.parse(event.data);
        } catch {}
        if (parsed && parsed.event === 'ping' && typeof parsed.data === 'number') {
          session.webSocket.send(JSON.stringify({ event: 'pong', data: parsed.data }));
        }
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

  broadcastDeviceConnect(sessionId, userAgent, room, deviceName = '') {
    try {
      const deviceInfo = parseUserAgent(userAgent);
      
      const connectMessage = {
        event: 'connect',
        data: {
          id: sessionId,
          type: deviceInfo.type,
          name: sanitizeDeviceName(deviceName),
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

  async touchSessionPresence(sessionId, updatedAt) {
    if (!this.env.DB) {
      return;
    }
    try {
      await this.env.DB.prepare('UPDATE room_presence SET updatedAt = ? WHERE sessionId = ?')
        .bind(Math.floor(updatedAt / 1000), sessionId)
        .run();
    } catch (error) {
      console.error(`更新会话活跃时间失败 (${sessionId}):`, error);
    }
  }

  async schedulePresenceCleanup() {
    const now = Date.now();
    let existingAlarm = now;
    try {
      existingAlarm = (await this.state.storage.getAlarm()) ?? now;
    } catch (error) {
      console.error('读取已有清理任务失败，将按当前时间重新安排:', error);
    }
    await this.state.storage.setAlarm(Math.max(now, existingAlarm) + PRESENCE_CLEAN_INTERVAL_MS);
  }

  async alarm() {
    try {
      await this.cleanupStalePresence(this.roomName || '');
    } catch (error) {
      console.error('清理残留在线状态失败:', error);
    } finally {
      await this.schedulePresenceCleanup();
    }
  }

  async cleanupStalePresence(room = '') {
    if (!this.env.DB) {
      return;
    }
    const cutoff = Math.floor((Date.now() - PRESENCE_TTL_MS) / 1000);
    const statement = room
      ? this.env.DB.prepare('DELETE FROM room_presence WHERE room = ? AND updatedAt < ?')
          .bind(room, cutoff)
      : this.env.DB.prepare('DELETE FROM room_presence WHERE updatedAt < ?')
          .bind(cutoff);
    const result = await statement.run();
    const removed = result?.meta?.changes ?? 0;
    if (removed > 0) {
      console.log(`清理了 ${removed} 条过期在线状态${room ? ` (房间: ${room})` : ''}`);
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