package lib

// 模板引擎的契约 fixture：把 `render.go` 在**固定时刻、固定上下文**下的输出导出成
// JSON，供 Rust 侧比对（`crates/core/tests/go_render.rs`）。
//
// 与动作库的 fixture 同一套思路（见 `action_fixture_test.go`）：期望值是**跑 Go 得到的**，
// 不是人写的。区别在于模板引擎的输入不是「一段文本 + 动作」，而是「一段带变量的模板」，
// 它的难点在：
//
//   · **偏移挂在变量自己身上**（`{{weekday:+1d}}` 的 +1d 只影响周几，不影响旁边的 date）；
//   · **月溢出夹取**（1 月 31 日 +1m 要夹到 2 月最后一天，闰年另算）；
//   · **周几四种写法**（zh / zh-short / en / en-short，en 是全名）；
//   · **偏移语法沿用动作库 date.add 的 `[+-]N[dwmy]`**（省略单位按天）；
//   · **求值要么全成功、要么整体失败**（未知变量不静默放过）；
//   · **`{{latest}}` 是唯一读外部状态的变量**，靠注入的来源读取器（fixture 里给桩）。
//
// ⚠️ 两个变量**不能逐字节比对**，所以 fixture 里不记它们的字面结果：
//   · `{{uuid}}` —— 每次求值都不同（这是它的意义）；
//   · `{{timestamp}}` —— 它是 now 的 Unix 秒，虽然对固定 now 是确定的，但「确定」这个性质
//     本身就是 Rust 侧要自己证的东西（拿 `ctx.now.timestamp()` 对），放进 fixture 等于
//     把实现细节抄了一遍。两条都用「这条用例**用了哪个不确定变量**」来标注，
//     Rust 侧据此换成「结果非空 / 长度对 / 是合法 UUID」这类不依赖字面值的断言。
//
// ⚠️ fixture 必须**固定时区**：now 写成带偏移的 RFC3339，Rust 按字符串解析回 `DateTime<FixedOffset>`。
// 跟着机器时区漂就是假绿（与 cron fixture 同一条坑）。
//
// 用法：
//
//	go test ./lib -run TestRenderFixtures                      # 校验（不复写）
//	UPDATE_FIXTURES=1 go test ./lib -run TestRenderFixtures     # 重新生成

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const renderFixtureDir = "../../cases/render"

type renderFixtureCase struct {
	// Tpl 是输入模板。
	Tpl string `json:"tpl"`
	// Expect 是 Go 跑出来的**完整**结果。用了不确定变量（uuid/timestamp）的用例不记 Expect，
	// 用 Uncertain 标注（见文件头注释）。
	Expect *string `json:"expect"`
	// Error 为 true 表示 Go 那边报错了。不记录错误文案（两侧措辞本来就可以不同）。
	Error bool `json:"error"`
	// Uncertain 标注这条用了哪个不确定变量（"uuid" 或 "timestamp"），仅用于 Rust 侧换成
	// 不依赖字面值的断言。
	Uncertain string `json:"uncertain,omitempty"`
	Why       string `json:"why,omitempty"`
}

type renderFixture struct {
	Note string `json:"note"`
	// Now 是固定的求值时刻（RFC3339 + 固定偏移），Rust 按它解析回时刻。
	Now  string `json:"now"`
	Task string `json:"task"`
	Room string `json:"room"`
	// Cases 是全部用例。
	Cases []renderFixtureCase `json:"cases"`
}

func renderFixtureContext() renderContext {
	// 固定偏移 +08:00，别用 time.Local（机器时区会漂）。
	loc := time.FixedZone("+08:00", 8*3600)
	now := time.Date(2026, 9, 24, 9, 30, 0, 0, loc) // 周四，和 render_test.go 的 renderBase 同一天
	// latest 桩：ops 与 empty 各一种，其余房间回「来自 <room>」。
	return renderContext{
		Now:  now,
		Task: "值班提醒",
		Room: "home",
		Latest: func(room string) (string, bool) {
			if room == "empty" {
				return "", false
			}
			return "来自 " + room, true
		},
	}
}

