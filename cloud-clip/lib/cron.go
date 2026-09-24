package lib

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

/**
*** FILE: cron.go
***   标准 5 字段 cron 表达式：解析、匹配、前后求时刻。
**/

// 为什么自己写而不是引一个 cron 库：
//  1. 我们只要「算下一次 / 上一次时刻」这两个纯函数，不要调度器、不要 Job、不要日志钩子
//     （调度由 scheduler.go 统一做，它已经处理了幂等和补发窗口）；
//  2. 「上一次时刻」不是 cron 库的常见能力，但调度器判到期**必须**有它
//     （见 task.go 里 dueOccurrence 那段论证）；
//  3. 字段格式和错误信息要能直接讲给用户听（「分钟只能 0-59」比 "invalid expression" 有用）。
//
// 支持的语法（够覆盖从 crontab 抄来的绝大多数表达式）：
//
//   - ?       任意（`?` 是 Quartz 的写法，抄过来的人常常带着它）
//     5                 单个值
//     1-5               范围
//     */10              步长
//     1-30/5            带步长的范围
//     1,15,30           列表（列表项本身也可以是范围或带步长）
//     MON-FRI  JAN-DEC  月份 / 星期的英文前三个字母（大小写都认）
//
// ⚠️ 只认 **5 个字段**（分 时 日 月 周）。带秒的 6 字段表达式会被明确拒绝并提示怎么改 ——
// 静默忽略第一段会让 `0 */5 * * * *`（本意每 5 分钟）变成「每小时第 0 分」，
// 而用户要过一整天才会发现少发了。
const (
	cronFieldCount = 5

	// 求下一个 / 上一个时刻时的迭代上限。因为下面用了「整段跳过」（月份不对就跳到下个月），
	// 实际迭代次数远小于这个值 —— 它只是兜底，防止写坏的表达式把调度器卡死。
	cronSearchLimit = 200000
)

type cronField struct {
	set map[int]bool
	// any 表示这个字段没有限制（`*` 或 `?`）。日 / 周的 OR 语义要用到它，
	// 光看 set 是分不出 `*` 和显式写全的。
	any bool
}

type cronSpec struct {
	expr   string
	minute cronField
	hour   cronField
	dom    cronField
	month  cronField
	dow    cronField
}

var (
	cronMonthNames = map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}
	cronDowNames = map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
	}
)

// ParseCron 解析一个 5 字段 cron 表达式。
func ParseCron(expr string) (*cronSpec, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) == 0 {
		return nil, fmt.Errorf("cron 表达式不能为空")
	}
	if len(fields) == 6 {
		return nil, fmt.Errorf("只支持 5 个字段（分 时 日 月 周），收到的看起来带「秒」："+
			"请去掉第一段，例如 `0 */5 * * * *` 应写成 `*/5 * * * *`（%q）", strings.TrimSpace(expr))
	}
	if len(fields) != cronFieldCount {
		return nil, fmt.Errorf("cron 表达式需要 5 个字段（分 时 日 月 周），收到 %d 个：%q",
			len(fields), strings.TrimSpace(expr))
	}

	spec := &cronSpec{expr: strings.Join(fields, " ")}

	var err error
	if spec.minute, err = parseCronField(fields[0], 0, 59, nil, "分钟"); err != nil {
		return nil, err
	}
	if spec.hour, err = parseCronField(fields[1], 0, 23, nil, "小时"); err != nil {
		return nil, err
	}
	if spec.dom, err = parseCronField(fields[2], 1, 31, nil, "日"); err != nil {
		return nil, err
	}
	// 星期字段上界取 7：0 和 7 都是周日（两种写法在 crontab 里都常见）
	if spec.month, err = parseCronField(fields[3], 1, 12, cronMonthNames, "月"); err != nil {
		return nil, err
	}
	if spec.dow, err = parseCronField(fields[4], 0, 7, cronDowNames, "星期"); err != nil {
		return nil, err
	}
	if spec.dow.set[7] {
		spec.dow.set[0] = true
	}
	return spec, nil
}

