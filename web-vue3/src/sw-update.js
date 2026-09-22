// 让「部署了新版本」这件事在所有设备上自动生效 —— 包括手机。
//
// 根因：PWA 的 Service Worker 把整站（含 index.html）precached，装机那次的旧
// bundle 会一直服务到 SW 更新为止。手机上用户不会去「强制刷新」，旧版本可能
// 挂很久。所以这里主动接管 SW 的更新周期：
//
//   · 检测到新 SW 安装完成（新 precache 就绪）→ 自动 reload 一次。
//     只需要 reload：workbox 的 precache 按 revision 哈希，新 SW 激活后自然
//     拿到新资源。用 sessionStorage 防死循环 —— SW 每次注册都可能触发
//     updatefound，不拦着的话弱网下会一直刷新刷到用户怀疑人生。
//
//   · 「正在使用旧版本，点击刷新」的兜底：如果自动 reload 没能换到新 SW
//     （个别浏览器上 waiting 状态不自动接管），给用户一个手动按钮。
//
// buildId 同时暴露给 App.vue 展示，用来在设置里肉眼核对「跑的是哪次构建」。

export const buildId = import.meta.env.__BUILD_ID__;

export function setupServiceWorkerUpdate() {
    if (!('serviceWorker' in navigator)) {
        return;
    }

    const reloadedKey = 'sw-reloaded-build';
    let registration = null;

    const reloadForNewBuild = () => {
        // 同一次构建只 reload 一次；换构建（buildId 变了）允许再刷。
        if (sessionStorage.getItem(reloadedKey) === buildId) {
            return false;
        }
        sessionStorage.setItem(reloadedKey, buildId);
        window.location.reload();
        return true;
    };

    // 兜底入口：注册成功后监听后续的更新
    const watchRegistration = (reg) => {
        registration = reg;
        reg.addEventListener('updatefound', () => {
            const installing = reg.installing;
            if (!installing) {
                return;
            }
            installing.addEventListener('statechange', () => {
                // installed = 新 precache 就绪；activated 需要等旧页面全部关闭，
                // 所以在 installed 就刷，别等 activated。
                if (installing.state === 'installed' && navigator.serviceWorker.controller) {
                    reloadForNewBuild();
                }
            });
        });
    };

    window.addEventListener('load', () => {
        navigator.serviceWorker
            .register('./sw.js', { scope: './', updateViaCache: 'none' })
            .then((reg) => {
                watchRegistration(reg);
                // 页面打开期间定期找更新（比如部署后一直挂着的标签页/手机 PWA）
                setInterval(() => reg.update().catch(() => {}), 60 * 60 * 1000);
            })
            .catch((error) => {
                console.error('Service worker registration failed:', error);
            });
    });

    // 双保险：页面是被 SW 控制的、而当前 SW 脚本已经换成新的 → 刷一次
    navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (registration) {
            reloadForNewBuild();
        }
    });
}
