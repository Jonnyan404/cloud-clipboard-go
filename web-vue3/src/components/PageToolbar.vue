<script setup>import { computed, inject, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { MODES } from '@/views/modes/registry.js';

const props = defineProps({
    variant: { type: String, default: 'default' },
});

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

const toolbarCollapsed = ref(localStorage.getItem('pageToolbarCollapsed') === 'true');

function toggleToolbar() {
    toolbarCollapsed.value = !toolbarCollapsed.value;
    localStorage.setItem('pageToolbarCollapsed', String(toolbarCollapsed.value));
}

const actions = inject('pageToolbarActions', {});

const roomCount = computed(() => Number(actions.roomCount?.value ?? actions.roomCount ?? 0));
const roomListEnabled = computed(() => Boolean(actions.roomListEnabled?.value ?? actions.roomListEnabled));
const roomBrowserVisible = computed(() => Boolean(actions.roomBrowserVisible?.value ?? actions.roomBrowserVisible));

const roomName = computed(() => ws.room || t('publicRoom'));

// 房间 chip 上的锁：🔒 = 进这个房间要密码，🌍 = 公开。
//
// 数据来自 `roomProtectionCache`，而它由 `fetchServerInfo` 用 `/server?room=` 的
// `roomProtected` 填充。**那个字段的含义是「这个房间实际要不要密码」**，不是
// 「roomAuth 里有没有配这一项」—— 显式 `{open: true}` 的房间有配置项但不要密码，
// 「有全局密码、没配过房间条目」的房间没配置项却要密码。服务端那边由
// resolveRoomAuth(...).Required 算（lib/handler.go，有 server_room_protected_test.go 钉着）。
//
// ⚠️ 缓存是**三态**的：true / false / undefined（还没问过服务端）。
// 这里用 Boolean() 把 undefined 归成 false，也就是「未知」先按公开画 ——
// 窗口是一次 /server 往返（connect() 里必发，见 store/websocket.js），所以只会闪一下；
// 别把它当成「已经确认公开」的信号用。
const isProtected = computed(() => Boolean(ws.roomProtectionCache?.[ws.normalizeRoomName(ws.room)]));
const latencyValue = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    return `${Math.round(ws.latency)} ms`;
});
const latencyHexColor = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    let colorName = 'success';
    if (ws.latency >= 60 && ws.latency < 120) {
        colorName = 'warning';
    } else if (ws.latency >= 120) {
        colorName = 'error';
    }
    const themeColors = theme.themes.value[isDark.value ? 'dark' : 'light'].colors;
    return themeColors[colorName] || colorName;
});

function setMode(mode) {
    app.setUiMode(mode);
}

// 定时自动化管理页的入口。
//
// ⚠️ **同一个标签页内跳转，不开新窗口**。凭据存在 sessionStorage，而 sessionStorage
// 是按标签页隔离的 —— 开新标签页等于让用户再登一次，正好把「免二次登录」这件事废掉。
// 同标签页跳过去则直接可用，浏览器返回键就回到这里（那边也有一个「返回主界面」）。
const automationUrl = computed(() => {
    const raw = String(app.config?.server?.prefix || '').trim().replace(/^\/+|\/+$/g, '');
    const prefix = raw ? `/${raw}` : '';
    const room = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    return `${prefix}/automation${room}`;
});

// 这个入口**按服务端的能力声明显示**，不能无条件渲染。
//
// ⚠️ Cloudflare Worker 部署里根本没有这一族接口（`/tasks`、`/automation` 都不存在，
// `/server` 也不下发 `automation`）。无条件渲染的话，点下去会被 Worker 末尾那条
// `router.all('*', handleFallback)` 当成 SPA 导航兜底掉 —— 用户看到的是
// 「点了定时任务、回到了首页」，和当初被 Service Worker 吞掉那次是同一种症状
// （那次是 denylist 漏了，见 vite.config.js 里那段注释）。
//
// ⚠️ 判 `=== true` 而不是 `!== false`：`app.config` 要等一次 /server 往返才有，
// 在此之前 `automation` 是 undefined。这里按「未知就先不显示」处理 ——
// 代价是工具栏这一格晚一拍出现（和 chip 上那把锁是同一个窗口），
// 而反过来（未知先显示）会在 Worker 部署下先露出一个点了没用的按钮。
//
// 服务端那边 `automation.enabled` 由 `AutomationCapability` 下发（lib/handler.go），
// 它同时受全局开关 `automation.enabled` 约束 —— 所以配置里关掉之后这个图标也会消失。
const automationEnabled = computed(() => app.config?.automation?.enabled === true);

