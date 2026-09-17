// 把 Go 版 cloud-clip/lib/unicode_text_test.go 的用例逐条搬到 JS，
// 验证 Cloudflare Worker 的移植版行为与 Go 后端一致。
// 用法：node --experimental-vm-modules test-sniff.mjs
import {
  sniffPayload,
  normalizeText,
  decodeUnicodeText,
  htmlDocumentToPlainText,
} from '../src/sniff.js';

let pass = 0;
let fail = 0;
const failures = [];

function check(name, got, want) {
  const g = JSON.stringify(got);
  const w = JSON.stringify(want);
  if (g === w) {
    pass++;
  } else {
    fail++;
    failures.push(`${name}\n    实际: ${g}\n    期望: ${w}`);
  }
}

const bytes = (...arr) => new Uint8Array(arr);
const utf8 = (s) => new TextEncoder().encode(s);
// Go 里的 "\x00" 字符串字面量就是 NUL
const nul = 0x00;

// ── TestDecodeUnicodeTextUTF16LE：无 BOM 的 UTF-16LE ──
{
  const payload = utf8('v\u00002\u00008\u0000 \u0000u\u0000t\u0000f\u00001\u00006\u0000 \u0000t\u0000e\u0000s\u0000t\u0000');
  check('UTF16LE 无 BOM', decodeUnicodeText(payload), 'v28 utf16 test');
}

// ── TestDecodeUnicodeTextUTF16BEWithBOM ──
{
  const payload = bytes(0xfe, 0xff, 0x00, 0x41, 0x4e, 0x2d);
  check('UTF16BE 带 BOM', decodeUnicodeText(payload), 'A中');
}

// ── TestDecodeUnicodeTextUTF32LE ──
{
  const payload = bytes(0xff, 0xfe, 0x00, 0x00, 0x68, 0, 0, 0, 0x69, 0, 0, 0);
  check('UTF32LE', decodeUnicodeText(payload), 'hi');
}

// ── TestDecodeUnicodeTextRejectsBinary ──
check('拒绝控制字符', decodeUnicodeText(bytes(0x01, 0x00, 0x02, 0x00, 0x03, 0x00)), null);
check('拒绝奇数长度', decodeUnicodeText(utf8('A\u0000B\u0000C')), null);
check('无 NUL 不走该路径', decodeUnicodeText(utf8('plain')), null);

// ── TestNormalizeTextStripsUTF8BOM ──
check('剥离 UTF-8 BOM', normalizeText(bytes(0xef, 0xbb, 0xbf, 0x68, 0x65, 0x6c, 0x6c, 0x6f)), 'hello');

// ── TestDecodeUnicodeTextEmptyOrSmall ──
check('空负载不解码', decodeUnicodeText(bytes()), null);
check('不足 4 字节不解码', decodeUnicodeText(utf8('a\u0000b')), null);

// ── TestSniffPayloadUTF16BecomesText ──
{
  const payload = utf8('v\u00002\u00008\u0000 \u0000t\u0000e\u0000s\u0000t\u0000');
  const { kind, ext } = sniffPayload(payload);
  check('UTF-16 嗅探为 text', [kind, ext], ['text', 'txt']);
  check('normalizeText 去掉了 NUL', normalizeText(payload).includes('\u0000'), false);
}

// ── TestSniffPayloadUTF16LEWithBOMNotMisclassifiedAsMP3 ──
{
  const s = 'plain text 你好';
  const arr = [0xff, 0xfe];
  for (const ch of s) {
    const c = ch.codePointAt(0);
    arr.push(c & 0xff, (c >> 8) & 0xff);
  }
  const payload = new Uint8Array(arr);
  const { kind, ext } = sniffPayload(payload);
  check('UTF-16LE BOM 不误判为 MP3', [kind, ext], ['text', 'txt']);
  check('UTF-16LE BOM 内容正确', normalizeText(payload), 'plain text 你好');
}

// ── TestHTMLDocumentToPlainText ──
{
  const html =
    '<!DOCTYPE html>\n<html><head><title>T</title></head>\n' +
    '<body><h1>标题</h1><p>Hello &amp; 你好</p><br><p>第二段</p></body></html>';
  check('HTML 转纯文本', htmlDocumentToPlainText(html), '标题 Hello & 你好 第二段');
}

// ── TestHTMLDocumentKeepsPlainTextAndCodes ──
check('普通文本不被改动', htmlDocumentToPlainText('a < b && c <not html>'), 'a < b && c <not html>');
check('代码不被改动', htmlDocumentToPlainText('func() bool { return a < b }'), 'func() bool { return a < b }');

// ── TestHTMLDocumentStripsScriptAndStyle ──
{
  const html = '<html><head><style>body{display:none}</style></head>' +
    '<body><script>alert(1)</script>可见文本</body></html>';
  check('剥离 script/style', htmlDocumentToPlainText(html), '可见文本');
}

// ── 额外：Go 端没有但必须对齐的嗅探行为 ──
check('纯 ASCII 是文本', sniffPayload(utf8('hello world')), { kind: 'text', ext: 'txt' });
check('PNG 是 image/png', sniffPayload(bytes(0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0)), { kind: 'image', ext: 'png' });
check('JPEG 是 image/jpg', sniffPayload(bytes(0xff, 0xd8, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0)), { kind: 'image', ext: 'jpg' });
check('PDF 是 file/pdf', sniffPayload(utf8('%PDF-1.4 xxxxxx')), { kind: 'file', ext: 'pdf' });
check('GIF 是 image/gif', sniffPayload(utf8('GIF89a............')), { kind: 'image', ext: 'gif' });
check('WEBP 是 image/webp', sniffPayload(utf8('RIFF....WEBPVP8 ')), { kind: 'image', ext: 'webp' });
check('随机二进制是 file/bin', sniffPayload(bytes(0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x1b, 0x00, 0x1c, 0x00, 0x1d, 0x00)), { kind: 'file', ext: 'bin' });

// ⚠️ 已知局限（Go 版同样存在，移植保持一致）：含 NUL 的随机字节若恰好能解成
// 全是可打印字符的 UTF-16，会被判成文本。validUnicodeRunes 只挡 < 0x20 的控制字符。
check('（局限）随机高字节可能被解成 UTF-16 文本', sniffPayload(bytes(0x00, 0xff, 0x13, 0x37, 0x00, 0xab, 0xcd, 0xef, 0x11, 0x22, 0x33, 0x44)).kind, 'text');

console.log(`\n通过 ${pass} 项，失败 ${fail} 项`);
if (failures.length) {
  console.log('\n失败明细：');
  for (const f of failures) console.log('  ✗ ' + f);
  process.exit(1);
}
console.log('全部与 Go 版行为一致 ✅');
