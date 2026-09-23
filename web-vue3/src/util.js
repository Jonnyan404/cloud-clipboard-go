import axios from 'axios';
import { marked } from 'marked';
import DOMPurify from 'dompurify';

export function prettyFileSize(size) {
    let units = ['TB', 'GB', 'MB', 'KB'];
    let unit = 'Bytes';
    while (size >= 1024 && units.length) {
        size /= 1024;
        unit = units.pop();
    };
    return `${Math.floor(100 * size) / 100} ${unit}`;
}

export function percentage(value, decimal = 2) {
    return (value * 100).toFixed(decimal) + '%';
}

export function formatTimestamp(timestamp) {
    if (!timestamp) return '';
    let date = new Date(timestamp * 1000);
    // 返回更详细的日期和时间格式，例如: YYYY-MM-DD HH:mm:ss
    return date.toLocaleString(undefined, { // 使用浏览器的默认 locale
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false // 使用 24 小时制
    });
};

/**
 * 构建不带房间密码的绝对 URL。
 * 分享/下载鉴权应使用服务端签发的短期 token（?t=），而不是 ?auth= 房间密码。
 */
export function buildCleanAbsoluteRouteUrl(path, prefix = '') {
    const normalizedPath = String(path || '').replace(/^\/+/, '');
    return new URL(`${prefix}/${normalizedPath}`, `${window.location.origin}/`).toString();
}

/**
 * 从地址里读一个 query 参数。**开局读一次**用（store 的初值），不订阅后续变化。
 *
 * 这个 app 是 **history 路由**，参数就在 search 里（`http://host/?mode=board`）——
 * 外部链接、书签、站内切换都是这一种写法。fragment 不参与（老 hash 地址已不兼容）。
 *
 * 为什么不用 `route.query`：store 的初值要在**第一次渲染之前**定下来，
 * 而 `router.isReady()` 是异步的 —— 那时候定不了，会先按旧值渲染一帧再跳，
 * 模式差异大的话就是一次可见的闪烁。
 */
export function readLocationParam(key) {
    const name = String(key || '');
    if (!name || typeof window === 'undefined') {
        return '';
    }
    return new URLSearchParams(window.location.search).get(name) || '';
}

/** 分享链接默认/约束（秒） */
export const SHARE_DEFAULT_TTL = 15 * 60; // 15 分钟
export const SHARE_MIN_TTL = 60; // 1 分钟
export const SHARE_MAX_TTL = 24 * 60 * 60; // 24 小时
export const SHARE_TTL_STEP = 60; // 滑块步进：1 分钟
export const SHARE_MAX_USES_LIMIT = 1000;

/** 分钟 <-> 秒，供 UI 滑块使用 */
export const SHARE_DEFAULT_TTL_MINUTES = Math.floor(SHARE_DEFAULT_TTL / 60);
export const SHARE_MIN_TTL_MINUTES = Math.floor(SHARE_MIN_TTL / 60);
export const SHARE_MAX_TTL_MINUTES = Math.floor(SHARE_MAX_TTL / 60);

export function normalizeShareTTL(ttl) {
    const value = Number(ttl);
    if (!Number.isFinite(value) || value <= 0) {
        return SHARE_DEFAULT_TTL;
    }
    if (value < SHARE_MIN_TTL) {
        return SHARE_MIN_TTL;
    }
    if (value > SHARE_MAX_TTL) {
        return SHARE_MAX_TTL;
    }
    return Math.floor(value);
}

export function minutesToShareTTL(minutes) {
    const mins = Number(minutes);
    if (!Number.isFinite(mins)) {
        return SHARE_DEFAULT_TTL;
    }
    return normalizeShareTTL(Math.round(mins) * 60);
}

export function shareTTLToMinutes(ttlSeconds) {
    return Math.max(
        SHARE_MIN_TTL_MINUTES,
        Math.min(SHARE_MAX_TTL_MINUTES, Math.round(normalizeShareTTL(ttlSeconds) / 60)),
    );
}

/**
 * 将秒数格式化为可读时长。
 * 需要传入 i18n t 函数：t(key, params)
 */
