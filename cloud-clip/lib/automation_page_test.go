package lib

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 渲染一次页面拿到最终 HTML（`[[.Prefix]]` 之类的占位符会被替换掉）。
func renderAutomationPage(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	data := struct {
		Prefix string
		Room   string
	}{Prefix: "", Room: "default"}
	if err := automationPageTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("渲染自动化管理页失败: %v", err)
	}
	return buf.String()
}

// countTopLevelItems 数一个 JS 数组字面量 `[a, b, c]` 里有几项。
//
// 不能简单按逗号 split：文案里有中文逗号、也有英文逗号（"Next three:" 之类），
// 必须跳过字符串内部。还要处理 `\n` 这类转义，否则引号配对会错。
func countTopLevelItems(literal string) int {
	literal = strings.TrimSpace(literal)
	literal = strings.TrimPrefix(literal, "[")
	literal = strings.TrimSuffix(literal, "]")

	count := 0
	hasContent := false
	inString := false
	escaped := false
	depth := 0

	for _, r := range literal {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
			hasContent = true
		case r == '[' || r == '{':
			depth++
			hasContent = true
		case r == ']' || r == '}':
			depth--
			hasContent = true
		case r == ',' && depth == 0:
			count++
		default:
			if !isJSSpace(r) {
				hasContent = true
			}
		}
	}
	if !hasContent {
		return 0
	}
	return count + 1
}

func isJSSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

