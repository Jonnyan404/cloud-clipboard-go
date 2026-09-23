package lib

import (
	"html"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPA 外壳（static/index.html）的服务端注入。
//
// 为什么由服务端来做这两件事：
//  1. **OG 卡片**：分享链接要能被微信 / Telegram / Slack 展开，而抓取程序**不执行 JS**、
//     浏览器也不会把 `#` 之后的部分发给服务器 —— 于是 token 必须在**路径**里，标签必须在
//     服务端就写进 HTML（见 share_landing.go）。
//  2. **`<base>`**：外壳会在 `/s/<token>` 这种**深路径**上被打开，而构建产物里的资源地址是
//     相对的（`./assets/…`、`vite.config.js` 的 `base: ''`）。不注入 `<base>` 的话它们会按
//     当前目录解析成 `/clip/s/assets/…`，全部 404。注入 `<base href="<prefix>/">` 之后，
//     相对资源、相对接口调用（前端把 `axios.defaults.baseURL` 设成 `document.baseURI`）
//     都回到 `<prefix>/` 这个基准目录上，路由 base 也从同一个值来。
//
// 注入用字符串定位而不是 HTML 解析：外壳是我们自己构建出来的，形状固定
// （`<head>` / `</head>` / 一处 `<title>`）。外壳形状出乎意料时**不注入**，
// 由调用方回落到通用卡片 —— 宁可少一张预览卡，也不要吐出一份半截 HTML。

const (
	shellHeadOpen = "<head>"
	shellHeadEnd  = "</head>"
)

// readShellHTML 读前端外壳。没有前端（未嵌入、也没配外部目录）时返回 false。
func (s *ClipboardServer) readShellHTML() (string, bool) {
	if s.staticFS == nil {
		return "", false
	}
	data, err := fs.ReadFile(s.staticFS, "index.html")
	if err != nil || len(data) == 0 {
		return "", false
	}
	return string(data), true
}

// shellBaseHref 外壳的基准目录：`<prefix>/`（没有 prefix 就是 `/`）。
//
// ⚠️ 前端也依赖这个值：它把 `document.baseURI` 同时当作 axios 的 baseURL 和路由 base
// （见 web-vue3/src/main.js、src/router/index.js）。改这里的形状要一并改那边。
func (s *ClipboardServer) shellBaseHref() string {
	return strings.TrimRight(s.config.Server.Prefix, "/") + "/"
}

// injectShellTags 往外壳里写 `<base>`、标题和额外的 head 标签。
// baseHref / title 为空表示不动对应那一项；headExtra 为空表示不追加标签。
// 外壳缺少 `<head>` 或 `</head>` 时返回 false（调用方回落）。
func injectShellTags(shell, baseHref, title, headExtra string) (string, bool) {
	headOpen := strings.Index(shell, shellHeadOpen)
	headEnd := strings.Index(shell, shellHeadEnd)
	if headOpen < 0 || headEnd < 0 || headEnd < headOpen {
		return "", false
	}

	// `<base>` 必须排在任何相对地址之前，所以紧跟在 `<head>` 后面。
	if baseHref != "" {
		at := headOpen + len(shellHeadOpen)
		shell = shell[:at] + "\n<base href=\"" + html.EscapeString(baseHref) + "\">" + shell[at:]
	}

	// 浏览器标签页的标题也换成分享标题（SPA 自己不设 document.title）。
	// 只认 `<head>` 里的那一处，避免动到别处的同名文本。
	if title != "" {
		if start := strings.Index(shell, "<title>"); start >= 0 && start < strings.Index(shell, shellHeadEnd) {
			if rel := strings.Index(shell[start:], "</title>"); rel >= 0 {
				contentStart := start + len("<title>")
				contentEnd := start + rel
				shell = shell[:contentStart] + html.EscapeString(title) + shell[contentEnd:]
			}
		}
	}

	if headExtra != "" {
		at := strings.Index(shell, shellHeadEnd)
		shell = shell[:at] + headExtra + "\n" + shell[at:]
	}

	return shell, true
}

// spaStaticHandler 发前端静态资源，并给**前端路由**兜底。
//
// 真文件优先（assets / favicon / manifest 等）；没有同名文件时：
//   - **要 HTML 的 GET/HEAD**（浏览器导航、聊天软件的抓取程序）回外壳，并注入 `<base>`，
//     这样 `/clip/s/<token>`、以及将来任何深度的前端路由都能直接刷新、直接粘贴打开；
//   - 其它一律 404 —— 接口路径绝不能被回成一份 html（那条「下载下来是个 html」的老坑，
//     Worker 侧注释里有完整记录）。
//
// 注意调用方已经把 prefix 剥掉了（`http.StripPrefix`），所以这里看到的是 `/s/<token>` 这种路径。
func (s *ClipboardServer) spaStaticHandler(fsys fs.FS, prefix string) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	baseHref := strings.TrimRight(prefix, "/") + "/"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if staticPathExists(fsys, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		if !wantsHTML(r) {
			http.NotFound(w, r)
			return
		}
		shell, ok := s.readShellHTML()
		if !ok {
			http.NotFound(w, r)
			return
		}
		page, ok := injectShellTags(shell, baseHref, "", "")
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeShellHTML(w, page)
	})
}

// staticPathExists 判断请求路径在外壳目录里有没有对应的实体（文件或目录）。
// 目录也算「存在」：交给 http.FileServer 自己处理索引与跳转，别把它当未知路由。
func staticPathExists(fsys fs.FS, urlPath string) bool {
	clean := strings.Trim(path.Clean("/"+urlPath), "/")
	if clean == "" || clean == "." {
		return true
	}
	_, err := fs.Stat(fsys, clean)
	return err == nil
}

// wantsHTML 这次请求是不是「要一份网页」。抓取程序普遍发 `Accept: */*`，所以它也算。
//
// 这条判断是接口不被回成 html 的那道闸：XHR 发的是 `application/json` 之类，命中不了。
func wantsHTML(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := r.Header.Get("Accept")
	if strings.TrimSpace(accept) == "" {
		return true
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "text/*") || strings.Contains(accept, "*/*")
}

func writeShellHTML(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, page)
}
