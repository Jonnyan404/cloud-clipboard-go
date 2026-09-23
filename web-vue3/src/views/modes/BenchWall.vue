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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
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
onMounted(() => window.addEventListener('resize', onResize));
onBeforeUnmount(() => window.removeEventListener('resize', onResize));

// 数据源和标准模式/速览**共用**同一个 `app.visibleReceived`（已含搜索过滤）——
// 别再自己 filter 一遍 received，否则搜索框在这个模式里会失效。
const items = computed(() => app.visibleReceived.filter((item) => item.type === 'text'));

// 动作链的输入文本。
//
// ⚠️ **这是唯一的输入源**：点左边列表里的条目 = 「把它的正文填进这里」，
// 而不是「切换到另一种输入模式」。只留一条数据通路 —— 加这个输入框就是为了
// 「想试个动作，却得先往剪贴板发一条」这个调试痛点。
const draft = ref('');

// 服务端存的是 HTML 实体编码过的正文（`<` 之类），填进来之前要还原回原文。
// 全站唯一实现在 util.js 的 decodeHtmlEntities —— 别在这里再写一份。
function fillFrom(item) {
    draft.value = decodeHtmlEntities(item.content || '');
}

// 首次有内容时自动填第一条 —— 一进这个模式就有东西可试。
// ⚠️ 只在**输入框还空着**时填，否则会把用户正在打的内容冲掉。
watch(
    items,
    (list) => {
        if (!draft.value && list.length) {
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

                <div class="bench-wall__items">
                    <button
                        v-for="item in items"
                        :key="item.id"
                        type="button"
                        class="bench-wall__item"
                        @click="fillFrom(item)"
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
    flex: 0 0 38%;
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
