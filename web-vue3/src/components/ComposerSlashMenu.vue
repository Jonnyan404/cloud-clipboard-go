<script setup>
// 「/」模板菜单：输入框行首打 `/` 弹出来的那一行小胶囊。
//
// 为什么抽成组件：它有**两个**落点 —— 行内输入框上方，以及**全屏输入窗**里。
// 全屏正是「写长文」的场景，模板恰恰在那时最有用；只做行内那个等于一半时间没有这个功能
// （实测就是这么漏的）。
//
// 组件本身不管开关和插入，只负责渲染 + 抛 `pick`：状态留在 UnifiedComposer，
// 因为「往哪个 textarea 的哪个位置插」只有那边知道。
import { useI18n } from 'vue-i18n';

defineProps({
    // [{ key, icon, text }]，其中 key 是 i18n 文案键
    items: { type: Array, required: true },
});
const emit = defineEmits(['pick']);

const { t } = useI18n();

// 触摸屏上点胶囊：`touchstart` 之后浏览器还会补一发合成 `mousedown`，不去重的话
// 模板会被插进去两次。去重窗口取 400ms —— 合成事件紧跟着来，而人不可能在 400ms
// 内点上两个不同的模板。
// `prevent` 也不能少：不拦的话点胶囊会先让输入框失焦（手机上键盘收起、光标丢失），
// 插入的位置就错了。
let lastPickAt = 0;
function pick(tpl) {
    const now = Date.now();
    if (now - lastPickAt < 400) {
        return;
    }
    lastPickAt = now;
    emit('pick', tpl);
}
</script>

<template>
    <div class="composer-slash">
        <button
            v-for="tpl in items"
            :key="tpl.key"
            type="button"
            class="composer-slash__item"
            @mousedown.prevent="pick(tpl)"
            @touchstart.prevent="pick(tpl)"
        >
            <v-icon size="18" class="me-2">{{ tpl.icon }}</v-icon>{{ t(tpl.key) }}
        </button>
    </div>
</template>

<style scoped>
.composer-slash {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    padding: 2px 4px 6px;
}

.composer-slash__item {
    display: inline-flex;
    align-items: center;
    border: 1px solid rgba(148, 163, 184, 0.45);
    background: transparent;
    color: inherit;
    border-radius: 999px;
    padding: 3px 10px;
    font-size: 12px;
    line-height: 1.4;
    cursor: pointer;
}

.composer-slash__item:hover {
    border-color: rgb(var(--v-theme-primary));
    color: rgb(var(--v-theme-primary));
}
</style>
