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

export default {
    data() {
        return {
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
            room: this.$router.currentRoute.query.room || '',
            roomInput: '',
            roomDialog: false,
            retry: 0,
            date: new Date(), // 用于文件过期计算
            event: {
                receive: data => {
                    this.$root.received.unshift(data);
                },
                receiveMulti: data => {
                    this.$root.received.unshift(...Array.from(data).reverse());
                },
                revoke: data => {
                    let index = this.$root.received.findIndex(e => e.id === data.id);
                    if (index === -1) return;
                    this.$root.received.splice(index, 1);
                },
                config: data => {
                    this.$root.config = data;
                    console.log(
                        `%c Cloud Clipboard ${data.version} by Jonnyan404 %c https://github.com/Jonnyan404/cloud-clipboard-go `,
                        'color:#fff;background-color:#1e88e5',
                        'color:#fff;background-color:#64b5f6'
                    );
                },
                connect: data => {
                    this.$root.device.push(data);
                },
                disconnect: data => {
                    let index = this.$root.device.findIndex(e => e.id === data.id);
                    if (index === -1) return;
                    this.$root.device.splice(index, 1);
                },
                update: data => {
                    // 处理文本消息更新事件
                    let index = this.$root.received.findIndex(e => e.id === data.id);
                    if (index !== -1) {
                        // 更新消息内容，保留其他属性
                        this.$root.received.splice(index, 1, { ...this.$root.received[index], ...data });
                    }
                },
                forbidden: () => {
                    this.clearAuthTokenForRoom(this.room);
                    this.$toast.error(this.$t('authExpired'));
                    this.openAuthDialog(this.room);
                },
            },
        };
    },
    watch: {
        room() {
            this.clearAuthRefreshTimer();
            this.authCode = this.getAuthTokenForRoom(this.room);
            this.disconnect();
            this.connect();
        },
        '$route.fullPath'() {
            this.syncAuthFromRoute(this.$route);
        },
    },
    methods: {
        normalizeRoomName(room = '') {
            const normalized = (room || '').trim();
            return normalized === 'default' ? '' : normalized;
        },
        getRoomStorageKey(room = this.room) {
            const normalizedRoom = this.normalizeRoomName(room);
            return normalizedRoom || DEFAULT_ROOM_KEY;
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
        // 返回指定房间实际使用的缓存条目（专属优先，其次全局令牌），用于读 token / 调度续期
        getEffectiveAuthEntry(room = this.room) {
            const now = Math.floor(Date.now() / 1000);
            const read = key => {
                const entry = this.roomAuthCache[key];
                if (typeof entry === 'string' && entry) {
                    return { token: entry, expiresAt: 0, key };
                }
                if (entry && typeof entry === 'object' && typeof entry.token === 'string' && entry.token) {
                    const expiresAt = Number(entry.expiresAt) || 0;
                    // 已过期的专属令牌视为不存在，回退使用全局令牌
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

            this.$set(this.roomAuthCache, key, {
                token: normalizedToken,
                expiresAt: effectiveExpiresAt,
            });
            this.persistRoomAuthCache();

            if (this.normalizeRoomName(room) === this.normalizeRoomName(this.room)) {
                this.authCode = normalizedToken;
            }
            this.scheduleAuthRefresh(room);
        },
        clearAuthTokenForRoom(room = this.room) {
            const key = this.getRoomStorageKey(room);
            if (Object.prototype.hasOwnProperty.call(this.roomAuthCache, key)) {
                this.$delete(this.roomAuthCache, key);
                this.persistRoomAuthCache();
            }

            if (this.normalizeRoomName(room) === this.normalizeRoomName(this.room)) {
                this.authCode = '';
                this.clearAuthRefreshTimer();
            }
        },
        syncAuthFromRoute(route = this.$router.currentRoute) {
            const routeRoom = this.normalizeRoomName(route?.query?.room || '');

            // Ignore any auth value from the URL. The auth credential stays on the
            // backend and must not be cached in the browser or exposed in history.
            if (routeRoom === this.normalizeRoomName(this.room)) {
                this.authCode = this.getAuthTokenForRoom(routeRoom);
            }
        },
        getKnownAuthTokensForRoom(room = this.room) {
            const tokens = [];
            const pushToken = token => {
                const normalizedToken = (token || '').trim();
                if (normalizedToken && !tokens.includes(normalizedToken)) {
                    tokens.push(normalizedToken);
                }
            };

            // 房间专属 token 优先；无专属 token 时回退使用全局令牌
            pushToken(this.getAuthTokenForRoom(room));
            pushToken(this.getGlobalAuthToken());

            return tokens;
        },
        getKnownAuthTokens(room = this.room) {
            const tokens = [];
            const pushToken = token => {
                let value = token;
                if (token && typeof token === 'object' && typeof token.token === 'string') {
                    value = token.token;
                }
                const normalizedToken = String(value || '').trim();
                if (normalizedToken && !tokens.includes(normalizedToken)) {
                    tokens.push(normalizedToken);
                }
            };

            pushToken(this.getAuthTokenForRoom(room));
            pushToken(this.authCode);
            Object.values(this.roomAuthCache).forEach(pushToken);

            return tokens;
        },
        clearAuthRefreshTimer() {
            if (this.authRefreshTimer) {
                clearTimeout(this.authRefreshTimer);
                this.authRefreshTimer = null;
            }
        },
        scheduleAuthRefresh(room = this.room) {
            this.clearAuthRefreshTimer();
            const normalizedRoom = this.normalizeRoomName(room);
            if (normalizedRoom !== this.normalizeRoomName(this.room)) {
                return;
            }

            const effective = this.getEffectiveAuthEntry(normalizedRoom);
            const token = effective ? effective.token : '';
            const expiresAt = effective ? effective.expiresAt : 0;
            if (!token || !expiresAt) {
                return;
            }

            const remainingSeconds = expiresAt - Math.floor(Date.now() / 1000);
            // 不足 60 秒则不再调度，交由 401 兜底处理
            if (remainingSeconds <= 60) {
                return;
            }
            // 到期前 60 秒静默续签，避免频繁刷新
            const delay = Math.max(0, Math.min((remainingSeconds - 60) * 1000, 24 * 60 * 60 * 1000));
            this.authRefreshTimer = setTimeout(async () => {
                this.authRefreshTimer = null;
                const refreshed = await this.refreshRoomSessionToken(normalizedRoom);
                if (refreshed && refreshed.token) {
                    // 续签结果按 scope 放回对应缓存，保持全局/房间语义
                    const cacheRoom = refreshed.scope === 'global' ? GLOBAL_ROOM_KEY : effective.key;
                    this.cacheAuthTokenForRoom(cacheRoom, refreshed.token, refreshed.expiresAt);
                    this.scheduleAuthRefresh(normalizedRoom);
                } else {
                    this.clearAuthTokenForRoom(normalizedRoom);
                    if (normalizedRoom === this.normalizeRoomName(this.room)) {
                        this.$toast.error(this.$t('authExpired'));
                        this.openAuthDialog(normalizedRoom);
                    }
                }
            }, delay);
        },
        async refreshRoomSessionToken(room) {
            const normalizedRoom = this.normalizeRoomName(room);
            const currentToken = this.getAuthTokenForRoom(normalizedRoom);
            if (!currentToken) {
                return null;
            }

            try {
                const response = await this.$http.post('auth/token/refresh', null, {
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
        setRoomProtection(room, isProtected) {
            const normalizedRoom = this.normalizeRoomName(room);
            this.$set(this.roomProtectionCache, normalizedRoom, Boolean(isProtected));
        },
        async fetchServerInfo(room = this.room, { token = '' } = {}) {
            const normalizedRoom = this.normalizeRoomName(room);
            const response = await this.$http.get('server', {
                params: new URLSearchParams([['room', normalizedRoom]]),
                headers: token ? {
                    Authorization: `Bearer ${token}`,
                } : undefined,
                __skipRoomAuthHandling: true,
            });
            if (Object.prototype.hasOwnProperty.call(response.data || {}, 'roomProtected')) {
                this.setRoomProtection(normalizedRoom, response.data.roomProtected);
            }
            return response.data;
        },
        async verifyRoomAccess(room, token) {
            const normalizedRoom = this.normalizeRoomName(room);
            const normalizedToken = (token || '').trim();
            if (!normalizedToken) {
                return false;
            }

            const serverInfo = await this.fetchServerInfo(normalizedRoom, {
                token: normalizedToken,
            });
            return serverInfo.auth ? serverInfo.authorized === true : true;
        },
        openAuthDialog(room, initialToken = '') {
            this.authPendingRoom = this.normalizeRoomName(room);
            this.roomDialog = false;
            // 输入框只接受密码，不预填会话令牌
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

            const candidateTokens = this.getKnownAuthTokensForRoom(normalizedRoom);
            for (const token of candidateTokens) {
                const verified = await this.verifyRoomAccess(normalizedRoom, token);
                if (verified) {
                    return token;
                }
            }

            if (interactive) {
                this.openAuthDialog(normalizedRoom);
            }

            return null;
        },
        async navigateToRoom(room) {
            const normalizedRoom = this.normalizeRoomName(room);
            const token = await this.resolveAuthTokenForRoom(normalizedRoom, { interactive: true });
            if (token === null) {
                return false;
            }

            const targetQuery = normalizedRoom ? { room: normalizedRoom } : {};
            const currentQuery = this.$router.currentRoute.query;
            
            // Check if already at target location to avoid NavigationDuplicated error
            if (normalizedRoom === this.normalizeRoomName(currentQuery.room || 'default')) {
                return true;
            }

            await this.$router.push({
                path: '/',
                query: targetQuery,
            });
            return true;
        },
        async submitAuthCodeForPendingRoom() {
            const targetRoom = this.authPendingRoom || this.normalizeRoomName(this.room);
            const password = (this.inputPassword || '').trim();
            if (!password || this.authDialogLoading) {
                return;
            }

            this.authDialogLoading = true;
            this.authCodeError = '';

            try {
                // 1. 验证密码是否正确
                const verified = await this.verifyRoomAccess(targetRoom, password);
                if (!verified) {
                    this.authCodeError = this.$t('authInvalid');
                    return;
                }

                // 2. 用验证过的密码获取 room session token
                const session = await this.obtainRoomSessionToken(targetRoom, password);
                if (!session || !session.token) {
                    this.authCodeError = this.$t('connectionFailedRetry');
                    return;
                }

                // 3. 只缓存 session token，不缓存原始密码。
                //    全局 scope 的令牌存入全局缓存，所有房间回退复用。
                if (session.scope === 'global') {
                    this.cacheAuthTokenForRoom(GLOBAL_ROOM_KEY, session.token, session.expiresAt);
                } else {
                    this.cacheAuthTokenForRoom(targetRoom, session.token, session.expiresAt);
                }
                // 全局令牌缓存到 __global__ 时不会触发续期调度，这里显式调度当前房间
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
                this.authCodeError = this.$t('connectionFailedRetry');
            } finally {
                this.authDialogLoading = false;
            }
        },
        async obtainRoomSessionToken(room, password) {
            const normalizedRoom = this.normalizeRoomName(room);
            try {
                const response = await this.$http.post('auth/token', {
                    password: password,
                }, {
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
                console.error('Failed to obtain session token:', error);
                return null;
            }
        },
        handleHttpUnauthorized(config = {}) {
            const room = this.getRequestRoom(config);
            this.clearAuthTokenForRoom(room);
            this.openAuthDialog(room);
        },
        async connect() {
            if (this.websocketConnecting) {
                return;
            }

            this.websocketConnecting = true;
            this.$toast(this.$t('connectingServer'));

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
                    // 通过 WebSocket 子协议传递 token，避免凭据出现在 URL/访问日志中
                    const protocols = resolvedToken ? [resolvedToken] : [];
                    const socket = new WebSocket(wsUrl, protocols);
                    socket.onopen = () => resolve(socket);
                    socket.onerror = reject;
                });

                this.websocket = ws;
                this.websocketConnecting = false;
                this.retry = 0;
                this.received = [];
                this.authCode = this.getAuthTokenForRoom(currentRoom);
                this.$toast(this.$t('connectionSuccess'));
                setInterval(() => {ws.send('')}, 30000);
                ws.onclose = () => {
                    this.websocket = null;
                    this.websocketConnecting = false;
                    this.device.splice(0);
                    if (this.retry < 3) {
                        this.retry++;
                        this.$toast(this.$t('reconnectingServer', { retry: this.retry }));
                        setTimeout(() => this.connect(), 3000);
                    } else if (this.getAuthTokenForRoom(this.room)) {
                        this.openAuthDialog(this.room);
                    }
                };
                ws.onmessage = e => {
                    try {
                        let parsed = JSON.parse(e.data);
                        (this.event[parsed.event] || (() => {}))(parsed.data);
                    } catch {}
                };
            } catch (error) {
                this.websocketConnecting = false;
                this.failure();
            }
        },
        disconnect() {
            this.websocketConnecting = false;
            if (this.websocket) {
                this.websocket.onclose = () => {};
                this.websocket.close();
                this.websocket = null;
            }
            this.$root.device = [];
        },
        failure() {
            this.websocket = null;
            this.$root.device = [];
            if (this.retry++ < 3) {
                // Retry connection logic might need translation too if it shows user messages
                this.connect();
            } else {
                // Use $t for the error message
                this.$toast.error(this.$t('connectionFailedRetry'), {
                    showClose: false,
                    dismissable: false,
                    timeout: -1, // Use -1 for infinite timeout as per Vuetify recommendation
                });
            }
        },
    },
    mounted() {
        this.syncAuthFromRoute(this.$route);
        this.authCode = this.getAuthTokenForRoom(this.room);
        this.connect();
    },
}