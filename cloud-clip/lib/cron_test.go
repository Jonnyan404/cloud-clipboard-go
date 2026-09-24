package lib

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 2026-09-24 是周四。这些测试里所有「星期几」的期望值都按真实日历核对过。
func cronBase(t *testing.T) *time.Location {
	t.Helper()
	return time.Local
}

func cronAt(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04:05", value, cronBase(t))
	if err != nil {
		t.Fatalf("解析 %q 失败: %v", value, err)
	}
	return at
}

func mustCron(t *testing.T, expr string) *cronSpec {
	t.Helper()
	spec, err := ParseCron(expr)
	if err != nil {
		t.Fatalf("ParseCron(%q) 意外失败: %v", expr, err)
	}
	return spec
}

func TestCronNext(t *testing.T) {
	cases := []struct {
		expr string
		from string
		want string
		why  string
	}{
		{"* * * * *", "2026-09-24 09:30:00", "2026-09-24 09:31:00", "每分钟"},
		{"*/15 * * * *", "2026-09-24 09:07:00", "2026-09-24 09:15:00", "步长"},
		{"0 9 * * *", "2026-09-24 08:00:00", "2026-09-24 09:00:00", "今天还没到"},
		// 恰好踩在点上时必须给**下一次**，不能是同一刻（否则幂等键会把这一趟算成已跑过）
		{"0 9 * * *", "2026-09-24 09:00:00", "2026-09-25 09:00:00", "踩在点上 → 下一次"},
		{"30 9 * * 1-5", "2026-09-24 09:00:00", "2026-09-24 09:30:00", "工作日（当天是周四）"},
		{"30 9 * * 1-5", "2026-09-24 10:00:00", "2026-09-25 09:30:00", "周五"},
		{"30 9 * * 1-5", "2026-09-25 10:00:00", "2026-09-28 09:30:00", "跨过周末到周一"},
		{"0 12 * * 0", "2026-09-24 12:00:00", "2026-09-27 12:00:00", "周日（0）"},
		{"0 12 * * 7", "2026-09-24 12:00:00", "2026-09-27 12:00:00", "周日（7 也是周日）"},
		{"0 0 1 * *", "2026-09-24 12:00:00", "2026-10-01 00:00:00", "每月 1 号"},
		{"0 0 29 2 *", "2026-09-24 12:00:00", "2028-02-29 00:00:00", "闰年 2 月 29 日"},
		{"0 9 * JAN MON", "2026-09-24 12:00:00", "2027-01-04 09:00:00", "英文月份 + 星期（2027-01-01 是周五）"},
		{"0 */6 * * *", "2026-09-24 07:30:00", "2026-09-24 12:00:00", "每 6 小时"},
		{"15,45 9 * * *", "2026-09-24 09:10:00", "2026-09-24 09:15:00", "列表"},
	}

	for _, c := range cases {
		t.Run(c.expr+" @ "+c.from, func(t *testing.T) {
			got, ok := mustCron(t, c.expr).Next(cronAt(t, c.from))
			if !ok {
				t.Fatalf("Next 没算出结果（%s）", c.why)
			}
			if want := cronAt(t, c.want); !got.Equal(want) {
				t.Fatalf("%s：Next(%s) = %s，期望 %s", c.why, c.from, got.Format("2006-01-02 15:04:05"), c.want)
			}
		})
	}
}

// 标准 cron 的「日 / 周」OR 语义。这条不直观但必须遵守 —— 按 AND 理解的话，
// `0 9 1 * 1` 一年只触发几次，用户会以为任务坏了。
func TestCronDayAndWeekdayAreOred(t *testing.T) {
	spec := mustCron(t, "0 9 1 * 1")
	got, ok := spec.Next(cronAt(t, "2026-09-24 12:00:00"))
	if !ok {
		t.Fatal("Next 没算出结果")
	}
	// OR：下一个周一（09-28）就触发。
	// 如果实现成 AND，要等到「既是 1 号又是周一」——那是 2027-02-01，差了好几个月。
	if want := cronAt(t, "2026-09-28 09:00:00"); !got.Equal(want) {
		t.Fatalf("得到 %s，期望 %s（说明「日 / 周」被按 AND 处理了）",
			got.Format("2006-01-02 15:04"), want.Format("2006-01-02 15:04"))
	}

	// 只限制一个时不该 OR：`0 9 * * 1` 只看星期
	onlyDow := mustCron(t, "0 9 * * 1")
	got, _ = onlyDow.Next(cronAt(t, "2026-09-24 12:00:00"))
	if want := cronAt(t, "2026-09-28 09:00:00"); !got.Equal(want) {
		t.Fatalf("只限制星期时得到 %s，期望 %s", got.Format("2006-01-02 15:04"), want.Format("2006-01-02 15:04"))
	}
}

