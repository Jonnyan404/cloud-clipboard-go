package lib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func actionCtx() renderContext {
	// ⚠️ 固定 +08:00，**刻意不用 time.Local**：`inspect.timestamp` 这类动作的结果会跟着
	// 时区变，用本地时区的话同一条断言在 CI（通常是 UTC）和本机会得出不同答案 ——
	// 那种「换台机器就红」的测试比没有还糟。
	// 用 FixedZone 而不是 LoadLocation("Asia/Shanghai")：后者依赖 tzdata，精简镜像里没有。
	loc := time.FixedZone("CST", 8*3600)
	return renderContext{Now: time.Date(2026, 9, 24, 9, 30, 0, 0, loc), Task: "t", Room: "home"}
}

// steps 把一串动作 id 变成链（都是无参数步骤）。测试里这样写短一点。
func steps(ids ...string) []AutomationChainStep {
	out := make([]AutomationChainStep, 0, len(ids))
	for _, id := range ids {
		out = append(out, AutomationChainStep{ID: id})
	}
	return out
}

func runAction(t *testing.T, id, input string) string {
	t.Helper()
	if !IsServerRenderAction(id) {
		t.Fatalf("动作 %s 不在服务端可执行集合里", id)
	}
	out, err := applyRenderActions(input, steps(id), actionCtx())
	if err != nil {
		t.Fatalf("动作 %s 执行失败: %v", id, err)
	}
	return out
}

