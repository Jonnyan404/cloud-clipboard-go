package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 测试用的房间布局，覆盖全部四档策略：
//
//	secret    房间密码，未写 automation → 跟随鉴权档位 = room
//	lobby     开放房间，显式 automation=single
//	public    开放房间，未写 automation → none（默认关）
//	locked    房间密码，但显式 automation=none（配置层逃生阀）
//	default   没配过 + 设了全局密码 → 回落全局 = room
const (
	testGlobalPass = "global-pass"
	testRoomPass   = "room-pass"
	testLockedPass = "locked-pass"
)

func newAutomationServer(t *testing.T) *ClipboardServer {
	t.Helper()
	cfg := &Config{}
	cfg.Server.Auth = testGlobalPass
	cfg.Server.RoomAuth = RoomAuthConfig{
		"secret": {Password: testRoomPass},
		"lobby":  {Open: true, Automation: automationPolicySingle},
		"public": {Open: true},
		"locked": {Password: testLockedPass, Automation: automationPolicyNone},
	}
	cfg.Text.Limit = 4096
	cfg.Automation.Enabled = true
	cfg.Automation.TickSeconds = 1
	cfg.Automation.GraceSeconds = 60

	return &ClipboardServer{
		config:          cfg,
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    NewMessageQueue(10, nil),
		uploadFileMap:   make(map[string]File),
		historyFilePath: filepath.Join(t.TempDir(), "history.json"),
		room_ws:         make(map[*websocket.Conn]string),
		websockets:      make(map[*websocket.Conn]bool),
		connDeviceIDMap: make(map[*websocket.Conn]string),
		deviceConnected: make(map[string]DeviceMeta),
		roomStats:       make(map[string]*RoomStat),
	}
}

func taskReq(method, target, auth, taskToken, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	if taskToken != "" {
		req.Header.Set("X-Task-Token", taskToken)
	}
	return req
}

func taskBody(extra string) string {
	base := `"freq":"daily","time":"09:30","template":"明天是 {{date:+1d}}（{{weekday:+1d}}）"`
	if extra != "" {
		base += "," + extra
	}
	return "{" + base + "}"
}

// ── 凭据分档 ────────────────────────────────────────────────────────

func TestAutomationScopeTiers(t *testing.T) {
	s := newAutomationServer(t)

	cases := []struct {
		room     string
		token    string
		wantOK   bool
		wantTier string
		why      string
	}{
		{"secret", "", false, automationPolicyNone, "有房间密码但没给凭据"},
		{"secret", testRoomPass, true, automationPolicyRoom, "房间密码 → 本房间不限条数"},
		{"secret", testGlobalPass, true, automationTierAdmin, "全局密码 → 管理员"},
		{"lobby", "", true, automationPolicySingle, "开放房间显式开了 single"},
		{"public", "", false, automationPolicyNone, "开放房间默认关闭（这是「什么都不配」的部署默认行为）"},
		{"locked", testLockedPass, false, automationPolicyNone, "配置层逃生阀：连房间密码也开不了"},
		{"locked", testGlobalPass, true, automationTierAdmin, "全局密码仍是管理员（那是改配置文件的活）"},
		{"default", "", false, automationPolicyNone, "default 回落全局密码，无凭据不可用"},
		{"某个没配过的房间", testRoomPass, false, automationPolicyNone, "别的房间的密码不能跨房间用"},
	}

	for _, c := range cases {
		t.Run(c.room+"/"+c.token, func(t *testing.T) {
			req := taskReq(http.MethodGet, "/tasks?room="+c.room, c.token, "", "")
			got := s.resolveAutomationScope(req, c.token)
			if got.OK != c.wantOK || got.Tier != c.wantTier {
				t.Fatalf("%s：得到 OK=%v tier=%s，期望 OK=%v tier=%s",
					c.why, got.OK, got.Tier, c.wantOK, c.wantTier)
			}
		})
	}
}

func TestTasksEndpointRejectsUnauthorizedByDefault(t *testing.T) {
	s := newAutomationServer(t)

	for _, target := range []string{"/tasks?room=public", "/tasks?room=secret", "/tasks?room=default"} {
		req := taskReq(http.MethodGet, target, "", "", "")
		w := httptest.NewRecorder()
		s.handleTasks(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s 无凭据应 403，得到 %d", target, w.Code)
		}
		if code := errorCode(t, w); code != "automation_forbidden" {
			t.Errorf("%s 错误码应为 automation_forbidden，得到 %q", target, code)
		}
	}
}