var jsEntryRe = regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9_]*):\s*(\[.*?\]),?\s*$`)

// ── 动作文案：服务端只给 key，所以漏译只能在运行时发现 —— 这里提前挡住 ──

// 服务端动作的每个 i18n key 都必须在**四份 locale 里都查到非空译文**。
//
// 这条测试守的就是用户提的那个问题本身：key 拼错或忘了翻译，界面上会直接显示
// `text.trimLines` 这种 id。服务端只下发 key（理由见 render_actions.go），
// 所以漏译编译期发现不了，只能靠这里。
func TestRenderActionI18nKeysExistInAllLocales(t *testing.T) {
	dir := filepath.Join("..", "..", "web-vue3", "src", "locales")
	keys := ServerRenderActionI18nKeys()
	if len(keys) < 10 {
		t.Fatalf("i18n key 数量异常（%d），契约检查失去意义", len(keys))
	}

	for _, code := range []string{"zh", "zh-TW", "en", "ja"} {
		data, err := os.ReadFile(filepath.Join(dir, code+".json"))
		if err != nil {
			t.Skipf("读不到 %s.json，跳过契约检查: %v", code, err)
		}
		var messages map[string]interface{}
		if err := json.Unmarshal(data, &messages); err != nil {
			t.Fatalf("解析 %s.json 失败: %v", code, err)
		}
		for _, key := range keys {
			value, ok := messages[key]
			if !ok {
				t.Errorf("locale %s 缺少 %s —— 界面上会显示成动作 id，用户看不懂", code, key)
				continue
			}
			if s, _ := value.(string); strings.TrimSpace(s) == "" {
				t.Errorf("locale %s 的 %s 是空字符串", code, key)
			}
		}
	}
}

// 动作清单必须带上文案 key，否则前端只能退回显示 id —— 那正是要修的问题。
func TestRenderActionListCarriesI18nKeys(t *testing.T) {
	list := ServerRenderActionList()
	if len(list) == 0 {
		t.Fatal("动作清单为空")
	}
	for _, item := range list {
		for _, field := range []string{"id", "group", "groupKey", "key"} {
			if s, _ := item[field].(string); strings.TrimSpace(s) == "" {
				t.Errorf("动作 %s 的 %s 为空: %+v", item["id"], field, item)
			}
		}
		// 带参数的动作：每个参数都要有 key 和 labelKey；
		// 下拉参数还要每个选项都有 value + labelKey（少了 labelKey 界面上会出现一排裸 key）。
		if raw, ok := item["params"]; ok {
			params, isList := raw.([]map[string]interface{})
			if !isList || len(params) == 0 {
				t.Errorf("动作 %s 的 params 形态不对: %+v", item["id"], raw)
				continue
			}
			for _, p := range params {
				key, _ := p["key"].(string)
				labelKey, _ := p["labelKey"].(string)
				if strings.TrimSpace(key) == "" || strings.TrimSpace(labelKey) == "" {
					t.Errorf("动作 %s 的参数缺 key / labelKey: %+v", item["id"], p)
				}
				if rawOptions, ok := p["options"]; ok {
					options, isOptions := rawOptions.([]map[string]string)
					if !isOptions || len(options) == 0 {
						t.Errorf("动作 %s 的参数 %s 的 options 形态不对: %+v", item["id"], key, rawOptions)
						continue
					}
					for _, o := range options {
						if strings.TrimSpace(o["value"]) == "" || strings.TrimSpace(o["labelKey"]) == "" {
							t.Errorf("动作 %s 的参数 %s 有选项缺 value / labelKey: %+v", item["id"], key, o)
						}
					}
				}
			}
		}
	}
}

// 每个**服务端动作**用到的 i18n key（动作名 / 分组名 / 参数标签）都必须在管理页的 MSG 表里。
//
// ⚠️ 这条测试补的是一个**盲区**：页面脚本里的 `t("...")` 字面量由
// TestAutomationPageScriptI18nKeysHaveMessages 管着，但动作名 / 分组名 / 参数标签是
// **用字段间接引用**的（`labelFor(a.key, a.id)`），那条测试扫不到 ——
// 于是「新下沉一个动作、忘了抄译文」会静默通过，界面上只冒出一个裸 key。
// 2026-09-24 一口气漏了 19 个动作名 + 2 个分组名（`vartask` 也是同一类漏法）。
// 每个**服务端动作**用到的 i18n key（动作名 / 分组名 / 参数标签）都必须在管理页的
// `ACTION_LABELS` 表里有条目。
//
// ⚠️ 页面里有**两张**文案表，别搞混：
//   - `MSG`           —— 页面自己的 UI 文案（`t("...")` 取的那张）；
//   - `ACTION_LABELS` —— 动作名 / 分组名（`labelFor(...)` 取的那张，key 与 SPA locales 相同）。
//
// ⚠️ 这条测试补的是一个**盲区**：`t("...")` 字面量由
// TestAutomationPageScriptI18nKeysHaveMessages 管着，但动作名 / 分组名 / 参数标签是
// **用字段间接引用**的（`labelFor(a.key, a.id)`），那张测试管不到 ——
// 于是「新下沉一个动作、忘了抄译文」会静默通过，界面上只冒出一个裸 key。
// 2026-09-24 一口气漏了 19 个动作名 + 2 个分组名（`vartask` 也是同一类漏法，
// 它漏在 MSG —— 变量名同样是间接引用的）。
func TestAutomationPageMessagesCoverActionKeys(t *testing.T) {
	page := renderAutomationPage(t)
	labelBlock := jsObjectBlock(t, page, "var ACTION_LABELS = {", "\n  };")

	have := map[string]bool{}
	// 按行切分而不是正则：表里每一行是 `    key: [四种语言],`，
	// 这样解析不依赖正则引擎对 `^` / 量词的处理，也不会被注释行干扰。
	for _, line := range strings.Split(labelBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		idx := strings.Index(trimmed, ":")
		if idx <= 0 {
			continue
		}
		// 只认「值以 `[` 开头」的行 —— 表里每条都是 `key: ["…", …]`
		if !strings.HasPrefix(strings.TrimSpace(trimmed[idx+1:]), "[") {
			continue
		}
		have[strings.TrimSpace(trimmed[:idx])] = true
	}
	if len(have) == 0 {
		t.Fatal("没能从 ACTION_LABELS 表里解析出任何 key —— 是这条测试自己坏了")
	}

	for _, key := range ServerRenderActionI18nKeys() {
		if !have[key] {
			t.Errorf("动作相关的 i18n key %q 在管理页 ACTION_LABELS 表里没有条目 —— 界面上会显示一个裸 key", key)
		}
	}
}

// ── 页面文案表 ──────────────────────────────────────────────────────

// 文案表里**每条都必须有 4 种语言**。
//
// 漏一种不会报错，只会在切到那个语言时静默回退到中文 —— 用户看到的是
// 「切了语言但这页还是中文」，而且只有恰好懂那门语言的人才会发现。
func TestAutomationPageMessagesCoverAllLanguages(t *testing.T) {
	page := renderAutomationPage(t)

	for _, block := range []struct {
		name, start, end string
		minEntries       int
	}{
		{"MSG", "var MSG = {", "\n  };", 50},
		{"ACTION_LABELS", "var ACTION_LABELS = {", "\n  };", 15},
	} {
		body := jsObjectBlock(t, page, block.start, block.end)
		rows := jsEntryRe.FindAllStringSubmatch(body, -1)
		if len(rows) < block.minEntries {
			t.Fatalf("%s 只解析出 %d 条（至少应有 %d 条），解析大概坏了", block.name, len(rows), block.minEntries)
		}
		for _, m := range rows {
			if n := countTopLevelItems(m[2]); n != len(pageLanguages()) {
				t.Errorf("%s 的 %s 有 %d 种语言，应为 %d 种：%s",
					block.name, m[1], n, len(pageLanguages()), m[2])
			}
		}
	}

	// DAYS 是嵌套数组，形状与上面两个不同，单独数：4 组 × 7 天
	daysBlock := jsObjectBlock(t, page, "var DAYS = [", "\n  ];")
	groups := regexp.MustCompile(`(?m)^\s{4}\[.*\]\s*,?\s*$`).FindAllString(daysBlock, -1)
	if len(groups) != len(pageLanguages()) {
		t.Fatalf("DAYS 有 %d 组，应为 %d 组", len(groups), len(pageLanguages()))
	}
	for i, line := range groups {
		if n := countTopLevelItems(strings.TrimSuffix(strings.TrimSpace(line), ",")); n != 7 {
			t.Errorf("DAYS 第 %d 组有 %d 天，应为 7 天", i+1, n)
		}
	}
}

// 页面上每一个 data-i18n / data-i18n-ph 的属性值都必须在文案表里有对应条目。
//
// 这条守的是另一类漏译：加了属性、忘了文案 —— 界面上会直接显示 `fieldName`
// 这种 key 本身。写错一个字母也是一样的后果。
func TestAutomationPageI18nAttributesHaveMessages(t *testing.T) {
	page := renderAutomationPage(t)
	msgBlock := jsObjectBlock(t, page, "var MSG = {", "\n  };")

	defined := map[string]bool{}
	for _, m := range jsEntryRe.FindAllStringSubmatch(msgBlock, -1) {
		defined[m[1]] = true
	}

	used := map[string]bool{}
	for _, attr := range []string{"data-i18n", "data-i18n-ph"} {
		re := regexp.MustCompile(attr + `="([A-Za-z][A-Za-z0-9_]*)"`)
		for _, m := range re.FindAllStringSubmatch(page, -1) {
			used[m[1]] = true
		}
	}
	if len(used) < 15 {
		t.Fatalf("只找到 %d 个 i18n 属性，解析大概坏了", len(used))
	}
	for key := range used {
		if !defined[key] {
			t.Errorf("页面用了 data-i18n=%q，但文案表里没有这一条 —— 界面上会显示 key 本身", key)
		}
	}
}

// 脚本里每一个字面量 `t("key")` 都必须在文案表里有条目。
//
// 守的是和上面那条同一类漏译，只是入口不同：`t()` 取不到条目时会**原样返回 key**
// （见页面上 t 的实现），所以写错一个字母的后果就是界面上明晃晃写着 `runAtEmpty`。
// data-i18n 属性那条管不到 JS 里的调用点 —— 只在 JS 里用到的文案（如新加的
// `runAtEmpty` / `runAtPastWarn`）全靠这一条看着。
func TestAutomationPageScriptI18nKeysHaveMessages(t *testing.T) {
	page := renderAutomationPage(t)

	defined := map[string]bool{}
	for _, block := range []string{"var MSG = {", "var ACTION_LABELS = {"} {
		for _, m := range jsEntryRe.FindAllStringSubmatch(jsObjectBlock(t, page, block, "\n  };"), -1) {
			defined[m[1]] = true
		}
	}
	if len(defined) < 50 {
		t.Fatalf("只解析到 %d 条文案，解析大概坏了", len(defined))
	}

	used := map[string]bool{}
	// `(?:^|[^\w$.])` 这个前缀是必须的：只写 `t\("` 会把
	// `new URLSearchParams(location.search).get("lang")` 里的 `t("lang")` 也算成调用点
	// （`get` 结尾的那个 t）。前面是标识符字符或 `.` 的一律不算。
	for _, m := range regexp.MustCompile(`(?:^|[^\w$.])t\("([A-Za-z][A-Za-z0-9_]*)"`).FindAllStringSubmatch(page, -1) {
		used[m[1]] = true
	}
	if len(used) < 30 {
		t.Fatalf("只找到 %d 个字面量 t() 调用，解析大概坏了", len(used))
	}
	for key := range used {
		if !defined[key] {
			t.Errorf("脚本里用了 t(%q)，但文案表里没有这一条 —— 界面上会直接显示这个 key", key)
		}
	}
}

// 动作名不能是动作 id 的形状：如果哪天有人把 label 写成 id，界面上小白照样看不懂。
func TestAutomationPageActionLabelsAreHumanReadable(t *testing.T) {
	page := renderAutomationPage(t)
	block := jsObjectBlock(t, page, "var ACTION_LABELS = {", "\n  };")
	rows := jsEntryRe.FindAllStringSubmatch(block, -1)
	if len(rows) == 0 {
		t.Fatal("没解析到动作文案")
	}
	for _, m := range rows {
		// 每种语言的第一个值（中文）不应含有 `.`（动作 id 的分隔符）
		first := strings.SplitN(m[2], ",", 2)[0]
		if strings.Contains(first, ".") {
			t.Errorf("%s 的中文文案像是动作 id 而不是人话: %s", m[1], first)
		}
	}
}

func pageLanguages() []string {
	return []string{"zh", "zh-TW", "en", "ja"}
}

// 凭据存储的契约：页面用的 sessionStorage 键名与用途必须和 SPA 完全一致。
//
// 这是「同一标签页内免二次登录」的**唯一**依据 —— 两个页面之间没有别的约定，
// 就靠这一个键、一种结构。谁单方面改了名字，另一边会**静默地**重新要求登录
// （不报错、不崩，只是「为什么又要我输密码」），所以必须有测试盯着。
//
// 同理也盯住「存的是令牌不是密码」：如果哪天有人把 sessionStorage 换回 localStorage
// 或把令牌换回密码，这里会红。
func TestAutomationPageSharesSpaAuthStorageContract(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web-vue3", "src", "store", "websocket.js"))
	if err != nil {
		t.Skipf("读不到 SPA 的 websocket.js，跳过契约检查: %v", err)
	}

	consts := map[string]string{}
	re := regexp.MustCompile(`(?m)^const (ROOM_AUTH_CACHE_KEY|DEFAULT_ROOM_KEY|GLOBAL_ROOM_KEY) = '([^']+)';`)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		consts[m[1]] = m[2]
	}
	for _, name := range []string{"ROOM_AUTH_CACHE_KEY", "DEFAULT_ROOM_KEY", "GLOBAL_ROOM_KEY"} {
		if consts[name] == "" {
			t.Fatalf("在 SPA 的 websocket.js 里找不到 %s —— 契约测试自身失效了", name)
		}
	}

	page := renderAutomationPage(t)
	for name, value := range consts {
		want := `var ` + name + ` = "` + value + `";`
		if !strings.Contains(page, want) {
			t.Errorf("页面的 %s 与 SPA 不一致：页面里应当有 %s", name, want)
		}
	}

	if !strings.Contains(page, "sessionStorage.getItem(ROOM_AUTH_CACHE_KEY)") {
		t.Error("页面应从 sessionStorage 读凭据缓存（与 SPA 一致，关标签页即失效）")
	}
	if !strings.Contains(page, "Math.floor(Date.now() / 1000)") {
		t.Error("页面应把 expiresAt 当 Unix **秒**比较（SPA 用的是秒）")
	}
	if strings.Contains(page, `localStorage.setItem("ccgAutomationAuth"`) {
		t.Error("页面仍在写明文密码 —— 这条契约就是为了防止它回来")
	}
	if !strings.Contains(page, "/auth/token?room=") || !strings.Contains(page, "/auth/token/refresh?room=") {
		t.Error("页面必须走 /auth/token 换令牌 + 静默续签，而不是直接拿密码当 Bearer")
	}
}

