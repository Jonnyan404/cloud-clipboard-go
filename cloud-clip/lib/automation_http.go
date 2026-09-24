package lib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

/**
*** FILE: automation_http.go
***   /tasks 的 REST 接口：列表、创建/更新、删除、试跑。
**/

// 契约（与 docs/api.zh-CN.md 同源）：
//
//	GET    /tasks?room=R            列出这个房间里「我管得着」的任务
//	POST   /tasks?room=R            创建或整体更新一条（body 里带 id 就是更新）
//	DELETE /tasks/:id?room=R        删一条
//	POST   /tasks/:id/run?room=R    试跑（默认只算不发），?send=1 才真发
//
// 三个刻意的设计点：
//
//  1. **body 里没有 room 字段**。房间来自鉴权上下文（?room= 必须通过 canAccessRoom），
//     不接受客户端在任务里声明投递目标 —— 否则一条已授权的任务就能把消息发到别的房间。
//     这和 inferRequestRoom 里那条教训同源："不能信客户端传的 ?room="。
//
//  2. 用 **JSON body**，与 /auth/token、/share 一致，而不是学 /text 的 text/plain：
//     任务天然是结构化对象，塞进纯文本等于自己发明一套微型格式。
//
//  3. 更新时**不允许改房间**（upsertAutomationTask 会保留原任务的 Room）。
const automationBodyMaxBytes = 64 * 1024

// automationTaskRequest 请求体。注意这里**没有 Room** —— 见上面第 1 条。
type automationTaskRequest struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Enabled     *bool                 `json:"enabled"`
	Freq        string                `json:"freq"`
	Time        string                `json:"time"`
	Cron        string                `json:"cron"`
	ByWeekday   []int                 `json:"byWeekday"`
	RunAt       string                `json:"runAt"`
	TZ          string                `json:"tz"`
	Template    string                `json:"template"`
	Chain       []AutomationChainStep `json:"chain"`
	KeepHistory *bool                 `json:"keepHistory"`
	Sender      string                `json:"sender"`
}

// fillDefaultTZ 把默认时区**写进任务**，而不是运行时兜底。
//
// 一个任务定义要是自洽的：换一台服务器、改一次配置，不该让同一条任务换个时刻触发。
// 运行时兜底会让「存下来的是 09:30、实际在别处变成 17:30」这种问题只在那一天才暴露。
func (s *ClipboardServer) fillDefaultTZ(task *AutomationTask) {
	if strings.TrimSpace(task.TZ) == "" {
		task.TZ = s.automationDefaultTZName()
	}
}

// handleTasks GET（列表）/ POST（创建或更新）。
func (s *ClipboardServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	if !s.automationEnabled() {
		writeError(w, http.StatusNotFound, "automation_disabled", "Automation is disabled",
			"定时自动化未启用（automation.enabled = false）")
		return
	}

	token := extractAuthToken(r)
	scope := s.resolveAutomationScope(r, token)
	if !scope.OK {
		writeError(w, http.StatusForbidden, "automation_forbidden", "Automation is not allowed in this room",
			fmt.Sprintf("房间 %s 未开放自动化（需要该房间的凭据，或由管理员在 roomAuth 里设置 automation）", scope.Room))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleTaskList(w, r, scope)
	case http.MethodPost:
		s.handleTaskUpsert(w, r, scope)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and POST are allowed", "仅允许 GET / POST 请求")
	}
}

