<script setup>
// 看板卡片的正文。
//
// 抽成组件不是为了复用，是因为 useMarkdown / useTaskListToggle 都是**按条目**的 composable
// （各自持 ref 和防抖计时器），必须一条一个实例 —— 在 v-for 里面调不了。
//
// 约定跟标准模式卡片、便签卡片完全一致：**任务列表 / 表格默认渲染成 md**（见 useMarkdown），
// 普通文本仍走原文预览 —— 看板卡片面积小，把 `**加粗**` 渲染出来反而更花。
// 渲染成 md 时复选框可点，落盘走 POST /text?id=<id>（见 useTaskListToggle）。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useWebSocketStore } from '@/store/websocket';
import { useMarkdown } from '@/composables/useMarkdown.js';
import { useTaskListToggle } from '@/composables/useTaskListToggle.js';
import MarkdownBody from '@/components/MarkdownBody.vue';

const props = defineProps({
    meta: { type: Object, required: true },
});

const ws = useWebSocketStore();
const { t } = useI18n();

// 房间要**现取**：传值的话，切换房间之后打勾会发到上一个房间。
const { text, onMdClick } = useTaskListToggle(props.meta, () => ws.room);
const md = useMarkdown(() => text.value);
const preview = computed(() => text.value.trim() || t('emptyHere'));
</script>

<template>
    <div
        class="board-card-body"
        :class="{ 'board-card-body--rendered': !!md.html }"
        @click="onMdClick"
    >
        <markdown-body v-if="md.html" :html="md.html"></markdown-body>
        <template v-else>{{ preview }}</template>
    </div>
</template>

<style scoped>
.board-card-body {
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 12.5px;
    line-height: 1.5;
}

/* 渲染成 md 时放开行数限制：一个任务列表被截到 4 行，后面的复选框就点不到了。 */
.board-card-body--rendered {
    display: block;
    white-space: normal;
    /* 表格宽度按内容走，宽表别把卡片（连带整列）顶开 —— 在卡片里横向滚。 */
    overflow-x: auto;
}

/* ⚠️ 用**两个类**（`.a.a--b`）压过 MarkdownBody 自己的 `.markdown-body`：
   两者特异性相同时比的是打包顺序，不是谁在文件里更靠后。看板卡片比通用 md 区域小，
   字号得跟着卡片走，否则一段 md 在卡片里显得特别大。 */
.board-card-body.board-card-body--rendered :deep(.markdown-body) {
    font-size: 12.5px;
    line-height: 1.5;
}
</style>
