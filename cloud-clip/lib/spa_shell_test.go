package lib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// 前端路由兜底：真文件优先发文件；未知路径只对**要 HTML 的**导航回外壳（并注入 `<base>`），
// 其它一律 404 —— 接口路径绝不能被回成一份 html。
func TestSpaFallbackServesTheShellOnlyToHtmlRequests(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte(testShell)},
		"assets/index-abc123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
		"favicon.ico":            &fstest.MapFile{Data: []byte("\x00\x00")},
	}
	s := &ClipboardServer{config: &Config{}, staticFS: fsys}
	s.config.Server.Prefix = "/clip"
	handler := s.spaStaticHandler(fsys, "/clip")

	// 真文件照旧由 FileServer 发（这里是给「深路径下资源还找得到吗」留的锚点）
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log(1)") {
		t.Fatalf("a real file must still be served as a file, got %d: %s", rec.Code, rec.Body.String())
	}

	// 前端路由（深浅都算）：回外壳，并注入带 prefix 的 `<base>`
	for _, route := range []string{"/s/some.token", "/settings/profile"} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s should fall back to the shell, got %d", route, rec.Code)
		}
		if !isTheShell(rec.Body.String()) {
			t.Fatalf("%s should get the shell, got: %s", route, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `<base href="/clip/">`) {
			t.Fatalf("%s must get the prefixed <base>, got: %s", route, rec.Body.String())
		}
	}

	// 抓取程序普遍发 `Accept: */*`
	req := httptest.NewRequest(http.MethodGet, "/s/some.token", nil)
	req.Header.Set("Accept", "*/*")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !isTheShell(rec.Body.String()) {
		t.Fatalf("a crawler must get the shell too, got %d", rec.Code)
	}

	// 接口形状的请求：404，不能回 html（「下载下来是个 html」那条老坑）
	for name, mutate := range map[string]func(*http.Request){
		"json accept": func(r *http.Request) { r.Header.Set("Accept", "application/json") },
		"post":        func(r *http.Request) { r.Method = http.MethodPost },
	} {
		req = httptest.NewRequest(http.MethodGet, "/unknown/api", nil)
		mutate(req)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", name, rec.Code)
		}
	}
}

// 目录也要当「存在」交给 FileServer：否则 `/assets` 会被当成未知路由回一份外壳。
func TestSpaFallbackLetsTheFileServerHandleDirectories(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte(testShell)},
		"assets/index-abc123.js": &fstest.MapFile{Data: []byte("x")},
	}
	s := &ClipboardServer{config: &Config{}, staticFS: fsys}
	handler := s.spaStaticHandler(fsys, "")

	if !staticPathExists(fsys, "/") || !staticPathExists(fsys, "/assets") || !staticPathExists(fsys, "/assets/index-abc123.js") {
		t.Fatal("the root, a directory and a file must all count as existing")
	}
	if staticPathExists(fsys, "/nope") {
		t.Fatal("an unknown path must not count as existing")
	}

	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	// FileServer 接住了它：不该是「回外壳」那条路
	if strings.Contains(rec.Body.String(), `<div id="app"></div>`) {
		t.Fatalf("a directory must not be answered with the shell, got %d", rec.Code)
	}
}