func (s *ClipboardServer) handleTaskList(w http.ResponseWriter, r *http.Request, scope automationScope) {
	now := time.Now()
	tasks := s.snapshotAutomationTasks()
	taskToken := extractTaskToken(r)

	views := make([]map[string]interface{}, 0, len(tasks))
	foreign := 0
	for _, task := range tasks {
		// 列表**始终按房间过滤**，管理员也一样：管理员的额外能力是「能用 ?room= 指向任意房间」，
		// 不是「一次看到所有房间的任务」。混在一起会让面板在切换房间时显示错的内容。
		if normalizeRoomName(task.Room) != scope.Room {
			continue
		}
		// single 档房间（通常是公开房间）里，不是自己建的那条**连内容都不给看**。
		// 它确实能被任何人删改（公开房间没有凭据可分辨人），但至少不该让所有人都读到
		// 里面写了什么、发到哪、什么时候发。
		if !s.taskOwnedBy(task, scope, taskToken) {
			foreign++
			continue
		}
		views = append(views, taskView(task, now))
	}

	resp := map[string]interface{}{
		"room":    scope.Room,
		"tier":    scope.Tier,
		"admin":   scope.Admin,
		"tasks":   views,
		"max":     s.automationRoomLimit(scope),
		"now":     now.Unix(),
		"actions": ServerRenderActionList(),
		"vars":    RenderVariableNames(),
	}
	if foreign > 0 {
		// 只说「还有几条不归你管」，不透露内容。
		resp["foreignCount"] = foreign
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Printf("错误: 编码 /tasks 响应失败: %v", err)
	}
}

func (s *ClipboardServer) handleTaskUpsert(w http.ResponseWriter, r *http.Request, scope automationScope) {
	var req automationTaskRequest
	r.Body = http.MaxBytesReader(w, r.Body, automationBodyMaxBytes)
	// 不开 DisallowUnknownFields：前端和服务端是分开迭代的，
	// 「多传了一个字段」直接 400 会让每次前端先行发布都炸一片请求。
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Cannot parse request body", "请求体无法解析")
		return
	}
	defer r.Body.Close()

	task := &AutomationTask{
		ID:        strings.TrimSpace(req.ID),
		Name:      req.Name,
		Enabled:   true,
		Freq:      req.Freq,
		Time:      req.Time,
		Cron:      req.Cron,
		ByWeekday: req.ByWeekday,
		RunAt:     req.RunAt,
		TZ:        req.TZ,
		// ⚠️ 房间来自鉴权上下文，不是请求体
		Room:     scope.Room,
		Template: req.Template,
		Chain:    req.Chain,
		Sender:   req.Sender,
	}
	s.fillDefaultTZ(task)
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.KeepHistory != nil {
		task.KeepHistory = *req.KeepHistory
	}

	isNew := task.ID == ""
	var issuedToken string

	if isNew {
		task.ID = uuid.NewString()
	} else {
		existing, ok := s.findAutomationTask(task.ID)
		if !ok {
			writeError(w, http.StatusNotFound, "task_not_found", "Task not found", "任务不存在")
			return
		}
		if !s.taskOwnedBy(existing, scope, extractTaskToken(r)) {
			writeError(w, http.StatusForbidden, "task_forbidden", "Task belongs to someone else",
				"这条任务不是你创建的，无法修改")
			return
		}
		if !scope.Admin && normalizeRoomName(existing.Room) != scope.Room {
			writeError(w, http.StatusForbidden, "task_forbidden", "Task belongs to another room",
				"这条任务属于别的房间")
			return
		}
	}

	if isNew {
		if limit := s.automationRoomLimit(scope); limit > 0 {
			if s.countAutomationTasksInRoom(scope.Room) >= limit {
				msg := fmt.Sprintf("房间 %s 的定时任务已达到上限 %d", scope.Room, limit)
				if scope.Tier == automationPolicySingle {
					msg = fmt.Sprintf("房间 %s 是公开房间，只允许设置 1 条定时任务", scope.Room)
				}
				writeError(w, http.StatusBadRequest, "task_limit_reached", "Task limit reached", msg)
				return
			}
		}
	}

	if err := task.normalizeAndValidate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_task", "Invalid task", err.Error())
		return
	}

	// 正文里引用了别的房间当输入源时，当前凭据得能读它。
	// 这件事只能在这里判 —— 调度器手上没有任何凭据（见 automation_source.go 的说明）。
	if err := s.checkTemplateSourceRooms(task.Template, scope); err != nil {
		writeError(w, http.StatusBadRequest, "source_room_forbidden", "Source room not readable", err.Error())
		return
	}

	if isNew && scope.Tier == automationPolicySingle {
		// 公开房间里的客户端没有任何凭据（resolveRoomAuth 返回 Required=false，
		// canAccessRoom 恒为 true），服务端无从分辨谁是谁 —— 所以这条任务必须
		// 有一把只有创建者知道的钥匙，否则它谁都能改、谁都能删。
		// 明文只在本响应里出现一次，之后只剩哈希。
		issuedToken = newAutomationToken()
		task.OwnerHash = hashAutomationToken(issuedToken)
	}

	if err := s.upsertAutomationTask(task); err != nil {
		writeError(w, http.StatusInternalServerError, "task_save_failed", "Failed to save task", err.Error())
		return
	}

	resp := map[string]interface{}{
		"task": taskView(task, time.Now()),
	}
	if issuedToken != "" {
		resp["taskToken"] = issuedToken
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Printf("错误: 编码 /tasks 响应失败: %v", err)
	}
}

