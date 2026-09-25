package lib

// 定时任务的契约 fixture：把 task.go 的**求值**（next / due / runKey）与**校验+归一化**
// 导出成 JSON，供 Rust 侧比对（`crates/core/tests/go_task.rs`）。
//
// 与 cron / render 的 fixture 同一套思路：期望值是**跑 Go 得到的**，不是人写的。
// 定时任务这一层的难点在：
//
//   · **判到期用 `dueOccurrence`（最近的过去）而不是 `nextRunAfter`（下一次未来）**——
//     两者的差异正是「错过的那次会不会被发现」；
//   · **weekly 的周几匹配**（0 = 周日，与 `getDay()` 一致）、**daily 的 HH:MM 解析**、
//     **once 的 runAt 归一化**（本地写法 → 带时区的 RFC3339）、**cron 的表达式归一化**；
//   · **幂等键是分钟级**（`2006-01-02T15:04`）；
//   · **校验发生在保存时**：坏频次 / 坏时间 / weekly 没选周几 / cron 永远算不出未来 /
//     未知动作 / 空模板 都要拦。
//
// ⚠️ fixture 必须**固定时区**（`Asia/Shanghai`）与**固定 now**：跟着机器时区 / 真实时钟漂
// 就是假绿（与 cron / render fixture 同一条坑）。
//
// 用法：
//
//	go test ./lib -run TestTaskFixtures                      # 校验（不复写）
//	UPDATE_FIXTURES=1 go test ./lib -run TestTaskFixtures     # 重新生成

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const taskFixtureDir = "../../cases/task"

type taskFixtureOccurrence struct {
	Expr    string `json:"expr"`
	Freq    string `json:"freq"`
	Time    string `json:"time,omitempty"`
	Cron    string `json:"cron,omitempty"`
	ByDay   []int  `json:"byWeekday,omitempty"`
	RunAt   string `json:"runAt,omitempty"`
	Now     string `json:"now"`
	Next    *int64 `json:"next"`    // nextRunAfter 的 Unix 秒；无后续触发为 null
	Due     *int64 `json:"due"`     // dueOccurrence 的 Unix 秒；无到期为 null
	DueKey  string `json:"dueKey"`  // due 那次触发的幂等键
	NextKey string `json:"nextKey"` // next 那次触发的幂等键
	Why     string `json:"why,omitempty"`
}

type taskFixtureValidate struct {
	Name  string           `json:"name"`
	Input taskFixtureInput `json:"input"`
	OK    bool             `json:"ok"`
	// 归一化后的关键字段（OK 时才有）。
	Freq      string `json:"freq,omitempty"`
	Time      string `json:"time,omitempty"`
	Cron      string `json:"cron,omitempty"`
	ByDay     []int  `json:"byWeekday,omitempty"`
	RunAt     string `json:"runAt,omitempty"`
	NameOut   string `json:"nameOut,omitempty"`
	SenderOut string `json:"senderOut,omitempty"`
	Why       string `json:"why,omitempty"`
}

type taskFixtureInput struct {
	Name        string                `json:"name"`
	Freq        string                `json:"freq"`
	Time        string                `json:"time"`
	Cron        string                `json:"cron"`
	ByWeekday   []int                 `json:"byWeekday"`
	RunAt       string                `json:"runAt"`
	TZ          string                `json:"tz"`
	Template    string                `json:"template"`
	Chain       []AutomationChainStep `json:"chain"`
	KeepHistory bool                  `json:"keepHistory"`
	Sender      string                `json:"sender"`
}

type taskFixture struct {
	Note        string                  `json:"note"`
	TZ          string                  `json:"tz"`
	Now         string                  `json:"now"`
	Occurrences []taskFixtureOccurrence `json:"occurrences"`
	Validates   []taskFixtureValidate   `json:"validates"`
}

// 固定的 now：2026-09-24 09:30 +08:00（周四）。
func taskFixtureNow() time.Time {
	loc := time.FixedZone("+08:00", 8*3600)
	return time.Date(2026, 9, 24, 9, 30, 0, 0, loc)
}