func TestTasksEndpointDisabled(t *testing.T) {
	s := newAutomationServer(t)
	s.config.Automation.Enabled = false

	req := taskReq(http.MethodGet, "/tasks?room=secret", testRoomPass, "", "")
	w := httptest.NewRecorder()
	s.handleTasks(w, req)
	if w.Code != http.StatusNotFound || errorCode(t, w) != "automation_disabled" {
		t.Fatalf("关闭时应 404 automation_disabled，得到 %d / %q", w.Code, errorCode(t, w))
	}
}

// ── CRUD ────────────────────────────────────────────────────────────

func TestTaskCRUDWithRoomPassword(t *testing.T) {
	s := newAutomationServer(t)

	// 建
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "", taskBody(`"name":"值班提醒"`)))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var created struct {
		Task struct {
			ID   string `json:"id"`
			Room string `json:"room"`
			Name string `json:"name"`
		} `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if created.Task.ID == "" || created.Task.Room != "secret" {
		t.Fatalf("创建结果不对: %+v", created.Task)
	}

	// 列
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=secret", testRoomPass, "", ""))
	var listed struct {
		Tasks []struct {
			ID          string `json:"id"`
			NextRunAt   int64  `json:"nextRunAt"`
			KeepHistory bool   `json:"keepHistory"`
		} `json:"tasks"`
		Tier string `json:"tier"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	if len(listed.Tasks) != 1 || listed.Tasks[0].ID != created.Task.ID {
		t.Fatalf("列表应含刚建的任务，得到 %+v", listed.Tasks)
	}
	if listed.Tasks[0].NextRunAt == 0 {
		t.Fatal("列表应带下次触发时间")
	}
	if listed.Tasks[0].KeepHistory {
		t.Fatal("keepHistory 默认应为 false（不占房间历史额度）")
	}
	if listed.Tier != automationPolicyRoom {
		t.Fatalf("tier 应为 room，得到 %q", listed.Tier)
	}

	// 删
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodDelete, "/tasks/"+created.Task.ID+"?room=secret", testRoomPass, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("删除应 200，得到 %d", w.Code)
	}
	if got := len(s.snapshotAutomationTasks()); got != 0 {
		t.Fatalf("删除后应为 0 条，得到 %d", got)
	}
}

// 房间来自鉴权上下文，请求体里的 room 必须被忽略 —— 否则一条已授权的任务
// 就能把消息发到别的房间去。
func TestTaskRoomCannotBeSpoofedByBody(t *testing.T) {
	s := newAutomationServer(t)

	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"freq":"daily","time":"09:30","template":"hi","room":"别人的房间"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}

	tasks := s.snapshotAutomationTasks()
	if len(tasks) != 1 {
		t.Fatalf("应有 1 条任务，得到 %d", len(tasks))
	}
	if tasks[0].Room != "secret" {
		t.Fatalf("房间必须来自凭据，得到 %q", tasks[0].Room)
	}
}

func TestRoomCredentialCannotTouchAnotherRoomsTask(t *testing.T) {
	s := newAutomationServer(t)

	// 用管理员的身份在 default 房间建一条
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=default", testGlobalPass, "", taskBody("")))
	if w.Code != http.StatusOK {
		t.Fatalf("管理员创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var created struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)

	// 拿着别的房间的密码去删 → 403
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodDelete, "/tasks/"+created.Task.ID+"?room=secret", testRoomPass, "", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("跨房间删除应 403，得到 %d", w.Code)
	}
	if got := len(s.snapshotAutomationTasks()); got != 1 {
		t.Fatalf("任务不该被删掉，当前 %d 条", got)
	}

	// 拿到 secret 房间去查，也不该看到 default 的那条
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=secret", testRoomPass, "", ""))
	var listed struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Tasks) != 0 {
		t.Fatalf("别的房间的任务不该出现在列表里，得到 %d 条", len(listed.Tasks))
	}
}

