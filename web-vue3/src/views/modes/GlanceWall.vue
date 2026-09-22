<script setup>
// 速览模式：**查阅**用的视图。左边一列条目，右边是选中那一条的完整预览。
//
// 布局照 PixPin 的「超级剪贴板」面板：搜索在头部（占位符里带总数）、分类是文字 tab、
// 条目左侧一条时间栏、右侧常驻预览。
//
// 为什么是「两栏主从」而不是「单列时间流」（我第一版的做法）：
//   单列里看一条完整内容，要么展开卡片（把列表顶下去）、要么开弹窗（盖住列表）——
//   两种都要「离开列表」。主从布局里选中即预览，**列表和内容同时在场**，
//   这才是「查阅」。列表那一列只放一行摘要，扫起来也快。
//
// 复用而非新造：分类与搜索和标准模式共用（同一个 localStorage 键、同一个
// `app.visibleReceived`）；条目正文的渲染在 GlancePreview 里（宽屏是右侧面板，
// 窄屏是全屏弹窗，两边共用一份）。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { isImageName, looksLikeTable, looksLikeTaskList } from '@/util.js';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import PageToolbar from '@/components/PageToolbar.vue';
import GlancePreview from '@/components/glance/GlancePreview.vue';
import { useLocalRooms } from '@/composables/useLocalRooms.js';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

// 房间：**本地管理**的那一套（和工作台共用），不受服务端 `roomList` 控制。
const { localRooms, createRoom: createLocalRoom, removeRoom, switchToRoom } = useLocalRooms();
const currentRoom = computed(() => ws.normalizeRoomName(ws.room));
const roomLabel = computed(() => (currentRoom.value ? currentRoom.value : t('publicRoom')));
const newRoomDialog = ref(false);
const newRoomName = ref('');

function openNewRoomDialog() {
    newRoomName.value = '';
    newRoomDialog.value = true;
}

function createRoom() {
    if (createLocalRoom(newRoomName.value)) {
        newRoomDialog.value = false;
    }
}

// 分类过滤。和标准模式**共用同一个 localStorage 键** —— 它们是同一个概念
// （「我在看哪一类内容」）。
const TIMELINE_FILTER_KEY = 'timelineFilter';
const TIMELINE_FILTER_KEYS = ['all', 'text', 'image', 'file', 'task', 'table'];
const storedFilter = localStorage.getItem(TIMELINE_FILTER_KEY);
const timelineFilter = ref(TIMELINE_FILTER_KEYS.includes(storedFilter) ? storedFilter : 'all');
function setTimelineFilter(key) {
    timelineFilter.value = key;
    localStorage.setItem(TIMELINE_FILTER_KEY, key);
}

// 「任务列表」「表格」是**文本条目里的 markdown 结构**，不是新的条目类型 ——
// 所以它们排在三个类型之后：前三格回答「是什么」，后两格回答「里面有什么结构」。
const FILTER_OPTIONS = [
    { key: 'all', labelKey: 'filterAll' },
    { key: 'text', labelKey: 'filterText' },
    { key: 'image', labelKey: 'filterImage' },
    { key: 'file', labelKey: 'filterFile' },
    { key: 'task', labelKey: 'filterTaskList' },
    { key: 'table', labelKey: 'filterTable' },
];

// tab 的选中态。⚠️ 这和我第一版「只有正在生效的条件才高亮」不同 ——
// tab 是**导航**，没有选中态就不成其为 tab（参考图里「全部」也是加粗的）。
const filterActive = (key) => timelineFilter.value === key;

const searchActive = computed(() => Boolean(String(app.searchQuery || '').trim()));
// 搜索框的占位符里带总数（参考图是「检索 971 条剪贴板历史」）——
// 「一共有多少条」是免费的信息，而且它让搜索框一眼可读。
const searchPlaceholder = computed(() => t('glanceSearchPlaceholder', { count: app.received.length }));

const filteredReceived = computed(() => {
    const list = app.visibleReceived;
    if (timelineFilter.value === 'all') return list;
    if (timelineFilter.value === 'text') return list.filter((item) => item.type === 'text');
    if (timelineFilter.value === 'task') {
        return list.filter((item) => item.type === 'text' && looksLikeTaskList(item.content));
    }
    if (timelineFilter.value === 'table') {
        return list.filter((item) => item.type === 'text' && looksLikeTable(item.content));
    }
    // 「图片」= 文件条目里文件名是图片扩展名的那些；文本条目永远只归「文本」。
    // 判型用 util 的 isImageName（全站唯一实现），别在这里再抄一份正则。
    const wantImage = timelineFilter.value === 'image';
    return list.filter((item) => item.type === 'file' && isImageName(item.name) === wantImage);
});

