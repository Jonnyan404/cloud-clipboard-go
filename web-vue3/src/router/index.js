import { createRouter, createWebHashHistory } from 'vue-router';
import Home from '@/views/Home.vue';
import Device from '@/views/Device.vue';
import About from '@/views/About.vue';

const router = createRouter({
    history: createWebHashHistory(),
    routes: [
        {
            path: '/',
            component: Home,
            meta: { keepAlive: true },
        },
        {
            path: '/device',
            component: Device,
            meta: { keepAlive: true },
        },
        {
            path: '/about',
            component: About,
            meta: { keepAlive: true },
        },
    ],
});

export default router;
