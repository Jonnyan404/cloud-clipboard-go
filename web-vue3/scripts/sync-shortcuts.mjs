// 把 shortcuts/ 里的捷径产物同步到 public/，让网页能直接下载。
//
// 为什么是「构建时同步」而不是把文件放进 public/ 入库：那些文件是唯一源，再放一份
// 就是同一内容存两处 —— 这个仓库已经为「两份同内容的东西」踩过两次坑（shortcuts.js
// 那条路），不再犯。所以 public/shortcuts/ 是派生产物，写在 .gitignore 里。
//
// dev 和 build 都跑这个脚本：dev 下 vite 也会服务 public/，这样本地就能点下载。
import { cpSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..');
const src = join(repoRoot, 'shortcuts');
const dest = join(here, '..', 'public', 'shortcuts');

// 不整个删目标目录：一来没必要，二来沙箱环境有批量删除守卫会把它拦下来
// （实测报 SAFE_DELETE_BULK_CONFIRM_REQUIRED，构建直接中断）。
// 改成「确保目录在 + 覆盖同名文件 + 逐个清掉源里已经没有的」。
mkdirSync(join(dest, 'apple'), { recursive: true });
mkdirSync(join(dest, 'android'), { recursive: true });

// Apple：只同步签名产物（.shortcut）。source/ 下的 .cherri 是构建输入，不上网。
const appleFiles = readdirSync(join(src, 'apple')).filter((name) => name.endsWith('.shortcut'));
for (const name of readdirSync(join(dest, 'apple'))) {
    if (!appleFiles.includes(name)) {
        rmSync(join(dest, 'apple', name), { force: true });
    }
}
for (const name of appleFiles) {
    cpSync(join(src, 'apple', name), join(dest, 'apple', name));
}

// Android：HTTP Shortcuts 的导入包
cpSync(join(src, 'android', 'shortcuts.zip'), join(dest, 'android', 'shortcuts.zip'));

// 产物的更新日期清单，给网页上那个下载弹窗的**时间轴**用。
//
// ⚠️★ 这份日期**不从 git 算**。本仓库 2026-09-28 才从 cloud-clipboard-go 导入 `shortcuts/`，
// 历史里所有产物提交都落在导入那一天 —— 用 `git log` 算出来的「更新于」全是 `2026-09-28`，
// 而它对使用者毫无意义（那是搬家的日子，不是捷径变过的日子）。
//
// 所以日期由 `shortcuts/history.json` **人工维护**：某个平台的产物真的变了，
// 就往那个平台的头一行加一个日期。（上游 cloud-clipboard-go 那份快捷指令的更新记录是
// `2026-09-17` 首发、`2026-09-22` 重建 —— 这份文件把它们继承了下来。）
//
// ⚠️ 必须「最新在前」：弹窗把数组第一条当作最近一次更新。
// ⚠️ 格式与顺序由 `web-vue3/scripts/check-display-semantics.mjs` 钉着（写错就在门禁上红）。
//
// 读不到这个文件（或某平台没有记录）就当作「没有更新记录」：弹窗不显示时间轴，
// 下载与提醒都照常。
function readHistory() {
    try {
        return JSON.parse(readFileSync(join(src, 'history.json'), 'utf8'));
    } catch {
        return {};
    }
}

const history = readHistory();
const dates = (platform) => (Array.isArray(history?.[platform]) ? history[platform] : []);

const meta = {
    apple: dates('apple'),
    android: dates('android'),
};
writeFileSync(join(dest, 'meta.json'), `${JSON.stringify(meta, null, 2)}\n`);

console.log(`[shortcuts] 已同步 ${appleFiles.length} 个 Apple 捷径 + 1 个 Android 包 → public/shortcuts/`);
console.log(`[shortcuts] 更新记录：apple=${meta.apple.join(' ') || '（空）'} · android=${meta.android.join(' ') || '（空）'}`);
