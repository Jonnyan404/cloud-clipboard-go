import { config } from './config'; // 导入配置

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
            date: new Date(),
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
            this.$toast(this.$t('connectingServer'));

            // 根据配置决定 server 端点
            const serverEndpoint = config.apiBaseURL ? 
                `${config.apiBaseURL}/server` : 
                '/server';

            this.$http.get(serverEndpoint).then(response => {
                if (this.authCode) localStorage.setItem('auth', this.authCode);
                return new Promise((resolve, reject) => {
                    let wsUrl;
                    
                    if (config.wsBaseURL) {
                        // 使用配置的 WebSocket 基础路径（Cloudflare 环境）
                        wsUrl = new URL(`${config.wsBaseURL}/push`);
                        // 确保使用正确的协议
                        wsUrl.protocol = wsUrl.protocol === 'https:' ? 'wss:' : 'ws:';
                    } else {
                        // 本地或传统部署环境
                        wsUrl = new URL(response.data.server);
                        wsUrl.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
                        wsUrl.port = location.port;
                    }
                    
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
                this.connect();
            } else {
                this.$toast.error(this.$t('connectionFailedRetry'), {
                    showClose: false,
                    dismissable: false,
                    timeout: -1,
                });
            }
        },
    },
    mounted() {
        this.connect();
    },
}