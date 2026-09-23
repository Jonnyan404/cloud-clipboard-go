package lib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// ogCard 从落地页 HTML 里抠出要断言的几个字段。
func ogCard(t *testing.T, html string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tag := range []string{"og:title", "og:description", "og:url", "og:image", "og:site_name", "og:type"} {
		needle := `property="` + tag + `" content="`
		idx := strings.Index(html, needle)
		if idx < 0 {
			continue
		}
		rest := html[idx+len(needle):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		out[tag] = rest[:end]
	}
	return out
}

func fetchLanding(t *testing.T, s *ClipboardServer, token string) (int, string, http.Header) {
	t.Helper()
	return fetchLandingPath(t, s, "/s/"+token)
}

// fetchLandingPath 直接按原始路径取落地页 —— 需要带 query（`?q=1`）时用它。
func fetchLandingPath(t *testing.T, s *ClipboardServer, rawPath string) (int, string, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, rawPath, nil)
	req.Host = "clip.example.com"
	// 线上是反向代理在前，https 靠这个头认出来
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	s.handleShareLanding(w, req)
	return w.Code, w.Body.String(), w.Header()
}

// testShell 是一份**构建产物形状**的外壳（只保留判定要用的部分）：
// 深路径下要靠 `<base>` 才解析得对的相对资源、一处 `<title>`、以及要原样保留的应用挂载点。
const testShell = `<!DOCTYPE html>
<html lang="zh" data-build-id="test">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cloud Clipboard</title>
<link rel="icon" href="./favicon.ico">
<script type="module" crossorigin src="./assets/index-abc123.js"></script>
</head>
<body>
<div id="app"></div>
</body>
</html>
`

// attachTestShell 给测试服务器装上前端外壳 —— 正常路径下 `/s/<token>` 就是把卡片注进它里面。
func attachTestShell(t *testing.T, s *ClipboardServer) *ClipboardServer {
	t.Helper()
	s.staticFS = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(testShell)}}
	return s
}

// isTheShell 外壳本身还在吗（应用挂载点 + 构建产物里的相对资源都原样保留）。
func isTheShell(html string) bool {
	return strings.Contains(html, `<div id="app"></div>`) && strings.Contains(html, `src="./assets/index-abc123.js"`)
}

