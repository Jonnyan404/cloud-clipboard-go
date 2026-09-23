// SPA 外壳（index.html）的服务端注入 —— 与 Go 侧 `lib/spa_shell.go` 一一对应，行为必须一致。
//
// 为什么由服务端来做这两件事：
//   1. **OG 卡片**：分享链接要能被微信 / Telegram / Slack 展开，而抓取程序**不执行 JS**、
//      浏览器也不会把 `#` 之后的部分发给服务器 —— 于是 token 必须在**路径**里，标签必须在
//      服务端就写进 HTML（见 share-landing.js）。
//   2. **`<base>`**：外壳会在 `/s/<token>`、以及任何前端深路径上被打开，而构建产物里的资源
//      地址是相对的（`./assets/…`）。不注入 `<base>` 的话它们会按当前目录解析成
//      `/s/assets/…`，全部 404、页面白屏。注入之后相对资源、相对接口调用（前端把
//      `axios.defaults.baseURL` 设成 `document.baseURI`）和路由 base 都回到同一个基准目录。
//
// 为什么用字符串定位而不是 `HTMLRewriter`：外壳是我们自己构建出来的，形状固定
// （`<head>` / `</head>` / 一处 `<title>`）；而本仓库的测试是**纯 Node 跑的**
// （node:sqlite 当 D1、esbuild 打包，不起 wrangler 运行时），`HTMLRewriter` 在那里不存在。
// 字符串注入可以直接单测，形状出乎意料时**不注入**、由调用方回落到通用卡片 ——
// 宁可少一张预览卡，也不要吐出一份半截 HTML。

const SHELL_HEAD_OPEN = '<head>';
const SHELL_HEAD_END = '</head>';
const SHELL_TITLE_OPEN = '<title>';
const SHELL_TITLE_END = '</title>';

// Worker 没有子路径部署的概念（Go 侧有 `-prefix`），基准目录恒为根。
// ⚠️ 前端也依赖这个值：它把 `document.baseURI` 同时当作 axios 的 baseURL 和路由 base
// （见 web-vue3/src/base.js、src/router/index.js）。改这里的形状要一并改那边。
export const SHELL_BASE_HREF = '/';

export function escapeHtml(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/**
 * 读前端外壳。资源层没绑定、或取不到 index.html 时返回 null（调用方回落）。
 */
export async function readShellHtml(env, request) {
  if (!env || !env.ASSETS) {
    return null;
  }
  try {
    const res = await env.ASSETS.fetch(new Request(new URL('/index.html', request.url), { method: 'GET' }));
    if (!res || !res.ok) {
      return null;
    }
    const html = await res.text();
    return html ? html : null;
  } catch (error) {
    console.error('Read SPA shell error:', error);
    return null;
  }
}

/**
 * 往外壳里写 `<base>`、标题和额外的 head 标签。
 *
 * baseHref / title / headExtra 为空表示不动对应那一项；
 * 外壳缺少 `<head>` 或 `</head>` 时返回 null —— 调用方回落，别自己吐半截 HTML。
 */
export function injectShellTags(shell, { baseHref = '', title = '', headExtra = '' } = {}) {
  const headOpen = shell.indexOf(SHELL_HEAD_OPEN);
  const headEnd = shell.indexOf(SHELL_HEAD_END);
  if (headOpen < 0 || headEnd < 0 || headEnd < headOpen) {
    return null;
  }

  let page = shell;

  // `<base>` 必须排在任何相对地址之前，所以紧跟在 `<head>` 后面。
  if (baseHref) {
    const at = headOpen + SHELL_HEAD_OPEN.length;
    page = `${page.slice(0, at)}\n<base href="${escapeHtml(baseHref)}">${page.slice(at)}`;
  }

  // 浏览器标签页的标题也换成分享标题（SPA 自己不设 document.title）。
  // 只认 `<head>` 里的那一处，避免动到别处的同名文本。
  if (title) {
    const start = page.indexOf(SHELL_TITLE_OPEN);
    if (start >= 0 && start < page.indexOf(SHELL_HEAD_END)) {
      const contentStart = start + SHELL_TITLE_OPEN.length;
      const contentEnd = page.indexOf(SHELL_TITLE_END, start);
      if (contentEnd >= contentStart) {
        page = page.slice(0, contentStart) + escapeHtml(title) + page.slice(contentEnd);
      }
    }
  }

  if (headExtra) {
    const at = page.indexOf(SHELL_HEAD_END);
    page = `${page.slice(0, at)}${headExtra}\n${page.slice(at)}`;
  }

  return page;
}
