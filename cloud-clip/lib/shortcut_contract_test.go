package lib

// Android 快捷指令（HTTP Shortcuts）**真正发出的请求形状**，打在真的路由上（不是直接调 handler）。
//
// 为什么值得单独一份：那几条捷径用的形状和网页端不一样，网页端全绿并不能说明捷径能用 ——
//   · 鉴权走**查询串** `?auth=`，不是 `Authorization` 头
//   · 「接收最新」走的是 `/content/latest.json` 路径后缀，不是 `?json=1`
//   · 「展示文件」直连 `/file/<uuid>/<name>`，**且不带 room**（服务端按文件记录的房间鉴权）
// 曾经坏过的正是这里：下载是**第二次**请求，第一次 URL 上的 auth 不会自动跟过来，
// 于是文本正常、文件/图片一律 401。
//
// 请求形状取自 shortcuts/android/shortcuts.json 的导出（那个文件带密码不入库，
// 所以这里只能照抄一份 —— **改捷径的 URL 时同步改这里**）：
//
//	发送文本   POST {url}/text?room={room}&auth={auth}&name={name}         body: text/plain
//	发送文件   POST {url}/upload?room={room}&auth={auth}&name={name}       multipart，字段名 file
//	接收最新   GET  {url}/content/latest.json?room={room}&auth={auth}
//	接收指定ID GET  {url}/content/{ID}?room={room}&json=true&auth={auth}
//	展示文件   GET  {url}/file/{uuid}/{name}?auth={auth}
//
// 同一份契约的另一半在 cloudflare/workers/test/shortcut-contract.test.mjs —— 两边都要过。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

const shortcutDevice = "Android%E5%BF%AB%E6%8D%B7%E6%8C%87%E4%BB%A4" // name 参数是编码后的
const shortcutFileName = "测试 图 1.png"                                // 中文 + 空格，顺带验编码

var shortcutPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}

// newShortcutServer 起一个带**真路由**的服务器。绕开 setupRoutes 就等于绕开
// authMiddleware 的注册方式，那正是这条链路的一半；而手搓 ClipboardServer 结构体
// 又会漏掉构造函数里那些字段（UA 解析器、各种 map），一 POST 就 nil panic。
func newShortcutServer(t *testing.T, globalAuth string, roomAuth RoomAuthConfig) *httptest.Server {
	t.Helper()

	dir := t.TempDir()
	cfg := &Config{}
	cfg.Server.Auth = globalAuth
	cfg.Server.RoomAuth = roomAuth
	cfg.Server.StorageDir = filepath.Join(dir, "uploads")
	cfg.Server.HistoryFile = filepath.Join(dir, "history.json")
	cfg.Server.History = 50
	cfg.Server.RoomList = false // 别在测试里起房间清理 goroutine
	cfg.Text.Limit = 40960
	cfg.File.Limit = 204857600

	s, err := NewClipboardServer(cfg)
	if err != nil {
		t.Fatalf("构造服务器失败: %v", err)
	}
	s.logger = log.New(io.Discard, "", 0) // 测试输出里别刷屏
	s.setupRoutes()

	srv := httptest.NewServer(s.httpServer.Handler)
	t.Cleanup(srv.Close)
	return srv
}

func shortcutDo(t *testing.T, method, url, body, contentType string) (int, []byte) {
	t.Helper()

	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s 请求失败: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func shortcutWant(t *testing.T, label string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: 期望 HTTP %d，实际 %d", label, want, got)
	}
}

// shortcutSendFile 走 multipart —— 快捷指令的「发送文件」就是这样，part 自带真实文件名
func shortcutSendFile(t *testing.T, base, auth string) {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", shortcutFileName)
	if err != nil {
		t.Fatalf("构造 multipart 失败: %v", err)
	}
	if _, err := fw.Write(shortcutPNG); err != nil {
		t.Fatalf("写入 multipart 失败: %v", err)
	}
	mw.Close()

	status, _ := shortcutDo(t, http.MethodPost,
		base+"/upload?room=default&auth="+auth+"&name="+shortcutDevice,
		buf.String(), mw.FormDataContentType())
	shortcutWant(t, "发送文件 POST /upload", status, http.StatusOK)
}

// shortcutLatest 取「接收最新」的响应（.json 后缀 + 查询串鉴权，与捷径逐字一致）
func shortcutLatest(t *testing.T, base, auth string) map[string]interface{} {
	t.Helper()

	status, data := shortcutDo(t, http.MethodGet,
		base+"/content/latest.json?room=default&auth="+auth, "", "")
	shortcutWant(t, "接收最新 GET /content/latest.json", status, http.StatusOK)

	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("latest 响应不是 JSON: %v (%s)", err, string(data))
	}
	return out
}

