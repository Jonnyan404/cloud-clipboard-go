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
.code-block {
    /* 令牌配色：亮/暗两套，只覆盖十几个令牌，而不是引一整套主题 CSS。
       暗色靠 `.v-theme--dark`（Vuetify 挂在应用根上的类）切换。 */
    --cb-comment: #6a737d;
    --cb-keyword: #d73a49;
    --cb-string: #032f62;
    --cb-number: #005cc5;
    --cb-title: #6f42c1;
    --cb-attr: #005cc5;
    --cb-type: #e36209;
    --cb-meta: #6a737d;
    --cb-tag: #22863a;
    --cb-delete: #b31d28;
    --cb-insert: #22863a;

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
.v-theme--dark .code-block {
    --cb-comment: #7f848e;
    --cb-keyword: #c678dd;
    --cb-string: #98c379;
    --cb-number: #d19a66;
    --cb-title: #61afef;
    --cb-attr: #d19a66;
    --cb-type: #e5c07b;
    --cb-meta: #7f848e;
    --cb-tag: #e06c75;
    --cb-delete: #e06c75;
    --cb-insert: #98c379;
}
.code-block code {
    font-family: inherit;
    font-size: inherit;
}

/* ⚠️ v-html 注入的元素拿不到 scoped 的 data-v 属性，所以这些规则必须 :deep()。 */
.code-block :deep(.hljs-comment),
.code-block :deep(.hljs-quote) {
    color: var(--cb-comment);
    font-style: italic;
}
.code-block :deep(.hljs-keyword),
.code-block :deep(.hljs-selector-tag),
.code-block :deep(.hljs-literal),
.code-block :deep(.hljs-doctag),
.code-block :deep(.hljs-formula) {
    color: var(--cb-keyword);
}
.code-block :deep(.hljs-string),
.code-block :deep(.hljs-regexp),
.code-block :deep(.hljs-addition),
.code-block :deep(.hljs-char.escape) {
    color: var(--cb-string);
}
.code-block :deep(.hljs-number) {
    color: var(--cb-number);
}
.code-block :deep(.hljs-title),
.code-block :deep(.hljs-section),
.code-block :deep(.hljs-selector-id),
.code-block :deep(.hljs-selector-class) {
    color: var(--cb-title);
}
.code-block :deep(.hljs-attr),
.code-block :deep(.hljs-attribute),
.code-block :deep(.hljs-variable),
.code-block :deep(.hljs-template-variable),
.code-block :deep(.hljs-params) {
    color: var(--cb-attr);
}
.code-block :deep(.hljs-type),
.code-block :deep(.hljs-class),
.code-block :deep(.hljs-built_in),
.code-block :deep(.hljs-symbol),
.code-block :deep(.hljs-bullet),
.code-block :deep(.hljs-link) {
    color: var(--cb-type);
}
.code-block :deep(.hljs-meta) {
    color: var(--cb-meta);
}
.code-block :deep(.hljs-tag),
.code-block :deep(.hljs-name) {
    color: var(--cb-tag);
}
.code-block :deep(.hljs-deletion) {
    color: var(--cb-delete);
}
.code-block :deep(.hljs-emphasis) {
    font-style: italic;
}
.code-block :deep(.hljs-strong) {
    font-weight: 600;
}
</style>
