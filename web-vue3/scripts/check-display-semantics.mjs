// 显示语义的契约检查（纯 Node，不需要浏览器、不需要起服务）。
//
//     cd web-vue3 && node scripts/check-display-semantics.mjs        # 查源码 + 已构建产物
//     cd web-vue3 && node scripts/check-display-semantics.mjs --src  # 只查源码（没构建过也能跑）
//
// 为什么需要它：这几条约定**跨文件、跨界面**，而且都没有编译期保护 ——
//   1. 那个开关的**名字**必须说它真管的东西（动作图标），不能还叫「Markdown 切换」；
//   2. 「动作图标」这条 labelKey 必须在**四份**语言文件里都存在（改名时最容易漏掉某一份，
//      漏了不会报错，只会在那个语言下显示成 blank / key 本身）；
//   2b. 四份语言文件的**键集合**必须完全一致 —— 少一条那门语言下会印出键本身，多一条是死键；
//   3. 「默认看原文还是渲染视图」全站只有一份判断（util.js 的 prefersRenderedView），
//      标准 / 便签两个模式必须给出同一个答案 —— 尤其**普通 markdown 文本默认是原文**，
//      这条正是聊天模式从「气泡无条件渲染」改过来的地方，改回去会静默地改变行为；
//   4. 输入框粘文件时必须**拦下浏览器的默认粘贴行为**，而且只在该拦的时候拦：
//      漏了它 → 文件名会落进输入框；无条件拦 → 纯文本粘贴被吞掉。两个输入组件各有一份；
//   5. 快捷指令更新时间轴的日期清单是**人工维护**的（`shortcuts/history.json`）——
//      顺序写反 / 写重复 / 平台名打错都不会报错，只会让时间轴显示得莫名其妙。
//
// ⚠️ 这个脚本要能在**纯 Node** 里跑：它直接 import `src/util.js`（没有 vite，也就没有
// `@` 别名）—— `util.js` 里那句 import 因此用相对路径。别名一回来，整个脚本就
// ERR_MODULE_NOT_FOUND（判据一条都跑不到，比没有更糟）。
//
// 断言的是**语义**，不是实现：prefersRenderedView 里换一种写法、把几条正则合并，
// 只要答案不变，这个脚本就该继续绿。

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

const { prefersRenderedView, looksLikeMarkdown } = await import(
    new URL('../src/util.js', import.meta.url).href
);
const { DISPLAY_TOGGLES, DEFAULT_DISPLAY } = await import(
    new URL('../src/data/displayToggles.js', import.meta.url).href
);

let failed = 0;
function ok(name, condition, detail = '') {
    console.log(`${condition ? 'ok  ' : 'FAIL'}  ${name}${detail && !condition ? `  → ${detail}` : ''}`);
    if (!condition) failed += 1;
}

// ── 1. 默认视图：只有「结构化内容」默认渲染 ────────────────────────────
const CASES = [
    // [内容, 默认该渲染吗, 说明]
    ['- [ ] 买牛奶\n- [x] 写周报', true, '任务列表'],
    ['1. [ ] 有序任务列表', true, '有序任务列表'],
    ['| a | b |\n| --- | --- |\n| 1 | 2 |', true, 'GFM 表格'],
    ['# 标题', false, '普通 markdown 默认原文'],
    ['**加粗** 和 `inline code`', false, '行内标记默认原文'],
    ['- 普通列表项', false, '普通无序列表默认原文'],
    ['5 * 3 = 15', false, '普通文本不能被误判成 markdown'],
    ['这是一句普通的话', false, '中文普通文本'],
    ['见 https://example.com/a_b_c', false, '裸链接'],
];
for (const [text, expected, label] of CASES) {
    const got = prefersRenderedView(text);
    ok(`prefersRenderedView: ${label} → ${expected ? '渲染' : '原文'}`, got === expected, JSON.stringify(text.slice(0, 30)));
}

// 默认渲染的那几种，必须**同时**过 looksLikeMarkdown —— 否则图标不出现，
// 那条「默认渲染」根本走不到（渲染路径上还有一道 looksLikeMarkdown 的闸）。
for (const [text, expected, label] of CASES) {
    if (!expected) continue;
    ok(`默认渲染的内容也拿得到图标: ${label}`, looksLikeMarkdown(text), JSON.stringify(text.slice(0, 30)));
}

