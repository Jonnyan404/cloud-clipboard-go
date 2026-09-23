// 分享链接用到的几个纯函数：摘要、大小、有效期文案。
//
// 单独一个模块是因为它们同时被两处需要（share.js 写记录列表的名称、share-landing.js
// 渲染 OG 卡片），而 share-landing.js 已经依赖 share.js —— 放在任何一边都会成环。
// 与 Go 侧 share_landing.go 里那几个同名函数保持一致。

export const SHARE_TITLE_LIMIT = 80;
export const SHARE_DESC_LIMIT = 160;
export const SHARE_IMAGE_MAX_BYTES = 5 * 1024 * 1024;
export const SHARE_NAME_LIMIT = 60;

export function truncateRunes(value, limit) {
  const text = String(value == null ? '' : value).trim();
  const chars = [...text];
  if (limit <= 0 || chars.length <= limit) {
    return text;
  }
  return `${chars.slice(0, limit).join('').trim()}…`;
}

/**
 * 卡片/记录列表里给一段文本取「一眼能认出来」的那一行。
 *
 * 刻意只取第一行有内容的行：正文可能含账号、验证码、完整密码 —— 摘要少搬一点，
 * 第三方预览缓存里就少留一点（那段缓存删不掉，见 share-landing.js 的说明）。
 */
export function firstSummaryLine(text, limit = SHARE_TITLE_LIMIT) {
  for (const line of String(text == null ? '' : text).split('\n')) {
    let clean = line.replace(/\s+/g, ' ').trim();
    clean = clean.replace(/^[#>*-·|\s]+/, '').replace(/[#>*-·|\s]+$/, '').trim();
    if (clean) {
      return truncateRunes(clean, limit);
    }
  }
  return '';
}

export function formatShareSize(bytes) {
  const value = Number(bytes || 0);
  if (!Number.isFinite(value) || value <= 0) {
    return '';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return unit === 0 ? `${value}B` : `${size.toFixed(1)}${units[unit]}`;
}

export function shareExpiryNote(exp) {
  const value = Number(exp || 0);
  if (value <= 0) {
    return '';
  }
  const remaining = value - Math.floor(Date.now() / 1000);
  if (remaining <= 0) {
    return '已过期';
  }
  if (remaining < 3600) {
    return `${Math.ceil(remaining / 60)} 分钟内有效`;
  }
  return `${Math.ceil(remaining / 3600)} 小时内有效`;
}

export function joinShareMeta(siteName, ...parts) {
  const kept = parts.filter((part) => String(part || '').trim() !== '');
  if (!kept.length) {
    return `通过 ${siteName} 分享，打开即可查看。`;
  }
  return kept.join(' · ');
}

export function isPreviewableImageName(name) {
  const lower = String(name || '').toLowerCase().trim();
  const idx = lower.lastIndexOf('.');
  if (idx < 0) {
    return false;
  }
  return ['png', 'jpg', 'jpeg', 'gif', 'webp', 'avif', 'bmp'].includes(lower.slice(idx + 1));
}
