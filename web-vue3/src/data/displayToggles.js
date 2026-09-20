// 「个性化」显示开关的**唯一定义处**。
//
// 加一个新开关只要两步：这里加一行，消费点读 `app.display.<key>`。
// 设置面板里的开关列表会自动多出一项，存储结构也不用动 —— 每个模式的配置
// 本来就是一个对象，缺哪个键由 DEFAULT_DISPLAY 兜底。
//
// key 起得短（timestamp / device / ip）是因为它要出现在每个消费点里，
// 读起来是 `app.display.timestamp`，不是 `app.display.showTimestamp`。
//
// 开关会越来越多（目标量级是几十项），所以每一项都必须声明 `group` ——
// 设置面板按组渲染小节，否则一屏几十个开关没法看。
export const DISPLAY_GROUPS = [
    { key: 'meta', labelKey: 'displayGroupMeta' },
    { key: 'content', labelKey: 'displayGroupContent' },
    { key: 'composer', labelKey: 'displayGroupComposer' },
    { key: 'card', labelKey: 'displayGroupCard' },
];

// 图标类开关（composer / card 两组）只对标准模式有意义：
// 输入区是 UnifiedComposer、卡片是 received-item/*，两者都只被 DefaultMode 用。
// 所以整组都写 modes: ['default']，别的模式的面板里不会冒出拨了没反应的开关。
const ONLY_DEFAULT = ['default'];

export const DISPLAY_TOGGLES = [
    { key: 'timestamp', group: 'meta', labelKey: 'showTimestamp', icon: 'mdi-clock-outline' },
    { key: 'device', group: 'meta', labelKey: 'showDeviceInfo', icon: 'mdi-devices' },
    { key: 'ip', group: 'meta', labelKey: 'showSenderIP', icon: 'mdi-ip-network-outline' },
    // markdown 接在两个模式：标准（时间流卡片 + 文件预览）和便签（阅读器大视图）。
    // 必须声明 modes：个性化面板是「每模式一组开关」，不声明的话用户会在聊天/终端模式里
    // 拨到一个完全没反应的开关 —— 显示一个不生效的开关，比不显示它更糟。
    { key: 'markdown', group: 'content', labelKey: 'markdownToggle', icon: 'mdi-language-markdown-outline', modes: ['default', 'sticky'] },
    // 时间流上方的分类条（全部 / 文本 / 图片 / 文件）。
    // ⚠️ **这是「默认关」的例外**，下面 DEFAULT_DISPLAY 里那条「默认全开」的原则在这里**故意不适用**：
    // 分类条是可选的新浏览方式，不是原本就有的东西 —— 默认开会在所有老用户的时间流上
    // 凭空多一条横条。Jonny 明确要求默认关。
    // 只对标准模式：分类条只存在于 DefaultMode 的时间流上。
    { key: 'timelineFilter', group: 'content', labelKey: 'showTimelineFilter', icon: 'mdi-filter-variant', modes: ONLY_DEFAULT },
    // 时间流上方的搜索条。默认关，跟分类条同一条理由（新增的可选浏览方式，
    // 默认开会给老用户凭空加一条横条）。
    // ⚠️ 例外：**六个模式都把发送区关掉**（纯预览模式）时会自动出现，
    // 不依赖这个开关 —— 那种状态下没有发送动作，搜索才是主操作。
    { key: 'timelineSearch', group: 'content', labelKey: 'showTimelineSearch', icon: 'mdi-magnify', modes: ONLY_DEFAULT },

    // ── 输入区（标准模式输入框下方那排小图标）────────────────────────
    // 文案复用已有的 traditionalColors / shortcuts / toggleDarkMode / reward，
    // 不另造同义词。
    // 输入框右上角的「全屏」按钮不在这里 —— 它贴着输入框，属于输入区本身，
    // 不是那排可收的图标。
    { key: 'composerDevice', group: 'composer', labelKey: 'showComposerDevice', icon: 'mdi-laptop', modes: ONLY_DEFAULT },
    { key: 'composerSwap', group: 'composer', labelKey: 'showComposerSwap', icon: 'mdi-swap-vertical', modes: ONLY_DEFAULT },
    { key: 'composerPalette', group: 'composer', labelKey: 'traditionalColors', icon: 'mdi-palette-swatch', modes: ONLY_DEFAULT },
    { key: 'composerShortcuts', group: 'composer', labelKey: 'shortcuts', icon: 'mdi-flash', modes: ONLY_DEFAULT },
    { key: 'composerTheme', group: 'composer', labelKey: 'toggleDarkMode', icon: 'mdi-theme-light-dark', modes: ONLY_DEFAULT },
    { key: 'composerReward', group: 'composer', labelKey: 'reward', icon: 'mdi-currency-cny', modes: ONLY_DEFAULT },
    // 两个「整块关掉」的开关：文本输入框 / 上传文件。
    // 关掉整块是给「只想收、不想发」或「只用其中一种」的人用的 ——
    // 只关图标那排解决不了这个（图标底下还留着空输入框）。
    //
    // ⚠️ 这两个**故意不写 modes**，意思是「六个模式的面板里都要出现这一项」——
    // 不是「六个模式共用一份值」。存储始终是 displayByMode[模式][开关]，
    // 每个模式各存各的（见 store/app.js），在聊天里关掉不会影响标准模式。
    //
    // 为什么必须每个模式都出现：发送区有两套实现 ——
    // UnifiedComposer（标准模式）和 StickyComposer（其余五个模式），
    // 但「文本输入 / 上传文件」这两块两边都有，所以每一端都该能关。
    // 上面那排图标则相反：只有 UnifiedComposer 有，所以是 ONLY_DEFAULT。
    { key: 'composerText', group: 'composer', labelKey: 'showComposerText', icon: 'mdi-text-box-outline' },
    { key: 'composerUpload', group: 'composer', labelKey: 'showComposerUpload', icon: 'mdi-cloud-upload-outline' },

    // ── 卡片（时间流卡片右上角那排图标）──────────────────────────────
    // 文本卡片只有「复制」，文件卡片是「下载」；两者都有的（复制链接/二维码/删除）
    // 共用一个开关 —— 同一件事在两个组件里各有一个开关，用户没法预期。
    { key: 'cardDownload', group: 'card', labelKey: 'download', icon: 'mdi-download', modes: ONLY_DEFAULT },
    { key: 'cardPreview', group: 'card', labelKey: 'preview', icon: 'mdi-text-box-search-outline', modes: ONLY_DEFAULT },
    { key: 'cardCopy', group: 'card', labelKey: 'copyText', icon: 'mdi-content-copy', modes: ONLY_DEFAULT },
    { key: 'cardCopyLink', group: 'card', labelKey: 'copyLink', icon: 'mdi-link-variant', modes: ONLY_DEFAULT },
    { key: 'cardQr', group: 'card', labelKey: 'showQrCode', icon: 'mdi-qrcode', modes: ONLY_DEFAULT },
    { key: 'cardDelete', group: 'card', labelKey: 'delete', icon: 'mdi-close-circle-outline', modes: ONLY_DEFAULT },
];

