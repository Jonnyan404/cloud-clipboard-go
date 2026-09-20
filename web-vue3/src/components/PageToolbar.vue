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

const roomName = computed(() => ws.room || t('publicRoom'));
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
                <div class="page-toolbar__group">
                    <v-tooltip v-if="roomListEnabled" :text="t('roomList')" location="bottom">
                        <template v-slot:activator="{ props }">
                            <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openRoomBrowser && actions.openRoomBrowser()">
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

        <!-- 模式切换器摊在工具栏下沿，取代原来的「标准 ▾」下拉。
             六种界面模式是这个项目的主要卖点，藏在下拉后面新用户根本不知道有几档；
             摊开之后一眼看到全部六档，也省掉「点开再选」那一步。
             宽屏图标 + 文字、窄屏只留图标：六个带文字的 chip 在 375px 放不下，
             而横滑比下拉更不可发现。**同一份标记，纯 CSS 切换，不开第二套状态。** -->
        <div class="page-toolbar__modes">
            <button
                v-for="mode in MODES"
                :key="mode.key"
                type="button"
                class="page-toolbar__mode-chip"
                :class="{ 'page-toolbar__mode-chip--active': app.uiMode === mode.key }"
                :title="t(mode.labelKey)"
                :aria-current="app.uiMode === mode.key ? 'true' : undefined"
                @click="setMode(mode.key)"
            >
                <v-icon size="15">{{ mode.icon }}</v-icon>
                <span class="page-toolbar__mode-chip-label d-none d-sm-inline">{{ t(mode.labelKey) }}</span>
            </button>
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

/* 折叠开关一并收掉模式行：它是工具栏的一部分，不另开一个开关、也不另存一份状态 */
.page-toolbar--collapsed .page-toolbar__inner,
.page-toolbar--collapsed .page-toolbar__modes {
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

/* 清空是这条栏里唯一的破坏性动作，和「设置」长得一模一样不合适。
   平时不喧哗，悬停才变红。 */
.page-toolbar__clear:hover :deep(.v-icon) {
    color: rgb(var(--v-theme-error));
}

/* ── 模式切换器（工具栏下沿那一行）───────────────────────────────── */

/* 和工具栏共用底色（它就在同一个 .page-toolbar 里），只用一条发丝线分出上下两带。
   尺寸压到最小可用：这一行是常驻的，每多 1px 都是从内容区拿走的。 */
.page-toolbar__modes {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-wrap: wrap;
    gap: 4px;
    max-width: 1100px;
    margin: 0 auto;
    padding: 0 16px 4px;
    border-top: 1px solid rgba(148, 163, 184, 0.18);
}

.page-toolbar__mode-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: 1px solid transparent;
    background: transparent;
    border-radius: 999px;
    padding: 1px 8px;
    font-size: 0.7rem;
    line-height: 1.3;
    font-family: inherit;
    color: rgba(100, 116, 139, 0.95);
    cursor: pointer;
    white-space: nowrap;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
}

.page-toolbar__mode-chip:hover {
    background: rgba(148, 163, 184, 0.18);
}

/* 当前项：实心反白 —— 沿用原来那个下拉触发器 hover 时的蓝，视觉语言不另起一套。
   一行六个里只靠字重区分太弱，认不出哪个是当前的。 */
.page-toolbar__mode-chip--active {
    background: #1e88e5;
    border-color: #1e88e5;
    color: #fff;
    font-weight: 600;
}

.page-toolbar__mode-chip--active :deep(.v-icon) {
    color: #fff;
}

.page-toolbar--dark .page-toolbar__modes {
    border-top-color: rgba(148, 163, 184, 0.22);
}

.page-toolbar--dark .page-toolbar__mode-chip {
    color: rgba(203, 213, 225, 0.9);
}

.page-toolbar--dark .page-toolbar__mode-chip:hover {
    background: rgba(148, 163, 184, 0.2);
}

/* 深色下换成浅蓝底 + 深字：整块 #1e88e5 在深底上太重 */
.page-toolbar--dark .page-toolbar__mode-chip--active {
    background: #90caf9;
    border-color: #90caf9;
    color: #0d1117;
}

.page-toolbar--dark .page-toolbar__mode-chip--active :deep(.v-icon) {
    color: #0d1117;
}

@media (max-width: 600px) {
    .page-toolbar__room {
        flex: 0 1 auto;
        max-width: 100%;
    }
}
</style>