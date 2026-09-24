import { computed, ref, watch } from 'vue';
import { findAction, makeStep, stepId, stepParams } from '@/data/actions.js';

/**
 * 动作链：一串动作 id，按顺序作用在正文上。
 *
 * 为什么是「链」而不是「选一个动作」：方案 B 的核心就是**可叠多步** ——
 * 「JSON 压缩 → Base64 编码 → URL 编码」是一条链，不是三个互斥的选择。
 * 只选一个动作时它自然退化成「换一种方式看这条」，所以一个 UI 覆盖两种需求。
 *
 * ⚠️ 状态是**模块级单例**。链属于「工作台」这一个会话状态，动作面板、结果区、
 * 模式顶部读的必须是同一份 —— 每个组件各持一份 ref 会立刻不同步。
 *
 * ⚠️ 执行本身不在这里。见 data/actions.js 的 runChain（纯函数，不带响应式，好测）。
 * 这里只管「链是什么」和「模板怎么存」。
 */

const CHAIN_KEY = 'ccgActionChain';
const TEMPLATES_KEY = 'ccgActionTemplates';

function readJson(key, fallback) {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) {
            return fallback;
        }
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? parsed : fallback;
    } catch {
        // 隐私模式 / 坏数据：当作没有。链丢了不是错误，不该让页面起不来。
        return fallback;
    }
}

function writeJson(key, value) {
    try {
        localStorage.setItem(key, JSON.stringify(value));
    } catch {
        /* 存不下就算了，内存里仍然生效 */
    }
}

// 只保留还存在的动作 id —— 动作被删掉（或改名）之后，老 localStorage 里会留着孤儿 id，
// 不清掉的话链上会挂着一个永远跑不了、还删不掉的步骤。
//
// ⚠️ 链元素有**两种形态**（字符串 / `{id, params}`），所以判存在性必须走 `stepId`，
// 不能拿元素当字符串用（带参数的步骤会变成 `[object Object]`，于是被整条清掉）。
function sanitize(chain) {
    return (Array.isArray(chain) ? chain : []).filter((step) => Boolean(findAction(stepId(step))));
}

const chain = ref(sanitize(readJson(CHAIN_KEY, [])));
const templates = ref(
    readJson(TEMPLATES_KEY, [])
        .filter((tpl) => tpl && typeof tpl.id === 'string')
        .map((tpl) => ({ ...tpl, steps: sanitize(tpl.steps) }))
        .filter((tpl) => tpl.steps.length),
);

watch(chain, (value) => writeJson(CHAIN_KEY, value), { deep: true });
watch(templates, (value) => writeJson(TEMPLATES_KEY, value), { deep: true });

function newTemplateId() {
    if (globalThis.crypto && typeof crypto.randomUUID === 'function') {
        return `tpl-${crypto.randomUUID()}`;
    }
    return `tpl-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function useActionChain() {
    // ── 链本身 ────────────────────────────────────────────────────
    // 每一步：id + **这一步自己的参数** + 动作定义。
    // ⚠️ params 从**链元素**里读，不从 action 上读 —— 同一个动作可以在链上出现两次、
    // 两次用不同参数，所以参数属于「这一步」而不是「这个动作」。
    const steps = computed(() => chain.value
        .map((step) => ({ id: stepId(step), params: stepParams(step), action: findAction(stepId(step)) }))
        .filter((s) => s.action));

    /**
     * 追加一个动作到链尾。
     *
     * ⚠️ **允许重复**：链是流水线，「Base64 编码」跑两次是合法意图（双重编码）。
     * 不做「已存在就忽略」的去重 —— 那会让用户点两次却只有一步，且没有任何反馈。
     */
    function add(id) {
        if (findAction(id)) {
            chain.value = [...chain.value, id];
        }
    }

    function removeAt(index) {
        chain.value = chain.value.filter((_, i) => i !== index);
    }

    function clear() {
        chain.value = [];
    }

    /** 上移/下移一步。`delta` 为 -1 / +1，越界不动。 */
    function move(index, delta) {
        const target = index + delta;
        if (index < 0 || index >= chain.value.length || target < 0 || target >= chain.value.length) {
            return;
        }
        const next = [...chain.value];
        [next[index], next[target]] = [next[target], next[index]];
        chain.value = next;
    }

    /**
     * 改链上某一步的参数。
     *
     * ⚠️ 用 **index** 而不是 id 定位：链**允许重复**（同一步骤出现两次是合法意图），
     * 按 id 找会改错那一个。
     *
     * ⚠️ 值清空后会自动退回**字符串形态**（见 makeStep）—— 存储里不会留下
     * `{id, params: {find: ''}}` 这种没意义的空壳。
     */
    function setParam(index, key, value) {
        const step = chain.value[index];
        if (!step) {
            return;
        }
        const params = { ...stepParams(step), [key]: value };
        const next = [...chain.value];
        next[index] = makeStep(stepId(step), params);
        chain.value = next;
    }

    function setChain(steps) {
        chain.value = sanitize(steps);
    }

    // ── 模板 ──────────────────────────────────────────────────────
    /** 把当前链存成一个命名模板。链为空时不存（存一个空模板没有意义）。 */
    function saveTemplate(name) {
        const label = String(name || '').trim();
        if (!label || !chain.value.length) {
            return null;
        }
        const tpl = { id: newTemplateId(), name: label, steps: [...chain.value] };
        templates.value = [...templates.value, tpl];
        return tpl;
    }

    function applyTemplate(id) {
        const tpl = templates.value.find((t) => t.id === id);
        if (tpl) {
            setChain(tpl.steps);
        }
    }

    function removeTemplate(id) {
        templates.value = templates.value.filter((t) => t.id !== id);
    }

    return {
        chain,
        steps,
        templates,
        add,
        removeAt,
        clear,
        move,
        setParam,
        setChain,
        saveTemplate,
        applyTemplate,
        removeTemplate,
    };
}
