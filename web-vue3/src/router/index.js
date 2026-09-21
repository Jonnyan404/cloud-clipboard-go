import { createRouter, createWebHashHistory } from 'vue-router';
import Home from '@/views/Home.vue';

const router = createRouter({
    history: createWebHashHistory(),
    routes: [
        {
            path: '/',
            component: Home,
            meta: { keepAlive: true },
        },
        {
            // 分享页：收件人打开 `/#/s?t=<token>` 看到的那一页。
            // 走 hash 路由而不是服务端渲染 —— 两个后端就都不用碰模板，
            // 只需要会拼 `https://host<prefix>/#/s?t=...` 这一个地址（见 buildSharePageURL）。
            // 用懒加载：收件人不需要主应用那一整包。
            path: '/s',
            component: () => import('@/views/ShareView.vue'),
            meta: { keepAlive: false, sharePage: true },
        },
    ],
});

export default router;