export function formatShareDuration(seconds, t) {
    const total = normalizeShareTTL(seconds);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    if (hours > 0 && minutes > 0) {
        return t('shareDurationHoursMinutes', { hours, minutes });
    }
    if (hours > 0) {
        return t('shareDurationHours', { hours });
    }
    return t('shareDurationMinutes', { minutes: Math.max(1, minutes) });
}

/** 0 = 不限次数 */
export function normalizeShareMaxUses(maxUses) {
    const value = Number(maxUses);
    if (!Number.isFinite(value) || value <= 0) {
        return 0;
    }
    if (value > SHARE_MAX_USES_LIMIT) {
        return SHARE_MAX_USES_LIMIT;
    }
    return Math.floor(value);
}

/**
 * 向服务端申请分享链接。
 *
 * 服务端**一律**签发 token（开放房间也发），返回的 url 就是分享地址
 * `https://host<prefix>/s/<token>` —— token 在**路径**里，服务端读得到，社交平台抓到的是
 * 一份注入了 OG 卡片的 HTML（见 lib/spa_shell.go）。房间是否需要鉴权不影响这里 ——
 * 以前开放房间走的是裸 `/content/<id>`，TTL / 次数限制全被静默丢弃。
 *
 * @param {{type:string,id?:string|number,uuid?:string,ttl?:number,maxUses?:number,password?:string,room?:string}} options
 */
export async function createShareLink({ type, id, uuid, ttl, maxUses, password, room } = {}) {
    const params = new URLSearchParams();
    if (room) {
        params.set('room', room);
    }

    const body = { type };
    if (id !== undefined && id !== null && id !== '') {
        body.id = String(id);
    }
    if (uuid) {
        body.uuid = uuid;
    }
    if (ttl !== undefined && ttl !== null && ttl !== '') {
        body.ttl = normalizeShareTTL(ttl);
    }
    if (maxUses !== undefined && maxUses !== null && maxUses !== '') {
        const uses = normalizeShareMaxUses(maxUses);
        if (uses > 0) {
            body.maxUses = uses;
        }
    }
    // 密码只进请求体，不进 URL（服务端把它 HMAC 进 token，URL 里连哈希都看不到）
    const pwd = String(password || '').trim();
    if (pwd) {
        body.password = pwd;
    }

    const response = await axios.post('share', body, { params });
    return response.data;
}

/**
 * 把服务端给的分享页地址**换成本浏览器自己的 origin**（保留它的路径与 `#` 片段）。
 *
 * 为什么必须换：服务端是用**请求的 Host** 拼这个地址的（见 buildSharePageURL）。
 * 而分享页是**前端路由**，它得落在前端所在的 origin 上 —— 只要中间有一层会改写 Host 的
 * 代理，服务端拼出来的主机就是错的：
 *   - dev：`vite.config.js` 的 proxy 写了 `changeOrigin: true`，于是 Host 变成后端
 *     （`localhost:9501`），链接指向一个**没有前端**的后端 → 点开白页；
 *   - 线上：任何 `proxy_set_header Host` 改写过的反代同理。
 *
 * 服务端那个 `url` 保留不动（脚本/第三方客户端仍然可以直接用），只是网页端不采用它的主机。
 * 前缀由服务端决定，这里只换 origin —— 所以带 prefix 部署时也不会丢。
 */
export function withCurrentOrigin(url) {
    const raw = String(url || '');
    if (!raw || typeof window === 'undefined') {
        return raw;
    }
    const m = raw.match(/^[a-z][a-z0-9+.-]*:\/\/[^/]+(\/.*)?$/i);
    return m ? window.location.origin + (m[1] || '/') : raw;
}

/**
 * 往**老 hash 分享地址**上补展示格式（f=md|raw）。返回的地址直接给收件人用。
 *
 * ⚠️ 老地址走 hash 路由，`?t=` 在 **fragment** 里 —— `new URL(u).searchParams` 看到的是空的，
 * 拿它去 set 会把参数拼到 `#` 前面，页面读不到。必须拆 fragment 再拼。
 * 新地址（`<prefix>/s/<token>`）没有 fragment，直接返回原值。
 *
 * ⚠️ 目前**没有调用方**：发送方预设展示格式那个设置已经删了（分享页自带 raw↔md 切换）。
 * 留着是为了将来真有「按链接预设格式」的需求时不用重新踩 fragment 这个坑。
 */
