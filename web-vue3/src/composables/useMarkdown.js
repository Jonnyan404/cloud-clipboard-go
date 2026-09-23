import { computed, reactive, ref, watch } from 'vue';
import { useAppStore } from '@/store/app';
import {
    formatJson,
    looksLikeCode,
    looksLikeMarkdown,
    looksLikeTable,
    looksLikeTaskList,
    minifyJson,
    renderMarkdownHtml,
} from '@/util.js';
import { detectLanguage } from '@/highlight.js';

/**
 * 一条内容的显示方式。除了「原文 / Markdown」，还有三个**结构化视图**：
 *   · `json`     —— JSON 美化（两空格缩进）
 *   · `json-min` —— JSON 压缩（一行）
 *   · `code`     —— 代码视图：整段当源码，转义 + 按语言上色
 *
 * ⚠️ **代码视图为什么必须存在**：markdown 会把 `*` `_` `#` `-` `[]` 当标记，
 * 一段 Go / Shell 丢进去点「md 渲染」会被重排得面目全非（Jonny 实测：整段缩进和注释全乱）。
 * 代码只能走「转义 + 高亮」这条路，**不能过 markdown**。
 *
 * 两层职责分得很清楚：
 *   · 个性化里的开关（app.display.markdown）—— 只决定**要不要显示那些切换图标**
 *   · 右上角那几个图标 —— 决定**这一条用哪种方式看**
 *
 * 开关默认**开**（见 displayToggles 的 DEFAULT_DISPLAY.markdown）。
 *
 * ⚠️ 这一条**默认看原文**。「默认渲染 md」只针对**聊天气泡** —— 那里不做每条一个切换图标，
 * 直接按内容判断渲染，见 ChatWall 的 bubbleHtml。别把这个默认值改成 md。
 *
 * **例外：任务列表和表格默认就是 md。** 这两种内容的全部价值在结构上 —— 默认看原文等于
 * 「先让你读一遍 `- [ ]`、再点一下才看到清单」。只有这两种结构破例，普通文本照旧看原文。
 * （个性化那个开关仍然是上位开关：关掉它，这里连切换图标都不显示。）
 *
 * @param {() => string} getText        取原始文本
 * @param {() => boolean} isMarkdownFile 可选：扩展名是 .md 这类可靠信号。
 *        文件场景扩展名比内容启发式可信（一份只有一句话的 README 靠启发式判不出来）。
 */
