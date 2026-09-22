package lib

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

// 请求体可能是 UTF-16 —— 快捷指令把字符串变量当请求体发出去时就是这样。
// 服务端必须认出来并解码，否则中英文一起变乱码（老捷径为此在客户端做过一堆转换动作，
// 每一种都有副作用）。这里把三种信号和「别误判」都钉住。
func TestTextBodyUTF16(t *testing.T) {
	// 带中文 + markdown 标记：既是「中文为主」（②那条隔位 NUL 对它无感）又含 ASCII
	const mixed = "# 标题\n- 第一条\n- 第二条\n"
	const ascii = "just a plain sentence"

	utf16Bytes := func(s string, order binary.ByteOrder, bom []byte) string {
		var out []byte
		out = append(out, bom...)
		for _, u := range utf16.Encode([]rune(s)) {
			var pair [2]byte
			order.PutUint16(pair[:], u)
			out = append(out, pair[:]...)
		}
		return string(out)
	}

	cases := []struct {
		name string
		body string
	}{
		{"UTF-16LE + BOM", utf16Bytes(mixed, binary.LittleEndian, []byte{0xFF, 0xFE})},
		{"UTF-16BE + BOM", utf16Bytes(mixed, binary.BigEndian, []byte{0xFE, 0xFF})},
		{"UTF-16LE 无 BOM · ASCII 为主（靠隔位 NUL 认）", utf16Bytes(ascii, binary.LittleEndian, nil)},
		{"UTF-16LE 无 BOM · 中文为主（靠「不是合法 UTF-8」认）", utf16Bytes(mixed, binary.LittleEndian, nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newShortcutServer(t, "global-pw", RoomAuthConfig{})

			// 走默认分支（不声明 Content-Type），和捷径实况一致
			status, raw := shortcutDo(t, "POST", srv.URL+"/text?room=default&auth=global-pw", tc.body, "")
			if status != 200 {
				t.Fatalf("期望 HTTP 200，实际 %d，响应体=%q", status, string(raw))
			}

			status, raw = shortcutDo(t, "GET", srv.URL+"/content/latest?room=default&auth=global-pw&format=json", "", "")
			if status != 200 {
				t.Fatalf("读回内容失败：HTTP %d，响应体=%q", status, string(raw))
			}
			var entry map[string]any
			if err := json.Unmarshal(raw, &entry); err != nil {
				t.Fatalf("响应不是 JSON: %v", err)
			}
			got, _ := entry["content"].(string)

			want := mixed
			if strings.Contains(tc.name, "ASCII") {
				want = ascii
			}
			if got != want {
				t.Errorf("解码结果不对\n期望 = %q\n实际 = %q", want, got)
			}
		})
	}

	// 反向：正常的 UTF-8 正文**不能**被当成 UTF-16 去解（误判会把好好的中文弄坏）
	t.Run("UTF-8 正文不受影响", func(t *testing.T) {
		srv := newShortcutServer(t, "global-pw", RoomAuthConfig{})
		status, raw := shortcutDo(t, "POST", srv.URL+"/text?room=default&auth=global-pw", mixed, "text/plain")
		if status != 200 {
			t.Fatalf("期望 HTTP 200，实际 %d，响应体=%q", status, string(raw))
		}
		status, raw = shortcutDo(t, "GET", srv.URL+"/content/latest?room=default&auth=global-pw&format=json", "", "")
		if status != 200 {
			t.Fatalf("读回内容失败：HTTP %d", status)
		}
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatalf("响应不是 JSON: %v", err)
		}
		if got, _ := entry["content"].(string); got != mixed {
			t.Errorf("UTF-8 正文被改动了\n期望 = %q\n实际 = %q", mixed, got)
		}
	})
}