export function withSharePageFormat(url, format) {
    const raw = String(url || '');
    const hashIndex = raw.indexOf('#');
    if (!raw || hashIndex < 0) {
        return raw;
    }
    const value = String(format || '').toLowerCase() === 'md' ? 'md' : 'raw';
    const head = raw.slice(0, hashIndex);
    const fragment = raw.slice(hashIndex + 1);
    const queryIndex = fragment.indexOf('?');
    const routePath = queryIndex < 0 ? fragment : fragment.slice(0, queryIndex);
    const params = new URLSearchParams(queryIndex < 0 ? '' : fragment.slice(queryIndex + 1));
    params.set('f', value);
    return `${head}#${routePath}?${params.toString()}`;
}

/**
 * 往分享地址上补「这是扫码进来的」（q=1）—— 给二维码那个地址专用。
 *
 * 为什么要区分：扫码和点链接打开的是**同一个页面**，服务端分不出来，而「有多少人是扫过来的」
 * 正是二维码最想知道的事。带上这个参数，前端上报时就能告诉服务端一次。
 *
 * 分享地址是 `<prefix>/s/<token>`（**没有 `#`**）：token 在路径里，服务端读得到、OG 照旧，
 * 而 q 拼在普通 query 上，分享页用 `route.query.q` 读它。
 */
export function withShareQrFlag(url) {
    const raw = String(url || '');
    if (!raw) {
        return raw;
    }
    const queryIndex = raw.indexOf('?');
    const head = queryIndex < 0 ? raw : raw.slice(0, queryIndex);
    const params = new URLSearchParams(queryIndex < 0 ? '' : raw.slice(queryIndex + 1));
    params.set('q', '1');
    return `${head}?${params.toString()}`;
}

/**
 * 上报「分享页被真人打开了」一次。
 *
 * 相对路径（无前导斜杠）—— 与 createShareLink 同一约定，部署在子路径下不需要配置。
 *
 * 上报失败一律静默：它是统计，不是功能。分享页不该因为计数失败而报错，
 * 更不该因为服务端没有这条记录就把已打开的内容藏起来。
 *
 * @param {string} token 分享 token（就是分享页地址里的 t）
 * @param {{qr?:boolean}} options qr=true 表示这次是扫码进来的
 */
export async function reportShareVisit(token, { qr = false } = {}) {
    const value = String(token || '').trim();
    if (!value) {
        return null;
    }
    try {
        const response = await axios.post('share/visit', { token: value, qr: Boolean(qr) }, { __skipRoomAuthHandling: true });
        return response.data || null;
    } catch {
        return null;
    }
}

/**
 * 读某个房间的分享记录（最近分享过什么、被打开了几次）。
 *
 * 鉴权和「在该房间签发分享」完全一致：房间设了密码就必须带该房间的凭据。
 * 开放房间的这份列表是**公开可读**的（列表里不含 token，拿不到正文）—— 服务端注释里有论证。
 *
 * @param {{room?:string, limit?:number}} options
 */
export async function fetchShareRecords({ room = '', limit = 0 } = {}) {
    const params = {};
    if (room) {
        params.room = room;
    }
    if (limit > 0) {
        params.limit = limit;
    }
    const response = await axios.get('share/list', { params });
    return response.data || {};
}

export function copyTextToClipboard(textToCopy) {
    if (navigator.clipboard && window.isSecureContext) {
        return navigator.clipboard.writeText(textToCopy);
    }
    return new Promise((resolve, reject) => {
        try {
            const textArea = document.createElement('textarea');
            textArea.value = textToCopy;
            textArea.style.position = 'absolute';
            textArea.style.left = '-9999px';
            document.body.appendChild(textArea);
            textArea.select();
            const successful = document.execCommand('copy');
            document.body.removeChild(textArea);
            if (successful) {
                resolve();
            } else {
                reject(new Error('execCommand copy failed'));
            }
        } catch (err) {
            reject(err);
        }
    });
}

const CLIENT_ID_KEY = 'ccgDeviceId';

