<script setup>
// 动作链：把选中的正文按顺序喂给一串动作，右边实时显示结果。
//
// 这是方案 B 的主体 —— 「可叠多步」就体现在这里：链上有几步、什么顺序，都由用户当场决定。
//
// ⚠️ 结果区**默认只显示链末端的输出**。中间步骤的价值在「链对不对」，
// 不在「每一步长什么样」；把每步结果都铺出来会让这一栏变成一坨日志。
// 每一步只带一个字数变化（`62 → 47`），够判断这一步有没有起作用。
//
// ⚠️ 动作**不改数据**。这里从头到尾只读正文，产出的结果要落盘必须显式点「另存为新条目」。
// 理由：`POST /text?id=` 会刷新 timestamp，覆盖原条目会让它在时间流里跳到最前面
// （看板挪列时专门避开过同一个坑）。看，就该只是看。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { runChain } from '@/data/actions.js';
import { useActionChain } from '@/composables/useActionChain.js';
import { copyTextToClipboard } from '@/util.js';
import { toast } from '@/plugins/toast';
import ActionPicker from './ActionPicker.vue';

const props = defineProps({
    text: { type: String, default: '' },
});

const emit = defineEmits(['save-as-new']);

const mdiBookmarkOutline = 'mdi-bookmark-outline';
const mdiContentCopy = 'mdi-content-copy';
const mdiContentSaveOutline = 'mdi-content-save-outline';
const mdiContentSavePlusOutline = 'mdi-content-save-plus-outline';

const { t } = useI18n();
const { chain, steps, templates, add, removeAt, clear, move, saveTemplate, applyTemplate, removeTemplate } = useActionChain();

const naming = ref(false);
const templateName = ref('');

// ⚠️ 结果是**异步**取的（哈希走 crypto.subtle 只有 Promise），而且必须防竞态：
// 在列表里快速点几条、或者连着删几个动作时，先发起的计算可能后回来 ——
// 那样右边显示的是**上一条内容**的结果，界面上还看不出来。用一个自增序号挡掉。
const result = ref({ steps: [], output: '', error: '' });
let runSeq = 0;

watch(
    [() => props.text, chain],
    async () => {
        const seq = ++runSeq;
        const next = await runChain(props.text, chain.value, { t });
        if (seq === runSeq) {
            result.value = next;
        }
    },
    { immediate: true, deep: true },
);

// 链空着 = 看原文。这不是「没结果」，是「还没选动作」。
const isEmptyChain = computed(() => chain.value.length === 0);
const hasError = computed(() => Boolean(result.value.error));

// 结果怎么渲染，看**最后一步**：
//   1. 动作给了 `html`（双表示，如注音制表）→ 直接用它；
//   2. 动作声明了 `render: 'html'`（md 渲染 / 代码高亮）→ output 本身就是 HTML；
//   3. 其余（JSON、编解码、文本处理）是纯文本 → 走 <pre>，才不会丢掉空白和缩进。
const renderedHtml = computed(() => {
    const last = result.value.steps[result.value.steps.length - 1];
    if (!last) {
        return '';
    }
    if (last.html) {
        return last.html;
    }
    return last.action?.render === 'html' ? last.output : '';
});

const displayText = computed(() => {
    if (isEmptyChain.value) {
        return props.text;
    }
    if (hasError.value) {
        return '';
    }
    return result.value.output;
});

function pick(id) {
    add(id);
    // 选完不收起面板：连着叠三步是常态，每步都要重新展开太烦。
}

async function copyResult() {
    try {
        await copyTextToClipboard(displayText.value);
        toast(t('copySuccess'));
    } catch {
        toast(t('copyFailedGeneral'));
    }
}

function confirmSaveTemplate() {
    const tpl = saveTemplate(templateName.value);
    if (tpl) {
        naming.value = false;
        templateName.value = '';
        toast(t('actionTemplateSaved'));
    }
}

// 链上每一步的「字数变化」。用字符数而不是字节 —— 用户看的是正文。
//
// ⚠️ 必须容忍 `undefined`：链走到第 N 步失败时 runChain 会**停在那里**，
// 于是 `result.steps` 比链本身短。用链的下标去索引它会越界，直接崩在 step.input 上。
function stepDelta(step) {
    if (!step) {
        return '';
    }
    const before = Array.from(step.input || '').length;
    const after = Array.from(step.output || '').length;
    return `${before} → ${after}`;
}
</script>

