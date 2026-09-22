import { defineStore } from 'pinia';
import { DEFAULT_DISPLAY, DISPLAY_TOGGLES, LEGACY_STORAGE_KEYS } from '@/data/displayToggles';
import { MODES } from '@/views/modes/registry';
import { readLocationParam } from '@/util.js';

// 某个模式「没被用户单独配过」时的取值。
//
// 老版本把三个开关存成三个全局 key，升级后必须保证「看起来完全没变」——
// 所以有老值就用老值，没有才用出厂默认。只算一次：它纯粹是兜底，
// 用户一改就写进 displayByMode，之后不再回头看这里。
const INITIAL_DISPLAY = (() => {
    const initial = { ...DEFAULT_DISPLAY };
    for (const [key, storageKey] of Object.entries(LEGACY_STORAGE_KEYS)) {
        const stored = localStorage.getItem(storageKey);
        if (stored !== null) {
            initial[key] = stored === 'true';
            localStorage.removeItem(storageKey); // 迁完就删，免得两处状态打架
        }
    }
    return initial;
})();

// 只读回用户显式配置过的模式。坏数据当没有 —— 大不了回落到 INITIAL_DISPLAY。
// 另带一次性的 markdown 旧值迁移（见内）。
// 分享默认值：{ ttlMinutes, maxUses, password }。坏数据当没有，回落到出厂值。
// ⚠️ 这里**故意没有展示格式** —— 分享页自己带 raw↔md 切换，发送方再预设一次是多余的。
function loadShareDefaults() {
    const fallback = { ttlMinutes: 15, maxUses: 0, password: '' };
    try {
        const raw = localStorage.getItem('shareDefaults');
        if (!raw) return fallback;
        const parsed = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object') return fallback;
        return {
            ttlMinutes: Number.isFinite(Number(parsed.ttlMinutes)) ? Number(parsed.ttlMinutes) : fallback.ttlMinutes,
            maxUses: Number.isFinite(Number(parsed.maxUses)) ? Number(parsed.maxUses) : fallback.maxUses,
            // 密码也存下来：不弹窗时要用它，否则「关了弹窗就没法设密码」。
            password: typeof parsed.password === 'string' ? parsed.password : fallback.password,
        };
    } catch {
        return fallback;
    }
}

function loadDisplayByMode() {
    const raw = localStorage.getItem('displayByMode');
    if (!raw) return {};
    try {
        const parsed = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object') return {};
        // markdown 开关曾经只在 default 模式露面（modes:['default']），便签等模式的
        // 个性化面板里根本没有这个开关 —— 所以那些模式下存的 markdown:false 不可能是
        // 用户亲手关的，只能是「拨别的开关时把旧默认值(false)顺手存进去」的陈旧值。
        // 现在默认值已翻成 true，清掉这些陈旧 false 让新默认生效；
        // 只清一次（打版本戳），之后用户亲手关的 false 原样尊重。
        // default 模式不动：那里的开关一直可见，存的 false 可能是用户本意。
        if (localStorage.getItem('displayByModeMigrated') !== '1') {
            for (const [mode, cfg] of Object.entries(parsed)) {
                if (mode !== 'default' && cfg && typeof cfg === 'object' && cfg.markdown === false) {
                    delete cfg.markdown;
                }
            }
            localStorage.setItem('displayByMode', JSON.stringify(parsed));
            localStorage.setItem('displayByModeMigrated', '1');
        }
        return parsed;
    } catch {
        return {};
    }
}