export function getClientId() {
    try {
        let id = localStorage.getItem(CLIENT_ID_KEY);
        if (!id) {
            id = (globalThis.crypto && typeof crypto.randomUUID === 'function')
                ? crypto.randomUUID()
                : `ccg-${Date.now()}-${Math.random().toString(16).slice(2)}`;
            localStorage.setItem(CLIENT_ID_KEY, id);
        }
        return id;
    } catch (err) {
        return '';
    }
}

// 设备显示名：客户端声明的 name 优先，没有才回落 UA 推断出的 os / type。
// 快捷指令、curl 这类 UA 认不出来的来源，就是靠 name 显示成可读的名字。
export function deviceLabel(senderDevice, fallback = '') {
    if (!senderDevice) {
        return fallback;
    }
    return senderDevice.name || senderDevice.os || senderDevice.type || fallback;
}

/**
 * 从 axios 错误里取出「给人看」的文案。
 *
 * 服务端统一返回 { code, error, message }：message 是中文人话，error 是英文人话。
 * 优先用 message，退回 error。老服务端或反向代理（Cloudflare 502 之类）返回的可能是
 * 纯文本甚至 HTML，那就截断后原样兜出来 —— 总比只显示一句泛化的「失败」强。
 *
 * 之前的写法是直接读 data.msg，而服务端从来不返回 msg 字段，所以这段逻辑一直是死的，
 * 前端永远只显示泛化提示。
 */
export function errorMessage(error) {
    const data = error?.response?.data;
    if (!data) return '';
    if (typeof data === 'string') return data.trim().slice(0, 200);
    return data.message || data.error || '';
}

/**
 * 内容像不像 markdown。
 *
 * 为什么要判断而不是无脑渲染：剪贴板里绝大多数是普通文本，而 markdown 的标记跟日常
 * 符号高度重合 —— `5 * 3 = 15` 会被渲染成斜体、`1. 打开设置` 会被当成有序列表。
 * 所以宁可漏判：漏判的代价是用户看到原文，误判的代价是内容被改形。
 */
// 解析一段 JSON。**只认对象和数组** —— 裸的 `123` / `"abc"` / `true` 也是合法 JSON，
// 但对它们做「美化」没有任何意义，却会让这类普通文本凭空多出一个图标。认不出来返回 undefined。
function parseJsonObject(text) {
    const s = String(text || '').trim();
    if (!s) {
        return undefined;
    }
    // 先按首字符挡一道：绝大多数普通文本到这就出去了，不用去试 JSON.parse。
    if (s[0] !== '{' && s[0] !== '[') {
        return undefined;
    }
    try {
        const value = JSON.parse(s);
        return value !== null && typeof value === 'object' ? value : undefined;
    } catch {
        return undefined;
    }
}

/** 内容是一段**能美化的 JSON**（对象或数组）。给「美化」图标当显示条件。 */
export function looksLikeJson(text) {
    const s = String(text || '');
    // 和 looksLikeMarkdown 同一个上限：几万字的条目算一次就够卡一下了。
    if (!s.trim() || s.length > 20000) {
        return false;
    }
    return parseJsonObject(s) !== undefined;
}

/**
 * 把 JSON 美化（两空格缩进）。
 *
 * **不是 JSON、或本来就已美化过 → 返回空串**：调用方据此决定要不要给这个入口 ——
 * 已经美化过的内容上再放一个「美化」按钮，点了没反应，比没有更差。
 */
export function formatJson(text) {
    const raw = String(text || '');
    const parsed = parseJsonObject(raw);
    if (parsed === undefined) {
        return '';
    }
    const pretty = JSON.stringify(parsed, null, 2);
    return pretty === raw.trim() ? '' : pretty;
}

/**
 * 把 JSON 压成一行。**不是 JSON、或本来就是一行 → 返回空串**（同 formatJson 的约定：
 * 点了没反应的按钮比没有更差）。
 */
export function minifyJson(text) {
    const raw = String(text || '');
    const parsed = parseJsonObject(raw);
    if (parsed === undefined) {
        return '';
    }
    const compact = JSON.stringify(parsed);
    return compact === raw.trim() ? '' : compact;
}