// 每一类的条数。一次遍历算完六个数，别对每个分类各 filter 一遍 —— 那是六趟。
const filterCounts = computed(() => {
    const list = app.visibleReceived;
    const counts = { all: list.length, text: 0, image: 0, file: 0, task: 0, table: 0 };
    for (const item of list) {
        if (item.type === 'text') {
            counts.text += 1;
            if (looksLikeTaskList(item.content)) counts.task += 1;
            if (looksLikeTable(item.content)) counts.table += 1;
        } else if (item.type === 'file') {
            if (isImageName(item.name)) counts.image += 1;
            else counts.file += 1;
        }
    }
    return counts;
});

// 左侧时间栏。今天显示**时刻**，更早显示**日期标签**（昨天 / M/D）——
// 照参考图的做法：时间栏是一列窄字，扫的时候一眼分得清今天和更早。
// 用**本地时区**的零点切，不是 UTC —— 不然东八区的凌晨会被算进「昨天」。
function dayBucket(timestamp) {
    const value = Number(timestamp || 0);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000;
    if (value >= today) return 'today';
    if (value >= today - 86400) return 'yesterday';
    return 'earlier';
}

function timeGutter(item) {
    const value = Number(item?.timestamp || 0);
    if (!value) return '';
    const date = new Date(value * 1000);
    const bucket = dayBucket(value);
    if (bucket === 'today') {
        return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
    }
    if (bucket === 'yesterday') return t('dateYesterday');
    return `${date.getMonth() + 1}/${date.getDate()}`;
}

// 列表里每行只放**一行摘要** —— 主从布局的意义就是列表要窄、要好扫，
// 完整内容在右边。换行符会破坏单行省略，所以压成一行。
function rowText(item) {
    if (item?.type === 'file') {
        return item.name || 'file';
    }
    const text = String(item?.content || '').replace(/\s+/g, ' ').trim();
    return text || t('emptyHere');
}

const selected = ref(null);
// 选中项默认取第一条；列表变了（搜索 / 换分类 / 新消息）而选中项已经不在列表里时，
// 也跟着回到第一条 —— 否则右边会一直显示一条列表里已经没有的内容。
function syncSelection() {
    const list = filteredReceived.value;
    if (!list.length) {
        selected.value = null;
        return;
    }
    const stillThere = list.some((item) => item.id === selected.value?.id);
    if (!stillThere) {
        selected.value = list[0];
    }
}
watch(filteredReceived, syncSelection, { immediate: true });
onMounted(syncSelection);

// 窄屏没有并排的空间，预览改成**全屏弹窗**：同一个 GlancePreview，两个呈现位置。
const previewDialog = ref(false);
function selectItem(item) {
    selected.value = item;
    if (!isWide.value) {
        previewDialog.value = true;
    }
}
const isWide = ref(true);
function syncWidth() {
    isWide.value = window.innerWidth > 1024;
}

// ── 上下键切换条目 ─────────────────────────────────────────────────────
// ⚠️ 必须 `preventDefault`：方向键的默认动作是**滚动容器**。不拦的话「选中换了」和
// 「列表/页面也滚了一段」会同时发生 —— 用起来就像滚动条失控。
//
// 监听挂在 window 上、不是挂在列表上：这一屏的主操作是搜索，焦点通常在搜索框里；
// 而方向键在**单行**输入框里本来就没有含义（不会移动光标），所以在这里接管没有代价。
function moveSelection(step) {
    const list = filteredReceived.value;
    if (!list.length) {
        return;
    }
    const current = list.findIndex((item) => item.id === selected.value?.id);
    const next = current < 0 ? 0 : current + step;
    if (next < 0 || next >= list.length) {
        return; // 到头/到尾就不动
    }
    selected.value = list[next];
    // 选中项得留在视野里，否则按住不放它就跑出屏幕了。
    // 用 `block: 'nearest'` —— 只滚「刚好够看见」那一点，不会把整列翻过去。
    nextTick(() => {
        const rows = document.querySelectorAll('.glance-wall__row');
        rows[next]?.scrollIntoView({ block: 'nearest' });
    });
}

