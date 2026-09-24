package lib

import (
	"encoding/json"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
)

func testTask(freq, clock string) *AutomationTask {
	return &AutomationTask{
		ID:       "task-1",
		Name:     "值班提醒",
		Enabled:  true,
		Freq:     freq,
		Time:     clock,
		Room:     "home",
		Template: "明天是 {{date:+1d}}（{{weekday:+1d}}）",
	}
}

func newTaskOnlyServer(t *testing.T) *ClipboardServer {
	t.Helper()
	return &ClipboardServer{
		config:          &Config{},
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    NewMessageQueue(10, nil),
		historyFilePath: filepath.Join(t.TempDir(), "history.json"),
	}
}

func mustParse(t *testing.T, layout, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation(layout, value, time.Local)
	if err != nil {
		t.Fatalf("解析 %q 失败: %v", value, err)
	}
	return at
}

func TestNextRunAfterDaily(t *testing.T) {
	task := testTask(automationFreqDaily, "09:30")

	// 还没到点 → 就是今天
	next, ok := task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-24 08:00"))
	if !ok || next.Format("2006-01-02 15:04") != "2026-09-24 09:30" {
		t.Fatalf("得到 %v（ok=%v）", next, ok)
	}

	// 刚过点 → 明天
	next, ok = task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-24 09:30"))
	if !ok || next.Format("2006-01-02 15:04") != "2026-09-25 09:30" {
		t.Fatalf("同一时刻应算「已过」，得到 %v", next)
	}

	// 跨月
	next, _ = task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-30 12:00"))
	if next.Format("2006-01-02 15:04") != "2026-10-01 09:30" {
		t.Fatalf("跨月得到 %v", next)
	}
}

func TestNextRunAfterWeekly(t *testing.T) {
	task := testTask(automationFreqWeekly, "10:00")
	// 0=周日 … 6=周六；1 = 周一
	task.ByWeekday = []int{1}

	// 2026-09-24 是周四 → 下一个周一 = 09-28
	next, ok := task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-24 12:00"))
	if !ok || next.Format("2006-01-02 15:04") != "2026-09-28 10:00" {
		t.Fatalf("得到 %v（ok=%v）", next, ok)
	}

	// 周一当天、还没到点 → 就是今天
	next, _ = task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-28 08:00"))
	if next.Format("2006-01-02 15:04") != "2026-09-28 10:00" {
		t.Fatalf("周一当天得到 %v", next)
	}

	// 多个星期：周一 + 周三 + 周五
	task.ByWeekday = []int{1, 3, 5}
	next, _ = task.nextRunAfter(mustParse(t, "2006-01-02 15:04", "2026-09-24 12:00")) // 周四
	if next.Format("2006-01-02 15:04") != "2026-09-25 10:00" {                        // 周五
		t.Fatalf("多星期得到 %v", next)
	}
}

func TestDueOccurrencePicksMostRecentPast(t *testing.T) {
	task := testTask(automationFreqDaily, "09:30")

	// 今天还没到点 → 最近一次是昨天 09:30（而不是「今天 09:30」）
	due, ok := task.dueOccurrence(mustParse(t, "2006-01-02 15:04", "2026-09-24 08:00"))
	if !ok || due.Format("2006-01-02 15:04") != "2026-09-23 09:30" {
		t.Fatalf("得到 %v（ok=%v）", due, ok)
	}

	// 已过点 → 今天 09:30
	due, _ = task.dueOccurrence(mustParse(t, "2006-01-02 15:04", "2026-09-24 10:00"))
	if due.Format("2006-01-02 15:04") != "2026-09-24 09:30" {
		t.Fatalf("得到 %v", due)
	}
}

func TestDueOccurrenceWeeklySkipsNonMatchingDays(t *testing.T) {
	task := testTask(automationFreqWeekly, "09:00")
	task.ByWeekday = []int{1} // 周一

	// 2026-09-24 是周四 → 最近一次周一 = 09-21
	due, ok := task.dueOccurrence(mustParse(t, "2006-01-02 15:04", "2026-09-24 12:00"))
	if !ok || due.Format("2006-01-02 15:04") != "2026-09-21 09:00" {
		t.Fatalf("得到 %v（ok=%v）", due, ok)
	}
}

