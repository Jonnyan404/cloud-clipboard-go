package lib

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

/**
*** FILE: scheduler.go
***   定时任务的调度：谁在什么时候该跑、跑完记什么。
**/

// 调度精度是**分钟**（任务的 Time 就是 HH:MM），但扫描是秒级的 —— 因为到期判定写的是
// 「now >= 某个计划时刻」而不是「now == 那个时刻」。这样服务重启、机器休眠、
// 容器被暂停都不会漏掉触发，代价只是最多晚一个 tick 发出去。
//
// 为什么是**进程内 ticker** 而不是外部 cron：任务存的是「动作 id + 模板 + 时刻」，
// 外面的 cron 只能调 HTTP 接口，那就得把凭据写进 crontab —— 而凭据一旦离开服务端，
// 「谁能给这个房间装自动化」这条边界就守不住了。
const (
	defaultAutomationTickSeconds  = 30
	defaultAutomationGraceSeconds = 600
)

func (s *ClipboardServer) automationEnabled() bool {
	return s != nil && s.config != nil && s.config.Automation.Enabled
}

func (s *ClipboardServer) automationTickInterval() time.Duration {
	seconds := 0
	if s.config != nil {
		seconds = s.config.Automation.TickSeconds
	}
	if seconds <= 0 {
		seconds = defaultAutomationTickSeconds
	}
	return time.Duration(seconds) * time.Second
}

// automationGraceWindow 错过多久就不再补发。
//
// 默认 10 分钟：服务重启、镜像更新这类短暂中断，重启后补发一条「今天该做什么」是有用的；
// 而宕机一整夜之后补发昨天那条提醒纯属噪音，用户还会以为任务出了毛病。
func (s *ClipboardServer) automationGraceWindow() time.Duration {
	seconds := 0
	if s.config != nil {
		seconds = s.config.Automation.GraceSeconds
	}
	if seconds <= 0 {
		seconds = defaultAutomationGraceSeconds
	}
	return time.Duration(seconds) * time.Second
}

// startAutomationScheduler 起一个后台 goroutine 扫到期任务。重复调用是安全的。
func (s *ClipboardServer) startAutomationScheduler() {
	if !s.automationEnabled() {
		s.logger.Println("定时自动化未启用（automation.enabled = false）")
		return
	}

	s.runMutex.Lock()
	if s.automationStop != nil {
		s.runMutex.Unlock()
		return // 已经在跑了，别起第二个
	}
	stop := make(chan struct{})
	s.automationStop = stop
	s.runMutex.Unlock()

	interval := s.automationTickInterval()
	// 配置里的默认时区写错了就先说一次：否则用户会以为任务按上海时间跑，
	// 而实际落回了服务器本地（容器里通常是 UTC）—— 那种偏差只在真到了那一天才看得出来。
	if configured := strings.TrimSpace(s.config.Automation.DefaultTZ); configured != "" {
		if _, err := time.LoadLocation(configured); err != nil {
			s.logger.Printf("警告: automation.defaultTZ = %q 无法识别，将使用 %s",
				configured, defaultAutomationTZName)
		}
	}
	s.logger.Printf("定时自动化已启用：每 %s 扫描一次，错过的补发窗口 %s，默认时区 %s",
		interval, s.automationGraceWindow(), s.automationDefaultTZName())

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.runDueAutomationTasks(time.Now())
			}
		}
	}()
}

func (s *ClipboardServer) stopAutomationScheduler() {
	s.runMutex.Lock()
	stop := s.automationStop
	s.automationStop = nil
	s.runMutex.Unlock()

	if stop != nil {
		close(stop)
	}
}

// automationDue 一次到期待执行。
type automationDue struct {
	task    *AutomationTask
	key     string
	instant time.Time
	late    bool
}

