<script setup>
import { computed } from 'vue';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { useAppStore } from '@/store/app';

const mdiViewList = 'mdi-view-list';
const mdiHeart = 'mdi-heart';
const mdiHeartOutline = 'mdi-heart-outline';
const mdiLock = 'mdi-lock';
const mdiMagnify = 'mdi-magnify';
const mdiHomeOutline = 'mdi-home-outline';

const props = defineProps({
    // 分组在 App.vue 里算好（收藏 / 活跃 / 其他 依赖派生逻辑，不属于展示层）
    groups: { type: Array, default: () => [] },
    currentRoom: { type: Object, default: null },
    // 摘要 pill 里显示房间名，下面分组标题是「当前房间」—— 两个字符串，别混用
    currentRoomName: { type: String, default: '' },
    currentRoomLabel: { type: String, default: '' },
    title: { type: String, default: '' },
    count: { type: Number, default: 0 },
    favoriteCount: { type: Number, default: 0 },
    activeCount: { type: Number, default: 0 },
    loading: { type: Boolean, default: false },
    hasRooms: { type: Boolean, default: true },
    search: { type: String, default: '' },
    // dock 和 bottom sheet 只有外层容器不同，内部结构一致；差异只在 body 的高度上限
    variant: { type: String, default: 'dock' },
    // 侧栏停在哪一侧。只有 dock 变体用得上：内侧那条发丝线得画在朝向内容的那一边，
    // 而且要用模式的 --rl-border（画在 App.vue 的容器上就只能写死一个灰，接缝一眼看得出来）。
    dockSide: { type: String, default: 'right' },
});

const emit = defineEmits(['update:search', 'select', 'favorite']);

const app = useAppStore();
const theme = useTheme();
const { t } = useI18n();

const isDark = computed(() => theme.current.value?.dark ?? false);

// 这一层是「模式皮肤」的全部机关：模式只决定这几个自定义属性的值，
// 行结构本身不跟着变。所以是 6 组 token，不是 6 份模板。
const skinClass = computed(() => `rl--${app.uiMode || 'default'}`);

function displayName(room) {
    return room && room.name ? room.name : t('publicRoom');
}

// 行内的「N 设备 · 消息 M」。
// 这两个数一度被塞进 title 里省行高 —— 那是错的：触屏没有 hover，手机上等于彻底消失，
// 不是"藏起来"而是"没了"。现在放回行内，但仍保持一行（不回到 82px 的两行副标题）。
//
// 两处留白，都是为了省下宽度给房间名（实测这两个数占 40~90px，是行里最贵的一段）：
// - 设备数为 0 时不写。活跃绿点已经编码了「有没有设备」（服务端 isActive = deviceCount > 0），
//   再写一遍「0 设备」等于把绿点的缺席说第二次。
// - 消息数为 0 时不写。同「不写非活跃」。
// 两个都是 0 时整段不渲染。
function countsLabel(room) {
    const parts = [];
    const devices = room.deviceCount || 0;
    const messages = room.messageCount || 0;
    if (devices > 0) {
        parts.push(`${devices} ${t('devices')}`);
    }
    if (messages > 0) {
        parts.push(`${t('messages')} ${messages}`);
    }
    return parts.join(' · ');
}

// 相对时间。原来每行挂两行副标题（`N 设备 · 消息 N` 和 `最后活跃 · 时间`），
// 一行 82px、移动端 103px，八个房间一屏放不下。现在只留相对时间，
// 设备数/消息数挪进 title —— 鼠标停在行上还能看到，视觉上不再占两行。
function relativeTime(timestamp) {
    if (!timestamp || timestamp === 0) {
        return t('never');
    }
    const diff = Math.floor(Date.now() / 1000) - timestamp;
    if (diff < 60) {
        return t('justNow');
    }
    if (diff < 3600) {
        return t('minutesAgo', { minutes: Math.floor(diff / 60) });
    }
    if (diff < 86400) {
        return t('hoursAgo', { hours: Math.floor(diff / 3600) });
    }
    return t('daysAgo', { days: Math.floor(diff / 86400) });
}

