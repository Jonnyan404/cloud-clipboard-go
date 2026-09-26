import { defineStore } from 'pinia';
import axios from 'axios';
import router from '@/router';
import { APP_BASE_URL } from '@/base.js';
import { useAppStore } from './app';

const ROOM_AUTH_CACHE_KEY = 'roomAuthCache';
const DEFAULT_ROOM_KEY = '__default__';
const GLOBAL_ROOM_KEY = '__global__';

// 「这个后端会不会 `GET /content`」—— 也就是它是不是**新后端**（认 `?history=0` 那一版）。
//
// 为什么要**跨刷新记住**：`?history=0` 是**连接时**的查询参数，而探测得先连一次 WS
// 才能从握手的 `config` 里读到 `latestId`（见 `applyContentApiSupport`）。
// 不记住的话，每次刷新都会先被白推一份历史 —— `history=0` 就白加了。
//
// ⚠️ 按 `APP_BASE_URL` 分开存：同一个浏览器可能连过**不同的部署**（自建的、别人的），
// 而它们的能力不一样。键里带上 base，两边就不会互相污染。
function contentApiCacheKey() {
    return `contentApiSupport:${APP_BASE_URL}`;
}

function loadContentApiSupport() {
    try {
        return localStorage.getItem(contentApiCacheKey()) === '1';
    } catch {
        return false;
    }
}