// jsObjectBlock 取出 `start` 到其后第一个 `end` 之间的内容。
func jsObjectBlock(t *testing.T, src, start, end string) string {
	t.Helper()
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("页面里找不到 %q", start)
	}
	rest := src[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("找不到 %q 的结束标记", start)
	}
	return rest[:j]
}

// 页面里与「什么时候」区块相关的 id 必须都在 —— 这些是 JS 直接引用的，
// 少一个就是运行时 `$("xxx") is null`，整页白屏，而 Go 侧的字符串测试看不出来。
func TestAutomationPageWhenBlockWiring(t *testing.T) {
	page := renderAutomationPage(t)

	for _, id := range []string{
		"fMin", "fHour", "fDom", "fMonth", "fDow", "cronPresets", "cronWarn",
		"fRunAt", "fTZ", "schedHint", "cronNextHint",
		"whenCronName", "whenOnceName", "backLink",
	} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("页面缺少 id=%q —— JS 里 $() 会拿到 null", id)
		}
	}

	// 五个字段框：各配一个单位字，且默认值是 `*`（= 不限制）。
	// 顺序由「分 时 日 月 周」这五个字直接给出，用户不用背 —— 这正是拆成五个框的理由。
	for _, f := range []struct{ id, unit, def string }{
		{"fMin", "cronUnitMinute", "*"},
		{"fHour", "cronUnitHour", "*"},
		{"fDom", "cronUnitDom", "*"},
		{"fMonth", "cronUnitMonth", "*"},
		{"fDow", "cronUnitDow", "*"},
	} {
		re := regexp.MustCompile(`id="` + f.id + `" value="` + regexp.QuoteMeta(f.def) + `"`)
		if !re.MatchString(page) {
			t.Errorf("%s 的默认值应当是 %q（不限制）", f.id, f.def)
		}
		re = regexp.MustCompile(`id="` + f.id + `"[\s\S]{0,120}?data-i18n="` + f.unit + `"`)
		if !re.MatchString(page) {
			t.Errorf("%s 后面应当紧跟着 %s 这个单位字", f.id, f.unit)
		}
	}

	// 「仅一次」的时刻必须是**原生日期时间选择器**，不是自由文本框。
	// 文本框要求用户自己敲 `2026-10-01T09:30` —— 那个 `T` 是纯发明物，没人猜得到，
	// 敲错一格服务端只回一句「需要 runAt（RFC3339 或 2006-01-02T15:04）」。
	// 选择器的 value 形态正好是服务端认的「本地写法」，两边不用转换。
	if !strings.Contains(page, `id="fRunAt" type="datetime-local"`) {
		t.Error(`fRunAt 应当是 input[type=datetime-local]（日期时间选择器），不是自由文本框`)
	}

	// 常用写法按钮的文案
	for _, key := range []string{"cronPresetsLabel", "cronPresetDaily", "cronPresetWeekday", "cronPresetMonday", "cronPresetMonthly", "cronWarnSpammy"} {
		if !strings.Contains(page, key+":") {
			t.Errorf("文案表缺少 %s", key)
		}
	}

	// 五个字段框必须**排成一行**。用 wrap 的话窗口稍窄就会把「周」挤到第二行 ——
	// 只掉一个下来比整体换行更难看，而且掉的正好是「分 时 日 月 周」这个顺序提示的尾巴。
	cronRow := regexp.MustCompile(`\.cron-row \{[^}]*\}`).FindString(page)
	if !strings.Contains(cronRow, "flex-wrap: nowrap") {
		t.Errorf(".cron-row 必须是 nowrap（现在是 %q）—— 否则窄窗口下「周」会掉到第二行", cronRow)
	}

	// 两档必须是 radio 且同组：做成下拉框会把没选的那档藏起来。
	if n := strings.Count(page, `name="ccFreq"`); n != 2 {
		t.Errorf("ccFreq 单选组应有 2 个，找到 %d 个", n)
	}
	// 旧控件一并清掉，别留死代码或两种控件并存的歧义
	for _, dead := range []string{`id="fFreq"`, `id="fDays"`, `id="fCron"`, `id="cronEcho"`} {
		if strings.Contains(page, dead) {
			t.Errorf("旧的 %s 还在：控件并存会让人不知道以哪个为准", dead)
		}
	}

	// 提示区（翻译 / 提醒 / 未来时刻）**不带标签**，直接跟在档位下面 ——
	// 加一个「翻译」的标签只是多一层要读的字。
	if !strings.Contains(page, `class="when-hint"`) {
		t.Error("提示区没有独立的容器")
	}
	if strings.Contains(page, `data-i18n="cronTranslate"`) {
		t.Error("「翻译」标签又回来了 —— 这个标签是刻意去掉的")
	}
	// 提示区的三个 id 必须都在（JS 直接引用）
	for _, id := range []string{"schedHint", "cronWarn", "cronNextHint"} {
		if !strings.Contains(page, `id="`+id+`"`) {
			t.Errorf("提示区缺少 id=%q", id)
		}
	}
}

