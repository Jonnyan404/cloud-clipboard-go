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

// ── 请求形状自检：断言**导出里真实写着的 URL** ──
//
// 为什么要有这一段：跨后端那两个契约测试
//   cloud-clip/lib/shortcut_contract_test.go
//   cloudflare/workers/test/shortcut-contract.test.mjs
// 把请求形状**抄成了常量**。抄的那份会漂 —— 在应用里改了捷径的 URL，那两个测试照样绿，
// 因为它们测的是自己抄的那份，跟真实捷径没关系。
// 这个脚本本来就必须读 shortcuts.json，所以「抄的和真的对不对得上」放在这里最省事。
//
// URL 里的变量是 {{uuid}} 形式，先按 variables 映射回 key 再比对，这样期望值可读。
const EXPECTED_URLS = {
  '发送文本':   '{url}/text?room={room}&auth={auth}&name={name}',
  '发送文件':   '{url}/upload?room={room}&auth={auth}&name={name}',
  '接收最新':   '{url}/content/latest?room={room}&auth={auth}&format=json',
  '接收指定ID': '{url}/content/{ID}?room={room}&format=json&auth={auth}',
  '展示文件':   '{downloadUrl}',
};

const idToKey = new Map((data.variables || []).map(v => [v.id, v.key]));
const allShortcuts = data.categories.flatMap(cat => cat.shortcuts);
const shapeFailures = [];

for (const s of allShortcuts) {
  const want = EXPECTED_URLS[s.name];
  if (!want) {
    shapeFailures.push(`出现了未登记的捷径「${s.name}」—— 新增捷径要在这里补一行期望形状`);
    continue;
  }
  const got = String(s.url || '').replace(/\{\{([^}]+)\}\}/g,
    (m, id) => (idToKey.has(id) ? `{${idToKey.get(id)}}` : m));
  if (got !== want) {
    shapeFailures.push(`[${s.name}]\n      实际: ${got}\n      期望: ${want}`);
  }
}

for (const name of Object.keys(EXPECTED_URLS)) {
  if (!allShortcuts.some(s => s.name === name)) {
    shapeFailures.push(`期望里有「${name}」，但导出里没有 —— 捷径被删了？`);
  }
}

if (shapeFailures.length) {
  console.error('!! 请求形状自检失败（导出与契约不一致）：');
  for (const f of shapeFailures) console.error('  ✗ ' + f);
  console.error('   → 改完捷径记得同步改那两个跨后端的契约测试。');
  process.exit(1);
}
console.log(`✓ 请求形状自检通过（${allShortcuts.length} 条捷径与契约逐字一致）`);

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