// handleTaskItem DELETE /tasks/:id，POST /tasks/:id/run。
func (s *ClipboardServer) handleTaskItem(w http.ResponseWriter, r *http.Request) {
	if !s.automationEnabled() {
		writeError(w, http.StatusNotFound, "automation_disabled", "Automation is disabled",
			"定时自动化未启用（automation.enabled = false）")
		return
	}

	scope := s.resolveAutomationScope(r, extractAuthToken(r))
	if !scope.OK {
		writeError(w, http.StatusForbidden, "automation_forbidden", "Automation is not allowed in this room",
			fmt.Sprintf("房间 %s 未开放自动化", scope.Room))
		return
	}

	id, action := splitTaskPath(strings.TrimPrefix(r.URL.Path, s.config.Server.Prefix+"/tasks/"))
	if id == "" {
		writeError(w, http.StatusNotFound, "task_not_found", "Task not found", "任务不存在")
		return
	}

	// /tasks/preview 是「试算一条还没保存的任务」，不走 id 查找。
	// 放在这里而不是注册成独立路由：ServeMux 会把 /tasks/preview 交给 /tasks/ 这棵子树，
	// 多注册一条反而要处理优先级，不如在唯一入口里分一个分支。
	if id == "preview" {
		s.handleTaskPreview(w, r, scope)
		return
	}

	// 同理：/tasks/cron 是表达式校验器，不对应某一条任务。
	if id == "cron" {
		s.handleCronCheck(w, r, scope)
		return
	}

	// /tasks/rooms 是「哪些房间有任务」的清单，也不对应某一条任务 ——
	// 它是「切换房间」那个入口的数据源（见 handleTaskRooms）。
	if id == "rooms" {
		s.handleTaskRooms(w, r, scope)
		return
	}

	task, ok := s.findAutomationTask(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task_not_found", "Task not found", "任务不存在")
		return
	}
	if !scope.Admin && normalizeRoomName(task.Room) != scope.Room {
		writeError(w, http.StatusForbidden, "task_forbidden", "Task belongs to another room", "这条任务属于别的房间")
		return
	}
	if !s.taskOwnedBy(task, scope, extractTaskToken(r)) {
		writeError(w, http.StatusForbidden, "task_forbidden", "Task belongs to someone else", "这条任务不是你创建的")
		return
	}

	switch {
	case action == "" && r.Method == http.MethodDelete:
		s.deleteAutomationTask(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "id": id})

	case action == "run" && r.Method == http.MethodPost:
		s.handleTaskRun(w, r, task, scope)

	case action == "toggle" && r.Method == http.MethodPost:
		s.handleTaskToggle(w, r, task, scope)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Unsupported method or action",
			"仅支持 DELETE /tasks/:id、POST /tasks/:id/run、POST /tasks/:id/toggle")
	}
}

