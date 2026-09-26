package lib

// CORS 放行规则：**只放桌面客户端与本机开发**，一切别的来源都不放。
//
// ⚠️★ 为什么这几条值一个专门的文件：原来的规则是 `Access-Control-Allow-Origin: *`，
// 而那在本机服务端上**是一个真的洞** —— 这个服务端跑在用户自己的机器上、房间默认不要密码，
// 于是**用户访问的任意网站**都能把剪贴板历史整段读走（理由写在 `corsMiddleware` 上）。
// 收窄之后最容易犯的两个错是**对称的**：
//
//   - 收得太狠 → 把桌面客户端一起挡掉，表现是「桌面端一条接口都调不通」，只在真机上看得见；
//   - 收得太松 → 洞又回来了（`*`、`null`、或者「包含 127 就算过」那种模糊判断）。
//
// 所以两边都各钉一组用例。Rust 侧同一份规则在 `clip9/crates/server/src/cors.rs`。
//
// ⚠️ 文件里分两层：① `allowedCORSOrigin` 这个**判定函数**；② **中间件真的发了哪些头**
// （后者是 2026-09-26 对活服务端实测后补的 —— 光测判定函数，「头忘了发」照样全绿）。

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedCORSOrigin(t *testing.T) {
	allowed := []string{
		// ⚠️ 实测值：桌面 webview 发出来的就是这个（原来注释里猜的 http://tauri.localhost 是错的）。
		"tauri://localhost",
		"Tauri://LocalHost", // 大小写不敏感
		// 本机开发（vite dev 的端口是任意的）。
		"http://localhost",
		"http://localhost:5173",
		"https://localhost:8443",
		"http://127.0.0.1",
		"http://127.0.0.1:9501",
		"https://127.0.0.1:9501",
	}
	for _, origin := range allowed {
		if got := allowedCORSOrigin(origin); got == "" {
			t.Errorf("%q 应当放行，却被拒了", origin)
		}
	}

	rejected := []string{
		// ⚠️★ 这几条就是收窄要挡的东西。
		"https://evil.example",
		"http://evil.example:9501",
		"https://example.com",
		// 看着像回环、其实不是。
		"http://localhost.evil.example",
		"http://127.0.0.1.evil.example",
		"http://192.168.1.10:9501",
		"http://10.0.0.1",
		// ⚠️ `null`（file:// 页面、沙箱 iframe、data: URL）**不放行**。
		"null",
		// 形状不对的一律拒。
		"",
		"   ",
		"localhost:5173",            // 缺协议
		"http://",                   // 缺 host
		"http://localhost:5173/",    // 带路径
		"http://localhost:5173/api", // 带路径
		"http://localhost:5173?x=1", // 带查询
		"http://user@localhost:5173",
		"http://localhost:",     // 空端口
		"http://localhost:port", // 端口不是数字
		"ftp://localhost:5173",  // 非 http(s)
		"tauri://localhost/x",   // 桌面来源也不许带路径
		// IPv6 回环**没放行**（少见，真要支持得单独加一条）。
		"http://[::1]:5173",
		"http://[::1]",
	}
	for _, origin := range rejected {
		if got := allowedCORSOrigin(origin); got != "" {
			t.Errorf("%q 必须被拒，却放行成了 %q", origin, got)
		}
	}
}

// 放行时**原样回写**那个来源（不是 `*`，也不是自己拼一个）。
func TestAllowedCORSOriginEchoesTheOrigin(t *testing.T) {
	if got := allowedCORSOrigin("http://localhost:5173"); got != "http://localhost:5173" {
		t.Errorf("应当原样回写来源，得到 %q", got)
	}
	// 前后空白要去掉（header 值本来不该有，但别依赖对手的规范程度）。
	if got := allowedCORSOrigin("  tauri://localhost  "); got != "tauri://localhost" {
		t.Errorf("应当去掉空白，得到 %q", got)
	}
}

// 中间件层：**真的发了哪些头**。
//
// ⚠️★ 为什么非要这一层：判定函数全绿而中间件忘了发头（或者反过来）是**两种不同的故障**，
// 而症状都只是「桌面端一条接口都调不通」。这里用 `httptest` 直接调中间件 ——
// 它不碰 `s` 的字段，所以零值 `&ClipboardServer{}` 就够，不需要起真服务端。
func TestCORSMiddlewareSendsTheHeaders(t *testing.T) {
	handler := (&ClipboardServer{}).corsMiddleware(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	cases := []struct {
		name      string
		origin    string
		wantAllow string
	}{
		{"放行的来源（桌面端）", "tauri://localhost", "tauri://localhost"},
		{"放行的来源（本机开发）", "http://localhost:5173", "http://localhost:5173"},
		{"被拒的来源", "https://evil.example", ""},
		{"没有 Origin 头", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/server", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantAllow {
				t.Errorf("Access-Control-Allow-Origin = %q，期望 %q", got, tc.wantAllow)
			}
			// ⚠️ `Vary: Origin` **无条件**要带（理由写在 `corsMiddleware` 上）：
			// 一份没有放行头的响应同样会被缓存，之后再喂给本该放行的来源。
			if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
				t.Errorf("Vary = %q，应当包含 Origin", got)
			}
		})
	}
}

