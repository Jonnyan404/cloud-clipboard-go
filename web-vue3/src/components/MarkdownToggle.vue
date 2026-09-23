<script setup>
// 预览框右上角那排「用哪种方式看」的图标 —— **动作库驱动**。
//
// 以前是四个写死的槽位（原文 / md-或-代码 / JSON 美化 / JSON 压缩）。现在图标来自
// data/actions.js 的注册表：**有针对性且命中**的动作直接露图标（最多 2 个），
// 其余的收进 `⋯` 面板 —— 面板里还有全部通用动作（转大写、编解码…）。
//
// ⚠️ **这里不放复制** —— 复制统一走卡片上原来那个复制图标（它会复制**当前视图**的内容，
// 见各消费方对 `md.copyText` 的用法）。同一个卡片上两个复制按钮、行为还容易不一致。
//
// 纯图标，不带任何按钮外框：以前用 v-btn-group 套两个小按钮，
// 外框再怎么压也有 ~22px 高，叠在单行内容（~25px）上还会把内容区撑出滚动条。
//
// 浮动定位（absolute）由组件自己带，所以调用方要满足三件事：
//   1. 外面套一层 position: relative 且**不滚动**的盒子 —— 图标才不会跟着内容滚走；
//   2. 真正滚动的那个盒子上留出 `--md-toggle-gutter`（见文件末尾的全局变量），
//      否则图标会盖住第一行行尾；
//   3. 那层盒子上别挂 overflow —— 0 高 + overflow 会把图标整个裁掉，且不报错。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ActionPicker from '@/components/bench/ActionPicker.vue';

const props = defineProps({
    // `null` = 原文；否则是动作 id
    mode: {
        type: String,
        default: null,
    },
    // 这条内容**有针对性**的动作（match 命中的），按注册顺序
    actions: {
        type: Array,
        default: () => [],
    },
});

const emit = defineEmits(['update:mode']);
const { t } = useI18n();

const mdiCodeTags = 'mdi-code-tags';
const mdiDotsHorizontal = 'mdi-dots-horizontal';

const menuOpen = ref(false);

// 卡片上直接露图标的动作：**最多 2 个**。
// 留一个位置给 ⋯ —— 图标总数封在 3 个，--md-toggle-gutter 的两档（64 / 96px）才够用。
const inlineActions = computed(() => props.actions.slice(0, 2));
const hasMore = computed(() => props.actions.length > 2);

function choose(id) {
    emit('update:mode', id);
    menuOpen.value = false;
}
</script>

<template>
    <div class="md-toggle">
        <!-- 原文。`null` 不是动作 —— 它表示「不跑任何动作」。 -->
        <v-tooltip :text="t('rawText')" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === null }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', null)"
                    @keydown.enter.prevent="emit('update:mode', null)"
                    @keydown.space.prevent="emit('update:mode', null)"
                >
                    <v-icon size="20">{{ mdiCodeTags }}</v-icon>
                </span>
            </template>
        </v-tooltip>

        <!-- 这条内容**用得着**的动作 -->
        <v-tooltip v-for="action in inlineActions" :key="action.id" :text="t(action.nameKey)" location="top">
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    :class="{ 'md-toggle__icon--active': mode === action.id }"
                    role="button"
                    tabindex="0"
                    @click="emit('update:mode', action.id)"
                    @keydown.enter.prevent="emit('update:mode', action.id)"
                    @keydown.space.prevent="emit('update:mode', action.id)"
                >
                    <v-icon size="20">{{ action.icon }}</v-icon>
                </span>
            </template>
        </v-tooltip>

        <!-- 其余动作（含全部通用动作）。
             ⚠️ 复用工作台的 ActionPicker —— 同一套分组和搜索，**别在这里再造一个简化版菜单**，
             两份迟早会漂（这个仓库在「同一份逻辑抄了几份」上栽过好几次）。 -->
        <v-menu
            v-if="hasMore"
            v-model="menuOpen"
            location="bottom end"
            :close-on-content-click="false"
            :offset="6"
        >
            <template v-slot:activator="{ props: activatorProps }">
                <span
                    v-bind="activatorProps"
                    class="md-toggle__icon"
                    role="button"
                    tabindex="0"
                    :title="t('actionMore')"
                >
                    <v-icon size="20">{{ mdiDotsHorizontal }}</v-icon>
                </span>
            </template>
            <div class="md-toggle__panel">
                <ActionPicker @pick="choose" />
            </div>
        </v-menu>
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
       这里 12px vs 8px 的滚动条。 */
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

/* ⋯ 展开的动作面板。给固定尺寸 —— 不固定的话菜单会随内容宽高乱跳，
   而且 ActionPicker 内部是「搜索框 + 可滚动的分组区」，需要一个有界的高度才滚得起来。 */
.md-toggle__panel {
    display: flex;
    flex-direction: column;
    width: 340px;
    max-width: calc(100vw - 32px);
    max-height: 320px;
    padding: 10px;
    overflow: hidden;
}
</style>

<style>
/* 故意不写 scoped：这是本组件对外的一条约定的宽度 ——
   图标（每个 20px 字形 + 6px 内边距）加右缩进，就是滚动内容要让出的宽度。
   消费方在**滚动盒**上写 `padding-right: var(--md-toggle-gutter)`。

   ⚠️ 图标**最多三个**（原文 + 2 个针对性动作 + ⋯），所以只有两个值。
   消费方**别自己去数图标** —— 把 `useMarkdown` 的 `gutter` 绑到 `--md-toggle-gutter` 上即可，
   数图标那件事在那边一处做完（见 iconCount）。

   注意别把它加在不滚动的外层盒子上：滚动条不会跟着让位，白加。 */
:root {
    --md-toggle-gutter: 64px;      /* 两个图标 */
    --md-toggle-gutter-wide: 96px; /* 三个图标 */
    /* 让位的**高度**：`1lh` = 该容器自己的一个行高 —— 所以无论字号/行高是多少，
       都只让开**一行**。（`lh` 是较新的单位，消费方会再写一个 px 兜底。）
       消费方用 `::before` 做一个 float 占位块，宽 × 高就是这两个值：
       只有和图标垂直重叠的那一行会绕开，下面的行恢复整宽。
       ⚠️ 正文以 `<pre>` 开头时**不能用浮动占位** —— 见消费方 `--block` 那条注释。 */
    --md-toggle-height: 1lh;
}
</style>