// 某个分组下有哪些开关。加开关不用动这里。
export function togglesInGroup(groupKey) {
    return DISPLAY_TOGGLES.filter((toggle) => toggle.group === groupKey);
}

// 某个**模式**在某个分组下能用哪些开关。
// 没声明 `modes` 的开关对所有模式通用；声明了的只在列出的模式里出现。
export function togglesForMode(modeKey, groupKey) {
    return DISPLAY_TOGGLES.filter(
        (toggle) => toggle.group === groupKey && (!toggle.modes || toggle.modes.includes(modeKey)),
    );
}

// 出厂默认值（新用户、以及某个模式从没被单独配置过时用它）。
export const DEFAULT_DISPLAY = {
    timestamp: true,
    // 默认开：设备名是「这条消息从哪台设备发的」的唯一线索，尤其快捷指令这类
    // UA 认不出来的来源。想关的人可以在设置里按模式关掉。
    device: true,
    ip: false,
    // 默认开：实测过，启发式（looksLikeMarkdown）能把普通文本挡在外面 ——
    // `这是一句普通的话` 不会冒图标，`# hi` / `**hi**` / `- a` 才会。
    // 之前默认关，结果是用户写了 markdown 却以为功能没生效（开关藏在设置里，找不到）。
    markdown: true,
    // ⚠️ 上面那条「默认全开」的原则在这里**故意破例**：分类条是新增的可选浏览方式，
    // 默认开等于给所有老用户的时间流凭空加一条横条。Jonny 要求默认关。
    timelineFilter: false,
    // 同上：默认关，但纯预览模式下会自动出现。
    timelineSearch: false,
    // 输入区与卡片上的图标：**默认全开**。
    // 默认关等于「升级后功能消失」，用户根本不知道是设置里多了个开关；
    // 默认开则相反 —— 想清静的人自己去设置里关，找不到也不会觉得坏了什么。
    composerDevice: true,
    composerSwap: true,
    composerPalette: true,
    composerShortcuts: true,
    composerTheme: true,
    composerReward: true,
    // 默认开：跟上面那排图标同一条理由 —— 默认关等于「升级后功能消失」。
    composerText: true,
    composerUpload: true,
    cardDownload: true,
    cardPreview: true,
    cardCopy: true,
    cardCopyLink: true,
    cardQr: true,
    cardDelete: true,
};

// 老版本把三个开关存成三个全局 key，新结构是「每个模式一组」。
// 这个映射只用于一次性迁移，见 store/app.js。
export const LEGACY_STORAGE_KEYS = {
    timestamp: 'showTimestamp',
    device: 'showDeviceInfo',
    ip: 'showSenderIP',
};