func TestRenderActionOutputs(t *testing.T) {
	cases := []struct {
		id, input, want string
	}{
		{"text.trimLines", "  a  \n\tb\t", "a\nb"},
		{"text.dropBlank", "a\n\n  \nb", "a\nb"},
		{"text.dedupe", "a\na\n\nb\nb", "a\n\nb"},
		{"text.sort", "b\nc\na", "a\nb\nc"},
		{"text.upper", "abc", "ABC"},
		{"text.lower", "ABC", "abc"},

		{"encode.base64", "abc", "YWJj"},
		{"encode.base64.decode", "YWJj", "abc"},
		{"encode.hex", "abc", "61 62 63"},
		{"encode.hex.decode", "61 62 63", "abc"},
		// 关键：encodeURIComponent 与 url.QueryEscape 的区别就在这两行 ——
		// 空格是 %20 不是 +，而解码时 + 不该变成空格。
		{"encode.url", "a b&c", "a%20b%26c"},
		{"encode.url.decode", "a%20b%26c", "a b&c"},
		{"encode.url", "+", "%2B"},
		{"encode.url.decode", "+", "+"},

		{"inspect.sha256", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},

		{"date.add", "2026-09-24 +1d", "2026-09-25"},
		{"date.add", "2026-09-24 +1w", "2026-10-01"},
		{"date.add", "2026-01-31 +1m", "2026-02-28"},
		{"date.add", "今天 +1d", "2026-09-25"},
		// 基准带时间 → 结果保留时间
		{"date.add", "2026-09-24 10:30 +2d", "2026-09-26 10:30"},

		// ── 格式化 ──────────────────────────────────────────────────
		// ⚠️ 键顺序必须**原样保留**（`b` 在 `a` 前面）：服务端用的是 json.Indent，
		// 不是 Unmarshal+Marshal —— 后者会按字典序把键重排，和前端的 JSON.stringify 不一样。
		{"format.json.pretty", `{"b":1,"a":[1,2]}`, `{
  "b": 1,
  "a": [
    1,
    2
  ]
}`},
		{"format.json.min", "{\n  \"a\": 1,\n  \"b\": [1, 2]\n}", `{"a":1,"b":[1,2]}`},

		// ── HTML 实体 / Unicode 转义 ────────────────────────────────
		// 只转四个（& < > "），**不转单引号** —— 这是和 html.EscapeString 的区别。
		{"encode.html", `a<b>&"c"`, "a&lt;b&gt;&amp;&quot;c&quot;"},
		{"encode.html.decode", "&lt;a&gt; &amp; &quot;x&quot;", `<a> & "x"`},
		// ⚠️ emoji 必须走 **UTF-16 单元**：`🪶`（U+1FAB6）要输出成一对代理 `\ud83e\udeb6`，
		// 按码点写会得到 `\u1fab6` 这种「看着对、其实 JS 解不出来」的东西。
		{"encode.unicode", "岚🪶", `\u5c9a\ud83e\udeb6`},
		{"encode.unicode.decode", `\u5c9a\ud83e\udeb6`, "岚🪶"},

		// ── 文本 ────────────────────────────────────────────────────
		{"text.reverse", "abc", "cba"},
		// 按码点反转：emoji 不能被拆成两个半截
		{"text.reverse", "a🪶b", "b🪶a"},
		// 中文标点必须在**排除集**里，否则「见 https://a.com，然后」会把逗号和后面一起吞进去
		{"text.extractUrl", "见 https://a.com，然后 www.b.cn/x", "https://a.com\nwww.b.cn/x"},
		{"text.extractEmail", "a@b.com 和 c.d@e.co", "a@b.com\nc.d@e.co"},
		{"text.extractPhone", "13800138000 和 13900139000", "13800138000\n13900139000"},
		// 12 位连写不能被截出前 11 位
		{"text.extractPhone", "138001380001", ""},
		// 每段限位 0-255，否则 999.999.999.999 也会被当成 IP
		{"text.extractIp", "1.2.3.4 和 999.999.999.999", "1.2.3.4"},
		{"text.extractNumber", "价格 -12.5 和 1,234", "-12.5\n1,234"},

		// ── 中文 ────────────────────────────────────────────────────
		// ⚠️ 空格要单独处理：0x20+0xFEE0 = U+FF00 是**未分配字符**，全角空格是 U+3000
		{"zh.fullwidth", "a b!", "ａ　ｂ！"},
		{"zh.halfwidth", "ａ　ｂ！", "a b!"},
		{"zh.punctuation", "你好，世界。", "你好,世界."},
		// 必须**分节**（4 位一节 + 万/亿），逐位查表表达不出「十万」这种组合单位
		{"zh.number", "10001", "一万零一"},
		{"zh.number", "10000", "一万"},
		{"zh.number", "10000001", "一千万零一"},
		{"zh.number", "10", "十"},
		{"zh.number", "0", "零"},
		{"zh.number", "-12.5", "负十二点五"},

		// ── 时间戳 ──────────────────────────────────────────────────
		// 1735689600 = 2025-01-01T00:00:00Z，在 +08:00 的 ctx 下就是 08:00
		{"inspect.timestamp", "1735689600", "2025-01-01 08:00:00"},
		// ⚠️ 按 **UTC** 解析，和前端 Date.parse('2025-01-01') 一致
		{"inspect.dateToTimestamp", "2025-01-01", "1735689600"},
	}

	for _, c := range cases {
		if got := runAction(t, c.id, c.input); got != c.want {
			t.Errorf("%s(%q) = %q，期望 %q", c.id, c.input, got, c.want)
		}
	}
}

func TestRenderActionDateDiff(t *testing.T) {
	got := runAction(t, "date.diff", "2026-01-01 ~ 2026-03-15")
	if got != "73 天\n10 周 3 天" {
		t.Fatalf("date.diff = %q", got)
	}
	// 两行写法等价
	if two := runAction(t, "date.diff", "2026-01-01\n2026-03-15"); two != got {
		t.Fatalf("两行写法应等价，得到 %q", two)
	}
}

// 带参数的动作：参数从**链元素**上取，不是从动作定义上取。
//
// 这条同时钉住「同一条链上同一个动作出现两次、两次用不同参数」——
// 那是「链允许重复」这条约定的直接后果，也是参数必须挂在步骤上（而不是动作上）的原因。
func TestRenderActionWithParams(t *testing.T) {
	chain := []AutomationChainStep{
		{ID: "text.replace", Params: map[string]string{"find": "内网", "with": "外网"}},
		{ID: "text.replace", Params: map[string]string{"find": "测试", "with": "正式"}},
	}
	out, err := applyRenderActions("内网测试环境", chain, actionCtx())
	if err != nil {
		t.Fatalf("链执行失败: %v", err)
	}
	if out != "外网正式环境" {
		t.Fatalf("结果 = %q，期望 外网正式环境", out)
	}
}