// ── 2. 动作图标开关：名字与语言文件 ──────────────────────────────────
const toggle = DISPLAY_TOGGLES.find((item) => item.key === 'markdown');
ok('存储键仍是 markdown（改名要迁移用户已存配置）', Boolean(toggle));
ok('labelKey 已改成 actionIcons', toggle?.labelKey === 'actionIcons', String(toggle?.labelKey));
ok('默认开着（关了等于升级后功能消失）', DEFAULT_DISPLAY.markdown === true);
ok(
    '两个模式都露面，且只有这两个（面板是「每模式一组开关」）',
    JSON.stringify(toggle?.modes) === JSON.stringify(['default', 'sticky']),
    JSON.stringify(toggle?.modes),
);

const LOCALES = ['zh', 'zh-TW', 'en', 'ja'];
for (const locale of LOCALES) {
    const file = path.join(root, 'src/locales', `${locale}.json`);
    const dict = JSON.parse(fs.readFileSync(file, 'utf8'));
    const label = dict[toggle.labelKey];
    ok(`locale ${locale}: 有 ${toggle.labelKey} 且非空`, typeof label === 'string' && label.trim().length > 0);
    ok(`locale ${locale}: 已不残留旧的 markdownToggle`, !('markdownToggle' in dict));
}
// 中文那份额外钉一下措辞：Jonny 定的就是「动作图标」这四个字。
const zh = JSON.parse(fs.readFileSync(path.join(root, 'src/locales/zh.json'), 'utf8'));
ok('zh: 措辞就是「动作图标」', zh[toggle.labelKey] === '动作图标', zh[toggle.labelKey]);

// 2b. 四份语言文件的**键集合**必须完全一致。
//
// 「i18n key 是契约」这条约定原本只被 `tools/desktop-ui-smoke.mjs` 的判据 14 守着，
// 而它只管**桌面外壳**那本字典（`i18n.js` 的 DICTS）—— SPA 这四份一直没人看着。
// 少一条的症状是那门语言下把键本身印在界面上（`scUpdateHistory`），多一条是死键。
// 增删键时最容易漏改其中一份，所以钉在集合上，而不是「有没有某个键」。
const allDicts = {};
for (const locale of LOCALES) {
    allDicts[locale] = JSON.parse(fs.readFileSync(path.join(root, 'src/locales', `${locale}.json`), 'utf8'));
}
for (const locale of LOCALES.filter((name) => name !== 'zh')) {
    const keys = new Set(Object.keys(allDicts[locale]));
    const missing = Object.keys(allDicts.zh).filter((key) => !keys.has(key));
    const extra = Object.keys(allDicts[locale]).filter((key) => !(key in allDicts.zh));
    ok(
        `locale ${locale}: 键集合与 zh 一致`,
        missing.length === 0 && extra.length === 0,
        `${missing.length ? `缺 ${JSON.stringify(missing.slice(0, 8))}` : ''}${extra.length ? ` 多 ${JSON.stringify(extra.slice(0, 8))}` : ''}`,
    );
}

