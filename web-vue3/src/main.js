import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import vuetify from './plugins/vuetify';
import i18n from './vue-i18n';
import pinia from './store';
import { setupAxiosInterceptors } from './store/interop';
import { useWebSocketStore } from './store/websocket';
import { useAppStore } from './store/app';

setupAxiosInterceptors();

const app = createApp(App);

app.use(pinia);

const appStore = useAppStore();
const wsStore = useWebSocketStore();
app.use(router);
app.use(vuetify);
app.use(i18n);

router.isReady().then(() => {
    appStore.dark = localStorage.getItem('darkmode') || 'prefer';
    // 分享页是给收件人看的独立页面：不建 WebSocket、不碰房间状态。
    // 它只认 URL 里的 token，走自己那几个相对路径请求（见 views/ShareView.vue）。
    if (router.currentRoute.value.meta?.sharePage) {
        return;
    }
    wsStore.initFromRoute(router.currentRoute.value.query.room || '');
    wsStore.connect();
});

app.mount('#app');