// handleTaskRun 试跑 / 立即发送。
func (s *ClipboardServer) handleTaskRun(w http.ResponseWriter, r *http.Request, task *AutomationTask, scope automationScope) {
	// 默认是**试跑**：只回渲染结果，不投递、不落盘、不动执行记录。
	// 想真发一次必须显式 ?send=1 ——「点一下看看效果」和「现在就发出去」是两件事，
	// 默认值选错的那一边会让用户在房间里留下一条本来只想看看的消息。
	send := isTruthyQuery(r, "send")

	if send {
		// 「立即发送」用**现在**作基准并落一次执行记录里的输出，但不改幂等键 ——
		// 手动发一次不等于这次计划触发已经完成，到点该发还要发。
		output, err := s.executeAutomationTask(task, time.Now(), false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "render_failed", "Render failed", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sent":        true,
			"output":      output,
			"taskId":      task.ID,
			"room":        task.Room,
			"keepHistory": task.KeepHistory,
			"sentAt":      time.Now().Unix(),
		})
		return
	}

	loc, err := task.location()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_task", "Invalid task", err.Error())
		return
	}

	// 试跑的基准默认取**下次触发时刻**，而不是「现在」：
	// 否则下午配一个「每天 09:30、正文写 {{date:+1d}}」的任务，试跑看到的是今天 +1，
	// 而明早真正发出的会是明天 +1 —— 预览反而误导人。
	reference := time.Now()
	if next, ok := task.nextRunAfter(reference); ok {
		reference = next
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("at")); raw != "" {
		parsed, parseErr := parsePreviewReference(raw, loc)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_reference", "Invalid reference time", parseErr.Error())
			return
		}
		reference = parsed
	}

	output, usedAt, err := s.previewAutomationTask(task, reference)
	if err != nil {
		writeError(w, http.StatusBadRequest, "render_failed", "Render failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview":     true,
		"output":      output,
		"taskId":      task.ID,
		"room":        task.Room,
		"keepHistory": task.KeepHistory,
		"referenceAt": usedAt.Unix(),
		"nextRunAt":   task.NextRunAt(time.Now()),
		"scheduledAt": usedAt.Format(time.RFC3339),
	})
}

func parsePreviewReference(raw string, loc *time.Location) (time.Time, error) {
	if at, err := time.Parse(time.RFC3339, raw); err == nil {
		return at, nil
	}
	if at, ok := parseDateToken(raw, loc); ok {
		return at, nil
	}
	return time.Time{}, fmt.Errorf("无法解析基准时刻 %q（可用 RFC3339，或 2026-09-25 / 2026-09-25 09:30）", raw)
}