// parseCronField 解析单个字段。
//
// label 只用于错误信息：用户看到「分钟只能 0-59」才知道改哪里，
// 看到「字段取值超范围」还得自己猜是哪个字段。
func parseCronField(raw string, min, max int, names map[string]int, label string) (cronField, error) {
	field := cronField{set: map[int]bool{}}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return field, fmt.Errorf("%s字段不能为空", label)
	}
	if trimmed == "*" || trimmed == "?" {
		field.any = true
		for v := min; v <= max; v++ {
			field.set[v] = true
		}
		return field, nil
	}

	for _, part := range strings.Split(trimmed, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return field, fmt.Errorf("%s字段里有空的列表项：%q", label, raw)
		}

		step := 1
		rangePart := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			rangePart = strings.TrimSpace(part[:idx])
			stepRaw := strings.TrimSpace(part[idx+1:])
			n, err := strconv.Atoi(stepRaw)
			if err != nil || n <= 0 {
				return field, fmt.Errorf("%s字段的步长 %q 不是正整数", label, stepRaw)
			}
			step = n
		}

		start, end := min, max
		switch {
		case rangePart == "*" || rangePart == "?":
			// 保持 min..max
		case strings.Contains(rangePart, "-"):
			segs := strings.SplitN(rangePart, "-", 2)
			s, err := cronValue(segs[0], names)
			if err != nil {
				return field, fmt.Errorf("%s字段：%v", label, err)
			}
			e, err := cronValue(segs[1], names)
			if err != nil {
				return field, fmt.Errorf("%s字段：%v", label, err)
			}
			start, end = s, e
		default:
			v, err := cronValue(rangePart, names)
			if err != nil {
				return field, fmt.Errorf("%s字段：%v", label, err)
			}
			start, end = v, v
			// `a/n` 按标准 cron 理解成 `a-max/n`（不是「从 a 开始走 n 步到 a」）
			if step > 1 {
				end = max
			}
		}

		if start < min || end > max {
			return field, fmt.Errorf("%s字段的取值必须在 %d-%d 之间：%q", label, min, max, part)
		}
		if start > end {
			return field, fmt.Errorf("%s字段的范围起点大于终点：%q", label, part)
		}
		for v := start; v <= end; v += step {
			field.set[v] = true
		}
	}

	if len(field.set) == 0 {
		return field, fmt.Errorf("%s字段没有匹配任何值：%q", label, raw)
	}
	return field, nil
}

func cronValue(raw string, names map[string]int) (int, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return 0, fmt.Errorf("空的取值")
	}
	if names != nil {
		if v, ok := names[s]; ok {
			return v, nil
		}
		// 允许 `JAN-DEC` 这种写法的名字，也允许完整单词 `MONDAY` 的前三位
		if len(s) > 3 {
			if v, ok := names[s[:3]]; ok {
				return v, nil
			}
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q 既不是数字也不是已知的名称", raw)
	}
	return v, nil
}

// dayMatches 处理标准 cron 里那条不直观但必须遵守的规则：
//
//	「日」和「星期」都**被限制**时，任一个匹配就算匹配（OR）；
//	只限制了一个时，只看那一个。
//
// 为什么不能按 AND 理解：写 `0 9 1 * 1` 的人想表达的是「每月 1 号**或**每周一 9 点」。
// 按 AND 的话只有「既是 1 号又是周一」才触发，一年也就几次 —— 用户会以为任务坏了。
func (c *cronSpec) dayMatches(t time.Time) bool {
	if c.dom.any && c.dow.any {
		return true
	}
	if c.dom.any {
		return c.dow.set[int(t.Weekday())]
	}
	if c.dow.any {
		return c.dom.set[t.Day()]
	}
	return c.dom.set[t.Day()] || c.dow.set[int(t.Weekday())]
}