func taskFixtureOccurrences() []taskFixtureOccurrence {
	now := taskFixtureNow()
	unix := func(t time.Time) int64 { return t.Unix() }
	cases := []struct {
		freq  string
		tm    string
		cron  string
		byDay []int
		runAt string
		why   string
	}{
		// daily：当天 09:30 已到 → due 是今天 09:30；next 是明天 09:30
		{"daily", "09:30", "", nil, "", "每天 09:30，now 正好是 09:30"},
		{"daily", "10:00", "", nil, "", "now 是 09:30，10:00 还没到 → due 是昨天 10:00"},
		{"daily", "08:00", "", nil, "", "08:00 已过 → due 是今天 08:00"},

		// weekly：周四，byWeekday 含周四
		{"weekly", "09:00", "", []int{4}, "", "周四的每周任务，due 是今天（周四）09:00"},
		{"weekly", "09:00", "", []int{1}, "", "只选周一，due 是本周一 09:00"},
		{"weekly", "09:00", "", []int{4, 1}, "", "周一和周四，due 是今天（周四）09:00"},

		// once
		{"once", "", "", nil, "2026-09-24T08:00:00+08:00", "过去的时刻 → due 是它，next 无"},
		{"once", "", "", nil, "2026-09-25T08:00:00+08:00", "未来的时刻 → due 无，next 是它"},

		// cron
		{"cron", "", "30 9 * * *", nil, "", "每天 09:30，due 是今天 09:30"},
		{"cron", "", "0 */2 * * *", nil, "", "每两小时整点，now 是 09:30 → due 是 08:00"},
	}

	out := make([]taskFixtureOccurrence, 0, len(cases))
	for _, c := range cases {
		task := &AutomationTask{
			Freq: c.freq, Time: c.tm, Cron: c.cron, ByWeekday: c.byDay,
			RunAt: c.runAt, TZ: "Asia/Shanghai",
		}
		item := taskFixtureOccurrence{
			Expr: describeTaskExpr(c), Freq: c.freq, Time: c.tm, Cron: c.cron,
			ByDay: c.byDay, RunAt: c.runAt, Now: now.Format(time.RFC3339), Why: c.why,
		}
		if next, ok := task.nextRunAfter(now); ok {
			v := unix(next)
			item.Next = &v
			item.NextKey = task.runKey(next)
		}
		if due, ok := task.dueOccurrence(now); ok {
			v := unix(due)
			item.Due = &v
			item.DueKey = task.runKey(due)
		}
		out = append(out, item)
	}
	return out
}

func describeTaskExpr(c struct {
	freq  string
	tm    string
	cron  string
	byDay []int
	runAt string
	why   string
}) string {
	switch c.freq {
	case "daily":
		return "daily " + c.tm
	case "weekly":
		return "weekly " + c.tm
	case "once":
		return "once " + c.runAt
	case "cron":
		return "cron " + c.cron
	}
	return c.freq
}

