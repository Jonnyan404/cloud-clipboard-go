package lib

// 定时自动化的契约 fixture：cron 的**解析 / 求时刻 / 「翻译」**在 Go 下跑出来的实际结果。
//
// 为什么要有它：`core::cron` 是 P2 的第一步（`ARCHITECTURE.md` §8），而 cron 的语义里
// 全是「读代码看不出来」的边界 —— 日/周是 **OR**、`?` 等价于 `*`、`7` 也是周日、
// 带步长的范围、跨月跳步的近似、闰年、以及「不可能的日期」靠迭代上限兜底。
// 基准仍然是 `cron.go` 那 682 行自写实现：**期望值是跑出来的，不是人写的**。
//
// ⚠️⚠️ 生成时**必须固定时区**（`cronFixtureZone`），不能用 `time.Local`。
// `cron_test.go` 里的 `cronAt` 用的是 Local，而它比的是**墙上时间**所以跟时区无关；
// 但 fixture 要把时刻写成 RFC3339 交给 Rust —— 那串字符串会跟着跑它的机器漂，
// 于是「同一份用例在 UTC 与 +08:00 的机器上结论不同」。那正是这个项目最防的假绿。
//
// 用法：
//
//	cd cloud-clip && UPDATE_FIXTURES=1 go test ./lib -run TestCronFixtures
//	cd ../rust && cargo test -p clip9-core --test go_cron
//
// ⚠️ 重新生成时**先看 diff**：某个表达式的下一次时刻变了，等于「同一个任务在切换前后
// 触发的时间不一样」。那可能是修了 bug，也可能是漂了 —— 两种情况都要人来判断。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture 落点（`go test` 的 cwd 是 cloud-clip/lib）。
const cronFixtureDir = "../../cases/cron"

// 固定时区：与任务默认时区一致（`task.go` 的默认 `Asia/Shanghai`）。
// 用 FixedZone 而不是 LoadLocation("Asia/Shanghai")：不依赖机器上的 tzdata，
// 而东八区没有夏令时，两者在任何日期上都给出同一个墙上时间。
var cronFixtureZone = time.FixedZone("+08:00", 8*3600)

type cronFixtureParseInput struct {
	Expr string
	// ⚠️ 默认值的方向很重要：**「合法」是绝大多数**，所以用 `Bad` 表示「这条应当解析失败」。
	// （反过来用 `OK bool` 的话，写用例时漏一个字段就把一条合法表达式标成「期望报错」，
	//  而生成器会立刻报「期望报错却成功了」—— 我第一版就是这么写的。）
	Bad bool
	Why string
}

type cronFixtureParse struct {
	Expr string `json:"expr"`
	OK   bool   `json:"ok"`
	// Normalized 是 `ParseCron` 归一化后的表达式（多余空白压成单个空格）。
	// 只在 ok 时出现 —— 同一条表达式的两种写法不该在库里存成两份。
	Normalized string `json:"normalized,omitempty"`
	Why        string `json:"why,omitempty"`
}

type cronFixtureDescribe struct {
	Expr string `json:"expr"`
	// Desc 存的是 Go **marshal 出来的 JSON 原文**：这样连**键名与 omitempty**
	// 也一起钉住了（渲染那一页的脚本读的就是这个形状）。用结构体对比会把
	// 「`mode` 必须始终出现、`times` 没有就不出现」这类约定漏掉。
	Desc json.RawMessage `json:"desc"`
}

type cronFixtureOccurrence struct {
	Expr string `json:"expr"`
	From string `json:"from"`
	// Next / Prev 用指针：`null` 表示**算不出来**（`0 0 30 2 *` 这种语法合法但
	// 永远等不到的表达式，靠迭代上限兜底返回 false）—— 那与「没测」不是一回事。
	Next  *string  `json:"next"`
	Prev  *string  `json:"prev"`
	Next3 []string `json:"next3"`
}

type cronFixtureFile struct {
	Note        string                  `json:"note"`
	Zone        string                  `json:"zone"`
	Parse       []cronFixtureParse      `json:"parse"`
	Describe    []cronFixtureDescribe   `json:"describe"`
	Occurrences []cronFixtureOccurrence `json:"occurrences"`
}

