package lib

import (
	"encoding/json"
	"fmt"
	"testing"
)

// 看板的列：`POST /content/<id>/column`。
//
// 这是看板的最小实现 —— 固定三列（todo / doing / done）、卡片就是剪贴板条目本身、
// 不建新表也不做列内顺序。这里钉住的是**契约**，不是实现细节：
// 列名归一化、非法输入被拒、错误响应形状统一，以及**挪列不动 timestamp**。
func TestBoardColumnUpdate(t *testing.T) {
	srv := newShortcutServer(t, "", RoomAuthConfig{})

	status, body := shortcutDo(t, "POST", srv.URL+"/text?room=default", "待办事项", "text/plain")
	if status != 200 {
		t.Fatalf("POST /text 失败: %d %s", status, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("解析 /text 响应失败: %v (%s)", err, body)
	}

	before := boardContentJSON(t, srv.URL, created.ID)
	if before["column"] != "" {
		t.Fatalf("新条目的列应当是空串（= 待办），实测 %q", before["column"])
	}

	status, body = shortcutDo(t, "POST",
		srv.URL+"/content/"+created.ID+"/column?room=default", `{"column":"doing"}`, "application/json")
	if status != 200 {
		t.Fatalf("挪列失败: %d %s", status, body)
	}
	var moved struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Column string `json:"column"`
	}
	if err := json.Unmarshal(body, &moved); err != nil {
		t.Fatalf("解析挪列响应失败: %v (%s)", err, body)
	}
	if moved.Column != "doing" || moved.ID != created.ID || moved.Type != "text" {
		t.Fatalf("挪列响应不对: %+v", moved)
	}

	after := boardContentJSON(t, srv.URL, created.ID)
	if after["column"] != "doing" {
		t.Fatalf("挪列没生效: %v", after["column"])
	}
	// ⚠️ 挪列**不能**改时间戳。updateTextMessage 改正文时会刷时间戳，但挪位置不该让卡片
	// 在时间流里跳到最前面 —— 那等于拖一下就把整个列表重排了。
	if fmt.Sprint(after["timestamp"]) != fmt.Sprint(before["timestamp"]) {
		t.Fatalf("挪列改了时间戳: %v → %v", before["timestamp"], after["timestamp"])
	}

	// 空串归一成 todo（新条目默认落待办，客户端不必自己填）
	status, body = shortcutDo(t, "POST",
		srv.URL+"/content/"+created.ID+"/column?room=default", `{"column":""}`, "application/json")
	if status != 200 {
		t.Fatalf("空串应当被接受并归一成 todo: %d %s", status, body)
	}
	if got := boardContentJSON(t, srv.URL, created.ID)["column"]; got != "todo" {
		t.Fatalf("空串没有归一成 todo: %v", got)
	}

	// 大小写和空白都该容错
	status, _ = shortcutDo(t, "POST",
		srv.URL+"/content/"+created.ID+"/column?room=default", `{"column":"  DONE  "}`, "application/json")
	if status != 200 {
		t.Fatalf("带空白/大写的列名应当容错: %d", status)
	}
	if got := boardContentJSON(t, srv.URL, created.ID)["column"]; got != "done" {
		t.Fatalf("列名没有归一化: %v", got)
	}

	cases := []struct {
		label      string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"未知列", "POST", "/content/" + created.ID + "/column?room=default", `{"column":"nope"}`, 400, "invalid_column"},
		{"不存在的 id", "POST", "/content/999999/column?room=default", `{"column":"todo"}`, 404, "content_not_found"},
		{"非 POST", "GET", "/content/" + created.ID + "/column?room=default", "", 405, "method_not_allowed"},
		{"请求体不是 JSON", "POST", "/content/" + created.ID + "/column?room=default", "not json", 400, "invalid_body"},
		{"id 不是数字", "POST", "/content/abc/column?room=default", `{"column":"todo"}`, 400, "invalid_content_id"},
	}
	for _, tc := range cases {
		status, body := shortcutDo(t, tc.method, srv.URL+tc.path, tc.body, "application/json")
		if status != tc.wantStatus {
			t.Errorf("%s: 期望 HTTP %d，实测 %d（%s）", tc.label, tc.wantStatus, status, body)
			continue
		}
		boardAssertErrorShape(t, tc.label, body, tc.wantCode)
	}
}

// boardContentJSON 取一条内容的 JSON 形态（顺带验证 column 出现在 HTTP 响应里，
// 不只出现在 WebSocket 载荷里 —— 两处不一致会让「刷新后列丢失」这类问题很难查）。
func boardContentJSON(t *testing.T, base, id string) map[string]interface{} {
	t.Helper()
	status, body := shortcutDo(t, "GET", base+"/content/"+id+"?room=default&json=1", "", "")
	if status != 200 {
		t.Fatalf("GET /content/%s?json=1 失败: %d %s", id, status, body)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("解析内容 JSON 失败: %v (%s)", err, body)
	}
	return m
}

// boardAssertErrorShape 错误响应必须恒为 {code,error,message} 且三者都非空
// （与 TestErrorResponseShapeIsStable 同一条契约，这里只覆盖看板新增的错误码）。
func boardAssertErrorShape(t *testing.T, label string, body []byte, wantCode string) {
	t.Helper()
	var e struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("%s: 错误响应不是 JSON: %v (%s)", label, err, body)
	}
	if e.Code != wantCode {
		t.Fatalf("%s: 错误码期望 %q，实测 %q（%s）", label, wantCode, e.Code, body)
	}
	if e.Error == "" || e.Message == "" {
		t.Fatalf("%s: 错误响应必须同时带 error 和 message: %s", label, body)
	}
}
