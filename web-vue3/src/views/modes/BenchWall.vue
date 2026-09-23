<script setup>
// 动作工作台（第 9 个模式，key `bench`）：左边时间流当**输入源**，右边是**动作链**和实时结果。
//
// 和速览的区别（两个都是主从两栏，容易混）：
//   速览是「**读**这条内容」—— 右边是渲染好的完整预览，只读。
//   工作台是「**加工**这条内容」—— 右边是一条可叠多步的动作链，链一变结果立刻重算。
//
// 为什么单独做一个模式而不是塞进标准模式：动作链需要**稳定的第二栏**。
// 挤在卡片里的话，链一长卡片就高得离谱，而时间流的节奏靠的正是「每条卡片一样高」。
//
// ⚠️ 只列**文本条目**。文件条目没有正文（FileReceive 只有 Name/Size/Cache/Expire/Thumbnail/URL，
// 取正文要另发 GET /file/<cache>/<name>），第一版不做 —— 列出来点进去是空的，比不列更糟。
//
// ⚠️ 这个模式**不改数据**。动作结果要落盘只能显式点「另存为新条目」（走现成的 POST /text）。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import PageToolbar from '@/components/PageToolbar.vue';
import ActionChain from '@/components/bench/ActionChain.vue';
import { decodeHtmlEntities, errorMessage, formatTimestamp } from '@/util.js';
import { toast } from '@/plugins/toast';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

// 宽屏断点 **1024**（和速览一致）：平板那一档也放不下并排两栏。
const isWide = ref(window.innerWidth >= 1024);
function onResize() {
    isWide.value = window.innerWidth >= 1024;
}
onMounted(() => {
    window.addEventListener('resize', onResize);
    // ⚠️ 监听挂 `window` 而不是列表上：焦点通常不在列表里（用户可能在输入框、或刚点完别处），
    // 挂在元素上收不到。这和速览是同一套做法。
    window.addEventListener('keydown', onKeydown);
});

onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize);
    window.removeEventListener('keydown', onKeydown);
});

// 数据源和标准模式/速览**共用**同一个 `app.visibleReceived`（已含搜索过滤）——
// 别再自己 filter 一遍 received，否则搜索框在这个模式里会失效。
const items = computed(() => app.visibleReceived.filter((item) => item.type === 'text'));

// 动作链的输入文本。
//
// ⚠️ **这是唯一的输入源**：点左边列表里的条目 = 「把它的正文填进这里」，
// 而不是「切换到另一种输入模式」。只留一条数据通路 —— 加这个输入框就是为了
// 「想试个动作，却得先往剪贴板发一条」这个调试痛点。
const draft = ref('');

// 列表容器的引用 —— 上下键切换后要把选中项滚进视野，得从它往下找。
const itemsEl = ref(null);

// 当前条目在列表里的下标。
//
// ⚠️ 上一轮把「选中态」删掉是对的（输入框是唯一输入源），但键盘导航需要一个
// 「我现在在哪一条」的位置 —— 所以它回来了，只是**只服务于上下键和视觉反馈**，
// 不再是「另一种输入模式」。
const activeIndex = ref(-1);

// 服务端存的是 HTML 实体编码过的正文（`<` 之类），填进来之前要还原回原文。
// 全站唯一实现在 util.js 的 decodeHtmlEntities —— 别在这里再写一份。
function fillFrom(item) {
    draft.value = decodeHtmlEntities(item.content || '');
}

/** 选中第 index 条：填进输入框 + 记下位置 + 把它滚进视野。 */
function selectIndex(index) {
    const list = items.value;
    if (index < 0 || index >= list.length) {
        return; // 到头 / 到尾就不动
    }
    activeIndex.value = index;
    fillFrom(list[index]);
    // 选中项得留在视野里，否则按住不放它就跑出屏幕了。
    // `block: 'nearest'` —— 只滚「刚好够看见」那一点，不会把整列翻过去。
    nextTick(() => {
        const rows = itemsEl.value?.querySelectorAll('.bench-wall__item');
        rows?.[index]?.scrollIntoView({ block: 'nearest' });
    });
}

