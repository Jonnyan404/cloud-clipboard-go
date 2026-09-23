import { SHARE_TOKEN_QUERY_KEY, findContentById, findFileMeta, parseShareToken } from './share';
import {
  SHARE_IMAGE_MAX_BYTES,
  SHARE_TITLE_LIMIT,
  firstSummaryLine,
  formatShareSize,
  isPreviewableImageName,
  joinShareMeta,
  shareExpiryNote,
  truncateRunes,
} from './share-summary';
import { SHELL_BASE_HREF, escapeHtml, injectShellTags, readShellHtml } from './spa-shell';

// GET /s/<token> —— 分享链接的**唯一地址**：一份注入了 OG 卡片的 SPA 外壳。
// 与 Go 侧 share_landing.go 对应，行为一致（那边文件头有完整论证）。
//
// 为什么必须有服务端这一步：分享页是 SPA，而抓取程序**不执行 JS**；浏览器也不会把 `#` 之后
// 的部分发给服务器。所以 token 必须在**路径**里（不能是 `/#/s?t=…`），OG 标签必须在服务端就
// 写进 HTML —— 否则微信 / Telegram / Slack 抓到的只是一个空壳，预览里只有域名。
//
// 真人拿到的也是这一份 HTML：跑起 SPA 后由前端路由 `/s/:token` 接管，看到的就是分享页本身。
// 也就是说**抓取程序和真人是同一个地址**，没有第二跳、也没有第二个身份（2026-09 之前不是这样：
// 那时前端走 hash 路由，这个页面把真人 `location.replace` 到 `/#/s?t=…`，同一个分享有两个地址）。
//
// ⚠️ 因此这条路径上的内容是**公开可抓、且会被第三方缓存**的：把链接贴进微信 /
// Telegram / Slack，摘要会出现在预览里（群里所有人都看得到），而且平台侧的缓存
// **删不掉** —— 之后过期、删内容、加密码都不影响对方已经抓到的副本。所以：
//   1. 带密码的分享**绝不输出内容摘要**（只说「需要密码」）；
//   2. 失效 / 已删除的分享只输出通用卡片；
//   3. 页面带 noindex，别让搜索引擎把分享页当正文收录；
//   4. 这里**不计数**：抓取程序会反复访问，统计只认前端分享页的上报（见 /share/visit）。
//
// 这个页面**不消耗使用次数**：只读内容元信息，取正文仍走 /content、/file。
//
// 没有前端外壳可用时（资源层没绑定 / 取不到 index.html），退化成一张通用卡片：
// 抓取程序要的就是标签，而真人那边本来也没有前端可以看。
const SITE_NAME = 'Cloud Clipboard';

function summarySiteMeta(...parts) {
  return joinShareMeta(SITE_NAME, ...parts);
}

/**
 * og:image：只在图片、且**不限次数**的分享上给。
 *
 * 抓取程序抓图会走 /file 的 token 校验，而那里会**消耗一次使用额度** ——
 * 限次的分享被预览用掉一次，真人点开时就成「已用完」了。另外别把大文件塞进对方预览。
 */
function maybeFillShareImage(card, { origin, fileUUID, fileName, fileSize, claims, token }) {
  if (Number(claims.maxUses || 0) > 0) {
    return;
  }
  if (Number(fileSize || 0) > SHARE_IMAGE_MAX_BYTES) {
    return;
  }
  if (!isPreviewableImageName(fileName) || !fileUUID) {
    return;
  }
  const name = fileName || 'image';
  card.image = `${origin}/file/${fileUUID}/${encodeURIComponent(name)}?${SHARE_TOKEN_QUERY_KEY}=${encodeURIComponent(token)}`;
}

async function fillCardFromContent(card, { request, env, claims, token }) {
  const origin = new URL(request.url).origin;

  const describeFile = async (uuid, name, size) => {
    card.title = truncateRunes(name || '文件', SHARE_TITLE_LIMIT);
    card.description = summarySiteMeta('文件', formatShareSize(size), shareExpiryNote(claims.exp));
    maybeFillShareImage(card, { origin, fileUUID: uuid, fileName: name, fileSize: size, claims, token });
  };

  if (claims.type === 'content') {
    const contentId = Number(claims.id);
    if (!Number.isFinite(contentId)) {
      return;
    }
    const row = await findContentById(env, contentId, claims.room, true);
    if (!row) {
      card.title = '内容已被删除或过期';
      card.description = '这条分享指向的内容已不在服务器上。';
      return;
    }
    if (String(row.type || 'text') === 'file') {
      const meta = await findFileMeta(env, row.uuid);
      if (!meta) {
        card.title = '内容已被删除或过期';
        card.description = '这条分享指向的文件已不在服务器上。';
        return;
      }
      await describeFile(meta.uuid, meta.name, meta.size);
      return;
    }

    const summary = firstSummaryLine(row.content);
    if (!summary) {
      card.title = '有人分享了一段文本';
      card.description = `通过 ${SITE_NAME} 分享，打开即可查看。`;
      return;
    }
    card.title = summary;
    card.description = summarySiteMeta('分享的文本', shareExpiryNote(claims.exp));
    return;
  }

  if (claims.type === 'file') {
    const meta = await findFileMeta(env, claims.id);
    if (!meta) {
      card.title = '内容已被删除或过期';
      card.description = '这条分享指向的文件已不在服务器上。';
      return;
    }
    await describeFile(meta.uuid, meta.name, meta.size);
  }
}

