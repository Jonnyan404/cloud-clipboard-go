import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vuetify from 'vite-plugin-vuetify';

export default defineConfig({
    plugins: [
        vue(),
        vuetify({ autoImport: true }),
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
