import { computed, reactive, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { looksLikeMarkdown, looksLikeTable, looksLikeTaskList, renderMarkdownHtml } from '@/util.js';

/**
 * 一条内容的「原文 / Markdown」显示方式。
 *
 * 两层职责分得很清楚：
 *   · 个性化里的开关（app.display.markdown）—— 只决定**要不要显示那两个切换图标**
 *   · 右上角那两个图标 —— 决定**这一条用哪种方式看**，raw 还是 md
 *
 * 开关默认**开**（见 displayToggles 的 DEFAULT_DISPLAY.markdown）。
 *
 * ⚠️ 这一条（标准模式卡片 / 便签阅读器 / 文件预览）**默认看原文**。
 * 「默认渲染 md」只针对**聊天气泡** —— 那里不做每条一个切换图标，
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

    // 默认值要**跟着内容走**，不能只在 setup 时定一次：
    // 文件预览的正文是异步抓回来的，setup 时 getText() 还是空串 —— 定成 raw 之后就再也不会变。
    // 用「用户覆盖值 ?? 按内容算出来的默认值」表达：用户没点过右上角那个切换时，
    // 正文一到就重新定默认；点过之后一律听用户的。
    const override = ref(null);
    const mode = computed({
        get: () => override.value ?? (structured() ? 'md' : 'raw'),
        set: (next) => { override.value = next; },
    });

    // 内容不像 markdown 时显示这两个图标没有意义，所以即使开关开着也不显示。
    const available = computed(
        () => app.display.markdown && (isMarkdownFile() || looksLikeMarkdown(getText())),
    );
    const html = computed(() => (available.value && mode.value === 'md'
        // 任务列表的复选框要能点。**文件预览那条路不接**：那边的正文是截断过的
        // （displayedTextPreview 会 slice 掉尾巴），按下标回写会把截断后的内容当成全文。
        ? renderMarkdownHtml(getText(), {
            interactiveTasks: looksLikeTaskList(getText()) && !isMarkdownFile(),
        })
        : ''));

    function setMode(next) {
        mode.value = next;
    }

    // 必须用 reactive 包一层再返回。
    //
    // 直接 `return { available, html, mode, setMode }` 的话，模板里写 `md.html` 拿到的是
    // **Ref 对象**而不是值 —— 它恒为 truthy，于是 `v-if="md.html"` 永远成立，`v-html` 又把它
    // 转成字符串 "[object Object]"，结果是「渲染区在，但一片空白」。
    // （模板只对**顶层**的 ref 自动解包，嵌在对象里的不会。踩过一次。）
    return reactive({ available, html, mode, setMode });
}