// 文本那条链：发送 → 接收最新 → 接收指定ID
func shortcutTextFlow(t *testing.T, base, auth string) {
	t.Helper()

	status, _ := shortcutDo(t, http.MethodPost,
		base+"/text?room=default&auth="+auth+"&name="+shortcutDevice,
		"捷径验收文本", "text/plain")
	shortcutWant(t, "发送文本 POST /text", status, http.StatusOK)

	latest := shortcutLatest(t, base, auth)
	if latest["type"] != "text" {
		t.Fatalf("最新一条应当是 text，实际 %v", latest["type"])
	}
	if latest["content"] != "捷径验收文本" {
		t.Errorf("文字内容不对: %v", latest["content"])
	}

	// id 在 JSON 里是字符串还是数字，两个后端历史上不一致；快捷指令只是把它拼进 URL，
	// 所以这里跟着用 %v 拼，不假设类型。
	id := fmt.Sprintf("%v", latest["id"])
	status, data := shortcutDo(t, http.MethodGet,
		base+"/content/"+id+"?room=default&json=true&auth="+auth, "", "")
	shortcutWant(t, "接收指定ID GET /content/{id}?json=true", status, http.StatusOK)

	var byID map[string]interface{}
	if err := json.Unmarshal(data, &byID); err != nil {
		t.Fatalf("按 ID 的响应不是 JSON: %v (%s)", err, string(data))
	}
	if byID["content"] != "捷径验收文本" {
		t.Errorf("按 ID 拿到的内容不对: %v", byID["content"])
	}
}

// 文件那条链：发送 → 接收最新（拿 uuid）→ 展示文件（第二次请求，要自己带 auth）
func shortcutFileFlow(t *testing.T, base, auth string) {
	t.Helper()

	shortcutSendFile(t, base, auth)

	latest := shortcutLatest(t, base, auth)
	// PNG 判成 image（与 Worker 侧一致）；快捷指令那段 JS 其实只看 name + size，
	// 但两个后端在这里不能各说各话。
	if latest["type"] != "image" {
		t.Fatalf("最新一条应当是 image，实际 %v", latest["type"])
	}
	if latest["name"] != shortcutFileName {
		t.Errorf("文件名没有保真: %v", latest["name"])
	}
	uuid, _ := latest["uuid"].(string)
	if uuid == "" {
		t.Fatal("latest 里没有 uuid，快捷指令拼不出下载 URL")
	}

	// 下载 URL **不带 room** —— 服务端按文件自己记录的房间鉴权
	dl := base + "/file/" + uuid + "/" + url.PathEscape(shortcutFileName)
	status, data := shortcutDo(t, http.MethodGet, dl+"?auth="+auth, "", "")
	shortcutWant(t, "展示文件 GET /file/{uuid}/{name}?auth=", status, http.StatusOK)
	if !bytes.Equal(data, shortcutPNG) {
		t.Errorf("下载到的不是原字节: %d 字节 vs %d 字节", len(data), len(shortcutPNG))
	}
}

// 「接收最新」的语义：**后插入的那条**。
//
// timestamp 是秒级，所以「同一秒里先发文本、再发文件」是真实场景。Go 侧是倒着遍历消息队列，
// 天然取最后插入的；这条测试是给**将来有人把它改成按时间排序**准备的 —— Worker 侧就踩了
// 这个坑（只按秒级 timestamp 排序，并列时实测给的是先插入的那条），修在 content.js，
// 回归测试见 test/receive-path.test.mjs 的 H 段。
func TestShortcutLatestPrefersNewestInserted(t *testing.T) {
	srv := newShortcutServer(t, "", RoomAuthConfig{})
	base := srv.URL

	status, _ := shortcutDo(t, http.MethodPost,
		base+"/text?room=default&auth=&name="+shortcutDevice, "先发的文本", "text/plain")
	shortcutWant(t, "先发文本", status, http.StatusOK)

	shortcutSendFile(t, base, "") // 后发文件（同秒或下一秒，两种都该取它）

	latest := shortcutLatest(t, base, "")
	if latest["type"] != "image" {
		t.Errorf("应当取后插入的文件，实际拿到 type=%v name=%v", latest["type"], latest["name"])
	}
}

func TestShortcutContractEncryptedRoom(t *testing.T) {
	srv := newShortcutServer(t, "global-pw", nil)

	// 文本和文件用各自的服务器：两条消息的 timestamp 都是秒级，混在一个队列里
	// 「最新」取到谁不确定，测出来会是假失败。
	shortcutTextFlow(t, srv.URL, "global-pw")

	fileSrv := newShortcutServer(t, "global-pw", nil)
	shortcutFileFlow(t, fileSrv.URL, "global-pw")
}

