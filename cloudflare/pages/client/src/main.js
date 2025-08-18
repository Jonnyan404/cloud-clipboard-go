import Vue from 'vue';
import App from './App.vue';
import router from './router';
import vuetify from './plugins/vuetify';
import websocket from './websocket';
import axios from 'axios';
import VueAxios from 'vue-axios';
import linkify from 'vue-linkify';
import i18n from './vue-i18n';
import { config } from './config'; // 导入配置

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

// 根据配置设置 axios 基础 URL
if (config.apiBaseURL) {
    axios.defaults.baseURL = config.apiBaseURL;
    console.log('使用 API 基础路径:', config.apiBaseURL);
}

const app = new Vue({
    mixins: [websocket],
    data() {
        return {
            date: new Date,
            dark: null,
            config: {
                version: '',
                server: {
                    prefix: '',
                },
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
            // 环境信息
            deployConfig: config, // 将部署配置暴露给组件使用
            // 显示设置
            showTimestamp: localStorage.getItem('showTimestamp') !== null 
                ? localStorage.getItem('showTimestamp') === 'true' 
                : true,
            showDeviceInfo: localStorage.getItem('showDeviceInfo') !== null 
                ? localStorage.getItem('showDeviceInfo') === 'true' 
                : false,
            showSenderIP: localStorage.getItem('showSenderIP') !== null 
                ? localStorage.getItem('showSenderIP') === 'true' 
                : false,
        };
    },
    router,
    vuetify,
    i18n,
    render: h => h(App),
    watch: {
        dark(newval) {
            this.$vuetify.theme.dark = this.useDark;
            localStorage.setItem('darkmode', newval);
        },
        showTimestamp(newVal) {
            localStorage.setItem('showTimestamp', newVal);
        },
        showDeviceInfo(newVal) {
            localStorage.setItem('showDeviceInfo', newVal);
        },
        showSenderIP(newVal) {
            localStorage.setItem('showSenderIP', newVal);
        },
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