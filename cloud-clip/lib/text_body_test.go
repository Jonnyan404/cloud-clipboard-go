package lib

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// /text 的请求体有**三种形态**：JSON、multipart 表单、以及「整个 body 就是正文」。
// 前两种是给快捷指令用的 —— 它把字符串变量当请求体发出去时字节会变成 UTF-16，
// 而结构化请求体是按 UTF-8 序列化的。
//
// 这个测试钉三件事：
//  1. 三种形态**都能逐字节存下**同一段带中文和 markdown 的正文（转义 / 编码一个都不许动）；
//  2. `application/x-www-form-urlencoded` **刻意不认**，必须继续走「整个 body 是正文」——
//     它是 `curl --data-binary` 不带 `-H` 时的默认类型，当表单解析会把这类请求静默存成空串；
//  3. 声明了 JSON 但正文不是 JSON 时要**明确报错**，不能把半截正文存进去。
func TestTextRequestBodyForms(t *testing.T) {
	// 带中文、行首 `-`、竖线表格 —— 正是会被 markdown 转换器转义的那几种字符
	content := "# 标题\n- 第一条\n- 第二条\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n"

	const boundary = "----ccgTestBoundary"
	multipartBody := fmt.Sprintf(
		"--%s\r\nContent-Disposition: form-data; name=\"content\"\r\n\r\n%s\r\n--%s--\r\n",
		boundary, content, boundary)
	jsonBody, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		t.Fatalf("构造 JSON 正文失败: %v", err)
	}

	cases := []struct {
		name  string
		ctype string
		body  string
		want  string // 期望**逐字节**存下来的正文
	}{
		{
			name:  "纯文本（老客户端）",
			ctype: "text/plain",
			body:  content,
			want:  content,
		},
		{
			name:  "不声明 Content-Type（老捷径）",
			ctype: "",
			body:  content,
			want:  content,
		},
		{
			name:  "JSON",
			ctype: "application/json",
			body:  string(jsonBody),
			want:  content,
		},
		{
			name:  "multipart 表单",
			ctype: "multipart/form-data; boundary=" + boundary,
			body:  multipartBody,
			want:  content,
		},
		{
			// ⚠️ 不是「没实现」，是**故意不认**：curl --data-binary 默认就带这个类型，
			// 当表单解析会得到空的 content 字段 —— 正文会被静默丢掉。
			name:  "urlencoded 刻意不认，正文原样存下",
			ctype: "application/x-www-form-urlencoded",
			body:  "content=" + content,
			want:  "content=" + content,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newShortcutServer(t, "global-pw", RoomAuthConfig{})

			status, raw := shortcutDo(t, "POST", srv.URL+"/text?room=default&auth=global-pw", tc.body, tc.ctype)
			if status != 200 {
				t.Fatalf("期望 HTTP 200，实际 %d，响应体=%q", status, string(raw))
			}

			// ?format=json 是**规范**信号（?json=1 和 .json 后缀是兼容信号，已发布的捷径在用）。
			// 不显式要 json 时，文本条目回的是正文本身。
			status, raw = shortcutDo(t, "GET", srv.URL+"/content/latest?room=default&auth=global-pw&format=json", "", "")
			if status != 200 {
				t.Fatalf("读回内容失败：HTTP %d，响应体=%q", status, string(raw))
			}
			var entry map[string]any
			if err := json.Unmarshal(raw, &entry); err != nil {
				t.Fatalf("响应不是 JSON: %v，响应体=%q", err, string(raw))
			}
			got, _ := entry["content"].(string)
			if got != tc.want {
				t.Errorf("正文被改动了\n期望 = %q\n实际 = %q", tc.want, got)
			}
		})
	}

	// 错误路径单独跑：坏 JSON 必须回 400 + invalid_body，且**不能**留下一条内容
	t.Run("坏 JSON 被拒绝且不留内容", func(t *testing.T) {
		srv := newShortcutServer(t, "global-pw", RoomAuthConfig{})

		status, raw := shortcutDo(t, "POST", srv.URL+"/text?room=default&auth=global-pw", `{"content": `, "application/json")
		if status != 400 {
			t.Fatalf("期望 HTTP 400，实际 %d，响应体=%q", status, string(raw))
		}
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("错误体不是 JSON: %v，响应体=%q", err, string(raw))
		}
		if payload["code"] != "invalid_body" {
			t.Errorf("code 期望 invalid_body，实际 %q", payload["code"])
		}
		if !strings.Contains(payload["message"], "请求体") {
			t.Errorf("message 应当说明是请求体的问题，实际 %q", payload["message"])
		}

		// 房间应当是空的 —— 被拒绝的请求不能悄悄存下半条
		status, raw = shortcutDo(t, "GET", srv.URL+"/content/latest?room=default&auth=global-pw&format=json", "", "")
		if status == 200 {
			t.Errorf("坏请求不该留下内容，却读到了：%q", string(raw))
		}
	})
}
