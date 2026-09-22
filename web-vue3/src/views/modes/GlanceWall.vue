<script setup>
// 速览模式：**以查阅为主**的视图 —— 搜索条和分类条在前，时间流在后，没有发送区。
//
// 和标准模式的区别（标准模式是「干活」的地方，发送区占了大半屏）：
//   · **没有发送区**。想发东西去别的模式。
//   · 搜索条和分类条**常驻**。标准模式里它们是可选的显示开关（默认关）。
//   · 时间流按日期分组（今天 / 昨天 / 更早）—— 查阅时最常问的是「今天进来了什么」。
//   · 卡片保持原样：同一批条目、同一套组件，只是排布不同。
//     **看板是同一批条目的一个视图，速览也是** —— 不新建数据、不新建卡片。
//
// 分类和搜索复用标准模式那一套（同一个 localStorage 键、同一个 app.visibleReceived），
// 所以两个模式看到的是同一个筛选状态。分成两份的话，切个模式内容就变了，看着像丢了东西。
import { computed, ref } from 'vue';
import { isImageName, looksLikeTable, looksLikeTaskList } from '@/util.js';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import PageToolbar from '@/components/PageToolbar.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';
import { useLocalRooms } from '@/composables/useLocalRooms.js';

const mdiTimeline = 'mdi-timeline';
const mdiTextBox = 'mdi-text-box-outline';
const mdiImage = 'mdi-image-outline';
const mdiFile = 'mdi-file-outline';
const mdiCheckboxMarkedOutline = 'mdi-checkbox-marked-outline';
const mdiTable = 'mdi-table';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
// 房间侧栏的开关在 App.vue 里，通过这个 provide 暴露给工具栏 —— 这里复用同一个动作，
// 免得「房间 chip」变成第二个开关状态。
// 房间**不走服务端那份列表**，用本地管理的那一套（和工作台共用，见 useLocalRooms）。
//
// 为什么：服务端把 `server.roomList` 关掉时 `/rooms` 什么都不返回，工具栏那个房间图标
// 也整个不渲染 —— 但「我在哪几个房间之间切」本来就不需要服务端知道，本地记一份就够。
// 所以这个 chip **不受那个开关控制**，永远可用。
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
    { key: 'all', labelKey: 'filterAll', icon: mdiTimeline },
    { key: 'text', labelKey: 'filterText', icon: mdiTextBox },
    { key: 'image', labelKey: 'filterImage', icon: mdiImage },
    { key: 'file', labelKey: 'filterFile', icon: mdiFile },
    { key: 'task', labelKey: 'filterTaskList', icon: mdiCheckboxMarkedOutline },
    { key: 'table', labelKey: 'filterTable', icon: mdiTable },
];

// 分类条的高亮规则：**只有正在生效的条件才高亮**。
// 「全部」永远不高亮 —— 它代表「不筛」，点亮它等于告诉用户有个筛选在生效，而其实没有。
const filterActive = (key) => key !== 'all' && timelineFilter.value === key;

// 搜索框的「激活态」判据：有查询。它同时也是「搜索正在生效」的文字证据
// （右边那行「找到 N 条」），不只靠边框颜色。
const searchActive = computed(() => Boolean(String(app.searchQuery || '').trim()));

