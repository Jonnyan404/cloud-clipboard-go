<script setup>
import { useI18n } from 'vue-i18n';

// 预览框右上角那排「用哪种方式看」的切换图标。
//
// 槽位是**按内容决定**的（最多三个）：
//   1. 原文        —— 永远有
//   2. 代码 / md   —— 像代码给「代码」，否则像 markdown 给 md。
//                     两者不会同时出现：JSON 渲染成 markdown 和原文一模一样，
//                     而**代码过 markdown 会被重排**（`*` `_` `#` `-` 会被当标记，
//                     一段 Go 点「md 渲染」整段缩进和注释就乱了）。
//   3. JSON 美化   —— 内容是 JSON 才有
//   4. JSON 压缩   —— 内容是 JSON 且**压得动**才有
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
    // 内容是「能美化的 JSON」→ 多一个「美化」槽位
    jsonAvailable: {
        type: Boolean,
        default: false,
    },
    // 内容是 JSON 且**压得动**（本来就不是一行）→ 多一个「压缩」槽位
    jsonCompactAvailable: {
        type: Boolean,
        default: false,
    },
    // 内容像源码 → 第二个槽位给「代码」而不是 md
    codeAvailable: {
        type: Boolean,
        default: false,
    },
});
const emit = defineEmits(['update:mode']);
const { t } = useI18n();

const mdiCodeTags = 'mdi-code-tags';
const mdiLanguageMarkdown = 'mdi-language-markdown';
const mdiCodeBraces = 'mdi-code-braces';
const mdiIndentIncrease = 'mdi-format-indent-increase';
const mdiIndentDecrease = 'mdi-format-indent-decrease';
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

        <v-tooltip v-if="codeAvailable" :text="t('codeView')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === 'code' }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', 'code')"
                    @keydown.enter.prevent="emit('update:mode', 'code')"
                    @keydown.space.prevent="emit('update:mode', 'code')"
                >
                    <v-icon size="20">{{ mdiCodeBraces }}</v-icon>
                </span>
            </template>
        </v-tooltip>
        <v-tooltip v-else :text="t('renderMarkdown')" location="top">
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

        <v-tooltip v-if="jsonAvailable" :text="t('beautifyJson')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === 'json' }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', 'json')"
                    @keydown.enter.prevent="emit('update:mode', 'json')"
                    @keydown.space.prevent="emit('update:mode', 'json')"
                >
                    <v-icon size="20">{{ mdiIndentIncrease }}</v-icon>
                </span>
            </template>
        </v-tooltip>

        <v-tooltip v-if="jsonCompactAvailable" :text="t('minifyJson')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === 'json-min' }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', 'json-min')"
                    @keydown.enter.prevent="emit('update:mode', 'json-min')"
                    @keydown.space.prevent="emit('update:mode', 'json-min')"
                >
                    <v-icon size="20">{{ mdiIndentDecrease }}</v-icon>
                </span>
            </template>
        </v-tooltip>
    </div>
</template>

<style scoped>
/* 浮在预览框右上角：不占布局，内容该多高还多高。
   无底色、无阴影、无边框 —— 就是几个字形，叠在任何底色（便签黄/卡片白/暗色）上都成立。
   高度 < 30px，永远矮于单行行高，不可能再撑出内容区的滚动条。 */
.md-toggle {
    position: absolute;
    /* 只让开一行，所以图标尽量贴顶：top 越小，它下面那行越不容易擦到字形。 */
    top: 2px;
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
   图标（每个 20px 字形 + 6px 内边距）加右缩进，就是滚动内容要让出的宽度。
   消费方在**滚动盒**上写 `padding-right: var(--md-toggle-gutter)`。

   ⚠️ 图标**最多三个**（原文 + md/代码 + JSON 美化/压缩），所以有两个值。
   消费方**别自己去数图标** —— 把 `useMarkdown` 的 `gutter` 绑到 `--md-toggle-gutter` 上即可，
   数图标那件事在那边一处做完（见 iconCount）。

   注意别把它加在不滚动的外层盒子上：滚动条不会跟着让位，白加。 */
:root {
    --md-toggle-gutter: 64px;      /* 两个图标 */
    --md-toggle-gutter-wide: 96px; /* 三个图标（内容是 JSON 时） */
    /* 让位的**高度**：`1lh` = 该容器自己的一个行高 —— 所以无论字号/行高是多少，
       都只让开**一行**。（`lh` 是较新的单位，消费方会再写一个 px 兜底。）
       消费方用 `::before` 做一个 float 占位块，宽 × 高就是这两个值：
       只有和图标垂直重叠的那一行会绕开，下面的行恢复整宽。
       （以前是给整个容器 padding-right，等于每一行都压窄 64px，图标下方的宽度全浪费。）
       ⚠️ 正文以 `<pre>` 开头时**不能用浮动占位** —— 见消费方 `--block` 那条注释。 */
    --md-toggle-height: 1lh;
}
</style>