// Next 返回严格晚于 after 的下一次触发时刻。
//
// ⚠️ 逐分钟硬扫是不可行的：`0 0 29 2 *`（闰年 2 月 29 日）要扫四年。
// 所以每一层不匹配就**整段跳过** —— 月不对跳到下月、日不对跳到明天、
// 时不对跳到下一个整点。最坏情况的迭代次数因此被压到「年 × 12 + 年 × 366 + 日 × 24 + 60」这个量级。
func (c *cronSpec) Next(after time.Time) (time.Time, bool) {
	t := truncateToMinute(after).Add(time.Minute)
	for i := 0; i < cronSearchLimit; i++ {
		if !c.month.set[int(t.Month())] {
			t = startOfNextMonth(t)
			continue
		}
		if !c.dayMatches(t) {
			t = startOfNextDay(t)
			continue
		}
		if !c.hour.set[t.Hour()] {
			t = startOfNextHour(t)
			continue
		}
		if !c.minute.set[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// Prev 返回不晚于 before 的最近一次触发时刻。
//
// 调度器判到期必须用到它：用「下一次」判断会让错过的那些**永远没人发现**
// （now 一直在往后走，每次都算出下一个未来时刻）。见 task.go 的 dueOccurrence。
func (c *cronSpec) Prev(before time.Time) (time.Time, bool) {
	t := truncateToMinute(before)
	for i := 0; i < cronSearchLimit; i++ {
		if !c.month.set[int(t.Month())] {
			t = endOfPrevMonth(t)
			continue
		}
		if !c.dayMatches(t) {
			t = endOfPrevDay(t)
			continue
		}
		if !c.hour.set[t.Hour()] {
			t = endOfPrevHour(t)
			continue
		}
		if !c.minute.set[t.Minute()] {
			t = t.Add(-time.Minute)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// CronNextTimes 连续算 count 个未来时刻，给「表达式对不对」的即时预览用。
//
// ⚠️ 界面上的「翻译」只是辅助，**具体时刻列表不能省** —— 描述是对表达式的归纳，
// 归纳总有说不全的时候（`*/5 9-18 * * 1-5` 说成「每 5 分钟」就漏了 9-18 点这个限制）。
// 两者并排给，用户核对的是时刻，翻译只用来第一眼判断方向对不对。
func CronNextTimes(spec *cronSpec, from time.Time, count int) []time.Time {
	if spec == nil || count <= 0 {
		return nil
	}
	out := make([]time.Time, 0, count)
	cursor := from
	for i := 0; i < count; i++ {
		next, ok := spec.Next(cursor)
		if !ok {
			break
		}
		out = append(out, next)
		cursor = next
	}
	return out
}

// ── 时刻运算：一律**按日历字段**构造，不用 Truncate ──────────────────
//
// Truncate 是按「相对 Unix 纪元」截断的，在 UTC 里正好是整点/整分，
// 但在 +05:30 这类半小时偏移的时区里会把结果挪到 :30 —— 那是个只在别人机器上
// 复现的 bug。按字段构造没有这个问题。

func truncateToMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}

func startOfNextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(time.Hour)
}

func startOfNextDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

func startOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
}

func endOfPrevHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(-time.Minute)
}

func endOfPrevDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Add(-time.Minute)
}

func endOfPrevMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Add(-time.Minute)
}