/**
 * 上下键切换输入源条目。
 *
 * ⚠️ **焦点在输入框 / 可编辑元素里时不接管** —— 那时上下键是**移光标**，用户正在编辑文本。
 * 速览那边可以无脑接管（它的搜索框是单行的，上下键本来没别的含义），这里不行：
 * 动作台的输入框是**多行 textarea**。
 */
function onKeydown(event) {
    if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') {
        return;
    }
    const el = event.target;
    if (el && (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT' || el.isContentEditable)) {
        return;
    }
    // ⚠️ 必须拦掉默认动作：方向键默认是**滚容器**，不拦就会「换了条目 + 页面也滚了」。
    event.preventDefault();
    const delta = event.key === 'ArrowDown' ? 1 : -1;
    selectIndex(activeIndex.value < 0 ? 0 : activeIndex.value + delta);
}

// 首次有内容时自动填第一条 —— 一进这个模式就有东西可试。
// ⚠️ 只在**输入框还空着**时填，否则会把用户正在打的内容冲掉。
watch(
    items,
    (list) => {
        if (!draft.value && list.length) {
            activeIndex.value = 0;
            fillFrom(list[0]);
        }
    },
    { immediate: true },
);

function summary(item) {
    const text = decodeHtmlEntities(item.content || '').replace(/\s+/g, ' ').trim();
    return text || t('shareHistoryText');
}

/** 把动作结果另存为一条**新**条目。走的是现成的 `POST /text`，后端零改动。 */
async function saveAsNew(content) {
    const body = String(content ?? '');
    if (!body) {
        return;
    }
    try {
        await axios.post('text', body, {
            params: new URLSearchParams([['room', ws.room]]),
            // text/plain = 「整个 body 就是正文」那条分支（见服务端 POST /text 的 Content-Type 约定）
            headers: { 'Content-Type': 'text/plain' },
        });
        toast(t('actionSaveAsNewDone'));
    } catch (err) {
        toast(errorMessage(err) || t('sendFailed'));
    }
}
</script>

<template>
    <div class="bench-wall" :class="{ 'bench-wall--dark': isDark, 'bench-wall--wide': isWide }">
        <PageToolbar />

        <div class="bench-wall__body">
            <!-- 左：输入源 -->
            <aside class="bench-wall__list">
                <div class="bench-wall__list-head">
                    <v-icon size="14">mdi-inbox-arrow-down-outline</v-icon>
                    <span>{{ t('benchSourceTitle') }}</span>
                    <span class="bench-wall__count">{{ items.length }}</span>
                </div>

                <div ref="itemsEl" class="bench-wall__items">
                    <button
                        v-for="(item, index) in items"
                        :key="item.id"
                        type="button"
                        class="bench-wall__item"
                        :class="{ 'bench-wall__item--active': index === activeIndex }"
                        @click="selectIndex(index)"
                    >
                        <span class="bench-wall__item-time">{{ formatTimestamp(item.timestamp) }}</span>
                        <span class="bench-wall__item-text">{{ summary(item) }}</span>
                    </button>

                    <div v-if="!items.length" class="bench-wall__empty">
                        {{ t('benchEmpty') }}
                    </div>
                </div>
            </aside>

            <!-- 右：输入框 + 动作链 + 结果。
                 输入框放在这一栏的**顶部**而不是左栏 —— 左栏那点宽度写长文本太憋屈，
                 而右边本来就是「工作区」（输入 → 加工 → 结果），三样在同一条竖线上。 -->
            <section class="bench-wall__panel">
                <textarea
                    v-model="draft"
                    class="bench-wall__draft"
                    :placeholder="t('benchDraftPlaceholder')"
                    spellcheck="false"
                ></textarea>

                <ActionChain v-if="draft" class="bench-wall__chain" :text="draft" @save-as-new="saveAsNew" />
                <div v-else class="bench-wall__empty bench-wall__empty--panel">
                    {{ t('benchDraftEmpty') }}
                </div>
            </section>
        </div>
    </div>
</template>

<style scoped>
.bench-wall {
    display: flex;
    flex-direction: column;
    /* dvh 而不是 vh：移动端地址栏收起/展开会改可视高度，用 vh 会让底部被裁掉 */
    height: 100dvh;
    overflow: hidden;
}

.bench-wall__body {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 10px 12px 14px;
}