func TestCronPrev(t *testing.T) {
	cases := []struct {
		expr string
		from string
		want string
	}{
		{"0 9 * * *", "2026-09-24 15:00:00", "2026-09-24 09:00:00"},
		{"0 9 * * *", "2026-09-24 08:00:00", "2026-09-23 09:00:00"},
		// Prev 是「不晚于」，踩在点上就返回它自己 —— 调度器判到期要的正是这个语义
		{"0 9 * * *", "2026-09-24 09:00:00", "2026-09-24 09:00:00"},
		{"0 9 * * *", "2026-09-24 09:00:30", "2026-09-24 09:00:00"},
		{"30 9 * * 1", "2026-09-24 12:00:00", "2026-09-21 09:30:00"},
		{"* * * * *", "2026-09-24 09:30:45", "2026-09-24 09:30:00"},
		{"0 0 1 * *", "2026-09-24 12:00:00", "2026-09-01 00:00:00"},
	}

	for _, c := range cases {
		t.Run(c.expr+" @ "+c.from, func(t *testing.T) {
			got, ok := mustCron(t, c.expr).Prev(cronAt(t, c.from))
			if !ok {
				t.Fatal("Prev 没算出结果")
			}
			if want := cronAt(t, c.want); !got.Equal(want) {
				t.Fatalf("Prev(%s) = %s，期望 %s", c.from, got.Format("2006-01-02 15:04:05"), c.want)
			}
		})
	}
}

func TestCronNextTimes(t *testing.T) {
	spec := mustCron(t, "0 9 * * *")
	got := CronNextTimes(spec, cronAt(t, "2026-09-24 12:00:00"), 3)
	if len(got) != 3 {
		t.Fatalf("应给出 3 个时刻，得到 %d", len(got))
	}
	want := []string{"2026-09-25 09:00", "2026-09-26 09:00", "2026-09-27 09:00"}
	for i, w := range want {
		if got[i].Format("2006-01-02 15:04") != w {
			t.Fatalf("第 %d 个是 %s，期望 %s", i+1, got[i].Format("2006-01-02 15:04"), w)
		}
	}
}

func TestCronRejectsBadExpressions(t *testing.T) {
	cases := []struct {
		expr    string
		wantSub string
		why     string
	}{
		{"", "不能为空", "空表达式"},
		{"0 9 * *", "5 个字段", "少一个字段"},
		// 6 字段（带秒）是最常见的粘贴错误，必须给出「怎么改」而不是只说错了
		{"0 */5 * * * *", "去掉第一段", "带秒的 6 字段"},
		{"60 * * * *", "0-59", "分钟越界"},
		{"0 24 * * *", "0-23", "小时越界"},
		{"0 0 0 * *", "1-31", "日为 0"},
		{"0 0 * 13 *", "1-12", "月越界"},
		{"0 0 * * 8", "0-7", "星期越界"},
		{"*/0 * * * *", "步长", "步长为 0"},
		{"5-1 * * * *", "起点大于终点", "反了的范围"},
		{"abc * * * *", "既不是数字也不是已知的名称", "非数字"},
	}

	for _, c := range cases {
		t.Run(c.why, func(t *testing.T) {
			_, err := ParseCron(c.expr)
			if err == nil {
				t.Fatalf("ParseCron(%q) 应当报错", c.expr)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("错误信息 %q 应包含 %q", err.Error(), c.wantSub)
			}
		})
	}
}