// ── 3. 消费点没有各写一套默认值 ─────────────────────────────────────
//
// ⚠️ 这一节原来还看着 `src/views/modes/ChatWall.vue`，但聊天模式早已不在模式列表里
// （`src/views/modes/` 下没有这个文件）—— 于是 `readFileSync` 当场 ENOENT，
// **整节崩在这里，后面的判据一条都跑不到**。看起来像「脚本坏了」，实际是判据过时了。
// 判据跟着代码走：那两条一并删掉。
//
// ⚠️ 别顺手把「没有第二份判定组合」这条扩到全仓去：`util.js` 里 `prefersRenderedView`
// 的**实现本身**就是 `looksLikeTaskList || looksLikeTable`，`data/actions.js` 里那个
// 「要不要给这个动作图标」的 match 也长得一样 —— 两者都不是「又抄了一份默认视图判断」。
// 扩到全仓只会得到一条**永远红**的判据（实测如此），所以这里只钉**消费点**。
const walkSources = (dir) => {
    const out = [];
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) out.push(...walkSources(full));
        else if (/\.(js|vue)$/.test(entry.name)) out.push(full);
    }
    return out;
};
const sourceFiles = walkSources(path.join(root, 'src'));
const md = fs.readFileSync(path.join(root, 'src/composables/useMarkdown.js'), 'utf8');
ok('useMarkdown 复用 prefersRenderedView', /prefersRenderedView\(/.test(md));
ok(
    'useMarkdown 里没有自己拼一套「任务列表 / 表格」判定',
    !/looksLikeTaskList\([^)]*\)\s*\|\|\s*looksLikeTable/.test(md),
    '除了 prefersRenderedView，别处又拼了一套',
);
// 这条可以放心扫全仓：`mdModes` / `markdownToggle` 是**退役的键名**，哪儿都不该再出现。
const joined = sourceFiles.map((file) => fs.readFileSync(file, 'utf8')).join('\n');
ok('全仓已无残留的 mdModes / markdownToggle', !/mdModes|markdownToggle/.test(joined));

// ── 4. 粘贴文件：必须拦下浏览器的默认行为 ─────────────────────────────
//
// 「在主输入框里粘一张截图 → 输入框里多出一行文件名」这个 bug 就是漏了
// `event.preventDefault()` 造成的：剪贴板里带文件时（复制的文件、截图），浏览器默认
// 会把**文件名**当文本插进光标处，而且那行字还会被当成正文发出去。
//
// 但这个修法有**两个方向**都会坏，而两个方向都没有编译期保护：
//   · 漏了 preventDefault   → 文件名照样落进输入框（就是用户报的那个 bug）；
//   · 无条件 preventDefault → 纯文本粘贴被吞掉（「按了粘贴没反应」）。
// 所以判的不是「有没有这一句」，而是**顺序**：拦的动作要落在「有文件」的判断之后、
// 收文件之前。两个输入组件各有一份 handlePaste（主输入框 / 便签），两份都要过。
const functionBody = (source, name) => {
    const at = source.indexOf(`function ${name}(`);
    if (at < 0) return null;
    const open = source.indexOf('{', source.indexOf(')', at));
    if (open < 0) return null;
    let depth = 0;
    for (let i = open; i < source.length; i += 1) {
        if (source[i] === '{') depth += 1;
        else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i);
    }
    return null;
};

