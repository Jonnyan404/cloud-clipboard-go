import { defineStore } from 'pinia';
import axios from 'axios';
import router from '@/router';
import { useAppStore } from './app';

const ROOM_AUTH_CACHE_KEY = 'roomAuthCache';
const DEFAULT_ROOM_KEY = '__default__';
const GLOBAL_ROOM_KEY = '__global__';

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

        /* ---------- 连接 ---------- */
        async connect() {
            const app = useAppStore();
            if (this.websocketConnecting) {
                return;
            }
            this.websocketConnecting = true;
            try {
                const currentRoom = this.normalizeRoomName(this.room);
                const serverInfo = await this.fetchServerInfo(currentRoom);
                let resolvedToken = '';
                if (serverInfo.auth) {
                    resolvedToken = await this.resolveAuthTokenForRoom(currentRoom, { interactive: true });
                    if (resolvedToken === null) {
                        this.websocketConnecting = false;
                        return;
                    }
                }
                const ws = await new Promise((resolve, reject) => {
                    const wsUrl = new URL(serverInfo.server);
                    wsUrl.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
                    wsUrl.port = location.port;
                    wsUrl.searchParams.set('room', currentRoom);
                    const protocols = resolvedToken ? [resolvedToken] : [];
                    const socket = new WebSocket(wsUrl, protocols);
                    socket.onopen = () => resolve(socket);
                    socket.onerror = reject;
                });

                this.websocket = ws;
                this.websocketConnecting = false;
                this.retry = 0;
                app.received = [];
                this.authCode = this.getAuthTokenForRoom(currentRoom);
                if (this.heartbeatTimer) {
                    clearInterval(this.heartbeatTimer);
                }
                const heartbeat = () => {
                    if (this.websocket && this.websocket.readyState === WebSocket.OPEN) {
                        this.websocket.send('');
                    }
                };
                this.heartbeatTimer = setInterval(heartbeat, 30000);
                ws.onclose = () => {
                    if (this.heartbeatTimer) {
                        clearInterval(this.heartbeatTimer);
                        this.heartbeatTimer = null;
                    }
                    this.websocket = null;
                    this.websocketConnecting = false;
                    app.device = [];
                    if (this.retry < 3) {
                        this.retry++;
                        setTimeout(() => this.connect(), 3000);
                    } else if (this.getAuthTokenForRoom(this.room)) {
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
        handleEvent(event, data) {
            const app = useAppStore();
            switch (event) {
                case 'receive':
                    app.received.unshift(data);
                    break;
                case 'receiveMulti':
                    app.received.unshift(...Array.from(data).reverse());
                    break;
                case 'revoke': {
                    const index = app.received.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        app.received.splice(index, 1);
                    }
                    break;
                }
                case 'config': {
                    app.config = data;
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
                    const index = app.received.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        app.received.splice(index, 1, { ...app.received[index], ...data });
                    }
                    break;
                }
                case 'forbidden': {
                    this.clearAuthTokenForRoom(this.room);
                    this.openAuthDialog(this.room);
                    break;
                }
            }
        },
        disconnect() {
            const app = useAppStore();
            this.websocketConnecting = false;
            if (this.websocket) {
                this.websocket.onclose = () => {};
                this.websocket.close();
                this.websocket = null;
            }
            app.device = [];
        },
        failure() {
            const app = useAppStore();
            this.websocket = null;
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
            const token = await this.resolveAuthTokenForRoom(normalizedRoom, { interactive: true });
            if (token === null) {
                return false;
            }
            const targetQuery = normalizedRoom ? { room: normalizedRoom } : {};
            const currentQuery = router.currentRoute.value.query;
            if (normalizedRoom === this.normalizeRoomName(currentQuery.room || 'default')) {
                return true;
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
