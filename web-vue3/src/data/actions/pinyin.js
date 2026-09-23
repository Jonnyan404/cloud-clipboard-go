// 汉字注音 —— **三种输出格式**。
//
//   逐字：`答(dá) 复(fù)`     —— 每个字各自标音，保留原字位置，最贴近「注音」的字面意思
//   制表：`dá⇥fù` / `答⇥复`   —— 拼音一行、汉字一行（tab 分隔），垂直对齐，便于对照朗读
//   分词：`答复(dáfù)`        —— 按**词**分组、拼音连写，接近词典/教材的标法
//
// ⚠️ **这个模块必须走动态 import**（见 data/actions.js 里动作定义的 run）：
// pinyin-pro 的产物 317KB，而「给汉字标拼音」大部分人一年用不到一次。
// 放进主包等于让所有人替这几个动作付下载成本。

import { pinyin } from 'pinyin-pro';

/**
 * 逐字注音：`答(dá) 复(fù)`。
 *
 * **保留原字** —— 注音的用途是「对照着读」：只给一串拼音，读者还得自己找位置；
 * 括号标注能同时看到字和音。
 *
 * ⚠️ 用 `type: 'all'`（按字返回数组），**不用默认的字符串模式**：
 * 默认模式返回空格分隔的纯拼音串，**原字和标点全丢了**，没法对照
 * （实测：`pinyin('阿岚 test 123')` → `'ā lán   t e s t   1 2 3'`）。
 */
export function annotate(text) {
    const s = String(text || '');
    if (!s) {
        return '';
    }
    return pinyin(s, { type: 'all' })
        .map((item) => (item.pinyin ? `${item.origin}(${item.pinyin})` : item.origin))
        .join('');
}

/**
 * 制表格式：拼音一行、汉字一行。
 *
 * ⚠️ **返回的是 `{ html, text }` 双表示**，不是单个字符串 —— 这是本文件唯一一个这样做的动作。
 *
 * 为什么必须用 HTML 表格渲染：纯文本对齐在这套字体栈下**做不到**。
 * 代码块的 `font-family` 是 `ui-monospace, Menlo, monospace` —— 全是拉丁等宽字体，
 * **没有汉字字形**，汉字会 fallback 到系统中文字体（macOS 上是 PingFang）。
 * 而 Menlo 的字符宽约 0.6em、PingFang 的汉字宽是 1em —— **汉字不是拉丁字符的整数倍宽**。
 * 于是不管是 tab 还是「按宽度补空格」，拼音行和汉字行的列宽都对不上（用户报的就是这个）。
 * 只有让浏览器自己排版（`<td>`）才能保证对齐。
 *
 * 复制用的 `text` 仍然是 tab 分隔的纯文本 —— 粘到表格软件里能正确分列。
 */
export function toTable(text) {
    const lines = String(text || '').split('\n');
    const rows = [];
    for (const line of lines) {
        if (!line) {
            rows.push(null); // 空行 —— 保留段落结构
            continue;
        }
        const items = pinyin(line, { type: 'all' });
        rows.push({
            // 非汉字（标点、空格、英文）的 pinyin 是空串，用原字符占位，列数才对得上
            pinyin: items.map((item) => item.pinyin || item.origin),
            chars: items.map((item) => item.origin),
        });
    }
    return {
        html: buildTableHtml(rows),
        text: buildTableText(rows),
    };
}

// 单元格样式内联写 —— 这段 HTML 是 `v-html` 进预览区的，**不经过组件的 scoped CSS**
// （scoped 样式靠 data 属性匹配，外部注入的 HTML 上没有那个属性）。
const TABLE_CELL_STYLE = 'padding:0 10px 0 0;white-space:nowrap;text-align:left';

// ⚠️ 必须转义：结果是 v-html 的。DOMPurify 还会再清洗一遍，但转义是更根本的那道防线
// —— 内容是别人发过来的（剪贴板就是个「别人能往你这里塞字符串」的通道）。
function escapeHtml(value) {
    return String(value ?? '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function buildTableHtml(rows) {
    const body = rows.map((row) => {
        if (!row) {
            // 空行给一个矮占位行，别让段落粘在一起
            return '<tr><td style="height:0.7em"></td></tr>';
        }
        const pinyinCells = row.pinyin
            .map((cell) => `<td style="${TABLE_CELL_STYLE}">${escapeHtml(cell)}</td>`)
            .join('');
        const charCells = row.chars
            .map((cell) => `<td style="${TABLE_CELL_STYLE}">${escapeHtml(cell)}</td>`)
            .join('');
        return `<tr>${pinyinCells}</tr><tr>${charCells}</tr>`;
    }).join('');

    // font-family: inherit —— 跟着代码块的等宽字体走，别在这里另立一套
    return `<table style="border-collapse:collapse;font-family:inherit;font-size:inherit">${body}</table>`;
}

function buildTableText(rows) {
    const out = [];
    for (const row of rows) {
        if (!row) {
            out.push('');
            continue;
        }
        out.push(row.pinyin.join('\t'));
        out.push(row.chars.join('\t'));
    }
    return out.join('\n');
}

/**
 * 分词格式：按**词**分组，拼音连写。`答复(dáfù)你好(nǐhǎo)`
 *
 * ⚠️ 用 **`Intl.Segmenter`**（浏览器原生，零依赖）而不是 pinyin-pro 的 `segment` ——
 * 后者是**逐字**切分（实测 `segment('答复你好世界')` 返回 6 个单字），拿不到词边界。
 * 而 `Intl.Segmenter('zh', {granularity:'word'})` 能正确切出 `['答复','你好','世界']`。
 *
 * ⚠️ `Intl.Segmenter` 在 Firefox 125 之前不存在。拿不到就**降级成逐字格式** ——
 * 格式仍然一致（只是没有词分组），总比整个动作报错强。
 */
export function toWordGroups(text) {
    const s = String(text || '');
    if (!s) {
        return '';
    }
    if (typeof Intl === 'undefined' || typeof Intl.Segmenter !== 'function') {
        return annotate(s);
    }

    const segmenter = new Intl.Segmenter('zh', { granularity: 'word' });
    let out = '';
    for (const part of segmenter.segment(s)) {
        if (!part.isWordLike) {
            // 空格、标点原样输出 —— 它们不是「词」，不该被括号包住
            out += part.segment;
            continue;
        }
        const py = pinyin(part.segment).replace(/\s+/g, '');
        // ⚠️ 不含汉字时 `pinyin` 会原样返回（`test` → `t e s t` → 去空格 → `test`），
        // 那就别加括号 —— `test(test)` 是纯噪音。
        out += py === part.segment ? part.segment : `${part.segment}(${py})`;
    }
    return out;
}