func TestShortcutContractRejectsBadCredentials(t *testing.T) {
	srv := newShortcutServer(t, "global-pw", nil)
	base := srv.URL

	shortcutDo(t, http.MethodPost, base+"/text?room=default&auth=global-pw", "secret", "text/plain")

	status, _ := shortcutDo(t, http.MethodGet, base+"/content/latest.json?room=default", "", "")
	shortcutWant(t, "无凭据读最新被拒", status, http.StatusUnauthorized)

	status, _ = shortcutDo(t, http.MethodGet, base+"/content/latest.json?room=default&auth=wrong", "", "")
	shortcutWant(t, "密码错读最新被拒", status, http.StatusUnauthorized)

	// 下载这条最容易漏：它是第二次请求，第一次 URL 上的 auth 不会自动跟过来
	fileSrv := newShortcutServer(t, "global-pw", nil)
	shortcutSendFile(t, fileSrv.URL, "global-pw")
	latest := shortcutLatest(t, fileSrv.URL, "global-pw")
	uuid, _ := latest["uuid"].(string)
	dl := fileSrv.URL + "/file/" + uuid + "/" + url.PathEscape(shortcutFileName)

	status, _ = shortcutDo(t, http.MethodGet, dl, "", "")
	shortcutWant(t, "无凭据下载被拒", status, http.StatusUnauthorized)

	status, _ = shortcutDo(t, http.MethodGet, dl+"?auth=wrong", "", "")
	shortcutWant(t, "密码错下载被拒", status, http.StatusUnauthorized)

	status, _ = shortcutDo(t, http.MethodGet, dl+"?auth=global-pw", "", "")
	shortcutWant(t, "密码对下载通过", status, http.StatusOK)
}

func TestShortcutContractOpenRoom(t *testing.T) {
	// 开放实例：auth 变量留空，URL 里就是 `auth=`（空值），不是没有这个参数
	srv := newShortcutServer(t, "", RoomAuthConfig{})
	shortcutTextFlow(t, srv.URL, "")

	fileSrv := newShortcutServer(t, "", RoomAuthConfig{})
	shortcutFileFlow(t, fileSrv.URL, "")
}

func TestShortcutContractRoomPassword(t *testing.T) {
	srv := newShortcutServer(t, "", RoomAuthConfig{
		"vault": {Password: "vault-pw"},
	})
	base := srv.URL

	status, _ := shortcutDo(t, http.MethodPost,
		base+"/text?room=vault&auth=vault-pw&name="+shortcutDevice, "vault 里的文本", "text/plain")
	shortcutWant(t, "vault + 房间密码", status, http.StatusOK)

	status, _ = shortcutDo(t, http.MethodPost,
		base+"/text?room=vault&auth=default-pw&name="+shortcutDevice, "不该进去", "text/plain")
	shortcutWant(t, "vault + 别的密码", status, http.StatusUnauthorized)

	// 没设密码的房间，auth 传空串也要能进
	status, _ = shortcutDo(t, http.MethodPost,
		base+"/text?room=default&auth=&name="+shortcutDevice, "default 房间开着", "text/plain")
	shortcutWant(t, "default（没设密码）+ 空 auth", status, http.StatusOK)
}

// /content/* 的格式选择：?format= 优先，旧的三种信号保留为兼容。
//
// 为什么要锁：同一件事以前有三种表达（.json 后缀、?json=1、Accept 头），谁优先全靠读代码；
// 现在多了一个显式的 ?format=，优先级必须写死在这里。已发布的捷径走 .json 后缀 ——
// 那条路一旦断了，用户手机上装好的捷径就全废。
func TestContentFormatSelection(t *testing.T) {
	srv := newShortcutServer(t, "global-pw", nil)
	base := srv.URL
	auth := "global-pw"
	const body = "格式化测试内容"

	status, _ := shortcutDo(t, http.MethodPost, base+"/text?room=default&auth="+auth, body, "text/plain")
	shortcutWant(t, "先放一条文本", status, http.StatusOK)

	latest := shortcutLatest(t, base, auth)
	id, _ := latest["id"].(string) // 服务端把它序列化成字符串，不是数字
	if id == "" {
		t.Fatalf("没拿到 id: %v", latest)
	}

	cases := []struct {
		name     string
		path     string
		accept   string
		wantJSON bool
	}{
		{"不带任何信号 → raw", "/content/" + id + "?room=default&auth=" + auth, "", false},
		{"?format=json", "/content/" + id + "?room=default&auth=" + auth + "&format=json", "", true},
		{"?format=raw 压过 Accept 头", "/content/" + id + "?room=default&auth=" + auth + "&format=raw", "application/json", false},
		{"?format=raw 压过 .json 后缀", "/content/" + id + ".json?room=default&auth=" + auth + "&format=raw", "", false},
		{".json 后缀（已发布捷径在用）", "/content/" + id + ".json?room=default&auth=" + auth, "", true},
		{"?json=1（旧信号）", "/content/" + id + "?room=default&auth=" + auth + "&json=1", "", true},
		{"Accept 头", "/content/" + id + "?room=default&auth=" + auth, "application/json", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, base+tc.path, nil)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("期望 200，实际 %d，响应体=%q", resp.StatusCode, string(raw))
			}

			if tc.wantJSON {
				var payload map[string]interface{}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("期望 JSON，实际不是: %v（%q）", err, string(raw))
				}
				if payload["content"] != body {
					t.Fatalf("content 字段不对: %v", payload["content"])
				}
				return
			}
			if strings.TrimSpace(string(raw)) != body {
				t.Fatalf("期望原文 %q，实际 %q", body, strings.TrimSpace(string(raw)))
			}
		})
	}

	// 不认识的 format 必须报错，不能静默回落成 raw —— 客户端以为拿到 HTML、
	// 实际拿到原文，是会出事的。
	t.Run("?format=html 报 400 而不是回落", func(t *testing.T) {
		status, raw := shortcutDo(t, http.MethodGet,
			base+"/content/"+id+"?room=default&auth="+auth+"&format=html", "", "")
		if status != http.StatusBadRequest {
			t.Fatalf("期望 400，实际 %d，响应体=%q", status, string(raw))
		}
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("错误体不是 JSON: %v", err)
		}
		if payload["code"] != "unsupported_format" {
			t.Fatalf("code 期望 unsupported_format，实际 %q", payload["code"])
		}
	})
}

