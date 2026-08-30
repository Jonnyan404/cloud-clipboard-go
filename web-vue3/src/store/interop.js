import axios from 'axios';
import { useAppStore } from './app';
import { useWebSocketStore } from './websocket';

/**
 * 迁移桥接层。
 * 旧 Vue2 代码通过 root 实例（$root）共享状态、用 $http 发请求、用 $toast 提示。
 * Vue3 中：
 *  - 状态迁移到 Pinia（store/app、store/websocket）
 *  - axios 拦截器在此注册（401 → handleHttpUnauthorized）
 */
export function setupAxiosInterceptors() {
    axios.interceptors.request.use(config => {
        if (!config.headers) {
            config.headers = {};
        }
        const ws = useWebSocketStore();
        if (!config.headers.Authorization) {
            const token = ws.getRequestAuthToken(config);
            if (token) {
                config.headers.Authorization = `Bearer ${token}`;
            }
        }
        return config;
    });

    axios.interceptors.response.use(
        response => response,
        error => {
            const status = error && error.response ? error.response.status : 0;
            const config = error && error.config ? error.config : {};
            if (status === 401 && !config.__skipRoomAuthHandling) {
                const ws = useWebSocketStore();
                ws.handleHttpUnauthorized(config);
            }
            return Promise.reject(error);
        },
    );
}