function loadRoomAuthCache() {
    try {
        const raw = sessionStorage.getItem(ROOM_AUTH_CACHE_KEY);
        if (!raw) {
            return {};
        }
        const parsed = JSON.parse(raw);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch {
        return {};
    }
}

export const useWebSocketStore = defineStore('websocket', {
    state: () => ({
        websocket: null,
        websocketConnecting: false,
        authCode: '',
        inputPassword: '',
        authCodeDialog: false,
        authPendingRoom: '',
        authCodeError: '',
        authDialogLoading: false,
        roomAuthCache: loadRoomAuthCache(),
        roomProtectionCache: {},
        authRefreshTimer: null,
        room: '',
        roomInput: '',
        roomDialog: false,
        retry: 0,
        heartbeatTimer: null,
        pingTimer: null,
        latency: null,
        pendingReceiveQueue: [],
        receiveFlushTimer: null,
        // 这个后端会不会 `GET /content`（= 是不是新后端）。见 `applyContentApiSupport`。
        contentApiSupported: loadContentApiSupport(),
    }),

    getters: {
        // 用于请求鉴权：与旧 $root.getRequestAuthToken 等价
        currentRoom() {
            return this.normalizeRoomName(this.room);
        },
    },

    actions: {
        initFromRoute(roomQuery) {
            this.room = this.normalizeRoomName(roomQuery || '');
        },

        /* ---------- 房间/鉴权工具 ---------- */
        normalizeRoomName(room = '') {
            const normalized = (room || '').trim();
            return normalized === 'default' ? '' : normalized;
        },
        getRoomStorageKey(room = this.room) {
            return this.normalizeRoomName(room) || DEFAULT_ROOM_KEY;
        },
        persistRoomAuthCache() {
            sessionStorage.setItem(ROOM_AUTH_CACHE_KEY, JSON.stringify(this.roomAuthCache));
        },
        getGlobalAuthToken() {
            const entry = this.roomAuthCache[GLOBAL_ROOM_KEY];
            if (typeof entry === 'string') {
                return entry;
            }
            if (entry && typeof entry === 'object' && typeof entry.token === 'string') {
                return entry.token;
            }
            return '';
        },
        getEffectiveAuthEntry(room = this.room) {
            const now = Math.floor(Date.now() / 1000);
            const read = key => {
                const entry = this.roomAuthCache[key];
                if (typeof entry === 'string' && entry) {
                    return { token: entry, expiresAt: 0, key };
                }
                if (entry && typeof entry === 'object' && typeof entry.token === 'string' && entry.token) {
                    const expiresAt = Number(entry.expiresAt) || 0;
                    if (expiresAt > 0 && expiresAt <= now) {
                        return null;
                    }
                    return { token: entry.token, expiresAt, key };
                }
                return null;
            };
            return read(this.getRoomStorageKey(room)) || read(GLOBAL_ROOM_KEY);
        },
        getAuthTokenForRoom(room = this.room) {
            const effective = this.getEffectiveAuthEntry(room);
            return effective ? effective.token : '';
        },
        cacheAuthTokenForRoom(room, token, expiresAt = 0) {
            const normalizedToken = (token || '').trim();
            const key = this.getRoomStorageKey(room);
            if (!normalizedToken) {
                this.clearAuthTokenForRoom(room);
                return;
            }
            const existing = this.roomAuthCache[key];
            const effectiveExpiresAt = Number(expiresAt) > 0
                ? Number(expiresAt)
                : (existing && typeof existing === 'object' && Number(existing.expiresAt) > 0 ? Number(existing.expiresAt) : 0);
            this.roomAuthCache[key] = { token: normalizedToken, expiresAt: effectiveExpiresAt };
            this.persistRoomAuthCache();
            if (this.normalizeRoomName(room) === this.currentRoom) {
                this.authCode = normalizedToken;
            }
            this.scheduleAuthRefresh(room);
        },
        clearAuthTokenForRoom(room = this.room) {
            const key = this.getRoomStorageKey(room);
            if (Object.prototype.hasOwnProperty.call(this.roomAuthCache, key)) {
                delete this.roomAuthCache[key];
                this.persistRoomAuthCache();
            }
            if (this.normalizeRoomName(room) === this.currentRoom) {
                this.authCode = '';
                this.clearAuthRefreshTimer();
            }
        },
        getKnownAuthTokens(room = this.room) {
            const tokens = [];
            const push = token => {
                let value = token;
                if (token && typeof token === 'object' && typeof token.token === 'string') {
                    value = token.token;
                }
                const normalized = String(value || '').trim();
                if (normalized && !tokens.includes(normalized)) {
                    tokens.push(normalized);
                }
            };
            push(this.getAuthTokenForRoom(room));
            push(this.authCode);
            Object.values(this.roomAuthCache).forEach(push);
            return tokens;
        },
        clearAuthRefreshTimer() {
            if (this.authRefreshTimer) {
                clearTimeout(this.authRefreshTimer);
                this.authRefreshTimer = null;
            }
        },
        setRoomProtection(room, isProtected) {
            this.roomProtectionCache[this.normalizeRoomName(room)] = Boolean(isProtected);
        },
        async fetchServerInfo(room = this.room, { token = '' } = {}) {
            const response = await axios.get('server', {
                params: new URLSearchParams([['room', this.normalizeRoomName(room)]]),
                headers: token ? { Authorization: `Bearer ${token}` } : undefined,
                __skipRoomAuthHandling: true,
            });
            if (Object.prototype.hasOwnProperty.call(response.data || {}, 'roomProtected')) {
                this.setRoomProtection(room, response.data.roomProtected);
            }
            return response.data;
        },
        async verifyRoomAccess(room, token) {
            if (!token) {
                return false;
            }
            const serverInfo = await this.fetchServerInfo(room, { token });
            return serverInfo.auth ? serverInfo.authorized === true : true;
        },
        openAuthDialog(room, initialToken = '') {
            this.authPendingRoom = this.normalizeRoomName(room);
            this.roomDialog = false;
            this.inputPassword = '';
            this.authCodeError = '';
            this.authDialogLoading = false;
            this.authCodeDialog = true;
        },
        async resolveAuthTokenForRoom(room, { interactive = true } = {}) {
            const normalizedRoom = this.normalizeRoomName(room);
            const cachedToken = this.getAuthTokenForRoom(normalizedRoom);
            if (cachedToken) {
                return cachedToken;
            }
            const serverInfo = await this.fetchServerInfo(normalizedRoom);
            if (!serverInfo.auth) {
                return '';
            }
            const candidates = this.getKnownAuthTokens(normalizedRoom);
            for (const token of candidates) {
                if (await this.verifyRoomAccess(normalizedRoom, token)) {
                    return token;
                }
            }
            if (interactive) {
                this.openAuthDialog(normalizedRoom);
            }
            return null;
        },
        async obtainRoomSessionToken(room, password) {
            try {
                const response = await axios.post('auth/token', { password }, {
                    params: new URLSearchParams([['room', this.normalizeRoomName(room)]]),
                    __skipRoomAuthHandling: true,
                });
                const data = response.data || {};
                return {
                    token: data.token || null,
                    expiresAt: Number(data.expiresAt) || 0,
                    scope: data.scope === 'global' ? 'global' : '',
                };
            } catch (error) {
                console.error('Failed to obtain session token:', error);
                return null;
            }
        },
        async refreshRoomSessionToken(room) {
            const normalizedRoom = this.normalizeRoomName(room);
            const currentToken = this.getAuthTokenForRoom(normalizedRoom);
            if (!currentToken) {
                return null;
            }
            try {
                const response = await axios.post('auth/token/refresh', null, {
                    params: new URLSearchParams([['room', normalizedRoom]]),
                    __skipRoomAuthHandling: true,
                });
                const data = response.data || {};
                return {
                    token: data.token || null,
                    expiresAt: Number(data.expiresAt) || 0,
                    scope: data.scope === 'global' ? 'global' : '',
                };
            } catch (error) {
                console.error('Failed to refresh session token:', error);
                return null;
            }
        },
        scheduleAuthRefresh(room = this.room) {
            this.clearAuthRefreshTimer();
            const normalizedRoom = this.normalizeRoomName(room);
            if (normalizedRoom !== this.currentRoom) {
                return;
            }
            const effective = this.getEffectiveAuthEntry(normalizedRoom);
            const token = effective ? effective.token : '';
            const expiresAt = effective ? effective.expiresAt : 0;
            if (!token || !expiresAt) {
                return;
            }
            const remainingSeconds = expiresAt - Math.floor(Date.now() / 1000);
            if (remainingSeconds <= 60) {
                return;
            }
            const delay = Math.max(0, Math.min((remainingSeconds - 60) * 1000, 24 * 60 * 60 * 1000));
            this.authRefreshTimer = setTimeout(async () => {
                this.authRefreshTimer = null;
                const refreshed = await this.refreshRoomSessionToken(normalizedRoom);
                if (refreshed && refreshed.token) {
                    const cacheRoom = refreshed.scope === 'global' ? GLOBAL_ROOM_KEY : effective.key;
                    this.cacheAuthTokenForRoom(cacheRoom, refreshed.token, refreshed.expiresAt);
                    this.scheduleAuthRefresh(normalizedRoom);
                } else {
                    this.clearAuthTokenForRoom(normalizedRoom);
                    if (normalizedRoom === this.currentRoom) {
                        this.openAuthDialog(normalizedRoom);
                    }
                }
            }, delay);
        },

        getWebSocketEndpoint(room = this.room) {
            const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
            // ⚠️ 必须用 APP_BASE_URL（从 `document.baseURI` 推导），**不能**用
            // `app.config.server.prefix`。
            //
            // 后者只能从 WebSocket 握手时那条 `config` 事件拿到（见本文件 `case 'config'`），
            // 而这里正是**为了连上 WebSocket** 才拼地址 —— 首次连接时它还是空串，
            // 于是拼出 `/push` 而不是 `/clip/push`，服务端直接 404。
            // 这是个鸡生蛋：要连上才知道 prefix，要知道 prefix 才能连上。
            //
            // APP_BASE_URL 没有这个问题：它从文档目录推导，服务端在深路径上会注入
            // `<base href="<prefix>/">`（见 lib/spa_shell.go），页面加载时就确定了。
            // （Issue #23；#7「prefix 不更新到链接」是同一根因的另一面。）
            const wsUrl = new URL('push', APP_BASE_URL);
            wsUrl.protocol = protocol;
            const normalizedRoom = this.normalizeRoomName(room);
            if (normalizedRoom) {
                wsUrl.searchParams.set('room', normalizedRoom);
            }
            // 已知这个后端会 `GET /content` → 让 WS **只推实时**，历史改走那条 HTTP 路径。
            //
            // ⚠️ 老后端（用户自己部署的 Worker 不会自动更新）收到这个**不认识的参数会忽略它**、
            // 仍然推历史 —— 而那时握手里也不会有 `latestId`，于是我们退回老路、
            // 也**不会**去请求 `/content`（否则拿到的是 SPA 兜底的 HTML）。见 `applyContentApiSupport`。
            if (this.contentApiSupported) {
                wsUrl.searchParams.set('history', '0');
            }
            return wsUrl.toString();
        },

        /* ---------- 连接 ---------- */
        async connect() {
            const app = useAppStore();
            if (this.websocketConnecting) {
                return;
            }
            this.websocketConnecting = true;
            try {
                const currentRoom = this.normalizeRoomName(this.room);
                let resolvedToken = this.getAuthTokenForRoom(currentRoom);

                // 无论是否已缓存 token，都先探测 /server 以可靠获知房间是否需要认证。
                // 若仅在 app.config?.auth 为真时才探测，首次加载（config 需在认证后才会收到）
                // 受保护房间时会永远探测不到，导致既不连接也不弹认证窗口。
                const serverInfo = await this.fetchServerInfo(currentRoom);
                if (!resolvedToken && serverInfo.auth) {
                    resolvedToken = await this.resolveAuthTokenForRoom(currentRoom, { interactive: true });
                    if (resolvedToken === null) {
                        this.websocketConnecting = false;
                        return;
                    }
                }

                const wsUrl = this.getWebSocketEndpoint(currentRoom);
                const protocols = resolvedToken ? [resolvedToken] : [];

                const ws = await new Promise((resolve, reject) => {
                    const socket = new WebSocket(wsUrl, protocols);
                    socket.onopen = () => resolve(socket);
                    socket.onerror = reject;
                });

                this.websocket = ws;
                this.websocketConnecting = false;
                this.retry = 0;
                this.authCode = resolvedToken || this.getAuthTokenForRoom(currentRoom);
                if (this.heartbeatTimer) {
                    clearInterval(this.heartbeatTimer);
                }
                const heartbeat = () => {
                    if (this.websocket && this.websocket.readyState === WebSocket.OPEN) {
                        this.websocket.send('');
                    }
                };
                this.heartbeatTimer = setInterval(heartbeat, 30000);
                const ping = () => {
                    if (this.websocket && this.websocket.readyState === WebSocket.OPEN) {
                        try {
                            this.websocket.send(JSON.stringify({ event: 'ping', data: Date.now() }));
                        } catch {}
                    }
                };
                ping();
                this.pingTimer = setInterval(ping, 3000);
                ws.onclose = async () => {
                    if (this.heartbeatTimer) {
                        clearInterval(this.heartbeatTimer);
                        this.heartbeatTimer = null;
                    }
                    if (this.pingTimer) {
                        clearInterval(this.pingTimer);
                        this.pingTimer = null;
                    }
                    this.latency = null;
                    this.websocket = null;
                    this.websocketConnecting = false;
                    app.device = [];
                    if (this.retry < 3) {
                        this.retry++;
                        setTimeout(() => this.connect(), 3000);
                        return;
                    }
                    // 重试耗尽后，若服务器仍要求认证（可能因 token 失效/过期），
                    // 清除本地 token 并弹出认证窗口，否则静默失败用户无法感知。
                    try {
                        const info = await this.fetchServerInfo(this.room);
                        if (info.auth) {
                            this.clearAuthTokenForRoom(this.room);
                            this.openAuthDialog(this.room);
                        }
                    } catch {
                        this.openAuthDialog(this.room);
                    }
                };
                ws.onmessage = e => {
                    try {
                        const parsed = JSON.parse(e.data);
                        this.handleEvent(parsed.event, parsed.data);
                    } catch {}
                };
            } catch (error) {
                this.websocketConnecting = false;
                this.failure();
            }
        },
        syncRoomView(targetRoom) {
            const app = useAppStore();
            const normalizedRoom = this.normalizeRoomName(targetRoom);
            const cached = app.roomMessagesCache[normalizedRoom];
            if (cached && Array.isArray(cached)) {
                app.received = [...cached];
            } else {
                app.received = [];
            }
        },
        saveRoomCache(room = this.room) {
            const app = useAppStore();
            const normalizedRoom = this.normalizeRoomName(room);
            app.roomMessagesCache[normalizedRoom] = [...app.received];
        },
        flushPendingReceives() {
            if (this.receiveFlushTimer) {
                clearTimeout(this.receiveFlushTimer);
                this.receiveFlushTimer = null;
            }
            if (!this.pendingReceiveQueue.length) {
                return;
            }
            const app = useAppStore();
            const newItems = this.pendingReceiveQueue.splice(0);
            this.mergeMessages(newItems);
        },
        mergeMessages(incomingItems) {
            const app = useAppStore();
            if (!incomingItems || !incomingItems.length) {
                return;
            }
            const currentList = [...app.received];
            const existingIdMap = new Map();
            currentList.forEach((item, index) => {
                existingIdMap.set(item.id, index);
            });

            for (const item of incomingItems) {
                if (existingIdMap.has(item.id)) {
                    const idx = existingIdMap.get(item.id);
                    currentList[idx] = { ...currentList[idx], ...item };
                } else {
                    currentList.push(item);
                }
            }

            // 按时间倒序排列 (最新的排在最前)
            currentList.sort((a, b) => (Number(b.timestamp) || 0) - (Number(a.timestamp) || 0));

            // 如果有配置历史条数限制，进行截断
            const limit = Number(app.config?.server?.history || 0);
            if (limit > 0 && currentList.length > limit) {
                currentList.splice(limit);
            }

            app.received = currentList;
            this.saveRoomCache();
        },
        queueReceive(data) {
            this.pendingReceiveQueue.unshift(data);
            if (!this.receiveFlushTimer) {
                this.receiveFlushTimer = setTimeout(() => {
                    this.flushPendingReceives();
                }, 32);
            }
        },
        // 握手 `config` 到了 —— 顺便判定「这个后端会不会 `GET /content`」。
        //
        // ⚠️★ 探测信号就是 `latestId`（`docs/specs/ws-live-only.md` §0.4）：
        // 握手载荷里**有**它 = 新后端（有 `/content`、认 `?history=0`）；
        // **没有** = 老后端 → 退回「靠 WS 推历史」那条老路。
        // 它本来就是为「历史/实时的边界说不清楚」而加的，这里兼任能力标记 —— **一个字段，两个用途**。
        //
        // ⚠️ 判的是「**有没有这个字段**」，不是「值大于 0」：空房间的合法值就是 `0`，
        // 而 `0` 恰恰也说明它是新后端。
        applyContentApiSupport(config) {
            const supported = Object.prototype.hasOwnProperty.call(config || {}, 'latestId');
            if (supported !== this.contentApiSupported) {
                this.contentApiSupported = supported;
                try {
                    localStorage.setItem(contentApiCacheKey(), supported ? '1' : '0');
                } catch { /* 存不下就算了：代价只是下次刷新多一次白推 */ }
            }
            if (supported) {
                this.loadHistoryFromHttp();
            }
        },
        // 从 `GET /content` 取这个房间的历史 —— WS 只推实时之后，历史就只剩这一条路。
        //
        // ⚠️★ **必须真的去请求一次**：`?history=0` 之后握手不再推历史，
        // 不请求的话「刷新一下就什么都看不到了」—— 这是这个变更最容易漏的一条，
        // `docs/specs/ws-live-only.md` §5 第 8 条专门点了它。
        async loadHistoryFromHttp(room = this.room) {
            const normalizedRoom = this.normalizeRoomName(room);
            try {
                const response = await axios.get('content', {
                    params: new URLSearchParams([['room', normalizedRoom]]),
                });
                // 请求还在飞的时候房间被换掉了 —— 这批历史不属于当前房间，丢掉。
                if (this.normalizeRoomName(this.room) !== normalizedRoom) {
                    return;
                }
                const messages = response.data && Array.isArray(response.data.messages)
                    ? response.data.messages
                    : [];
                // ⚠️ `mergeMessages` 按 id 去重，所以「第一次连接时 WS 还推了一份历史」
                // 不会重影 —— 那是探测阶段必然会发生的一次重复，不必额外处理。
                this.mergeMessages(messages);
            } catch (error) {
                // 取历史失败**不影响实时**：WS 照样连着、新消息照收，只是这个房间暂时是空的。
                // 不做重试：下一次握手（重连 / 切房间回来）会再走一遍这里。
                console.error('Failed to load history from /content:', error);
            }
        },
        handleEvent(event, data) {
            const app = useAppStore();
            switch (event) {
                case 'receive':
                    this.queueReceive(data);
                    break;
                case 'receiveMulti':
                    this.flushPendingReceives();
                    this.mergeMessages(Array.isArray(data) ? data : [data]);
                    break;
                case 'revoke': {
                    this.flushPendingReceives();
                    const index = app.received.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        app.received.splice(index, 1);
                        this.saveRoomCache();
                    }
                    break;
                }
                case 'config': {
                    this.flushPendingReceives();
                    app.config = data;
                    // 顺手判定这个后端会不会 `GET /content`，会的话历史就改走那条路（上面那两条注释）。
                    this.applyContentApiSupport(data);
                    console.log(
                        `%c Cloud Clipboard ${data.version} by Jonnyan404 %c https://github.com/Jonnyan404/cloud-clipboard-go `,
                        'color:#fff;background-color:#1e88e5',
                        'color:#fff;background-color:#64b5f6'
                    );
                    break;
                }
                case 'connect':
                    app.device.push(data);
                    break;
                case 'disconnect': {
                    const index = app.device.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        app.device.splice(index, 1);
                    }
                    break;
                }
                case 'update': {
                    this.flushPendingReceives();
                    const index = app.received.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        app.received.splice(index, 1, { ...app.received[index], ...data });
                        this.saveRoomCache();
                    }
                    break;
                }
                case 'forbidden': {
                    this.flushPendingReceives();
                    this.clearAuthTokenForRoom(this.room);
                    this.openAuthDialog(this.room);
                    break;
                }
                case 'pong': {
                    if (typeof data === 'number' && data > 0) {
                        const rtt = Date.now() - data;
                        if (rtt >= 0) {
                            this.latency = rtt;
                        }
                    }
                    break;
                }
            }
        },
        disconnect() {
            const app = useAppStore();
            this.websocketConnecting = false;
            if (this.websocket) {
                this.websocket.onopen = null;
                this.websocket.onmessage = null;
                this.websocket.onerror = null;
                this.websocket.onclose = null;
                this.websocket.close();
                this.websocket = null;
            }
            this.clearAuthRefreshTimer();
            if (this.heartbeatTimer) {
                clearInterval(this.heartbeatTimer);
                this.heartbeatTimer = null;
            }
            if (this.pingTimer) {
                clearInterval(this.pingTimer);
                this.pingTimer = null;
            }
            if (this.receiveFlushTimer) {
                clearTimeout(this.receiveFlushTimer);
                this.receiveFlushTimer = null;
            }
            this.pendingReceiveQueue = [];
            this.saveRoomCache();
            app.device = [];
        },
        switchRoom(targetRoom) {
            const app = useAppStore();
            const oldRoom = this.normalizeRoomName(this.room);
            const newRoom = this.normalizeRoomName(targetRoom);
            if (oldRoom === newRoom) {
                return;
            }
            // 1. 先把当前视图（旧房间内容）保存回旧房间的 cache，避免混入新房间
            app.roomMessagesCache[oldRoom] = [...app.received];
            // 2. 断开旧连接并清空所有 handler，防止旧连接的残留消息写入新房间
            this.websocketConnecting = false;
            if (this.websocket) {
                this.websocket.onopen = null;
                this.websocket.onmessage = null;
                this.websocket.onerror = null;
                this.websocket.onclose = null;
                this.websocket.close();
                this.websocket = null;
            }
            this.clearAuthRefreshTimer();
            if (this.heartbeatTimer) {
                clearInterval(this.heartbeatTimer);
                this.heartbeatTimer = null;
            }
            if (this.pingTimer) {
                clearInterval(this.pingTimer);
                this.pingTimer = null;
            }
            this.latency = null;
            if (this.receiveFlushTimer) {
                clearTimeout(this.receiveFlushTimer);
                this.receiveFlushTimer = null;
            }
            this.pendingReceiveQueue = [];
            app.device = [];
            // 3. 切换当前房间
            this.room = newRoom;
            // 4. 载入新房间的 cache
            this.syncRoomView(newRoom);
            // 5. 连接新房间
            this.connect();
            // 6. 同步地址栏（使用 replace 避免历史堆栈膨胀）
            const targetQuery = newRoom ? { room: newRoom } : {};
            router.replace({ path: '/', query: targetQuery });
        },
        failure() {
            const app = useAppStore();
            this.websocket = null;
            this.latency = null;
            if (this.pingTimer) {
                clearInterval(this.pingTimer);
                this.pingTimer = null;
            }
            app.device = [];
            if (this.retry++ < 3) {
                this.connect();
            } else {
                // 连接失败提示
            }
        },
        handleHttpUnauthorized(config = {}) {
            const room = this.getRequestRoom(config);
            this.clearAuthTokenForRoom(room);
            this.openAuthDialog(room);
        },
        getRequestRoom(config = {}) {
            if (config.params instanceof URLSearchParams) {
                return this.normalizeRoomName(config.params.get('room') || this.room);
            }
            if (config.params && typeof config.params === 'object' && config.params.room !== undefined) {
                return this.normalizeRoomName(config.params.room);
            }
            return this.normalizeRoomName(this.room);
        },
        getRequestAuthToken(config = {}) {
            return this.getAuthTokenForRoom(this.getRequestRoom(config));
        },
        async navigateToRoom(room) {
            const normalizedRoom = this.normalizeRoomName(room);
            const targetQuery = normalizedRoom ? { room: normalizedRoom } : {};
            const currentQuery = router.currentRoute.value.query;
            if (normalizedRoom === this.normalizeRoomName(currentQuery.room || '')) {
                return true;
            }

            const knownToken = this.getAuthTokenForRoom(normalizedRoom);
            const isProtected = this.roomProtectionCache[normalizedRoom];
            const app = useAppStore();
            const globalAuth = Boolean(app.config?.auth);

            if (!knownToken && (isProtected === true || (isProtected === undefined && globalAuth))) {
                const token = await this.resolveAuthTokenForRoom(normalizedRoom, { interactive: true });
                if (token === null) {
                    return false;
                }
            }

            await router.push({ path: '/', query: targetQuery });
            return true;
        },
        async submitAuthCodeForPendingRoom() {
            const targetRoom = this.authPendingRoom || this.currentRoom;
            const password = (this.inputPassword || '').trim();
            if (!password || this.authDialogLoading) {
                return;
            }
            this.authDialogLoading = true;
            this.authCodeError = '';
            try {
                const verified = await this.verifyRoomAccess(targetRoom, password);
                if (!verified) {
                    this.authCodeError = 'authInvalid';
                    return;
                }
                const session = await this.obtainRoomSessionToken(targetRoom, password);
                if (!session || !session.token) {
                    this.authCodeError = 'connectionFailedRetry';
                    return;
                }
                if (session.scope === 'global') {
                    this.cacheAuthTokenForRoom(GLOBAL_ROOM_KEY, session.token, session.expiresAt);
                } else {
                    this.cacheAuthTokenForRoom(targetRoom, session.token, session.expiresAt);
                }
                this.scheduleAuthRefresh(targetRoom);
                this.inputPassword = '';
                this.authCodeDialog = false;
                this.authPendingRoom = '';
                if (this.normalizeRoomName(targetRoom) !== this.normalizeRoomName(this.room)) {
                    await this.navigateToRoom(targetRoom);
                    return;
                }
                this.retry = 0;
                this.connect();
            } catch (error) {
                console.error(error);
                this.authCodeError = 'connectionFailedRetry';
            } finally {
                this.authDialogLoading = false;
            }
        },
    },
});