// 替换是**字面**的，不是正则。
//
// ⚠️ 用 `axb` 而不是 `a.b` 来测：后者用正则和字面得到同样的结果，区分不出来。
// `axb` 里的 `.` 是「任意字符」才匹配得到 —— 字面替换不该碰它。
func TestRenderReplaceIsLiteralNotRegex(t *testing.T) {
	out, err := applyRenderActions("axb", []AutomationChainStep{
		{ID: "text.replace", Params: map[string]string{"find": ".", "with": "-"}},
	}, actionCtx())
	if err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if out != "axb" {
		t.Fatalf("结果 = %q，期望 axb（`.` 是普通字符，不是「任意字符」）", out)
	}
}

// 「查找」为空必须报错，不能当成「在每个字符之间插入」—— 那样用户只会看到一串乱码。
func TestRenderReplaceRejectsEmptyFind(t *testing.T) {
	for _, params := range []map[string]string{
		{"with": "x"}, // 完全没有 find
		{"find": ""},  // find 是空串
	} {
		if _, err := applyRenderActions("abc", []AutomationChainStep{
			{ID: "text.replace", Params: params},
		}, actionCtx()); err == nil {
			t.Errorf("params=%v 时应当报错", params)
		}
	}
}

// 替换的**常用模式**。每个模式一条 —— 这些正则两端各有一份实现，
// 所以行为要被钉住（正则字符串本身另有一条契约测试比对）。
func TestRenderReplaceModes(t *testing.T) {
	run := func(mode, input, with string) string {
		t.Helper()
		out, err := applyRenderActions(input, []AutomationChainStep{
			{ID: "text.replace", Params: map[string]string{"mode": mode, "with": with}},
		}, actionCtx())
		if err != nil {
			t.Fatalf("模式 %s 执行失败: %v", mode, err)
		}
		return out
	}

	cases := []struct{ mode, input, with, want string }{
		// ⚠️ `[0-9]+` 是**贪婪**的：`123` 整串匹配一次、替换成一个 `#`，不是逐位替换成 `###`
		{"digits", "订单 A123 共 45 元", "#", "订单 A# 共 # 元"},
		{"latin", "订单 A123 共 45 元", "#", "订单 #123 共 45 元"},
		// 连续空白压成一个 —— ⚠️ 只压水平空白，换行不在内（见 replaceModes 的说明）
		{"spaces", "a    b", " ", "a b"},
		{"email", "联系 a@b.com 或 c@d.cn", "***", "联系 *** 或 ***"},
		// 大小写不敏感（flags 里的 i）
		{"url", "见 https://a.com 和 WWW.B.CN", "***", "见 *** 和 ***"},
		{"phone", "打 13800138000", "***", "打 ***"},
		{"ip", "从 192.168.1.1 来", "***", "从 *** 来"},
		// with 留空 = 删除
		{"digits", "A123", "", "A"},
	}
	for _, c := range cases {
		if got := run(c.mode, c.input, c.with); got != c.want {
			t.Errorf("模式 %s(%q -> %q) = %q，期望 %q", c.mode, c.input, c.with, got, c.want)
		}
	}
}

// 替换文本一律当**字面**：`$1` 不能被当成组引用 —— 否则用户填个 `$` 就成了危险字符。
// 前端那边对应 `String.replace(re, () => with)`，同一条语义。
func TestRenderReplaceTreatsWithAsLiteral(t *testing.T) {
	out, err := applyRenderActions("a1b2", []AutomationChainStep{
		{ID: "text.replace", Params: map[string]string{"mode": "digits", "with": "$1"}},
	}, actionCtx())
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if out != "a$1b$1" {
		t.Fatalf("结果 = %q，期望 a$1b$1（`$1` 是字面文本，不是组引用）", out)
	}
}