function rowTitle(room) {
    const parts = [displayName(room)];
    parts.push(`${room.deviceCount || 0} ${t('devices')}`);
    parts.push(`${t('messages')} ${room.messageCount || 0}`);
    parts.push(`${t('lastActive')} ${relativeTime(room.lastActive)}`);
    return parts.join(' · ');
}

function isCurrent(room) {
    return props.currentRoom && room.name === props.currentRoom.name;
}
</script>

<template>
    <div
        class="rl"
        :class="[
            skinClass,
            `rl--${variant}`,
            { 'rl--dark': isDark, [`rl--dock-${dockSide}`]: variant === 'dock' },
        ]"
    >
        <!-- 头部放在组件里而不是留在 App.vue：它也得跟着模式走。
             之前头部留在外面，terminal 模式下头部是圆润的 Vuetify 样式、
             下面是等宽方角，接缝一眼就能看出来。 -->
        <div class="rl__header">
            <div class="rl__header-title">
                <v-icon size="small">{{ mdiViewList }}</v-icon>
                <span class="rl__title">{{ title }}</span>
                <span class="rl__count">{{ count }} {{ t('rooms') }}</span>
            </div>
            <div class="rl__header-actions">
                <slot name="actions"></slot>
            </div>
        </div>

        <div class="rl__body" :class="`rl__body--${variant}`">
            <div class="rl__toolbar">
                <v-text-field
                    :model-value="search"
                    :placeholder="t('searchRooms')"
                    :prepend-inner-icon="mdiMagnify"
                    variant="outlined"
                    density="compact"
                    clearable
                    hide-details
                    class="rl__search"
                    @update:model-value="emit('update:search', $event)"
                ></v-text-field>
            </div>

            <div class="rl__summary">
                <span class="rl__pill rl__pill--accent">{{ currentRoomName }}</span>
                <span class="rl__pill">{{ favoriteCount }} {{ t('favoriteRoomsLabel') }}</span>
                <span class="rl__pill">{{ activeCount }} {{ t('activeRoomsLabel') }}</span>
            </div>

            <div v-if="loading && !hasRooms" class="rl__state">
                <v-progress-circular indeterminate color="primary"></v-progress-circular>
                <div class="rl__state-text">{{ t('loadingRooms') }}</div>
            </div>

            <div v-else-if="!hasRooms" class="rl__state">
                <v-icon size="48" color="grey-lighten-1">{{ mdiHomeOutline }}</v-icon>
                <div class="rl__state-text">{{ t('noRoomsFound') }}</div>
            </div>

            <div v-else class="rl__sections">
                <section v-if="currentRoom" class="rl-group">
                    <div class="rl-group__label">{{ currentRoomLabel }}</div>
                    <div
                        class="rl-row rl-row--current"
                        role="button"
                        tabindex="0"
                        :title="rowTitle(currentRoom)"
                        @click="emit('select', currentRoom.name)"
                        @keydown.enter.prevent="emit('select', currentRoom.name)"
                    >
                        <span class="rl-row__mark" aria-hidden="true"></span>
                        <!-- 两行：第一行名字（+ 活跃点 + 锁），第二行元信息（计数 + 时间）。
                             挤成一行的话，332px 的 dock 里「名字 + 计数 + 时间 + 点 + 收藏」
                             装不下 —— 实测名字只剩 93~117px，14 字的名字被砍到 7 个字。
                             分两行后名字拿回整宽，行高由 min-height 兜底，八个房间照样一屏放得下。 -->
                        <span class="rl-row__text">
                            <span class="rl-row__title">
                                <span class="rl-row__name">{{ displayName(currentRoom) }}</span>
                                <span v-if="currentRoom.isActive" class="rl-row__dot" aria-hidden="true"></span>
                                <span v-if="currentRoom.isProtected" class="rl-row__lock">
                                    <v-icon size="x-small">{{ mdiLock }}</v-icon>
                                </span>
                            </span>
                            <span class="rl-row__meta">
                                <span v-if="countsLabel(currentRoom)" class="rl-row__counts">{{ countsLabel(currentRoom) }}</span>
                                <span v-if="countsLabel(currentRoom)" class="rl-row__sep" aria-hidden="true">·</span>
                                <span class="rl-row__time">{{ relativeTime(currentRoom.lastActive) }}</span>
                            </span>
                        </span>
                        <button
                            type="button"
                            class="rl-row__fav"
                            :class="{ 'rl-row__fav--on': currentRoom.isFavorite }"
                            @click.stop="emit('favorite', currentRoom.name)"
                        >
                            <v-icon size="x-small">{{ currentRoom.isFavorite ? mdiHeart : mdiHeartOutline }}</v-icon>
                        </button>
                    </div>
                </section>

                <section v-for="group in groups" :key="group.key" class="rl-group">
                    <div class="rl-group__label">{{ group.title }}</div>
                    <div
                        v-for="room in group.rooms"
                        :key="room.name"
                        class="rl-row"
                        :class="{ 'rl-row--current': isCurrent(room) }"
                        role="button"
                        tabindex="0"
                        :title="rowTitle(room)"
                        @click="emit('select', room.name)"
                        @keydown.enter.prevent="emit('select', room.name)"
                    >
                        <span class="rl-row__mark" aria-hidden="true"></span>
                        <span class="rl-row__text">
                            <span class="rl-row__title">
                                <span class="rl-row__name">{{ displayName(room) }}</span>
                                <span v-if="room.isActive" class="rl-row__dot" aria-hidden="true"></span>
                                <span v-if="room.isProtected" class="rl-row__lock">
                                    <v-icon size="x-small">{{ mdiLock }}</v-icon>
                                </span>
                            </span>
                            <span class="rl-row__meta">
                                <span v-if="countsLabel(room)" class="rl-row__counts">{{ countsLabel(room) }}</span>
                                <span v-if="countsLabel(room)" class="rl-row__sep" aria-hidden="true">·</span>
                                <span class="rl-row__time">{{ relativeTime(room.lastActive) }}</span>
                            </span>
                        </span>
                        <button
                            type="button"
                            class="rl-row__fav"
                            :class="{ 'rl-row__fav--on': room.isFavorite }"
                            @click.stop="emit('favorite', room.name)"
                        >
                            <v-icon size="x-small">{{ room.isFavorite ? mdiHeart : mdiHeartOutline }}</v-icon>
                        </button>
                    </div>
                </section>
            </div>
        </div>
    </div>
