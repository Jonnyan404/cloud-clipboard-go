package lib

// 动作库的契约 fixture：把**服务端可执行动作的注册表**与**它们在 Go 下的实际输出**
// 导出成 JSON，供 Rust 侧比对。
//
// 为什么需要它（`ARCHITECTURE.md` §5.4 早就点名了这件事）：
// 动作库现在要在**两个实现/两个目标**里跑同一套语义 —— 定时任务用原生 Rust，
// 预览区将来用 WASM 的同一份 Rust。而今天唯一的行为基准是 Go 的 `render_actions.go`。
// 「读代码觉得一致」和「真的一致」在这个文件里差得最远，因为每个动作都有一堆
// 边界（emoji 的代理对、全角空格、贪婪正则、html 实体的容忍度……），
// 而这些边界**只能靠跑出来**：
//
//   · 期望值不是人写的，是**跑 Go 得到的** —— 所以这份 fixture 是行为基准，不是文档；
//   · Rust 测试读同一份用例 → 两边跑同一组输入，才是真正的双跑验证；
//   · 将来 Go 停维护，这份用例继续给 Rust 用，零浪费。
//
// 用法：
//
//	go test ./lib -run TestActionFixtures                      # 校验（不复写）
//	UPDATE_FIXTURES=1 go test ./lib -run TestActionFixtures     # 重新生成
//
// ⚠️ 重新生成时**先看 diff**：某个动作的输出变了，等于「同一个任务在切换前后发出去的内容不一样」。
// 那可能是修 bug（好事），也可能是漂了（坏事）—— 两种情况都要人来判断，别顺手接受。
//
// ⚠️ 这份 fixture 覆盖**全部** 34 个服务端动作，不只 Rust 侧当前实现了的那些 ——
// 这样 Rust 每补一组动作都不用回来重新生成。Rust 侧用 `cases` 里出现过的 id 与
// 它自己的注册表比对，**缺了哪个 id 会红**（`crates/actions/tests/go_cases.rs`）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// fixture 落点：仓库根的 cases/actions/（`go test` 的 cwd 是 cloud-clip/lib）。
const actionFixtureDir = "../../cases/actions"

type actionFixtureParamOption struct {
	Value    string `json:"value"`
	LabelKey string `json:"labelKey"`
}

type actionFixtureCondition struct {
	Key    string `json:"key"`
	Equals string `json:"equals"`
}

type actionFixtureParam struct {
	Key         string                     `json:"key"`
	LabelKey    string                     `json:"labelKey"`
	Type        string                     `json:"type,omitempty"`
	Options     []actionFixtureParamOption `json:"options,omitempty"`
	VisibleWhen *actionFixtureCondition    `json:"visibleWhen,omitempty"`
}

type actionFixtureSpec struct {
	ID       string               `json:"id"`
	Group    string               `json:"group"`
	GroupKey string               `json:"groupKey"`
	Key      string               `json:"key"`
	Params   []actionFixtureParam `json:"params"`
}

type actionFixtureRegistry struct {
	Note      string              `json:"note"`
	Actions   []actionFixtureSpec `json:"actions"`
	I18nKeys  []string            `json:"i18nKeys"`
	CountHint int                 `json:"countHint"`
}

type actionFixtureCase struct {
	ID     string            `json:"id"`
	Input  string            `json:"input"`
	Params map[string]string `json:"params,omitempty"`
	// Expect 是 **Go 跑出来的**结果。报错时整项省略，只留 Error。
	//
	// ⚠️ 用**指针**：`omitempty` 对空字符串也生效，于是「成功但结果是空串」
	// （`encode.hex.decode` 拿到 `"  "` 就是这种）会和「报错」长得一模一样 ——
	// 两种情况的语义差着十万八千里，fixture 里必须分得开。
	Expect *string `json:"expect,omitempty"`
	// Error 为 true 表示 Go 那边报错了。**不记录错误文案**：两侧的措辞本来就可以不同，
	// 该对齐的是「成功时的输出」与「失败这件事本身」。
	Error bool   `json:"error,omitempty"`
	Why   string `json:"why,omitempty"`
}

type actionFixtureCases struct {
	Note    string `json:"note"`
	Context struct {
		// Now 是固定的「此刻」（+08:00）—— 与 render_actions_test.go 的 actionCtx 一致。
		// ⚠️ 时间类动作的输出跟着它走，所以它必须进 fixture：Rust 侧要拿同一个时钟跑。
		Now  string `json:"now"`
		Task string `json:"task"`
		Room string `json:"room"`
	} `json:"context"`
	Cases []actionFixtureCase `json:"cases"`
}