export function useMarkdown(getText, isMarkdownFile = () => false) {
    const app = useAppStore();
    const structured = () => looksLikeTaskList(getText()) || looksLikeTable(getText());
    const markdownish = () => isMarkdownFile() || looksLikeMarkdown(getText());

    // 默认值要**跟着内容走**，不能只在 setup 时定一次：
    // 文件预览的正文是异步抓回来的，setup 时 getText() 还是空串 —— 定成 raw 之后就再也不会变。
    // 用「用户覆盖值 ?? 按内容算出来的默认值」表达：用户没点过右上角那个切换时，
    // 正文一到就重新定默认；点过之后一律听用户的。
    const override = ref(null);
    const mode = computed({
        get: () => override.value ?? (structured() ? 'md' : 'raw'),
        set: (next) => { override.value = next; },
    });

    // 三种结构化视图各算一次，供多处复用 —— 别在 available / html / 模板里各算一遍。
    const prettyJson = computed(() => formatJson(getText()));
    const compactJson = computed(() => minifyJson(getText()));
    const jsonAvailable = computed(() => app.display.markdown && Boolean(prettyJson.value));
    const jsonCompactAvailable = computed(() => app.display.markdown && Boolean(compactJson.value));
    const codeAvailable = computed(() => app.display.markdown && looksLikeCode(getText()));

    // 内容既不像 markdown 也不是 JSON / 代码时，显示这些图标没有意义，所以即使开关开着也不显示。
    //
    // ⚠️ 两个 JSON 视图**都要算进来**：已经美化过的 JSON，`prettyJson` 是空串（没得美化），
    // 但 `compactJson` 有值（压得动）—— 只判 prettyJson 的话那种内容会一个图标都不显示，
    // 于是「压缩」这个唯一有用的入口根本够不着（QA 实测踩到）。
    const available = computed(
        () => app.display.markdown
            && (markdownish() || Boolean(prettyJson.value) || Boolean(compactJson.value) || codeAvailable.value),
    );

    // 代码视图的语言：**自动识别**（`highlightAuto` 限定在我们注册过的那几十种里）。
    // 认不出来就按「无语言」渲染 —— 那也**仍然是转义后的纯代码**，格式一个字都不会丢，
    // 只是没颜色。这比走 markdown 好得多（markdown 会重排）。
    const codeLanguage = ref('');
    watch(
        () => (mode.value === 'code' ? getText() : ''),
        async (text) => {
            codeLanguage.value = text ? await detectLanguage(text) : '';
        },
        { immediate: true },
    );

    // 把整段文本当成**一个围栏代码块**渲染。
    //
    // 为什么绕 markdown 这一圈、而不是直接 v-html 一个 `<pre>`：这样消费方**一行模板都不用改**
    // （它们本来就 `v-if="md.html"` 渲染 markdown-body），而且顺带吃到 MarkdownBody 里的代码高亮。
    //
    // ⚠️ 围栏长度必须**比正文里最长的连续反引号还长**，否则正文自带的 ``` 会把围栏提前闭合。
    function renderFenced(code, language) {
        const runs = String(code).match(/`+/g) || [];
        const fence = '`'.repeat(Math.max(3, ...runs.map((run) => run.length + 1)));
        return renderMarkdownHtml(`${fence}${language || ''}\n${code}\n${fence}`);
    }

    const html = computed(() => {
        if (!available.value) {
            return '';
        }
        switch (mode.value) {
            case 'json':
                return prettyJson.value ? renderFenced(prettyJson.value, 'json') : '';
            case 'json-min':
                return compactJson.value ? renderFenced(compactJson.value, 'json') : '';
            case 'code':
                return renderFenced(getText(), codeLanguage.value);
            case 'md':
                // 任务列表的复选框要能点。**文件预览那条路不接**：那边的正文是截断过的
                // （displayedTextPreview 会 slice 掉尾巴），按下标回写会把截断后的内容当成全文。
                return renderMarkdownHtml(getText(), {
                    interactiveTasks: looksLikeTaskList(getText()) && !isMarkdownFile(),
                });
            default:
                return '';
        }
    });

    // 渲染结果是不是以 `<pre>` 开头（代码视图 / JSON 美化 / 压缩都是）。
    //
    // 消费方据此**关掉浮动占位**：`<pre>` 带 `overflow-x: auto`，是个 BFC ——
    // 它不会绕着浮动块排版，而是被挤到浮动块**旁边**的窄列里，
    // 表现就是「正文和图标各占一列」（Jonny 实测踩到）。那种情况改成给 pre 自己留右边距。
    const leadsWithBlock = computed(() => /^\s*<pre[\s>]/.test(html.value || ''));

    // 「复制」该复制**你正在看的那一份**：原文 / 美化后的 JSON / 压缩后的 JSON / 代码。
    //
    // ⚠️ 别永远复制原文 —— 用户切到「压缩」视图，就是为了把那一行拿走。
    // md / code / raw 三种视图复制的都是原文（md 视图下拿走的是源文，不是渲染后的 HTML）。
    const copyText = computed(() => {
        if (mode.value === 'json') {
            return prettyJson.value || getText();
        }
        if (mode.value === 'json-min') {
            return compactJson.value || getText();
        }
        return getText();
    });

    // 图标有几个 → 正文要给图标让出多宽。宽度契约在 MarkdownToggle 的 `--md-toggle-gutter*`，
    // 消费方把 `gutter` 绑到那个变量上，所以图标增减只要改这里的选择。
    const iconCount = computed(() => {
        if (!available.value) {
            return 0;
        }
        let count = 1;                              // 原文
        if (jsonAvailable.value) {
            count += 1;                             // 美化
        }
        if (jsonCompactAvailable.value) {
            count += 1;                             // 压缩
        }
        // md / 代码 那个槽位只在不是 JSON 时占位（JSON 渲染成 markdown 和原文一模一样）
        if (!jsonAvailable.value && !jsonCompactAvailable.value && (markdownish() || codeAvailable.value)) {
            count += 1;
        }
        return count;
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
        codeAvailable,
        copyText,
        gutter,
        html,
        leadsWithBlock,
        jsonAvailable,
        jsonCompactAvailable,
        mode,
        setMode,
    });
}