// 解析用例：合法（附归一化结果）与非法（只记「要报错」）。
//
// ⚠️ 错误**文案**不记录：两侧措辞本就可以不同，该对齐的是「哪些输入算错」。
// 校验器「宽一点」和「严一点」都是真实的差异，所以非法那批要尽量齐。
func cronFixtureParseCases() []cronFixtureParseInput {
	return []cronFixtureParseInput{
		// ── 合法 ────────────────────────────────────────────────
		{Expr: "* * * * *"},
		{Expr: "*/15 * * * *", Why: "步长"},
		{Expr: "0/4 * * * *", Why: "`0/n` 与 `*/n` 等价 —— 导出的 crontab 里很常见"},
		{Expr: "0 9 * * *"},
		{Expr: "30 9 * * 1-5", Why: "工作日"},
		{Expr: "0 12 * * 0"},
		{Expr: "0 12 * * 7", Why: "7 也是周日，会被折进 0"},
		{Expr: "0 0 1 * *"},
		{Expr: "0 0 29 2 *", Why: "闰年 2 月 29 日：合法，只是要等"},
		{Expr: "0 9 * JAN MON", Why: "英文月份 + 星期"},
		{Expr: "0 9 * * mon-fri", Why: "小写也认"},
		{Expr: "0 9 * jan-december *", Why: "名称全拼也只取前三位"},
		{Expr: "0 */6 * * *"},
		{Expr: "15,45 9 * * *", Why: "列表"},
		{Expr: "1,15,30 * * * *"},
		{Expr: "1-30/5 * * * *", Why: "带步长的范围"},
		{Expr: "0 9 1/4 * *", Why: "从 1 号起每 4 天（日字段最小值就是 1）"},
		{Expr: "*/30 9-18 * * 1-5"},
		{Expr: "0,30 9-18 * * *"},
		{Expr: "5/10 9 * * *", Why: "起点不是最小值的步长：合法，只是描述不认"},
		{Expr: "0-30 9 * * *"},
		{Expr: "*/5 */2 */3 */2 */2", Why: "五个字段全带步长"},
		{Expr: "0,10,20,30,40,50 8,9 * * *"},
		{Expr: "  0   9  *  JAN  mon  ", Why: "多余空白 + 大小写 → 归一化"},
		{Expr: "0 9 ? * MON", Why: "`?` 等价于 `*`（Quartz 的写法）"},
		{Expr: "? ? ? ? ?"},
		{Expr: "0 9 * ? *"},
		{Expr: "0 9 * * ?"},
		{Expr: "59 23 31 12 6", Why: "各字段的上边界"},

		// ── 非法 ────────────────────────────────────────────────
		{Expr: "", Bad: true, Why: "空表达式"},
		{Expr: "   ", Bad: true, Why: "全是空白"},
		{Expr: "0 9 * *", Bad: true, Why: "只有 4 个字段"},
		{Expr: "0 9 * * * *", Bad: true, Why: "6 个字段（带秒）：最常见的粘贴错误"},
		{Expr: "0 */5 * * * *", Bad: true, Why: "同上，本意是「每 5 分钟」"},
		{Expr: "0 9 * * * * *", Bad: true, Why: "7 个字段"},
		{Expr: "60 * * * *", Bad: true, Why: "分钟越界"},
		{Expr: "0 24 * * *", Bad: true, Why: "小时越界"},
		{Expr: "0 0 0 * *", Bad: true, Why: "日为 0（最小是 1）"},
		{Expr: "0 0 32 * *", Bad: true, Why: "日越界"},
		{Expr: "0 0 * 13 *", Bad: true, Why: "月越界"},
		{Expr: "0 0 * 0 *", Bad: true, Why: "月为 0"},
		{Expr: "0 0 * * 8", Bad: true, Why: "星期越界（0-7）"},
		{Expr: "*/0 * * * *", Bad: true, Why: "步长为 0"},
		{Expr: "0/0 * * * *", Bad: true, Why: "同上，另一种写法"},
		{Expr: "*/x * * * *", Bad: true, Why: "步长不是数字"},
		{Expr: "5-1 * * * *", Bad: true, Why: "范围起点大于终点"},
		{Expr: "abc * * * *", Bad: true, Why: "既不是数字也不是名称"},
		{Expr: "1-2-3 * * * *", Bad: true, Why: "三个连字符"},
		{Expr: "MONDAYX * * * *", Bad: true, Why: "前三位也不是已知名称"},
		{Expr: "0 9 * * 1,,2", Bad: true, Why: "列表里有空项"},
		{Expr: ", * * * *", Bad: true, Why: "第一项就是空"},
		{Expr: "0 9 * JAN MON FRI", Bad: true, Why: "字段多了"},

		// ⚠️ 下面这条**合法**：`0 0 30 2 *`（2 月 30 日）语法没问题，
		// 只是永远等不到 —— 它得靠「算一次未来」在保存任务时发现，ParseCron 管不着。
		// 所以它在 occurrences 那节，`next` 为 null。
	}
}