/* ⚠️ 两处 CSS 一起才能让内部真的滚起来：
   容器给 grid/flex 项 minmax(0,1fr)（或 min-height:0），**子项**还要再写 min-height:0 ——
   否则子项的默认 min-height:auto 会把它撑到内容高度，滚动条永远不出现。 */
.bench-wall__list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    /* 窄屏：列表只占三成 —— 这一屏的主体是右边的「输入 → 加工 → 结果」，
       列表只是取数据的入口。38% 会把工作区挤得只剩一半，动作链和结果都看不全。 */
    flex: 0 0 30%;
    border: 1px solid rgba(148, 163, 184, 0.26);
    border-radius: 16px;
    background: rgba(255, 255, 255, 0.9);
    overflow: hidden;
}

.bench-wall__panel {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border: 1px solid rgba(148, 163, 184, 0.26);
    border-radius: 16px;
    background: rgba(255, 255, 255, 0.9);
    padding: 12px;
    overflow: hidden;
}

/* 宽屏：左右两栏 */
.bench-wall--wide .bench-wall__body {
    flex-direction: row;
}

.bench-wall--wide .bench-wall__list {
    flex: 0 0 320px;
}

.bench-wall__list-head {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 9px 12px;
    border-bottom: 1px solid rgba(148, 163, 184, 0.26);
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    color: rgba(71, 85, 105, 0.75);
    flex: 0 0 auto;
}

.bench-wall__count {
    margin-left: auto;
    font-weight: 500;
}

/* 手输的调试输入框 —— 在**右栏顶部**。固定高度 + 允许纵向拖拽：
   长文本要能看全几行，但不能把下面的动作链挤没。
   `color: inherit` 是必须的：textarea 默认是黑色，暗色主题下会看不见。
   背景用 transparent（跟着面板走），这样暗色主题不必再写一条覆盖规则。 */
.bench-wall__draft {
    flex: 0 0 auto;
    height: 88px;
    min-height: 44px;
    resize: vertical;
    border: 1px solid rgba(148, 163, 184, 0.42);
    border-radius: 10px;
    background: transparent;
    padding: 8px 10px;
    margin-bottom: 10px;
    font-family: inherit;
    font-size: 0.75rem;
    line-height: 1.55;
    color: inherit;
    outline: none;
}

.bench-wall__draft:focus {
    border-color: rgb(var(--v-theme-primary));
}

.bench-wall__draft::placeholder {
    color: rgba(71, 85, 105, 0.5);
}

/* 动作链占满输入框剩下的高度（它内部再分「链」和「结果」两块） */
.bench-wall__chain {
    flex: 1;
    min-height: 0;
}

.bench-wall__items {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 6px;
}

.bench-wall__item {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    text-align: left;
    padding: 7px 9px;
    border: 1px solid transparent;
    border-radius: 9px;
    background: transparent;
    cursor: pointer;
    font-size: 0.75rem;
    color: inherit;
    transition: background-color 0.15s ease, border-color 0.15s ease;
}

.bench-wall__item:hover {
    background: rgba(14, 165, 233, 0.07);
}

/* 当前条目（点选的，或上下键选中的那条）。它同时是「输入框里那段文字来自哪」的指示 ——
   没有这个高亮，用上下键切换时界面上看不出选中的是哪一条。 */
.bench-wall__item--active {
    background: rgba(var(--v-theme-primary), 0.12);
    border-color: rgba(var(--v-theme-primary), 0.45);
}

.bench-wall__item-time {
    flex: 0 0 auto;
    font-size: 0.625rem;
    color: rgba(71, 85, 105, 0.7);
    font-variant-numeric: tabular-nums;
}

.bench-wall__item-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.bench-wall__empty {
    padding: 24px 8px;
    text-align: center;
    font-size: 0.75rem;
    color: rgba(71, 85, 105, 0.7);
}

.bench-wall__empty--panel {
    margin: auto;
}

.bench-wall--dark .bench-wall__list,
.bench-wall--dark .bench-wall__panel {
    background: rgba(15, 23, 42, 0.9);
    border-color: rgba(71, 85, 105, 0.72);
}

.bench-wall--dark .bench-wall__list-head,
.bench-wall--dark .bench-wall__item-time,
.bench-wall--dark .bench-wall__empty {
    color: rgba(226, 232, 240, 0.72);
}
</style>
