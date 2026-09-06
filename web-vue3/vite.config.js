import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vuetify from 'vite-plugin-vuetify';
import { VitePWA } from 'vite-plugin-pwa';

export default defineConfig({
    plugins: [
        vue(),
        vuetify({ autoImport: true }),
        VitePWA({
            registerType: 'autoUpdate',
            injectRegister: null,
            includeAssets: ['favicon.svg', 'favicon.ico', 'pwa-192x192.png', 'pwa-512x512.png', 'reward.png'],
            manifest: {
                name: 'Cloud Clipboard',
                short_name: 'Clipboard',
                description: 'Browser-based cloud clipboard for text and files',
                lang: 'zh',
                start_url: './',
                scope: './',
                display: 'standalone',
                orientation: 'any',
                background_color: '#35495e',
                theme_color: '#35495e',
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
});