</template>

<style scoped>
/* ── 基础 token（default 模式 / 浅色） ─────────────────────────────
   所有颜色都写成自定义属性，并且定义在组件根上 —— 不能定义在 .app-shell 上，
   因为 v-bottom-sheet / v-dialog 会被 teleport 到 body 下的 overlay 容器，
   跑出应用子树，写在祖先上的变量和后代选择器在那里一概失配（踩过两次：
   房间列表 bottom sheet 的深色主题错乱、terminal reader 的调色板全失）。 */
.rl {
    --rl-font: inherit;
    --rl-radius: 10px;
    --rl-row-h: 44px;
    --rl-row-pad: 0 10px;
    --rl-title-size: 15px;
    --rl-name-size: 13.5px;
    --rl-time-size: 11.5px;
    --rl-label-size: 11px;
    --rl-label-transform: uppercase;
    --rl-surface-hover: rgba(148, 163, 184, 0.14);
    --rl-current-bg: rgba(59, 130, 246, 0.1);
    --rl-current-accent: #2563eb;
    --rl-text: #0f172a;
    --rl-muted: #64748b;
    --rl-border: rgba(148, 163, 184, 0.24);
    --rl-sep: 0;
    /* 侧栏自己的底色。默认跟应用背景走；下面每个模式再覆盖成它的页面底色 ——
       侧栏要看起来是「那一栏」，而不是浮在页面上的一块卡片。 */
    --rl-panel-bg: rgb(var(--v-theme-background));
    font-family: var(--rl-font);
    color: var(--rl-text);
}

