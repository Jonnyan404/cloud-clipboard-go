export default {
    data() {
        return {
            websocket: null,
            websocketConnecting: false,
            authCode: localStorage.getItem('auth') || '',
            authCodeDialog: false,
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
                    this.authCode = '';
                    localStorage.removeItem('auth');
                },
            },
        };
    },
    watch: {
        room() {
            this.disconnect();
            this.connect();
        },
    },
    methods: {
        connect() {
            this.websocketConnecting = true;
            this.$toast(this.$t('connectingServer')); // Translate toast
            this.$http.get('server').then(response => {
                if (this.authCode) localStorage.setItem('auth', this.authCode);
                return new Promise((resolve, reject) => {
                    const wsUrl = new URL(response.data.server);
                    wsUrl.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
                    wsUrl.port = location.port;
                    if (response.data.auth) {
                        if (this.authCode) {
                            wsUrl.searchParams.set('auth', this.authCode);
                        } else {
                            this.authCodeDialog = true;
                            return;
                        }
                    }
                    wsUrl.searchParams.set('room', this.room);
                    const ws = new WebSocket(wsUrl);
                    ws.onopen = () => resolve(ws);
                    ws.onerror = reject;
                });
            }).then((/** @type {WebSocket} */ ws) => {
                this.websocket = ws;
                this.websocketConnecting = false;
                this.retry = 0;
                this.received = [];
                this.$toast(this.$t('connectionSuccess')); // Translate toast
                setInterval(() => {ws.send('')}, 30000);
                ws.onclose = () => {
                    this.websocket = null;
                    this.websocketConnecting = false;
                    this.device.splice(0);
                    if (this.retry < 3) {
                        this.retry++;
                        this.$toast(this.$t('reconnectingServer', { retry: this.retry })); // Translate toast
                        setTimeout(() => this.connect(), 3000);
                    } else if (this.authCode) {
                        this.authCodeDialog = true;
                    }
                };
                ws.onmessage = e => {
                    try {
                        let parsed = JSON.parse(e.data);
                        (this.event[parsed.event] || (() => {}))(parsed.data);
                    } catch {}
                };
            }).catch(error => {
                // console.log(error);
                this.websocketConnecting = false;
                this.failure();
            });
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
            localStorage.removeItem('auth');
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
        this.connect();
    },
}