// actionFixtureCasesFor 是全部用例的**输入**表。期望值由 Go 现跑现取（见下）。
//
// ⚠️ 这里刻意比 `render_actions_test.go` 里那张表**更宽**：那张表钉的是
// 「我写在注释里的判断对不对」，这张表要的是「Rust 复现的行为一不一样」，
// 所以边界（空串、无效输入、代理对、全角空格、贪婪匹配）都要各来一条。
func actionFixtureCasesFor() []actionFixtureCase {
	return []actionFixtureCase{
		// ── format ────────────────────────────────────────────────
		// ⚠️ 键顺序必须原样保留（`b` 在 `a` 前面）：Go 用 json.Indent，不是
		// Unmarshal+Marshal（后者按字典序重排）。
		{ID: "format.json.pretty", Input: `{"b":1,"a":[1,2]}`, Why: "键顺序必须原样保留，不能按字典序重排"},
		{ID: "format.json.pretty", Input: `{"nested":{"x":[{"y":true}]},"s":"值"}`, Why: "嵌套 + 中文"},
		{ID: "format.json.pretty", Input: `  {"a":1}  `, Why: "前后空白"},
		// ⚠️ 空容器不能展开成多行（`{}` 就是 `{}`）—— 这条专门钉住边界。
		{ID: "format.json.pretty", Input: `{"a":[],"b":{}}`, Why: "空容器不展开"},
		{ID: "format.json.pretty", Input: `{"a":-1.50,"b":"x:y,z"}`, Why: "数字与字符串里的冒号/逗号不能被动"},
		// ⚠️ 已经美化过就**报错**（与前端同一条约定：结果和原文一样算失败）。
		{ID: "format.json.pretty", Input: "{\n  \"a\": 1\n}", Error: true, Why: "已经是美化过的 → 报错，不是原样返回"},
		{ID: "format.json.pretty", Input: `{"a":`, Error: true, Why: "坏 JSON 要报错，不能吐半截"},
		{ID: "format.json.min", Input: "{\n  \"a\": 1,\n  \"b\": [1, 2]\n}", Why: ""},
		{ID: "format.json.min", Input: "{\n  \"s\": \"a b, c\",\n  \"n\": [1, 2]\n}", Why: "字符串里的空白不能动"},
		// ⚠️ 已经是一行时**报错**，不是原样返回：这个动作的语义是「压成一行」，
		// 而没有可压的东西时静默成功会让人以为「跑过了」。
		{ID: "format.json.min", Input: `{"a":1}`, Error: true, Why: "已经是一行 → 报错（不是原样返回）"},
		{ID: "format.json.min", Input: `not json`, Error: true, Why: ""},

		// ── encode ────────────────────────────────────────────────
		{ID: "encode.base64", Input: "abc"},
		{ID: "encode.base64", Input: ""},
		{ID: "encode.base64", Input: "中文🪶"},
		{ID: "encode.base64.decode", Input: "YWJj"},
		{ID: "encode.base64.decode", Input: "5Lit5paH"},
		// ⚠️ 不做无填充回退：前端 atob('abc') 会抛，两边必须一样。
		{ID: "encode.base64.decode", Input: "abc", Error: true, Why: "长度不是 4 的倍数，前端 atob 也会抛"},
		{ID: "encode.base64.decode", Input: "!!!", Error: true},
		{ID: "encode.base64.decode", Input: "YQ==", Why: "标准填充"},
		// ⚠️ 非零的尾比特：Go 的 StdEncoding **不看**它（解出来是 a），而 Rust 的默认
		// `RequireCanonical` 会报错 —— 这条就是拿来钉住这个差异的。
		{ID: "encode.base64.decode", Input: "YR==", Why: "尾比特非零：Go 容忍，Rust 默认不容忍"},
		{ID: "encode.base64.decode", Input: "YWJj=", Error: true, Why: "填充长度不对"},
		{ID: "encode.url", Input: "a b&c"},
		{ID: "encode.url", Input: "+", Why: "必须是 %2B（encodeURIComponent 与 QueryEscape 的区别之一）"},
		{ID: "encode.url", Input: "~!*'()-_.", Why: "这几个 unreserved 不转义"},
		{ID: "encode.url", Input: "中文 🪶"},
		{ID: "encode.url.decode", Input: "a%20b%26c"},
		{ID: "encode.url.decode", Input: "+", Why: "`+` 原样保留，不能被当成空格"},
		{ID: "encode.url.decode", Input: "%E4%B8%AD%E6%96%87"},
		{ID: "encode.url.decode", Input: "%ZZ", Error: true},
		{ID: "encode.url.decode", Input: "%", Error: true, Why: "截断的百分号转义"},
		{ID: "encode.url.decode", Input: "a%4", Error: true, Why: "只有一位十六进制"},
		{ID: "encode.url.decode", Input: "没有百分号", Why: "没有 % 时原样返回"},
		{ID: "encode.hex", Input: "abc"},
		{ID: "encode.hex", Input: "中文"},
		{ID: "encode.hex", Input: ""},
		{ID: "encode.hex.decode", Input: "61 62 63"},
		{ID: "encode.hex.decode", Input: "616263", Why: "分隔符可有可无"},
		{ID: "encode.hex.decode", Input: "6e-6f", Why: "非 hex 字符一律丢掉"},
		{ID: "encode.hex.decode", Input: "616", Error: true, Why: "奇数长度报错"},
		{ID: "encode.hex.decode", Input: "  ", Why: "丢掉非 hex 之后是空串 → 返回空串，不是报错"},
		{ID: "encode.hex.decode", Input: "0x61", Error: true, Why: "x 被丢掉，剩 061 是奇数长度"},
		{ID: "encode.html", Input: `a<b>&"c"`, Why: "只转 & < > \"，不转单引号"},
		{ID: "encode.html", Input: "'"},
		{ID: "encode.html.decode", Input: "&lt;a&gt; &amp; &quot;x&quot;"},
		{ID: "encode.html.decode", Input: "&#39;"},
		{ID: "encode.html.decode", Input: "&nbsp;"},
		{ID: "encode.unicode", Input: "岚🪶", Why: "emoji 必须走 UTF-16 代理对"},
		{ID: "encode.unicode", Input: "abc"},
		{ID: "encode.unicode.decode", Input: `\u5c9a\ud83e\udeb6`},
		{ID: "encode.unicode.decode", Input: `\u0041`},
		{ID: "encode.unicode.decode", Input: `\u00`, Why: "位数不够 → 原样留着"},
		{ID: "encode.unicode.decode", Input: `\uZZZZ`, Why: "不是十六进制 → 原样留着"},
		{ID: "encode.unicode.decode", Input: `前\u0041后`, Why: "转义两侧的原文要保留"},
		{ID: "encode.unicode", Input: "a\x7fb", Why: "0x7f 也要转义（> 0x7e）"},

		// ── inspect ───────────────────────────────────────────────
		{ID: "inspect.sha256", Input: "abc"},
		{ID: "inspect.sha256", Input: ""},
		{ID: "inspect.sha256", Input: "中文🪶"},
		{ID: "inspect.timestamp", Input: "1735689600", Why: "在 +08:00 的 ctx 下是 08:00"},
		{ID: "inspect.timestamp", Input: "1735689600000", Why: "13 位 = 毫秒"},
		{ID: "inspect.timestamp", Input: "0", Error: true, Why: "只认 10 位（秒）/ 13 位（毫秒）"},
		{ID: "inspect.timestamp", Input: "abc", Error: true},
		{ID: "inspect.dateToTimestamp", Input: "2025-01-01", Why: "按 UTC 解析，和前端 Date.parse('2025-01-01') 一致"},
		{ID: "inspect.dateToTimestamp", Input: "2025-01-01 08:00:00"},
		{ID: "inspect.dateToTimestamp", Input: "不是日期", Error: true},

		// ── text ──────────────────────────────────────────────────
		{ID: "text.trimLines", Input: "  a  \n\tb\t"},
		{ID: "text.trimLines", Input: "单行"},
		{ID: "text.dropBlank", Input: "a\n\n  \nb"},
		{ID: "text.dropBlank", Input: "   \n\t\n"},
		{ID: "text.dedupe", Input: "a\na\n\nb\nb", Why: "空行不去重：连着几个空行是有意的排版"},
		{ID: "text.dedupe", Input: "x\n\ny\n\nx"},
		{ID: "text.sort", Input: "b\nc\na"},
		{ID: "text.sort", Input: "10\n9\n2", Why: "字节序，不是数值序"},
		{ID: "text.upper", Input: "abc"},
		{ID: "text.upper", Input: "中文"},
		{ID: "text.lower", Input: "ABC"},
		{ID: "text.reverse", Input: "abc"},
		{ID: "text.reverse", Input: "a🪶b", Why: "按码点反转，代理对不能被拆开"},
		{ID: "text.extractUrl", Input: "见 https://a.com，然后 www.b.cn/x", Why: "中文标点必须在排除集里"},
		{ID: "text.extractUrl", Input: "没有网址"},
		{ID: "text.extractEmail", Input: "a@b.com 和 c.d@e.co"},
		{ID: "text.extractPhone", Input: "13800138000 和 13900139000"},
		{ID: "text.extractPhone", Input: "138001380001", Why: "12 位连写不能被截出前 11 位"},
		{ID: "text.extractIp", Input: "1.2.3.4 和 999.999.999.999", Why: "每段限位 0-255"},
		{ID: "text.extractNumber", Input: "价格 -12.5 和 1,234"},
		// text.replace：默认模式 = 字面替换（用 axb 才能和正则区分开）
		{ID: "text.replace", Input: "axb", Params: map[string]string{"find": ".", "with": "-"}, Why: "字面替换，`.` 不是任意字符"},
		{ID: "text.replace", Input: "内网测试环境", Params: map[string]string{"find": "内网", "with": "外网"}},
		{ID: "text.replace", Input: "abc", Params: map[string]string{"with": "x"}, Error: true, Why: "查找为空必须报错"},
		{ID: "text.replace", Input: "abc", Params: map[string]string{"find": "", "with": "x"}, Error: true},
		{ID: "text.replace", Input: "订单 A123 共 45 元", Params: map[string]string{"mode": "digits", "with": "#"}, Why: "[0-9]+ 是贪婪的"},
		{ID: "text.replace", Input: "订单 A123 共 45 元", Params: map[string]string{"mode": "latin", "with": "#"}},
		{ID: "text.replace", Input: "a    b", Params: map[string]string{"mode": "spaces", "with": " "}, Why: "只压水平空白，换行不在内"},
		{ID: "text.replace", Input: "联系 a@b.com 或 c@d.cn", Params: map[string]string{"mode": "email", "with": "***"}},
		{ID: "text.replace", Input: "见 https://a.com 和 WWW.B.CN", Params: map[string]string{"mode": "url", "with": "***"}, Why: "大小写不敏感"},
		{ID: "text.replace", Input: "打 13800138000", Params: map[string]string{"mode": "phone", "with": "***"}},
		{ID: "text.replace", Input: "从 192.168.1.1 来", Params: map[string]string{"mode": "ip", "with": "***"}},
		{ID: "text.replace", Input: "A123", Params: map[string]string{"mode": "digits", "with": ""}, Why: "with 留空 = 删除"},
		{ID: "text.replace", Input: "a$1b", Params: map[string]string{"find": "$1", "with": "X"}, Why: "替换文本一律当字面，$1 不是组引用"},

		// ── zh ────────────────────────────────────────────────────
		{ID: "zh.fullwidth", Input: "a b!", Why: "0x20+0xFEE0 = U+FF00 是未分配字符，全角空格是 U+3000"},
		{ID: "zh.fullwidth", Input: "ＡＢ"},
		{ID: "zh.halfwidth", Input: "ａ　ｂ！"},
		{ID: "zh.punctuation", Input: "你好，世界。"},
		{ID: "zh.punctuation", Input: "（测试）「引用」"},
		{ID: "zh.number", Input: "10001"},
		{ID: "zh.number", Input: "10000"},
		{ID: "zh.number", Input: "10000001"},
		{ID: "zh.number", Input: "10"},
		{ID: "zh.number", Input: "0"},
		{ID: "zh.number", Input: "-12.5"},

		// ── date ──────────────────────────────────────────────────
		{ID: "date.add", Input: "2026-09-24 +1d"},
		{ID: "date.add", Input: "2026-09-24 +1w"},
		{ID: "date.add", Input: "2026-01-31 +1m", Why: "月末溢出要夹到当月最后一天"},
		{ID: "date.add", Input: "今天 +1d", Why: "今天 = ctx.Now 的日期"},
		{ID: "date.add", Input: "2026-09-24 10:30 +2d", Why: "基准带时间则结果保留时间"},
		{ID: "date.add", Input: "不是日期 +1d", Error: true},
		{ID: "date.diff", Input: "2026-01-01 ~ 2026-03-15"},
		{ID: "date.diff", Input: "2026-01-01\n2026-03-15", Why: "两行写法等价"},
		{ID: "date.diff", Input: "只有一行", Error: true},
	}
}

