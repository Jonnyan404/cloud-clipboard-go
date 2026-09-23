import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAppStore } from '@/store/app';
import { findAction, renderFenced, runChain, targetedActions } from '@/data/actions.js';
import { looksLikeTable, looksLikeTaskList } from '@/util.js';

/**
 * 一条内容的显示方式 —— **动作库驱动**。
 *
 * 以前这里写死五个分支（raw / md / code / json / json-min），想加一个「Base64 解码」
 * 要同时改三处：这个 switch、MarkdownToggle 的固定槽位、gutter 的宽度计算。
 * 现在「有哪些视图」完全由 `data/actions.js` 的注册表决定，这里只负责三件事：
 *   · 算这条内容能用哪些动作（`match` 命中的）
 *   · 维护「当前看哪个」（按内容算出的默认值 + 用户覆盖）
 *   · 把当前动作跑出来的结果变成消费方认的 `html`
 *
 * 两层职责仍然分得很清楚（别合并）：
 *   · 个性化里的开关（`app.display.markdown`）—— 只决定**要不要显示那些图标**
 *   · 右上角那几个图标 —— 决定**这一条用哪种方式看**
 *
 * ⚠️ 「原文」不是动作（它不跑任何东西），所以用 `null` 表示。
 *
 * ⚠️ 任务列表和表格**默认就是 md**：这两种内容的全部价值在结构上，默认看原文等于
 * 「先让你读一遍 `- [ ]`、再点一下才看到清单」。其余内容默认看原文。
 * （个性化那个开关仍是上位开关：关掉它，这里连切换图标都不显示。）
 *
 * @param {() => string} getText         取原始文本
 * @param {() => boolean} isMarkdownFile 可选：扩展名是 .md 这类可靠信号。
 *        文件场景扩展名比内容启发式可信（一份只有一句话的 README 靠启发式判不出来）。
 */
export function useMarkdown(getText, isMarkdownFile = () => false) {
    const app = useAppStore();
    const { t } = useI18n();

    // 这条内容能用哪些**有针对性**的动作（声明了 match 且命中），按注册顺序。
    //
    // ⚠️ isMarkdownFile 的用途：文件预览的正文是**截断过**的，启发式可能判不出来，
    // 而扩展名是可靠信号 —— 判准了就把「Markdown 渲染」补到最前面。
    const targeted = computed(() => {
        const list = targetedActions(getText());
        if (isMarkdownFile() && !list.some((action) => action.id === 'format.markdown')) {
            const markdownAction = findAction('format.markdown');
            if (markdownAction) {
                return [markdownAction, ...list];
            }
        }
        return list;
    });

    // 用户显式选过的动作 id。
    // `undefined` = 从没选过（跟随默认值），`null` = 明确要看原文，字符串 = 那个动作。
    const override = ref(undefined);

    // 默认值要**跟着内容走**，不能只在 setup 时定一次：
    // 文件预览的正文是异步抓回来的，setup 时 getText() 还是空串 ——
    // 那时定成 null（原文）之后就再也不会变。
    // 用「用户覆盖值 ?? 按内容算出来的默认值」表达这个先后关系。
    const defaultId = computed(() => {
        const text = getText();
        const structured = looksLikeTaskList(text) || looksLikeTable(text);
        return structured ? 'format.markdown' : null;
    });

    const mode = computed({
        get: () => (override.value === undefined ? defaultId.value : override.value),
        set: (next) => {
            override.value = next ?? null;
        },
    });

    // 图标排显示的条件：开关开着 + 至少有一个**针对性**动作。
    //
    // ⚠️ 通用动作（转大写、Base64 编码…）**不单独触发显示** —— 否则每条普通文本上都会
    // 挂一排图标，那正是现有设计要避免的噪音。它们在 `⋯` 面板里，而 `⋯` 只在图标排
    // 已经显示时才存在。
    const available = computed(() => app.display.markdown && targeted.value.length > 0);

    // 当前视图的渲染结果。**是异步的**：format.code 要 await 语言检测。
    const html = ref('');
    const viewText = ref('');
    let renderSeq = 0;

    async function renderCurrent() {
        const id = mode.value;
        if (!available.value || !id) {
            return { html: '', text: '' };
        }
        const result = await runChain(getText(), [id], { t });
        if (result.error) {
            // 跑不出来就当没有这个视图、退回原文 —— 卡片预览区不该弹错误（报错是工作台的活）
            return { html: '', text: '' };
        }
        const action = findAction(id);
        const last = result.steps[result.steps.length - 1];
        // 1. 动作给了 `html`（双表示，如注音制表）→ 用它渲染，`text` 留给复制
        if (last?.html) {
            return { html: last.html, text: result.output };
        }
        // 2. 动作声明了 render: 'html'（md 渲染 / 代码高亮）→ output 本身就是 HTML
        if (action?.render === 'html') {
            return { html: result.output, text: '' };
        }
        // 3. 纯文本结果（编解码 / 文本处理）统一包成 `<pre>` 形态的 HTML ——
        // 消费方本来就只认 `md.html` 一个分支，多一个分支等于要改 5 个消费方。
        return { html: renderFenced(result.output), text: result.output };
    }

    watch(
        [() => getText(), mode, available],
        async () => {
            const seq = ++renderSeq;
            const next = await renderCurrent();
            // 防竞态：快速切视图时，先发起的计算可能后回来
            if (seq === renderSeq) {
                html.value = next.html;
                viewText.value = next.text;
            }
        },
        { immediate: true },
    );

    // 渲染结果是不是以 `<pre>` 开头（代码视图 / JSON / 编解码结果都是）。
    //
    // 消费方据此**关掉浮动占位**：`<pre>` 带 overflow-x: auto，是个 BFC ——
    // 它不会绕着浮动块排版，而是被挤到浮动块**旁边**的窄列里，
    // 表现就是「正文和图标各占一列」。
    const leadsWithBlock = computed(() => /^\s*<pre[\s>]/.test(html.value || ''));

    // 「复制」该复制**你正在看的那一份**：原文 / 当前动作的结果。
    // ⚠️ 别永远复制原文 —— 用户切到「压缩」视图，就是为了把那一行拿走。
    const copyText = computed(() => viewText.value || getText());

    // 图标有几个 → 正文要给图标让出多宽。
    //
    // 布局是「原文 + 最多 2 个针对性动作 + 一个 ⋯」，**总数封在 3 个** ——
    // 这样 MarkdownToggle 的 --md-toggle-gutter 两档（64 / 96px）够用，不必再加档位。
    const iconCount = computed(() => {
        if (!available.value) {
            return 0;
        }
        return 1 + Math.min(targeted.value.length, 2) + (targeted.value.length > 2 ? 1 : 0);
    });
    const gutter = computed(() => (iconCount.value > 2 ? '96px' : '64px'));

    function setMode(next) {
        mode.value = next;
    }

    // 必须用 reactive 包一层再返回。
    //
    // 直接 `return { available, html, mode, setMode }` 的话，模板里写 `md.html` 拿到的是
    // **Ref 对象**而不是值 —— 它恒为 truthy，于是 `v-if="md.html"` 永远成立，`v-html` 又把它
    // 转成字符串 "[object Object]"，结果是「渲染区在，但一片空白」。
    // （模板只对**顶层**的 ref 自动解包，嵌在对象里的不会。踩过一次。）
    return reactive({
        available,
        actions: targeted,
        copyText,
        gutter,
        html,
        leadsWithBlock,
        mode,
        setMode,
    });
}