<template>
    <div class="action-chain">
        <!-- 链本体 -->
        <div class="action-chain__head">
            <span class="action-chain__title">{{ t('actionChainTitle') }}</span>
            <v-spacer />
            <v-btn
                v-if="chain.length"
                size="x-small"
                variant="text"
                density="comfortable"
                @click="clear"
            >
                {{ t('actionChainClear') }}
            </v-btn>
        </div>

        <div class="action-chain__steps">
            <div v-if="isEmptyChain" class="action-chain__hint">{{ t('actionChainEmptyHint') }}</div>

            <div v-for="(step, index) in steps" :key="`${step.id}-${index}`" class="action-chain__step">
                <span class="action-chain__step-n">{{ index + 1 }}</span>
                <v-icon size="14">{{ step.action.icon }}</v-icon>
                <span class="action-chain__step-name">{{ t(step.action.nameKey) }}</span>
                <span class="action-chain__step-delta">{{ stepDelta(result.steps[index]) }}</span>
                <span class="action-chain__step-actions">
                    <button type="button" class="action-chain__mini" :disabled="index === 0" :title="t('actionChainMoveUp')" @click="move(index, -1)">↑</button>
                    <button type="button" class="action-chain__mini" :disabled="index === steps.length - 1" :title="t('actionChainMoveDown')" @click="move(index, 1)">↓</button>
                    <button type="button" class="action-chain__mini action-chain__mini--danger" :title="t('delete')" @click="removeAt(index)">×</button>
                </span>
            </div>
        </div>

        <!-- 「添加动作」用**弹出面板**，不内联展开：
             内联那块（ActionPicker 最高 260px）在窄屏上会把整栏挤爆，
             「添加动作」按钮自己就被顶出可视区（用户报的）；桌面端也会把结果区推下去。
             弹层由 Vuetify teleport 到 body，不受父级 overflow 影响。
             `:close-on-content-click="false"` → 选完不关，连着叠三步不用重新打开。 -->
        <div class="action-chain__add">
            <v-menu location="bottom start" :close-on-content-click="false" :offset="6">
                <template v-slot:activator="{ props: menuProps }">
                    <v-btn
                        v-bind="menuProps"
                        size="small"
                        variant="tonal"
                        block
                        prepend-icon="mdi-plus"
                    >
                        {{ t('actionChainAdd') }}
                    </v-btn>
                </template>
                <div class="action-chain__picker">
                    <ActionPicker :text="text" direction="view" @pick="pick" />
                </div>
            </v-menu>
        </div>

        <!-- 模板 -->
        <div class="action-chain__templates">
            <v-menu location="bottom start" :close-on-content-click="true">
                <template v-slot:activator="{ props: menuProps }">
                    <v-btn v-bind="menuProps" size="x-small" variant="text" :prepend-icon="mdiBookmarkOutline" :disabled="!templates.length">
                        {{ t('actionTemplateApply') }}
                    </v-btn>
                </template>
                <v-list density="compact">
                    <v-list-item v-for="tpl in templates" :key="tpl.id" @click="applyTemplate(tpl.id)">
                        <v-list-item-title>{{ tpl.name }}</v-list-item-title>
                        <v-list-item-subtitle>{{ tpl.steps.length }} {{ t('actionChainSteps') }}</v-list-item-subtitle>
                        <template v-slot:append>
                            <v-btn
                                icon
                                size="x-small"
                                variant="text"
                                :title="t('delete')"
                                @click.stop="removeTemplate(tpl.id)"
                            >
                                <v-icon size="16">mdi-close</v-icon>
                            </v-btn>
                        </template>
                    </v-list-item>
                </v-list>
            </v-menu>

            <v-btn
                size="x-small"
                variant="text"
                :prepend-icon="mdiContentSaveOutline"
                :disabled="!chain.length"
                @click="naming = !naming"
            >
                {{ t('actionTemplateSave') }}
            </v-btn>
        </div>

        <div v-if="naming" class="action-chain__naming">
            <v-text-field
                v-model="templateName"
                density="compact"
                variant="outlined"
                hide-details
                single-line
                :placeholder="t('actionTemplateNamePlaceholder')"
                @keydown.enter.prevent="confirmSaveTemplate"
            />
            <v-btn size="small" variant="tonal" :disabled="!templateName.trim()" @click="confirmSaveTemplate">
                {{ t('confirm') }}
            </v-btn>
        </div>

        <v-divider class="my-3" />

        <!-- 结果 -->
        <div class="action-chain__head">
            <span class="action-chain__title">{{ t('actionResultTitle') }}</span>
            <v-spacer />
            <v-btn
                v-if="!hasError"
                size="x-small"
                variant="text"
                :prepend-icon="mdiContentCopy"
                :disabled="!displayText"
                @click="copyResult"
            >
                {{ t('copyText') }}
            </v-btn>
            <v-btn
                v-if="!hasError && !isEmptyChain"
                size="x-small"
                variant="text"
                :prepend-icon="mdiContentSavePlusOutline"
                @click="emit('save-as-new', displayText)"
            >
                {{ t('actionSaveAsNew') }}
            </v-btn>
        </div>

        <div class="action-chain__result">
            <!-- 动作失败：**明确说出来** + 给一条退路。静默失败最糟 —— 用户会以为内容本身坏了。 -->
            <div v-if="hasError" class="action-chain__error">
                <v-icon size="16" color="error">mdi-alert-circle-outline</v-icon>
                <span>{{ result.error }}</span>
            </div>

            <div v-else-if="renderedHtml" class="action-chain__html" v-html="renderedHtml"></div>

            <pre v-else-if="displayText" class="action-chain__pre">{{ displayText }}</pre>

            <div v-else class="action-chain__empty">{{ t('actionResultEmpty') }}</div>
        </div>
    </div>
