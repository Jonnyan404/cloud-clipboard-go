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
];

export const DISPLAY_TOGGLES = [
    { key: 'timestamp', group: 'meta', labelKey: 'showTimestamp', icon: 'mdi-clock-outline' },
    { key: 'device', group: 'meta', labelKey: 'showDeviceInfo', icon: 'mdi-devices' },
    { key: 'ip', group: 'meta', labelKey: 'showSenderIP', icon: 'mdi-ip-network-outline' },
    // markdown 接在两个模式：标准（时间流卡片 + 文件预览）和便签（阅读器大视图）。
    // 必须声明 modes：个性化面板是「每模式一组开关」，不声明的话用户会在聊天/终端模式里
    // 拨到一个完全没反应的开关 —— 显示一个不生效的开关，比不显示它更糟。
    { key: 'markdown', group: 'content', labelKey: 'markdownToggle', icon: 'mdi-language-markdown-outline', modes: ['default', 'sticky'] },
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
};

// 老版本把三个开关存成三个全局 key，新结构是「每个模式一组」。
// 这个映射只用于一次性迁移，见 store/app.js。
export const LEGACY_STORAGE_KEYS = {
    timestamp: 'showTimestamp',
    device: 'showDeviceInfo',
    ip: 'showSenderIP',
};
