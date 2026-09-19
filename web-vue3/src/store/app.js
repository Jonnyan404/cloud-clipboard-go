import { defineStore } from 'pinia';
import { DEFAULT_DISPLAY, LEGACY_STORAGE_KEYS } from '@/data/displayToggles';

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
        composerPrimary: localStorage.getItem('composerPrimary') || 'text',
        fullscreenSendClose: localStorage.getItem('fullscreenSendClose') !== null
            ? localStorage.getItem('fullscreenSendClose') === 'true'
            : true,
        uiMode: localStorage.getItem('uiMode') || 'default',
    }),
    actions: {
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