const filteredReceived = computed(() => {
    // 列表来源是 visibleReceived（已套搜索），不是 received。
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

// 每一类的条数。查阅时「有几条」本身就是要看的信息（草稿里就有）。
// 一次遍历算完六个数，别对每个分类各 filter 一遍 —— 那是六趟。
// ⚠️ 「文件」不含图片：和上面 filteredReceived 的判据保持一致
// （`isImageName(name) === wantImage`，wantImage 为 false 时留下的就是非图片文件）。
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

// 日期分组。用**本地时区**的零点切，不是 UTC —— 不然东八区的凌晨会被算进「昨天」。
function dayBucket(timestamp) {
    const value = Number(timestamp || 0);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000;
    if (value >= today) return 'today';
    if (value >= today - 86400) return 'yesterday';
    return 'earlier';
}

const groupedReceived = computed(() => {
    const buckets = [
        { key: 'today', labelKey: 'dateToday', items: [] },
        { key: 'yesterday', labelKey: 'dateYesterday', items: [] },
        { key: 'earlier', labelKey: 'dateEarlier', items: [] },
    ];
    const byKey = new Map(buckets.map((bucket) => [bucket.key, bucket]));
    for (const item of filteredReceived.value) {
        byKey.get(dayBucket(item.timestamp)).items.push(item);
    }
    return buckets.filter((bucket) => bucket.items.length);
});

// 左侧日期轴的跳转。用 id + scrollIntoView 而不是 ref 数组 —— 分组是 computed 出来的，
// ref 数组的索引会随筛选变化而错位。
// `block: 'start'` 会顶到视口最上面，所以分组头上写了 `scroll-margin-top` 让开工具栏。
function jumpToGroup(key) {
    const el = document.getElementById(`glance-group-${key}`);
    if (el && typeof el.scrollIntoView === 'function') {
        el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
}
</script>

<template>
    <!-- ⚠️ 没有标题：模式名和图标已经在工具栏的模式切换器里了，再写一遍是重复。 -->
    <div class="glance-wall" :class="{ 'glance-wall--dark': isDark }">
        <PageToolbar variant="glance"></PageToolbar>

        <div class="glance-wall__shell">
            <div class="glance-wall__search-row">
                <!-- 搜索条是这一屏的**主操作**：字号更大、描边比分类条强一档。 -->
                <v-text-field
                    :model-value="app.searchQuery"
                    class="glance-wall__search"
                    :class="{ 'glance-wall__search--active': searchActive }"
                    density="comfortable"
                    variant="solo"
                    rounded="pill"
                    flat
                    hide-details
                    prepend-inner-icon="mdi-magnify"
                    :placeholder="t('searchPlaceholder')"
                    @update:model-value="app.setSearchQuery"
                >
                    <template v-slot:append-inner>
                        <span v-if="searchActive" class="glance-wall__found">
                            {{ t('glanceFound', { count: filteredReceived.length }) }}
                        </span>
                        <button
                            v-if="searchActive"
                            type="button"
                            class="glance-wall__clear"
                            :aria-label="t('clear')"
                            @click="app.setSearchQuery('')"
                        >
                            <v-icon size="16">mdi-close</v-icon>
                        </button>
                    </template>
                </v-text-field>

                <!-- 房间缩成右边一个小 chip：「我在哪个房间」是状态，不是主操作。
                     点它开的是**本地房间列表**（可切 / 可加 / 可删），不依赖服务端的 roomList。 -->
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

            <!-- 分类条：只在真有内容时出现（空列表上摆一条没用的过滤条更碍事）。 -->
            <div v-if="app.received.length" class="glance-wall__filters">
                <v-chip
                    v-for="opt in FILTER_OPTIONS"
                    :key="opt.key"
                    size="small"
                    label
                    class="glance-wall__filter"
                    :variant="filterActive(opt.key) ? 'flat' : 'outlined'"
                    :color="filterActive(opt.key) ? 'primary' : undefined"
                    :aria-label="t(opt.labelKey)"
                    @click="setTimelineFilter(opt.key)"
                >
                    <!-- 窄屏只留图标：六个带字的分类在手机上会折成两行。
                         文字藏掉后图标要自己居中，所以间距交给 CSS 管（不用 `start`）。
                         计数跟着文案一起藏 —— 窄屏上光剩数字更看不懂。 -->
                    <v-icon size="16" class="glance-wall__filter-icon">{{ opt.icon }}</v-icon>
                    <span class="glance-wall__filter-label">
                        {{ t(opt.labelKey) }}<span class="glance-wall__filter-count">{{ filterCounts[opt.key] }}</span>
                    </span>
                </v-chip>
            </div>

            <!-- 时间轴区：左边一条日期轴，右边一条带竖轴的时间流。
                 轴是**可点的跳转**（还带每组条数）—— 查阅时「今天有多少条」本身就是要看的信息，
                 而且长列表里能直接跳。窄屏把轴收掉，分组头仍然在（见 CSS）。 -->
            <div v-if="groupedReceived.length" class="glance-wall__body">
                <nav class="glance-wall__rail">
                    <button
                        v-for="bucket in groupedReceived"
                        :key="bucket.key"
                        type="button"
                        class="glance-wall__rail-item"
                        @click="jumpToGroup(bucket.key)"
                    >
                        <span class="glance-wall__rail-label">{{ t(bucket.labelKey) }}</span>
                        <span class="glance-wall__rail-count">{{ bucket.items.length }}</span>
                    </button>
                </nav>

                <div class="glance-wall__timeline">
                    <!-- ⚠️ 这里**故意没有分组标题**：日期已经由左边那条轴承担
                         （带每组条数、可点着跳），再写一遍就是同一句话在同一行出现两次 ——
                         实测那个样子看着像渲染出了 bug（轴里「今天 1」右边紧跟着又一个「今天」）。
                         卡片自己带完整时间戳，「这是哪一天」的信息没丢。 -->
                    <div
                        v-for="bucket in groupedReceived"
                        :key="bucket.key"
                        :id="`glance-group-${bucket.key}`"
                        class="glance-wall__group"
                    >
                        <div
                            v-for="item in bucket.items"
                            :key="item.id"
                            class="glance-wall__item"
                        >
                            <component
                                :is="item.type === 'text' ? ReceivedText : ReceivedFile"
                                :meta="item"
                            />
                        </div>
                    </div>
                </div>
            </div>

            <div v-if="app.received.length && !filteredReceived.length" class="glance-wall__hint">
                {{ t('filterEmpty') }}
            </div>

            <div v-if="!app.received.length" class="glance-wall__empty">
                <v-icon size="42" color="primary">{{ mdiTimeline }}</v-icon>
                <div class="glance-wall__empty-title">{{ t('emptyTimelineTitle') }}</div>
                <div class="glance-wall__empty-sub">{{ t('timelineEmptySubtitle') }}</div>
            </div>
        </div>

        <!-- 新建房间。和工具栏那套无关 —— 建的是**本地**记住的房间。
             ⚠️ 弹窗被 teleport 出应用子树，样式必须挂在自己身上（见仓库里那条覆盖层约定）。 -->
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
    min-height: 100vh;
    min-height: 100dvh;
}

.glance-wall__shell {
    width: 100%;
    max-width: 980px;
    margin: 0 auto;
    padding: 8px 12px 24px;
}

.glance-wall__search-row {
    display: flex;
    align-items: center;
    gap: 10px;
}

.glance-wall__search {
    flex: 1;
    min-width: 0;
}

/* 搜索框：静止态的描边比分类条强一档（secondary 对 tertiary），让它天然是主角。
   激活态（有查询）再把描边换成主题色 + 一层很淡的底 —— 边框颜色单独变化太弱，
   底色一起动才一眼看得出「这一屏正在被筛」。 */
.glance-wall__search :deep(.v-field) {
    font-size: 14px;
    border: 1px solid rgba(var(--v-border-color), calc(var(--v-border-opacity) * 2.4));
    background: rgb(var(--v-theme-surface));
    transition: border-color 0.15s ease, background-color 0.15s ease;
}

.glance-wall__search--active :deep(.v-field) {
    border-color: rgb(var(--v-theme-primary));
    background: color-mix(in srgb, rgb(var(--v-theme-primary)) 6%, rgb(var(--v-theme-surface)));
}

.glance-wall__found {
    margin-inline-end: 8px;
    font-size: 12px;
    white-space: nowrap;
    color: rgb(var(--v-theme-primary));
}

.glance-wall__clear {
    display: inline-flex;
    align-items: center;
    padding: 0;
    border: none;
    background: none;
    cursor: pointer;
    color: inherit;
    opacity: 0.6;
}

.glance-wall__clear:hover {
    opacity: 1;
}

/* 房间 chip：小、次要、只有一个状态点。点它开房间侧栏。 */
.glance-wall__room {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    flex-shrink: 0;
    padding: 8px 13px;
    border: 1px solid rgba(var(--v-border-color), calc(var(--v-border-opacity) * 2.4));
    border-radius: 999px;
    background: none;
    cursor: pointer;
    font-size: 13px;
    color: inherit;
    white-space: nowrap;
    transition: border-color 0.15s ease;
}

.glance-wall__room:hover {
    border-color: rgb(var(--v-theme-primary));
}

/* 房间菜单里的一行：名字占满，删除按钮在最右。
   （菜单被 teleport 出去，但**这一层的元素仍由本组件渲染**，所以 scoped 样式照样命中；
   失效的只是「祖先的 CSS 变量」—— 所以下面别用 --v-theme-*。） */
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

/* 新建房间弹窗。⚠️ 弹窗被 teleport 出应用子树，`--v-theme-*` 拿不到，
   所以颜色**写死**（明暗各一套）—— 和看板的详情弹窗同一个路子。 */
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

.glance-wall__room-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #1d9e75;
}

.glance-wall__room-name {
    max-width: 12rem;
    overflow: hidden;
    text-overflow: ellipsis;
}

.glance-wall__filters {
    display: flex;
    flex-wrap: wrap;
    /* 居中：和标准模式的分类条一致。左对齐时它和上面的搜索框左边缘对齐，
       看着像搜索框的附属；居中之后它是独立的一层。 */
    justify-content: center;
    gap: 6px;
    padding: 12px 0 4px;
}

.glance-wall__filter {
    cursor: pointer;
}

.glance-wall__filter-icon {
    margin-inline-end: 6px;
}

/* 计数比文案淡一档：它是补充信息，不该跟分类名抢注意力。 */
.glance-wall__filter-count {
    margin-inline-start: 5px;
    opacity: 0.65;
    font-variant-numeric: tabular-nums;
}

/* 时间轴区：左轴 + 时间流。 */
.glance-wall__body {
    display: grid;
    grid-template-columns: 88px minmax(0, 1fr);
    gap: 18px;
    align-items: start;
}

.glance-wall__rail {
    position: sticky;
    /* 让开工具栏。和分组头的 scroll-margin-top 是同一个数。 */
    top: 64px;
    display: flex;
    flex-direction: column;
    gap: 2px;
}

.glance-wall__rail-item {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 8px 5px 10px;
    border: none;
    /* 左侧那条 2px 只在悬停时上色 —— **不做「选中态」**：轴是跳转，不是筛选
       （筛选在分类条上）。两处都表达「当前」，用户会以为它们是同一件事。 */
    border-left: 2px solid transparent;
    border-radius: 0 6px 6px 0;
    background: none;
    cursor: pointer;
    font-size: 12px;
    text-align: left;
    color: rgba(var(--v-theme-on-surface), 0.6);
    transition: color 0.15s ease, border-color 0.15s ease;
}

.glance-wall__rail-item:hover {
    border-left-color: rgb(var(--v-theme-primary));
    color: rgb(var(--v-theme-primary));
}

.glance-wall__rail-count {
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
}

/* 竖轴：一条发丝线，每个条目一个点。
   它让「这是一条时间流」看得见，而不只是「一摞卡片」。
   ⚠️ 这个容器**不能有 overflow** —— 轴和点都是绝对定位，父容器一裁就静默消失
   （DOM 在、几何量得出、就是看不见）。 */
.glance-wall__timeline {
    position: relative;
    min-width: 0;
    padding-left: 18px;
}

.glance-wall__timeline::before {
    content: '';
    position: absolute;
    left: 4px;
    top: 8px;
    bottom: 8px;
    width: 1px;
    background: rgba(var(--v-border-color), calc(var(--v-border-opacity) * 2));
}

.glance-wall__group {
    margin-top: 14px;
    /* 给轴的跳转用：`block: 'start'` 会顶到视口最上面，让开工具栏。 */
    scroll-margin-top: 64px;
}

.glance-wall__group:first-child {
    margin-top: 0;
}

.glance-wall__item {
    position: relative;
}

/* 条目上的点：钉在那条竖轴上。
   容器有 padding-left: 18px，轴在 left: 4px，点宽 7px —— 要落回轴上就是 left: -17px。 */
.glance-wall__item::before {
    content: '';
    position: absolute;
    left: -17px;
    top: 15px;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: rgba(var(--v-border-color), calc(var(--v-border-opacity) * 4));
}

.glance-wall__hint,
.glance-wall__empty {
    text-align: center;
    padding: 48px 16px;
    color: rgba(var(--v-theme-on-surface), 0.6);
}

.glance-wall__empty-title {
    margin: 16px 0 6px;
    font-size: 1.05rem;
    font-weight: 500;
    color: rgba(var(--v-theme-on-surface), 0.87);
}

.glance-wall__empty-sub {
    font-size: 0.875rem;
}

/* ── 媒体查询一律放在**最后** ────────────────────────────────────────────
   ⚠️ 这不是风格问题，是**顺序问题**：媒体查询里的选择器和基础规则**特异性相同**，
   所以谁生效只看谁在文件里更靠后 —— 放在基础规则前面的话会被整个盖掉，
   而且是静默的（样式看起来"没写生效"）。
   这里踩过一次：`@media (max-width: 1024px)` 写在 `.glance-wall__rail` 前面，
   于是 390px 下 `flex-direction` 仍是 column。 */

/* 窄屏只留图标：六个带字的分类在手机上会折成两行，白白占掉一条横条的高度。
   图标本身认得出来，文案靠 chip 上的 aria-label 保住可访问性。 */
@media (max-width: 768px) {
    .glance-wall__filter-label {
        display: none;
    }

    .glance-wall__filter-icon {
        margin-inline-end: 0;
    }
}

/* 窄屏：把竖着的日期轴**转成一条横向的胶囊行**，不是删掉。
   ── 竖轴在手机上占的是整整一列（88px ≈ 390px 屏宽的 22%），而横向只占一行高度（约 30px）。
   功能一个不少：每组条数还看得到、还能点着跳。
   断点写 1024 不是 768：只写 768 的话平板那一档（768–1024）会留着那一列。
   ⚠️ 时间轴的竖线和圆点**不动** —— 它们是绝对定位的覆盖层、不占位置。 */
@media (max-width: 1024px) {
    .glance-wall__body {
        grid-template-columns: minmax(0, 1fr);
        gap: 8px;
    }

    .glance-wall__rail {
        /* 窄屏是横排，**钉在工具栏下面** —— 分组头去掉了，这排胶囊就是唯一的日期指示，
           滚起来的时候不能让它跑掉。加一层底色，免得卡片从它下面透出来。 */
        position: sticky;
        top: 56px;
        z-index: 1;
        flex-direction: row;
        flex-wrap: wrap;
        gap: 6px;
        padding: 4px 0;
        background: rgb(var(--v-theme-background));
    }

    .glance-wall__rail-item {
        flex: 0 0 auto;
        /* 竖轴靠一条左边线表示悬停；横排要改成整圈描边，不然那条线看着像断了。 */
        border: 1px solid rgba(var(--v-border-color), calc(var(--v-border-opacity) * 2));
        border-radius: 999px;
        padding: 3px 10px;
    }

    .glance-wall__rail-item:hover {
        border-color: rgb(var(--v-theme-primary));
    }
}
</style>