// ── 「翻译」：把表达式归纳成一句人话 ────────────────────────────────
//
// ⚠️ 返回的是**结构化的形状 + 参数，不是成句的文案**。
// 管理页有 zh / zh-TW / en / ja 四份语言，服务端要是在这里拼一句中文，
// 英文和日文用户就会看到一句中文 —— 那比没有翻译还糟。
// 所以这里只回答「长什么样」，句子交给页面用它自己的文案表拼。
//
// 归纳不出来时 Mode = "unknown"，页面退回「只看具体时刻」——
// 宁可说「这个表达式太复杂」，也不要编一句听起来对、实际错的话。
type cronDescription struct {
	// everyMinute / everyNMinutes / everyNHours / minutesEachHour / times / unknown
	//
	// unknown 是**正常输出之一**，不是错误：归纳不出来时页面会说「请看下面的具体时刻」，
	// 那比编一句听起来对、实际错的话好。
	Mode string `json:"mode"`

	N int `json:"n,omitempty"` // everyNMinutes

	// minutesEachHour：原样回显的分钟写法（可能是 "0,30" 这样的列表）
	Minutes string `json:"minutes,omitempty"`

	// Hours 非空 = 时刻被限制在这些小时里，原样回显（"9-18" / "9,12,18"）
	Hours string `json:"hours,omitempty"`

	// times：展开成具体时刻 HH:MM。超过 cronDescMaxTimes 会被截断并置 More。
	Times []string `json:"times,omitempty"`
	More  bool     `json:"more,omitempty"`

	// daily / weekly / monthly / monthlyOrWeekly
	//
	// ⚠️ 「日」和「周」同时被限制时必须是 monthlyOrWeekly —— 那是 OR 语义
	// （每月 1 号**或**每周一），说成 AND 就把一年几次说成一年一次了。
	// daily / weekly / monthly / monthlyOrWeekly / everyNDays / everyNDaysOrWeekly
	Day      string `json:"day"`
	Weekdays []int  `json:"weekdays,omitempty"` // 0 = 周日
	Dom      string `json:"dom,omitempty"`      // 原样回显

	// DayN：「每隔 N 天」里的 N。**不能和 N 共用一个字段** ——
	// `0 */2 */3 * *` 同时有「每 2 小时」和「每隔 3 天」，两个 N 都得在。
	DayN int `json:"dayN,omitempty"`

	Month string `json:"month,omitempty"` // 非空 = 只在某些月份
}

// cronDescMaxTimes 展开成具体时刻时的上限。超了就截断 ——
// `0,30 9-18 * * *` 能展开出 20 个时刻，全列出来会把提示行撑爆。
const cronDescMaxTimes = 6