function onGlanceKeydown(event) {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
        return;
    }
    // 弹窗开着的时候（窄屏预览 / 新建房间）不接管 —— 那是另一层界面
    if (previewDialog.value || newRoomDialog.value) {
        return;
    }
    event.preventDefault();
    moveSelection(event.key === 'ArrowDown' ? 1 : -1);
}

onMounted(() => {
    syncWidth();
    window.addEventListener('resize', syncWidth);
    window.addEventListener('keydown', onGlanceKeydown);
});

onBeforeUnmount(() => {
    window.removeEventListener('resize', syncWidth);
    window.removeEventListener('keydown', onGlanceKeydown);
});
</script>

<template>
    <!-- ⚠️ 没有标题：模式名和图标已经在工具栏的模式切换器里了。 -->
    <div class="glance-wall" :class="{ 'glance-wall--dark': isDark }">
        <PageToolbar variant="glance"></PageToolbar>

        <div class="glance-wall__shell">
            <!-- 头部：搜索是这一屏的主操作，所以它**不带外框**、直接铺在头部，
                 占位符里带总数。房间缩成右边一个小 chip。 -->
            <div class="glance-wall__head">
                <v-icon size="18" class="glance-wall__search-icon">mdi-magnify</v-icon>
                <input
                    :value="app.searchQuery"
                    class="glance-wall__search"
                    type="text"
                    :placeholder="searchPlaceholder"
                    @input="app.setSearchQuery($event.target.value)"
                >
                <button
                    v-if="searchActive"
                    type="button"
                    class="glance-wall__clear"
                    :aria-label="t('clear')"
                    @click="app.setSearchQuery('')"
                >
                    <v-icon size="16">mdi-close</v-icon>
                </button>

                <!-- 房间：**本地房间列表**（可切 / 可加 / 可删），不依赖服务端的 roomList。 -->
                <v-menu location="bottom end">
                    <template v-slot:activator="{ props }">
                        <button v-bind="props" type="button" class="glance-wall__room">
                            <span class="glance-wall__room-dot"></span>
                            <span class="glance-wall__room-name">{{ roomLabel }}</span>
                        </button>
                    </template>
                    <v-list density="compact" min-width="200">
                        <v-list-item
                            v-for="room in localRooms"
                            :key="room || '__public__'"
                            :active="currentRoom === room"
                            @click="switchToRoom(room)"
                        >
                            <v-list-item-title class="glance-wall__room-item">
                                <span class="glance-wall__room-item-name">{{ room || t('publicRoom') }}</span>
                                <button
                                    v-if="room"
                                    type="button"
                                    class="glance-wall__room-remove"
                                    :title="t('delete')"
                                    @click.stop="removeRoom(room)"
                                >✕</button>
                            </v-list-item-title>
                        </v-list-item>
                        <v-divider class="my-1"></v-divider>
                        <v-list-item @click="openNewRoomDialog">
                            <v-list-item-title>＋ {{ t('workbenchNewRoom') }}</v-list-item-title>
                        </v-list-item>
                    </v-list>
                </v-menu>
            </div>

            <!-- 分类：**文字 tab**（不是胶囊）。比胶囊省横向空间，窄屏也不用藏文字。 -->
            <div class="glance-wall__tabs">
                <button
                    v-for="opt in FILTER_OPTIONS"
                    :key="opt.key"
                    type="button"
                    class="glance-wall__tab"
                    :class="{ 'glance-wall__tab--active': filterActive(opt.key) }"
                    @click="setTimelineFilter(opt.key)"
                >
                    {{ t(opt.labelKey) }}<span class="glance-wall__tab-count">{{ filterCounts[opt.key] }}</span>
                </button>
            </div>

            <!-- 两栏主从：左边列表、右边预览。列表要窄、要能一行扫完。 -->
            <div class="glance-wall__panes">
                <div class="glance-wall__list">
                    <button
                        v-for="item in filteredReceived"
                        :key="item.id"
                        type="button"
                        class="glance-wall__row"
                        :class="{ 'glance-wall__row--selected': selected && selected.id === item.id }"
                        @click="selectItem(item)"
                    >
                        <span class="glance-wall__row-time">{{ timeGutter(item) }}</span>
                        <span class="glance-wall__row-text">{{ rowText(item) }}</span>
                    </button>

                    <div v-if="!filteredReceived.length" class="glance-wall__list-empty">
                        {{ app.received.length ? t('filterEmpty') : t('emptyTimelineTitle') }}
                    </div>
                </div>

                <div v-if="isWide" class="glance-wall__pane">
                    <glance-preview :item="selected"></glance-preview>
                </div>
            </div>
        </div>

        <!-- 窄屏：预览是全屏弹窗（同一个 GlancePreview）。
             ⚠️ 弹窗被 teleport 出应用子树，样式必须挂在自己身上。 -->
        <v-dialog v-model="previewDialog" fullscreen transition="dialog-bottom-transition">
            <div class="glance-wall__sheet" :class="{ 'glance-wall__sheet--dark': isDark }">
                <div class="glance-wall__sheet-head">
                    <span>{{ t('preview') }}</span>
                    <button type="button" class="glance-wall__sheet-close" :aria-label="t('close')" @click="previewDialog = false">
                        <v-icon size="18">mdi-close</v-icon>
                    </button>
                </div>
                <glance-preview class="glance-wall__sheet-body" :item="selected"></glance-preview>
            </div>
        </v-dialog>

        <v-dialog v-model="newRoomDialog" max-width="360">
            <div class="glance-wall__dialog" :class="{ 'glance-wall__dialog--dark': isDark }">
                <div class="glance-wall__dialog-title">{{ t('workbenchNewRoom') }}</div>
                <v-text-field
                    v-model="newRoomName"
                    :placeholder="t('workbenchRoomPlaceholder')"
                    density="compact"
                    variant="outlined"
                    hide-details
                    autofocus
                    @keydown.enter.prevent="createRoom"
                ></v-text-field>
                <div class="glance-wall__dialog-actions">
                    <v-spacer></v-spacer>
                    <v-btn variant="text" size="small" @click="newRoomDialog = false">{{ t('cancel') }}</v-btn>
                    <v-btn color="primary" variant="flat" size="small" @click="createRoom">{{ t('workbenchCreate') }}</v-btn>
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.glance-wall {
    display: flex;
    flex-direction: column;
    height: 100vh;
    height: 100dvh;
}