// 一眼就是代码的行首关键字。**故意列得宽**（Jonny 要求「不必太保守」）：
// 宁可把一段像代码的东西当代码 —— 那只是多一个图标，用户还能切回原文 / md；
// 而漏判的代价是「只能点 md，然后看着 markdown 把代码重排」。
//
// 刻意不收 `from` / `use` / `type` / `new` 这类**英语里也常见**的词：
// 它们做行首在散文里太容易撞上，而它们所在的语言（Python / Rust / TS）另有
// `import` / `def` / `impl` / `fn` 这些更明确的信号。
const CODE_HINT_RE = /(^|\n)\s*(package|import|export|require|module|func|fn|def|class|struct|interface|enum|trait|impl|namespace|public|private|protected|static|final|void|return|const|let|var|val|async|await|throw|except|elif|lambda|#include|#!|SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|BEGIN|COMMIT|printf|println|console|echo|puts)\b/;

// 代码的**形状**，不依赖关键字：`;` `{}` 收尾、`foo(...)` 调用、箭头 / 管道 / 泛型、
// 标签、模板插值、`%s` 这类格式符。
const CODE_SHAPE_RE = /([;{}]\s*$|\w\s*\([^)]*\)\s*[;{]|=>|->|::|<\/?[a-z][\w-]*>|\$\{[^}]*\}|%[sdvf]\b)/;

/**
 * 内容像**一段源码**吗（决定要不要给「代码」视图那个图标）。
 *
 * 判定顺序（从便宜到贵）：
 *   1. 已经带 ``` 围栏的**不算** —— 那本来就是 markdown，围栏里的代码会被 MarkdownBody 高亮；
 *   2. 是合法 JSON 的**不算** —— JSON 有自己的「美化 / 压缩」两个视图，别抢；
 *   3. 行首命中关键字 → 是；
 *   4. **单行**也能是代码：看形状（`const a = 1;` / `foo(1, 2)`）；
 *   5. 多行：看「像代码的行」占比，门槛 1/4（一段代码里常夹空行和注释）。
 */
export function looksLikeCode(text) {
    const s = String(text || '');
    if (!s.trim() || s.length > 20000) {
        return false;
    }
    if (/^\s*```|[\r\n]\s*```/.test(s)) {
        return false;
    }
    if (parseJsonObject(s) !== undefined) {
        return false;
    }
    if (CODE_HINT_RE.test(s)) {
        return true;
    }
    const lines = s.split('\n').filter((line) => line.trim());
    if (lines.length < 2) {
        const one = s.trim();
        // 单行也常是代码：`foo(1, 2)` / `const a = 1;` / `x => x + 1`。
        // 末一条**要求整行就是一个调用**（`标识符(...)`），否则散文里的「见附录 (a)」
        // 也会被算进去 —— 中文不算 `\w`，所以那条天然挡得住。
        return CODE_SHAPE_RE.test(one) || /^[A-Za-z_$][\w.$]*\s*\([^)]*\)\s*[;{]?$/.test(one);
    }
    const codeLines = lines.filter((line) => CODE_LINE_RE.test(line) || CODE_SHAPE_RE.test(line)).length;
    return codeLines >= Math.max(2, Math.ceil(lines.length / 4));
}

// 没有关键字时看行首的代码结构：if/for/while 开头，或 `{` `}` `;` 收尾。
const CODE_LINE_RE = /([{};]\s*$|^\s*(if|for|while|else|try|catch|switch|case|do)\b)/;