// handleTaskPreview 试算一条**还没保存**的任务。
//
// 为什么要有这个入口：保存前先看到效果，是这个功能好不好用的分水岭 ——
// 如果只能「先保存再试跑」，用户每改一个字就要落一次盘，而且一个写坏的定义
// 会真的在凌晨把消息发进房间。试算是不落盘的纯函数调用，怎么点都不会有副作用。
func (s *ClipboardServer) handleTaskPreview(w http.ResponseWriter, r *http.Request, scope automationScope) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	var req automationTaskRequest
	r.Body = http.MaxBytesReader(w, r.Body, automationBodyMaxBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Cannot parse request body", "请求体无法解析")
		return
	}
	defer r.Body.Close()

	draft := &AutomationTask{
		Name:      req.Name,
		Enabled:   true,
		Freq:      req.Freq,
		Time:      req.Time,
		Cron:      req.Cron,
		ByWeekday: req.ByWeekday,
		RunAt:     req.RunAt,
		TZ:        req.TZ,
		Room:      scope.Room,
		Template:  req.Template,
		Chain:     req.Chain,
		Sender:    req.Sender,
	}
	s.fillDefaultTZ(draft)
	if err := draft.normalizeAndValidate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_task", "Invalid task", err.Error())
		return
	}

	// 试算也走同一道权限判定：否则用户能在试算里读到正文，
	// 而真正的任务又发不出去 —— 两种结果都很难解释。
	if err := s.checkTemplateSourceRooms(draft.Template, scope); err != nil {
		writeError(w, http.StatusBadRequest, "source_room_forbidden", "Source room not readable", err.Error())
		return
	}

	loc, _ := draft.location()
	reference := time.Now()
	if next, ok := draft.nextRunAfter(reference); ok {
		reference = next
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("at")); raw != "" {
		parsed, err := parsePreviewReference(raw, loc)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reference", "Invalid reference time", err.Error())
			return
		}
		reference = parsed
	}

	output, usedAt, err := s.previewAutomationTask(draft, reference)
	if err != nil {
		writeError(w, http.StatusBadRequest, "render_failed", "Render failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview":      true,
		"output":       output,
		"room":         scope.Room,
		"keepHistory":  false,
		"referenceAt":  usedAt.Unix(),
		"nextRunAt":    draft.NextRunAt(time.Now()),
		"referenceAt2": usedAt.Format(time.RFC3339),
	})
}

// handleTaskToggle 只改开关，其他字段一律不动。
//
// 为什么单独开一个入口，而不是复用 POST /tasks（那是**整体替换**）：列表页上的一个开关，
// 如果要求客户端把整条任务原样回传，任何一处不同步（前端还没拿到新加的字段、
// 或者另一个标签页刚改过）都会顺手把模板 / 动作链改坏。
func (s *ClipboardServer) handleTaskToggle(w http.ResponseWriter, r *http.Request, task *AutomationTask, scope automationScope) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	// 默认翻转；带 ?enabled=1|0 或 body {"enabled":true} 则按值设置。
	// 两种都支持：「开关」这个动作本身是翻转语义，而显式设值对脚本是幂等的。
	enabled := !task.Enabled
	if raw := strings.TrimSpace(r.URL.Query().Get("enabled")); raw != "" {
		enabled = isTruthy(raw)
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, automationBodyMaxBytes)
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		// 空 body 会得到 io.EOF，那就保持翻转语义 —— 不带参数调一下就是「切一下」
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Enabled != nil {
			enabled = *body.Enabled
		}
		r.Body.Close()
	}

	// 值拷贝：ID / Room / OwnerHash / 执行记录都由 upsertAutomationTask 保留，
	// 触发规则没变所以幂等键也不会被清掉 —— 重新打开一条任务不该补发一次。
	updated := *task
	updated.Enabled = enabled
	if err := s.upsertAutomationTask(&updated); err != nil {
		writeError(w, http.StatusInternalServerError, "task_save_failed", "Failed to save task", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"task": taskView(&updated, time.Now()),
	})
}