.glance-wall__shell {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
    padding: 6px 14px 12px;
    display: flex;
    flex-direction: column;
}

/* ── 头部：搜索 ────────────────────────────────────────────────────────
   照参考图：搜索**不带外框**，直接铺在头部，占位符里带总数。
   「搜索是这一屏的主操作」用位置表达（最上面、最左、占满），不用边框和底色强调。 */
.glance-wall__head {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
    padding: 4px 2px 10px;
    border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.glance-wall__search-icon {
    color: rgba(var(--v-theme-on-surface), 0.5);
}

.glance-wall__search {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: none;
    font-size: 15px;
    color: inherit;
}

.glance-wall__search::placeholder {
    color: rgba(var(--v-theme-on-surface), 0.45);
}

.glance-wall__clear {
    display: inline-flex;
    align-items: center;
    padding: 0 4px;
    border: none;
    background: none;
    cursor: pointer;
    color: inherit;
    opacity: 0.55;
}

.glance-wall__clear:hover {
    opacity: 1;
}

.glance-wall__room {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    flex-shrink: 0;
    padding: 5px 11px;
    border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
    border-radius: 999px;
    background: none;
    cursor: pointer;
    font-size: 12px;
    color: inherit;
    white-space: nowrap;
}

.glance-wall__room:hover {
    border-color: rgb(var(--v-theme-primary));
}

.glance-wall__room-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #1d9e75;
}

.glance-wall__room-name {
    max-width: 10rem;
    overflow: hidden;
    text-overflow: ellipsis;
}

.glance-wall__room-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
}

.glance-wall__room-item-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
}

.glance-wall__room-remove {
    flex-shrink: 0;
    padding: 0 4px;
    border: none;
    background: none;
    cursor: pointer;
    color: inherit;
    opacity: 0.45;
    line-height: 1;
}

.glance-wall__room-remove:hover {
    opacity: 1;
}

/* ── 分类 tab ─────────────────────────────────────────────────────────
   文字 tab（不是胶囊）：省横向空间，窄屏也不用把文字藏掉。
   选中态用**下划线**而不是填充色 —— 填充色会和卡片抢注意力，而下划线只占一条线。 */
.glance-wall__tabs {
    display: flex;
    align-items: center;
    gap: 18px;
    flex-shrink: 0;
    padding: 10px 2px 0;
    overflow-x: auto;
}