func TestOnceTask(t *testing.T) {
	task := testTask(automationFreqOnce, "")
	task.RunAt = "2026-10-01T09:30:00+08:00"

	past := mustParse(t, "2006-01-02 15:04", "2026-09-24 12:00")
	if next, ok := task.nextRunAfter(past); !ok || next.Unix() != mustParse(t, time.RFC3339, "2026-10-01T09:30:00+08:00").Unix() {
		t.Fatalf("future once 得到 %v（ok=%v）", next, ok)
	}
	if _, ok := task.dueOccurrence(past); ok {
		t.Fatal("还没到时间的 once 不该到期")
	}

	after := mustParse(t, "2006-01-02 15:04", "2026-10-02 12:00")
	if _, ok := task.dueOccurrence(after); !ok {
		t.Fatal("已过时间的 once 应当到期")
	}
	if _, ok := task.nextRunAfter(after); ok {
		t.Fatal("已过时间的 once 没有下一次")
	}
}

func TestRunKeyIsMinutePrecise(t *testing.T) {
	task := testTask(automationFreqDaily, "09:30")
	a := mustParse(t, "2006-01-02 15:04:05", "2026-09-24 09:30:00")
	b := mustParse(t, "2006-01-02 15:04:05", "2026-09-24 09:30:59")

	// 同一分钟内的两个表述必须得到同一个键 —— 否则同一刻会被当成两次触发。
	if task.runKey(a) != task.runKey(b) {
		t.Fatalf("同一分钟应得到同一键：%q vs %q", task.runKey(a), task.runKey(b))
	}
	if task.runKey(a) != "2026-09-24T09:30" {
		t.Fatalf("键格式变了: %q", task.runKey(a))
	}
}

func TestNormalizeAndValidate(t *testing.T) {
	t.Run("自动取名字", func(t *testing.T) {
		task := testTask(automationFreqDaily, "09:30")
		task.Name = ""
		task.Sender = ""
		if err := task.normalizeAndValidate(); err != nil {
			t.Fatalf("意外报错: %v", err)
		}
		if task.Name == "" {
			t.Fatal("名字应回退到正文首行，不该留空")
		}
		if task.Sender != defaultAutomationSender {
			t.Fatalf("发送者应回退到默认名，得到 %q", task.Sender)
		}
	})

	t.Run("once 归一化成 RFC3339", func(t *testing.T) {
		task := testTask(automationFreqOnce, "")
		task.RunAt = "2026-10-01T09:30"
		if err := task.normalizeAndValidate(); err != nil {
			t.Fatalf("意外报错: %v", err)
		}
		if _, err := time.Parse(time.RFC3339, task.RunAt); err != nil {
			t.Fatalf("应归一化成带时区的 RFC3339，得到 %q", task.RunAt)
		}
	})

	t.Run("weekly 去重排序", func(t *testing.T) {
		task := testTask(automationFreqWeekly, "09:30")
		task.ByWeekday = []int{5, 1, 5, 3}
		if err := task.normalizeAndValidate(); err != nil {
			t.Fatalf("意外报错: %v", err)
		}
		want := []int{1, 3, 5}
		if !sameIntSlice(task.ByWeekday, want) {
			t.Fatalf("得到 %v，期望 %v", task.ByWeekday, want)
		}
	})

	bad := []struct {
		name   string
		mutate func(*AutomationTask)
	}{
		{"空正文", func(x *AutomationTask) { x.Template = "" }},
		{"未知变量", func(x *AutomationTask) { x.Template = "{{nope}}" }},
		{"非法频率", func(x *AutomationTask) { x.Freq = "hourly" }},
		{"非法时间", func(x *AutomationTask) { x.Time = "25:00" }},
		{"时间格式错", func(x *AutomationTask) { x.Time = "930" }},
		{"weekly 没选天", func(x *AutomationTask) {
			x.Freq = automationFreqWeekly
			x.ByWeekday = nil
		}},
		{"星期越界", func(x *AutomationTask) {
			x.Freq = automationFreqWeekly
			x.ByWeekday = []int{7}
		}},
		{"时区写错", func(x *AutomationTask) { x.TZ = "Mars/Olympus" }},
		{"once 缺时间", func(x *AutomationTask) {
			x.Freq = automationFreqOnce
			x.RunAt = ""
		}},
		{"动作不在服务端子集", func(x *AutomationTask) { x.Chain = []AutomationChainStep{{ID: "pinyin.annotate"}} }},
	}

	for _, c := range bad {
		t.Run("拒绝:"+c.name, func(t *testing.T) {
			task := testTask(automationFreqDaily, "09:30")
			c.mutate(task)
			if err := task.normalizeAndValidate(); err == nil {
				t.Fatalf("%s 应被拒绝", c.name)
			}
		})
	}
}