// SPA 工具条上的入口必须指向 /automation，而且**不能开新窗口**。
//
// 凭据存在 sessionStorage，它是按标签页隔离的 —— 入口一旦加上 target="_blank"，
// 用户在新标签页里会被再要一次密码，「免二次登录」这件事就白做了。
// 这个断言看起来像在管样式，其实守的是一条会被静默破坏的功能。
func TestSpaToolbarEntryStaysInSameTab(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web-vue3", "src", "components", "PageToolbar.vue"))
	if err != nil {
		t.Skipf("读不到 SPA 的 PageToolbar.vue，跳过契约检查: %v", err)
	}
	text := string(src)

	if !strings.Contains(text, "/automation") {
		t.Fatal("SPA 工具条里找不到 /automation 入口 —— 用户根本不知道这个功能存在")
	}
	anchor := strings.Index(text, `:href="automationUrl"`)
	if anchor < 0 {
		t.Fatal(`入口没有用 :href="automationUrl"（应当是同标签页的普通链接）`)
	}
	from := anchor - 600
	if from < 0 {
		from = 0
	}
	to := anchor + 600
	if to > len(text) {
		to = len(text)
	}
	block := text[from:to]
	if strings.Contains(block, `target="_blank"`) {
		t.Error(`入口加了 target="_blank" —— 新标签页里 sessionStorage 是空的，用户会被再要一次密码`)
	}

	// 入口要带上当前房间，否则点进去会掉到 default 房间
	if !strings.Contains(text, "automationUrl") || !strings.Contains(text, "encodeURIComponent(ws.room)") {
		t.Error("入口没有把当前房间带进 URL —— 会跳到默认房间的自动化")
	}
}
