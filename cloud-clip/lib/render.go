package lib

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

/**
*** FILE: render.go
***   定时任务的正文模板 —— 把「写死在正文里的日期」换成触发那一刻才算的变量。
**/

// 为什么需要这一层：定时任务的正文里必须有「会变的部分」。用户写的是
// 「明天是几号、周几」，而「明天」只有在触发那一刻才知道。所以正文存成模板，
// 求值推迟到投递前 —— 也正因为如此，任务定义里**不能**存一段代码或一次求值结果。
//
// ⚠️ 偏移量挂在**变量自己**身上（{{weekday:+1d}}），而不是全局共享一个「今天」。
// 原因很实际：用户写 `{{date:+1d}}` 想要的是「明天是几号、明天是周几」，
// 如果周几取的是今天，文案会自相矛盾，而且这种错很难一眼看出来。
//
// 偏移语法**直接沿用动作库 date.add 的 `[+-]N[dwmy]`**，不新造一套：
// 同一个用户在预览区用的和在这里写的是同一种写法，少一份要记的规则。
const (
	renderVarDate      = "date"      // 2026-09-25
	renderVarWeekday   = "weekday"   // 周五 / 五 / Friday / Fri
	renderVarTime      = "time"      // 09:30
	renderVarDatetime  = "datetime"  // 2026-09-25 09:30
	renderVarTimestamp = "timestamp" // 1758760200
	renderVarUUID      = "uuid"
	renderVarTask      = "task" // 任务名（便于一条正文被多个任务复用）
	renderVarRoom      = "room"
	// renderVarLatest 是**唯一一个会读外部状态**的变量：取某个房间里最新一条人发的文本。
	// 它没有破坏「引擎是纯的」这条性质 —— 读消息队列这件事由调用方注入（见 renderContext.Latest），
	// 单测照样不启服务器就能覆盖求值。
	renderVarLatest = "latest" // {{latest}} / {{latest:房间名}}
)

// ErrEmptySource 「输入源现在是空的」—— 它不是模板写错，而是这个时刻没有素材。
//
// 调度器据此记 skipped 而不是 error：空房间是常态（没人发言、或消息都被顶掉了），
// 记成 error 会让「上次失败」长期挂在那条任务上，用户以为功能坏了。
var ErrEmptySource = errors.New("来源房间没有可用消息")