// 「翻译」用例：期望值是 Go 的 `DescribeCron` marshal 出来的原文。
func cronFixtureDescribeCases() []string {
	return []string{
		// 形状
		"30 9 * * *",
		"0 9 * * 1",
		"0 10 * * 1-5",
		"0 8 1 * *",
		"* * * * *",
		"*/5 * * * *",
		"*/4 * * * *",
		"*/4 9 * * *",
		"0/4 * * * *",
		"* 9 * * *",
		"*/30 9-18 * * 1-5",
		"0 * * * *",
		"0,30 9-18 * * *",
		"0 9 1 * 1",
		"0 9 * 3 *",
		"0 9,18 * * *",
		"0 9 * * 7",
		"0 9 * * SUN",
		"5/10 9 * * *",
		"0-30 9 * * *",
		"0 9 */15 * *",
		"0 9 */3 * *",
		"0 9 */2 * *",
		"0 9 */3 * 1",
		"0 9 * */2 *",
		"0 9 * */3 *",
		"0 */2 * * *",
		"0 */6 * * *",
		"* */2 * * *",
		"*/5 */2 */3 */2 */2",
		"0 9 */15 * 1",
		// 截断
		"0,10,20,30,40,50 8,9 * * *",
		// 剩下的是「句子里绝不能出现表达式语法」那一族：
		// `*` / `/` / `?` 一个都不许露出来（读不出来就该 unknown）。
		"0 9 1/4 * *",
		"0 9 * 2-11/3 *",
		"15 3 1 */4 *",
		"0 9 */15 * *",
		"0 0 29 2 *",
		"0 9-18 * * *",
		"0 9-18/2 * * *",
		"0 9 * JAN MON",
		"0 9 * * MON-FRI",
		"0 9 1,15 * 1-5",
		"? 9 * * ?",
		"59 23 31 12 6",
	}
}

type cronFixtureOccurrenceInput struct {
	expr string
	from string
	why  string
}

// 求时刻用例：`next` / `prev`（不晚于）与连续 3 个。
func cronFixtureOccurrenceCases() []cronFixtureOccurrenceInput {
	return []cronFixtureOccurrenceInput{
		// ── Next ────────────────────────────────────────────────
		{"* * * * *", "2026-09-24 09:30:00", "每分钟"},
		{"*/15 * * * *", "2026-09-24 09:07:00", "步长"},
		{"0 9 * * *", "2026-09-24 08:00:00", "今天还没到"},
		{"0 9 * * *", "2026-09-24 09:00:00", "踩在点上 → 下一次（Next 是严格晚于）"},
		{"30 9 * * 1-5", "2026-09-24 09:00:00", "工作日（当天是周四）"},
		{"30 9 * * 1-5", "2026-09-24 10:00:00", "周五"},
		{"30 9 * * 1-5", "2026-09-25 10:00:00", "跨过周末到周一"},
		{"0 12 * * 0", "2026-09-24 12:00:00", "周日（0）"},
		{"0 12 * * 7", "2026-09-24 12:00:00", "周日（7 也是周日）"},
		{"0 0 1 * *", "2026-09-24 12:00:00", "每月 1 号"},
		{"0 0 29 2 *", "2026-09-24 12:00:00", "闰年 2 月 29 日（要等两年）"},
		{"0 9 * JAN MON", "2026-09-24 12:00:00", "英文月份 + 星期"},
		{"0 */6 * * *", "2026-09-24 07:30:00", "每 6 小时"},
		{"15,45 9 * * *", "2026-09-24 09:10:00", "列表"},
		{"0 0 30 2 *", "2026-09-24 12:00:00", "★ 语法合法但永远等不到 → next 为 null"},
		{"0 9 31 4 *", "2026-09-24 12:00:00", "★ 4 月没有 31 号 → 同样等不到"},
		{"0 9 1 * 1", "2026-09-24 12:00:00", "★ 日/周都是 OR：下一个周一就触发"},
		{"0 9 * * 1", "2026-09-24 12:00:00", "只限制星期时不 OR"},
		{"0 9 1 * *", "2026-09-24 12:00:00", "只限制日时不 OR"},
		{"59 23 31 12 *", "2026-09-24 12:00:00", "跨年"},

		// ── Prev（不晚于）──────────────────────────────────────
		{"0 9 * * *", "2026-09-24 15:00:00", "当天的已经过了"},
		{"0 9 * * *", "2026-09-24 08:00:00", "要退到昨天"},
		{"0 9 * * *", "2026-09-24 09:00:00", "★ 踩在点上返回它自己（调度器判到期要的就是这个）"},
		{"0 9 * * *", "2026-09-24 09:00:30", "★ 秒被丢掉：仍返回 09:00"},
		{"30 9 * * 1", "2026-09-24 12:00:00", "退回本周一"},
		{"* * * * *", "2026-09-24 09:30:45", "每分钟：退回本分钟"},
		{"0 0 1 * *", "2026-09-24 12:00:00", "退回本月 1 号"},
		{"0 0 29 2 *", "2026-09-24 12:00:00", "退回上一个闰年 2 月 29 日"},
		{"0 9 1 * 1", "2026-09-24 12:00:00", "OR 语义下退回最近的 1 号或周一"},
		{"59 23 31 12 *", "2026-09-24 12:00:00", "退回上一年 12 月 31 日"},
	}
}