const currentMode = computed(() => MODES.find(mode => mode.key === app.uiMode) || MODES[0]);

// 下拉菜单分两组：正常模式在上，「即将下架」的折到分割线下面。
//
// ⚠️ 判断只读 registry 里的 `deprecated` 字段，**别在这里写死一份 key 列表** ——
// 个性化面板（App.vue）读的是同一个字段，写死两份迟早会漂。
const activeModes = computed(() => MODES.filter((mode) => !mode.deprecated));
const deprecatedModes = computed(() => MODES.filter((mode) => mode.deprecated));

// 「即将下架」那一组的投票入口。
//
// 为什么放在这个菜单里、而不是设置页或关于页：会点开这个分组的人，正是**还在用这几个模式的人**
// —— 他们才是该被问的人。放到别处，看到问卷的是另一批（根本没用过这些模式的）用户。
const DEPRECATED_VOTE_URL = 'https://wj.qq.com/s2/28003735/h3fa/';
</script>

<template>
    <div
        class="page-toolbar"
        :class="[
            `page-toolbar--${variant}`,
            { 'page-toolbar--dark': isDark },
            { 'page-toolbar--collapsed': toolbarCollapsed }
        ]"
    >
        <div class="page-toolbar__inner" v-show="!toolbarCollapsed">
            <div class="page-toolbar__leading">
                <v-tooltip v-if="ws.room" :text="t('backToDefaultRoom')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" class="page-toolbar__home-btn" v-bind="props" @click="ws.switchRoom('')">
                            <v-icon size="24">mdi-home-outline</v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>

                <!-- 连接图标只在「没连上」时出现。
                     连上之后它是个**死按钮**：toggleConnection 只在断开时重连，
                     而「连接好不好」已经由房间 chip 里的延迟数字（还带颜色分级）表达了 ——
                     常驻就只是白占一个位置、还让人以为点了有用。
                     断开时给 error 色：这是异常态，不是常态装饰。 -->
                <v-tooltip
                    v-if="!ws.websocket"
                    :text="ws.websocketConnecting ? t('connecting') : t('disconnected')"
                    location="bottom"
                >
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.toggleConnection && actions.toggleConnection()">
                            <v-icon size="24" :color="ws.websocketConnecting ? undefined : 'error'">
                                {{ ws.websocketConnecting ? 'mdi-lan-pending' : 'mdi-lan-disconnect' }}
                            </v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>

                <v-chip
                    size="small"
                    variant="tonal"
                    :color="variant === 'sticky' ? 'amber-darken-1' : variant === 'terminal' ? 'success' : 'primary'"
                    class="page-toolbar__room"
                    :title="t('showQrCode')"
                    @click="actions.openPageQr && actions.openPageQr()"
                >
                    <v-icon start size="x-small">
                        {{ isProtected ? 'mdi-lock' : 'mdi-earth' }}
                    </v-icon>
                    <span v-if="ws.room" class="page-toolbar__roomname">{{ roomName }}</span>
                    <span v-else>{{ roomName }}</span>
                    <span
                        v-if="ws.websocket && ws.latency !== null"
                        class="page-toolbar__latency"
                        :style="{ color: latencyHexColor }"
                    >
                        {{ latencyValue }}
                    </span>
                </v-chip>
            </div>

            <div class="page-toolbar__actions">
                <v-menu location="bottom end" min-width="192" :close-on-content-click="true">
                    <template v-slot:activator="{ props: menuProps }">
                        <button
                            v-bind="menuProps"
                            class="page-toolbar__mode"
                            :title="t('uiMode')"
                        >
                            <!-- 菜单栏里只留文字：旁边那几个都是纯图标按钮，这里放个图标反而
                                 多一层信息（而且 6 个模式图标挤在一起时辨识度本来就低）。
                                 图标留到下拉里，那里有文字并排，认得出。
                                 文字不能再挂 d-none d-sm-inline —— 图标去掉后它是唯一内容，
                                 窄屏藏了就只剩一个箭头。 -->
                            <span class="page-toolbar__mode-label">{{ t(currentMode.labelKey) }}</span>
                            <v-icon size="x-small" class="page-toolbar__mode-caret">mdi-chevron-down</v-icon>
                        </button>
                    </template>
                    <v-list density="compact" nav>
                        <v-list-item
                            v-for="mode in activeModes"
                            :key="mode.key"
                            :active="app.uiMode === mode.key"
                            @click="setMode(mode.key)"
                        >
                            <template v-slot:prepend>
                                <v-icon size="small">{{ mode.icon }}</v-icon>
                            </template>
                            <v-list-item-title>{{ t(mode.labelKey) }}</v-list-item-title>
                        </v-list-item>

                        <!-- 即将下架的那几个：**仍然完全可用**，只是不再推荐。
                             只弱化视觉（文字变灰 + 折到分割线下面），**不拦点击、不弹确认** ——
                             会点它们的人就是想用一下，弹个框只会打断他，而他并没做错什么。 -->
                        <template v-if="deprecatedModes.length">
                            <v-divider class="my-1"></v-divider>
                            <v-list-subheader class="page-toolbar__mode-deprecated">
                                {{ t('uiModeDeprecated') }}
                            </v-list-subheader>
                            <v-list-item
                                v-for="mode in deprecatedModes"
                                :key="mode.key"
                                :active="app.uiMode === mode.key"
                                @click="setMode(mode.key)"
                            >
                                <template v-slot:prepend>
                                    <v-icon size="small">{{ mode.icon }}</v-icon>
                                </template>
                                <v-list-item-title class="text-medium-emphasis">{{ t(mode.labelKey) }}</v-list-item-title>
                            </v-list-item>

                            <!-- 投票入口：外链、新窗口打开（别把用户从正在用的界面上带走）。
                                 ⚠️ rel="noopener noreferrer" 不能省 —— target=_blank 打开的同源页面
                                 能通过 window.opener 反向操作本页。 -->
                            <v-list-item
                                :href="DEPRECATED_VOTE_URL"
                                target="_blank"
                                rel="noopener noreferrer"
                                class="page-toolbar__vote"
                            >
                                <template v-slot:prepend>
                                    <v-icon size="small" color="primary">mdi-vote-outline</v-icon>
                                </template>
                                <v-list-item-title class="page-toolbar__vote-title">
                                    {{ t('uiModeDeprecatedVote') }}
                                </v-list-item-title>
                                <v-list-item-subtitle class="page-toolbar__vote-hint">
                                    {{ t('uiModeDeprecatedVoteHint') }}
                                </v-list-item-subtitle>
                                <template v-slot:append>
                                    <v-icon size="x-small" class="text-medium-emphasis">mdi-open-in-new</v-icon>
                                </template>
                            </v-list-item>
                        </template>
                    </v-list>
                </v-menu>

                <div class="page-toolbar__group">
                <!-- 这个按钮现在是房间侧栏的开关（原来只能「开」，关在侧栏头部那个 ✕ 上）。
                     开着时必须看得出来 —— 否则「已开启」和「点它能关」都读不出来。 -->
                <v-tooltip v-if="roomListEnabled" :text="roomBrowserVisible ? t('hideRoomBrowser') : t('showRoomBrowser')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn
                            icon
                            density="compact"
                            size="small"
                            variant="text"
                            class="page-toolbar__icon"
                            :class="{ 'page-toolbar__icon--active': roomBrowserVisible }"
                            v-bind="props"
                            @click="actions.openRoomBrowser && actions.openRoomBrowser()"
                        >
                            <v-badge :content="roomCount" :model-value="roomCount > 0" color="accent" overlap>
                                <v-icon size="24">mdi-view-list</v-icon>
                            </v-badge>
                        </v-btn>
                    </template>
                </v-tooltip>

                    <v-tooltip :text="t('enterRoom')" location="bottom">
                        <template v-slot:activator="{ props }">
                            <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openRoomDialog && actions.openRoomDialog()">
                                <v-icon size="24">mdi-door-open</v-icon>
                            </v-btn>
                        </template>
                    </v-tooltip>
                </div>

                <div class="page-toolbar__group">
                    <v-tooltip :text="t('clearClipboard')" location="bottom">
                        <template v-slot:activator="{ props }">
                            <v-btn icon density="compact" size="small" variant="text" class="page-toolbar__clear" v-bind="props" @click="actions.openClearAll && actions.openClearAll()">
                                <v-icon size="24">mdi-broom</v-icon>
                            </v-btn>
                        </template>
                    </v-tooltip>

                    <v-tooltip v-if="automationEnabled" :text="t('automationEntryHint')" location="bottom">
                        <template v-slot:activator="{ props }">
                            <v-btn
                                icon
                                density="compact"
                                size="small"
                                variant="text"
                                class="page-toolbar__icon"
                                v-bind="props"
                                :href="automationUrl"
                                :aria-label="t('automationEntry')"
                            >
                                <v-icon size="24">mdi-calendar-clock</v-icon>
                            </v-btn>
                        </template>
                    </v-tooltip>

                    <v-tooltip :text="t('settings')" location="bottom">
                        <template v-slot:activator="{ props }">
                            <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openSettings && actions.openSettings()">
                                <v-icon size="24">mdi-cog</v-icon>
                            </v-btn>
                        </template>
                    </v-tooltip>
                </div>
            </div>
        </div>

        <button
            type="button"
            class="page-toolbar__collapse-toggle"
            :class="{ 'page-toolbar__collapse-toggle--opened': !toolbarCollapsed }"
            :title="toolbarCollapsed ? t('expandToolbar') : t('collapseToolbar')"
            @click="toggleToolbar"
        >
            <v-icon size="small">{{ toolbarCollapsed ? 'mdi-chevron-double-down' : 'mdi-chevron-double-up' }}</v-icon>
        </button>
    </div>