export const useAppStore = defineStore('app', {
    state: () => ({
        dark: null,
        config: {
            version: '',
            server: { history: 0, prefix: '', roomList: false },
            text: { limit: 0 },
            file: { expire: 0, chunk: 0, limit: 0 },
        },
        send: {
            text: '',
            files: [],
        },
        received: [],
        roomMessagesCache: {},
        isRoomSyncing: false,
        device: [],
        // 每个界面模式一组显示开关，键是模式 key（default / chat / sticky / ...）。
        // 只存用户**改过**的模式，没配过的由 display getter 兜底。
        displayByMode: loadDisplayByMode(),
        // 纯预览模式的搜索词（见 getters 里的说明）
        searchQuery: '',
        // 分享的默认参数。**不是显示开关**（那套只存布尔），所以单独一份。
        // 关掉「分享弹窗」开关后，建链接直接用这里的值。
        shareDefaults: loadShareDefaults(),
        composerPrimary: localStorage.getItem('composerPrimary') || 'text',
        fullscreenSendClose: localStorage.getItem('fullscreenSendClose') !== null
            ? localStorage.getItem('fullscreenSendClose') === 'true'
            : true,
        // 界面模式：**地址里的 `?mode=` 优先**，其次是上次用过的，最后是标准模式。
        // 这样「一个 tab 一个模式」——每个 tab 有自己的地址，就互不干扰。
        // 房间本来就是这么做的（只走 URL、不落 localStorage），模式跟它保持一致。
        uiMode: readLocationParam('mode') || localStorage.getItem('uiMode') || 'default',
    }),
    actions: {
        setSearchQuery(value) {
            this.searchQuery = String(value || '');
        },
        // 分享默认值：改完立刻落盘，不弹窗时要用
        setShareDefaults(next) {
            this.shareDefaults = { ...this.shareDefaults, ...(next || {}) };
            try {
                localStorage.setItem('shareDefaults', JSON.stringify(this.shareDefaults));
            } catch { /* 存不下就算了，内存里仍然生效 */ }
        },
        setConfig(config) {
            this.config = config;
        },
        toggleComposerPrimary() {
            this.composerPrimary = this.composerPrimary === 'files' ? 'text' : 'files';
            localStorage.setItem('composerPrimary', this.composerPrimary);
        },
        toggleFullscreenSendClose() {
            this.fullscreenSendClose = !this.fullscreenSendClose;
            localStorage.setItem('fullscreenSendClose', String(this.fullscreenSendClose));
        },
        setUiMode(mode) {

            // 搜索条只存在于标准模式；不在这里清掉的话，切到别的模式会看到

            // 一份被悄悄过滤过的列表，而那个模式里根本没有搜索框可以清。

            this.searchQuery = '';

            this.uiMode = mode;
            localStorage.setItem('uiMode', mode);
        },
        // 拨动的是**当前模式**的开关。持久化写在 action 里（跟 uiMode / composerPrimary
        // 保持同一个写法），别让持久化分散到模板或 $subscribe 里去。
        setDisplayToggle(key, value) {
            const next = {
                ...this.displayByMode,
                [this.uiMode]: { ...this.display, [key]: value },
            };
            this.displayByMode = next;
            localStorage.setItem('displayByMode', JSON.stringify(next));
        },
    },
    getters: {
        // ⚠️ 这里**不要**再写一个 `searchQuery: (state) => state.searchQuery` 的 getter。
        // state 里的字段本来就能直接读（app.searchQuery），多加一个同名 getter 会把它挡住，
        // action 里 `this.searchQuery = ...` 的赋值就落不到 state 上 —— 搜索词永远是空，
        // 表现为「搜索框能输入但列表不过滤」。踩过。

        // 搜索过滤后的内容列表。**各模式渲染列表都取这个，不要直接取 received** ——
        // 否则搜索框只在部分模式生效，看着像坏了。
        // 计数、空态判断仍然用 received（那是「房间里有多少内容」，与搜索无关）。
        visibleReceived() {
            const q = String(this.searchQuery || '').trim().toLowerCase();
            if (!q) return this.received;
            return this.received.filter((item) => {
                const haystack = item.type === 'text'
                    ? String(item.content || '')
                    : String(item.name || '');
                return haystack.toLowerCase().includes(q);
            });
        },

        // 六个模式**全都**把文本区与上传区关掉了 —— 即「纯预览模式」。
        // 两个开关的出厂默认都是 true，所以必须每个模式都显式关掉才算成立。
        // 用 DEFAULT_DISPLAY 兜底：没配过的模式取默认值（true），于是不算关。
        // 输入区**什么都不剩**了：文本区、上传区、以及 composer 那一整组图标全关。
        // 这时候只剩一个空外框，该把整个输入区一起藏掉。
        // ⚠️ 只是图标全关、输入框还在时**不该**藏 —— 框是输入框的容器。
        // 图标键从注册表现推，别手写一份：以后加图标开关会自动算进来。
        composerFullyHidden() {
            const d = this.display;
            if (d.composerText || d.composerUpload) return false;
            const iconKeys = DISPLAY_TOGGLES
                .filter((x) => x.group === 'composer' && x.key !== 'composerText' && x.key !== 'composerUpload')
                .map((x) => x.key);
            return !iconKeys.some((k) => d[k]);
        },

        composerDisabledEverywhere() {
            // ⚠️ 模式列表**在 getter 里现取**，不要在模块顶层算成常量：
            // store ← registry ← 各模式 ← store 是个循环，顶层求值可能拿到还没初始化的 MODES。
            return MODES.map((m) => m.key).every((mode) => {
                const cfg = this.displayByMode[mode] || {};
                const off = (key) => (key in cfg ? cfg[key] : DEFAULT_DISPLAY[key]) === false;
                return off('composerText') && off('composerUpload');
            });
        },

        // 当前模式的显示开关。缺的键用 INITIAL_DISPLAY 兜底 —— 这样加新开关时，
        // 老用户不用迁移就能拿到默认值。
        display() {
            return { ...INITIAL_DISPLAY, ...(this.displayByMode[this.uiMode] || {}) };
        },
        useDark() {
            switch (this.dark) {
                case 'time': {
                    const hour = new Date().getHours();
                    return hour < 7 || hour >= 19;
                }
                case 'prefer':
                    return window.matchMedia('(prefers-color-scheme: dark)').matches;
                case 'enable':
                    return true;
                case 'disable':
                    return false;
                default:
                    return false;
            }
        },
    },
});