// 文本分享：预览里给出首行摘要 —— 这正是这个页面存在的理由
// （分享页是 hash 路由，抓取程序不执行 JS，只服务 SPA 的话预览里只有域名）。
func TestShareLandingSummarisesTextShare(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.addTextForTest(11, "default", "招行 APP 登录密码\n账号 6225********1234")

	token, _, err := s.issueShareToken("content", "11", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	code, html, header := fetchLanding(t, s, token)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	card := ogCard(t, html)
	if card["og:title"] != "招行 APP 登录密码" {
		t.Fatalf("expected the first line as the preview title, got %q", card["og:title"])
	}
	// 第二行（账号）刻意不进预览：摘要只取首行，别把整段正文搬进第三方缓存
	if strings.Contains(html, "6225") {
		t.Fatal("only the first line may enter the preview cache")
	}
	if !strings.Contains(card["og:description"], "分享的文本") {
		t.Fatalf("unexpected description: %q", card["og:description"])
	}
	if card["og:url"] != "https://clip.example.com/s/"+token {
		t.Fatalf("og:url should be the landing page itself, got %q", card["og:url"])
	}
	// 外壳本身原样保留：真人拿到的就是这一份 HTML，跑起 SPA 后由前端路由 /s/:token 接管。
	if !isTheShell(html) {
		t.Fatalf("the share page must be the SPA shell itself, got: %s", html)
	}
	// `<base>` 必须注入：外壳在 /s/<token> 这种深路径上被打开时，
	// 构建产物里的 `./assets/…` 要靠它才解析得对（前端也从它取路由 base 和 axios baseURL）。
	if !strings.Contains(html, `<base href="/">`) {
		t.Fatalf("<base> must be injected, got: %s", html)
	}
	if !strings.Contains(html, "<title>招行 APP 登录密码</title>") {
		t.Fatalf("the tab title should be the share title too, got: %s", html)
	}
	// 不再有「跳去另一个地址」那一步：同一个分享只有一个地址。
	if strings.Contains(html, "location.replace") || strings.Contains(html, "/#/s?t=") {
		t.Fatal("the share page must not bounce the visitor to a second URL any more")
	}
	if !strings.Contains(header.Get("X-Robots-Tag"), "noindex") {
		t.Fatal("the share landing page must not be indexed by search engines")
	}
}

// 带密码的分享：预览里**绝不放**内容摘要，否则等于把保护绕过去。
func TestShareLandingNeverLeaksPasswordProtectedContent(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.addTextForTest(12, "default", "这是需要密码才能看的秘密")

	token, _, err := s.issueShareToken("content", "12", "default", 600, 0, "pw123456")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	_, html, _ := fetchLanding(t, s, token)
	if strings.Contains(html, "秘密") {
		t.Fatal("a password-protected share must not leak its content into the preview")
	}
	if card := ogCard(t, html); card["og:title"] != "受密码保护的分享" {
		t.Fatalf("unexpected title: %q", card["og:title"])
	}
}

func TestShareLandingFallsBackForInvalidAndMissingContent(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.config.Server.Auth = "secret-pass"

	// 无效 token
	_, html, _ := fetchLanding(t, s, "garbage.token")
	if card := ogCard(t, html); card["og:title"] != "分享链接无效或已过期" {
		t.Fatalf("unexpected title for an invalid token: %q", card["og:title"])
	}
	// 失效的链接也要进分享页：那里才显示得出可读的错误（前端路由拿 token 去问 /share）。
	if !isTheShell(html) {
		t.Fatal("even an invalid link should still serve the share page itself")
	}

	// 已过期
	expired, err := s.signShareClaims(shareClaims{Type: "content", ID: "1", Room: "default", Exp: time.Now().Unix() - 10})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	_, html, _ = fetchLanding(t, s, expired)
	if card := ogCard(t, html); card["og:title"] != "分享链接无效或已过期" {
		t.Fatalf("unexpected title for an expired token: %q", card["og:title"])
	}

	// token 有效但内容已被删除
	token, _, err := s.issueShareToken("content", "999", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	_, html, _ = fetchLanding(t, s, token)
	if card := ogCard(t, html); card["og:title"] != "内容已被删除或过期" {
		t.Fatalf("unexpected title when the content is gone: %q", card["og:title"])
	}
}

func TestShareLandingDescribesFileShare(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.uploadFileMap["uuid-a"] = File{Name: "季度报表.pdf", UUID: "uuid-a", Size: 3 * 1024 * 1024, ExpireTime: time.Now().Unix() + 600}

	token, _, err := s.issueShareToken("file", "uuid-a", "default", 3600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	_, html, _ := fetchLanding(t, s, token)
	card := ogCard(t, html)
	if card["og:title"] != "季度报表.pdf" {
		t.Fatalf("unexpected title: %q", card["og:title"])
	}
	if !strings.Contains(card["og:description"], "3.0MB") {
		t.Fatalf("expected the size in the description, got %q", card["og:description"])
	}
	if card["og:image"] != "" {
		t.Fatalf("a pdf is not previewable as an image, got %q", card["og:image"])
	}
}

// og:image 的两个上限都是硬性的：只有图片、且只在不限次数的分享上给 ——
// 抓取程序抓图会走 /file 的 token 校验，而那里会**消耗一次使用额度**。
func TestShareLandingImagePreviewOnlyForUnlimitedShares(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.uploadFileMap["uuid-img"] = File{Name: "shot.png", UUID: "uuid-img", Size: 2048, ExpireTime: time.Now().Unix() + 600}

	unlimited, _, err := s.issueShareToken("file", "uuid-img", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	_, html, _ := fetchLanding(t, s, unlimited)
	card := ogCard(t, html)
	if !strings.Contains(card["og:image"], "/file/uuid-img/shot.png?t=") {
		t.Fatalf("expected an og:image pointing at the raw file with the same token, got %q", card["og:image"])
	}

	limited, _, err := s.issueShareToken("file", "uuid-img", "default", 600, 3, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	_, html, _ = fetchLanding(t, s, limited)
	if card := ogCard(t, html); card["og:image"] != "" {
		t.Fatal("a use-limited share must not offer og:image: the crawler fetch would burn a use")
	}
}

// 落地页是给抓取程序看的，抓取会反复发生 —— 它绝不能计数，
// 也不能消耗使用次数（真人还没点开）。
func TestShareLandingNeitherCountsNorConsumesUses(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.addTextForTest(21, "default", "只读预览")

	claims, _, err := s.newShareClaims("content", "21", "default", 600, 2, "")
	if err != nil {
		t.Fatalf("newShareClaims failed: %v", err)
	}
	token, err := s.signShareClaims(*claims)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	s.recordShareForTarget(claims, shareTarget{Room: "default", Kind: "text", Text: "只读预览"})

	for i := 0; i < 5; i++ {
		if code, _, _ := fetchLanding(t, s, token); code != http.StatusOK {
			t.Fatalf("landing fetch %d failed with %d", i, code)
		}
	}

	rec, ok := s.lookupShareRecord(claims.JTI)
	if !ok {
		t.Fatal("expected the share to be logged")
	}
	if rec.Visits != 0 || rec.Scans != 0 {
		t.Fatalf("crawler hits must not be counted as visits: %+v", rec)
	}
	// 两次额度都还在
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/content/21?t="+token, nil)
		if !s.validateShareToken(req, "content", "21", "default") {
			t.Fatalf("use %d should still be available after landing page fetches", i)
		}
	}
}

func TestShareLandingHonoursServerPrefix(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.config.Server.Prefix = "/clip"

	token, _, err := s.issueShareToken("content", "1", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/clip/s/"+token, nil)
	req.Host = "clip.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	s.handleShareLanding(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	card := ogCard(t, body)
	if card["og:url"] != "https://clip.example.com/clip/s/"+token {
		t.Fatalf("prefix must be part of the canonical url, got %q", card["og:url"])
	}
	// `<base>` 也要带 prefix：前端的路由 base 与 axios baseURL 都从它来。
	if !strings.Contains(body, `<base href="/clip/">`) {
		t.Fatalf("the injected <base> must carry the prefix, got: %s", body)
	}
}

// 扫码那条地址（`?q=1`，见前端 withShareQrFlag）与普通地址是**同一个页面**：标记只是给前端
// 上报用的 query。服务端不该把它写进摘要，也不该让同一张卡出现两个身份。
func TestShareLandingKeepsTheCanonicalStableWhenScanned(t *testing.T) {
	s := attachTestShell(t, newShareLogTestServer(t))
	s.addTextForTest(31, "default", "扫码进来的那条")

	token, _, err := s.issueShareToken("content", "31", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	_, html, _ := fetchLandingPath(t, s, "/s/"+token+"?q=1")
	if !isTheShell(html) {
		t.Fatalf("the scanned link must serve the same page, got: %s", html)
	}
	// og:url 是这条分享的**身份**：同一张卡不能因为来源标记而变一个地址。
	if card := ogCard(t, html); card["og:url"] != "https://clip.example.com/s/"+token {
		t.Fatalf("og:url must stay canonical without the qr flag, got %q", card["og:url"])
	}
	if !strings.Contains(html, `<base href="/">`) {
		t.Fatal("the scanned link still needs the injected <base>")
	}
}

// 没有前端外壳（没嵌入、也没配外部目录）时退化成一张通用卡片：
// 抓取程序要的就是那几行标签，而真人那边本来也没有前端可以看。
func TestShareLandingFallsBackToACardWithoutAShell(t *testing.T) {
	s := newShareLogTestServer(t)
	s.addTextForTest(41, "default", "没有前端时也能预览")

	token, _, err := s.issueShareToken("content", "41", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	code, html, header := fetchLanding(t, s, token)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if card := ogCard(t, html); card["og:title"] != "没有前端时也能预览" {
		t.Fatalf("the fallback card must still describe the share, got %q", card["og:title"])
	}
	if !strings.Contains(header.Get("X-Robots-Tag"), "noindex") {
		t.Fatal("the fallback card must stay noindex as well")
	}
}

// 注入的契约：外壳形状不对（缺 `<head>` 或 `</head>`）时**不注入**，交给调用方回落 ——
// 宁可少一张预览卡，也不要吐出一份半截 HTML。
func TestInjectShellTagsRefusesABrokenShell(t *testing.T) {
	for name, shell := range map[string]string{
		"no head":            `<html><body><div id="app"></div></body></html>`,
		"no head end":        `<html><head><title>x</title><body>y</body></html>`,
		"head end too early": `<html></head><head><title>x</title></head></html>`,
	} {
		if _, ok := injectShellTags(shell, "/", "T", "<meta name=\"x\">"); ok {
			t.Fatalf("%s: a shell we cannot inject into must be reported as unusable", name)
		}
	}

	page, ok := injectShellTags(testShell, "/clip/", "标题", "<meta property=\"og:title\" content=\"标题\">")
	if !ok {
		t.Fatal("a normal shell must be injectable")
	}
	// `<base>` 必须排在任何相对地址之前
	baseIdx := strings.Index(page, `<base href="/clip/">`)
	headIdx := strings.Index(page, "<head>")
	assetIdx := strings.Index(page, `src="./assets/index-abc123.js"`)
	if baseIdx < headIdx || baseIdx > assetIdx {
		t.Fatalf("the injected <base> must come before any relative url: %s", page)
	}
	if !strings.Contains(page, "<title>标题</title>") {
		t.Fatalf("the shell title must be replaced, got: %s", page)
	}
	if !strings.Contains(page, `<meta property="og:title" content="标题">`) {
		t.Fatalf("the extra head tags must be injected, got: %s", page)
	}
}

func TestShareSummaryHelpers(t *testing.T) {
	if got := firstSummaryLine("\n\n  # 标题行  \n第二行", 80); got != "标题行" {
		t.Fatalf("expected the first non-empty line without markup, got %q", got)
	}
	if got := firstSummaryLine("一二三四五", 3); got != "一二三…" {
		t.Fatalf("expected rune-safe truncation, got %q", got)
	}
	if got := firstSummaryLine("", 80); got != "" {
		t.Fatalf("empty content has no summary, got %q", got)
	}
	if got := formatShareSize(1024); got != "1.0KB" {
		t.Fatalf("unexpected size format: %q", got)
	}
	if got := formatShareSize(512); got != "512B" {
		t.Fatalf("unexpected size format: %q", got)
	}
	if got := formatShareSize(0); got != "" {
		t.Fatalf("unknown size should stay silent, got %q", got)
	}
	for name, want := range map[string]bool{
		"a.png": true, "a.JPEG": true, "a.webp": true, "a.pdf": false, "noext": false, "a.png.txt": false,
	} {
		if got := isPreviewableImageName(name); got != want {
			t.Fatalf("isPreviewableImageName(%q) = %v, want %v", name, got, want)
		}
	}
	if note := shareExpiryNote(time.Now().Unix() + 30*60); !strings.Contains(note, "分钟") {
		t.Fatalf("unexpected expiry note: %q", note)
	}
	if note := shareExpiryNote(time.Now().Unix() + 2*3600); !strings.Contains(note, "小时") {
		t.Fatalf("unexpected expiry note: %q", note)
	}
	if note := shareExpiryNote(0); note != "" {
		t.Fatalf("a token without expiry has nothing to say, got %q", note)
	}
}

// 落地页路径解析：prefix、尾斜杠、URL 转义都不能把 token 弄丢。
func TestShareTokenFromLandingPath(t *testing.T) {
	cases := map[string]string{
		"/s/abc.def":         "abc.def",
		"/clip/s/abc":        "abc",
		"/s/abc/":            "abc",
		"/clip/s/a%2Db":      "a-b",
		"/s/":                "",
		"/other/abc":         "",
		"/s":                 "",
		"/clip/s/A-B_c.123=": "A-B_c.123=",
	}
	for path, want := range cases {
		if got := shareTokenFromLandingPath(path); got != want {
			t.Fatalf("shareTokenFromLandingPath(%q) = %q, want %q", path, got, want)
		}
	}
}