/* dock 变体 = 真·侧栏：自己撑满父容器，列表区吃掉剩余高度自己滚。
   （之前是「100vh 减一个常量」的算法，既算不准也跟容器高度无关。） */
.rl--dock {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    background: var(--rl-panel-bg);
}

.rl--dock .rl__body--dock {
    flex: 1;
    min-height: 0;
    max-height: none;
    overflow: auto;
}

/* 内侧发丝线：画在朝向内容的那一边，颜色用模式的 --rl-border，
   所以便签模式是暖色线、终端模式是等宽深色线，接缝不会突然变成一条冷灰。 */
.rl--dock-left {
    border-right: 1px solid var(--rl-border);
}

.rl--dock-right {
    border-left: 1px solid var(--rl-border);
}

.rl--dark {
    --rl-surface-hover: rgba(148, 163, 184, 0.16);
    --rl-current-bg: rgba(96, 165, 250, 0.16);
    --rl-current-accent: #60a5fa;
    --rl-text: #e2e8f0;
    --rl-muted: #94a3b8;
    --rl-border: rgba(71, 85, 105, 0.7);
}

/* ── 六套模式皮肤：只改 token，不动行结构 ─────────────────────────
   每个模式给一个 --rl-panel-bg = 该模式的页面底色（取各 *Wall.vue 的根 background），
   侧栏才跟内容连成一片。 */

/* terminal：等宽、直角、行距紧，当前房间用 > 前缀而不是色块 */
.rl--terminal {
    --rl-panel-bg: #ffffff;
    --rl-font: 'SF Mono', 'Menlo', 'Consolas', monospace;
    --rl-radius: 0;
    --rl-row-h: 28px;
    --rl-row-pad: 0 6px;
    --rl-title-size: 14px;
    --rl-name-size: 12.5px;
    --rl-time-size: 11px;
    --rl-label-size: 10px;
    --rl-current-bg: transparent;
    --rl-current-accent: #0969da;
    --rl-surface-hover: rgba(9, 105, 218, 0.08);
    --rl-sep: 1;
}

.rl--dark.rl--terminal {
    --rl-panel-bg: #0d1117;
    --rl-current-accent: #79c0ff;
    --rl-surface-hover: rgba(121, 192, 255, 0.1);
    --rl-border: #30363d;
}

/* workbench：系统字、类表格、行高 40，当前房间靠左侧色条 */
.rl--workbench {
    --rl-panel-bg: #eef0f4;
    --rl-radius: 8px;
    --rl-row-h: 40px;
    --rl-name-size: 13px;
}

.rl--dark.rl--workbench {
    --rl-panel-bg: #13161c;
}

/* sticky：手写体、行高 52，当前房间整行马卡龙底，不用色条 */
.rl--sticky {
    --rl-panel-bg: #fdf7e4;
    --rl-font: 'Comic Sans MS', 'Kaiti', 'PingFang SC', sans-serif;
    --rl-radius: 4px;
    --rl-row-h: 52px;
    --rl-name-size: 14px;
    --rl-label-transform: none;
    --rl-current-bg: #fdeaa8;
    --rl-current-accent: #8a7f5c;
    --rl-muted: #8a7f5c;
    --rl-surface-hover: rgba(138, 127, 92, 0.12);
}

.rl--dark.rl--sticky {
    --rl-panel-bg: #211d12;
    --rl-current-bg: #6b5a2a;
    --rl-muted: #b9a97f;
}

/* chat：圆角大、行高 48，当前房间像一条气泡 */
.rl--chat {
    --rl-panel-bg: #f6f7fa;
    --rl-radius: 14px;
    --rl-row-h: 48px;
    --rl-current-bg: rgba(59, 130, 246, 0.12);
}

.rl--dark.rl--chat {
    --rl-panel-bg: #15171c;
}