// 预检（`OPTIONS`）与实际请求**一致**：放行的来源要答上允许的方法与头；
// 被拒的来源只回 200、**一个放行头都不带**（由浏览器判定预检失败）。
//
// ⚠️ 这条是「上行能不能发出去」的那条：上行带 `Authorization` → **一定**会预检，
// 所以预检不放行的话，「桌面端能读到 /server」是个假象（真上行一条都发不出去）。
func TestCORSPreflightMatchesTheActualRequest(t *testing.T) {
	handler := (&ClipboardServer{}).corsMiddleware(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/server", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	allowed := preflight("tauri://localhost")
	if allowed.Code != http.StatusOK {
		t.Errorf("放行的来源预检应当 200，得到 %d", allowed.Code)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("允许的方法里要有 POST，得到 %q", got)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Errorf("允许的头里要有 authorization，得到 %q", got)
	}

	rejected := preflight("https://evil.example")
	if got := rejected.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("被拒的来源不该拿到预检放行头，得到 %q", got)
	}
}

// newCORSServer 起一个带**真路由**的服务器。
//
// ⚠️ 为什么要起真的：这次收窄漏掉三处，根因就是**只测了判定函数与一层中间件** ——
// 另外三个端点各自抄了一份，没人碰过它们。绕开 `setupRoutes` 就等于看不见
// 「哪个端点挂在哪一层」，而那正是漏掉的地方。
func newCORSServer(t *testing.T) *httptest.Server {
	t.Helper()

	dir := t.TempDir()
	cfg := &Config{}
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
	s.logger = log.New(io.Discard, "", 0)
	s.setupRoutes()

	srv := httptest.NewServer(s.httpServer.Handler)
	// ⚠️ 顺序是故意的：后注册的先跑 → srv.Close（等在途请求）→ 排干异步落盘 → 删目录。
	t.Cleanup(func() { s.WaitForHistoryWrites() })
	t.Cleanup(srv.Close)
	return srv
}

// ⚠️★ 端点级回归：**每一个自己处理 CORS 的端点**都要被这条盖住。
//
// 2026-09-26 收窄 `*` 的时候漏了三处（`authMiddleware` 罩着的 `/text`、`/upload`、
// `/content/:id`，以及 `handleRooms`、`handle_share`）—— 它们仍然对任意来源放行，
// 而 `/content` 正是「任意网站读走本机剪贴板历史」那条路。
//
// ⚠️ 这条测试的价值全在**按端点打**：只测 `allowedCORSOrigin` 的话，
// 上面那三处漏改**一条都不会红**。
func TestCORSIsNarrowedOnEveryEndpoint(t *testing.T) {
	srv := newCORSServer(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		// 走 corsMiddleware 的
		{"GET /server", http.MethodGet, "/server"},
		{"GET /content（敏感读）", http.MethodGet, "/content?room=default&format=json"},
		{"GET /rooms（自己还有一份）", http.MethodGet, "/rooms"},
		// 走 authMiddleware 的（以前自己抄了一份 `*`）
		{"POST /text", http.MethodPost, "/text?room=default&name=t&client=c"},
		{"POST /upload", http.MethodPost, "/upload?room=default&name=t"},
		{"GET /content/1", http.MethodGet, "/content/1?format=json"},
		// handle_share（第三份抄件）
		{"GET /share", http.MethodGet, "/share?room=default"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// ① 被拒的来源：一个放行头都不许有。
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			req.Header.Set("Origin", "https://evil.example")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("任意网站拿到了放行头 %q —— 本机剪贴板历史读得走了", got)
			}
			if got := resp.Header.Get("Vary"); !strings.Contains(got, "Origin") {
				t.Errorf("Vary = %q，应当包含 Origin", got)
			}

			// ② 放行的来源（桌面端）：要拿到原样回写的头，否则桌面端一条接口都调不通。
			req2, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			req2.Header.Set("Origin", "tauri://localhost")
			resp2, err := http.DefaultClient.Do(req2)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp2.Body.Close()
			if got := resp2.Header.Get("Access-Control-Allow-Origin"); got != "tauri://localhost" {
				t.Errorf("桌面端来源应当被放行，Access-Control-Allow-Origin = %q", got)
			}
		})
	}
}

// ⚠️ 预检也要**逐端点**一致：桌面上行带 `Authorization` → 一定先预检。
// 预检不过的话，「能读到 /server」是个假象（真上行一条都发不出去）。
func TestCORSPreflightIsNarrowedOnEveryEndpoint(t *testing.T) {
	srv := newCORSServer(t)

	for _, path := range []string{"/text", "/upload", "/content/1", "/rooms", "/share", "/server"} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodOptions, srv.URL+path, nil)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			req.Header.Set("Origin", "https://evil.example")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "authorization")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("%s 的预检把 %q 放行了", path, got)
			}
		})
	}
}