// handleCronCheck 校验 cron 表达式，并给出接下来的几次触发时刻。
//
// 返回 **200 + valid:false**，而不是 400：这个接口的用途就是校验，表达式不合法是它的
// 正常输出之一，不是「请求写错了」。前端会在每次输入时调它 —— 当成错误抛会让人以为
// 页面坏了，而实际上只是还没输完。
func (s *ClipboardServer) handleCronCheck(w http.ResponseWriter, r *http.Request, scope automationScope) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and POST are allowed", "仅允许 GET / POST 请求")
		return
	}

	expr := strings.TrimSpace(r.URL.Query().Get("expr"))
	tzName := strings.TrimSpace(r.URL.Query().Get("tz"))
	if tzName == "" {
		tzName = s.automationDefaultTZName()
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_timezone", "Invalid time zone",
			fmt.Sprintf("无法识别的时区 %q（写法如 Asia/Shanghai）", tzName))
		return
	}

	resp := map[string]interface{}{
		"expr":  expr,
		"tz":    tzName,
		"valid": false,
		"room":  scope.Room,
	}

	spec, parseErr := ParseCron(expr)
	switch {
	case parseErr != nil:
		resp["error"] = parseErr.Error()
	default:
		next := CronNextTimes(spec, time.Now().In(loc), 5)
		if len(next) == 0 {
			resp["error"] = "这个表达式在未来算不出任何触发时刻（检查「日」和「月」是不是不可能的组合）"
			break
		}
		unix := make([]int64, 0, len(next))
		formatted := make([]string, 0, len(next))
		for _, t := range next {
			unix = append(unix, t.Unix())
			formatted = append(formatted, t.Format("2006-01-02 15:04 Mon"))
		}
		// 归一化后的表达式回给前端：用户写 `0  9 *  * *` 也顺手告诉他标准写法是什么
		resp["expr"] = spec.expr
		resp["valid"] = true
		resp["next"] = unix
		resp["nextFormatted"] = formatted
		// 结构化「翻译」—— 页面拿它拼人话。只给形状，不给成句文案（见 cron.go 里的论证）。
		resp["desc"] = DescribeCron(spec)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Printf("错误: 编码 /tasks/cron 响应失败: %v", err)
	}
}

// handleTaskRooms 列出**有定时任务的房间**（`GET /tasks/rooms`）。
//
// 为什么单独开一个接口：任务列表是**按房间过滤**的（管理员的额外能力是「能用 `?room=` 指向
// 任意房间」，不是「一次看到所有房间的任务」—— 见 handleTaskList 那段论证）。
// 但「切换到哪个房间」这个动作需要先知道**有哪些房间可切**，而那个信息**不属于任何一个房间** ——
// 它得是独立的一次查询，不能塞进某个房间的列表里当附属品。
//
// ⚠️ 权限：**只有管理员**能拿到跨房间的清单。非管理员只拿到自己那个房间 ——
// 把「别的房间有没有装自动化」告诉他，等于泄露房间名的存在性。
//
// ⚠️ 停用的任务**也算**（count 里包含）：用户切过去很可能就是为了把那条停用的打开。
func (s *ClipboardServer) handleTaskRooms(w http.ResponseWriter, r *http.Request, scope automationScope) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed", "仅允许 GET 请求")
		return
	}

	counts := map[string]int{}
	for _, task := range s.snapshotAutomationTasks() {
		room := normalizeRoomName(task.Room)
		if room == "" {
			continue
		}
		// 非管理员只看得到自己那个房间
		if !scope.Admin && room != scope.Room {
			continue
		}
		counts[room]++
	}

	type roomCount struct {
		Room  string `json:"room"`
		Count int    `json:"count"`
	}
	rooms := make([]roomCount, 0, len(counts))
	for room, n := range counts {
		rooms = append(rooms, roomCount{Room: room, Count: n})
	}
	// 排序：房间名升序。map 遍历顺序随机，不排的话界面上每次刷新顺序都在变。
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].Room < rooms[j].Room })

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"room":  scope.Room,
		"admin": scope.Admin,
		"rooms": rooms,
	}); err != nil {
		s.logger.Printf("错误: 编码 /tasks/rooms 响应失败: %v", err)
	}
}

// taskOwnedBy 这条任务当前请求人管不管得着。
//
// admin / room 档：房间内的任务按房间共享 —— 持有房间密码 = 是这个房间的成员，
// 成员之间互相看得到彼此设的任务，是 P1 的取舍（视图里带创建时间，误操作可追溯）。
// 真要一人一份，得引入「任务级 ownership」，那是另一个量级的设计。
//
// single 档：通常落在公开房间上，客户端连凭据都没有，只能靠建任务时下发的那把 task token。
func (s *ClipboardServer) taskOwnedBy(task *AutomationTask, scope automationScope, taskToken string) bool {
	if scope.Admin || scope.Tier == automationPolicyRoom {
		return true
	}
	return automationTokenMatches(taskToken, task.OwnerHash)
}

