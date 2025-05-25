import Vue from 'vue';
import App from './App.vue';
import router from './router';
import vuetify from './plugins/vuetify';
import websocket from './websocket';
import axios from 'axios';
import VueAxios from 'vue-axios';
import linkify from 'vue-linkify';
import i18n from './vue-i18n'; // 导入 i18n 实例

import {
    prettyFileSize,
    percentage,
    formatTimestamp,
} from './util';

import 'typeface-roboto/index.css';

Vue.config.productionTip = false;

Vue.use(VueAxios, axios);
Vue.directive('linkified', linkify);
Vue.filter('prettyFileSize', prettyFileSize);
Vue.filter('percentage', percentage);
Vue.filter('formatTimestamp', formatTimestamp);

const app = new Vue({
    mixins: [websocket],
    data() {
        return {
            date: new Date,
            dark: null,
            config: {
                version: '',
                text: {
                    limit: 0,
                },
                file: {
                    expire: 0,
                    chunk: 0,
                    limit: 0,
                },
            },
            send: {
                text: '',
                files: [],
            },
            received: [],
            device: [],
            // --- 新增显示设置 (默认 true) ---
            // --- 修改显示设置初始化逻辑 ---
            showTimestamp: localStorage.getItem('showTimestamp') !== null 
                ? localStorage.getItem('showTimestamp') === 'true' 
                : true,  // 默认显示时间戳
            showDeviceInfo: localStorage.getItem('showDeviceInfo') !== null 
                ? localStorage.getItem('showDeviceInfo') === 'true' 
                : false, // 默认不显示设备信息
            showSenderIP: localStorage.getItem('showSenderIP') !== null 
                ? localStorage.getItem('showSenderIP') === 'true' 
                : false, // 默认不显示发送者IP
            // --- 结束新增 ---
        };
    },
    router,
    vuetify,
    i18n, // 将 i18n 实例添加到 Vue
    render: h => h(App),
    watch: {
        dark(newval) {
            this.$vuetify.theme.dark = this.useDark;
            localStorage.setItem('darkmode', newval);
        },
        // --- 新增 Watchers ---
        showTimestamp(newVal) {
            localStorage.setItem('showTimestamp', newVal);
        },
        showDeviceInfo(newVal) {
            localStorage.setItem('showDeviceInfo', newVal);
        },
        showSenderIP(newVal) {
            localStorage.setItem('showSenderIP', newVal);
        },
        // --- 结束新增 ---
    },
    computed: {
        useDark: {
            cache: false,
            get() {
                switch (this.dark) {
                    case 'time':
                        const hour = new Date().getHours();
                        return hour < 7 || hour >= 19;
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
    },
    mounted() {
        this.dark = localStorage.getItem('darkmode') || 'prefer';
        this.$vuetify.theme.dark = this.useDark;
        setInterval(() => {
            this.date = new Date;
            this.$vuetify.theme.dark = this.useDark;
        }, 1000);
    },
})


axios.interceptors.request.use(config => {
    if (app.authCode) {
        config.headers.Authorization = `Bearer ${app.authCode}`;
    }
    return config;
});

app.$mount('#app');