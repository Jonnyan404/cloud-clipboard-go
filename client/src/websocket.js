const ROOM_AUTH_CACHE_KEY = 'roomAuthCache';
const DEFAULT_ROOM_KEY = '__default__';

function loadRoomAuthCache() {
    try {
        const raw = localStorage.getItem(ROOM_AUTH_CACHE_KEY);
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
            authCodeDialog: false,
            authPendingRoom: '',
            authCodeError: '',
            authDialogLoading: false,
            roomAuthCache: loadRoomAuthCache(),
            roomProtectionCache: {},
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
                },
            },
        };
    },
    watch: {
        room() {
            this.authCode = this.getAuthTokenForRoom(this.room);
            this.disconnect();
            this.connect();
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
            localStorage.setItem(ROOM_AUTH_CACHE_KEY, JSON.stringify(this.roomAuthCache));
        },
        getAuthTokenForRoom(room = this.room) {
            return this.roomAuthCache[this.getRoomStorageKey(room)] || '';
        },
        cacheAuthTokenForRoom(room, token) {
            const normalizedToken = (token || '').trim();
            const key = this.getRoomStorageKey(room);

            if (!normalizedToken) {
                this.clearAuthTokenForRoom(room);
                return;
            }

            this.$set(this.roomAuthCache, key, normalizedToken);
            this.persistRoomAuthCache();

            if (this.normalizeRoomName(room) === this.normalizeRoomName(this.room)) {
                this.authCode = normalizedToken;
            }
        },
        clearAuthTokenForRoom(room = this.room) {
            const key = this.getRoomStorageKey(room);
            if (Object.prototype.hasOwnProperty.call(this.roomAuthCache, key)) {
                this.$delete(this.roomAuthCache, key);
                this.persistRoomAuthCache();
            }

            if (this.normalizeRoomName(room) === this.normalizeRoomName(this.room)) {
                this.authCode = '';
            }
        },
        getKnownAuthTokens(room = this.room) {
            const tokens = [];
            const pushToken = token => {
                const normalizedToken = (token || '').trim();
                if (normalizedToken && !tokens.includes(normalizedToken)) {
                    tokens.push(normalizedToken);
                }
            };

            pushToken(this.getAuthTokenForRoom(room));
            pushToken(this.authCode);
            Object.values(this.roomAuthCache).forEach(pushToken);

            return tokens;
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
            this.authCode = initialToken || this.getAuthTokenForRoom(room) || '';
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

            const candidateTokens = this.getKnownAuthTokens(normalizedRoom);
            for (const token of candidateTokens) {
                const verified = await this.verifyRoomAccess(normalizedRoom, token);
                if (verified) {
                    this.cacheAuthTokenForRoom(normalizedRoom, token);
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

            await this.$router.push({
                path: '/',
                query: normalizedRoom ? { room: normalizedRoom } : {},
            });
            return true;
        },
        async submitAuthCodeForPendingRoom() {
            const targetRoom = this.authPendingRoom || this.normalizeRoomName(this.room);
            const token = (this.authCode || '').trim();
            if (!token || this.authDialogLoading) {
                return;
            }

            this.authDialogLoading = true;
            this.authCodeError = '';

            try {
                const verified = await this.verifyRoomAccess(targetRoom, token);
                if (!verified) {
                    this.authCodeError = this.$t('authInvalid');
                    return;
                }

                this.cacheAuthTokenForRoom(targetRoom, token);
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
                    if (resolvedToken) {
                        wsUrl.searchParams.set('auth', resolvedToken);
                    }
                    wsUrl.searchParams.set('room', currentRoom);
                    const socket = new WebSocket(wsUrl);
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
                        this.openAuthDialog(this.room, this.getAuthTokenForRoom(this.room));
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
        this.authCode = this.getAuthTokenForRoom(this.room);
        this.connect();
    },
}