// 错误响应的形状是所有客户端共用的契约，必须锁死。
//
// 为什么单独一条：Apple 快捷指令的「获取URL内容」**不暴露 HTTP 状态码**，只能读响应体，
// 所以错误体长什么样直接决定它能不能给出正确的提示。这里曾经按 Accept 分叉 ——
// 带 Accept 给 JSON、不带就给 text/plain —— 而捷径恰恰不发 Accept，
// 于是「文本超限」被误报成「服务器未确认保存，请检查部署地址及服务器状态」。
//
// 三个字段各有用途，缺一不可：code 给程序判断，error 给日志/英文用户，message 给人看。
func TestErrorResponseShapeIsStable(t *testing.T) {
	srv := newShortcutServer(t, "global-pw", nil)
	base := srv.URL

	// newShortcutServer 里 Text.Limit = 40960，超一个字符即可触发
	tooLong := strings.Repeat("A", 40961)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		ctype  string
		accept string
		status int
		code   string
	}{
		{"文本超限 · 捷径实况（不发 Accept）", http.MethodPost, "/text?room=default&auth=global-pw", tooLong, "text/plain", "", http.StatusRequestEntityTooLarge, "text_too_long"},
		{"文本超限 · Accept: application/json", http.MethodPost, "/text?room=default&auth=global-pw", tooLong, "text/plain", "application/json", http.StatusRequestEntityTooLarge, "text_too_long"},
		{"文本超限 · Accept: text/html", http.MethodPost, "/text?room=default&auth=global-pw", tooLong, "text/plain", "text/html", http.StatusRequestEntityTooLarge, "text_too_long"},
		{"方法不允许", http.MethodGet, "/text?room=default&auth=global-pw", "", "", "", http.StatusMethodNotAllowed, "method_not_allowed"},
		{"内容不存在", http.MethodGet, "/content/999999?room=default&auth=global-pw", "", "", "", http.StatusNotFound, "content_not_found"},
		// 走 /text 而不是 /content/latest：空房间的 latest 会先回「没有可用的内容」，
		// 那是业务状态不是鉴权状态，测鉴权得挑一条一定会过鉴权中间件的路径。
		{"凭据不对", http.MethodPost, "/text?room=default&auth=wrong-pw", "hello", "text/plain", "", http.StatusUnauthorized, "unauthorized_invalid_token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rdr io.Reader
			if tc.body != "" {
				rdr = strings.NewReader(tc.body)
			}
			req, err := http.NewRequest(tc.method, base+tc.path, rdr)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			if tc.ctype != "" {
				req.Header.Set("Content-Type", tc.ctype)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.status {
				t.Fatalf("期望 HTTP %d，实际 %d，响应体=%q", tc.status, resp.StatusCode, string(raw))
			}
			// Content-Type 必须声明 JSON：客户端就是靠它决定怎么解析的
			if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Fatalf("Content-Type 应为 JSON，实际 %q", ct)
			}
			var payload map[string]string
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("错误体不是 JSON: %v，响应体=%q", err, string(raw))
			}
			if payload["code"] != tc.code {
				t.Errorf("code 期望 %q，实际 %q", tc.code, payload["code"])
			}
			if payload["error"] == "" || payload["message"] == "" {
				t.Errorf("error/message 不能为空: %v", payload)
			}
		})
	}
}
