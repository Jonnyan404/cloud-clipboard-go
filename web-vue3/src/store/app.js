import { defineStore } from 'pinia';

export const useAppStore = defineStore('app', {
    state: () => ({
        dark: null,
        config: {
            version: '',
            server: { history: 0, prefix: '', roomList: false },
            text: { limit: 0 },
            file: { expire: 0, chunk: 0, limit: 0 },
        },
        send: {
            text: '',
            files: [],
        },
        received: [],
        roomMessagesCache: {},
        isRoomSyncing: false,
        device: [],
        showTimestamp: localStorage.getItem('showTimestamp') !== null
            ? localStorage.getItem('showTimestamp') === 'true'
            : true,
        // 默认开：设备名是「这条消息从哪台设备发的」的唯一线索，尤其快捷指令这类
        // UA 认不出来的来源。想关的人可以在设置里关掉，已显式设置过的用户不受影响
        // （上面判了 !== null，只有从没碰过这个开关的人才会落到新默认值）。
        showDeviceInfo: localStorage.getItem('showDeviceInfo') !== null
            ? localStorage.getItem('showDeviceInfo') === 'true'
            : true,
        showSenderIP: localStorage.getItem('showSenderIP') !== null
            ? localStorage.getItem('showSenderIP') === 'true'
            : false,
        composerPrimary: localStorage.getItem('composerPrimary') || 'text',
        fullscreenSendClose: localStorage.getItem('fullscreenSendClose') !== null
            ? localStorage.getItem('fullscreenSendClose') === 'true'
            : true,
        uiMode: localStorage.getItem('uiMode') || 'default',
    }),
    actions: {
        setConfig(config) {
            this.config = config;
        },
        toggleComposerPrimary() {
            this.composerPrimary = this.composerPrimary === 'files' ? 'text' : 'files';
            localStorage.setItem('composerPrimary', this.composerPrimary);
        },
        toggleFullscreenSendClose() {
            this.fullscreenSendClose = !this.fullscreenSendClose;
            localStorage.setItem('fullscreenSendClose', String(this.fullscreenSendClose));
        },
        setUiMode(mode) {
            this.uiMode = mode;
            localStorage.setItem('uiMode', mode);
        },
    },
    getters: {
        useDark() {
            switch (this.dark) {
                case 'time': {
                    const hour = new Date().getHours();
                    return hour < 7 || hour >= 19;
                }
                case 'prefer':
                    return window.matchMedia('(prefers-color-scheme: dark)').matches;
                case 'enable':
                    return true;
                case 'disable':
                    return false;
                default:
                    return false;
            }
        },
    },
});