</template>

<style scoped>
.page-toolbar {
    position: sticky;
    top: 0;
    z-index: 40;
}

.page-toolbar--collapsed .page-toolbar__inner {
    display: none !important;
}

.page-toolbar__collapse-toggle {
    position: absolute;
    left: 50%;
    bottom: -2px;
    transform: translateX(-50%);
    z-index: 5;
    width: 26px;
    height: 4px;
    border-radius: 999px;
    border: none;
    background: currentColor;
    opacity: 0.35;
    cursor: pointer;
    padding: 0;
    transition: opacity 0.15s, width 0.15s;
}

.page-toolbar__collapse-toggle:hover {
    opacity: 0.8;
    width: 36px;
}

.page-toolbar__collapse-toggle .v-icon {
    display: none;
}

.page-toolbar--dark .page-toolbar__collapse-toggle {
    background: currentColor;
    opacity: 0.3;
}

.page-toolbar--default {
    background: #f5f7fa;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.page-toolbar--sticky {
    background: #f3ead2;
    border-bottom: 1px solid rgba(120, 90, 40, 0.16);
}

.page-toolbar--mega {
    background: #fdfdfb;
    border-bottom: 1px solid rgba(17, 24, 39, 0.12);
}

.page-toolbar--terminal {
    background: #ffffff;
    border-bottom: 1px solid #d0d7de;
}