// runDueAutomationTasks 扫一遍任务：该跑的跑、错过的记跳过。
//
// 全程**分三段**而不是「持锁做完」：挑选在锁内、执行在锁外、记状态再回锁内。
// 执行会写 WebSocket 和文件，持锁做它会让所有 /tasks 请求被顶住 ——
// 一个连了十几个设备的房间，一次广播足够到秒级。
func (s *ClipboardServer) runDueAutomationTasks(now time.Time) {
	tasks := s.snapshotAutomationTasks()
	grace := s.automationGraceWindow()

	var due []automationDue
	var stale []automationDue

	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		occurrence, ok := task.dueOccurrence(now)
		if !ok {
			continue
		}
		key := task.runKey(occurrence)
		if key == task.LastRunKey {
			continue // 这一趟已经跑过了 —— 幂等键就是干这个的
		}
		item := automationDue{task: task, key: key, instant: occurrence, late: now.Sub(occurrence) > grace}
		if item.late {
			stale = append(stale, item)
		} else {
			due = append(due, item)
		}
	}

	for _, item := range stale {
		s.logger.Printf("定时任务 %s(%s) 错过触发窗口 %s，跳过不补发",
			item.task.Name, item.task.ID, item.instant.Format(time.RFC3339))
		// 记 key 而不是什么都不做：不记的话它每个 tick 都会被重新评估一遍，
		// 日志会被同一件事刷满，tasks.json 也会每 30 秒被重写一次。
		//
		// 「仅一次」同样自禁用：它的那个时刻已经过去了，留着 enabled=true 只会
		// 让列表一直显示「已启用」而实际上永远不会再发。
		s.recordAutomationRun(item.task.ID, item.key, "skipped", "错过触发窗口，未补发", "",
			item.task.Freq == automationFreqOnce)
	}

	for _, item := range due {
		output, err := s.executeAutomationTask(item.task, item.instant, false)
		status, errText := "ok", ""
		switch {
		case err == nil:
			s.logger.Printf("定时任务 %s(%s) 已投递到房间 [%s]（%d 字符，保留历史=%v）",
				item.task.Name, item.task.ID, item.task.Room, len([]rune(output)), item.task.KeepHistory)
		case errors.Is(err, ErrEmptySource):
			// 来源房间此刻没有素材 —— 这是**常态**（没人发言、或消息被顶掉了），
			// 不是任务坏了。记 skipped 而不是 error：error 会让「上次失败」长期挂在
			// 那条任务上，用户以为功能坏了，而其实只是那个房间今天没人说话。
			status, errText = "skipped", err.Error()
			s.logger.Printf("定时任务 %s(%s) 跳过：%v", item.task.Name, item.task.ID, err)
		default:
			status, errText = "error", err.Error()
			s.logger.Printf("定时任务 %s(%s) 执行失败: %v", item.task.Name, item.task.ID, err)
		}
		// 「仅一次」跑完自禁用（无论是成功还是失败）：失败再重试也还是同一个时刻，
		// 而用户看到的一直是「已启用」会以为还有下一次。
		s.recordAutomationRun(item.task.ID, item.key, status, errText, output, item.task.Freq == automationFreqOnce)
	}
}

// executeAutomationTask 算正文 → 跑动作链 → 投递（dryRun 时只算不发）。
//
// scheduled 是**预定**触发时刻，不是实际发送时刻。补发时正文里写的仍然是那个
// 「本该发出的时刻」—— 用户写 {{date:+1d}} 指的是那一天的明天，换成实际时刻
// 会得到一条内容不对、但看起来完全正常的消息。是不是补发由消息上的 Late 标记体现。
func (s *ClipboardServer) executeAutomationTask(task *AutomationTask, scheduled time.Time, dryRun bool) (string, error) {
	loc, err := task.location()
	if err != nil {
		return "", err
	}

	ctx := renderContext{
		Now:  scheduled.In(loc),
		Task: task.Name,
		Room: task.Room,
		// 唯一一个会读外部状态的地方：{{latest}} 取房间里的最新消息。
		// 引擎本身不碰消息队列，读这件事在这里注入（见 automation_source.go）。
		Latest: s.latestRoomText,
	}

	rendered, err := RenderTemplate(task.Template, ctx)
	if err != nil {
		return "", err
	}
	if len(task.Chain) > 0 {
		rendered, err = applyRenderActions(rendered, task.Chain, ctx)
		if err != nil {
			return "", err
		}
	}

	// 正文上限在**展开之后**再校验一次：模板本身很短，但 {{date:+1d}} 展开、
	// 或链上做了 Base64 编码之后可能变长，而 /text 的限制是按最终字符数算的。
	if s.config != nil {
		if limit := s.config.Text.Limit; limit > 0 && len(rendered) > limit {
			return "", fmt.Errorf("渲染后的正文超出限制 (%d > %d)", len(rendered), limit)
		}
	}

	if dryRun {
		return rendered, nil
	}

	s.deliverMessage("text", rendered, task.Room, messageSource{
		// 定时消息没有 IP（不是任何人发起的）—— 留空，前端会直接不显示这一项，
		// 而不是显示一个假 IP。
		DeviceName:  task.Sender,
		Source:      "automation",
		ScheduledAt: scheduled.Unix(),
		Late:        time.Since(scheduled) > s.automationGraceWindow(),
	}, task.KeepHistory)

	return rendered, nil
}

// previewAutomationTask 试跑：只算正文，不投递、不落盘、不动执行记录。
//
// 求值的基准默认取**下次触发时刻**（而不是「现在」）——
// 否则下午配一个「每天 09:30、正文写 {{date:+1d}}」的任务，
// 试跑看到的是今天 +1，而明早真正发出的会是明天 +1，预览就对不上了。
func (s *ClipboardServer) previewAutomationTask(task *AutomationTask, reference time.Time) (string, time.Time, error) {
	output, err := s.executeAutomationTask(task, reference, true)
	if err != nil {
		return "", reference, err
	}
	return output, reference, nil
}