.glance-wall__tab {
    flex-shrink: 0;
    padding: 0 0 8px;
    border: none;
    border-bottom: 2px solid transparent;
    background: none;
    cursor: pointer;
    font-size: 13px;
    color: rgba(var(--v-theme-on-surface), 0.6);
    transition: color 0.15s ease, border-color 0.15s ease;
}

.glance-wall__tab:hover {
    color: rgba(var(--v-theme-on-surface), 0.87);
}

.glance-wall__tab--active {
    color: rgb(var(--v-theme-primary));
    border-bottom-color: rgb(var(--v-theme-primary));
}

.glance-wall__tab-count {
    margin-inline-start: 5px;
    font-size: 11px;
    opacity: 0.65;
    font-variant-numeric: tabular-nums;
}

/* ── 两栏主从 ─────────────────────────────────────────────────────────
   左列固定宽（一行摘要够用就行），右栏吃掉剩下的。
   ⚠️ 两栏都必须 `min-height: 0`，否则 grid 项不会缩到内容以下，栏内的
   `overflow-y: auto` 永远不触发 —— 表现是「列里滚不动」。 */
.glance-wall__panes {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(0, 300px) minmax(0, 1fr);
    gap: 16px;
    padding-top: 10px;
}

.glance-wall__list {
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    padding-right: 4px;
}

.glance-wall__row {
    display: flex;
    align-items: baseline;
    gap: 10px;
    width: 100%;
    padding: 7px 8px;
    border: none;
    border-radius: 7px;
    background: none;
    cursor: pointer;
    text-align: left;
    color: inherit;
    transition: background-color 0.12s ease;
}

.glance-wall__row:hover {
    background: rgba(var(--v-theme-on-surface), 0.05);
}

.glance-wall__row--selected {
    background: rgba(var(--v-theme-primary), 0.1);
}

/* 时间栏：固定宽度 + 等宽数字，让整列对齐成一条 —— 这是「一眼扫出什么时候」的关键。 */
.glance-wall__row-time {
    flex: 0 0 42px;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    color: rgba(var(--v-theme-on-surface), 0.5);
}

.glance-wall__row-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 13px;
}

.glance-wall__list-empty {
    padding: 32px 8px;
    text-align: center;
    font-size: 12px;
    color: rgba(var(--v-theme-on-surface), 0.5);
}

.glance-wall__pane {
    min-height: 0;
    overflow-y: auto;
    padding-left: 16px;
    border-left: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

/* ── 窄屏：预览改全屏弹窗 ───────────────────────────────────────────── */
.glance-wall__sheet {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: rgb(var(--v-theme-surface));
    color: rgb(var(--v-theme-on-surface));
}

.glance-wall__sheet-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-shrink: 0;
    padding: 12px 16px;
    font-size: 13px;
    border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.glance-wall__sheet-close {
    border: none;
    background: none;
    cursor: pointer;
    color: inherit;
}

.glance-wall__sheet-body {
    flex: 1;
    min-height: 0;
    padding: 16px;
    overflow-y: auto;
}

/* ── 新建房间弹窗。⚠️ 弹窗被 teleport 出去，`--v-theme-*` 拿不到 → 颜色写死。 ── */
.glance-wall__dialog {
    background: #fff;
    color: #1f2937;
    border-radius: 14px;
    padding: 16px 18px 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
}

.glance-wall__dialog--dark {
    background: #0f172a;
    color: #e2e8f0;
}

.glance-wall__dialog-title {
    font-size: 1rem;
    font-weight: 500;
}

.glance-wall__dialog-actions {
    display: flex;
    align-items: center;
    gap: 4px;
}

/* ── 媒体查询一律放在**最后** ────────────────────────────────────────────
   ⚠️ 这不是风格问题，是**顺序问题**：媒体查询里的选择器和基础规则特异性相同，
   谁生效只看谁在文件里更靠后 —— 放在基础规则前面会被整个盖掉，而且是静默的。
   这里踩过一次。 */

/* 窄屏：两栏收成一栏（预览走全屏弹窗，见模板里的 isWide）。 */
@media (max-width: 1024px) {
    .glance-wall__panes {
        grid-template-columns: minmax(0, 1fr);
        gap: 0;
    }

    .glance-wall__pane {
        display: none;
    }

    .glance-wall__tabs {
        gap: 14px;
    }
}
</style>
