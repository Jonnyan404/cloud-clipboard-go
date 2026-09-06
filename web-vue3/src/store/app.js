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
        showDeviceInfo: localStorage.getItem('showDeviceInfo') !== null
            ? localStorage.getItem('showDeviceInfo') === 'true'
            : false,
        showSenderIP: localStorage.getItem('showSenderIP') !== null
            ? localStorage.getItem('showSenderIP') === 'true'
            : false,
        composerPrimary: localStorage.getItem('composerPrimary') || 'text',
        fullscreenSendClose: localStorage.getItem('fullscreenSendClose') !== null
            ? localStorage.getItem('fullscreenSendClose') === 'true'
            : true,
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
