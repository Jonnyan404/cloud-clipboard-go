package lib

import (
	"strings"
	"testing"
)

func TestDecodeUnicodeTextUTF16LE(t *testing.T) {
	// BOM-less UTF-16LE (macOS Shortcuts 富文本剪贴板常见)
	payload := []byte("v\x002\x008\x00 \x00u\x00t\x00f\x001\x006\x00 \x00t\x00e\x00s\x00t\x00")
	got, ok := decodeUnicodeText(payload)
	if !ok || got != "v28 utf16 test" {
		t.Fatalf("decodeUnicodeText = %q, %v; want %q, true", got, ok, "v28 utf16 test")
	}
}

func TestDecodeUnicodeTextUTF16BEWithBOM(t *testing.T) {
	payload := []byte{0xFE, 0xFF, 0x00, 'A', 0x4E, 0x2D}
	got, ok := decodeUnicodeText(payload)
	if !ok || got != "A中" {
		t.Fatalf("decodeUnicodeText = %q, %v; want %q, true", got, ok, "A中")
	}
}

func TestDecodeUnicodeTextUTF32LE(t *testing.T) {
	payload := []byte{0xFF, 0xFE, 0x00, 0x00, 'h', 0, 0, 0, 'i', 0, 0, 0}
	got, ok := decodeUnicodeText(payload)
	if !ok || got != "hi" {
		t.Fatalf("decodeUnicodeText = %q, %v; want %q, true", got, ok, "hi")
	}
}

func TestDecodeUnicodeTextRejectsBinary(t *testing.T) {
	// 控制字符(非 \t\n\r)不是文本
	if _, ok := decodeUnicodeText([]byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00}); ok {
		t.Fatal("control-char payload should not decode as text")
	}
	// 奇数长度不是完整 UTF-16LE
	if _, ok := decodeUnicodeText([]byte("A\x00B\x00C")); ok {
		t.Fatal("odd-length payload should not decode as text")
	}
	// 无 NUL 的纯 ASCII 不进入该路径
	if _, ok := decodeUnicodeText([]byte("plain")); ok {
		t.Fatal("NUL-free payload should not decode as unicode text")
	}
}

func TestNormalizeTextStripsUTF8BOM(t *testing.T) {
	if got := normalizeText([]byte("\xEF\xBB\xBFhello")); got != "hello" {
		t.Fatalf("normalizeText = %q, want %q", got, "hello")
	}
}

func TestDecodeUnicodeTextEmptyOrSmall(t *testing.T) {
	if _, ok := decodeUnicodeText(nil); ok {
		t.Fatal("empty payload should not decode")
	}
	if _, ok := decodeUnicodeText([]byte("a\x00b")); ok {
		t.Fatal("sub-4-byte payload should not decode")
	}
}

func TestSniffPayloadUTF16BecomesText(t *testing.T) {
	payload := []byte("v\x002\x008\x00 \x00t\x00e\x00s\x00t\x00")
	kind, ext := sniffPayload(payload)
	if kind != "text" || ext != "txt" {
		t.Fatalf("sniffPayload = %q, %q; want text, txt", kind, ext)
	}
	if strings.Contains(string(normalizeText([]byte("v\x002\x008\x00 \x00t\x00e\x00s\x00t\x00"))), "\x00") {
		t.Fatal("normalizeText should strip NUL bytes from UTF-16 payload")
	}
}

func TestSniffPayloadUTF16LEWithBOMNotMisclassifiedAsMP3(t *testing.T) {
	// UTF-16LE BOM(0xFF 0xFE)曾被底层 MP3 帧同步判断(0xFF 且次字节 0xE0 掩码)误判为音频。
	payload := []byte{0xFF, 0xFE}
	for _, r := range "plain text 你好" {
		payload = append(payload, byte(r), byte(r>>8))
	}
	kind, ext := sniffPayload(payload)
	if kind != "text" || ext != "txt" {
		t.Fatalf("sniffPayload = %q, %q; want text, txt", kind, ext)
	}
	if got := normalizeText(payload); got != "plain text 你好" {
		t.Fatalf("normalizeText = %q, want %q", got, "plain text 你好")
	}
}

func TestHTMLDocumentToPlainText(t *testing.T) {
	html := "<!DOCTYPE html>\n<html><head><title>T</title></head>\n" +
		"<body><h1>标题</h1><p>Hello &amp; 你好</p><br><p>第二段</p></body></html>"
	got := htmlDocumentToPlainText(html)
	want := "标题 Hello & 你好 第二段"
	if got != want {
		t.Fatalf("htmlDocumentToPlainText = %q, want %q", got, want)
	}
}

func TestHTMLDocumentKeepsPlainTextAndCodes(t *testing.T) {
	if got := htmlDocumentToPlainText("a < b && c <not html>"); got != "a < b && c <not html>" {
		t.Fatalf("plain text must not be altered, got %q", got)
	}
	code := "func() bool { return a < b }"
	if got := htmlDocumentToPlainText(code); got != code {
		t.Fatalf("code must be preserved, got %q", got)
	}
}

func TestHTMLDocumentStripsScriptAndStyle(t *testing.T) {
	html := "<html><head><style>body{display:none}</style></head>" +
		"<body><script>alert(1)</script>可见文本</body></html>"
	if got := htmlDocumentToPlainText(html); got != "可见文本" {
		t.Fatalf("htmlDocumentToPlainText = %q, want 可见文本", got)
	}
}