// 未知模式要报错，不能静默退化成字面替换。
func TestRenderReplaceRejectsUnknownMode(t *testing.T) {
	if _, err := applyRenderActions("abc", []AutomationChainStep{
		{ID: "text.replace", Params: map[string]string{"mode": "nope", "with": "x"}},
	}, actionCtx()); err == nil {
		t.Fatal("未知模式应当报错")
	}
}

// 替换模式的正则必须和前端**逐字一致**。
//
// 这是「不给用户正则输入框」这个决定的全部理由：固定的正则让「两边行为一致」变成**可验证**的。
// 数据源是 `web-vue3/src/data/replace-modes.json`（前端 import 它），服务端有一份等价的硬编码 ——
// 谁改了一边没同步另一边，这条测试立刻红，而不是等用户在凌晨发现
// 「预览区替换了、定时任务没替换」。
func TestReplaceModesMatchFrontend(t *testing.T) {
	path := filepath.Join("..", "..", "web-vue3", "src", "data", "replace-modes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读不到替换模式表（%s），跳过契约检查: %v", path, err)
	}
	var file struct {
		Modes []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Flags  string `json:"flags"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	if len(file.Modes) == 0 {
		t.Fatalf("%s 里没有模式", path)
	}

	got := ReplaceModeKeys()
	if len(got) != len(file.Modes) {
		t.Fatalf("模式数量不一致：服务端 %d 个（%v），前端 %d 个", len(got), got, len(file.Modes))
	}
	for i, m := range file.Modes {
		if got[i] != m.Key {
			// **顺序也要一致** —— 它就是下拉里的顺序，第一项还是默认值
			t.Errorf("第 %d 个模式不一致：服务端 %q，前端 %q", i, got[i], m.Key)
			continue
		}
		source, caseFold, _ := replaceModeByKey(m.Key)
		if source != m.Source {
			t.Errorf("模式 %s 的正则不一致：\n  服务端 %q\n  前端   %q", m.Key, source, m.Source)
		}
		if wantFold := strings.Contains(m.Flags, "i"); caseFold != wantFold {
			t.Errorf("模式 %s 的大小写标志不一致：服务端 %v，前端 flags=%q", m.Key, caseFold, m.Flags)
		}
	}
}

func TestRenderActionChainOrder(t *testing.T) {
	// 先转大写再编码 —— 顺序不可交换，这条测试守的就是「按顺序执行」。
	out, err := applyRenderActions("abc", steps("text.upper", "encode.base64"), actionCtx())
	if err != nil {
		t.Fatalf("链执行失败: %v", err)
	}
	if out != "QUJD" {
		t.Fatalf("链结果 = %q，期望 QUJD（base64 of ABC）", out)
	}
}

func TestRenderActionChainStopsOnFailure(t *testing.T) {
	_, err := applyRenderActions("abc", steps("encode.base64.decode", "text.upper"), actionCtx())
	if err == nil {
		t.Fatal("第一步就该失败（abc 不是合法 base64）")
	}
	if !strings.Contains(err.Error(), "encode.base64.decode") {
		t.Fatalf("错误信息应指明是哪一步失败，得到 %q", err.Error())
	}
}

func TestUnknownRenderActionIsRejected(t *testing.T) {
	_, err := applyRenderActions("x", steps("pinyin.annotate"), actionCtx())
	if err == nil {
		t.Fatal("未下沉的动作必须报错，不能静默跳过")
	}
	// 报错要给出可用集合 —— 否则用户不知道「为什么这个动作不行，哪些行」
	if !strings.Contains(err.Error(), "text.trimLines") {
		t.Fatalf("错误信息应列出自可用动作，得到 %q", err.Error())
	}
}

func TestRenderActionRejectsBadDateInput(t *testing.T) {
	for _, bad := range []string{"随便一段文字", "2026-02-31 +1d", "2026-09-24 加一天"} {
		if _, err := applyRenderActions(bad, steps("date.add"), actionCtx()); err == nil {
			t.Errorf("date.add(%q) 应当报错", bad)
		}
	}
}

// ── 与前端动作库的契约 ──────────────────────────────────────────────
//
// 定时任务里存的是**动作 id**，服务端和前端各有一份实现。两侧 id 一旦漂开，
// 用户会看到「预览区能跑，定时任务报错」这种最难查的错。
//
// 这个测试直接去读前端的动作表，把「同名」这条不变量钉住 ——
// 它是 render_actions.go 文件头那句「id 必须逐字一致」的可执行版本。
func TestRenderActionIDsMatchFrontend(t *testing.T) {
	path := filepath.Join("..", "..", "web-vue3", "src", "data", "actions.js")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读不到前端动作表（%s），跳过契约检查: %v", path, err)
	}
	text := string(source)

	for _, id := range ServerRenderActionIDs() {
		if !strings.Contains(text, "id: '"+id+"'") && !strings.Contains(text, `id: "`+id+`"`) {
			t.Errorf("服务端动作 %s 在前端 actions.js 中不存在 —— 两侧 id 已经漂开", id)
		}
	}
}

// 服务端**不该**包含只有浏览器能跑的动作。这条测试的价值在于：
// 将来有人顺手把 pinyin 加进 serverRenderActions，会在这里被拦住 ——
// 因为服务端没有那个词典，加进去只会在凌晨触发时静默失败。
//
// ⚠️ 这里必须列**前端真实存在的 id**。此前写的是 `pinyin.annotate` / `zh.convert` ——
// 两个早就不存在的旧 id，于是这条测试一直在检查空气（永远绿）。
// 所以下面顺带断言「这些 id 在前端确实存在」，让它没法再退化成空气。
func TestClientOnlyActionsStayOutOfServerSet(t *testing.T) {
	clientOnly := []struct{ id, why string }{
		// 「生成」类（direction: insert）。它们在链里会把前面算出来的正文**整个丢掉** ——
		// 链的不变式是「输入有意义 → 输出是变换结果」，而「生成」压根不看输入。
		// 要它们的效果请用模板变量（内联、不覆盖任何东西）。
		{"generate.uuid", "生成类：链里会丢掉前面的正文；要它请用正文里的 {{uuid}}"},
		{"generate.time", "同上，用 {{time}}"},
		{"generate.datetime", "同上，用 {{datetime}}"},

		{"format.markdown", "产出 HTML 是为了「看」，而且依赖 marked"},
		{"format.code", "同上，还要 highlight.js 做语言检测"},
		{"zh.pinyin", "依赖 pinyin-pro 词典（317KB，前端动态 import）"},
		{"zh.pinyin.table", "同上"},
		{"zh.pinyin.word", "同上"},
		{"zh.simplified", "依赖 opencc-js 词典（6MB 量级）"},
		{"zh.traditional", "同上"},
		{"inspect.stats", "输出里带给人看的标签，而服务端没有 i18n —— 只能固定成某一种语言"},
		{"inspect.detect", "判定依赖前端的 markdown / 代码启发式，在服务端复刻一份必然漂开"},
	}

	for _, c := range clientOnly {
		if IsServerRenderAction(c.id) {
			t.Errorf("%s 不该出现在服务端可执行集合里：%s", c.id, c.why)
		}
	}

	path := filepath.Join("..", "..", "web-vue3", "src", "data", "actions.js")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读不到前端动作表（%s），跳过存在性检查: %v", path, err)
	}
	text := string(source)
	for _, c := range clientOnly {
		if !strings.Contains(text, "id: '"+c.id+"'") && !strings.Contains(text, `id: "`+c.id+`"`) {
			t.Errorf("clientOnly 清单里的 %s 在前端 actions.js 里不存在 —— 这条测试正在检查空气", c.id)
		}
	}
}