.page-toolbar--dark.page-toolbar--default {
    background: #1e1e24;
    border-bottom-color: rgba(148, 163, 184, 0.22);
}

.page-toolbar--dark.page-toolbar--sticky {
    background: #211d12;
    border-bottom-color: rgba(238, 232, 214, 0.12);
}

.page-toolbar--dark.page-toolbar--mega {
    background: #101318;
    border-bottom-color: rgba(255, 255, 255, 0.1);
}

.page-toolbar--terminal.page-toolbar--dark {
    background: #0d1117;
    border-bottom-color: #21262d;
}

.page-toolbar--workbench {
    background: #eef0f4;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.page-toolbar--dark.page-toolbar--workbench {
    background: #13161c;
    border-bottom-color: rgba(255, 255, 255, 0.08);
}

.page-toolbar--chat {
    background: #f6f7fa;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.page-toolbar--dark.page-toolbar--chat {
    background: #15171c;
    border-bottom-color: rgba(255, 255, 255, 0.08);
}

.page-toolbar__inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    max-width: 1100px;
    margin: 0 auto;
    padding: 8px 16px;
    flex-wrap: nowrap;
}

.page-toolbar__leading {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 auto;
    min-width: 0;
}

.page-toolbar__home-btn {
    flex-shrink: 0;
}

.page-toolbar__room {
    cursor: pointer;
    flex: 0 1 auto;
    min-width: 0;
    max-width: 46%;
}

.page-toolbar__room :deep(.v-chip__content) {
    min-width: 0;
}

.page-toolbar__room :deep(.v-chip__content > span) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
}

