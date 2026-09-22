// 让「部署了新版本」这件事在所有设备上自动生效 —— 包括手机。
//
// 根因：PWA 的 Service Worker 把整站（含 index.html）precached，装机那一次的旧 bundle
// 会一直服务到 SW 更新为止；手机上没人会去「强制刷新」，旧版本可能挂很久。
//
// 这个模块只做两件事：
//   1. 新版本**接管页面**时刷新一次；
//   2. 页面一直挂着时每小时问一次有没有新版本（没有导航事件，浏览器不会自己去查）。
//
// 为什么只认 `controllerchange`：`sw.js` 开着 `skipWaiting` + `clientsClaim`
// （来自 vite.config.js 的 `registerType: 'autoUpdate'`），新 SW 装上就会立刻激活并
// `clients.claim()` —— 客户端能看到的信号就是它，而且信号到达时页面**已经**由新 SW
// 服务，刷下去必定是新资源。所以既不需要防抖，也不会有循环。
//
// 为什么不再监听 `updatefound`/`installed`：那一刻页面**还归旧 SW 管**，刷新可能白刷；
// 更糟的是当时用来防抖的「本次构建 id」是**旧** buildId，白刷一次之后，真正该刷的
// controllerchange 会被自己的防抖判等挡掉，那个标签页就永远停在旧版。两个信号源合一之后
// 只剩一条路径，这类竞态也随之消失。
//
// 首次访问不算更新：新 SW 抢走控制权同样会触发 controllerchange，但那时用户还没见过任何
// 旧版本 —— 刷一下只是白屏加一次无谓的加载。
//
// buildId 同时暴露给 App.vue 展示，用来在设置里肉眼核对「跑的是哪次构建」。

import { watch } from 'vue';
import { useAppStore } from '@/store/app';
import { toast } from '@/plugins/toast';
import i18n from '@/vue-i18n';

export const buildId = import.meta.env.__BUILD_ID__;

/**
 * 现在刷新会不会弄丢用户正在写的东西。
 *
 * 两类都要看：
 *   · **还没发出去的内容** —— `app.send` 只在内存里，没有草稿持久化，刷一下就没了；
 *   · **屏幕上正在编辑的输入框** —— 房间名、分享密码、搜索词这类，同样是内存里的草稿。
 *
 * 这个判定只在「真要刷的那一刻」求值：页面已经在用旧版本是小事，把人写了一半的东西冲掉
 * 就没法解释了。
 */
function hasUnsavedInput() {
    const app = useAppStore();
    if (app.send.text || app.send.files.length) {
        return true;
    }
    const el = document.activeElement;
    const editing = el && (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT');
    return Boolean(editing && el.value);
}

export function setupServiceWorkerUpdate() {
    // dev 下 vite-plugin-pwa 不生成 sw.js（没开 devOptions），注册只会 404 报错
    if (!import.meta.env.PROD || !('serviceWorker' in navigator)) {
        return;
    }

    // 只有「本来就被某个 SW 控制着」才说明这是版本更替，不是首次安装
    const isVersionChange = Boolean(navigator.serviceWorker.controller);
    // 新版本已经接管，只是在等一个不会丢数据的时机
    let waitingForIdle = false;

    const reloadForNewVersion = () => {
        if (hasUnsavedInput()) {
            // 用户正在写东西：别抢。等输入框空了再换（下面那个 watch），或者等他下次自己
            // 打开页面 —— 新 SW 已经接管，那时加载到的就是新版。
            if (!waitingForIdle) {
                waitingForIdle = true;
                toast(i18n.global.t('newVersionReady'));
            }
            return;
        }
        window.location.reload();
    };

    navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (isVersionChange) {
            reloadForNewVersion();
        }
    });

    // 输入框空了（消息发出去了、编辑器关了）→ 补上那次没刷成的刷新
    watch(hasUnsavedInput, (unsaved) => {
        if (waitingForIdle && !unsaved) {
            window.location.reload();
        }
    });

    window.addEventListener('load', () => {
        navigator.serviceWorker
            .register('./sw.js', { scope: './', updateViaCache: 'none' })
            .then((registration) => {
                // 页面一直开着时定期找更新 —— 长开的标签页 / 手机 PWA 没有导航事件，
                // 浏览器不会自己去问。（后台标签页的定时器会被节流，所以这只是尽力而为，
                // 可靠的那条路仍然是「下次导航时浏览器自己查」。）
                setInterval(() => registration.update().catch(() => {}), 60 * 60 * 1000);
            })
            .catch((error) => {
                console.error('Service worker registration failed:', error);
            });
    });
}