func TestGlobalPasswordIsAdminAcrossRooms(t *testing.T) {
	s := newAutomationServer(t)

	// 在没配过 roomAuth 的房间里建（回落全局密码 → policy=room，但凭据是管理员）
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=aroom", testGlobalPass, "", taskBody("")))
	if w.Code != http.StatusOK {
		t.Fatalf("管理员应能在任意房间创建，得到 %d：%s", w.Code, w.Body.String())
	}

	// 也能看见 secret 房间里的任务
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "", taskBody("")))
	if w.Code != http.StatusOK {
		t.Fatalf("房间密码创建应 200，得到 %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=secret", testGlobalPass, "", ""))
	var listed struct {
		Tasks []struct {
			Room string `json:"room"`
		} `json:"tasks"`
		Admin bool `json:"admin"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if !listed.Admin || len(listed.Tasks) != 1 {
		t.Fatalf("管理员应看到房间内任务，得到 admin=%v tasks=%d", listed.Admin, len(listed.Tasks))
	}
}

// ── single 档（公开房间）────────────────────────────────────────────

func TestSingleTierIssuesTaskTokenAndLimitsToOne(t *testing.T) {
	s := newAutomationServer(t)

	// 第一条：不需要任何凭据（公开房间），但会拿到一把钥匙
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=lobby", "", "", taskBody(`"name":"每日待办"`)))
	if w.Code != http.StatusOK {
		t.Fatalf("公开房间首条应允许，得到 %d：%s", w.Code, w.Body.String())
	}
	var created struct {
		Task      map[string]interface{} `json:"task"`
		TaskToken string                 `json:"taskToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.TaskToken == "" {
		t.Fatal("public/single 档必须下发 task token —— 否则这条任务谁都能改删")
	}
	if _, leaked := created.Task["ownerHash"]; leaked {
		t.Fatal("ownerHash 不该外发")
	}
	id, _ := created.Task["id"].(string)

	// 第二条：超限
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=lobby", "", "", taskBody(`"name":"第二条"`)))
	if w.Code != http.StatusBadRequest || errorCode(t, w) != "task_limit_reached" {
		t.Fatalf("公开房间第二条应被拒，得到 %d / %q", w.Code, errorCode(t, w))
	}

	// 列表：没有钥匙就看不到内容，只知道「有 1 条不归你管」
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=lobby", "", "", ""))
	var listed struct {
		Tasks        []json.RawMessage `json:"tasks"`
		ForeignCount int               `json:"foreignCount"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Tasks) != 0 || listed.ForeignCount != 1 {
		t.Fatalf("无钥匙时应看到 0 条 + foreignCount=1，得到 %d / %d", len(listed.Tasks), listed.ForeignCount)
	}

	// 带钥匙：看得到、删得掉
	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=lobby", "", created.TaskToken, ""))
	listed.Tasks = nil
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Tasks) != 1 {
		t.Fatalf("带钥匙应看到 1 条，得到 %d", len(listed.Tasks))
	}

	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodDelete, "/tasks/"+id+"?room=lobby", "", "", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("没钥匙不该能删，得到 %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodDelete, "/tasks/"+id+"?room=lobby", "", created.TaskToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("带钥匙应能删，得到 %d", w.Code)
	}
}

// ── 试算 ────────────────────────────────────────────────────────────

func TestPreviewDoesNotPersistOrSend(t *testing.T) {
	s := newAutomationServer(t)

	body := `{"freq":"daily","time":"09:30","template":"明天是 {{date:+1d}}（{{weekday:+1d}}）","at":"2026-09-24T09:30:00+08:00"}`
	w := httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost, "/tasks/preview?room=secret&at=2026-09-24T09:30:00%2B08:00", testRoomPass, "", body))
	if w.Code != http.StatusOK {
		t.Fatalf("试算应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var resp struct {
		Preview bool   `json:"preview"`
		Output  string `json:"output"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Preview || !strings.Contains(resp.Output, "09-25") {
		t.Fatalf("试算结果不对: %+v", resp)
	}

	// 关键：试算不留痕
	if got := len(s.snapshotAutomationTasks()); got != 0 {
		t.Fatalf("试算不该创建任务，得到 %d 条", got)
	}
	if got := len(s.messageQueue.List); got != 0 {
		t.Fatalf("试算不该发消息，得到 %d 条历史", got)
	}
}

