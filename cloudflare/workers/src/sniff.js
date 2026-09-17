// 内容嗅探与文本规范化 —— 与 Go 版 cloud-clip/lib/handler.go 逐条对齐。
//
// 本文件**刻意零依赖**：不 import 任何东西，因此可以被 Node 直接加载做单测
// （Worker 的其余代码用打包器风格的无后缀导入，Node 的 ESM 解析不了）。
//
// 两个后端的行为必须一致，否则同一个客户端在不同部署上会得到不同结果。
// 单测用例见 cloud-clip/lib/unicode_text_test.go —— 同一批用例跑两边。

const UTF8_STRICT = new TextDecoder('utf-8', { fatal: true });

export function isValidUtf8(bytes) {
  try {
    UTF8_STRICT.decode(bytes);
    return true;
  } catch {
    return false;
  }
}

export function hasNul(bytes) {
  return bytes.indexOf(0) >= 0;
}

// ── Unicode 文本解码（移植自 decodeUnicodeText）──
// 仅当整个负载可完整解码且全部为可打印/空白字符时才认为是文本，
// 否则返回 null，确保真正的二进制仍按文件处理（"文件就是文件"）。

function utf16Units(bytes, littleEndian) {
  const units = new Array(bytes.length >> 1);
  for (let i = 0; i < units.length; i++) {
    const a = bytes[2 * i];
    const b = bytes[2 * i + 1];
    units[i] = littleEndian ? (a | (b << 8)) : ((a << 8) | b);
  }
  return units;
}

function validUnicodeRunes(str) {
  if (!str) return null;
  let out = '';
  for (const ch of str) {
    const cp = ch.codePointAt(0);
    if (cp === 0xfeff) continue; // BOM
    if (cp < 0x20 && cp !== 0x09 && cp !== 0x0a && cp !== 0x0d) return null;
    out += ch;
  }
  return out || null;
}

function decodeUTF32(bytes, littleEndian) {
  if (bytes.length === 0 || bytes.length % 4 !== 0) return null;
  let out = '';
  for (let i = 0; i + 4 <= bytes.length; i += 4) {
    const v = littleEndian
      ? (bytes[i] | (bytes[i + 1] << 8) | (bytes[i + 2] << 16) | (bytes[i + 3] << 24))
      : ((bytes[i] << 24) | (bytes[i + 1] << 16) | (bytes[i + 2] << 8) | bytes[i + 3]);
    out += String.fromCodePoint(v >>> 0);
  }
  return validUnicodeRunes(out);
}

export function decodeUnicodeText(data) {
  if (data.length < 4 || !hasNul(data)) return null;
  if (data[0] === 0xff && data[1] === 0xfe && data[2] === 0x00 && data[3] === 0x00) {
    return decodeUTF32(data.subarray(4), true); // UTF-32LE
  }
  if (data[0] === 0x00 && data[1] === 0x00 && data[2] === 0xfe && data[3] === 0xff) {
    return decodeUTF32(data.subarray(4), false); // UTF-32BE
  }
  let units;
  if (data[0] === 0xff && data[1] === 0xfe) {
    units = utf16Units(data.subarray(2), true); // UTF-16LE + BOM
  } else if (data[0] === 0xfe && data[1] === 0xff) {
    units = utf16Units(data.subarray(2), false); // UTF-16BE + BOM
  } else {
    // 无 BOM：按 UTF-16LE 尝试（macOS 快捷指令的剪贴板条目常见）
    if (data.length % 2 !== 0) return null;
    units = utf16Units(data, true);
  }
  return validUnicodeRunes(String.fromCharCode.apply(null, units));
}

