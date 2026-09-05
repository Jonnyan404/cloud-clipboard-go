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
    ],
});

export default router;