func TestPreviewRejectsBadTemplate(t *testing.T) {
	s := newAutomationServer(t)
	w := httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost, "/tasks/preview?room=secret", testRoomPass, "",
		`{"freq":"daily","time":"09:30","template":"{{nope}}"}`))
	if w.Code != http.StatusBadRequest || errorCode(t, w) != "invalid_task" {
		t.Fatalf("未知变量应 400 invalid_task，得到 %d / %q", w.Code, errorCode(t, w))
	}
}

// ── 投递行为 ────────────────────────────────────────────────────────

func TestEphemeralDeliveryDoesNotConsumeRoomHistory(t *testing.T) {
	s := newAutomationServer(t)

	// 保留历史：进队列、占额度
	s.deliverMessage("text", "保留历史的定时消息", "secret", messageSource{
		DeviceName: "定时任务", Source: "automation", ScheduledAt: 1000,
	}, true)
	if got := len(s.messageQueue.List); got != 1 {
		t.Fatalf("保留历史时应入队，得到 %d", got)
	}

	// 不保留历史（默认）：只广播，不入队
	before := len(s.messageQueue.List)
	s.deliverMessage("text", "临时定时消息", "secret", messageSource{
		DeviceName: "定时任务", Source: "automation", ScheduledAt: 2000,
	}, false)
	if got := len(s.messageQueue.List); got != before {
		t.Fatalf("不保留历史时不该占房间历史额度，得到 %d（原 %d）", got, before)
	}
}

func TestAutomationMessageCarriesSourceAndUniqueID(t *testing.T) {
	s := newAutomationServer(t)

	first := s.deliverMessage("text", "一", "secret", messageSource{DeviceName: "值班机器人", Source: "automation", ScheduledAt: 111}, false)
	second := s.deliverMessage("text", "二", "secret", messageSource{DeviceName: "值班机器人", Source: "automation", ScheduledAt: 222}, false)

	if first.Data.ID() <= 0 || second.Data.ID() <= 0 {
		t.Fatalf("临时消息也要有唯一 ID（前端拿它做列表 key）：%d / %d", first.Data.ID(), second.Data.ID())
	}
	if first.Data.ID() == second.Data.ID() {
		t.Fatal("两条临时消息的 ID 不该相同")
	}
	if first.Data.TextReceive.Source != "automation" {
		t.Fatalf("应标记来源，得到 %q", first.Data.TextReceive.Source)
	}
	if first.Data.TextReceive.ScheduledAt != 111 {
		t.Fatalf("应带上预定触发时刻，得到 %d", first.Data.TextReceive.ScheduledAt)
	}
	if first.Data.TextReceive.SenderIP != "" {
		t.Fatalf("定时消息没有 IP，不该编一个：%q", first.Data.TextReceive.SenderIP)
	}
	// 发送者显示名要靠 name 字段（前端 deviceLabel 的取值顺序是 name → os → type）
	if got := first.Data.TextReceive.SenderDevice["name"]; got != "值班机器人" {
		t.Fatalf("发送者显示名应为「值班机器人」，得到 %q（完整: %v）", got, first.Data.TextReceive.SenderDevice)
	}
}

// ── 调度器 ──────────────────────────────────────────────────────────

func TestSchedulerFiresOnceAndIsIdempotent(t *testing.T) {
	s := newAutomationServer(t)
	// ⚠️ 用**合成时刻**，不依赖「测试恰好跑在某一分钟的第几秒」。
	// 拿 time.Now() 当基准的调度测试会随机器启动耗时随机变红 —— 那种红没有信息量。
	synthetic := time.Date(2026, 10, 6, 12, 0, 30, 0, time.Local)

	task := testTask(automationFreqDaily, "12:00")
	task.Room = "secret"
	task.Template = "{{date}} 到点了"
	task.KeepHistory = true // 用历史队列当「发了几条」的观察点
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(synthetic)
	if got := len(s.messageQueue.List); got != 1 {
		t.Fatalf("应投递 1 条，得到 %d", got)
	}
	if got := s.snapshotAutomationTasks()[0].LastStatus; got != "ok" {
		t.Fatalf("执行状态应为 ok，得到 %q", got)
	}
	// 正文要用**预定时刻**求值
	if got := s.messageQueue.List[0].Data.TextReceive.Content; got != "2026-10-06 到点了" {
		t.Fatalf("正文应为预定时刻渲染，得到 %q", got)
	}

	// 再扫一遍：幂等键必须挡住重复投递
	s.runDueAutomationTasks(synthetic.Add(30 * time.Second))
	if got := len(s.messageQueue.List); got != 1 {
		t.Fatalf("同一趟不该重复投递，得到 %d 条", got)
	}
}