func renderFixtureCases() []renderFixtureCase {
	return []renderFixtureCase{
		// ── date ──────────────────────────────────────────────────
		{Tpl: "{{date}}", Why: "基准日期"},
		{Tpl: "{{date:+1d}}"},
		{Tpl: "{{date:-1d}}"},
		{Tpl: "{{date:+1w}}"},
		{Tpl: "{{date:+1}}", Why: "省略单位按天，与动作库 date.add 一致"},
		{Tpl: "{{date}} 与 {{date:+1d}}", Why: "同一条里两个 date 各自偏移"},
		{Tpl: "{{date:+1m}}", Why: "月溢出夹取（9 月 24 日 +1m = 10 月 24 日，不夹）"},
		{Tpl: "{{date:abc}}", Error: true, Why: "非法偏移"},
		{Tpl: "{{date:1d+}}", Error: true, Why: "符号在单位后，非法"},

		// ── weekday ───────────────────────────────────────────────
		{Tpl: "{{weekday}}", Why: "默认 zh 全名"},
		{Tpl: "{{weekday:+1d}}"},
		{Tpl: "{{weekday:-1d}}"},
		{Tpl: "{{weekday:+2d}}"},
		{Tpl: "{{weekday:en}}", Why: "en 是全名"},
		{Tpl: "{{weekday:+1d|en}}"},
		{Tpl: "{{weekday:+1d|en-short}}"},
		{Tpl: "{{weekday:zh-short}}"},
		{Tpl: "{{weekday:en|+1d}}", Why: "参数顺序可换"},
		{Tpl: "{{weekday:+1d|cn-simple}}", Error: true, Why: "写错的样式名"},
		{Tpl: "{{weekday:+1d|en|zh}}", Error: true, Why: "参数段太多"},

		// ── 偏移挂在变量自己身上（关键设计）───────────────────────
		{Tpl: "今天是 {{date}}（{{weekday}}），明天是 {{date:+1d}}（{{weekday:+1d}}）",
			Why: "周几必须跟着自己的偏移走，否则文案自相矛盾"},

		// ── time / datetime / task / room ─────────────────────────
		{Tpl: "{{time}}"},
		{Tpl: "{{datetime}}"},
		{Tpl: "{{task}}"},
		{Tpl: "{{room}}"},
		{Tpl: "{{time:+1d}}", Error: true, Why: "time 不接受参数"},
		{Tpl: "{{datetime:x}}", Error: true, Why: "datetime 不接受参数"},

		// ── 不确定变量 ────────────────────────────────────────────
		{Tpl: "{{uuid}}", Uncertain: "uuid"},
		{Tpl: "{{uuid:x}}", Error: true, Why: "uuid 不接受参数"},
		{Tpl: "{{timestamp}}", Uncertain: "timestamp"},
		{Tpl: "{{timestamp:x}}", Error: true, Why: "timestamp 不接受参数"},

		// ── 纯文本 / 全有或全无 ────────────────────────────────────
		{Tpl: "没有变量", Why: "纯文本原样返回"},
		{Tpl: "今天是 {{date}}，{{dtae}}", Error: true, Why: "任一变量失败则整体失败，不吐半成品"},
		{Tpl: "{{ nope }}", Error: true, Why: "变量名两侧空格也要能认出来是未知变量"},

		// ── latest ────────────────────────────────────────────────
		{Tpl: "{{latest}}", Why: "不带参数取本房间"},
		{Tpl: "转：{{latest:ops}}", Why: "带参数取指定房间"},
		{Tpl: "{{latest:empty}}", Error: true, Why: "空来源是可识别的错误"},
		{Tpl: "{{latest:my room}}", Error: true, Why: "房间名含空格直接拒"},
	}
}

func writeRenderFixture(path string, f renderFixture) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
}

func TestRenderFixtures(t *testing.T) {
	ctx := renderFixtureContext()
	cases := renderFixtureCases()

	f := renderFixture{
		Note:  "模板引擎的行为基准：期望值由 Go 的 render.go 跑出（固定时刻 + 固定上下文）。uuid / timestamp 用 uncertain 标注，见文件头注释。",
		Now:   ctx.Now.Format(time.RFC3339),
		Task:  ctx.Task,
		Room:  ctx.Room,
		Cases: make([]renderFixtureCase, 0, len(cases)),
	}

	for _, c := range cases {
		out, err := RenderTemplate(c.Tpl, ctx)
		item := c
		switch {
		case err != nil && c.Error:
			// 符合预期：只记「要报错」，不记文案。
		case err != nil:
			t.Errorf("RenderTemplate(%q) 意外报错: %v", c.Tpl, err)
			continue
		case c.Error:
			t.Errorf("RenderTemplate(%q) 期望报错，却成功得到 %q", c.Tpl, out)
			continue
		case c.Uncertain != "":
			// 不确定变量：不记字面结果。但仍要保证它「真的出来了」—— 空串说明没求值。
			if out == "" {
				t.Errorf("RenderTemplate(%q) 用了 %s 却得到空串", c.Tpl, c.Uncertain)
				continue
			}
		default:
			got := out
			item.Expect = &got
		}
		f.Cases = append(f.Cases, item)
	}

	if os.Getenv("UPDATE_FIXTURES") == "" {
		// 校验模式：与磁盘上的 fixture 逐条比，不一致就报错（不静默覆盖）。
		onDisk, err := os.ReadFile(filepath.Join(renderFixtureDir, "render.json"))
		if err != nil {
			t.Fatalf("读不到现有 fixture（%v）—— 先跑 UPDATE_FIXTURES=1 生成一份", err)
		}
		want, _ := json.MarshalIndent(f, "", "  ")
		want = append(want, '\n')
		if string(onDisk) != string(want) {
			t.Fatalf("fixture 已过期 —— 重新生成：UPDATE_FIXTURES=1 go test ./lib -run TestRenderFixtures\n（先看 diff：输出变了等于切换前后发出去的内容变了）")
		}
		return
	}

	if err := os.MkdirAll(renderFixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRenderFixture(filepath.Join(renderFixtureDir, "render.json"), f)
	t.Logf("已写出 %d 条用例到 %s", len(f.Cases), renderFixtureDir)
}
