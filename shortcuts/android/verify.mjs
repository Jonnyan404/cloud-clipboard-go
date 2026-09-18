// 自检：把 shortcuts.json 里那段 codeOnSuccess 真跑一遍，看它拼出的下载 URL。
//
// 为什么要跑而不是看：这段 JS 是**字符串**存进 JSON 的，肉眼改对了、转义错了，
// 导入到手机上才发现，代价很大。这里用 node:vm 起个沙箱、把 HTTP Shortcuts 的
// 宿主函数打桩，断言最终 URL。
//
//   node verify.mjs <shortcuts.json>
//
// 同一条 JS 在 JSON 里存了两份（「接收最新」和「接收指定ID」各一份），本脚本顺带断言
// 两份逐字相同 —— 只在应用里改了其中一条，是这里最容易犯的错，而且症状很隐蔽
// （另一条照样能跑，只是行为不同）。
import vm from 'node:vm';
import fs from 'node:fs';

const jsonPath = process.argv[2];
if (!jsonPath) {
  console.error('用法: node verify.mjs <shortcuts.json>');
  process.exit(2);
}

const data = JSON.parse(fs.readFileSync(jsonPath, 'utf8'));
const codes = data.categories
  .flatMap(cat => cat.shortcuts)
  .filter(sc => sc.codeOnSuccess && sc.codeOnSuccess.includes('downloadUrl'))
  .map(sc => ({ name: sc.name, code: sc.codeOnSuccess }));

if (codes.length === 0) {
  console.error('!! 没有找到带 downloadUrl 的 codeOnSuccess');
  process.exit(1);
}

// 两份必须逐字相同。只在应用里改了其中一条，另一条照样能跑，只是行为不同 ——
// 这种分叉不会报错，只会让「接收最新」和「接收指定ID」悄悄不一样。
const variants = new Set(codes.map(c => c.code));
if (variants.size !== 1) {
  console.error(`!! 带下载逻辑的 ${codes.length} 处 codeOnSuccess 不一致（${variants.size} 个版本）`);
  for (const c of codes) console.error(`     ${c.name}: ${c.code.length} 字符`);
  console.error('   → 多半是只在应用里改了其中一条捷径。两条都要改。');
  process.exit(1);
}

const NAME = '图片 1.png';   // 非 ASCII + 空格，顺带验编码
const UUID = 'U-1';
const BASE = `http://host:9501/file/${UUID}/${encodeURIComponent(NAME)}`;

// 跑一次，返回那次调用丢给「展示文件」的 downloadUrl
function run(code, auth) {
  let captured = null;
  const vars = { url: 'http://host:9501', auth };
  const ctx = vm.createContext({
    response: { body: JSON.stringify({ type: 'image', name: NAME, size: 12, uuid: UUID }) },
    getVariable: k => (k in vars ? vars[k] : ''),
    copyToClipboard: () => {},
    showToast: () => {},
    confirm: () => false,
    openUrl: () => {},
    enqueueShortcut: (id, para) => { captured = para; },
  });
  vm.runInContext(code, ctx, { filename: 'codeOnSuccess.js' });
  return captured && captured.downloadUrl;
}

const failures = [];
for (const { name, code } of codes) {
  const cases = [
    ['设了密码', '456', `${BASE}?auth=456`],
    ['没设密码', '', BASE],
    // 密码里带 & 不编码就会把查询串截断成 ?auth=p，服务端拿到错密码
    ['密码含特殊字符', 'p w+d&x', `${BASE}?auth=${encodeURIComponent('p w+d&x')}`],
  ];
  for (const [label, auth, want] of cases) {
    const got = run(code, auth);
    if (got !== want) failures.push(`[${name}] ${label}\n      实际: ${got}\n      期望: ${want}`);
  }
}

if (failures.length) {
  console.error('!! 下载 URL 自检失败：');
  for (const f of failures) console.error('  ✗ ' + f);
  process.exit(1);
}

console.log(`✓ 下载 URL 自检通过（${codes.length} 处 × 3 种凭据场景）`);
console.log(`    带密码 → ${run(codes[0].code, '456')}`);