func taskFixtureValidates(t *testing.T) []taskFixtureValidate {
	cases := []taskFixtureValidate{
		{
			Name: "daily 基本", Input: taskFixtureInput{
				Freq: "daily", Time: "09:30", TZ: "Asia/Shanghai", Template: "今天是 {{date}}",
			},
			OK: true, Freq: "daily", Time: "09:30", NameOut: "今天是 {{date}}",
			SenderOut: "定时任务",
		},
		{
			Name: "once 归一化成 RFC3339", Input: taskFixtureInput{
				Freq: "once", RunAt: "2026-09-25 08:00", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: true, Freq: "once", RunAt: "2026-09-25T08:00:00+08:00", NameOut: "x",
			SenderOut: "定时任务",
		},
		{
			Name: "weekly 去重排序", Input: taskFixtureInput{
				Freq: "weekly", Time: "09:00", ByWeekday: []int{4, 1, 4, 1}, TZ: "Asia/Shanghai", Template: "x",
			},
			OK: true, Freq: "weekly", Time: "09:00", ByDay: []int{1, 4}, NameOut: "x",
			SenderOut: "定时任务",
		},
		{
			Name: "cron 归一化", Input: taskFixtureInput{
				Freq: "cron", Cron: "0  9 *  * *", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: true, Freq: "cron", Cron: "0 9 * * *", NameOut: "x", SenderOut: "定时任务",
		},
		{
			Name: "名字从模板推导", Input: taskFixtureInput{
				Freq: "daily", Time: "09:00", TZ: "Asia/Shanghai", Template: "第一行很长很长很长很长很长很长很长很长很长很长",
			},
			OK: true, Freq: "daily", Time: "09:00", SenderOut: "定时任务",
			NameOut: "第一行很长很长很长很长很长很长很长很长…",
		},
		{
			Name: "空模板报错", Input: taskFixtureInput{
				Freq: "daily", Time: "09:00", TZ: "Asia/Shanghai",
			},
			OK: false, Why: "正文不能为空",
		},
		{
			Name: "坏频次", Input: taskFixtureInput{
				Freq: "hourly", Time: "09:00", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: false, Why: "频率只能是 once / daily / weekly / cron",
		},
		{
			Name: "坏时间", Input: taskFixtureInput{
				Freq: "daily", Time: "25:00", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: false, Why: "时间超出范围",
		},
		{
			Name: "weekly 没选周几", Input: taskFixtureInput{
				Freq: "weekly", Time: "09:00", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: false, Why: "「每周」需要至少选一天",
		},
		{
			Name: "cron 永远算不出未来", Input: taskFixtureInput{
				Freq: "cron", Cron: "0 0 30 2 *", TZ: "Asia/Shanghai", Template: "x",
			},
			OK: false, Why: "在未来算不出任何触发时刻",
		},
		{
			Name: "未知动作", Input: taskFixtureInput{
				Freq: "daily", Time: "09:00", TZ: "Asia/Shanghai", Template: "x",
				Chain: []AutomationChainStep{{ID: "not.a.real.action"}},
			},
			OK: false, Why: "不能用于定时任务",
		},
		{
			Name: "坏时区", Input: taskFixtureInput{
				Freq: "daily", Time: "09:00", TZ: "Not/AZone", Template: "x",
			},
			OK: false, Why: "无法识别的时区",
		},
	}

	for i := range cases {
		c := &cases[i]
		task := &AutomationTask{
			Name: c.Input.Name, Freq: c.Input.Freq, Time: c.Input.Time, Cron: c.Input.Cron,
			ByWeekday: c.Input.ByWeekday, RunAt: c.Input.RunAt, TZ: c.Input.TZ,
			Room: "home", Template: c.Input.Template, Chain: c.Input.Chain,
			KeepHistory: c.Input.KeepHistory, Sender: c.Input.Sender,
		}
		// fillDefaultTZ 等价物：Go 里这是 HTTP 层做的，这里直接模拟
		if task.TZ == "" {
			task.TZ = defaultAutomationTZName
		}
		err := task.normalizeAndValidate()
		if err != nil {
			if c.OK {
				t.Errorf("%s: 意外报错 %v", c.Name, err)
			}
			continue
		}
		if !c.OK {
			t.Errorf("%s: 期望报错（%s）却成功了", c.Name, c.Why)
			continue
		}
		c.Freq = task.Freq
		c.Time = task.Time
		c.Cron = task.Cron
		c.ByDay = task.ByWeekday
		c.RunAt = task.RunAt
		c.NameOut = task.Name
		c.SenderOut = task.Sender
	}
	return cases
}

func TestTaskFixtures(t *testing.T) {
	now := taskFixtureNow()
	f := taskFixture{
		Note:        "定时任务的求值与校验基准：期望值由 Go 的 task.go 跑出（固定时区 + 固定 now）。",
		TZ:          "Asia/Shanghai",
		Now:         now.Format(time.RFC3339),
		Occurrences: taskFixtureOccurrences(),
		Validates:   taskFixtureValidates(t),
	}

	path := filepath.Join(taskFixtureDir, "task.json")
	if os.Getenv("UPDATE_FIXTURES") == "" {
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读不到现有 fixture（%v）—— 先跑 UPDATE_FIXTURES=1 生成", err)
		}
		want, _ := json.MarshalIndent(f, "", "  ")
		want = append(want, '\n')
		if string(onDisk) != string(want) {
			t.Fatalf("fixture 已过期 —— 重新生成：UPDATE_FIXTURES=1 go test ./lib -run TestTaskFixtures\n（先看 diff：求值/校验变了等于切换前后行为变了）")
		}
		return
	}

	if err := os.MkdirAll(taskFixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("已写出 %d 条求值 + %d 条校验到 %s", len(f.Occurrences), len(f.Validates), taskFixtureDir)
}