func DescribeCron(spec *cronSpec) cronDescription {
	desc := cronDescription{Mode: "unknown", Day: "daily"}
	if spec == nil {
		return desc
	}
	fields := strings.Fields(spec.expr)
	if len(fields) != cronFieldCount {
		return desc
	}
	minTok, hourTok, domTok, monthTok := fields[0], fields[1], fields[2], fields[3]

	// ── 日期部分 ──────────────────────────────────────────────────
	//
	// ⚠️ 每个要在句子里露面的字段都必须先过 cronFieldText：它保证句子里出现的是
	// 「1-5」「1,15」这种能读的东西，而不是 `*/3` 这种**代码**。
	// 过去是把原始 token 直接拼进句子，于是 `0 9 */3 * *` 会描述成「每月 */3 日 09:00」——
	// 那不是描述，是把表达式念了一遍。读不出来就返回 unknown，页面会说「看具体时刻」。
	monthText, monthOK := cronFieldText(spec.month, monthTok)

	// 「日」字段带步长 → 「每隔 N 天」。
	//
	// ⚠️ 这是个**近似说法**，界面上必须交代清楚：`*/3` 落在「日」上展开是
	// 1,4,7,…,31，跨月时会从 31 直接跳到下月 1 号 —— 间隔只有 1 天，不是 3 天。
	// 标准 5 字段 cron 表达不出真正等距的「每 N 天」，所以这里只能给最接近的说法，
	// 把「按月算」这件事放进客户端的悬停提示里（见 automation_page.html 的预设按钮）。
	// 日的字段最小值是 1，所以 `1/n` 与 `*/n` 等价。
	domStep := cronStepToken(domTok, 1)

	switch {
	case spec.dom.any && spec.dow.any:
		desc.Day = "daily"
	case spec.dom.any:
		desc.Day = "weekly"
		desc.Weekdays = cronWeekdayList(spec.dow)
	case spec.dow.any:
		// 先试**精确**的写法：展开后短就列出具体日期（`*/15` → 「每月 1,16,31 日」）。
		// 列不全（`*/3` 是 11 个）才退回近似说法「每隔 N 天」——
		// 近似说法能覆盖任意步长，但跨月那次间隔会短，所以它排在精确写法后面。
		if domText, domOK := cronFieldText(spec.dom, domTok); domOK {
			desc.Day = "monthly"
			desc.Dom = domText
			break
		}
		if domStep <= 0 {
			return desc
		}
		desc.Day = "everyNDays"
		desc.DayN = domStep
	default:
		// 「日」和「周」都被限制 = OR 语义，两边都得说得出来
		if domText, domOK := cronFieldText(spec.dom, domTok); domOK {
			desc.Day = "monthlyOrWeekly"
			desc.Dom = domText
		} else if domStep > 0 {
			desc.Day = "everyNDaysOrWeekly"
			desc.DayN = domStep
		} else {
			return desc
		}
		desc.Weekdays = cronWeekdayList(spec.dow)
	}
	if !spec.month.any {
		if !monthOK {
			return desc
		}
		desc.Month = monthText
	}

	// ── 时刻部分 ──────────────────────────────────────────────────
	minAny, hourAny := cronTokenIsAny(minTok), cronTokenIsAny(hourTok)

	switch {
	case minAny && hourAny:
		desc.Mode = "everyMinute"

	// 「每分钟」但只在某些小时里 —— `* 9 * * *` 是很常见的 crontab 写法
	case minAny:
		hours, ok := cronFieldText(spec.hour, hourTok)
		if !ok {
			return desc
		}
		desc.Mode = "everyMinute"
		desc.Hours = hours

	default:
		// 分钟带步长：`*/n` / `0/n` → 每 n 分钟
		if n := cronStepToken(minTok, 0); n > 0 {
			// ⚠️ 先把要用的都算出来、**全部可读**了再写 desc。
			// 之前是先写 Mode 再校验 hours，hours 不可读时直接 return —— 于是
			// `*/5 */2 * * *` 会输出「每 5 分钟」，把「只在每 2 小时」这个限制**整段丢掉**。
			// 那不是「说不准」，那是说了一句错话。
			hours := ""
			if !hourAny {
				text, ok := cronFieldText(spec.hour, hourTok)
				if !ok {
					return desc
				}
				hours = text
			}
			desc.Mode = "everyNMinutes"
			desc.N = n
			desc.Hours = hours
			break
		}

		// 小时带步长：`0 */2 * * *` = 每 2 小时的第 0 分。
		// 单独给一个 mode，因为「每 2 小时」是个完整、有用的说法 ——
		// 展开成 12 个时刻去数反而看不懂。
		if n := cronStepToken(hourTok, 0); n > 0 {
			if minAny {
				return desc // 「每 2 小时里的每分钟」不值得为它多造一个句子，老实说不准
			}
			mins, ok := cronFieldText(spec.minute, minTok)
			if !ok {
				return desc
			}
			desc.Mode = "everyNHours"
			desc.N = n
			desc.Minutes = mins
			break
		}

		// 分钟、小时都不带步长：能翻的两种说法
		mins, minsPlain := cronPlainValues(minTok, 0, 59)
		hours, hoursPlain := cronPlainValues(hourTok, 0, 23)

		if hourAny {
			// 小时不限 → 「每小时的第 M 分」
			minsText, ok := cronFieldText(spec.minute, minTok)
			if !ok {
				return desc
			}
			desc.Mode = "minutesEachHour"
			desc.Minutes = minsText
			break
		}

		// 两边都是枚举值 → 展开成 HH:MM（「每天 09:30」这种最常见的样子）
		if hoursPlain && minsPlain {
			times := make([]string, 0, len(hours)*len(mins))
			for _, h := range hours {
				for _, m := range mins {
					times = append(times, fmt.Sprintf("%02d:%02d", h, m))
				}
			}
			if len(times) > cronDescMaxTimes {
				times = times[:cronDescMaxTimes]
				desc.More = true
			}
			desc.Mode = "times"
			desc.Times = times
			break
		}

		// 小时是范围 / 列表 / 短展开 → 「每小时的第 M 分（只在 … 点）」
		minsText, minsOK := cronFieldText(spec.minute, minTok)
		hoursText, hoursOK := cronFieldText(spec.hour, hourTok)
		if !minsOK || !hoursOK {
			return desc
		}
		desc.Mode = "minutesEachHour"
		desc.Minutes = minsText
		desc.Hours = hoursText
	}
	return desc
}