/* mega：大字号、行高 56 */
.rl--mega {
    --rl-panel-bg: #fdfdfb;
    --rl-radius: 14px;
    --rl-row-h: 56px;
    --rl-name-size: 16px;
    --rl-time-size: 12px;
}

.rl--dark.rl--mega {
    --rl-panel-bg: #101318;
}

/* ── 头部（同样吃 token，不然和下面的行接不上） ─────────────────── */

.rl__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 14px 16px 10px;
    border-bottom: 1px solid var(--rl-border);
}

.rl__header-title {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
}

.rl__title {
    font-size: var(--rl-title-size);
    font-weight: 500;
    white-space: nowrap;
}

.rl__count {
    font-size: var(--rl-label-size);
    color: var(--rl-muted);
    border: 1px solid var(--rl-border);
    border-radius: calc(var(--rl-radius) * 0.6);
    padding: 1px 7px;
    white-space: nowrap;
}

.rl__header-actions {
    display: flex;
    align-items: center;
    gap: 2px;
    flex: none;
}

/* ── body ───────────────────────────────────────────────────────── */

/* 头部 ~64px + body 上限 = 容器上限，和拆分前两块的合计高度一致 */
.rl__body--dock {
    max-height: calc(100vh - 148px);
    overflow: auto;
    padding: 14px 16px 18px;
}

.rl__body--sheet {
    max-height: 62vh;
    overflow: auto;
    padding: 16px 20px 20px;
}

.rl__body {
    display: grid;
    gap: 14px;
    align-content: start;
}

/* ── 行结构（六套皮肤共用，不随模式变） ───────────────────────── */

.rl__toolbar {
    display: flex;
    align-items: center;
}

.rl__search {
    flex: 1;
}

/* 搜索框跟着模式走圆角。注意要写 `.rl__toolbar :deep(.v-field)` 而不是
   `.rl__search :deep(.v-field)` —— class 是挂在 v-text-field 根上的，
   那个根自己就是 .v-field，写成后代选择器等于要求它是自己的后代，永远不匹配。 */
.rl__toolbar :deep(.v-field) {
    border-radius: var(--rl-radius);
}

.rl__summary {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}

.rl__pill {
    font-size: var(--rl-label-size);
    color: var(--rl-muted);
    border: 1px solid var(--rl-border);
    border-radius: calc(var(--rl-radius) * 0.6);
    padding: 2px 8px;
    white-space: nowrap;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
}

.rl__pill--accent {
    color: var(--rl-current-accent);
    border-color: var(--rl-current-accent);
}

.rl__state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 32px 0;
}

.rl__state-text {
    font-size: var(--rl-name-size);
    color: var(--rl-muted);
}

/* minmax(0, 1fr) 不是可选的：单列 auto 轨道取 max-content，
   行内容一超过容器宽度轨道就跟着撑出去，整行横向溢出（右端的时间和收藏按钮被裁掉）。
   加行内计数时正是这样暴露的 —— 改前内容恰好没超，所以没显形。
   轨道能收缩还不够，grid 子项默认 min-width: auto 会继续顶着，所以 .rl-row 也要写 min-width: 0。 */
.rl__sections {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
    min-width: 0;
}

.rl-group {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 4px;
    min-width: 0;
}

.rl-group__label {
    font-size: var(--rl-label-size);
    font-weight: 500;
    letter-spacing: 0.08em;
    text-transform: var(--rl-label-transform);
    color: var(--rl-muted);
    padding: 0 2px 2px;
}

.rl-row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    min-height: var(--rl-row-h);
    padding: var(--rl-row-pad);
    border-radius: var(--rl-radius);
    cursor: pointer;
    /* --rl-sep 为 0 时不画线，为 1 时画一条 —— 用 calc 让它可切换，
       省得给每个模式再写一遍 border 规则 */
    border-bottom: calc(var(--rl-sep) * 1px) solid var(--rl-border);
    transition: background-color 0.15s ease;
}

.rl-row:hover {
    background: var(--rl-surface-hover);
}

