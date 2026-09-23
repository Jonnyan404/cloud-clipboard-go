<script setup>
// 代码块：按文件名（扩展名）选语言高亮，认不出来就按纯文本渲染。
//
// 为什么单独一个组件：分享页、速览预览、各个模式的阅读器都要「给一段源码上色」，
// 而「选语言 / 按需加载高亮器 / 失败回落纯文本」这套逻辑只该有一份。
import { ref, watch } from 'vue';
import { highlightCode } from '@/highlight.js';

const props = defineProps({
    // 源码正文
    text: { type: String, default: '' },
    // 文件名或路径 —— 只用来判语言（看扩展名）
    name: { type: String, default: '' },
});

const html = ref('');

// ⚠️ 竞态：换文件时上一次高亮可能还在飞（高亮器是动态 import 的）。
// 用一个自增序号挡住 —— 回来时序号已经不是当前那次，就丢掉，
// 否则会出现「B 的正文配 A 的高亮」。
let seq = 0;
watch(
    () => [props.text, props.name],
    async ([text, name]) => {
        const mine = ++seq;
        html.value = '';
        if (!text) {
            return;
        }
        const result = await highlightCode(text, name);
        if (mine === seq) {
            html.value = result || '';
        }
    },
    { immediate: true },
);
</script>

<template>
    <!-- 高亮成功走 v-html（highlight.js 自己转义过，安全）；否则退回纯文本插值。 -->
    <pre class="code-block"><code v-if="html" class="hljs" v-html="html"></code><code v-else>{{ text }}</code></pre>
</template>

<style scoped>
/* 只管布局与排版 —— **令牌配色在 `src/styles/highlight.css`**（全局一份，
   markdown 里的代码块要用同一套，写在 scoped 里那边就用不上了）。 */
.code-block {
    margin: 0;
    max-height: 62vh;
    overflow: auto;
    padding: 12px 14px;
    border-radius: 8px;
    background: rgba(var(--v-theme-on-surface), 0.04);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.8125rem;
    line-height: 1.6;
    color: rgba(var(--v-theme-on-surface), 0.87);
    /* 代码按行排，长行横向滚 —— 折行会把缩进层级弄乱，比滚动更难读。 */
    white-space: pre;
}
.code-block code {
    font-family: inherit;
    font-size: inherit;
}
</style>