// taskView 外发形态。
//
// OwnerHash **永远不外发**：它是那把钥匙的哈希，反推不出明文，但也没有任何理由让它离开服务端。
func taskView(task *AutomationTask, now time.Time) map[string]interface{} {
	view := map[string]interface{}{
		"id":          task.ID,
		"name":        task.Name,
		"enabled":     task.Enabled,
		"freq":        task.Freq,
		"time":        task.Time,
		"cron":        task.Cron,
		"byWeekday":   task.ByWeekday,
		"runAt":       task.RunAt,
		"tz":          task.TZ,
		"room":        task.Room,
		"template":    task.Template,
		"chain":       task.Chain,
		"keepHistory": task.KeepHistory,
		"sender":      task.Sender,
		"createdAt":   task.CreatedAt,
		"updatedAt":   task.UpdatedAt,
		"nextRunAt":   task.NextRunAt(now),
		"lastRunAt":   task.LastRunAt,
		"lastStatus":  task.LastStatus,
		"lastError":   task.LastError,
		"lastOutput":  task.LastOutput,
	}
	// cron 任务带上结构化「翻译」：列表里那一行也就不用甩一串 `*/30 9-18 * * 1-5` 了。
	// 解析失败就当没有 —— 视图是只读展示，不该因为一个坏表达式让整个列表 500。
	if task.Freq == automationFreqCron {
		if spec, err := ParseCron(task.Cron); err == nil {
			view["desc"] = DescribeCron(spec)
		}
	}
	return view
}

func (s *ClipboardServer) automationRoomLimit(scope automationScope) int {
	if scope.Admin {
		return 0 // 0 = 不限。管理员是在救火，不该被自己设的配额挡住
	}
	if scope.Tier == automationPolicySingle {
		return 1
	}
	return automationMaxTasksPerRoom
}

func (s *ClipboardServer) countAutomationTasksInRoom(room string) int {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()
	return s.countAutomationTasksInRoomLocked(room)
}

// AutomationCapability 给 /server 下发的能力声明。
//
// 为什么由服务端下发、而不是前端自己判断「这个房间有没有密码」：
// 「这个房间能不能装自动化」由三件事共同决定（全局密码、房间密码、roomAuth.automation），
// 前端要自己算就得把整套鉴权规则复制一份，然后随着服务端改动慢慢漂开 ——
// 表现就是「面板显示了但保存 403」或者反过来「明明能用却不显示面板」。能力声明只有一个来源。
func (s *ClipboardServer) AutomationCapability(r *http.Request) map[string]interface{} {
	if !s.automationEnabled() {
		return map[string]interface{}{"enabled": false}
	}
	scope := s.resolveAutomationScope(r, extractAuthToken(r))
	return map[string]interface{}{
		"enabled":   true,
		"room":      scope.Room,
		"tier":      scope.Tier, // admin | room | single | none
		"allowed":   scope.OK,
		"admin":     scope.Admin,
		"max":       s.automationRoomLimit(scope),
		"defaultTZ": s.automationDefaultTZName(),
		"vars":      RenderVariableNames(),
		"actions":   ServerRenderActionList(),
	}
}

// extractTaskToken 取「single 档房间」里那把任务钥匙。
// 头优先（管理页用），退到查询串（curl / 快捷指令更顺手）。
func extractTaskToken(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Task-Token")); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get("taskToken"))
}

// splitTaskPath 把 `/tasks/` 之后的部分拆成 id 与动作。
// 例如 "abc/run" → ("abc", "run")；"abc" → ("abc", "")。
func splitTaskPath(rest string) (string, string) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", ""
	}
	if idx := strings.LastIndex(rest, "/"); idx >= 0 {
		return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+1:])
	}
	return strings.TrimSpace(rest), ""
}

func isTruthyQuery(r *http.Request, key string) bool {
	return isTruthy(r.URL.Query().Get(key))
}

func isTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
