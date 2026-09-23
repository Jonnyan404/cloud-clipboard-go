import { createApp } from 'vue';
// 代码高亮的令牌配色：全局一份（CodeBlock 与 MarkdownBody 共用，见文件头注释）
import './styles/highlight.css';
import axios from 'axios';
import App from './App.vue';
import router from './router';
import vuetify from './plugins/vuetify';
import i18n from './vue-i18n';
import pinia from './store';
import { APP_BASE_URL } from './base.js';
import { setupAxiosInterceptors } from './store/interop';
import { useWebSocketStore } from './store/websocket';
import { useAppStore } from './store/app';
import { setupServiceWorkerUpdate } from './sw-update.js';

// 全部接口调用都用**相对路径**（`share`、`content/7`…），而 history 模式下文档目录不再是
// `<prefix>/`：分享页的地址是 `<prefix>/s/<token>`，相对路径会被解析成 `<prefix>/s/share`。
// 统一给 axios 一个绝对 baseURL（= 外壳的基准目录，也就是 `<prefix>/`），
// 这个约定与「相对路径不带前导斜杠」配套 —— 带前导斜杠的调用会绕过 baseURL（本仓库没有那种写法）。
axios.defaults.baseURL = APP_BASE_URL;

setupAxiosInterceptors();

const app = createApp(App);

app.use(pinia);

// SW 更新检测在 pinia 之后装：它要读「有没有还没发出去的内容」来决定现在刷还是等会刷
// （手机 PWA 上用户永远不会手动强刷，见 sw-update.js）。
setupServiceWorkerUpdate();

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