// cronHasLetter 判断字段写法里有没有字母（月份 / 星期的英文名）。
// 有字母就走展开，别把 JAN 这样的名字直接念进句子里。
func cronHasLetter(tok string) bool {
	for _, r := range tok {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// cronDescMaxList 展开成列表时的上限。
//
// 上限存在的意义不是怕长，而是**超过它就不该叫描述了**：
// `*/3` 在「日」上展开是 1,4,7,…,31 共 11 个 —— 罗列 11 个日期读起来比表达式还费劲，
// 那时候说「说不准，请看具体时刻」才是诚实的。
const cronDescMaxList = 6

// cronFieldText 把一个字段写成人能读的一小段，读不出来就返回 false。
//
// 三种能读的写法直接原样用：单值 `9`、列表 `1,15`、范围 `9-18`。
// 带步长的（`*/2`）展开成具体值 —— 短就列出来（`*/2` 在「月」上是 1,3,5,7,9,11），
// 长就放弃。**绝不把 `*/2` 这样的原文放进句子里**：那不是描述，是把代码念了一遍。
func cronFieldText(field cronField, raw string) (string, bool) {
	if field.any {
		return "", true
	}
	tok := strings.TrimSpace(raw)
	// 数字以外的写法（步长 */2、月份名 JAN）都走展开这条路：
	// 原样放进句子会得到「JAN 月」「*/2 月」这种东西。
	if !strings.Contains(tok, "/") && !cronHasLetter(tok) {
		return tok, true
	}
	// 0..60 能覆盖所有字段的取值范围（分 0-59 / 时 0-23 / 日 1-31 / 月 1-12 / 周 0-7）
	values := make([]int, 0, len(field.set))
	for v := 0; v <= 60; v++ {
		if field.set[v] {
			values = append(values, v)
		}
	}
	if len(values) == 0 || len(values) > cronDescMaxList {
		return "", false
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ","), true
}

func cronTokenIsAny(tok string) bool {
	return tok == "*" || tok == "?"
}

// cronStepToken 认出「从字段最小值开始、每 n 走一步」这一种形状，返回 n；否则返回 0。
//
// 认三种写法，它们其实**完全等价**：`*/n`、`?/n`、以及 `0/n`（字段最小值/n）。
// 最后一种很容易被忽略 —— 导出的 crontab 里 `0/4` 和 `*/4` 一样常见，
// 不认它就会掉进「说不准」，用户会以为描述功能坏了。
//
// 起点**不是**最小值的（`5/10`）仍然留给未知分支：它是「从第 5 分起每 10 分」，
// 说成「每 10 分钟」会漏掉「从 5 开始」这半句。
func cronStepToken(tok string, fieldMin int) int {
	idx := strings.Index(tok, "/")
	if idx <= 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(tok[idx+1:]))
	if err != nil || n <= 0 {
		return 0
	}
	switch head := strings.TrimSpace(tok[:idx]); head {
	case "*", "?":
		return n
	default:
		if v, err := strconv.Atoi(head); err == nil && v == fieldMin {
			return n
		}
	}
	return 0
}

// cronPlainValues 只接受「逗号分隔的单个数字」这种最朴素的写法。
//
// 刻意不认范围和步长：`9-18` 展开成 10 个时刻再拼成一句话，不如直接说「9-18 点」。
// 区分开之后，「展开成具体时刻」这条路就只走那些真的能列清楚的表达式。
func cronPlainValues(tok string, min, max int) ([]int, bool) {
	seen := map[int]bool{}
	out := make([]int, 0, 4)
	for _, item := range strings.Split(tok, ",") {
		item = strings.TrimSpace(item)
		if item == "" || strings.ContainsAny(item, "-/") {
			return nil, false
		}
		v, err := strconv.Atoi(item)
		if err != nil || v < min || v > max {
			return nil, false
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Ints(out)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// cronWeekdayList 取星期字段里被选中的值，0 = 周日。
// 7 是周日的另一种写法，ParseCron 已把它折进 0，这里只需跳过 7 本身。
func cronWeekdayList(field cronField) []int {
	out := make([]int, 0, 7)
	for v := 0; v <= 6; v++ {
		if field.set[v] {
			out = append(out, v)
		}
	}
	return out
}
