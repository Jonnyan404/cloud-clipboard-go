package lib

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 2026-09-24 是周四（用 `date -j -f %Y-%m-%d` 核对过，别凭直觉改）。
func renderBase() time.Time {
	return time.Date(2026, 9, 24, 9, 30, 0, 0, time.Local)
}

func renderMust(t *testing.T, tpl string, at time.Time) string {
	t.Helper()
	out, err := RenderTemplate(tpl, renderContext{Now: at, Task: "值班提醒", Room: "home"})
	if err != nil {
		t.Fatalf("RenderTemplate(%q) 意外报错: %v", tpl, err)
	}
	return out
}

func TestRenderTemplateVariables(t *testing.T) {
	base := renderBase()
	cases := []struct {
		tpl  string
		want string
	}{
		{"{{date}}", "2026-09-24"},
		{"{{date:+1d}}", "2026-09-25"},
		{"{{date:-1d}}", "2026-09-23"},
		{"{{date:+1w}}", "2026-10-01"},
		// 省略单位按天 —— 与动作库 date.add 的默认一致
		{"{{date:+1}}", "2026-09-25"},
		{"{{date}} 与 {{date:+1d}}", "2026-09-24 与 2026-09-25"},

		{"{{weekday}}", "周四"},
		{"{{weekday:+1d}}", "周五"},
		{"{{weekday:-1d}}", "周三"},
		{"{{weekday:+2d}}", "周六"},
		{"{{weekday:en}}", "Thursday"},
		{"{{weekday:+1d|en}}", "Friday"},
		{"{{weekday:+1d|en-short}}", "Fri"},
		{"{{weekday:zh-short}}", "四"},
		// 参数顺序可换：用户不必记「偏移在前还是样式在前」
		{"{{weekday:en|+1d}}", "Friday"},

		{"{{time}}", "09:30"},
		{"{{datetime}}", "2026-09-24 09:30"},
		{"{{task}}", "值班提醒"},
		{"{{room}}", "home"},
		{"没有变量", "没有变量"},
	}

	for _, c := range cases {
		if got := renderMust(t, c.tpl, base); got != c.want {
			t.Errorf("RenderTemplate(%q) = %q，期望 %q", c.tpl, got, c.want)
		}
	}
}

// 用户要的正是这一句：「日期取明天、周几也取明天」。
// 这条测试就是那个设计决定（偏移挂在变量自己身上，而不是共享一个「今天」）的看守。
func TestWeekdayOffsetBelongsToItsOwnVariable(t *testing.T) {
	base := renderBase()

	tpl := "今天是 {{date}}（{{weekday}}），明天是 {{date:+1d}}（{{weekday:+1d}}）"
	want := "今天是 2026-09-24（周四），明天是 2026-09-25（周五）"
	if got := renderMust(t, tpl, base); got != want {
		t.Fatalf("得到 %q，期望 %q", got, want)
	}

	// 反例：如果周几没有自己的偏移，就会得到「明天是 09-25（周四）」这种自相矛盾的文案。
	// 这里显式断言两种写法结果不同，免得将来有人「简化」成共享基准。
	withOffset := renderMust(t, "{{weekday:+1d}}", base)
	withoutOffset := renderMust(t, "{{weekday}}", base)
	if withOffset == withoutOffset {
		t.Fatalf("{{weekday:+1d}} 与 {{weekday}} 不应相等（都是 %q）", withOffset)
	}
}

func TestRenderTemplateMonthClamp(t *testing.T) {
	// 1 月 31 日 + 1 个月：2 月没有 31 号。直接加月会溢出到 3 月 3 日，
	// 正确结果是夹到 2 月最后一天 —— 前端 date.add 的 addMonths 是同一套规则。
	jan31 := time.Date(2026, 1, 31, 10, 0, 0, 0, time.Local)
	if got := renderMust(t, "{{date:+1m}}", jan31); got != "2026-02-28" {
		t.Errorf("1月31日 +1m = %q，期望 2026-02-28（2026 不是闰年）", got)
	}
	if got := renderMust(t, "{{date:+1y}}", jan31); got != "2027-01-31" {
		t.Errorf("1月31日 +1y = %q，期望 2027-01-31", got)
	}
	// 闰年二月
	leap := time.Date(2028, 1, 31, 10, 0, 0, 0, time.Local)
	if got := renderMust(t, "{{date:+1m}}", leap); got != "2028-02-29" {
		t.Errorf("2028年1月31日 +1m = %q，期望 2028-02-29", got)
	}
}

func TestRenderTemplateTimestampAndUUID(t *testing.T) {
	base := renderBase()

	// 时间戳拿 base.Unix() 比，不写死字面量 —— 写死就等于把这个测试绑在某个时区上
	if got, want := renderMust(t, "{{timestamp}}", base), strconv.FormatInt(base.Unix(), 10); got != want {
		t.Fatalf("{{timestamp}} = %q，期望 %q", got, want)
	}

	first := renderMust(t, "{{uuid}}", base)
	second := renderMust(t, "{{uuid}}", base)
	if first == second || len(first) != 36 {
		t.Fatalf("uuid 每次应不同且形如标准 UUID，得到 %q / %q", first, second)
	}
}

