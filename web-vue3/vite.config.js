import { fileURLToPath, URL } from 'node:url';
import { randomBytes } from 'node:crypto';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vuetify from 'vite-plugin-vuetify';
import { VitePWA } from 'vite-plugin-pwa';

export default defineConfig(({ command }) => {
    // 每次构建生成一个唯一指纹，注入到 bundle 和 <html data-build-id> 两处，用来核对
    // 「线上跑的是哪次构建」。判据用 `command` 而不是 `process.env.NODE_ENV`：开发服务器
    // 下根本没有构建这回事，固定 'dev' 即可，而 NODE_ENV 是外部环境设的，不该拿来当依据
    // （它一旦不是 production，指纹就会静默变成 'dev' 并写进产物）。
    const buildId = command === 'build' ? randomBytes(4).toString('hex') : 'dev';

    return {
        plugins: [
            vue(),
            vuetify({ autoImport: true }),
            // define 只替换 JS 模块；index.html 里的 __BUILD_ID__ 得靠这个钩子。
            // 写到 <html data-build-id> 上，view-source 就能核对线上是哪次构建。
            //
            // dev 下额外注入 `<base href="/">`：history 路由的深路径（`/s/<token>`）靠它把相对
            // 地址（`./assets/…`、以及 axios 的相对接口路径）拉回根目录。dev 没有 prefix，
            // 所以固定 `/` 就是对的；线上这一份由服务端注入，带真实 prefix（见 lib/spa_shell.go）。
            {
                name: 'inject-build-id-into-html',
                transformIndexHtml(html) {
                    const withBuildId = html.replaceAll('__BUILD_ID__', buildId);
                    if (command !== 'build') {
                        return withBuildId.replace('<head>', '<head>\n    <base href="/">');
                    }
                    return withBuildId;
                },
            },
            VitePWA({
                registerType: 'autoUpdate',
                injectRegister: null,
                includeAssets: ['favicon.svg', 'favicon.ico', 'apple-touch-icon.png', 'pwa-192x192.png', 'pwa-512x512.png', 'reward-wechat.png', 'reward-alipay.png'],
                manifest: {
                    name: 'Cloud Clipboard',
                    short_name: 'Clipboard',
                    description: 'Browser-based cloud clipboard for text and files',
                    lang: 'zh',
                    start_url: './',
                    scope: './',
                    display: 'standalone',
                    orientation: 'any',
                    background_color: '#1e88e5',
                    theme_color: '#1e88e5',
                    icons: [
                        {
                            src: 'pwa-192x192.png',
                            sizes: '192x192',
                            type: 'image/png',
                        },
                        {
                            src: 'pwa-512x512.png',
                            sizes: '512x512',
                            type: 'image/png',
                        },
                        {
                            src: 'pwa-512x512.png',
                            sizes: '512x512',
                            type: 'image/png',
                            purpose: 'maskable',
                        },
                    ],
                },
                workbox: {
                    globPatterns: ['**/*.{js,css,html,svg,png,ico,woff2,woff,ttf,eot}'],
                    cleanupOutdatedCaches: true,
                    navigateFallback: 'index.html',
                    navigateFallbackDenylist: [
                        // /s/<token> 的响应由**服务端**生成：外层是 SPA 外壳，但 `<base href="<prefix>/">`
                        // 和 OG 标签是服务端注进去的（见 lib/spa_shell.go）。被 SW 从预缓存回掉的话，
                        // 深路径上会拿到一份没有 `<base>` 的外壳 —— `./assets/…` 全按 `/clip/s/` 解析，
                        // 页面直接白。所以分享链接的导航一律走网络。
                        // （抓取程序本来就不跑 SW，OG 那条路不受影响。）
                        /^\/s\//,
                        /^\/server/,
                        /^\/text/,
                        /^\/auth/,
                        /^\/upload/,
                        /^\/push/,
                        /^\/rooms/,
                        /^\/share/,
                        /^\/file\//,
                        /^\/revoke/,
                        /^\/content\//,
                        /^\/push/,
                        // ⚠️ 下面这三个是**服务端直接吐出去的东西，不是 SPA 路由**，
                        // 漏一个的后果非常难查：SW 的导航兜底会把点击整个吞掉、
                        // 返回预缓存里的 index.html —— 现象就是「点了定时任务，界面还是 SPA 首页」，
                        // 而且刷新一下有时又好了（SW 生效时机不同），最难查的那种。
                        // 契约测试 TestSpaServiceWorkerCoversEveryServerRoute 盯着这份名单：
                        // main.go 里每加一条服务端路由，这里漏了就红。
                        /^\/automation/,
                        /^\/tasks/,
                        /^\/myip/,
                    ],
                    runtimeCaching: [
                        {
                            urlPattern: /^\/(server|text|auth|upload|push|rooms|share|file|revoke|content|tasks|myip)/,
                            handler: 'NetworkOnly',
                            method: 'GET',
                        },
                    ],
                },
            }),
        ],
        define: {
            '__VUE_PROD_HYDRATION_MISMATCH_DETAILS__': false,
            // 交给 src/sw-update.js 消费（设置弹窗展示 + 「现在刷还是等会刷」的判定）
            'import.meta.env.__BUILD_ID__': JSON.stringify(buildId),
        },
        resolve: {
            alias: {
                '@': fileURLToPath(new URL('./src', import.meta.url)),
            },
        },
        base: '',
        build: {
            outDir: 'dist',
            sourcemap: false,
            chunkSizeWarningLimit: 600,
            rollupOptions: {
                output: {
                    manualChunks(id) {
                        if (!id.includes('node_modules')) {
                            return undefined;
                        }
                        if (id.includes('/vuetify') || id.includes('/@mdi/') || id.includes('mdi/fonts')) {
                            return 'vuetify';
                        }
                        if (id.includes('/vue/') || id.includes('/vue-router') || id.includes('/pinia') || id.includes('/@vue/')) {
                            return 'vue-core';
                        }
                        if (id.includes('/vue-i18n')) {
                            return 'i18n';
                        }
                        if (id.includes('/qrcode.vue') || id.includes('/axios')) {
                            return 'vendor';
                        }
                        return undefined;
                    },
                },
            },
        },
        server: {
            port: 1210,
            proxy: {
                '/server': { target: 'http://localhost:9501/', changeOrigin: true },
                '/push': { target: 'http://localhost:9501/', changeOrigin: true, ws: true },
                '/auth': { target: 'http://localhost:9501/', changeOrigin: true },
                '/rooms': { target: 'http://localhost:9501/', changeOrigin: true },
                '/share': { target: 'http://localhost:9501/', changeOrigin: true },
                '/file': { target: 'http://localhost:9501/', changeOrigin: true },
                '/text': { target: 'http://localhost:9501/', changeOrigin: true },
                '/upload': { target: 'http://localhost:9501/', changeOrigin: true },
                '/revoke': { target: 'http://localhost:9501/', changeOrigin: true },
                '/content': { target: 'http://localhost:9501/', changeOrigin: true },
                // 定时自动化：管理页 `/automation` 与它调的接口 `/tasks`。
                //
                // ⚠️ 这两个**必须代理**，和下面 `/s/` 那条的理由正好相反：
                // `/automation` 是**服务端渲染的独立页面**（go:embed 进二进制，见
                // lib/automation_page.go），dev server 根本没有这一页 —— 不代理的话，
                // 点工具栏那个入口会被 vite 的 SPA 回退接住、回到首页，看起来就是「功能坏了」
                // （线上是另一个成因：被 Service Worker 的导航兜底吞掉，症状一模一样）。
                // `/tasks` 是那个页面自己调的接口（相对路径），同一个理由。
                //
                // 代理之后浏览器看到的仍是 dev server 的源，所以 `sessionStorage` 里那份
                // `roomAuthCache` 照常共享，进去不会二次要密码。
                '/automation': { target: 'http://localhost:9501/', changeOrigin: true },
                '/tasks': { target: 'http://localhost:9501/', changeOrigin: true },
                // ⚠️ `/s/` **刻意不代理**。分享地址（`/s/<token>`）现在由服务端返回一份注入了 OG
                // 的外壳（见 lib/spa_shell.go），代理的话 dev 下打开分享链接会落到「后端嵌入的
                // 上次构建产物」上 —— 改前端代码看不到效果，比不代理更迷惑。这里的 SPA 回退会让
                // dev server 自己的 HTML 接住它（dev 那份带 `<base href="/">`），分享页照常开发；
                // 要看注入出来的卡片就 curl 后端：`curl -s localhost:9501/s/<token> | grep og:`。
            },
        },
    };
});