</template>

<style scoped>
.action-chain {
    display: flex;
    flex-direction: column;
    min-height: 0;
    /* 兜底：链很长（或结果很长）时整块能滚 —— 否则窄屏上「添加动作」和结果区
       会被父级的 overflow: hidden 直接裁掉，而且界面上看不出发生了什么。 */
    overflow-y: auto;
}

.action-chain__head {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 6px;
}

.action-chain__title {
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    opacity: 0.75;
}

.action-chain__steps {
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.action-chain__hint {
    font-size: 0.75rem;
    opacity: 0.7;
    padding: 8px 0;
}

.action-chain__step {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    border: 1px solid rgba(148, 163, 184, 0.42);
    border-radius: 8px;
    /* 主题变量，别写死浅色 —— 暗色下会是一块白 */
    background: rgba(var(--v-theme-surface-variant), 0.35);
    font-size: 0.75rem;
}

.action-chain__step-n {
    width: 16px;
    height: 16px;
    flex: 0 0 auto;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 5px;
    background: linear-gradient(135deg, #0ea5e9, #14b8a6);
    color: #fff;
    font-size: 0.625rem;
    font-weight: 700;
}

.action-chain__step-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.action-chain__step-delta {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.625rem;
    opacity: 0.7;
    flex: 0 0 auto;
}

.action-chain__step-actions {
    display: inline-flex;
    gap: 1px;
    flex: 0 0 auto;
}

.action-chain__mini {
    width: 20px;
    height: 20px;
    border: none;
    border-radius: 4px;
    background: transparent;
    /* inherit + opacity，不写死深灰 —— 暗色主题下写死的颜色直接看不见 */
    color: inherit;
    opacity: 0.7;
    cursor: pointer;
    font-size: 0.75rem;
    line-height: 1;
    padding: 0;
}

.action-chain__mini:hover:not(:disabled) {
    background: rgba(var(--v-theme-primary), 0.14);
    color: rgb(var(--v-theme-primary));
}

.action-chain__mini:disabled {
    opacity: 0.3;
    cursor: default;
}

.action-chain__mini--danger:hover {
    background: rgba(var(--v-theme-error), 0.14);
    color: rgb(var(--v-theme-error));
}

.action-chain__add {
    margin-top: 8px;
}

/* 动作面板现在住在 v-menu 的弹层里（见模板里的说明）。
   固定尺寸是必须的：ActionPicker 内部是「搜索框 + 可滚动的分组区」，
   需要一个**有界**的高度才滚得起来。
   ⚠️ 也要有实底 —— 弹层浮在内容之上，透明背景会和底下的文字糊在一起。 */
.action-chain__picker {
    display: flex;
    flex-direction: column;
    width: 340px;
    max-width: calc(100vw - 32px);
    max-height: 320px;
    padding: 10px;
    background: rgb(var(--v-theme-surface));
    border-radius: 4px;
}

.action-chain__templates {
    display: flex;
    gap: 4px;
    margin-top: 6px;
}

.action-chain__naming {
    display: flex;
    gap: 6px;
    align-items: center;
    margin-top: 6px;
}

/* ⚠️ 结果区**不自己滚**（没有 max-height / overflow）—— 滚动交给外层 `.action-chain`。
   两层滚动容器在触屏上很糟：手指在里面滑只滚内层，滚到底外层不动，看着像卡住了。
   背景用主题变量而不是写死的浅色 —— 暗色下 `#f8fafc` 会变成一块刺眼的白。 */
.action-chain__result {
    min-height: 140px;
    border: 1px solid rgba(148, 163, 184, 0.42);
    border-radius: 10px;
    padding: 10px;
    background: rgba(var(--v-theme-surface-variant), 0.35);
}

.action-chain__pre {
    margin: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.75rem;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-all;
}

.action-chain__html {
    font-size: 0.8125rem;
    line-height: 1.7;
    word-break: break-word;
}

.action-chain__error {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 0.75rem;
    /* 主题变量：写死的深红在暗色主题下对比度太低 */
    color: rgb(var(--v-theme-error));
}

.action-chain__empty {
    font-size: 0.75rem;
    opacity: 0.7;
}
</style>