const COMPOSERS = [
    { label: '主输入框', file: 'src/components/UnifiedComposer.vue' },
    { label: '便签输入框', file: 'src/components/sticky/StickyComposer.vue' },
];
for (const { label, file } of COMPOSERS) {
    const body = functionBody(fs.readFileSync(path.join(root, file), 'utf8'), 'handlePaste');
    if (body === null) {
        ok(`${label} 里找得到 handlePaste`, false, `${file} 里没有 function handlePaste —— 这条判据要跟着代码改`);
        continue;
    }
    const lines = body.split('\n');
    const guard = lines.findIndex((line) => /files\.length/.test(line));
    const prevent = lines.findIndex((line) => /event\.preventDefault\(\)/.test(line));
    const call = lines.findIndex((line) => /handleSelectFiles\(/.test(line));
    ok(
        `${label}：有文件时拦下默认粘贴（否则文件名会落进输入框）`,
        prevent >= 0 && prevent > guard && call > prevent,
        prevent < 0
            ? 'handlePaste 里没有 event.preventDefault()'
            : `顺序不对（判断在第 ${guard} 行 / 拦在第 ${prevent} 行 / 收文件在第 ${call} 行）—— 拦在判断之前会把纯文本粘贴也吞掉`,
    );
}

// ── 5. 快捷指令的更新时间轴 ────────────────────────────────────────────
//
// 日期清单是**人工维护**的（`shortcuts/history.json`），不是 git 日期：本仓库的
// `shortcuts/` 是 2026-09-28 整批导入的，`git log` 只会给出搬家那天（详见
// `scripts/sync-shortcuts.mjs`）。人工清单最容易出的三种错都不会报错，只会让时间轴
// 显示得莫名其妙，所以逐条钉住：
//   · 写反顺序  → 「最近一次更新」显示成最早那次，用户据此以为自己的捷径是新的；
//   · 写重复    → 轴上多一个假节点；
//   · 平台名打错 → 那个平台的时间轴整块空掉（没有报错）。
const historyPath = path.join(root, '..', 'shortcuts', 'history.json');
let history = null;
try {
    history = JSON.parse(fs.readFileSync(historyPath, 'utf8'));
} catch {
    history = null;
}
ok('shortcuts/history.json 存在且能解析', history !== null && typeof history === 'object');

// 字段名必须和 `meta.json` / `ShortcutsDialog.vue` 用的那两个一致。
const PLATFORMS = ['apple', 'android'];
if (history) {
    for (const key of Object.keys(history)) {
        ok(`history.json 里没有多余的平台键：${key}`, PLATFORMS.includes(key), '打错字不会报错，那个平台的时间轴会整块空掉');
    }
    for (const platform of PLATFORMS) {
        const dates = history[platform];
        ok(`${platform}: 是日期数组`, Array.isArray(dates), `${typeof dates} —— 弹窗会 Array.isArray 挡掉，时间轴不显示`);
        if (!Array.isArray(dates) || dates.length === 0) continue;
        ok(`${platform}: 全是 YYYY-MM-DD`, dates.every((d) => /^\d{4}-\d{2}-\d{2}$/.test(d)), JSON.stringify(dates));
        ok(`${platform}: 没有重复`, new Set(dates).size === dates.length, JSON.stringify(dates));
        const sorted = [...dates].sort().reverse();
        ok(`${platform}: 最新在前`, JSON.stringify(dates) === JSON.stringify(sorted), `应该是 ${JSON.stringify(sorted)}`);
    }
    for (const platform of PLATFORMS) {
        ok(`${platform} 有更新记录（时间轴要有内容可画）`, (history[platform] ?? []).length > 0);
    }
}

// 弹窗那一侧：确实在画时间轴，而且旧的那条警告已经拆干净（拆一半会同时看到两套说法）。
const dialog = fs.readFileSync(path.join(root, 'src/components/ShortcutsDialog.vue'), 'utf8');
ok('弹窗用 scUpdateHistory 当时间轴标题', dialog.includes("t('scUpdateHistory')"));
ok('弹窗按当前 tab 取更新记录', /activeHistory/.test(dialog));
ok('旧的「旧版请重新导入」提示已拆掉', !dialog.includes('scOutdatedNotice'));
ok('旧的「更新于 {date}」已拆掉', !dialog.includes('scUpdatedAt'));
for (const locale of LOCALES) {
    const dict = allDicts[locale];
    ok(`locale ${locale}: 有 scUpdateHistory 且非空`, typeof dict.scUpdateHistory === 'string' && dict.scUpdateHistory.trim().length > 0);
    ok(`locale ${locale}: 已不残留 scOutdatedNotice / scUpdatedAt`, !('scOutdatedNotice' in dict) && !('scUpdatedAt' in dict));
}

// ── 6. 已构建产物（有就查，没有就跳过）────────────────────────────────
if (!process.argv.includes('--src')) {
    const assetsDir = path.join(root, 'dist/assets');
    const bundle = fs.existsSync(assetsDir)
        ? fs.readdirSync(assetsDir).filter((f) => f.endsWith('.js'))
            .map((f) => fs.readFileSync(path.join(assetsDir, f), 'utf8')).join('\n')
        : '';
    if (!bundle) {
        console.log('skip  dist/assets 不存在或没有 JS（没构建过；先跑 npm run build）');
    } else {
        // 压缩器可能把非 ASCII 转成 \uXXXX，两种形态都认。
        const hasLabel = bundle.includes('动作图标') || bundle.includes('\\u52a8\\u4f5c\\u56fe\\u6807');
        ok('产物里有新标签「动作图标」', hasLabel);
        ok('产物里不再有 markdownToggle 这个 key', !bundle.includes('markdownToggle'));
    }
}

console.log(failed ? `\n${failed} 条失败` : '\n全部通过');
process.exit(failed ? 1 : 0);