func TestSchedulerSkipsMissedWindows(t *testing.T) {
	s := newAutomationServer(t)
	s.config.Automation.GraceSeconds = 60
	synthetic := time.Date(2026, 10, 6, 12, 0, 30, 0, time.Local)

	// 两小时前的点 —— 远超 60 秒的补发窗口
	missed := testTask(automationFreqDaily, "10:00")
	missed.Room = "secret"
	missed.KeepHistory = true
	if err := missed.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(missed); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 30 秒前的点 —— 在窗口内，应当补发
	recent := testTask(automationFreqDaily, "12:00")
	recent.ID = "task-2"
	recent.Room = "secret"
	recent.KeepHistory = true
	if err := recent.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(recent); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(synthetic)

	if got := len(s.messageQueue.List); got != 1 {
		t.Fatalf("只有窗口内的那条该发出去，得到 %d 条", got)
	}

	byID := map[string]*AutomationTask{}
	for _, task := range s.snapshotAutomationTasks() {
		byID[task.ID] = task
	}
	if got := byID[missed.ID].LastStatus; got != "skipped" {
		t.Fatalf("错过的那条应记为 skipped，得到 %q（错误: %q）", got, byID[missed.ID].LastError)
	}
	if got := byID[recent.ID].LastStatus; got != "ok" {
		t.Fatalf("窗口内那条应为 ok，得到 %q", got)
	}

	// 再扫一遍不该重新评估（否则 tasks.json 会每 tick 被重写一次）
	before := byID[missed.ID].LastRunAt
	s.runDueAutomationTasks(synthetic.Add(30 * time.Second))
	for _, task := range s.snapshotAutomationTasks() {
		if task.ID == missed.ID && task.LastRunAt != before {
			t.Fatal("已记过 skipped 的任务不该被每 tick 重新写一遍")
		}
	}
}

func TestOnceTaskDisablesItselfAfterRunning(t *testing.T) {
	s := newAutomationServer(t)
	synthetic := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)

	task := testTask(automationFreqOnce, "")
	task.RunAt = synthetic.Format(time.RFC3339)
	task.Room = "secret"
	task.KeepHistory = true
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(synthetic.Add(5 * time.Second))

	after := s.snapshotAutomationTasks()[0]
	if after.Enabled {
		t.Fatal("「仅一次」跑完应自禁用，否则状态会一直显示「已启用」")
	}
	if after.LastStatus != "ok" {
		t.Fatalf("状态应为 ok，得到 %q", after.LastStatus)
	}
}

func TestTaskExecutionRendersTemplateAtScheduledInstant(t *testing.T) {
	s := newAutomationServer(t)

	task := testTask(automationFreqDaily, "09:30")
	task.Room = "secret"
	// 用「预定时刻」而不是「实际时刻」求值 —— 补发时正文才不会是错的
	scheduled := time.Date(2026, 9, 24, 9, 30, 0, 0, time.Local)
	task.Template = "明天是 {{date:+1d}}（{{weekday:+1d}}）"
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}

	out, err := s.executeAutomationTask(task, scheduled, true)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if out != "明天是 2026-09-25（周五）" {
		t.Fatalf("得到 %q", out)
	}
}