// ── 内容魔数嗅探（移植自 sniffPayload）──
// kind 只用于区分 text 与非 text；ext 会成为默认文件名，
// 而文件名经 FileHandler.determineFileType 决定接收端拿到 image 还是 file —— 所以这张表是承重的，
// 不能因为"看着啰嗦"就精简掉。
export function sniffPayload(data) {
  const nul = hasNul(data);
  // 含 NUL 的负载优先尝试按 UTF-16/32 文本解码。此判断必须放在魔数表之前，
  // 否则 UTF-16LE 的 BOM(0xFF 0xFE) 会命中 MP3 帧同步判断(0xFF + 0xE0 掩码)被误判为音频。
  if (nul && decodeUnicodeText(data) !== null) return { kind: 'text', ext: 'txt' };

  if (data.length >= 12) {
    const eq = (off, arr) => arr.every((b, i) => data[off + i] === b);
    if (eq(0, [0x89, 0x50, 0x4e, 0x47])) return { kind: 'image', ext: 'png' };
    if (eq(0, [0xff, 0xd8, 0xff])) return { kind: 'image', ext: 'jpg' };
    if (eq(0, [0x47, 0x49, 0x46, 0x38, 0x37, 0x61]) || eq(0, [0x47, 0x49, 0x46, 0x38, 0x39, 0x61])) {
      return { kind: 'image', ext: 'gif' };
    }
    if (eq(0, [0x52, 0x49, 0x46, 0x46])) { // RIFF
      if (eq(8, [0x57, 0x45, 0x42, 0x50])) return { kind: 'image', ext: 'webp' };
      if (eq(8, [0x41, 0x56, 0x49, 0x20])) return { kind: 'file', ext: 'avi' };
      if (eq(8, [0x57, 0x41, 0x56, 0x45])) return { kind: 'file', ext: 'wav' };
    }
    if (eq(4, [0x66, 0x74, 0x79, 0x70])) { // ftyp
      const sub = String.fromCharCode(data[8], data[9], data[10], data[11]);
      if (sub === 'qt  ') return { kind: 'file', ext: 'mov' };
      if (sub === 'M4A ' || sub === 'M4B ') return { kind: 'file', ext: 'm4a' };
      if (sub === 'M4V ') return { kind: 'file', ext: 'm4v' };
      return { kind: 'file', ext: 'mp4' };
    }
    if (eq(0, [0x1a, 0x45, 0xdf, 0xa3])) return { kind: 'file', ext: 'mkv' };
    if (eq(0, [0x00, 0x00, 0x01, 0xba]) || eq(0, [0x00, 0x00, 0x01, 0xb3])) return { kind: 'file', ext: 'mpg' };
    if (eq(0, [0x25, 0x50, 0x44, 0x46])) return { kind: 'file', ext: 'pdf' }; // %PDF
    if (eq(0, [0x1f, 0x8b])) return { kind: 'file', ext: 'gz' };
    if (eq(0, [0x50, 0x4b])) return { kind: 'file', ext: 'zip' }; // PK
    if (eq(0, [0x49, 0x44, 0x33]) || (data[0] === 0xff && (data[1] & 0xe0) === 0xe0)) {
      return { kind: 'file', ext: 'mp3' };
    }
    if (eq(0, [0x66, 0x4c, 0x61, 0x43])) return { kind: 'file', ext: 'flac' }; // fLaC
    if (eq(0, [0x4f, 0x67, 0x67, 0x53])) return { kind: 'file', ext: 'ogg' }; // OggS
  }

  if (nul) return { kind: 'file', ext: 'bin' };
  if (isValidUtf8(data)) return { kind: 'text', ext: 'txt' };
  return { kind: 'file', ext: 'bin' };
}

// ── 文本规范化（移植自 normalizeText / htmlDocumentToPlainText）──

const HTML_DOC_RE = /^\s*(?:<!doctype\s+html|<html\b|<\?xml\b|<head\b|<body\b)/is;
const HTML_BLOCK_RE = /<(?:script|style|head|title|noscript|template)[^>]*>[\s\S]*?<\/(?:script|style|head|title|noscript|template)>/gi;
// 必须带 i 标志：Go 版是 (?is)，而 HTML 里 <!DOCTYPE> 常是大写 —— 漏掉 i 会让声明整段留下。
const TAG_RE = /<!doctype[^>]*>|<!\[[^\]]*\]>|<!--[\s\S]*?-->|<\s*\/?\s*[a-zA-Z][a-zA-Z0-9]*(?:\s[^>]*)?\s*>/gis;
const HTML_SPACE_RE = /[ \t\f\v]+/g;
const HTML_BLANK_LINE_RE = /\r?\n\s*\r?\n+/g;

// 仅在文本形如完整 HTML 文档时剥离标签与常见实体，普通文本（含 < 或 > 的代码等）原样保留。
export function htmlDocumentToPlainText(text) {
  if (!HTML_DOC_RE.test(text)) return text;
  let s = text.replace(HTML_BLOCK_RE, ' ');
  s = s.replace(TAG_RE, ' ');
  s = s.replace(/&nbsp;/g, ' ');
  s = s.replace(/&amp;/g, '&');
  s = s.replace(/&lt;/g, '<');
  s = s.replace(/&gt;/g, '>');
  s = s.replace(/&quot;/g, '"');
  s = s.replace(/&apos;/g, "'");
  s = s.replace(/&#39;/g, "'");
  s = s.replace(HTML_SPACE_RE, ' ');
  s = s.replace(HTML_BLANK_LINE_RE, '\n');
  return s.trim();
}

// 将文本负载统一为 UTF-8：UTF-16/32 负载解码为 UTF-8，并去掉 UTF-8 BOM。
export function normalizeText(bytes) {
  const decoded = decodeUnicodeText(bytes);
  if (decoded !== null) return decoded;
  const s = new TextDecoder('utf-8').decode(bytes);
  return s.startsWith('\uFEFF') ? s.slice(1) : s;
}
