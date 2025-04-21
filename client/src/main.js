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
            received: [],
            device: [],
            send: {
                text: '',
                files: [],
            },
            config: {
                version: '',
                text: { limit: 0 },
                file: { expire: 0, chunk: 0, limit: 0 },
            },
            dark: localStorage.getItem('dark') || 'prefer',
            authCode: localStorage.getItem('auth') || '',
            authCodeDialog: false,
            room: '',
            roomInput: '',
            roomDialog: false,
        };
    },
    router,
    vuetify,
    i18n, // 将 i18n 实例添加到 Vue
    render: h => h(App),
    watch: {
        dark(newval) {
            localStorage.setItem('dark', newval);
            this.$vuetify.theme.dark = this.useDark;
        },
        room(newVal, oldVal) {
            if (this.websocket && newVal !== oldVal) {
                // 如果房间改变，重新连接 WebSocket
                this.websocket.close();
                this.connect();
            }
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
                }
            },
        },
    },
    created() {
        // 获取初始配置
        this.$http.get('config').then(response => {
            this.config = response.data;
        });
        // 初始化深色模式
        this.$vuetify.theme.dark = this.useDark;
        // 连接 WebSocket
        this.connect();
    },
    mounted() {
        // 监听系统深色模式变化
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
            if (this.dark === 'prefer') {
                this.$vuetify.theme.dark = e.matches;
            }
        });
    },
})

axios.interceptors.request.use(config => {
    if (app.authCode) {
        config.headers.Authorization = `Bearer ${app.authCode}`;
    }
    return config;
});

app.$mount('#app');