/**
 * 要写进外壳 `<head>` 的那几行：noindex + description + og:* + twitter:*。
 *
 * 值一律转义：标题来自内容首行、文件名，description 里也可能带用户内容 ——
 * 直接拼进属性就是一处 XSS。`og:image` 只在有图时出现，`twitter:card` 跟着它选。
 */
function shareCardHeadTags(card) {
  const title = escapeHtml(card.title);
  const description = escapeHtml(card.description);
  const tags = [
    '<meta name="robots" content="noindex, nofollow">',
    `<meta name="description" content="${description}">`,
    '<meta property="og:type" content="website">',
    `<meta property="og:site_name" content="${escapeHtml(card.siteName)}">`,
    `<meta property="og:title" content="${title}">`,
    `<meta property="og:description" content="${description}">`,
    `<meta property="og:url" content="${escapeHtml(card.canonical)}">`,
  ];
  if (card.image) {
    const image = escapeHtml(card.image);
    tags.push(`<meta property="og:image" content="${image}">`);
    tags.push(`<meta name="twitter:image" content="${image}">`);
  }
  tags.push(`<meta name="twitter:card" content="${card.image ? 'summary_large_image' : 'summary'}">`);
  tags.push(`<meta name="twitter:title" content="${title}">`);
  tags.push(`<meta name="twitter:description" content="${description}">`);
  return tags.join('\n');
}

/**
 * 没有外壳可用时的兜底页：只有一张卡。
 *
 * ⚠️ 这里**没有按钮、也没有跳转脚本** —— 真人拿到的正常路径是「外壳 + 注入的卡片」，
 * 由前端路由 `/s/:token` 接管；走到这个分支说明根本没有前端，给个按钮也无处可去。
 */
function renderShareCardHtml(card) {
  const image = card.image
    ? `<meta property="og:image" content="${escapeHtml(card.image)}">\n<meta name="twitter:image" content="${escapeHtml(card.image)}">`
    : '';
  const twitterCard = card.image ? 'summary_large_image' : 'summary';

  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(card.title)}</title>
<meta name="robots" content="noindex, nofollow">
<meta name="description" content="${escapeHtml(card.description)}">
<meta property="og:type" content="website">
<meta property="og:site_name" content="${escapeHtml(card.siteName)}">
<meta property="og:title" content="${escapeHtml(card.title)}">
<meta property="og:description" content="${escapeHtml(card.description)}">
<meta property="og:url" content="${escapeHtml(card.canonical)}">
${image}
<meta name="twitter:card" content="${twitterCard}">
<meta name="twitter:title" content="${escapeHtml(card.title)}">
<meta name="twitter:description" content="${escapeHtml(card.description)}">
<style>
:root { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
  font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans SC", sans-serif;
  background: #f6f7f9; color: #1f2430; }
@media (prefers-color-scheme: dark) { body { background: #16181d; color: #e9ecf2; } }
main { max-width: 30rem; padding: 2rem 1.5rem; text-align: center; }
h1 { font-size: 1.1rem; margin: 0 0 .5rem; }
p { margin: 0; opacity: .75; word-break: break-word; }
</style>
</head>
<body>
<main>
<h1>${escapeHtml(card.title)}</h1>
<p>${escapeHtml(card.description)}</p>
</main>
</body>
</html>
`;
}

export async function handleShareLanding(request, env) {
  const url = new URL(request.url);
  const token = String(request.params?.token || decodeURIComponent(url.pathname.split('/s/').pop() || '')).trim();

  const card = {
    siteName: SITE_NAME,
    title: '分享链接无效或已过期',
    description: '这条分享可能已过期、次数用尽，或者链接不完整。',
    image: '',
    canonical: `${url.origin}/s/${encodeURIComponent(token)}`,
  };

  const claims = token ? await parseShareToken(env, token) : null;
  if (claims) {
    if (claims.pwdHash) {
      // 有密码就到此为止：预览里放内容摘要等于把保护绕过去。
      card.title = '受密码保护的分享';
      card.description = '打开后需要输入分享密码才能查看内容。';
    } else {
      try {
        await fillCardFromContent(card, { request, env, claims, token });
      } catch (error) {
        console.error('Share landing card error:', error);
      }
    }
  }

  const headers = {
    'Content-Type': 'text/html; charset=utf-8',
    // noindex：分享页不该被搜索引擎收录。抓取程序不看这个头，所以 OG 仍然有效。
    'X-Robots-Tag': 'noindex, nofollow',
    // private + must-revalidate：别让中间缓存/CDN 长期留着带摘要的这份 HTML。
    'Cache-Control': 'private, max-age=0, must-revalidate',
    // 地址里带 token：不让它作为 Referer 流到第三方；顺带挡住 MIME 嗅探。
    'Referrer-Policy': 'no-referrer',
    'X-Content-Type-Options': 'nosniff',
  };

  // 正常路径：把卡片注入 SPA 外壳，真人跑起前端路由后看到的就是分享页本身。
  const shell = await readShellHtml(env, request);
  if (shell) {
    const page = injectShellTags(shell, {
      baseHref: SHELL_BASE_HREF,
      title: card.title,
      headExtra: shareCardHeadTags(card),
    });
    if (page) {
      return new Response(page, { status: 200, headers });
    }
  }

  // 没有外壳可用：只给一张卡。
  return new Response(renderShareCardHtml(card), { status: 200, headers });
}