func TestRenderTemplateRejectsBadInput(t *testing.T) {
	base := renderBase()
	cases := []struct {
		name    string
		tpl     string
		wantSub string
	}{
		{"未知变量", "{{dtae}}", "未知变量"},
		{"未知变量（含空格）", "{{ nope }}", "未知变量"},
		{"非法偏移", "{{date:abc}}", "无法识别的日期偏移"},
		{"偏移符号在单位后", "{{date:1d+}}", "无法识别的日期偏移"},
		{"time 不接受参数", "{{time:+1d}}", "不接受参数"},
		{"uuid 不接受参数", "{{uuid:x}}", "不接受参数"},
		{"周几样式写错", "{{weekday:+1d|cn-simple}}", "无法识别"},
		{"周几参数段太多", "{{weekday:+1d|en|zh}}", "最多两段"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RenderTemplate(c.tpl, renderContext{Now: base})
			if err == nil {
				t.Fatalf("RenderTemplate(%q) 应当报错，却成功了", c.tpl)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("错误信息 %q 应包含 %q", err.Error(), c.wantSub)
			}
		})
	}
}

// 求值要么全成功、要么整体失败 —— 绝不返回「替换了一半」的正文。
// 半成品会带着 `{{dtae}}` 原样发进房间，而用户几小时后才看得到。
func TestRenderTemplateIsAllOrNothing(t *testing.T) {
	out, err := RenderTemplate("今天是 {{date}}，{{dtae}}", renderContext{Now: renderBase()})
	if err == nil {
		t.Fatalf("应当失败")
	}
	if out != "" {
		t.Fatalf("失败时不应返回半成品，得到 %q", out)
	}
}

func TestValidateTemplate(t *testing.T) {
	if err := ValidateTemplate("明天是 {{date:+1d}}（{{weekday:+1d}}）"); err != nil {
		t.Fatalf("合法模板被拒: %v", err)
	}
	if err := ValidateTemplate("{{date:+1d}} {{bogus}}"); err == nil {
		t.Fatal("含未未知变量的模板应被拒")
	}
	if err := ValidateTemplate("纯文本没有变量"); err != nil {
		t.Fatalf("纯文本模板不该出错: %v", err)
	}
}

func TestRenderVariableNamesStable(t *testing.T) {
	names := RenderVariableNames()
	if len(names) != 9 {
		t.Fatalf("变量数量应为 9，得到 %d: %v", len(names), names)
	}
	// 每一个列出的变量都必须真的能求值（防止加了名字忘了实现）。
	// latest 需要一个「来源消息读取器」，所以给它一个桩 —— 它读的是外部状态，
	// 这是全部变量里唯一一个不是纯函数的，见 renderContext.Latest。
	ctx := renderContext{
		Now:    renderBase(),
		Task:   "t",
		Room:   "home",
		Latest: func(string) (string, bool) { return "上一条消息", true },
	}
	for _, name := range names {
		if _, err := RenderTemplate("{{"+name+"}}", ctx); err != nil {
			t.Errorf("变量 %s 声明了却求不出值: %v", name, err)
		}
	}
}

// latest 的三种情形：本房间、指定房间、来源为空。
func TestRenderLatestVariable(t *testing.T) {
	seen := []string{}
	ctx := renderContext{
		Now:  renderBase(),
		Room: "home",
		Latest: func(room string) (string, bool) {
			seen = append(seen, room)
			if room == "empty" {
				return "", false
			}
			return "来自 " + room, true
		},
	}

	if got, err := RenderTemplate("{{latest}}", ctx); err != nil || got != "来自 home" {
		t.Fatalf("不带参数应当取本房间，得到 %q / %v", got, err)
	}
	if got, err := RenderTemplate("转：{{latest:ops}}", ctx); err != nil || got != "转：来自 ops" {
		t.Fatalf("带参数应当取指定房间，得到 %q / %v", got, err)
	}
	if !contains(seen, "ops") {
		t.Fatalf("应当向读取器要过 ops，实际问过：%v", seen)
	}

	// 来源为空时必须是**可识别的**错误：调度器据此记 skipped 而不是 error
	_, err := RenderTemplate("{{latest:empty}}", ctx)
	if err == nil {
		t.Fatal("空来源应当报错")
	}
	if !errors.Is(err, ErrEmptySource) {
		t.Fatalf("空来源的错误必须能被 errors.Is(ErrEmptySource) 认出来，得到 %v", err)
	}

	// 房间名带空格：直接拒，别让一个含空格的房间名悄悄变成合法的空房间
	if _, err := RenderTemplate("{{latest:my room}}", ctx); err == nil {
		t.Fatal("房间名含空格应当被拒")
	}
}

// TemplateLatestRooms 是 HTTP 层做权限校验的依据，必须把每个引用到的房间都列出来。
func TestTemplateLatestRooms(t *testing.T) {
	bare, rooms := TemplateLatestRooms("前言 {{date}} {{latest}} 中 {{latest:ops}} 后 {{latest:ops}} {{latest: lobby }}")
	if !bare {
		t.Error("应当识别出不带参数的 {{latest}}")
	}
	if len(rooms) != 2 || rooms[0] != "ops" || rooms[1] != "lobby" {
		t.Fatalf("应当列出 ops 与 lobby 各一次，得到 %v", rooms)
	}
	if bare2, rooms2 := TemplateLatestRooms("没有变量"); bare2 || len(rooms2) != 0 {
		t.Fatalf("没有 latest 时应当两个都为空，得到 %v / %v", bare2, rooms2)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