.rl-row:focus-visible {
    outline: 2px solid var(--rl-current-accent);
    outline-offset: -2px;
}

.rl-row--current {
    background: var(--rl-current-bg);
}

/* 当前房间标记：默认是左侧色条 */
.rl-row__mark {
    flex: none;
    width: 3px;
    height: 55%;
    border-radius: 2px;
    background: transparent;
}

.rl-row--current .rl-row__mark {
    background: var(--rl-current-accent);
}

/* terminal 的当前房间标记是一个 > 前缀，跟等宽终端的语感一致。
   两行布局之后要 align-self: flex-start 顶到名字那一行 ——
   默认的垂直居中会让它落在名字和元信息之间，像第三行的东西。 */
.rl--terminal .rl-row__mark {
    width: 10px;
    height: auto;
    align-self: flex-start;
    background: transparent;
    border-radius: 0;
    font-size: var(--rl-name-size);
    line-height: 1.2;
}

.rl--terminal .rl-row--current .rl-row__mark::before {
    content: '>';
    color: var(--rl-current-accent);
}

/* sticky 的当前房间靠整行马卡龙底，再加色条就重复了 */
.rl--sticky .rl-row__mark {
    display: none;
}

/* 名字 + 元信息两行。名字那一行有活跃点/锁，元信息那一行是计数 + 时间。 */
.rl-row__text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
}

.rl-row__title {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
}

/* 名字必须能省略 —— 之前被 Vuetify 的 .v-list-item-title(nowrap) 顶掉
   word-break，长名字直接被外层 overflow:hidden 硬裁，两个房间看起来一模一样。
   现在它独占一行的宽度，不再和计数/时间抢。 */
.rl-row__name {
    flex: 0 1 auto;
    min-width: 0;
    font-size: var(--rl-name-size);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* 元信息行：计数 · 时间。整行 muted，子元素不再各写一遍字号和颜色。
   挤不下时优先截断的是计数（它可以少一段），时间永远留到最后 —— 它是这一行里最该看到的。
   gap 留 0：间距由 .rl-row__sep 自己带（两侧各 8px），否则「计数 / 点 / 时间」三个元素
   之间的两个间距会不一样宽。 */
.rl-row__meta {
    display: flex;
    align-items: center;
    gap: 0;
    min-width: 0;
    font-size: var(--rl-time-size);
    color: var(--rl-muted);
    white-space: nowrap;
    overflow: hidden;
}

/* 「计数 · 时间」中间那个点。只在计数真的渲染出来时才有 —— 
   两个数都是 0 的时候，这一行只剩时间，不该顶着一个孤零零的分隔符。 */
.rl-row__sep {
    flex: none;
    margin: 0 8px;
}

.rl-row__lock {
    flex: none;
    display: inline-flex;
    color: #b45309;
}

.rl--dark .rl-row__lock {
    color: #fbbf24;
}

/* 设备数 / 消息数。曾经被塞进 title 里省行高 —— 触屏没有 hover，
   在手机上等于彻底消失，所以放回行内。两个都是 0 时不渲染。 */
.rl-row__counts {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
}

/* 字号和颜色都由 .rl-row__meta 给，这里只管「别被压缩」 */
.rl-row__time {
    flex: none;
}

/* 只有活跃才有点。原来每个房间都写「非活跃」，八个里写七个，纯噪音 */
.rl-row__dot {
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #16a34a;
}

.rl--dark .rl-row__dot {
    background: #4ade80;
}

.rl-row__fav {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: calc(var(--rl-radius) * 0.5);
    color: var(--rl-muted);
    opacity: 0;
    transition: opacity 0.15s ease, color 0.15s ease;
}

.rl-row:hover .rl-row__fav,
.rl-row__fav:focus-visible,
.rl-row__fav--on {
    opacity: 1;
}

.rl-row__fav--on {
    color: #ff5252;
}

/* 触屏没有 hover，收藏按钮得一直可见，否则点不到 */
@media (hover: none) {
    .rl-row__fav {
        opacity: 0.75;
    }
}
</style>
