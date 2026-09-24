package lib

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 这个文件守的是一类**跨端、又特别难查**的故障：Service Worker 的导航兜底。
//
// SPA 是个 PWA，注册的 sw.js 里有一条 NavigationRoute：**所有没被排除的同源导航**，
// 一律回预缓存里的 index.html。于是任何「服务端直接吐出来的页面 / 接口」一旦不在
// navigateFallbackDenylist 里，用户点过去就会看到 SPA 首页 —— 不是 404、不是白屏，
// 而是**一个看起来完全正常的界面**，而且刷新一下有时又好了（SW 生效时机不同）。
// 这个现象极难从「SPA 里点了没反应」这个描述追到 SW 上：
//   - Go 侧路由是好的（curl 得到 200）；
//   - SPA 侧链接是好的（DOM 里 href 正确）；
//   - 只有浏览器里两条都对、结果却错，因为中间隔着 SW。
//
// 所以这里钉一条不变量：**main.go 里注册的每一条服务端路由，都必须在 SW 的
// navigateFallbackDenylist 里有对应项**。谁以后加了新路由忘了同步，这条会红，
// 并在错误信息里直接给出该往 vite.config.js 里补什么。

// TestSpaServiceWorkerCoversEveryServerRoute main.go 的每条服务端路由都要被 SW 放行。
func TestSpaServiceWorkerCoversEveryServerRoute(t *testing.T) {
	vitePath := filepath.Join("..", "..", "web-vue3", "vite.config.js")
	viteSrc, err := os.ReadFile(vitePath)
	if err != nil {
		t.Skipf("读不到 %s，跳过跨端契约检查: %v", vitePath, err)
	}

	denylist := swDenylistSources(t, string(viteSrc))
	if len(denylist) < 8 {
		// 名单被读空的话，下面所有断言都会「通过」—— 那是这条测试最坏的失败方式
		t.Fatalf("只解析出 %d 条 denylist，解析大概坏了", len(denylist))
	}

	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读不到 main.go: %v", err)
	}
	routes := serverRoutePaths(string(mainSrc))
	if len(routes) < 15 {
		t.Fatalf("只解析出 %d 条路由，解析大概坏了: %v", len(routes), routes)
	}

	compiled := make([]*regexp.Regexp, 0, len(denylist))
	for _, src := range denylist {
		re, err := regexp.Compile(src)
		if err != nil {
			t.Fatalf("denylist 里的 %q 不是合法正则（Go 与 JS 在这里语法一致）: %v", src, err)
		}
		compiled = append(compiled, re)
	}

	for _, route := range routes {
		if route == "/" {
			continue // SPA 兜底本身，正是「被兜」的对照组
		}
		hit := false
		for _, re := range compiled {
			if re.MatchString(route) {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("服务端路由 %q 没有被 Service Worker 放行：\n"+
				"  用户点过去会看到 SPA 首页，而不是这个页面（sw.js 的导航兜底会吞掉它）。\n"+
				"  修法：在 %s 的 navigateFallbackDenylist 里加一条 /^\\%s/。",
				route, vitePath, regexp.QuoteMeta(route))
		}
	}
}

// TestSpaServiceWorkerDenylistHasNoTypo 名单里不该有「永远不会匹配任何路由」的条目。
//
// 另半边：上面那条管「少了」，这条管「多了且写错」。`/^\\/automations/` 这种手滑
// 不会报任何错，只是静默失效 —— 而它失效的后果和漏写一模一样。
func TestSpaServiceWorkerDenylistHasNoTypo(t *testing.T) {
	vitePath := filepath.Join("..", "..", "web-vue3", "vite.config.js")
	viteSrc, err := os.ReadFile(vitePath)
	if err != nil {
		t.Skipf("读不到 %s，跳过: %v", vitePath, err)
	}
	denylist := swDenylistSources(t, string(viteSrc))

	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读不到 main.go: %v", err)
	}
	routes := serverRoutePaths(string(mainSrc))

	// 自己探一遍：拿每条 entry 的「前缀字面量」去撞路由表。
	// 不做正则求交（那太重），只要求「去掉锚点后能成为某条路由的前缀」。
	for _, src := range denylist {
		literal := strings.TrimPrefix(src, "^")
		literal = strings.TrimSuffix(literal, "$")
		literal = strings.TrimSuffix(literal, "/") // `^/file/` 这种要把结尾斜杠去掉再比
		ok := false
		for _, route := range routes {
			if strings.HasPrefix(route, literal) || strings.HasPrefix(literal, route) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("denylist 里的 %q 匹配不上任何一条服务端路由 —— 多半是写错了，"+
				"而写错不会报任何错，只是静默失效", src)
		}
	}
}