func TestServerExposesAutomationCapability(t *testing.T) {
	s := newAutomationServer(t)

	req := taskReq(http.MethodGet, "/server?room=lobby", "", "", "")
	w := httptest.NewRecorder()
	s.handle_server(w, req)

	var resp struct {
		Automation struct {
			Enabled bool     `json:"enabled"`
			Allowed bool     `json:"allowed"`
			Tier    string   `json:"tier"`
			Room    string   `json:"room"`
			Max     int      `json:"max"`
			Vars    []string `json:"vars"`
			Actions []struct {
				ID string `json:"id"`
			} `json:"actions"`
		} `json:"automation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 /server 失败: %v", err)
	}
	if !resp.Automation.Enabled || !resp.Automation.Allowed {
		t.Fatalf("能力声明不对: %+v", resp.Automation)
	}
	if resp.Automation.Tier != automationPolicySingle || resp.Automation.Room != "lobby" {
		t.Fatalf("档位/房间不对: %+v", resp.Automation)
	}
	if resp.Automation.Max != 1 {
		t.Fatalf("single 档上限应为 1，得到 %d", resp.Automation.Max)
	}
	if len(resp.Automation.Vars) == 0 || len(resp.Automation.Actions) == 0 {
		t.Fatal("应下发可用变量与动作，前端才能渲染胶囊、置灰不可用的动作")
	}
}

func TestAutomationPageRenders(t *testing.T) {
	s := newAutomationServer(t)
	req := taskReq(http.MethodGet, "/automation?room=secret", "", "", "")
	w := httptest.NewRecorder()
	s.handleAutomationPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("页面应 200，得到 %d", w.Code)
	}
	body := w.Body.String()
	// 页面里必须出现给用户看的变量原文（分隔符换对了，`{{ }}` 才不会被模板引擎吃掉）
	if !strings.Contains(body, "{{date:+1d}}") {
		t.Fatal("页面应保留 {{date:+1d}} 这样的变量示例文案")
	}
	if !strings.Contains(body, `room: "secret"`) {
		t.Fatal("页面应带上房间参数")
	}
	if strings.Contains(body, "[[") {
		t.Fatal("模板分隔符残留，说明 Go 占位没被替换")
	}
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		return ""
	}
	return body.Code
}

// ── cron / 开关 / 默认时区 ──────────────────────────────────────────

func TestCronTaskCanBeCreated(t *testing.T) {
	s := newAutomationServer(t)

	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"name":"工作日整点","freq":"cron","cron":"0 9-18 * * 1-5","template":"整点了"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}

	var created struct {
		Task struct {
			Freq      string `json:"freq"`
			Cron      string `json:"cron"`
			Time      string `json:"time"`
			TZ        string `json:"tz"`
			NextRunAt int64  `json:"nextRunAt"`
		} `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if created.Task.Freq != automationFreqCron || created.Task.Cron != "0 9-18 * * 1-5" {
		t.Fatalf("freq / cron 不对: %+v", created.Task)
	}
	if created.Task.Time != "" {
		t.Fatalf("cron 任务不该同时留下 time 字段（%q）—— 两个排期字段并存必然有一天被读错",
			created.Task.Time)
	}
	if created.Task.NextRunAt == 0 {
		t.Fatal("cron 任务也要能算出下次触发时间")
	}
	if created.Task.TZ != defaultAutomationTZName {
		t.Fatalf("未写时区时应填入默认值 %s，得到 %q", defaultAutomationTZName, created.Task.TZ)
	}
}

func TestCronTaskRejectsBadExpression(t *testing.T) {
	s := newAutomationServer(t)
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"freq":"cron","cron":"0 */5 * * * *","template":"x"}`))
	if w.Code != http.StatusBadRequest || errorCode(t, w) != "invalid_task" {
		t.Fatalf("6 字段表达式应 400 invalid_task，得到 %d / %q", w.Code, errorCode(t, w))
	}
	var body struct {
		Message string `json:"message"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if !strings.Contains(body.Message, "去掉第一段") {
		t.Fatalf("错误信息要告诉用户怎么改，得到 %q", body.Message)
	}
}

func TestCronCheckEndpoint(t *testing.T) {
	s := newAutomationServer(t)

	w := httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodGet,
		"/tasks/cron?room=secret&expr=0%209%20*%20*%20*&tz=Asia%2FShanghai", testRoomPass, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var ok struct {
		Valid         bool     `json:"valid"`
		TZ            string   `json:"tz"`
		NextFormatted []string `json:"nextFormatted"`
	}
	json.Unmarshal(w.Body.Bytes(), &ok)
	if !ok.Valid || len(ok.NextFormatted) != 5 {
		t.Fatalf("应回 valid:true 和 5 个未来时刻，得到 %+v", ok)
	}
	if ok.TZ != "Asia/Shanghai" {
		t.Fatalf("应回显时区，得到 %q", ok.TZ)
	}

	// ⚠️ 表达式不合法时返回 **200 + valid:false**，不是 400：
	// 这个接口的用途就是校验，不合法是它的正常输出 —— 前端每次输入都会调它，
	// 当成错误抛会让人以为页面坏了。
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodGet, "/tasks/cron?room=secret&expr=nope", testRoomPass, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("表达式不合法也应 200，得到 %d", w.Code)
	}
	var bad struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	json.Unmarshal(w.Body.Bytes(), &bad)
	if bad.Valid || bad.Error == "" {
		t.Fatalf("应回 valid:false + error，得到 %+v", bad)
	}
}

