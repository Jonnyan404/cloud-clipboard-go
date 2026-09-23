<script setup>
// 动作选择面板：搜动作、按组列出来、点一下**追加到链尾**。
//
// ⚠️ 是「追加」不是「替换」—— 这是它和普通下拉菜单的根本区别。点第二个动作时
// 第一个不会消失，两者串成一条流水线。只点一个时自然退化成「换一种方式看这条」，
// 所以「单动作」和「多动作组合」共用这一个界面，不做两套。
//
// ⚠️ 不适用的动作**置灰但不隐藏**。隐藏会让人以为功能不存在（「为什么这里没有 JSON 美化？」）；
// 置灰 + 一句说明，用户能自己得出「哦，这条不是 JSON」。
// 这条约定和 util.js 里 formatJson 返回空串就不显示入口是同一个思路的放宽版。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { ACTION_GROUPS, ACTIONS } from '@/data/actions.js';

const props = defineProps({
    // 当前正文，用来算「哪些动作适用」。空串 = 不标注适用性。
    text: { type: String, default: '' },
    // 'view'（看）| 'insert'（发）
    direction: { type: String, default: 'view' },
});

const emit = defineEmits(['pick']);

const { t } = useI18n();
const query = ref('');

function isApplicable(action) {
    if (!action.match) {
        return true; // 通用动作，任何文本都能跑
    }
    if (!props.text) {
        return true; // 没有内容可判断时，一律当作可用（别让人以为一半动作消失了）
    }
    return action.match(props.text);
}

// 搜索匹配 i18n 之后的**显示名**（用户看到什么就搜什么）+ 动作 id
// （搜 `b64` / `json` 这类短 id 片段比搜中文名快）。
function matchesQuery(action) {
    const q = query.value.trim().toLowerCase();
    if (!q) {
        return true;
    }
    return t(action.nameKey).toLowerCase().includes(q) || action.id.toLowerCase().includes(q);
}

const groups = computed(() => {
    const pool = ACTIONS.filter((a) => a.direction === props.direction && matchesQuery(a));
    return ACTION_GROUPS
        .map((group) => ({
            ...group,
            // 适用的排前面 —— 用户十有八九要的就是那几个
            actions: pool
                .filter((a) => a.group === group.key)
                .sort((a, b) => Number(isApplicable(b)) - Number(isApplicable(a))),
        }))
        .filter((group) => group.actions.length);
});
</script>

<template>
    <div class="action-picker">
        <v-text-field
            v-model="query"
            density="compact"
            variant="outlined"
            hide-details
            single-line
            clearable
            prepend-inner-icon="mdi-magnify"
            :placeholder="t('actionSearchPlaceholder')"
            class="action-picker__search"
        />

        <div class="action-picker__body">
            <template v-for="group in groups" :key="group.key">
                <div class="action-picker__group">{{ t(group.labelKey) }}</div>
                <div class="action-picker__grid">
                    <button
                        v-for="action in group.actions"
                        :key="action.id"
                        type="button"
                        class="action-picker__item"
                        :class="{ 'action-picker__item--dim': !isApplicable(action) }"
                        :title="isApplicable(action) ? t(action.nameKey) : t('actionNotApplicable')"
                        @click="emit('pick', action.id)"
                    >
                        <v-icon size="16">{{ action.icon }}</v-icon>
                        <span class="action-picker__label">{{ t(action.nameKey) }}</span>
                    </button>
                </div>
            </template>

            <div v-if="!groups.length" class="action-picker__empty">{{ t('actionSearchEmpty') }}</div>
        </div>
    </div>
</template>

<style scoped>
.action-picker {
    display: flex;
    flex-direction: column;
    min-height: 0;
}

.action-picker__search {
    margin-bottom: 8px;
}

.action-picker__body {
    overflow-y: auto;
    min-height: 0;
    padding-right: 2px;
}

.action-picker__group {
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    opacity: 0.75;
    margin: 10px 0 6px;
}

.action-picker__group:first-child {
    margin-top: 0;
}

.action-picker__grid {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
}

/* 原生 button 而不是 v-btn：这里一次可能铺几十个，v-btn 的密度/变体开销不必要，
   而且我们只需要「小胶囊」这一种形态。 */
.action-picker__item {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    /* 移动端点击区域：高度不小于 30px */
    padding: 5px 10px;
    border: 1px solid rgba(148, 163, 184, 0.5);
    border-radius: 999px;
    /* 主题变量而不是写死的白底深字 —— 这个面板现在住在弹层里（surface 底色），
       暗色主题下写死的话就是一堆白胶囊浮在暗底上。 */
    background: rgb(var(--v-theme-surface));
    color: inherit;
    font-size: 0.75rem;
    line-height: 1.4;
    cursor: pointer;
    transition: border-color 0.15s ease, color 0.15s ease;
    max-width: 100%;
}

.action-picker__item:hover {
    border-color: rgb(var(--v-theme-primary));
    color: rgb(var(--v-theme-primary));
}

/* 不适用的置灰（仍可点 —— 点了会给出明确错误，不是「没反应」） */
.action-picker__item--dim {
    opacity: 0.42;
}

.action-picker__label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.action-picker__empty {
    font-size: 0.75rem;
    opacity: 0.7;
    padding: 14px 2px;
    text-align: center;
}
</style>
