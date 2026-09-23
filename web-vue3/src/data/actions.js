// 动作库：把「用另一种方式看这条内容」做成一张可扩展的表。
//
// 为什么要有它：以前预览区右上角那排图标是**写死的分支** —— useMarkdown 里一个 switch
// （raw / md / code / json / json-min）、MarkdownToggle 里四个固定槽位、再加一套 gutter 宽度计算。
// 想加一个「Base64 解码」要同时改三处。收成注册表之后，**加动作 = 往 ACTIONS 里加一行**。
//
// 两条硬约定（照 data/displayToggles.js 的范式）：
//   · 每个动作必须声明 `group` —— 几十个动作不分组没法看（面板按组渲染小节）
//   · `match` 是**可选**的：声明了就只在内容匹配时出现；不声明就是通用动作，任何文本都能跑
//
// ⚠️ `match` 必须是**廉价纯函数**（只看首字符 / 单个正则，绝不做 JSON.parse）。
// 原因：动作轴要算「这一屏有哪些动作能跑」，会按「一屏条目 × 全部动作」调用它 ——
// 32 个动作 × 100 条就是 3200 次。JSON.parse 是 `run` 的事，不是 `match` 的事。
//
// ⚠️ 动作一律**不发网络请求**，全部在前端算。它是「看」的一部分，和「卡片怎么排版」同一层；
// 看的时候产生请求会让滚动时间流变成几百次调用（设计稿 §8 有完整论证）。
// 唯一要落盘的两个动作（另存 / 覆盖）走现成的 POST /text，**后端零改动**。
//
// ⚠️ 正文上限是 4096 字符（服务端 text.limit），所以这里不需要考虑大文本的性能问题。

import { detectLanguage } from '@/highlight.js';
import {
    decodeHtmlEntities,
    formatJson,
    looksLikeCode,
    looksLikeMarkdown,
    looksLikeTable,
    looksLikeTaskList,
    minifyJson,
    renderMarkdownHtml,
} from '@/util.js';

// ── 分组 ────────────────────────────────────────────────────────────
// 面板按这个顺序渲染小节。加分组要同时补 i18n（四份 locale）。
export const ACTION_GROUPS = [
    { key: 'format', labelKey: 'actionGroupFormat' },
    { key: 'encode', labelKey: 'actionGroupEncode' },
    { key: 'text', labelKey: 'actionGroupText' },
    { key: 'zh', labelKey: 'actionGroupZh' },
    { key: 'inspect', labelKey: 'actionGroupInspect' },
    { key: 'date', labelKey: 'actionGroupDate' },
    { key: 'generate', labelKey: 'actionGroupGenerate' },
];

// ── 廉价判定（match 专用）──────────────────────────────────────────
// 只看首字符 / 一个正则。**别在这里做真正的转换** —— 那是 run 的活。

function isJsonLike(text) {
    const s = String(text || '').trim();
    return s.length > 1 && (s[0] === '{' || s[0] === '[');
}

// TextDecoder 实例复用 —— match 会被「一屏条目 × 全部动作」地调用，每次 new 一个解码器是白开销。
const utf8StrictDecoder = new TextDecoder('utf-8', { fatal: true });

function isBase64Like(text) {
    const s = String(text || '').trim();
    // ① 形状：至少 8 位（更短的解码出来没有意义）+ 长度是 4 的倍数 + 只含 base64 字符集。
    if (s.length < 8 || s.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(s)) {
        return false;
    }
    // ② 内容：解码出来必须是**合法 UTF-8**。
    //
    // ⚠️ 光靠 ① 会把 `abcdefgh` 这种普通英文单词也判成 base64（它确实只由 base64 字符组成）——
    // 于是每条英文短句都多一个「Base64 解码」，点下去才发现是错的。
    // 补这一步之后，纯字母单词解出来是非法字节序列，自然被挡在外面。
    //
    // 这一步仍然廉价（atob 是纯计算，几十字节的输入是微秒级），但**绝不能省**。
    try {
        const binary = atob(s);
        utf8StrictDecoder.decode(Uint8Array.from(binary, (ch) => ch.charCodeAt(0)));
        return true;
    } catch {
        return false;
    }
}

function isUrlEncodedLike(text) {
    return /%[0-9A-Fa-f]{2}/.test(String(text || ''));
}

