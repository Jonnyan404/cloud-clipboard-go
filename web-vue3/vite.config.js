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
            {
                name: 'inject-build-id-into-html',
                transformIndexHtml(html) {
                    return html.replaceAll('__BUILD_ID__', buildId);
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
                    ],
                    runtimeCaching: [
                        {
                            urlPattern: /^\/(server|text|auth|upload|push|rooms|share|file|revoke|content)/,
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
            },
        },
    };
});