// `0 0 30 2 *`（2 月 30 日）语法完全合法，但永远等不到 —— 这一条只能在
// **保存任务**时靠「算一次未来」发现（见 normalizeAndValidate）。ParseCron 管不着它。
func TestCronImpossibleDateIsCaughtWhenSavingTask(t *testing.T) {
	task := testTask(automationFreqCron, "")
	task.Cron = "0 0 30 2 *"
	err := task.normalizeAndValidate()
	if err == nil {
		t.Fatal("不可能的日期组合应当在保存时被拒")
	}
	if !strings.Contains(err.Error(), "算不出任何触发时刻") {
		t.Fatalf("错误信息应说明算不出触发时刻，得到 %q", err.Error())
	}

	// 合法的闰年表达式必须放行，而且不该慢到影响保存
	ok := testTask(automationFreqCron, "")
	ok.Cron = "0 0 29 2 *"
	if err := ok.normalizeAndValidate(); err != nil {
		t.Fatalf("闰年 2 月 29 日应当合法: %v", err)
	}
	if ok.Cron != "0 0 29 2 *" {
		t.Fatalf("表达式应被原样保留，得到 %q", ok.Cron)
	}
}

func TestCronFieldNormalization(t *testing.T) {
	// 多余空白 + 大小写，落盘时都归一化 —— 同一条表达式的两种写法不该存成两份
	spec := mustCron(t, "  0   9  *  JAN  mon  ")
	if spec.expr != "0 9 * JAN mon" {
		t.Fatalf("归一化后应为 `0 9 * JAN mon`，得到 %q", spec.expr)
	}
	// `?` 按 `*` 处理（Quartz 的写法，从网上抄表达式的人常常带着它）
	quartz := mustCron(t, "0 9 ? * MON")
	if !quartz.dom.any {
		t.Fatal("`?` 应当等价于 `*`（日字段未被限制）")
	}
}

// ── 「翻译」 ────────────────────────────────────────────────────────