// 链元素的两种 JSON 形态都要认 —— 存量 tasks.json 里**全是字符串**，
// 只认对象的话，用户升一次级就丢掉整条链。
func TestAutomationChainStepJSON(t *testing.T) {
	raw := []byte(`{"tasks":[{"id":"t1","chain":["text.trimLines",{"id":"text.replace","params":{"find":"a","with":"b"}}]}]}`)
	var file automationFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(file.Tasks) != 1 {
		t.Fatalf("任务数 = %d", len(file.Tasks))
	}
	chain := file.Tasks[0].Chain
	if len(chain) != 2 {
		t.Fatalf("链长度 = %d，期望 2", len(chain))
	}
	if chain[0].ID != "text.trimLines" || len(chain[0].Params) != 0 {
		t.Errorf("第一步解析错: %+v", chain[0])
	}
	if chain[1].ID != "text.replace" || chain[1].Params["find"] != "a" || chain[1].Params["with"] != "b" {
		t.Errorf("第二步解析错: %+v", chain[1])
	}

	// 写回时**无参数必须是字符串**：和存量数据同形，diff 也干净。
	out, err := json.Marshal(chain)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	want := `["text.trimLines",{"id":"text.replace","params":{"find":"a","with":"b"}}]`
	if string(out) != want {
		t.Fatalf("序列化 = %s，期望 %s", out, want)
	}
}

func TestAutomationPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	first := &ClipboardServer{
		config:          &Config{},
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    NewMessageQueue(10, nil),
		historyFilePath: path,
	}
	task := testTask(automationFreqDaily, "09:30")
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := first.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 新进程（新 ClipboardServer）读同一份文件
	second := &ClipboardServer{
		config:          &Config{},
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    NewMessageQueue(10, nil),
		historyFilePath: path,
	}
	loaded := second.snapshotAutomationTasks()
	if len(loaded) != 1 {
		t.Fatalf("应读出 1 条任务，得到 %d", len(loaded))
	}
	if loaded[0].Name != task.Name || loaded[0].Template != task.Template {
		t.Fatalf("读回的内容不对: %+v", loaded[0])
	}
	if loaded[0].NextRunAt(time.Now()) == 0 {
		t.Fatal("读回的任务应仍能算出下次触发时间")
	}
}

// 别让「改一下时间」把下一次触发吞掉，也别让任务被改到别的房间去。
func TestUpsertKeepsRoomAndResetsRunKeyOnScheduleChange(t *testing.T) {
	s := newTaskOnlyServer(t)

	task := testTask(automationFreqDaily, "09:30")
	task.Room = "home"
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 模拟已执行过一次
	s.recordAutomationRun(task.ID, "2026-09-24T09:30", "ok", "", "昨天的内容", false)
	before := s.snapshotAutomationTasks()[0]
	if before.LastRunKey == "" {
		t.Fatal("执行记录应被写下")
	}

	// 改时间：即使请求里把 Room 换成别的，也必须保留原房间
	updated := testTask(automationFreqDaily, "10:00")
	updated.Room = "别人的房间"
	if err := updated.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(updated); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	after := s.snapshotAutomationTasks()[0]
	if after.Room != "home" {
		t.Fatalf("房间不允许被改，得到 %q", after.Room)
	}
	if after.LastRunKey != "" {
		t.Fatalf("改了触发规则就该清掉幂等键，否则 10:00 的第一次会被当成 09:30 那次而跳过（得到 %q）", after.LastRunKey)
	}
	if after.CreatedAt != task.CreatedAt {
		t.Fatal("创建时间应保留")
	}
}