.page-toolbar__roomname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
}

.page-toolbar__latency {
    font-size: 0.7rem;
    font-weight: 700;
    margin-left: 6px;
    flex-shrink: 0;
}

.page-toolbar__room :deep(.v-icon) {
    flex-shrink: 0;
}

/* 右侧按语义分三段：视图（模式）/ 房间 / 系统。
   组内贴紧、组间留空档 —— 用间距而不是竖线分隔符：这条栏本来就很密，
   再加一种视觉元素只会更吵。
   ⚠️ 不要靠「图标大小」分主次：差 2px 眼睛看不出来，只会显得没对齐。
   层级交给分组间距和悬停色表达。 */
.page-toolbar__actions {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-shrink: 0;
    min-width: 0;
}

.page-toolbar__group {
    display: flex;
    align-items: center;
    gap: 2px;
}

/* 开关类图标的「已开启」态（目前是房间侧栏那一个）。用和模式触发器同一支蓝，
   整套工具栏只有这一支强调色。 */
.page-toolbar__icon--active {
    background: rgba(30, 136, 229, 0.16);
}

.page-toolbar__icon--active :deep(.v-icon) {
    color: #1e88e5;
}

.page-toolbar--dark .page-toolbar__icon--active {
    background: rgba(144, 202, 249, 0.2);
}

.page-toolbar--dark .page-toolbar__icon--active :deep(.v-icon) {
    color: #90caf9;
}

/* 清空是这条栏里唯一的破坏性动作，和「设置」长得一模一样不合适。
   平时不喧哗，悬停才变红。 */
.page-toolbar__clear:hover :deep(.v-icon) {
    color: rgb(var(--v-theme-error));
}

/* 模式触发器。改前是「半透明白底 + 淡边框 + 悬停整块变实心蓝」，两个毛病：
   1) 白底淡边框让它读起来像状态标签，跟左边的房间 chip 撞脸，看不出是个控件；
   2) 悬停直接变实心蓝，跟旁边那几个图标按钮的轻悬停不是一套语言。
   现在跟图标按钮统一：无边框、静止一层中性底色、悬停只加深一点。
/* 投票入口：一条虚线分隔 + 主色标题，让它在一列灰扑扑的「即将下架」模式名里
   显得是「另一类东西，而且可点」。 */
.page-toolbar__vote {
    margin-top: 2px;
    border-top: 1px dashed rgba(148, 163, 184, 0.5);
}

.page-toolbar__vote-title {
    /* ⚠️ v-list-item-title 默认 nowrap + 省略号 —— 不改成 normal 的话，
       「这些模式该不该下架」会被截成「这些模式该不…」，那问卷就白放了。 */
    white-space: normal;
    font-size: 0.8125rem;
    color: rgb(var(--v-theme-primary));
}

.page-toolbar__vote-hint {
    white-space: normal;
    font-size: 0.6875rem;
    line-height: 1.4;
}

/* 模式选择器。几条取舍（第一版踩过）：
   1) 白底淡边框让它读起来像状态标签，跟左边的房间 chip 撞脸，看不出是个控件；
   2) 悬停直接变实心蓝，跟旁边那几个图标按钮的轻悬停不是一套语言。
   现在跟图标按钮统一：无边框、静止一层中性底色、悬停只加深一点。
   胶囊形状保留 —— 它是「选择器」，胶囊比圆角方块更能说明这件事。 */
.page-toolbar__mode {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border: none;
    background: rgba(148, 163, 184, 0.14);
    border-radius: 999px;
    padding: 3px 10px;
    font-size: 0.78rem;
    line-height: 1.3;
    font-family: inherit;
    cursor: pointer;
    color: rgba(71, 85, 105, 0.95);
    transition: background 0.15s, color 0.15s;
}

.page-toolbar__mode:hover {
    background: rgba(148, 163, 184, 0.26);
}

.page-toolbar--dark .page-toolbar__mode {
    background: rgba(148, 163, 184, 0.16);
    color: rgba(203, 213, 225, 0.92);
}

.page-toolbar--dark .page-toolbar__mode:hover {
    background: rgba(148, 163, 184, 0.28);
}

.page-toolbar__mode-caret {
    opacity: 0.7;
}

@media (max-width: 600px) {
    .page-toolbar__room {
        flex: 0 1 auto;
        max-width: 100%;
    }
}
</style>