func TestDescribeCronShapes(t *testing.T) {
	cases := []struct {
		expr string
		want cronDescription
		why  string
	}{
		{
			expr: "30 9 * * *",
			want: cronDescription{Mode: "times", Times: []string{"09:30"}, Day: "daily"},
			why:  "最常见的写法：每天 09:30",
		},
		{
			expr: "0 9 * * 1",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "weekly", Weekdays: []int{1}},
			why:  "每周一 09:00",
		},
		{
			expr: "0 10 * * 1-5",
			want: cronDescription{Mode: "times", Times: []string{"10:00"}, Day: "weekly", Weekdays: []int{1, 2, 3, 4, 5}},
			why:  "工作日 —— 范围要展开成具体星期几，否则页面上没法翻成人话",
		},
		{
			expr: "0 8 1 * *",
			want: cronDescription{Mode: "times", Times: []string{"08:00"}, Day: "monthly", Dom: "1"},
			why:  "每月 1 日 08:00",
		},
		{
			expr: "* * * * *",
			want: cronDescription{Mode: "everyMinute", Day: "daily"},
			why:  "每分钟",
		},
		{
			expr: "*/5 * * * *",
			want: cronDescription{Mode: "everyNMinutes", N: 5, Day: "daily"},
			why:  "每 5 分钟",
		},
		{
			// `*/4` 这种步长写法必须认得（有人问过「是不是不识别」）
			expr: "*/4 * * * *",
			want: cronDescription{Mode: "everyNMinutes", N: 4, Day: "daily"},
			why:  "每 4 分钟",
		},
		{
			expr: "*/4 9 * * *",
			want: cronDescription{Mode: "everyNMinutes", N: 4, Hours: "9", Day: "daily"},
			why:  "只在 9 点，每 4 分钟（单值小时也要能说通顺）",
		},
		{
			// 和 `*/4 * * * *` 完全等价，只是 crontab 里两种写法都有人写
			expr: "0/4 * * * *",
			want: cronDescription{Mode: "everyNMinutes", N: 4, Day: "daily"},
			why:  "`0/4` 与 `*/4` 等价，不能掉进「说不准」",
		},
		{
			expr: "* 9 * * *",
			want: cronDescription{Mode: "everyMinute", Hours: "9", Day: "daily"},
			why:  "每分钟、但只在 9 点（常见的 crontab 写法，之前落到 unknown）",
		},
		{
			expr: "*/30 9-18 * * 1-5",
			want: cronDescription{Mode: "everyNMinutes", N: 30, Hours: "9-18",
				Day: "weekly", Weekdays: []int{1, 2, 3, 4, 5}},
			why: "带小时范围时必须把 9-18 说出来 —— 只说「每 30 分钟」就是撒谎",
		},
		{
			expr: "0 * * * *",
			want: cronDescription{Mode: "minutesEachHour", Minutes: "0", Day: "daily"},
			why:  "每小时的第 0 分",
		},
		{
			expr: "0,30 9-18 * * *",
			want: cronDescription{Mode: "minutesEachHour", Minutes: "0,30", Hours: "9-18", Day: "daily"},
			why:  "列表 + 小时范围：不该展开成 20 个时刻，那会把提示行撑爆",
		},
		{
			expr: "0 9 1 * 1",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "monthlyOrWeekly",
				Dom: "1", Weekdays: []int{1}},
			why: "日 + 周同时被限制：必须表达成「或」。说成 AND 会把一年几次说成一年一次",
		},
		{
			expr: "0 9 * 3 *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "daily", Month: "3"},
			why:  "只在 3 月",
		},
		{
			expr: "0 9,18 * * *",
			want: cronDescription{Mode: "times", Times: []string{"09:00", "18:00"}, Day: "daily"},
			why:  "一天两个时刻",
		},
		{
			// 7 是周日的另一种写法，得和 0 说的是同一件事
			expr: "0 9 * * 7",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "weekly", Weekdays: []int{0}},
			why:  "星期 7 = 0",
		},
		{
			expr: "0 9 * * SUN",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "weekly", Weekdays: []int{0}},
			why:  "英文星期名",
		},
		{
			// 以前这条只能返回 unknown。现在展开成具体分钟 —— 比说「每 10 分钟」准确，
			// 因为 `5/10` 是「从第 5 分起每 10 分」，正好就是这 6 个值。
			expr: "5/10 9 * * *",
			want: cronDescription{Mode: "minutesEachHour", Minutes: "5,15,25,35,45,55", Hours: "9", Day: "daily"},
			why:  "起点不是最小值的步长：展开成具体分钟，别拼成「每 10 分钟」",
		},
		{
			expr: "0-30 9 * * *",
			want: cronDescription{Mode: "minutesEachHour", Minutes: "0-30", Hours: "9", Day: "daily"},
			why:  "分钟是范围：原样念「第 0-30 分」就够清楚",
		},
	}

	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			spec, err := ParseCron(c.expr)
			if err != nil {
				t.Fatalf("解析 %q 失败: %v", c.expr, err)
			}
			got := DescribeCron(spec)
			if got.Mode != c.want.Mode {
				t.Fatalf("%s\n  Mode: 期望 %q 得到 %q", c.why, c.want.Mode, got.Mode)
			}
			if got.Day != c.want.Day {
				t.Fatalf("%s\n  Day: 期望 %q 得到 %q", c.why, c.want.Day, got.Day)
			}
			if got.N != c.want.N || got.Minutes != c.want.Minutes || got.Hours != c.want.Hours ||
				got.Dom != c.want.Dom || got.Month != c.want.Month {
				t.Fatalf("%s\n  期望 %+v\n  得到 %+v", c.why, c.want, got)
			}
			if strings.Join(got.Times, "|") != strings.Join(c.want.Times, "|") {
				t.Fatalf("%s\n  Times: 期望 %v 得到 %v", c.why, c.want.Times, got.Times)
			}
			if strings.Join(itoaAll(got.Weekdays), "|") != strings.Join(itoaAll(c.want.Weekdays), "|") {
				t.Fatalf("%s\n  Weekdays: 期望 %v 得到 %v", c.why, c.want.Weekdays, got.Weekdays)
			}
		})
	}
}

func itoaAll(values []int) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strconv.Itoa(v))
	}
	return out
}

// 时刻列表被截断时要置 More —— 否则页面会把 6 个时刻当成全部，
// 用户以为「一天就这 6 次」。
func TestDescribeCronTruncatesLongTimeLists(t *testing.T) {
	spec, err := ParseCron("0,10,20,30,40,50 8,9 * * *")
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	desc := DescribeCron(spec)
	if desc.Mode != "times" || len(desc.Times) != cronDescMaxTimes || !desc.More {
		t.Fatalf("应当截断到 %d 个并标 More，得到 %+v", cronDescMaxTimes, desc)
	}
	if desc.Times[0] != "08:00" {
		t.Fatalf("截断应从最早的时刻开始，得到 %v", desc.Times)
	}
}

