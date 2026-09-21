import { onBeforeUnmount, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { decodeHtmlEntities, toggleTaskListItem, updateTextEntry } from '@/util.js';

/**
 * 一条文本条目的「任务列表可打勾」状态。
 *
 * 三件事：
 *   1. 本地正文（`text`）—— 打勾改的是它，不是 `props.meta.content`；
 *   2. 点击处理（`onMdClick`）—— 挂到渲染 markdown 的容器上；
 *   3. 落盘（防抖）—— 走 `POST /text?id=<id>`，见 util.js 的 updateTextEntry。
 *
 * 界面立刻变（乐观更新），请求防抖 700ms：连点几个框只发一次。
 * **失败就退回上次保存成功的那一版并提示** —— 显示着「已勾」却没存上是更糟的谎。
 *
 * ⚠️ 复制文本也要用这里返回的 `text`（用户看到什么就复制什么）。
 *
 * @param {object} meta    条目（用 id 定位、content 当初始值）
 * @param {() => string} getRoom 房间要现取 —— 传值会在切换房间后发错房间
 */
export function useTaskListToggle(meta, getRoom) {
    const { t } = useI18n();
    const text = ref(decodeHtmlEntities(meta?.content || ''));
    let saved = text.value;
    let timer = null;

    async function persist() {
        const next = text.value;
        if (next === saved) return;
        try {
            await updateTextEntry(meta?.id, getRoom(), next);
            saved = next;
        } catch (error) {
            console.error('保存任务列表失败:', error);
            toast(t('taskSaveFailed'));
            text.value = saved;
        }
    }

    function flip(index) {
        text.value = toggleTaskListItem(text.value, index);
        clearTimeout(timer);
        timer = setTimeout(persist, 700);
    }

    // 只接复选框的点击，别的点击原样放过（容器上还有别的行为）。
    function onMdClick(e) {
        const box = e.target;
        if (!box || box.tagName !== 'INPUT' || box.type !== 'checkbox') return;
        const boxes = [...(e.currentTarget?.querySelectorAll('input[type=checkbox]') || [])];
        const index = boxes.indexOf(box);
        if (index < 0) return;
        // 拦掉浏览器自己翻转：状态以源码为准，翻转后由 v-html 重新渲染出来
        e.preventDefault();
        // ⚠️ 还要拦住冒泡：便签卡片整块是「点开阅读器」，不拦的话一勾就把大视图打开了
        e.stopPropagation();
        flip(index);
    }

    onBeforeUnmount(() => clearTimeout(timer));

    return { text, onMdClick };
}