// 连续 3 个时刻（给「表达式对不对」的即时预览用）。
func cronFixtureNextTimesCases() []cronFixtureOccurrenceInput {
	return []cronFixtureOccurrenceInput{
		{"0 9 * * *", "2026-09-24 12:00:00", "最常见的每日一次"},
		{"*/15 * * * *", "2026-09-24 09:07:00", "步长"},
		{"30 9 * * 1-5", "2026-09-25 10:00:00", "跨周末（周一、周二、周三）"},
		{"0 0 29 2 *", "2026-09-24 12:00:00", "闰日：三次要跨 8 年"},
		{"0 0 30 2 *", "2026-09-24 12:00:00", "★ 永远等不到 → 空列表"},
	}
}

func writeCronFixture(t *testing.T, value any) {
	t.Helper()
	if err := os.MkdirAll(cronFixtureDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	blob, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	blob = append(blob, '\n')
	path := filepath.Join(cronFixtureDir, "cron.json")
	if os.Getenv("UPDATE_FIXTURES") == "" {
		old, err := os.ReadFile(path)
		if err != nil || string(old) != string(blob) {
			t.Errorf("%s 与当前实现不一致 —— 看一眼 diff：是修了 bug，还是漂了？\n"+
				"确认无误后再用 UPDATE_FIXTURES=1 go test ./lib -run TestCronFixtures 重新生成", path)
		}
		return
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
	t.Logf("已生成 %s（%d 字节）", path, len(blob))
}

func cronFixtureAt(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04:05", value, cronFixtureZone)
	if err != nil {
		t.Fatalf("解析 %q 失败: %v", value, err)
	}
	return at
}

func cronFixtureRFC3339(t *testing.T, at time.Time, ok bool) *string {
	t.Helper()
	if !ok {
		return nil
	}
	text := at.Format(time.RFC3339)
	return &text
}

func TestCronFixtures(t *testing.T) {
	out := cronFixtureFile{
		Note: "由 cloud-clip/lib/cron_fixture_test.go 生成。期望值是 Go（cron.go）跑出来的，不是人写的；" +
			"错误文案不记录 —— 两侧措辞本就可以不同，该对齐的是「哪些输入算错」与「算出来的时刻」。",
		Zone: cronFixtureZone.String(),
	}

	// ── 1) 解析 ───────────────────────────────────────────────────
	for _, c := range cronFixtureParseCases() {
		item := cronFixtureParse{Expr: c.Expr, Why: c.Why}
		spec, err := ParseCron(c.Expr)
		switch {
		case err != nil && c.Bad:
			// 符合预期：只记「要报错」，不记文案。
		case err != nil:
			t.Errorf("ParseCron(%q) 意外报错: %v", c.Expr, err)
			continue
		case c.Bad:
			t.Errorf("ParseCron(%q) 期望报错，却成功了（归一化后 %q）", c.Expr, spec.expr)
			continue
		default:
			item.OK = true
			item.Normalized = spec.expr
		}
		out.Parse = append(out.Parse, item)
	}

	// ── 2) 「翻译」───────────────────────────────────────────────
	for _, expr := range cronFixtureDescribeCases() {
		spec, err := ParseCron(expr)
		if err != nil {
			t.Errorf("DescribeCron 的用例 %q 解析失败: %v", expr, err)
			continue
		}
		blob, err := json.Marshal(DescribeCron(spec))
		if err != nil {
			t.Fatalf("marshal desc 失败: %v", err)
		}
		out.Describe = append(out.Describe, cronFixtureDescribe{Expr: expr, Desc: json.RawMessage(blob)})
	}

	// ── 3) 求时刻 ────────────────────────────────────────────────
	addOccurrence := func(c cronFixtureOccurrenceInput, withNext3 bool) {
		spec, err := ParseCron(c.expr)
		if err != nil {
			t.Errorf("求时刻的用例 %q 解析失败: %v", c.expr, err)
			return
		}
		from := cronFixtureAt(t, c.from)
		next, okNext := spec.Next(from)
		prev, okPrev := spec.Prev(from)
		item := cronFixtureOccurrence{
			Expr:  c.expr,
			From:  from.Format(time.RFC3339),
			Next:  cronFixtureRFC3339(t, next, okNext),
			Prev:  cronFixtureRFC3339(t, prev, okPrev),
			Next3: []string{},
		}
		if withNext3 {
			for _, at := range CronNextTimes(spec, from, 3) {
				item.Next3 = append(item.Next3, at.Format(time.RFC3339))
			}
		}
		out.Occurrences = append(out.Occurrences, item)
	}
	for _, c := range cronFixtureOccurrenceCases() {
		addOccurrence(c, false)
	}
	for _, c := range cronFixtureNextTimesCases() {
		addOccurrence(c, true)
	}

	writeCronFixture(t, out)

	// ── 4) 顺手钉住两条不变量 ─────────────────────────────────────
	//
	// ① 「翻译」里绝不能出现表达式语法：`*` / `/` / `?` 一个都不许露。
	//    这是那个页面唯一不能破的规矩（「`*/3` 说成『每月 */3 日』不是描述，是把代码念了一遍」）。
	// ② 「不可能的日期」必须**能算出「算不出来」**，而不是卡住或给个错答案 ——
	//    它靠迭代上限兜底，所以顺便把耗时也看一眼（保存任务时会调它）。
	for _, item := range out.Describe {
		if got := describeSyntaxLeaks(item.Desc); len(got) > 0 {
			t.Errorf("%q 的描述里漏出了表达式语法 %v：%s", item.Expr, got, item.Desc)
		}
	}
	started := time.Now()
	spec, err := ParseCron("0 0 30 2 *")
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if _, ok := spec.Next(cronFixtureAt(t, "2026-09-24 12:00:00")); ok {
		t.Error("2 月 30 日应当算不出下一次触发时刻")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Errorf("算「不可能」用了 %s —— 迭代上限是不是没生效？", elapsed)
	}
}

// describeSyntaxLeaks 找出描述里出现的表达式语法字符。空 = 干净。
func describeSyntaxLeaks(blob json.RawMessage) []string {
	var parsed struct {
		Mode    string   `json:"mode"`
		Minutes string   `json:"minutes"`
		Hours   string   `json:"hours"`
		Times   []string `json:"times"`
		Day     string   `json:"day"`
		Dom     string   `json:"dom"`
		Month   string   `json:"month"`
	}
	if err := json.Unmarshal(blob, &parsed); err != nil {
		return []string{"<无法解析>"}
	}
	var leaks []string
	check := func(field, value string) {
		for _, bad := range []string{"*", "/", "?"} {
			if value != "" && strings.Contains(value, bad) {
				leaks = append(leaks, field+"="+value)
			}
		}
	}
	check("minutes", parsed.Minutes)
	check("hours", parsed.Hours)
	check("dom", parsed.Dom)
	check("month", parsed.Month)
	for _, at := range parsed.Times {
		check("times", at)
	}
	return leaks
}
