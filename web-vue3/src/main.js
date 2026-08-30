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
    wsStore.initFromRoute(router.currentRoute.value.query.room || '');
    wsStore.connect();
});

app.mount('#app');
