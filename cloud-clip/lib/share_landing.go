package lib

import (
	"html"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GET /s/<token> —— 分享链接的**唯一地址**：一份注入了 OG 卡片的 SPA 外壳。
//
// 为什么必须有服务端这一步：分享页是 SPA，而抓取程序**不执行 JS**；浏览器也不会把 `#` 之后
// 的部分发给服务器。所以 token 必须在**路径**里（不能是 `/#/s?t=…`），OG 标签必须在服务端就
// 写进 HTML —— 否则微信 / Telegram / Slack 抓到的只是一个空壳，预览里只有域名。
//
// 真人拿到的也是这一份 HTML：跑起 SPA 后由前端路由 `/s/:token` 接管，看到的就是分享页本身。
// 也就是说**抓取程序和真人是同一个地址**，没有第二跳、也没有第二个身份（2026-09 之前不是这样：
// 那时前端走 hash 路由，落地页把真人 `location.replace` 到 `/#/s?t=…`，同一个分享有两个地址）。
//
// ⚠️ 这条路径上的摘要**公开可抓、且会被第三方缓存**：贴进微信 / Telegram / Slack，摘要会出现
// 在预览里（群里所有人都看得到），而且平台侧的缓存**删不掉** —— 之后过期、删内容、加密码都不
// 影响对方已经抓到的副本。所以：
//  1. 带密码的分享**绝不输出内容摘要**（只说「需要密码」）；
//  2. 失效 / 已删除的分享只输出通用卡片；
//  3. 页面带 noindex，别让搜索引擎把分享页当正文收录；
//  4. 这里**不计数**：抓取程序会反复访问，统计只认前端分享页的上报（见 handleShareVisit）。
//
// 这个页面**不消耗使用次数**：只读内容元信息，取正文仍走 /content、/file。
//
// 没有前端外壳可用时（没嵌入、也没配外部目录），退化成一张通用卡片（见 shareLandingTemplate）：
// 抓取程序要的就是标签，而真人那边本来也没有前端可以看。
const (
	shareSiteName        = "Cloud Clipboard"
	landingTitleLimit    = 80
	landingDescLimit     = 160
	landingImageMaxBytes = 5 * 1024 * 1024
)

// shareCard 一张卡，字段一一对应 og:* / twitter:* 标签。
type shareCard struct {
	SiteName    string
	Title       string
	Description string
	Image       string // 留空则平台用站点图标 / 占位图
	Canonical   string // og:url：分享地址本身（就是当前这个地址）
}

// shareLandingTemplate 是「没有前端外壳」时的兜底页：只有一张卡。
// 正常路径**不用它渲染** —— 那一条是把下面这些标签注入进 SPA 外壳（见文件头）。
var shareLandingTemplate = template.Must(template.New("share-card").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="robots" content="noindex, nofollow">
<meta name="description" content="{{.Description}}">
<meta property="og:type" content="website">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.Canonical}}">
{{if .Image}}<meta property="og:image" content="{{.Image}}">{{end}}
<meta name="twitter:card" content="{{if .Image}}summary_large_image{{else}}summary{{end}}">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .Image}}<meta name="twitter:image" content="{{.Image}}">{{end}}
<style>
:root { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
  font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans SC", sans-serif;
  background: #f6f7f9; color: #1f2430; }
@media (prefers-color-scheme: dark) { body { background: #16181d; color: #e9ecf2; } }
main { max-width: 30rem; padding: 2rem 1.5rem; text-align: center; }
h1 { font-size: 1.1rem; margin: 0 0 .5rem; }
p { margin: 0; opacity: .75; word-break: break-word; }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
</main>
</body>
</html>
`))

// shareTokenFromLandingPath 从 /<prefix>/s/<token> 里取出 token。
// 用 base64url 字符集，理论上可以含 `.`；分隔符是最后一个 "/s/"（token 不含 "/"）。
func shareTokenFromLandingPath(path string) string {
	idx := strings.LastIndex(path, "/s/")
	if idx < 0 {
		return ""
	}
	raw := strings.Trim(path[idx+len("/s/"):], "/")
	if raw == "" {
		return ""
	}
	if unescaped, err := url.PathUnescape(raw); err == nil {
		return strings.TrimSpace(unescaped)
	}
	return strings.TrimSpace(raw)
}

func (s *ClipboardServer) handleShareLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed", "仅允许 GET 请求")
		return
	}

	token := shareTokenFromLandingPath(r.URL.Path)
	card := shareCard{
		SiteName:  shareSiteName,
		Canonical: s.buildLandingURL(r, token),
	}
	card.Title = "分享链接无效或已过期"
	card.Description = "这条分享可能已过期、次数用尽，或者链接不完整。"

	claims, ok := s.parseShareToken(token)
	if ok && (claims.Exp == 0 || claims.Exp > time.Now().Unix()) {
		switch {
		case claims.PwdHash != "":
			// 有密码就到此为止：预览里放内容摘要等于把保护绕过去。
			card.Title = "受密码保护的分享"
			card.Description = "打开后需要输入分享密码才能查看内容。"
		default:
			s.fillShareCardFromContent(r, &card, claims, token)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// noindex：分享页不该被搜索引擎收录。抓取程序不看这个头，所以 OG 仍然有效。
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	// private + must-revalidate：别让中间缓存/CDN 长期留着带摘要的这份 HTML。
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	// 地址里带 token：不让它作为 Referer 流到第三方；顺带挡住 MIME 嗅探。
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// 正常路径：把卡片注入 SPA 外壳，真人跑起前端路由后看到的就是分享页本身。
	if shell, ok := s.readShellHTML(); ok {
		if page, ok := injectShellTags(shell, s.shellBaseHref(), card.Title, shareCardHeadTags(card)); ok {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, page)
			return
		}
	}

	// 没有外壳可用（没嵌入、也没配外部目录）：只给一张卡。
	w.WriteHeader(http.StatusOK)
	_ = shareLandingTemplate.Execute(w, card)
}

// shareCardHeadTags 拼要写进外壳 `<head>` 的那几行：noindex + description + og:* + twitter:*。
//
// 值一律转义：标题来自内容首行、文件名，description 里也可能带用户内容 —— 直接拼进属性就是
// 一处 XSS。`og:image` 只在有图时出现，`twitter:card` 跟着它选 `summary_large_image`。
func shareCardHeadTags(card shareCard) string {
	title := html.EscapeString(card.Title)
	description := html.EscapeString(card.Description)
	canonical := html.EscapeString(card.Canonical)

	var b strings.Builder
	b.WriteString("<meta name=\"robots\" content=\"noindex, nofollow\">\n")
	b.WriteString("<meta name=\"description\" content=\"" + description + "\">\n")
	b.WriteString("<meta property=\"og:type\" content=\"website\">\n")
	b.WriteString("<meta property=\"og:site_name\" content=\"" + html.EscapeString(card.SiteName) + "\">\n")
	b.WriteString("<meta property=\"og:title\" content=\"" + title + "\">\n")
	b.WriteString("<meta property=\"og:description\" content=\"" + description + "\">\n")
	b.WriteString("<meta property=\"og:url\" content=\"" + canonical + "\">\n")
	if card.Image != "" {
		image := html.EscapeString(card.Image)
		b.WriteString("<meta property=\"og:image\" content=\"" + image + "\">\n")
		b.WriteString("<meta name=\"twitter:image\" content=\"" + image + "\">\n")
	}
	twitterCard := "summary"
	if card.Image != "" {
		twitterCard = "summary_large_image"
	}
	b.WriteString("<meta name=\"twitter:card\" content=\"" + twitterCard + "\">\n")
	b.WriteString("<meta name=\"twitter:title\" content=\"" + title + "\">\n")
	b.WriteString("<meta name=\"twitter:description\" content=\"" + description + "\">")
	return b.String()
}

// fillShareCardFromContent 内容还在的情况下，把摘要填进卡片。
// 取不到内容时保持调用方设好的通用文案。
func (s *ClipboardServer) fillShareCardFromContent(r *http.Request, card *shareCard, claims *shareClaims, token string) {
	switch claims.Type {
	case "content":
		contentID, err := strconv.Atoi(strings.TrimSpace(claims.ID))
		if err != nil {
			return
		}
		target, found := s.findShareTarget(contentID, claims.Room, true)
		if !found {
			card.Title = "内容已被删除或过期"
			card.Description = "这条分享指向的内容已不在服务器上。"
			return
		}
		switch target.Kind {
		case "file":
			card.Title = truncateRunes(target.FileName, landingTitleLimit)
			card.Description = joinShareMeta("文件", formatShareSize(target.FileSize), shareExpiryNote(claims.Exp))
			s.maybeSetShareImage(r, card, target.FileUUID, target.FileName, target.FileSize, claims, token)
		default:
			summary := firstSummaryLine(target.Text, landingTitleLimit)
			if summary == "" {
				card.Title = "有人分享了一段文本"
				card.Description = "通过 " + shareSiteName + " 分享，打开即可查看。"
				return
			}
			card.Title = summary
			card.Description = joinShareMeta("分享的文本", shareExpiryNote(claims.Exp))
		}
	case "file":
		s.runMutex.Lock()
		fileInfo, exists := s.uploadFileMap[claims.ID]
		s.runMutex.Unlock()
		if !exists {
			card.Title = "内容已被删除或过期"
			card.Description = "这条分享指向的文件已不在服务器上。"
			return
		}
		card.Title = truncateRunes(fileInfo.Name, landingTitleLimit)
		card.Description = joinShareMeta("文件", formatShareSize(fileInfo.Size), shareExpiryNote(claims.Exp))
		s.maybeSetShareImage(r, card, claims.ID, fileInfo.Name, fileInfo.Size, claims, token)
	}
}

// maybeSetShareImage 给图片分享补 og:image，让预览直接显示缩略图。
//
// 两个上限是硬性的：
//   - 只在**不限次数**的分享上启用 —— 抓取程序抓图会走 /file 的 token 校验，
//     而那里会**消耗一次使用额度**（validateShareToken），额满之后真人点开就成了
//     「已被使用完」，预览把链接用掉是绝对不能接受的；
//   - 只给图片、且不超过 landingImageMaxBytes，避免把大文件塞进对方预览。
func (s *ClipboardServer) maybeSetShareImage(r *http.Request, card *shareCard, fileUUID, fileName string, fileSize int64, claims *shareClaims, token string) {
	if claims.MaxUses > 0 {
		return
	}
	if fileSize > 0 && fileSize > landingImageMaxBytes {
		return
	}
	if !isPreviewableImageName(fileName) {
		return
	}
	if strings.TrimSpace(fileUUID) == "" {
		return
	}
	name := fileName
	if name == "" {
		name = "image"
	}
	card.Image = s.buildAbsoluteURL(r, "/file/"+fileUUID+"/"+url.PathEscape(name), url.Values{shareTokenQueryKey: []string{token}})
}

// buildLandingURL 拼分享地址 —— 分享链接就是这个地址（抓取程序与真人共用，见文件头）。
// 路径里的 token 用 PathEscape：base64url 里没有 `/`，但 `%` 之类的边界还是交给它处理。
func (s *ClipboardServer) buildLandingURL(r *http.Request, token string) string {
	return s.buildAbsoluteURL(r, "/s/"+url.PathEscape(token), nil)
}

// firstSummaryLine 取文本的第一行有内容的行，压平空白后截断 —— 卡片只显示两三行。
func firstSummaryLine(text string, limit int) string {
	for _, line := range strings.Split(text, "\n") {
		clean := strings.Join(strings.Fields(line), " ")
		clean = strings.Trim(clean, "#>*-·|")
		clean = strings.TrimSpace(clean)
		if clean != "" {
			return truncateRunes(clean, limit)
		}
	}
	return ""
}

func truncateRunes(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func formatShareSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(bytes, 10) + units[unit]
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + units[unit]
}

// shareExpiryNote 卡片上那句「剩余多久」。过期时间缺失（无 TTL 的旧 token）就闭嘴。
func shareExpiryNote(exp int64) string {
	if exp <= 0 {
		return ""
	}
	remaining := exp - time.Now().Unix()
	if remaining <= 0 {
		return "已过期"
	}
	if remaining < 3600 {
		minutes := (remaining + 59) / 60
		return strconv.FormatInt(minutes, 10) + " 分钟内有效"
	}
	return strconv.FormatInt((remaining+3599)/3600, 10) + " 小时内有效"
}

func joinShareMeta(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return "通过 " + shareSiteName + " 分享，打开即可查看。"
	}
	return strings.Join(kept, " · ")
}

func isPreviewableImageName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	idx := strings.LastIndex(lower, ".")
	if idx < 0 {
		return false
	}
	switch lower[idx+1:] {
	case "png", "jpg", "jpeg", "gif", "webp", "avif", "bmp":
		return true
	}
	return false
}
