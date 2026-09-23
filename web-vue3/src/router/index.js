import { createRouter, createWebHistory } from 'vue-router';
import Home from '@/views/Home.vue';
import { APP_BASE } from '@/base.js';

// history 路由（不是 hash）。
//
// 为什么必须换：分享链接的 token 要能被**服务端读到** —— 浏览器根本不会把 `#` 之后的部分发出去，
// 而社交平台的抓取程序又不执行 JS，于是 OG 预览标签只能由服务端写进 HTML。token 在路径里，
// 服务端才能既渲染预览卡片、又把同一份外壳交给真人（见 lib/spa_shell.go 文件头）。
//
// base 取**运行时**推导出来的 APP_BASE（服务端注入的 `<base>` / 文档目录），不是构建期的
// `import.meta.env.BASE_URL`：prefix 是部署时配的，同一个产物要能落在任意子路径下。
const router = createRouter({
    history: createWebHistory(APP_BASE),
    routes: [
        {
            path: '/',
            component: Home,
            meta: { keepAlive: true },
        },
        {
            // 分享页：`<prefix>/s/<token>` —— 这就是分享链接本身，抓取程序和真人拿的是同一份
            // 服务端 HTML（OG 卡片已经写在外壳里）。
            // token 允许缺失（直接打开 `/s`）：给一个可读的「链接无效」，而不是无路由白屏。
            path: '/s/:token?',
            component: () => import('@/views/ShareView.vue'),
            meta: { keepAlive: false, sharePage: true },
        },
    ],
});

export default router;