func TestDefaultTimezoneIsShanghai(t *testing.T) {
	loc, err := time.LoadLocation(defaultAutomationTZName)
	if err != nil {
		t.Skipf("本机没有时区数据，跳过: %v", err)
	}

	s := newAutomationServer(t)
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"name":"时区检查","freq":"daily","time":"09:30","template":"x"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}

	var created struct {
		Task struct {
			TZ        string `json:"tz"`
			NextRunAt int64  `json:"nextRunAt"`
		} `json:"task"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Task.TZ != defaultAutomationTZName {
		t.Fatalf("默认时区应为 %s，得到 %q", defaultAutomationTZName, created.Task.TZ)
	}

	// 关键断言：下次触发换算到上海时区**就是 09:30**。
	// 只查 TZ 字段是不够的 —— 字段写对了但没参与计算，一样会发错时刻。
	next := time.Unix(created.Task.NextRunAt, 0).In(loc)
	if next.Hour() != 9 || next.Minute() != 30 {
		t.Fatalf("下次触发在上海时区应是 09:30，实际是 %s", next.Format("2006-01-02 15:04 -07:00"))
	}
}

func TestExplicitTimezoneOverridesDefault(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skipf("本机没有时区数据，跳过: %v", err)
	}
	s := newAutomationServer(t)
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"name":"纽约","freq":"daily","time":"09:30","tz":"America/New_York","template":"x"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var created struct {
		Task struct {
			TZ        string `json:"tz"`
			NextRunAt int64  `json:"nextRunAt"`
		} `json:"task"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Task.TZ != "America/New_York" {
		t.Fatalf("显式时区应被保留，得到 %q", created.Task.TZ)
	}
	loc, _ := time.LoadLocation("America/New_York")
	next := time.Unix(created.Task.NextRunAt, 0).In(loc)
	if next.Hour() != 9 || next.Minute() != 30 {
		t.Fatalf("下次触发在纽约时区应是 09:30，实际是 %s", next.Format("2006-01-02 15:04 -07:00"))
	}
}

