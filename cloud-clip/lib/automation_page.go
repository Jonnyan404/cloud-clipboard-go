package lib

import (
	_ "embed" // automation_page.html
	"html/template"
	"net/http"
	"strings"
)

/**
*** FILE: automation_page.go
***   /automation —— 定时自动化的管理页。
**/

// 为什么是**服务端渲染的独立页面**，而不是 SPA 里的一个面板：
//  1. 这个功能的使用频率极低（配一次用很久），却要在整个生命周期里最可靠 ——
//     把它挂在 SPA 的构建产物里，一次前端打包出问题就连「改一下时间」都做不到；
//  2. 前端资源整条链路挂掉时它仍然可用（页面里的样式和脚本全内联，不依赖 static）；
//  3. 落地成本最低：不动 web-vue3 的构建，也就不动那条已经被九种模式绑住的链路。
//
// 页面本身**不含任何权限逻辑** —— 能不能建、能建几条、能改哪些，全部由 /tasks 决定。
//
// 多语言（zh / zh-TW / en / ja）：
//   - 动作名、分组名的译文**从 SPA 的 locales 里抄**（key 完全相同），不另造一套；
//     服务端只下发 i18n key（见 render_actions.go），译文只留一份。
//   - 语言选择与 SPA **共用 localStorage['locale']**：在任一边切了语言，另一边也跟着变，
//     不会出现「主界面英文、这个页面中文」这种割裂。
//   - 语言优先级：?lang= > localStorage['locale'] > navigator.language（规则与 SPA 一致）。
//
// ⚠️ 分隔符换成 `[[ ]]`：这个页面里到处是 `{{date}}` 这样的变量示例文案，
// 用默认的 `{{ }}` 会被 Go 模板引擎当成自己的指令（编译期就 panic: function "date" not defined）。
// 也正因如此，**这个 raw string 里不能出现反引号** —— 会直接把它截断。
//
//go:embed automation_page.html
var automationPageHTML string

// 模板放在**独立文件**里（automation_page.html），不再用 Go 的 raw string。
//
// 为什么换掉：raw string 里**不能出现反引号**，而这个页面到处是
// 「*/30 9-18 * * 1-5」这样的示例文案，写注释时手滑一次就截断字符串，
// 报一堆看起来毫不相关的语法错 —— 这件事反复发生过（前后四次），
// 而每次排查都要几分钟。搬到 .html 之后，那个陷阱从结构上不存在了，
// 顺带还能拿到 HTML 的语法高亮。
var automationPageTemplate = template.Must(
	template.New("automation-page").Delims("[[", "]]").Parse(automationPageHTML))

func (s *ClipboardServer) handleAutomationPage(w http.ResponseWriter, r *http.Request) {
	room := "default"
	if _, hasRoom := r.URL.Query()["room"]; hasRoom {
		room = normalizeRoomName(r.URL.Query().Get("room"))
	}

	data := struct {
		Prefix string
		Room   string
	}{
		Prefix: strings.TrimSpace(s.config.Server.Prefix),
		Room:   room,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if err := automationPageTemplate.Execute(w, data); err != nil {
		s.logger.Printf("错误: 渲染自动化管理页失败: %v", err)
	}
}