export function looksLikeMarkdown(text) {
    const s = String(text || '');
    // 太长不渲染：一个几万字的条目渲染一次就够列表卡一下了
    if (!s.trim() || s.length > 20000) return false;
    return /(^|\n)\s{0,3}(#{1,6}\s|>\s|[-*+]\s|\d+\.\s|```)/.test(s)
        || /\[[^\]]+\]\([^)\s]+\)/.test(s)                    // [文字](链接)
        || /\*\*[^\s][^*]*\*\*|__[^\s][^_]*__/.test(s)         // 粗体
        || /`[^`\n]+`/.test(s)                                  // 行内代码
        || looksLikeTable(s);                                   // 表格：上面几条都认不出来
}

/**
 * 内容里有 GFM 任务列表（`- [ ] xxx` / `- [x] xxx`）。
 *
 * 有序变体（`1. [ ]`）也算 —— GFM 允许，渲染出来同样是复选框。
 * 只看行首标记，不看缩进层级：嵌套任务列表的每一行都以 `- [ ]` 开头，自然命中。
 */
export function looksLikeTaskList(text) {
    return /(^|\n)\s{0,3}([-*+]|\d+\.)\s+\[[ xX]\](\s|$)/.test(String(text || ''));
}

/**
 * 内容里有 GFM 表格。
 *
 * 判据是「表头行 + 紧跟一行分隔线」，不是「有竖线」：随手打的 `a | b` 到处都是
 * （shell 管道、位运算），拿竖线当判据会误判一大片。分隔线（`|---|---|`）才是表格的签名。
 */
export function looksLikeTable(text) {
    const lines = String(text || '').split('\n');
    for (let i = 0; i + 1 < lines.length; i++) {
        if (!lines[i].includes('|')) continue;
        if (/^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)+\|?\s*$/.test(lines[i + 1])) {
            return true;
        }
    }
    return false;
}

/**
 * 把 markdown 渲染成可以安全插进 DOM 的 HTML。
 *
 * **必须清洗**：内容可能是别人发过来的，`<img src=x onerror=...>` 这类注入是真实风险
 * —— 剪贴板本身就是个「别人能往你这里塞字符串」的通道。DOMPurify 默认配置会去掉
 * script、事件属性、javascript: 这类 URL。
 *
 * @param {object} [opts]
 * @param {boolean} [opts.interactiveTasks] 任务列表的复选框可点。默认 false（清洗后 `disabled`
 *        会被去掉但没人处理点击，看着能点其实没反应）—— 只有真的会接住点击的调用点才开。
 */
let allowCheckboxInteraction = false;
let checkboxHookInstalled = false;
function ensureCheckboxHook() {
    if (checkboxHookInstalled) return;
    DOMPurify.addHook('afterSanitizeAttributes', (node) => {
        if (!allowCheckboxInteraction) return;
        if (node.tagName === 'INPUT' && node.getAttribute('type') === 'checkbox') {
            // marked 给任务列表的复选框加 `disabled`；要能点就得摘掉。
            // 只在交互开关打开时摘 —— 否则聊天气泡里的复选框也能点，而那里没人接住点击。
            node.removeAttribute('disabled');
        }
    });
    checkboxHookInstalled = true;
}

export function renderMarkdownHtml(text, opts = {}) {
    ensureCheckboxHook();
    const html = marked.parse(String(text || ''), { breaks: true, gfm: true });
    allowCheckboxInteraction = Boolean(opts.interactiveTasks);
    try {
        return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } });
    } finally {
        allowCheckboxInteraction = false;
    }
}

/**
 * 翻转第 `index` 个任务列表项（`- [ ]` ↔ `- [x]`），返回新文本。
 *
 * 按**渲染顺序**数，和页面上复选框的顺序一一对应 —— 调用方传的是「第几个复选框被点了」。
 * 有序变体（`1. [ ]`）一起认，和 looksLikeTaskList 同一套标记。
 * 越界就原样返回，不抛错。
 */
export function toggleTaskListItem(text, index) {
    let seen = 0;
    return String(text || '').replace(
        /(^|\n)(\s{0,3}(?:[-*+]|\d+\.)\s+\[)([ xX])(\])/g,
        (match, lead, prefix, mark, close) => {
            if (seen++ !== index) return match;
            return lead + prefix + (mark === ' ' ? 'x' : ' ') + close;
        },
    );
}

// 文件名是不是图片。**全站唯一实现** —— 之前这段正则在 6 个地方各抄了一份
// （StickyNote 两处、ChatWall / MegaWall / TerminalWall / WorkbenchWall 各一处），
// 再加一份就是第 7 份，改一处必漏其余。判型只看扩展名，不看内容：
// 服务端不嗅探、客户端也不该嗅探，两边同一套标准。
const IMAGE_NAME_RE = /\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i;
export function isImageName(name) {
    return IMAGE_NAME_RE.test(String(name || ''));
}

// 文件条目「能不能就地预览、该按哪一类渲染」—— **全站唯一实现**。
//
// 为什么收进这里：这三条正则（视频 / 音频 / 文本类文件）本来在 7 个文件里各抄了一份
// （received-item/File、sticky/StickyNote、glance/GlancePreview、ChatWall / MegaWall /
// TerminalWall / WorkbenchWall），而且**已经漂移**：那几个模式墙「挑渲染分支」的 helper 认
// `.mov`，而它们自己的 `isPreviewableVideo` 不认 —— 同一个文件在弹窗里是视频播放器、
// 在卡片上却退回图标。同一份 `isImageName` 当年也是抄了 6 份才收进来的，别再抄第 8 份。
//
// 判型只看扩展名，不看内容：服务端不嗅探、客户端也不该嗅探（同 isImageName）。
//
// 返回 `'image' | 'video' | 'audio' | 'text'`，不预览时返回 `''`（调用方退回图标）。
// ⚠️ 图片走 `isImageName`（含 svg / ico / avif），别在这里再列一遍图片后缀。
const VIDEO_NAME_RE = /\.(mp4|webm|ogv|mov)$/i;
const AUDIO_NAME_RE = /\.(mp3|wav|ogg|opus|m4a|flac)$/i;
const TEXT_NAME_RE = /\.(txt|text|md|markdown|json|log|csv|tsv|ya?ml|xml|ini|conf|cfg|toml|properties|env|gitignore|dockerfile|js|jsx|mjs|cjs|ts|tsx|vue|css|scss|sass|less|html|htm|sql|sh|bash|zsh|fish|ps1|bat|cmd|go|py|java|kt|kts|rb|php|rs|c|cc|cpp|cxx|h|hh|hpp|hxx|swift|proto)$/i;

export function filePreviewKind(name) {
    const value = String(name || '');
    if (!value) {
        return '';
    }
    if (isImageName(value)) {
        return 'image';
    }
    if (VIDEO_NAME_RE.test(value)) {
        return 'video';
    }
    if (AUDIO_NAME_RE.test(value)) {
        return 'audio';
    }
    if (TEXT_NAME_RE.test(value)) {
        return 'text';
    }
    return '';
}

/**
 * 把一条内容挪到看板的某一列（`POST /content/<id>/column`）。
 *
 * 两个后端都有这条接口（Go `handleContentColumn` / Worker `ContentHandler.setColumn`）：
 * **固定三列**（todo / doing / done）、卡片就是剪贴板条目本身、不建新表，
 * 而且**不动 timestamp** —— 挪个位置不该让卡片在时间流里跳到最前面。
 */
export async function updateEntryColumn(id, room, column) {
    const response = await axios.post(
        `content/${encodeURIComponent(id)}/column`,
        { column },
        { params: new URLSearchParams([['room', room ?? '']]) },
    );
    return response.data;
}

/**
 * 把 HTML 实体还原成文本。
 *
 * 服务端存的是实体编码过的正文（`<` 之类），卡片里要显示原文就得先解回来。
 * **全站唯一实现** —— 之前 Text.vue / File.vue / StickyNote 各写了一份（第 4 份正在路上）。
 */
export function decodeHtmlEntities(text) {
    const el = document.createElement('textarea');
    el.innerHTML = String(text ?? '');
    return el.value;
}

/**
 * 覆盖一条**已有**文本条目的正文（`POST /text?id=<id>`）。
 *
 * 这条接口两个后端**早就有**（Go `updateTextMessage` / Worker `Text.update`）：落盘、
 * 广播 `update` 事件、id 不变。前端一直没人用 —— 任务列表打勾是第一个用它的地方，
 * 所以持久化不需要动服务端。
 *
 * ⚠️ 服务端会把时间戳更新成「现在」（最近改过的算最新），但前端收到 `update` 是**原地替换**
 * （见 store/websocket.js 的 case 'update'），所以卡片不会在眼皮底下跳走；
 * 刷新之后它会出现在最前面。
 */
export async function updateTextEntry(id, room, content) {
    await axios.post('text', String(content ?? ''), {
        params: new URLSearchParams([['room', room ?? ''], ['id', String(id)]]),
        headers: { 'Content-Type': 'text/plain' },
    });
}
