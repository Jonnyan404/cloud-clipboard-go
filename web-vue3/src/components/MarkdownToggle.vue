<script setup>
import { useI18n } from 'vue-i18n';

// 预览框右上角那对「原文 / Markdown」切换图标。
//
// 纯图标，不带任何按钮外框：之前用 v-btn-group 套两个小按钮，
// 外框再怎么压也有 ~22px 高，叠在单行内容（~25px）上还会把内容区撑出滚动条。
//
// 浮动定位（absolute）由组件自己带，所以调用方要满足三件事：
//   1. 外面套一层 position: relative 且**不滚动**的盒子 —— 图标才不会跟着内容滚走；
//   2. 真正滚动的那个盒子上留出 `--md-toggle-gutter`（见文件末尾的全局变量），
//      否则图标会盖住第一行行尾；
//   3. 那层盒子上别挂 overflow —— 0 高 + overflow 会把图标整个裁掉，且不报错。
defineProps({
    mode: {
        type: String,
        default: 'raw',
    },
});
const emit = defineEmits(['update:mode']);
const { t } = useI18n();

const mdiCodeTags = 'mdi-code-tags';
const mdiLanguageMarkdown = 'mdi-language-markdown';
</script>

<template>
    <div class="md-toggle">
        <v-tooltip :text="t('rawText')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === 'raw' }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', 'raw')"
                    @keydown.enter.prevent="emit('update:mode', 'raw')"
                    @keydown.space.prevent="emit('update:mode', 'raw')"
                >
                    <v-icon size="20">{{ mdiCodeTags }}</v-icon>
                </span>
            </template>
        </v-tooltip>
        <v-tooltip :text="t('renderMarkdown')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === 'md' }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', 'md')"
                    @keydown.enter.prevent="emit('update:mode', 'md')"
                    @keydown.space.prevent="emit('update:mode', 'md')"
                >
                    <v-icon size="20">{{ mdiLanguageMarkdown }}</v-icon>
                </span>
            </template>
        </v-tooltip>
    </div>
</template>

<style scoped>
/* 浮在预览框右上角：不占布局，内容该多高还多高。
   无底色、无阴影、无边框 —— 就是两个字形，叠在任何底色（便签黄/卡片白/暗色）上都成立。
   高度 < 30px，永远矮于单行行高，不可能再撑出内容区的滚动条。 */
.md-toggle {
    position: absolute;
    top: 4px;
    /* 右缩进必须**大于滚动条宽度**：滚动条永远贴着滚动盒的右沿，
       缩进不够就会压在它上面（便签阅读器的「图标盖住滚动条」就是这么来的）。
       这里 12px vs 8px 的滚动条，见下面各消费方的 ::-webkit-scrollbar。 */
    right: 12px;
    z-index: 2;
    display: inline-flex;
    align-items: center;
    gap: 2px;
}

.md-toggle__icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 2px 3px;
    border-radius: 4px;
    cursor: pointer;
    opacity: 0.55;
    color: inherit;
}

.md-toggle__icon:hover {
    opacity: 1;
}

.md-toggle__icon--active {
    opacity: 1;
    color: rgb(var(--v-theme-primary));
}
</style>

<style>
/* 故意不写 scoped：这是本组件对外的一条约定的宽度 ——
   两个图标（2 × 20px 字形 + 内边距）加右缩进，正好是滚动内容要让出的宽度。
   消费方在**滚动盒**上写 `padding-right: var(--md-toggle-gutter)`，
   以后调图标尺寸只改这里一处，不会三个组件各留一个对不上的魔法数字。

   注意别把它加在不滚动的外层盒子上：滚动条不会跟着让位，白加。 */
:root {
    --md-toggle-gutter: 64px;
}
</style>