// 五个字段**全都带步长**时的行为。
//
// 这是被人问出来的一个真实缺口：以前是把字段的原始 token 直接拼进句子，
// 于是 `0 9 */3 * *` 描述成「每月 */3 日 09:00」——那不是描述，是把代码念了一遍。
// 现在的规矩是：**句子里只许出现能读的东西**（`1-5` / `1,15` / 展开后的值），
// 读不出来就老实返回 unknown，让界面说「请看下面的具体时刻」。
func TestDescribeCronRejectsUnreadableFields(t *testing.T) {
	cases := []struct {
		expr string
		want cronDescription
		why  string
	}{
		{
			expr: "0 9 */15 * *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "monthly", Dom: "1,16,31"},
			why:  "`*/15` 在「日」上只有 1,16,31 三个值 —— 短到可以列出来",
		},
		{
			// 11 个日期列不全，但它是「每隔 3 天」这个常用说法，比不说强。
			// 界面上会通过预设按钮的悬停提示交代「按月算、跨月会短」。
			expr: "0 9 */3 * *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "everyNDays", DayN: 3},
			why:  "`*/3` 在「日」上列不全 → 退回「每隔 3 天」这个近似说法",
		},
		{
			expr: "0 9 */2 * *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "everyNDays", DayN: 2},
			why:  "`*/2` 同上（16 个日期，列不全）",
		},
		{
			expr: "0 9 */3 * 1",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "everyNDaysOrWeekly",
				DayN: 3, Weekdays: []int{1}},
			why: "带步长的「日」+「周」：OR 语义两边都要说",
		},
		{
			expr: "0 9 * */2 *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "daily", Month: "1,3,5,7,9,11"},
			why:  "`*/2` 在「月」上是 6 个值 —— 列出月份，别念 `*/2`",
		},
		{
			expr: "0 9 * */3 *",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "daily", Month: "1,4,7,10"},
			why:  "`*/3` 在「月」上是 1,4,7,10",
		},
		{
			expr: "0 */2 * * *",
			want: cronDescription{Mode: "everyNHours", N: 2, Minutes: "0", Day: "daily"},
			why:  "`0 */2 * * *` = 每 2 小时的第 0 分。展开成 12 个时刻反而看不懂，单独给一档",
		},
		{
			expr: "0 */6 * * *",
			want: cronDescription{Mode: "everyNHours", N: 6, Minutes: "0", Day: "daily"},
			why:  "每 6 小时",
		},
		{
			expr: "* */2 * * *",
			want: cronDescription{Mode: "unknown", Day: "daily"},
			why:  "分钟不限 + 小时带步长：没有合适的说法，老实说不准",
		},
		{
			// 五个字段全带 `/`：小时是 12 个值、日是 11 个值，都超了上限 → 必须说不准
			expr: "*/5 */2 */3 */2 */2",
			want: cronDescription{Mode: "unknown"},
			why:  "全带步长且多处过长 —— 绝不能拼出「*/2 月 每月 */3 日（只在 */2 点）」这种话",
		},
		{
			// 只把「时」换成一个能展开的小步长，「日/月/周」都能说清 → 仍然可以描述
			expr: "0 9 */15 * 1",
			want: cronDescription{Mode: "times", Times: []string{"09:00"}, Day: "monthlyOrWeekly",
				Dom: "1,16,31", Weekdays: []int{1}},
			why: "带步长的「日」+「周」，两边都读得出来就连起来说",
		},
	}

	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			spec, err := ParseCron(c.expr)
			if err != nil {
				t.Fatalf("解析 %q 失败: %v", c.expr, err)
			}
			got := DescribeCron(spec)
			if got.Mode != c.want.Mode {
				t.Fatalf("%s\n  Mode: 期望 %q 得到 %q（完整: %+v）", c.why, c.want.Mode, got.Mode, got)
			}
			if got.Mode == "unknown" {
				return
			}
			if got.Day != c.want.Day || got.N != c.want.N || got.DayN != c.want.DayN ||
				got.Minutes != c.want.Minutes || got.Dom != c.want.Dom ||
				got.Month != c.want.Month || got.Hours != c.want.Hours {
				t.Fatalf("%s\n  期望 %+v\n  得到 %+v", c.why, c.want, got)
			}
			if strings.Join(got.Times, "|") != strings.Join(c.want.Times, "|") {
				t.Fatalf("%s\n  Times: 期望 %v 得到 %v", c.why, c.want.Times, got.Times)
			}
			if strings.Join(itoaAll(got.Weekdays), "|") != strings.Join(itoaAll(c.want.Weekdays), "|") {
				t.Fatalf("%s\n  Weekdays: 期望 %v 得到 %v", c.why, c.want.Weekdays, got.Weekdays)
			}
		})
	}
}

