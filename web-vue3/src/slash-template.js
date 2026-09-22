/**
 * 「/」模板菜单的**共用判定**：输入框**行首**打 `/` 弹出 markdown 模板。
 *
 * 为什么抽成模块：标准模式（UnifiedComposer）和其余模式（StickyComposer）各有一份
 * 一模一样的逻辑，而之前的移动端故障正好是这类「一处有、另一处没有」的典型 —— 手机上
 * 两个输入框一起坏，修的时候也必须一起修。判定只留一份，就不会再漂。
 *
 * 判定**只看文本**（`/` 落在行首、它前面只有空白），不看按键事件 —— 原因见 slashMenuShouldOpen。
 */

// 模板正文用中性的占位符，不放进 i18n —— 它们是要被用户改写的骨架，不是文案。
// 菜单项文案复用分类条那两个键（`filterTaskList` / `filterTable`）：说的是同一个东西，
// 没必要为「筛选」和「插入」各存一份同义文案（两份迟早会漂）。
export const SLASH_TEMPLATES = [
    { key: 'filterTaskList', icon: 'mdi-checkbox-marked-outline', text: '- [ ] \n- [ ] \n- [ ] ' },
    { key: 'filterTable', icon: 'mdi-table', text: '| A | B |\n| --- | --- |\n|  |  |' },
];

// 全角 `/` 也算：中文输入法切到全角标点时打出来的是 `／`。用户做的是同一个动作
// （行首打一个斜杠），只因为输入法状态不同就不弹菜单，解释不通。
const SLASH_CHARS = '/／';

/**
 * 判定要用的文本：**优先读元素自己的 `value`**，而不是组件里的 model。
 *
 * ⚠️ 这个顺序不是风格问题：`v-model` 在 `<textarea>` 上是**指令**（`vModelText`），
 * 它的 input 回调比组件模板上的 `@input` 晚绑定、也就晚执行 —— 在 `@input` 里读
 * `app.send.text` 拿到的是**打字之前**的值，判定会永远慢一拍（表现就是菜单不弹）。
 * 而元素的 `value` 在 `input` 事件派发前就已经是新值，它才是「此刻浏览器里的文本」。
 */
function textOf(event, fallbackText) {
    const el = event?.target;
    if (el && typeof el.value === 'string') {
        return el.value;
    }
    return String(fallbackText || '');
}

/** 光标位置；拿不到 `selectionStart` 就退成「文末」。 */
function caret(el, text) {
    return el && typeof el.selectionStart === 'number' ? el.selectionStart : text.length;
}

/** 从行首到 `at` 之间是否只有空白（缩进过的行也算行首）。 */
function blankBefore(text, at) {
    return !/\S/.test(text.slice(text.lastIndexOf('\n', at - 1) + 1, at));
}

/** 光标前那一位是不是**刚打下的**行首 `/`。 */
function slashJustTyped(el, text) {
    const at = caret(el, text) - 1;
    // ⚠️ 不能用 `SLASH_CHARS.includes(text[at] || '')` 兜底：`includes('')` 是 **true**，
    // 越界会变成「任何位置都算」。光标越界本身不该发生，但一个 false 判定比一个
    // 「看运气」的 true 便宜得多。
    const char = at < 0 ? undefined : text[at];
    if (char === undefined || !SLASH_CHARS.includes(char)) {
        return false;
    }
    return blankBefore(text, at);
}

/**
 * 硬件键盘按住的那个 `/` **还没落进文本**（keydown 早于字符插入），判定点在光标**当前位置**。
 *
 * 手机上走不到这里（keydown 给不出 `/`），但桌面上它比 input 早一帧 —— 留着是为了桌面
 * 手感不变。**它只是快路径，不是唯一入口。**
 */
export function slashPendingAt(el, text) {
    const source = String(text || '');
    return blankBefore(source, caret(el, source));
}

/**
 * 这次 `input` 事件是不是「刚在行首打下一个 `/`」——是就该把菜单弹出来。
 *
 * 为什么不能只在 `keydown` 里认 `e.key === '/'`：**手机上 keydown 报不出 `/`**。
 * Android 的输入法（中文/日文）在组合期间会把**每个**按键都报成 `key: "Unidentified"` /
 * `keyCode: 229`，`/` 是从组合里上屏的，只有 `input` 事件看得见它 —— 于是同一套动作
 * 在桌面（有硬件键盘）能用、在手机上完全没反应。收敛到「看文本」这条规则上，两边才等价。
 */
export function slashMenuShouldOpen(event, fallbackText) {
    if (event?.isComposing) {
        // 还在组合：字符没上屏，此刻 element.value 里还没有那个 `/`
        return false;
    }
    const inputType = event?.inputType || '';
    // 空 inputType（部分浏览器的 compositionend）放行；粘贴/拖拽进来的内容不算「刚打了一个 `/`」，
    // 删除、撤销同理 —— 选中一段文字删掉会让光标恰好停在 `/` 后面，那不是用户在打模板。
    if (inputType && !(inputType.startsWith('insert') && !inputType.startsWith('insertFrom'))) {
        return false;
    }
    const el = event?.target;
    return slashJustTyped(el, textOf(event, fallbackText));
}

/**
 * 菜单是否还该挂着：本行**只有**那一个 `/` 才算。
 *
 * 收起来的时机和弹出来一样重要：继续敲别的（终端里的 `/usr/bin`、`/` 开头的日期…）
 * 菜单就该消失 —— 它只在「刚打了一个 `/`、还没写别的」那一瞬间有意义。不收的话，
 * 终端模式里敲一条路径就会一直挂着一排胶囊挡在输入框上面。
 */
export function slashMenuShouldStay(event, fallbackText) {
    const el = event?.target;
    const text = textOf(event, fallbackText);
    const end = caret(el, text);
    const line = text.slice(text.lastIndexOf('\n', end - 1) + 1, end).trim();
    return line.length === 1 && SLASH_CHARS.includes(line);
}

/**
 * 插模板前把光标前那个 `/`（半角/全角）一起换掉 —— 菜单是**替换**掉触发符，不是追加。
 * 只认紧贴光标的那一个，行首缩进、正文里的其他 `/` 都不动。
 */
export function stripTrailingSlash(head) {
    return String(head || '').replace(/[\/／]$/, '');
}
