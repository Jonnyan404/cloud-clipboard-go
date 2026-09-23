<script setup>
// markdown 渲染结果的展示层。
//
// 单独抽出来是因为文本卡片和文件预览都要用它 —— 渲染逻辑在 useMarkdown，
// 样式在这里，两边各自只负责「什么时候渲染」。
//
// html 必须已经过 DOMPurify（见 util.js 的 renderMarkdownHtml）。
import { nextTick, onMounted, ref, watch } from 'vue';
import { highlightCode } from '@/highlight.js';

const props = defineProps({
    html: {
        type: String,
        default: '',
    },
});

const root = ref(null);

// 给 ``` 代码块上色。
//
// 为什么在**渲染之后**回头处理，而不是挂 marked 的 renderer：marked 的 renderer 是
// **同步**的，而高亮器是 `import()` 按需加载的 —— 在 renderer 里 await 不了。
// 所以先照常渲染（代码本来就是转义好的纯文本，看得见），再把这些块找出来上色。
//
// 认不出语言就原样留着（`language-` 那一段是 marked 写的）。上色失败也不影响阅读。
async function highlightCodeBlocks() {
    const el = root.value;
    if (!el) {
        return;
    }
    const blocks = el.querySelectorAll('pre > code[class*="language-"]');
    for (const block of blocks) {
        if (block.classList.contains('hljs')) {
            continue;
        }
        const lang = (block.className.match(/language-([\w+#.-]+)/) || [])[1] || '';
        const source = block.textContent || '';
        if (!lang || !source) {
            continue;
        }
        const html = await highlightCode(source, lang);
        if (html) {
            // ⚠️ 直接进 innerHTML：highlight.js 自己会转义输入，所以是安全的；
            // 也正因为如此，**不要再往上拼任何没转义的内容**。
            block.innerHTML = html;
            block.classList.add('hljs');
        }
    }
}

// v-html 更新完 DOM 之后才轮得到我们；html 一变就重新上色。
watch(
    () => props.html,
    () => {
        nextTick(highlightCodeBlocks);
    },
);
onMounted(highlightCodeBlocks);
</script>

<template>
    <div ref="root" class="markdown-body" v-html="html"></div>
</template>

<style scoped>
/* 底色用 --v-theme-on-surface 的透明度，明暗主题都自动适配（暗色下它是白色） */
.markdown-body {
    font-size: 0.875rem;
    line-height: 1.65;
    word-break: break-word;
}
.markdown-body > :deep(:first-child) {
    margin-top: 0;
}
.markdown-body > :deep(:last-child) {
    margin-bottom: 0;
}
.markdown-body :deep(p) {
    margin: 0 0 8px;
}
.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4),
.markdown-body :deep(h5),
.markdown-body :deep(h6) {
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.4;
    margin: 14px 0 6px;
}
.markdown-body :deep(h1) {
    font-size: 1.2rem;
}
.markdown-body :deep(h2) {
    font-size: 1.1rem;
}
.markdown-body :deep(ul),
.markdown-body :deep(ol) {
    margin: 0 0 8px;
    padding-left: 22px;
}
.markdown-body :deep(li) {
    margin: 2px 0;
}
.markdown-body :deep(code) {
    background: rgba(var(--v-theme-on-surface), 0.08);
    padding: 1px 5px;
    border-radius: 4px;
    font-size: 0.85em;
}
.markdown-body :deep(pre) {
    background: rgba(var(--v-theme-on-surface), 0.08);
    padding: 10px 12px;
    border-radius: 8px;
    overflow-x: auto;
    margin: 0 0 8px;
}
.markdown-body :deep(pre code) {
    background: none;
    padding: 0;
}
.markdown-body :deep(blockquote) {
    border-left: 3px solid rgba(var(--v-theme-on-surface), 0.25);
    padding-left: 10px;
    margin: 0 0 8px;
    opacity: 0.85;
}
.markdown-body :deep(table) {
    border-collapse: collapse;
    width: 100%;
    margin: 0 0 8px;
}
.markdown-body :deep(th),
.markdown-body :deep(td) {
    border: 1px solid rgba(var(--v-theme-on-surface), 0.2);
    padding: 4px 8px;
    text-align: left;
}
.markdown-body :deep(a) {
    color: rgb(var(--v-theme-primary));
}
.markdown-body :deep(img) {
    max-width: 100%;
    border-radius: 8px;
}
.markdown-body :deep(hr) {
    border: none;
    border-top: 1px solid rgba(var(--v-theme-on-surface), 0.2);
    margin: 12px 0;
}
</style>