// TestEmbeddedSpaCarriesAutomationEntry 内嵌的 SPA（cloud-clip/lib/static）必须带上自动化入口。
//
// 为什么值得一条测试：正式构建走的是 `-tags embed`，用的是 `lib/static`，
// 而前端改完只在 `web-vue3/dist` 里 —— 要显式跑 `DEPLOY_STATIC=1 npm run build` 才会同步过去。
// 这个「忘了同步」是无症状的：`go build -tags embed` 完全成功，跑起来也正常，
// 只是界面永远停在上一版。用户遇到的现象是「SPA 里根本没有这个功能」，而代码明明写好了。
func TestEmbeddedSpaCarriesAutomationEntry(t *testing.T) {
	indexPath := filepath.Join("static", "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Skipf("读不到 %s，跳过: %v", indexPath, err)
	}

	entry := regexp.MustCompile(`/assets/index-[A-Za-z0-9_-]+\.js`).FindString(string(raw))
	if entry == "" {
		t.Fatalf("%s 里找不到入口 bundle", indexPath)
	}
	js, err := os.ReadFile(filepath.Join("static", strings.TrimPrefix(entry, "/")))
	if err != nil {
		// 产物可能是「未压缩 + .gz/.br」的形态，读不到就跳过，不误报
		t.Skipf("读不到内嵌产物 %s: %v", entry, err)
	}
	if !strings.Contains(string(js), "/automation") {
		t.Errorf("内嵌 SPA（cloud-clip/lib/static）里没有自动化入口 —— 用 `-tags embed` 构建出来的界面" +
			"会完全看不到这个功能，而 go build 不会报任何错。\n" +
			"  修法：在 web-vue3 里跑 `DEPLOY_STATIC=1 npm run build`（VitePWA 的 dist 会拷进 lib/static）。")
	}
}

// swDenylistSources 从 vite.config.js 里抠出 navigateFallbackDenylist 的正则字面量。
//
// 不引 JS 解析器（为一个配置项不值当），只做够用的解析：截出中括号之间的部分，
// 逐行取 `/…/` 这种字面量，跳过注释行。JS 与 Go 在这几个正则上语法一致，
// 所以能直接交给 regexp 编译；`\/` 换成 `/`。
func swDenylistSources(t *testing.T, viteSrc string) []string {
	t.Helper()
	start := strings.Index(viteSrc, "navigateFallbackDenylist: [")
	if start < 0 {
		t.Fatal("vite.config.js 里找不到 navigateFallbackDenylist —— 是被改名了还是删了？")
	}
	body := viteSrc[start:]
	if end := strings.Index(body, "]"); end >= 0 {
		body = body[:end]
	}

	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") ||
			strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") ||
			strings.HasPrefix(line, "navigateFallbackDenylist") {
			continue
		}
		if !strings.HasPrefix(line, "/") {
			continue
		}
		// 找闭合的 `/`：内容里可以出现 `\/`，所以得看反斜杠的奇偶
		closing := -1
		for i := 1; i < len(line); i++ {
			if line[i] == '/' && !isBackslashEscaped(line, i) {
				closing = i
				break
			}
		}
		if closing < 0 {
			continue
		}
		src := line[1:closing]
		if !strings.HasPrefix(src, "^") {
			continue // 只认锚定的写法，非锚定的没法可靠比对
		}
		out = append(out, strings.ReplaceAll(src, `\/`, "/"))
	}
	return out
}

func isBackslashEscaped(s string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// serverRoutePaths 抠出 main.go 里 `prefix+"…"` 形式的注册路径。
func serverRoutePaths(mainSrc string) []string {
	re := regexp.MustCompile(`mux\.Handle(?:Func)?\(prefix\s*\+\s*"([^"]*)"`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(mainSrc, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}