var (
	// {{name}} 或 {{name:参数}}。参数用 [^}]* 而不是 \w，因为它要装下 "+1d|en" 这种复合写法。
	renderVarRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*([^}]*?))?\s*\}\}`)

	// 偏移：+1d / -2w / +3m / +1y（单位和符号都可省，省了按天）。
	dateOffsetRe = regexp.MustCompile(`^([+-])?\s*(\d+)\s*([dwmy]?)$`)

	// date.add 的输入约定：`基准 运算符 数量 单位`，基准可省（省了用 ctx.Now）。
	dateAddInputRe = regexp.MustCompile(`^(.*?)\s*([+-])\s*(\d+)\s*([dwmy]?)\s*$`)

	// 日期间隔的两个操作数之间的分隔符，和前端 date.diff 保持一致。
	dateDiffSepRe = regexp.MustCompile(`\s*(?:~|～|→|->|至|到|\.\.+)\s*`)
)

// renderContext 一次求值需要的全部外部状态。
//
// ⚠️ Now 是**预定触发时刻**，不是「实际发送时刻」。服务重启 / 机器合盖导致的补发，
// 正文里写的仍然是那个「本该发出的时刻」—— 用户写 `{{date:+1d}}` 指的是那一天的明天，
// 补发时换成 now 会得到一条内容不对但看起来正常的消息。差异靠消息上的 Late 标记体现。
type renderContext struct {
	Now  time.Time
	Task string
	Room string

	// Latest 由调用方注入：给房间名，返回该房间最新一条**人发的**文本。
	// 引擎自己不碰消息队列 —— 保持「只依赖入参和时钟」这条性质，
	// 读外部状态这件事留在调用方（见 automation_source.go）。
	// 为 nil 时 {{latest}} 会报错 —— 宁可报错也不要静默留下一个空替换。
	Latest func(room string) (string, bool)
}

// RenderVariableNames 返回所有可用变量名（不含参数）。给 /server 下发、给管理页渲染胶囊用。
func RenderVariableNames() []string {
	return []string{
		renderVarDate, renderVarWeekday, renderVarTime, renderVarDatetime,
		renderVarTimestamp, renderVarUUID, renderVarTask, renderVarRoom, renderVarLatest,
	}
}

// RenderTemplate 把模板里的变量全部换掉。
//
// 语义：**任一变量求不出值就整体失败**，不返回「替换了一半」的正文。
// 静默放过未知变量会得到一条带着 `{{dtae}}` 原样发进房间的消息 ——
// 用户只有在读到自己房间里的乱码时才知道写错了，而那是几小时之后的事。
func RenderTemplate(tpl string, ctx renderContext) (string, error) {
	matches := renderVarRe.FindAllStringSubmatchIndex(tpl, -1)
	if len(matches) == 0 {
		return tpl, nil
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(tpl[last:m[0]])

		name := tpl[m[2]:m[3]]
		param := ""
		if m[4] >= 0 {
			param = tpl[m[4]:m[5]]
		}

		value, err := resolveRenderVariable(name, param, ctx)
		if err != nil {
			return "", err
		}
		b.WriteString(value)
		last = m[1]
	}
	b.WriteString(tpl[last:])
	return b.String(), nil
}

// ValidateTemplate 保存前先跑一遍求值，把「变量名写错 / 偏移写错」挡在创建那一刻。
//
// 用一个**固定时刻**求值：这里只关心模板本身是否可用，不关心结果是几号。
// 放在保存时而不是首次触发时校验 —— 后者意味着用户要等一整个周期才知道自己写错了。
func ValidateTemplate(tpl string) error {
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	// {{latest}} 给个桩：保存时要校验的是**模板写法**，
	// 而「那个房间现在有没有消息」是运行时的事 —— 谁也不能保证明天早上它还有消息。
	// 真正要拦的是「引用了你没有权限读的房间」，那件事在 HTTP 层用请求凭据判（见 automation_source.go）。
	_, err := RenderTemplate(tpl, renderContext{
		Now:    fixed,
		Latest: func(string) (string, bool) { return "x", true },
	})
	return err
}

// TemplateLatestRooms 列出模板里引用到的来源房间，供 HTTP 层做权限校验。
//
// 返回 (是否用了不带参数的 {{latest}}, 显式指定的房间名列表)。
// 不带参数的那种用的是**任务自己的房间**，调用方不用再判权限。
func TemplateLatestRooms(tpl string) (bareRoom bool, rooms []string) {
	seen := map[string]bool{}
	for _, m := range renderVarRe.FindAllStringSubmatch(tpl, -1) {
		if strings.ToLower(strings.TrimSpace(m[1])) != renderVarLatest {
			continue
		}
		param := strings.TrimSpace(m[2])
		if param == "" {
			bareRoom = true
			continue
		}
		room := normalizeRoomName(param)
		if room == "" || seen[room] {
			continue
		}
		seen[room] = true
		rooms = append(rooms, room)
	}
	return bareRoom, rooms
}

func resolveRenderVariable(name, param string, ctx renderContext) (string, error) {
	param = strings.TrimSpace(param)

	switch strings.ToLower(name) {
	case renderVarDate:
		base, err := applyDateOffset(ctx.Now, param, true)
		if err != nil {
			return "", err
		}
		return base.Format("2006-01-02"), nil

	case renderVarWeekday:
		offset, style, err := parseWeekdayParam(param)
		if err != nil {
			return "", err
		}
		base, err := applyDateOffset(ctx.Now, offset, true)
		if err != nil {
			return "", err
		}
		return formatWeekday(base.Weekday(), style)

	case renderVarTime:
		if param != "" {
			return "", fmt.Errorf("变量 time 不接受参数（收到 %q）", param)
		}
		return ctx.Now.Format("15:04"), nil

	case renderVarDatetime:
		if param != "" {
			return "", fmt.Errorf("变量 datetime 不接受参数（收到 %q）", param)
		}
		return ctx.Now.Format("2006-01-02 15:04"), nil

	case renderVarTimestamp:
		if param != "" {
			return "", fmt.Errorf("变量 timestamp 不接受参数（收到 %q）", param)
		}
		return strconv.FormatInt(ctx.Now.Unix(), 10), nil

	case renderVarUUID:
		if param != "" {
			return "", fmt.Errorf("变量 uuid 不接受参数（收到 %q）", param)
		}
		return uuid.NewString(), nil

	case renderVarTask:
		return ctx.Task, nil

	case renderVarRoom:
		return ctx.Room, nil

	case renderVarLatest:
		// 不带参数 = 本房间；带参数 = 指定房间。
		// 参数复用 `{{name:param}}` 这个槽位（date 用它装 "+1d"），所以写法上是统一的。
		room := ctx.Room
		if param != "" {
			if strings.ContainsAny(param, " \t\n") {
				return "", fmt.Errorf("变量 latest 的房间名不能含空格（收到 %q）", param)
			}
			room = normalizeRoomName(param)
		}
		if ctx.Latest == nil {
			return "", fmt.Errorf("变量 latest 需要服务端提供来源消息")
		}
		text, ok := ctx.Latest(room)
		if !ok {
			return "", fmt.Errorf("%w（房间 %s）", ErrEmptySource, room)
		}
		return text, nil
	}

	return "", fmt.Errorf("未知变量 {{%s}}（可用：%s）", name, strings.Join(RenderVariableNames(), " / "))
}

// applyDateOffset 按 `[+-]N[dwmy]` 偏移一个时刻。spec 为空时原样返回（allowEmpty 为真）。
func applyDateOffset(base time.Time, spec string, allowEmpty bool) (time.Time, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		if allowEmpty {
			return base, nil
		}
		return base, fmt.Errorf("日期偏移不能为空")
	}

	m := dateOffsetRe.FindStringSubmatch(spec)
	if m == nil {
		return base, fmt.Errorf("无法识别的日期偏移 %q（应形如 +1d / -2w / +3m / +1y）", spec)
	}

	amount, err := strconv.Atoi(m[2])
	if err != nil {
		return base, fmt.Errorf("无法解析偏移数量 %q: %w", m[2], err)
	}
	if m[1] == "-" {
		amount = -amount
	}

	switch strings.ToLower(m[3]) {
	case "w":
		return base.AddDate(0, 0, amount*7), nil
	case "m":
		return addMonthsClamped(base, amount), nil
	case "y":
		return addMonthsClamped(base, amount*12), nil
	default:
		return base.AddDate(0, 0, amount), nil
	}
}

// addMonthsClamped 加 N 个月，并把「日」夹到目标月的最后一天。
//
// ⚠️ 不能直接用 AddDate 的月份加法：`1月31日 + 1个月` 会因为 2 月没有 31 号而**溢出到 3月3日**。
// 正确结果应该是 2月28/29日。前端 date.add 里的 addMonths 是同一套处理 ——
// 两侧不一致的话，同一个动作在预览区和定时任务里会给出不同的日期。
func addMonthsClamped(t time.Time, months int) time.Time {
	day := t.Day()
	// 先归到 1 号再加月，避免加法过程中发生溢出
	first := time.Date(t.Year(), t.Month(), 1, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
	target := first.AddDate(0, months, 0)
	// 目标月的最后一天：下个月的第 0 天
	lastDay := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, target.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(target.Year(), target.Month(), day, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
}

// 周几的四种写法。索引就是 time.Weekday（0 = 周日），和前端 `getDay()` 一致。
var (
	weekdayZhFull  = [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	weekdayZhShort = [...]string{"日", "一", "二", "三", "四", "五", "六"}
	weekdayEnShort = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
)

func normalizeWeekdayStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "zh", "cn", "中文", "汉":
		return "zh"
	case "zh-short", "short", "简", "简写":
		return "zh-short"
	case "en", "英文", "english":
		return "en"
	case "en-short":
		return "en-short"
	}
	return ""
}

// parseWeekdayParam 解析 {{weekday:...+1d|en}} 里的参数。
//
// 参数有两种成分，顺序可换：偏移（+1d）和样式（en）。用 `|` 分隔，
// 这样 `{{weekday:+1d|en}}` 和 `{{weekday:en|+1d}}` 都成立 —— 用户不必记顺序。
func parseWeekdayParam(param string) (offset string, style string, err error) {
	style = "zh"
	param = strings.TrimSpace(param)
	if param == "" {
		return "", style, nil
	}

	parts := strings.Split(param, "|")
	if len(parts) > 2 {
		return "", "", fmt.Errorf("变量 weekday 的参数最多两段（偏移|样式），收到 %q", param)
	}

	for _, seg := range parts {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if s := normalizeWeekdayStyle(seg); s != "" {
			style = s
			continue
		}
		// 不是已知样式 → 那它只能是偏移。偏移已经占位时，说明这一段是**写错的样式名**
		// （比如 cn-simple）：报「无法识别的样式」比报「只能有一个偏移」有用得多，
		// 后者会让人以为是多写了一个偏移而去改错地方。
		if offset != "" {
			return "", "", fmt.Errorf("无法识别的周几样式 %q（可用 zh / zh-short / en / en-short）", seg)
		}
		offset = seg
	}
	return offset, style, nil
}

func formatWeekday(wd time.Weekday, style string) (string, error) {
	idx := int(wd)
	if idx < 0 || idx >= 7 {
		return "", fmt.Errorf("无效的星期值 %d", idx)
	}
	switch style {
	case "zh":
		return weekdayZhFull[idx], nil
	case "zh-short":
		return weekdayZhShort[idx], nil
	case "en":
		return wd.String(), nil
	case "en-short":
		return weekdayEnShort[idx], nil
	}
	return "", fmt.Errorf("无法识别的周几样式 %q", style)
}
