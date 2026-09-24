// 显示语义的契约检查（纯 Node，不需要浏览器、不需要起服务）。
//
//     cd web-vue3 && node scripts/check-display-semantics.mjs        # 查源码 + 已构建产物
//     cd web-vue3 && node scripts/check-display-semantics.mjs --src  # 只查源码（没构建过也能跑）
//
// 为什么需要它：这几条约定**跨文件、跨界面**，而且都没有编译期保护 ——
//   1. 那个开关的**名字**必须说它真管的东西（动作图标），不能还叫「Markdown 切换」；
//   2. 「动作图标」这条 labelKey 必须在**四份**语言文件里都存在（改名时最容易漏掉某一份，
//      漏了不会报错，只会在那个语言下显示成 blank / key 本身）；
//   3. 「默认看原文还是渲染视图」全站只有一份判断（util.js 的 prefersRenderedView），
//      标准 / 便签 / 聊天三处必须给出同一个答案 —— 尤其**普通 markdown 文本默认是原文**，
//      这条正是聊天模式从「气泡无条件渲染」改过来的地方，改回去会静默地改变行为。
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
    '三个模式都露面，且只有这三个（面板是「每模式一组开关」）',
    JSON.stringify(toggle?.modes) === JSON.stringify(['default', 'sticky', 'chat']),
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

// ── 3. 消费点没有各写一套默认值 ─────────────────────────────────────
const chat = fs.readFileSync(path.join(root, 'src/views/modes/ChatWall.vue'), 'utf8');
const md = fs.readFileSync(path.join(root, 'src/composables/useMarkdown.js'), 'utf8');
ok('ChatWall 复用 prefersRenderedView', /prefersRenderedView\(/.test(chat));
ok('ChatWall 不再无条件默认渲染（旧写法是 `|| \'md\'`）', !/\|\|\s*'md'/.test(chat), '找到 || \'md\'');
ok('useMarkdown 复用 prefersRenderedView', /prefersRenderedView\(/.test(md));
ok(
    '源码里没有第二份「任务列表 / 表格」判定组合',
    !/looksLikeTaskList\([^)]*\)\s*\|\|\s*looksLikeTable\(/.test(chat + md),
);
for (const name of ['useMarkdown.js', 'ChatWall.vue']) {
    const text = name.endsWith('.vue') ? chat : md;
    ok(`${name} 里没有残留的 mdModes/markdownToggle`, !/mdModes|markdownToggle/.test(text));
}

// ── 4. 已构建产物（有就查，没有就跳过）────────────────────────────────
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
