/**
 * 前端「基准目录」—— history 路由的 base、axios 的 baseURL、以及相对地址的解析依据。
 *
 * 值来自 `document.baseURI`：
 *   - 服务端在 `/s/<token>` 这类**深路径**上会注入 `<base href="<prefix>/">`（见 lib/spa_shell.go），
 *     否则外壳里 `./assets/…` 会按 `/clip/s/` 去解析，整套资源 404；
 *   - 根路径（`<prefix>/`）本来就不需要注入，因为文档目录就是 `<prefix>/`；
 *   - dev 由 vite.config.js 注入 `<base href="/">`（dev 没有 prefix）。
 *
 * 为什么不用 `import.meta.env.BASE_URL`：那是**构建期**常量（本项目 `base: ''`），而 prefix 是
 * **运行时**配置（服务端 `-prefix`）——同一个产物要能部署在任意子路径下。
 *
 * ⚠️ 这个模块必须在 router 之前被求值：`createWebHistory(APP_BASE)` 用它的值。
 */
function resolveAppBase() {
    if (typeof document === 'undefined' || typeof window === 'undefined') {
        return '/';
    }
    try {
        const base = new URL(document.baseURI, window.location.origin);
        return base.pathname.endsWith('/') ? base.pathname : `${base.pathname}/`;
    } catch {
        return '/';
    }
}

/** 路径形式（`/clip/`、`/`）——给 vue-router 的 `createWebHistory(base)` 用 */
export const APP_BASE = resolveAppBase();

/** 绝对形式（`https://host/clip/`）——给 axios 的 baseURL 与 `new URL(path, base)` 用 */
export const APP_BASE_URL = typeof window === 'undefined'
    ? APP_BASE
    : new URL(APP_BASE, window.location.origin).href;
