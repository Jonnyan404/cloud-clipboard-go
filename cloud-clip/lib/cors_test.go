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

import "testing"

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