function isHtmlEntityLike(text) {
    return /&(#\d+|#x[0-9A-Fa-f]+|[a-zA-Z]+);/.test(String(text || ''));
}

function isUnicodeEscapedLike(text) {
    return /\\u[0-9A-Fa-f]{4}/.test(String(text || ''));
}

function hasMultipleLines(text) {
    return String(text || '').includes('\n');
}

function isLongerThan(min) {
    return (text) => String(text || '').length > min;
}

// ── 编解码实现 ──────────────────────────────────────────────────────
// ⚠️ 中文必须过 UTF-8。直接 `btoa('中文')` 会抛 InvalidCharacterError ——
// btoa 只接受 Latin-1，而剪贴板里中文是常态。

function utf8ToBase64(text) {
    const bytes = new TextEncoder().encode(text);
    let binary = '';
    for (const byte of bytes) {
        binary += String.fromCharCode(byte);
    }
    return btoa(binary);
}

function base64ToUtf8(text) {
    const binary = atob(String(text || '').trim());
    const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
    return new TextDecoder().decode(bytes);
}

function utf8ToHex(text) {
    return Array.from(new TextEncoder().encode(text))
        .map((byte) => byte.toString(16).padStart(2, '0'))
        .join(' ');
}

function hexToUtf8(text) {
    const clean = String(text || '').replace(/[^0-9A-Fa-f]/g, '');
    if (clean.length % 2 !== 0) {
        throw new Error('hex 长度必须是偶数');
    }
    const bytes = new Uint8Array(clean.length / 2);
    for (let i = 0; i < clean.length; i += 2) {
        bytes[i / 2] = parseInt(clean.slice(i, i + 2), 16);
    }
    return new TextDecoder().decode(bytes);
}

// 转义成 `\uXXXX`。
//
// ⚠️ 必须按 **UTF-16 单元**遍历（`charCodeAt`），不能用 `Array.from` 按码点遍历。
// 按码点的话 emoji（如 🪶，一个码点两个单元）会被当成**单个字符**，
// 而 `charCodeAt(0)` 只取到高代理项 —— 输出 `\ud83e`，低半截直接丢了，
// 而且**往返不回来**（实测踩到过：`岚🪶` → `岚\ud83e`）。
//
// 只转义非 ASCII / 不可打印字符，其余原样保留 —— 全转的话 `abc` 会变成
// `\u0061\u0062\u0063`，读不了也没意义。
function toUnicodeEscapes(text) {
    const source = String(text || '');
    let out = '';
    for (let i = 0; i < source.length; i++) {
        const code = source.charCodeAt(i);
        out += (code > 0x7e || code < 0x20)
            ? `\\u${code.toString(16).padStart(4, '0')}`
            : source[i];
    }
    return out;
}

function fromUnicodeEscapes(text) {
    return String(text || '').replace(/\\u([0-9A-Fa-f]{4})/g, (_, hex) => String.fromCharCode(parseInt(hex, 16)));
}

function encodeHtmlEntities(text) {
    return String(text || '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// ── 文本处理实现 ────────────────────────────────────────────────────

function dedupeLines(text) {
    const seen = new Set();
    const out = [];
    for (const line of String(text || '').split('\n')) {
        // 空行不去重 —— 连着几个空行是有意的排版，去掉会改变结构
        if (line.trim() && seen.has(line)) {
            continue;
        }
        seen.add(line);
        out.push(line);
    }
    return out.join('\n');
}

function sortLines(text) {
    return String(text || '').split('\n').sort((a, b) => a.localeCompare(b, 'zh')).join('\n');
}

function dropBlankLines(text) {
    return String(text || '').split('\n').filter((line) => line.trim()).join('\n');
}

function trimEachLine(text) {
    return String(text || '').split('\n').map((line) => line.trim()).join('\n');
}

function reverseText(text) {
    // 用 Array.from 而不是 split('')：中文之外还有 emoji，代理对拆开就成乱码
    return Array.from(String(text || '')).reverse().join('');
}

// ── 代码视图（转义 + 高亮）──────────────────────────────────────────
// 把整段文本当成**一个围栏代码块**渲染。
//
// 为什么不直接 v-html 一个 <pre>：绕 markdown 这一圈能顺带吃到 MarkdownBody 里的代码高亮，
// 消费方也不用为「代码」单独写一套渲染分支。
//
// ⚠️ 围栏长度必须**比正文里最长的连续反引号还长**，否则正文自带的 ``` 会把围栏提前闭合。
//
// 导出是因为 useMarkdown 也要用它：动作产出的**纯文本**结果（编解码、文本处理）
// 统一包成 `<pre>` 形式的 HTML，消费方就只需要认 `md.html` 一个分支
// （多一个 `md.text` 分支等于要改 5 个消费方）。
export function renderFenced(code, language = '') {
    const runs = String(code).match(/`+/g) || [];
    const fence = '`'.repeat(Math.max(3, ...runs.map((run) => run.length + 1)));
    return renderMarkdownHtml(`${fence}${language}\n${code}\n${fence}`);
}

// ── 中文 ────────────────────────────────────────────────────────────

// 全角 ↔ 半角：ASCII 可见字符（0x21-0x7E）与全角（U+FF01-U+FF5E）相差 0xFEE0。
//
// ⚠️ 空格必须**单独**处理：0x20 + 0xFEE0 = U+FF00，那是个**未分配字符**，不是全角空格。
// 全角空格是 U+3000，得手写。踩过这个坑的话表现是「转换后空格变成方块/问号」。
function toFullWidth(text) {
    return String(text || '')
        .replace(/[!-~]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) + 0xfee0))
        .replace(/ /g, '\u3000');
}

function toHalfWidth(text) {
    return String(text || '')
        .replace(/[\uff01-\uff5e]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - 0xfee0))
        .replace(/\u3000/g, ' ');
}

// 中文标点 → 英文标点。逐字符查表。
// （`——` 这类双字符标点会被拆成两个 `-`，可接受 —— 为它做长串匹配不值得。）
const CN_PUNCT_MAP = {
    '，': ',', '。': '.', '、': ',', '；': ';', '：': ':',
    '？': '?', '！': '!', '（': '(', '）': ')', '【': '[', '】': ']',
    '《': '<', '》': '>', '「': '"', '」': '"', '『': "'", '』': "'",
    '“': '"', '”': '"', '‘': "'", '’': "'", '～': '~',
    '…': '.', '—': '-', '－': '-', '　': ' ',
};

function cnPunctuationToEn(text) {
    return Array.from(String(text || '')).map((ch) => CN_PUNCT_MAP[ch] ?? ch).join('');
}

// 数字 → 中文大写。
//
// ⚠️ 必须**分节**处理（4 位一节 + 万/亿），不能逐位查表：中文的单位是**组合**的
// （十万 = 十 + 万），逐位法根本表达不出来。
// 节内的「零」也要合并：`1001` → 一千零一（不是一千零零一），`10000` → 一万（不是一万零）。
const CN_DIGITS = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九'];
const CN_SMALL_UNITS = ['', '十', '百', '千'];
const CN_BIG_UNITS = ['', '万', '亿', '万亿'];

function cnSectionToText(section) {
    let out = '';
    let zeroPending = false;
    for (let i = 0; i < section.length; i++) {
        const digit = Number(section[i]);
        const unit = CN_SMALL_UNITS[section.length - 1 - i];
        if (digit === 0) {
            zeroPending = true;
            continue;
        }
        if (zeroPending && out) {
            out += CN_DIGITS[0];
        }
        zeroPending = false;
        out += CN_DIGITS[digit] + unit;
    }
    return out;
}

function numberToChinese(raw) {
    const s = String(raw || '').trim();
    if (!/^-?\d+(\.\d+)?$/.test(s)) {
        throw new Error('不是合法数字');
    }
    const negative = s.startsWith('-');
    const body = negative ? s.slice(1) : s;
    const [intPart, decPart] = body.split('.');

    let intText = CN_DIGITS[0];
    const trimmed = intPart.replace(/^0+/, '');
    if (trimmed) {
        // 从右往左每 4 位切一节，chunks[0] 是最低节
        const chunks = [];
        for (let i = trimmed.length; i > 0; i -= 4) {
            chunks.unshift(trimmed.slice(Math.max(0, i - 4), i));
        }
        let out = '';
        chunks.forEach((chunk, idx) => {
            const bigUnit = CN_BIG_UNITS[chunks.length - 1 - idx];
            const sectionText = cnSectionToText(chunk);
            if (!sectionText) {
                // 整节是 0：只有前面已经有内容才补零（`10000` 不该变成「一万零」）
                if (out && !out.endsWith(CN_DIGITS[0])) {
                    out += CN_DIGITS[0];
                }
                return;
            }
            // 这节有内容、但首位是 0（数值不满千）且前面已有输出 → 中间要补零
            // （`10000001` → 一千万**零**一）
            if (out && chunk[0] === '0' && !out.endsWith(CN_DIGITS[0])) {
                out += CN_DIGITS[0];
            }
            out += sectionText + bigUnit;
        });
        intText = out.replace(/零+$/, '');
        // 中文习惯说「十」「十二」，不说「一十」「一十二」；但「一百一十」要保留
        intText = intText.replace(/^一十/, '十');
    }

    let out = intText;
    if (decPart) {
        out += '点' + Array.from(decPart).map((d) => CN_DIGITS[Number(d)]).join('');
    }
    return (negative ? '负' : '') + out;
}

// ── 分析 ────────────────────────────────────────────────────────────
//
// ⚠️ 这几个的 run 会用到 `ctx.t` —— 统计报告的标签是**给人看的文案**，
// 硬编码中文会让 en / ja 用户看到一堆中文。ctx 由调用方（组件）注入 t 函数，
// 拿不到时退化成原样返回 key（测试里就是这么用的）。

function translator(ctx) {
    return typeof ctx?.t === 'function' ? ctx.t : (key) => key;
}

function textStats(text, ctx) {
    const tr = translator(ctx);
    const s = String(text || '');
    const chars = Array.from(s).length;
    const lines = s ? s.split('\n').length : 0;
    const bytes = new TextEncoder().encode(s).length;
    const cjk = (s.match(/[\u4e00-\u9fff]/g) || []).length;
    // 词数：中文没有空格分词，按「汉字数 + 西文单词数」算 —— 混排时才不会漏
    const latinWords = (s.replace(/[\u4e00-\u9fff]/g, ' ').match(/[A-Za-z0-9_'’-]+/g) || []).length;
    return [
        `${tr('inspectChars')}: ${chars}`,
        `${tr('inspectWords')}: ${latinWords + cjk}`,
        `${tr('inspectLines')}: ${lines}`,
        `${tr('inspectBytes')}: ${bytes}`,
        `${tr('inspectCjk')}: ${cjk}`,
    ].join('\n');
}

/** Unix 时间戳（秒或毫秒）→ 本地时间。 */
function timestampToDate(text) {
    const s = String(text || '').trim();
    if (!/^\d{9,13}$/.test(s)) {
        throw new Error('不是 10 位（秒）或 13 位（毫秒）时间戳');
    }
    const ms = s.length >= 13 ? Number(s) : Number(s) * 1000;
    const d = new Date(ms);
    if (Number.isNaN(d.getTime())) {
        throw new Error('时间戳超出可表示范围');
    }
    const pad2 = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} `
        + `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`;
}

/** 日期字符串 → Unix 时间戳（秒）。 */
function dateToTimestamp(text) {
    const s = String(text || '').trim();
    // 把 `2026/09/23` 这种斜杠写法归一成 `-`（Safari 对斜杠的解析行为不一致）
    const ms = Date.parse(s.replace(/\//g, '-'));
    if (Number.isNaN(ms)) {
        throw new Error('认不出这个日期');
    }
    return String(Math.floor(ms / 1000));
}

/** 猜这条内容是什么编码 / 什么格式。只给结论，不做转换。 */
function detectFormat(text, ctx) {
    const tr = translator(ctx);
    const s = String(text || '').trim();
    const hits = [];
    if (isJsonLike(s)) {
        try {
            JSON.parse(s);
            hits.push('JSON');
        } catch {
            // 首字符像 JSON 但不是合法 JSON —— 不列，避免误导
        }
    }
    if (isBase64Like(s)) {
        hits.push('Base64');
    }
    if (/^(?:[0-9A-Fa-f]{2}[\s-]?)+$/.test(s) && s.replace(/[^0-9A-Fa-f]/g, '').length % 2 === 0) {
        hits.push('Hex');
    }
    if (isUrlEncodedLike(s)) {
        hits.push('URL encoded');
    }
    if (isHtmlEntityLike(s)) {
        hits.push('HTML entity');
    }
    if (isUnicodeEscapedLike(s)) {
        hits.push('Unicode escape');
    }
    if (/^\d{9,13}$/.test(s)) {
        hits.push('Unix timestamp');
    }
    if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s)) {
        hits.push('UUID');
    }
    if (/^#[0-9A-Fa-f]{3,8}$/.test(s)) {
        hits.push('Hex color');
    }
    if (looksLikeMarkdown(s)) {
        hits.push('Markdown');
    }
    return hits.length ? hits.join(' / ') : tr('inspectNoMatch');
}

/**
 * 哈希。**这是唯一一个必须异步的动作** —— `crypto.subtle.digest` 只返回 Promise。
 *
 * ⚠️ `crypto.subtle` 只在**安全上下文**（HTTPS / localhost）里存在。
 * 自托管的剪贴板服务常被用 `http://192.168.x.x` 打开 —— 那时它是 undefined，
 * 必须给出**说得清原因**的错误，而不是一句「执行失败」。
 */
async function sha256(text, ctx) {
    const tr = translator(ctx);
    if (!globalThis.crypto?.subtle) {
        throw new Error(tr('inspectHashNeedsHttps'));
    }
    const bytes = new TextEncoder().encode(String(text || ''));
    const digest = await globalThis.crypto.subtle.digest('SHA-256', bytes);
    return Array.from(new Uint8Array(digest))
        .map((b) => b.toString(16).padStart(2, '0'))
        .join('');
}

// ── 日期计算 ────────────────────────────────────────────────────────
//
// ⚠️ 动作是**单输入单输出**的，而日期间隔天然需要两个日期、日期加减需要日期 + 偏移量。
// 这里用**约定式解析**：把两个参数写在同一段文本里。
//   · date.add ：`2026-01-01 +30d`（省略基准则从今天算）
//   · date.diff：两行日期，或 `2026-01-01 ~ 2026-03-15`
// 好处是**零新增机制** —— 不用给动作加参数系统，也就不用碰链的语义。
// 代价是用户得按约定写；等真出现高频需求再考虑参数化。

// 「今天 / 明天 / 昨天」这类相对词。
const DATE_KEYWORDS = {
    今天: 0, 明天: 1, 昨天: -1,
    today: 0, tomorrow: 1, yesterday: -1,
};

// 一个「日期 token」的完整形状。三种写法都认：
//   ISO / 斜杠：2026-09-23、2026/9/23
//   中文：      2026年09月23日、2026年9月23日
//   紧凑：      20260923（8 位）
// 前两种可以再跟时间（` 10:30` 或 `T10:30:00`）。
//
// ⚠️ 紧凑格式的月/日要**限位**（`0[1-9]|1[0-2]` / `0[1-9]|[12]\d|3[01]`）：
// 只写 `\d{2}\d{2}` 的话，`12345678` 这种普通数字串也会被判成日期 ——
// 于是每段 8 位数字都冒出一个「日期加减」。
const DATE_CORE = String.raw`(?:\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{4}年\d{1,2}月\d{1,2}日?|\d{4}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01]))`;
const DATE_TIME_PART = String.raw`(?:[ T]\d{1,2}:\d{2}(?::\d{2})?)?`;
const DATE_TOKEN_RE = new RegExp(
    `^(?:${DATE_CORE}${DATE_TIME_PART}|今天|明天|昨天|today|tomorrow|yesterday)?$`,
    'i',
);

/**
 * 解析一个日期 token，返回**本地时间**的 Date；认不出返回 null。
 *
 * 认这几种：`2026-09-23`、`2026/9/23`、`2026年09月23日`、`20260923`、
 * 前三种带时间（`2026-09-23 10:30`）、以及 `今天` / `明天` / `昨天`。
 *
 * ⚠️ 必须用 `new Date(y, m-1, d, ...)` **本地构造**，不能用 `new Date('2026-01-01')` ——
 * 后者按 **UTC** 解析，在西半球会变成「前一天」，日期计算直接错一天。
 */
function parseDateToken(raw) {
    const s = String(raw || '').trim();
    if (!s) {
        return null; // 空 = 调用方自己决定默认值（date.add 用「今天」）
    }

    const offsetDays = DATE_KEYWORDS[s.toLowerCase()];
    if (offsetDays !== undefined) {
        const d = new Date();
        d.setDate(d.getDate() + offsetDays);
        return d;
    }

    // 带时间分隔符的两种写法（ISO / 斜杠、中文），以及不带时间的紧凑 8 位
    const m = s.match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?)?$/)
        || s.match(/^(\d{4})年(\d{1,2})月(\d{1,2})日?(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?)?$/)
        || s.match(/^(\d{4})(\d{2})(\d{2})$/);
    if (!m) {
        return null;
    }
    const [, y, mo, d, hh, mi, ss] = m;
    const date = new Date(Number(y), Number(mo) - 1, Number(d), Number(hh || 0), Number(mi || 0), Number(ss || 0));

    // ⚠️ 回读校验：`2026-02-31` 会被 Date **悄悄滚到** 3 月 3 日。不校验的话，
    // 用户写错了日期却拿到一个「看起来正常」的结果 —— 比直接报错糟糕得多。
    if (date.getFullYear() !== Number(y) || date.getMonth() !== Number(mo) - 1 || date.getDate() !== Number(d)) {
        return null;
    }
    return date;
}

function formatDate(date, withTime) {
    const p = (n) => String(n).padStart(2, '0');
    const base = `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}`;
    return withTime ? `${base} ${p(date.getHours())}:${p(date.getMinutes())}` : base;
}

/**
 * 加 N 个月。
 *
 * ⚠️ 不能直接 `setMonth(getMonth() + n)` —— 「1月31日 + 1个月」会因为 2 月没有 31 号
 * 而**溢出到 3月3日**。正确做法：先把「日」归到 1 号再加月（避免滚动），
 * 最后把原来的「日」**夹到目标月的最后一天**（`1月31日 + 1个月` = `2月28/29日`）。
 */
function addMonths(date, n) {
    const day = date.getDate();
    const result = new Date(date.getTime());
    result.setDate(1);
    result.setMonth(result.getMonth() + n);
    const lastDay = new Date(result.getFullYear(), result.getMonth() + 1, 0).getDate();
    result.setDate(Math.min(day, lastDay));
    return result;
}

/** 本地零点的时间戳 —— 算「差几天」要用它，不能用毫秒差（夏令时那天是 23/25 小时）。 */
function startOfDayMs(date) {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

const DAY_MS = 86400000;

function dateAdd(text, ctx) {
    const tr = translator(ctx);
    const s = String(text || '').trim();
    const m = s.match(/^(.*?)\s*([+-])\s*(\d+)\s*([dwmy]?)\s*$/i);
    if (!m) {
        throw new Error(tr('actionDateAddHint'));
    }
    const [, baseRaw, sign, amountRaw, unitRaw] = m;

    const parsedBase = parseDateToken(baseRaw);
    if (baseRaw.trim() && !parsedBase) {
        // 写了基准但认不出 → 报错。**别悄悄回落到「今天」** —— 那会让用户拿到一个
        // 看着合理、其实完全不对的结果。
        throw new Error(tr('actionDateUnrecognized'));
    }

    const base = parsedBase || new Date(); // 没写基准 = 今天
    const amount = Number(amountRaw) * (sign === '-' ? -1 : 1);
    const unit = (unitRaw || 'd').toLowerCase();
    const withTime = /:/.test(baseRaw); // 基准带时间就保留时间，否则只给日期

    let result;
    if (unit === 'm') {
        result = addMonths(base, amount);
    } else if (unit === 'y') {
        result = addMonths(base, amount * 12);
    } else {
        result = new Date(base.getTime());
        result.setDate(result.getDate() + (unit === 'w' ? amount * 7 : amount));
    }
    return formatDate(result, withTime);
}

function dateDiff(text, ctx) {
    const tr = translator(ctx);
    const s = String(text || '').trim();

    // 两个日期：优先按分隔符拆，否则按行拆（一行一个）
    let parts = s.split(/\s*(?:~|～|→|->|至|到|\.\.+)\s*/).map((x) => x.trim()).filter(Boolean);
    if (parts.length < 2) {
        parts = s.split('\n').map((x) => x.trim()).filter(Boolean);
    }
    if (parts.length < 2) {
        throw new Error(tr('actionDateDiffHint'));
    }

    const a = parseDateToken(parts[0]);
    const b = parseDateToken(parts[1]);
    if (!a || !b) {
        throw new Error(tr('actionDateUnrecognized'));
    }

    const days = Math.round((startOfDayMs(b) - startOfDayMs(a)) / DAY_MS);
    const abs = Math.abs(days);
    const weeks = Math.floor(abs / 7);
    const rest = abs % 7;

    const lines = [`${days} ${tr('actionDateUnitDay')}`];
    if (weeks) {
        lines.push(`${weeks} ${tr('actionDateUnitWeek')} ${rest} ${tr('actionDateUnitDay')}`);
    }
    return lines.join('\n');
}

// match 要**廉价**：只用形状判断，不真去解析（解析是 run 的事）。
function looksLikeDateAdd(text) {
    const s = String(text || '').trim();
    if (!s || s.length > 40) {
        return false;
    }
    const m = s.match(/^(.*?)\s*([+-]\s*\d+\s*[dwmy]?)$/i);
    return Boolean(m) && DATE_TOKEN_RE.test(m[1].trim());
}

function looksLikeDateDiff(text) {
    const s = String(text || '').trim();
    if (!s || s.length > 80) {
        return false;
    }
    let parts = s.split(/\s*(?:~|～|→|->|至|到|\.\.+)\s*/).map((x) => x.trim()).filter(Boolean);
    if (parts.length < 2) {
        parts = s.split('\n').map((x) => x.trim()).filter(Boolean);
    }
    return parts.length === 2 && DATE_TOKEN_RE.test(parts[0]) && DATE_TOKEN_RE.test(parts[1]);
}

// ── 生成类（无输入 → 新正文）────────────────────────────────────────
// 这些动作的 run 忽略入参，只看时间。

function pad(n) {
    return String(n).padStart(2, '0');
}

function nowTime() {
    const d = new Date();
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function nowDateTime() {
    const d = new Date();
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${nowTime()}`;
}

function newUuid() {
    if (globalThis.crypto && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }
    // 老浏览器兜底（非安全上下文里 crypto.randomUUID 不存在）
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        const v = c === 'x' ? r : ((r & 0x3) | 0x8);
        return v.toString(16);
    });
}

// ── 动作表 ──────────────────────────────────────────────────────────
//
// 字段：
//   id       全局唯一，点号分段（格式 `分组.名字[.方向]`）
//   group    面板分组，必填
//   nameKey  i18n key，四份 locale 都要有
//   icon     mdi 图标名
//   direction 'view'（看，出现在预览/工作台）| 'insert'（发，出现在输入框）
//   render   'text'（默认，按纯文本显示）| 'html'（结果是 HTML，要 v-html 渲染）
//   match    可选。返回 true 表示「这条内容能用这个动作」
//   run      (text) => string。抛异常 = 这个动作对当前内容不适用（界面显示错误 + 退回原文）
//
// ⚠️ run 允许抛错，**不允许返回空串来表示失败** —— 空串是合法结果（比如"去空行"全空）。
// 失败必须显式抛，否则界面分不清「结果就是空」和「没跑成」。

export const ACTIONS = [
    // ── 格式化 ──────────────────────────────────────────────────────
    {
        id: 'format.markdown',
        group: 'format',
        nameKey: 'actionMarkdown',
        icon: 'mdi-language-markdown',
        direction: 'view',
        render: 'html',
        match: (text) => looksLikeMarkdown(text) || looksLikeTaskList(text) || looksLikeTable(text),
        run: (text) => renderMarkdownHtml(text),
    },
    {
        id: 'format.code',
        group: 'format',
        nameKey: 'actionCode',
        icon: 'mdi-code-braces',
        direction: 'view',
        render: 'html',
        match: (text) => looksLikeCode(text),
        // 语言检测是**异步**的（highlight.js 的 highlightAuto），所以这个动作是 async。
        // 不检测也能渲染（只是没颜色），但「代码高亮」的全部价值就在颜色上 ——
        // 原来的 useMarkdown 里就有这一步，别在重构里弄丢。
        run: async (text) => {
            const language = await detectLanguage(text);
            return renderFenced(text, language);
        },
    },
    {
        id: 'format.json.pretty',
        group: 'format',
        nameKey: 'actionJsonPretty',
        icon: 'mdi-format-indent-increase',
        direction: 'view',
        match: (text) => isJsonLike(text),
        run: (text) => {
            const out = formatJson(text);
            if (!out) {
                throw new Error('已经是美化过的 JSON');
            }
            return out;
        },
    },
    {
        id: 'format.json.min',
        group: 'format',
        nameKey: 'actionJsonMin',
        icon: 'mdi-format-indent-decrease',
        direction: 'view',
        match: (text) => isJsonLike(text),
        run: (text) => {
            const out = minifyJson(text);
            if (!out) {
                throw new Error('已经是一行，压不动了');
            }
            return out;
        },
    },

    // ── 编解码 ──────────────────────────────────────────────────────
    {
        id: 'encode.base64',
        group: 'encode',
        nameKey: 'actionBase64Encode',
        icon: 'mdi-lock-outline',
        direction: 'view',
        run: (text) => utf8ToBase64(text),
    },
    {
        id: 'encode.base64.decode',
        group: 'encode',
        nameKey: 'actionBase64Decode',
        icon: 'mdi-lock-open-outline',
        direction: 'view',
        match: (text) => isBase64Like(text),
        run: (text) => base64ToUtf8(text),
    },
    {
        id: 'encode.url',
        group: 'encode',
        nameKey: 'actionUrlEncode',
        icon: 'mdi-link-variant',
        direction: 'view',
        run: (text) => encodeURIComponent(text),
    },
    {
        id: 'encode.url.decode',
        group: 'encode',
        nameKey: 'actionUrlDecode',
        icon: 'mdi-link-variant-off',
        direction: 'view',
        match: (text) => isUrlEncodedLike(text),
        run: (text) => decodeURIComponent(text),
    },
    {
        id: 'encode.html',
        group: 'encode',
        nameKey: 'actionHtmlEncode',
        icon: 'mdi-code-tags',
        direction: 'view',
        run: (text) => encodeHtmlEntities(text),
    },
    {
        id: 'encode.html.decode',
        group: 'encode',
        nameKey: 'actionHtmlDecode',
        icon: 'mdi-code-tags-check',
        direction: 'view',
        match: (text) => isHtmlEntityLike(text),
        run: (text) => decodeHtmlEntities(text),
    },
    {
        id: 'encode.unicode',
        group: 'encode',
        nameKey: 'actionUnicodeEncode',
        icon: 'mdi-format-letter-case',
        direction: 'view',
        run: (text) => toUnicodeEscapes(text),
    },
    {
        id: 'encode.unicode.decode',
        group: 'encode',
        nameKey: 'actionUnicodeDecode',
        icon: 'mdi-format-letter-case-lower',
        direction: 'view',
        match: (text) => isUnicodeEscapedLike(text),
        run: (text) => fromUnicodeEscapes(text),
    },
    {
        id: 'encode.hex',
        group: 'encode',
        nameKey: 'actionHexEncode',
        icon: 'mdi-numeric',
        direction: 'view',
        run: (text) => utf8ToHex(text),
    },
    {
        id: 'encode.hex.decode',
        group: 'encode',
        nameKey: 'actionHexDecode',
        icon: 'mdi-numeric-off',
        direction: 'view',
        match: (text) => /^(?:[0-9A-Fa-f]{2}[\s-]?)+$/.test(String(text || '').trim()),
        run: (text) => hexToUtf8(text),
    },

    // ── 文本 ────────────────────────────────────────────────────────
    {
        id: 'text.upper',
        group: 'text',
        nameKey: 'actionUpperCase',
        icon: 'mdi-format-letter-case-upper',
        direction: 'view',
        run: (text) => String(text || '').toUpperCase(),
    },
    {
        id: 'text.lower',
        group: 'text',
        nameKey: 'actionLowerCase',
        icon: 'mdi-format-letter-case-lower',
        direction: 'view',
        run: (text) => String(text || '').toLowerCase(),
    },
    {
        id: 'text.dedupe',
        group: 'text',
        nameKey: 'actionDedupeLines',
        icon: 'mdi-filter-remove-outline',
        direction: 'view',
        match: hasMultipleLines,
        run: (text) => dedupeLines(text),
    },
    {
        id: 'text.sort',
        group: 'text',
        nameKey: 'actionSortLines',
        icon: 'mdi-sort-alphabetical-ascending',
        direction: 'view',
        match: hasMultipleLines,
        run: (text) => sortLines(text),
    },
    {
        id: 'text.dropBlank',
        group: 'text',
        nameKey: 'actionDropBlankLines',
        icon: 'mdi-format-line-spacing',
        direction: 'view',
        match: hasMultipleLines,
        run: (text) => dropBlankLines(text),
    },
    {
        id: 'text.trimLines',
        group: 'text',
        nameKey: 'actionTrimLines',
        icon: 'mdi-format-horizontal-align-left',
        direction: 'view',
        match: hasMultipleLines,
        run: (text) => trimEachLine(text),
    },
    {
        id: 'text.reverse',
        group: 'text',
        nameKey: 'actionReverse',
        icon: 'mdi-swap-horizontal',
        direction: 'view',
        match: isLongerThan(1),
        run: (text) => reverseText(text),
    },

    // ── 中文 ────────────────────────────────────────────────────────
    {
        id: 'zh.fullwidth',
        group: 'zh',
        nameKey: 'actionFullWidth',
        icon: 'mdi-format-vertical-align-center',
        direction: 'view',
        // 「含 ASCII 可打印字符」= 还有得转。已经全是全角的文本不该出现这个动作。
        match: (text) => /[!-~]/.test(String(text || '')),
        run: (text) => toFullWidth(text),
    },
    {
        id: 'zh.halfwidth',
        group: 'zh',
        nameKey: 'actionHalfWidth',
        icon: 'mdi-format-horizontal-align-center',
        direction: 'view',
        match: (text) => /[\uff01-\uff5e\u3000]/.test(String(text || '')),
        run: (text) => toHalfWidth(text),
    },
    {
        id: 'zh.punctuation',
        group: 'zh',
        nameKey: 'actionCnPunctuation',
        icon: 'mdi-comma',
        direction: 'view',
        match: (text) => /[，。、；：？！（）【】《》「」『』“”‘’…—－　]/.test(String(text || '')),
        run: (text) => cnPunctuationToEn(text),
    },
    {
        id: 'zh.number',
        group: 'zh',
        nameKey: 'actionNumberToChinese',
        icon: 'mdi-numeric-1-box-outline',
        direction: 'view',
        match: (text) => /^-?\d+(\.\d+)?$/.test(String(text || '').trim()),
        run: (text) => numberToChinese(text),
    },
    {
        id: 'zh.pinyin',
        group: 'zh',
        nameKey: 'actionPinyin',
        icon: 'mdi-ideogram-cjk',
        direction: 'view',
        match: (text) => /[\u4e00-\u9fff]/.test(String(text || '')),
        // ⚠️ 动态 import = **懒加载**：拼音表 317KB，只在用户真的点了这个动作时才下载，
        // 不进主包。ESM 自己缓存模块，所以下面三个注音动作不用各写一层缓存。
        run: async (text) => {
            const mod = await import('./actions/pinyin.js');
            return mod.annotate(text);
        },
    },
    {
        id: 'zh.pinyin.table',
        group: 'zh',
        nameKey: 'actionPinyinTable',
        icon: 'mdi-table',
        direction: 'view',
        match: (text) => /[\u4e00-\u9fff]/.test(String(text || '')),
        // 制表格式：拼音一行、汉字一行（tab 分隔），垂直对齐便于对照朗读
        run: async (text) => {
            const mod = await import('./actions/pinyin.js');
            return mod.toTable(text);
        },
    },
    {
        id: 'zh.pinyin.word',
        group: 'zh',
        nameKey: 'actionPinyinWord',
        icon: 'mdi-format-letter-matches',
        direction: 'view',
        match: (text) => /[\u4e00-\u9fff]/.test(String(text || '')),
        // 分词格式：按词分组、拼音连写。分词走**浏览器原生**的 Intl.Segmenter（零依赖）。
        run: async (text) => {
            const mod = await import('./actions/pinyin.js');
            return mod.toWordGroups(text);
        },
    },
    {
        id: 'zh.simplified',
        group: 'zh',
        nameKey: 'actionToSimplified',
        icon: 'mdi-swap-horizontal-bold',
        direction: 'view',
        match: (text) => /[\u4e00-\u9fff]/.test(String(text || '')),
        // 懒加载（opencc-js 带词典，6MB 量级），同上。
        //
        // ⚠️ 转完和原文一样就**报错**，不返回原样 —— 「点了没反应」是最糟的体验，
        // 用户分不清是「没有可转的字」还是「功能坏了」。match 挡不住这种情况：
        // 判「是不是繁体」需要词典，而 match 必须廉价。
        run: async (text, ctx) => {
            const mod = await import('./actions/opencc.js');
            const source = String(text || '');
            const out = mod.toSimplified(source);
            if (out === source) {
                throw new Error(translator(ctx)('actionNothingToConvert'));
            }
            return out;
        },
    },
    {
        id: 'zh.traditional',
        group: 'zh',
        nameKey: 'actionToTraditional',
        icon: 'mdi-swap-horizontal',
        direction: 'view',
        match: (text) => /[\u4e00-\u9fff]/.test(String(text || '')),
        run: async (text, ctx) => {
            const mod = await import('./actions/opencc.js');
            const source = String(text || '');
            const out = mod.toTraditional(source);
            if (out === source) {
                throw new Error(translator(ctx)('actionNothingToConvert'));
            }
            return out;
        },
    },

    // ── 分析 ────────────────────────────────────────────────────────
    {
        id: 'inspect.stats',
        group: 'inspect',
        nameKey: 'actionTextStats',
        icon: 'mdi-counter',
        direction: 'view',
        run: (text, ctx) => textStats(text, ctx),
    },
    {
        id: 'inspect.detect',
        group: 'inspect',
        nameKey: 'actionDetectFormat',
        icon: 'mdi-magnify-scan',
        direction: 'view',
        run: (text, ctx) => detectFormat(text, ctx),
    },
    {
        id: 'inspect.timestamp',
        group: 'inspect',
        nameKey: 'actionTimestampToDate',
        icon: 'mdi-clock-check-outline',
        direction: 'view',
        match: (text) => /^\d{9,13}$/.test(String(text || '').trim()),
        run: (text) => timestampToDate(text),
    },
    {
        id: 'inspect.dateToTimestamp',
        group: 'inspect',
        nameKey: 'actionDateToTimestamp',
        icon: 'mdi-clock-plus-outline',
        direction: 'view',
        // ⚠️ Date.parse 相对贵，**不能**对每条内容都跑。先用长度 + 形状挡一道，
        // 形状像日期了才真去解析 —— 这是 match 必须廉价这条约定的具体落法。
        match: (text) => {
            const s = String(text || '').trim();
            return s.length >= 8 && s.length <= 32
                && /\d{4}[-/]\d{1,2}[-/]\d{1,2}/.test(s)
                && !Number.isNaN(Date.parse(s.replace(/\//g, '-')));
        },
        run: (text) => dateToTimestamp(text),
    },
    {
        id: 'inspect.sha256',
        group: 'inspect',
        nameKey: 'actionSha256',
        icon: 'mdi-fingerprint',
        direction: 'view',
        run: (text, ctx) => sha256(text, ctx),
    },

    // ── 日期 ────────────────────────────────────────────────────────
    //
    // 这两个的输入是**约定式**的（见上面 dateAdd / dateDiff 的说明）：
    // 动作是单输入单输出，而日期计算天然要两个参数，所以把它们写在同一段文本里。
    {
        id: 'date.add',
        group: 'date',
        nameKey: 'actionDateAdd',
        icon: 'mdi-calendar-plus',
        direction: 'view',
        match: looksLikeDateAdd,
        run: (text, ctx) => dateAdd(text, ctx),
    },
    {
        id: 'date.diff',
        group: 'date',
        nameKey: 'actionDateDiff',
        icon: 'mdi-calendar-range',
        direction: 'view',
        match: looksLikeDateDiff,
        run: (text, ctx) => dateDiff(text, ctx),
    },

    // ── 生成（direction=insert：出现在输入框，不是预览区）────────────
    {
        id: 'generate.time',
        group: 'generate',
        nameKey: 'actionInsertTime',
        icon: 'mdi-clock-outline',
        direction: 'insert',
        run: () => nowTime(),
    },
    {
        id: 'generate.datetime',
        group: 'generate',
        nameKey: 'actionInsertDateTime',
        icon: 'mdi-calendar-clock',
        direction: 'insert',
        run: () => nowDateTime(),
    },
    {
        id: 'generate.uuid',
        group: 'generate',
        nameKey: 'actionInsertUuid',
        icon: 'mdi-identifier',
        direction: 'insert',
        run: () => newUuid(),
    },
];

/** 按 id 取一个动作。找不到返回 undefined（调用方自己兜底）。 */
export function findAction(id) {
    return ACTIONS.find((action) => action.id === id);
}

/** 某个分组下有哪些动作。面板按组渲染时用。 */
export function actionsInGroup(groupKey, direction = 'view') {
    return ACTIONS.filter((action) => action.group === groupKey && action.direction === direction);
}

/**
 * 这条内容「能用」哪些动作（match 命中的），按注册顺序。
 *
 * ⚠️ 这里**只调 match，不调 run** —— 动作轴每次渲染都要算它。
 * match 必须廉价，见文件头的约定。
 */
export function matchedActions(text, direction = 'view') {
    return ACTIONS.filter(
        (action) => action.direction === direction && (!action.match || action.match(text)),
    );
}

/**
 * **有针对性**（声明了 `match`）且命中的动作 —— 卡片右上角直接露图标的就是这几个。
 *
 * 和 `matchedActions` 的区别：后者含**通用动作**（没声明 match，任何文本都能跑）。
 * 通用动作（转大写、Base64 编码…）不该单独占图标位 —— 一条普通文本上挂一排图标，
 * 正是现有设计要避免的噪音；它们收进 `⋯` 面板里。
 */
export function targetedActions(text, direction = 'view') {
    return ACTIONS.filter(
        (action) => action.direction === direction && action.match && action.match(text),
    );
}

/**
 * 跑一条动作链，返回每一步的结果。
 *
 * ⚠️ **是 async 的**：`inspect.sha256` 走 `crypto.subtle.digest`，只有 Promise 形态。
 * 同步动作 `await` 一个非 Promise 值只多一次微任务，代价可忽略；
 * 但反过来（先写成同步、后来想加异步动作）就得改所有调用方 —— 所以一开始就按异步定。
 *
 * @param {string} text 原始正文
 * @param {string[]} ids 动作 id 序列
 * @param {{t?: Function}} [ctx] 给动作用的上下文（统计报告的标签要 i18n，不能硬编码中文）
 * @returns {Promise<{steps: Array, output: string, error: string}>}
 *
 * 语义：**某一步失败就停在那里**，返回前面成功的结果 + 那一步的错误。
 * 不继续往后跑 —— 链上后一步的输入依赖前一步的输出，硬着头皮跑下去得到的东西没有意义。
 */
export async function runChain(text, ids, ctx = {}) {
    const steps = [];
    let current = String(text ?? '');
    let error = '';

    for (const id of ids) {
        const action = findAction(id);
        if (!action) {
            error = `未知动作: ${id}`;
            break;
        }
        const input = current;
        try {
            const raw = await action.run(input, ctx);
            // 动作的返回值有**两种**形态：
            //   · 字符串          —— 纯文本结果（绝大多数动作）
            //   · `{ html, text }` —— **双表示**：html 用来渲染、text 用来复制。
            //     目前只有「注音制表」用它 —— 那个格式必须靠表格排版才能对齐，
            //     而复制时用户要的是能粘进表格软件的纯文本（见 pinyin.js 的说明）。
            let output;
            let html = '';
            if (raw && typeof raw === 'object' && typeof raw.html === 'string') {
                html = raw.html;
                output = String(raw.text ?? '');
            } else {
                output = String(raw ?? '');
            }
            steps.push({ id, action, input, output, html, error: '' });
            current = output;
        } catch (err) {
            const message = String(err?.message || err || '执行失败');
            steps.push({ id, action, input, output: '', error: message });
            error = message;
            break;
        }
    }

    return { steps, output: error ? '' : current, error };
}