// 不变量：**描述里不许出现表达式符号**（`*` `/` `?`）。
//
// 这是上面那类 bug 的通用防线 —— 与其逐个表达式断言「这句要长这样」，
// 不如钉住一条所有描述都必须满足的性质：句子里出现的东西一定是人能读的。
// 谁哪天又走了「把 token 直接拼进句子」的老路，这条会立刻红。
//
// 另外顺带钉住时刻的格式：必须是 HH:MM，不能把 `*/5` 之类的原文塞进 times。
func TestDescribeCronNeverLeaksExpressionSyntax(t *testing.T) {
	exprs := []string{
		"* * * * *", "*/4 * * * *", "0/4 * * * *", "5/10 9 * * *",
		"30 9 * * *", "0 10 * * 1", "0 0 1 * *", "0 9 1 * 1", "0 9 * JAN MON",
		"*/30 9-18 * * 1-5", "0,30 9-18 * * *", "0 9-18 * * *", "* 9 * * *",
		"* */2 * * *", "0 */2 * * *", "0 */6 * * *", "0 9 */2 * *", "0 9 */15 * *",
		"0 9 */3 * *", "0 9 */2 * *", "0 9 */3 * 1", "0 9 1/4 * *", "0 9 * */2 *",
		"0 9 * */3 *", "*/5 */2 */3 */2 */2",
		"0 9 */15 * 1", "0 9 1,15 * 1-5", "0 9 * 2-11/3 *", "15 3 1 */4 *",
		"0 0 29 2 *", "0 9 * * 7", "0 9 * * SUN", "0 9-18/2 * * *",
	}
	for _, expr := range exprs {
		spec, err := ParseCron(expr)
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", expr, err)
		}
		desc := DescribeCron(spec)
		for name, value := range map[string]string{
			"mode": desc.Mode, "day": desc.Day, "minutes": desc.Minutes,
			"hours": desc.Hours, "dom": desc.Dom, "month": desc.Month,
		} {
			for _, bad := range []string{"*", "/", "?"} {
				if strings.Contains(value, bad) {
					t.Errorf("%q 的 desc.%s = %q 里出现了 %q —— 描述里不能有表达式符号（完整: %+v）",
						expr, name, value, bad, desc)
				}
			}
		}
		// day 必须是已知取值之一（加了新值就该更新这里 —— 这正是它的作用）
		switch desc.Day {
		case "daily", "weekly", "monthly", "monthlyOrWeekly", "everyNDays", "everyNDaysOrWeekly":
		default:
			t.Errorf("%q 的 day 是未知取值 %q", expr, desc.Day)
		}
		// everyNDays / everyNDaysOrWeekly 必须带上 dayN
		if (desc.Day == "everyNDays" || desc.Day == "everyNDaysOrWeekly") && desc.DayN <= 0 {
			t.Errorf("%q 的 dayN 应该是正数，得到 %d", expr, desc.DayN)
		}

		// Mode 必须是已知取值之一，或者明确的 unknown
		switch desc.Mode {
		case "everyMinute", "everyNMinutes", "everyNHours", "minutesEachHour", "times", "unknown":
		default:
			t.Errorf("%q 的 mode 是未知取值 %q", expr, desc.Mode)
		}
		for _, tm := range desc.Times {
			if !regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(tm) {
				t.Errorf("%q 的 times 里有 %q，不是 HH:MM", expr, tm)
			}
		}
		for _, wd := range desc.Weekdays {
			if wd < 0 || wd > 6 {
				t.Errorf("%q 的 weekdays 里有 %d（应为 0-6）", expr, wd)
			}
		}
	}
}