func TestTaskToggleStopsAndResumesDelivery(t *testing.T) {
	s := newAutomationServer(t)
	body := `{"name":"可开关的任务","freq":"daily","time":"` +
		time.Now().Format("15:04") + `","template":"滴答","keepHistory":true}`

	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "", body))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var created struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)

	// 开着的任务到点会发
	s.runDueAutomationTasks(time.Now())
	if got := len(s.messageQueue.List); got != 1 {
		t.Fatalf("启用时应投递 1 条，得到 %d", got)
	}
	s.messageQueue.List = nil

	// 关掉
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost,
		"/tasks/"+created.Task.ID+"/toggle?room=secret", testRoomPass, "", `{"enabled":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("停用应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	var off struct {
		Task struct {
			Enabled bool   `json:"enabled"`
			Freq    string `json:"freq"`
		} `json:"task"`
	}
	json.Unmarshal(w.Body.Bytes(), &off)
	if off.Task.Enabled {
		t.Fatal("应当已经停用")
	}

	// 停用之后不该再投递（清掉幂等键再扫，确保是「开关」在挡，不是幂等键）
	tasks := s.snapshotAutomationTasks()
	for _, task := range tasks {
		task.LastRunKey = ""
	}
	s.runDueAutomationTasks(time.Now())
	if got := len(s.messageQueue.List); got != 0 {
		t.Fatalf("停用后不该投递，得到 %d 条", got)
	}

	// 再开回来：模板和动作链不能被开关弄丢
	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost,
		"/tasks/"+created.Task.ID+"/toggle?room=secret&enabled=1", testRoomPass, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("启用应 200，得到 %d：%s", w.Code, w.Body.String())
	}
	after := s.snapshotAutomationTasks()
	if len(after) != 1 || !after[0].Enabled {
		t.Fatalf("应当已重新启用: %+v", after)
	}
	if after[0].Template != "滴答" || after[0].Room != "secret" {
		t.Fatalf("开关不该动其他字段: %+v", after[0])
	}
}

func TestTaskToggleRespectsOwnership(t *testing.T) {
	s := newAutomationServer(t)

	// 公开房间（single 档）里的那条：没有钥匙的人连开关都不该能拨
	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=lobby", "", "", taskBody(`"name":"lobby 任务"`)))
	var created struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		TaskToken string `json:"taskToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)

	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost, "/tasks/"+created.Task.ID+"/toggle?room=lobby", "", "", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("没有钥匙不该能开关别人的任务，得到 %d", w.Code)
	}

	w = httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodPost,
		"/tasks/"+created.Task.ID+"/toggle?room=lobby", "", created.TaskToken, `{"enabled":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("带钥匙应能开关，得到 %d：%s", w.Code, w.Body.String())
	}
}

// ── cron 的「翻译」 ─────────────────────────────────────────────────

// /tasks/cron 要带回结构化描述。**必须是结构而不是成句的中文** ——
// 管理页有四份语言，服务端拼中文会让 en / ja 用户看到中文。
func TestCronCheckReturnsStructuredDescription(t *testing.T) {
	s := newAutomationServer(t)

	w := httptest.NewRecorder()
	s.handleTaskItem(w, taskReq(http.MethodGet,
		"/tasks/cron?room=secret&expr=*/30%209-18%20*%20*%201-5&tz=Asia%2FShanghai", testRoomPass, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("应 200，得到 %d：%s", w.Code, w.Body.String())
	}

	var body struct {
		Valid bool `json:"valid"`
		Desc  struct {
			Mode     string `json:"mode"`
			N        int    `json:"n"`
			Hours    string `json:"hours"`
			Day      string `json:"day"`
			Weekdays []int  `json:"weekdays"`
		} `json:"desc"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if !body.Valid {
		t.Fatalf("表达式应当合法：%s", w.Body.String())
	}
	if body.Desc.Mode != "everyNMinutes" || body.Desc.N != 30 {
		t.Fatalf("时刻部分应是「每 30 分钟」，得到 %+v", body.Desc)
	}
	// 只说「每 30 分钟」而漏掉 9-18 就是撒谎，这条断言盯的就是它
	if body.Desc.Hours != "9-18" {
		t.Fatalf("必须带上小时范围 9-18，得到 %q", body.Desc.Hours)
	}
	if body.Desc.Day != "weekly" || len(body.Desc.Weekdays) != 5 {
		t.Fatalf("日期部分应是 5 个工作日，得到 %+v", body.Desc)
	}

	// 表述里不能出现中文 —— 出现就说明又退回「服务端拼文案」了
	if strings.ContainsAny(w.Body.String(), "每天每分小时周月日") {
		t.Error("desc 里出现了中文字符，说明服务端在拼成句文案，多语言会崩")
	}
}

// 列表里的每条 cron 任务也要带 desc：否则编辑框说人话、列表甩一串表达式，
// 用户在两个地方看到两种东西。
func TestTaskViewCarriesCronDescription(t *testing.T) {
	s := newAutomationServer(t)

	w := httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodPost, "/tasks?room=secret", testRoomPass, "",
		`{"name":"周会","freq":"cron","cron":"0 10 * * 1","template":"x"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("创建应 200，得到 %d：%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.handleTasks(w, taskReq(http.MethodGet, "/tasks?room=secret", testRoomPass, "", ""))
	var list struct {
		Tasks []struct {
			Freq string `json:"freq"`
			Cron string `json:"cron"`
			Desc *struct {
				Mode     string   `json:"mode"`
				Day      string   `json:"day"`
				Times    []string `json:"times"`
				Weekdays []int    `json:"weekdays"`
			} `json:"desc"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	if len(list.Tasks) != 1 || list.Tasks[0].Desc == nil {
		t.Fatalf("列表里应当带 desc：%s", w.Body.String())
	}
	d := list.Tasks[0].Desc
	if d.Mode != "times" || len(d.Times) != 1 || d.Times[0] != "10:00" {
		t.Fatalf("时刻应是 10:00，得到 %+v", d)
	}
	if d.Day != "weekly" || len(d.Weekdays) != 1 || d.Weekdays[0] != 1 {
		t.Fatalf("应是每周一，得到 %+v", d)
	}
}