func writeActionFixture(t *testing.T, name string, value interface{}) {
	t.Helper()
	if err := os.MkdirAll(actionFixtureDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	blob, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	blob = append(blob, '\n')
	path := filepath.Join(actionFixtureDir, name)
	if os.Getenv("UPDATE_FIXTURES") == "" {
		old, err := os.ReadFile(path)
		if err != nil || string(old) != string(blob) {
			t.Errorf("%s 与当前实现不一致 —— 看一眼 diff：是修了 bug，还是漂了？\n"+
				"确认无误后再用 UPDATE_FIXTURES=1 go test ./lib -run TestActionFixtures 重新生成", path)
		}
		return
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
	t.Logf("已生成 %s（%d 字节）", path, len(blob))
}

func TestActionFixtures(t *testing.T) {
	// ── 1) 注册表：id / 分组 / i18n key / 参数声明 ──────────────────────
	registry := actionFixtureRegistry{
		Note: "由 cloud-clip/lib/action_fixture_test.go 生成。id 与前端 web-vue3/src/data/actions.js 逐字一致；" +
			"服务端只下发 i18n key，译文只在四份 locale 里。",
		CountHint: len(serverRenderActions),
	}
	for _, spec := range serverRenderActions {
		item := actionFixtureSpec{ID: spec.ID, Group: spec.Group, GroupKey: spec.GroupKey, Key: spec.Key, Params: []actionFixtureParam{}}
		for _, p := range spec.Params {
			param := actionFixtureParam{Key: p.Key, LabelKey: p.LabelKey, Type: p.Type}
			for _, o := range p.Options {
				param.Options = append(param.Options, actionFixtureParamOption{Value: o.Value, LabelKey: o.LabelKey})
			}
			if p.VisibleWhen != nil {
				param.VisibleWhen = &actionFixtureCondition{Key: p.VisibleWhen.Key, Equals: p.VisibleWhen.Equals}
			}
			item.Params = append(item.Params, param)
		}
		registry.Actions = append(registry.Actions, item)
	}
	registry.I18nKeys = ServerRenderActionI18nKeys()
	sort.Strings(registry.I18nKeys)
	writeActionFixture(t, "registry.json", registry)

	// ── 2) 行为用例：期望值由 Go **现跑现取** ────────────────────────────
	ctx := actionCtx()
	out := actionFixtureCases{}
	out.Note = "期望值是 Go 跑出来的（cloud-clip/lib/render_actions_test.go 的 actionCtx 那套时钟）。" +
		"报错的用例只有 error，不记文案 —— 两侧措辞本来就可以不同。"
	out.Context.Now = ctx.Now.Format("2006-01-02T15:04:05-07:00")
	out.Context.Task = ctx.Task
	out.Context.Room = ctx.Room

	for _, c := range actionFixtureCasesFor() {
		item := actionFixtureCase{ID: c.ID, Input: c.Input, Params: c.Params, Why: c.Why}
		got, err := applyRenderActions(c.Input, []AutomationChainStep{{ID: c.ID, Params: c.Params}}, ctx)
		switch {
		case err != nil:
			// ⚠️ 输入表里没写 Error 却报错了 = 我把用例写错了（比如参数不全）。
			// 直接失败，不要静默记成一条「error: true」—— 那会让 fixture 悄悄变成
			// 「这个动作就是会报错」的样子。
			if !c.Error {
				t.Errorf("用例 %s(%q, %v) 意外报错: %v", c.ID, c.Input, c.Params, err)
				continue
			}
			item.Error = true
		case c.Error:
			t.Errorf("用例 %s(%q, %v) 期望报错，却回了个结果: %q", c.ID, c.Input, c.Params, got)
			continue
		default:
			item.Expect = &got
		}
		out.Cases = append(out.Cases, item)
	}
	writeActionFixture(t, "cases.json", out)

	// ── 3) 顺手把「每个 id 都被用例覆盖到」钉住 ─────────────────────────
	covered := map[string]bool{}
	for _, c := range out.Cases {
		covered[c.ID] = true
	}
	for _, spec := range serverRenderActions {
		if !covered[spec.ID] {
			t.Errorf("动作 %s 一条用例都没有 —— 补进 actionFixtureCasesFor()", spec.ID)
		}
	}
	if len(out.Cases) < len(serverRenderActions) {
		t.Errorf("用例数 %d 少于动作数 %d，明显漏了", len(out.Cases), len(serverRenderActions))
	}
}
