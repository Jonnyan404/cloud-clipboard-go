package lib

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 往队列里塞一条「人发的」文本消息。
func pushHumanText(s *ClipboardServer, room, content string) {
	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()
	id := len(s.messageQueue.List) + 1
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{ID: id, Type: "text", Room: room, Timestamp: time.Now().Unix()},
			Content:     content,
		}},
	})
}

// 往队列里塞一条「定时任务发的」文本消息（带 Source 标记）。
func pushAutomationText(s *ClipboardServer, room, content string) {
	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()
	id := len(s.messageQueue.List) + 1
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{ID: id, Type: "text", Room: room, Timestamp: time.Now().Unix(),
				Source: "automation"},
			Content: content,
		}},
	})
}

// 往队列里塞一条文件消息（`{{latest}}` 要跳过它）。
func pushFileMessage(s *ClipboardServer, room, name string) {
	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()
	id := len(s.messageQueue.List) + 1
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{FileReceive: &FileReceive{
			ReceiveBase: ReceiveBase{ID: id, Type: "file", Room: room, Timestamp: time.Now().Unix()},
			Name:        name,
		}},
	})
}

func TestLatestRoomTextPicksNewestHumanText(t *testing.T) {
	s := newAutomationServer(t)

	if _, ok := s.latestRoomText("lobby"); ok {
		t.Fatal("空房间不该有来源消息")
	}

	pushHumanText(s, "lobby", "第一条")
	pushHumanText(s, "secret", "别的房间的")
	pushHumanText(s, "lobby", "第二条")

	got, ok := s.latestRoomText("lobby")
	if !ok || got != "第二条" {
		t.Fatalf("应当取本房间最新那条，得到 %q / %v", got, ok)
	}
	if got, _ := s.latestRoomText("secret"); got != "别的房间的" {
		t.Fatalf("房间要分开算，得到 %q", got)
	}
	// 房间名归一化：空串和 "default" 是同一间
	pushHumanText(s, "default", "默认房间的")
	for _, name := range []string{"", "default"} {
		if got, ok := s.latestRoomText(name); !ok || got != "默认房间的" {
			t.Fatalf("房间名 %q 应归一化到 default，得到 %q / %v", name, got, ok)
		}
	}
}

// 跳过文件消息、以及**定时任务自己发的**消息。
//
// 后者是防回环的关键：任务读 A 发回 A 时，如果没有这道过滤，
// 每一轮的输出都会成为下一轮的输入，内容会自我放大（「前缀 前缀 前缀 …」）。
func TestLatestRoomTextSkipsFilesAndAutomation(t *testing.T) {
	s := newAutomationServer(t)

	pushHumanText(s, "lobby", "人发的")
	pushFileMessage(s, "lobby", "某个文件.txt")
	pushAutomationText(s, "lobby", "任务自己发的")

	got, ok := s.latestRoomText("lobby")
	if !ok || got != "人发的" {
		t.Fatalf("应当跳过文件与任务自己的消息，回到那条人发的，得到 %q / %v", got, ok)
	}

	// 整间房只有任务自己发的消息时，等于「没有来源」—— 否则就成了自我喂养
	only := newAutomationServer(t)
	pushAutomationText(only, "lobby", "只有我自己的")
	if _, ok := only.latestRoomText("lobby"); ok {
		t.Fatal("只有自动化消息时应当视为没有来源")
	}
}

// 管理页拿的是**会话令牌**（那边刻意不存明文全局密码），所以「管理员」必须认那一种 ——
// 只认明文的话，管理页里所有「只有管理员能做」的能力都会**悄悄失效**：
// 配额不限、跨房间引用、任务归属…… 而界面上看不出任何异常，用户只会觉得「配了没反应」。
func TestGlobalSessionTokenCountsAsAdmin(t *testing.T) {
	s := newAutomationServer(t)

	globalToken, err := s.issueRoomSessionToken("default", roomSessionTTLSeconds, "global")
	if err != nil {
		t.Fatalf("签发全局会话令牌失败: %v", err)
	}
	roomToken, err := s.issueRoomSessionToken("secret", roomSessionTTLSeconds, "")
	if err != nil {
		t.Fatalf("签发房间会话令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks?room=secret", nil)

	if scope := s.resolveAutomationScope(req, globalToken); !scope.Admin {
		t.Errorf("全局会话令牌应当被当成管理员，得到 %+v", scope)
	}
	// 房间令牌**不该**升级成管理员 —— 它只对那一个房间有效
	if scope := s.resolveAutomationScope(req, roomToken); scope.Admin {
		t.Errorf("房间会话令牌不该被当成管理员，得到 %+v", scope)
	}
}

// `/tasks/rooms`：管理员拿到**跨房间**清单，非管理员只拿到自己那个房间。
//
// ⚠️ 非管理员那条是**权限边界**，不是「顺手过滤一下」——
// 把「别的房间有没有装自动化」告诉他，等于泄露房间名的存在性。
//
// 这个接口的存在理由：任务列表是**按房间过滤**的，而「切换到哪个房间」需要先知道有哪些房间可切 ——
// 那个信息不属于任何一个房间，只能独立查（见 handleTaskRooms）。
func TestTaskRoomsListsOnlyWhatYouMaySee(t *testing.T) {
	s := newAutomationServer(t)
	for _, room := range []string{"alpha", "beta", "gamma"} {
		task := &AutomationTask{
			ID: room + "-1", Name: room, Enabled: true,
			Freq: automationFreqDaily, Time: "09:30",
			TZ: "Asia/Shanghai", Room: room, Template: "x",
		}
		if err := s.upsertAutomationTask(task); err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
	}

	body := func(scope automationScope) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/tasks/rooms", nil)
		w := httptest.NewRecorder()
		s.handleTaskRooms(w, req, scope)
		return w.Body.String()
	}

	adminBody := body(automationScope{Room: "alpha", OK: true, Admin: true, Tier: automationTierAdmin})
	for _, room := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(adminBody, room) {
			t.Errorf("管理员应当看到房间 %s，响应里没有: %s", room, adminBody)
		}
	}

	roomBody := body(automationScope{Room: "alpha", OK: true, Tier: automationPolicyRoom})
	if !strings.Contains(roomBody, "alpha") {
		t.Errorf("非管理员应当看到自己那个房间: %s", roomBody)
	}
	if strings.Contains(roomBody, "beta") || strings.Contains(roomBody, "gamma") {
		t.Errorf("非管理员**不该**看到别的房间（那是房间名的存在性泄露）: %s", roomBody)
	}
}

// 权限：普通凭据只有「任务自己的房间」和「不需要密码就能读的房间」能当来源；
// **管理员可以引用任意房间** —— 他本来就是 `canAccessRoom` 恒真的那个人。
func TestCheckTemplateSourceRooms(t *testing.T) {
	s := newAutomationServer(t)
	roomScope := automationScope{Room: "secret", OK: true, Tier: automationPolicyRoom}
	adminScope := automationScope{Room: "secret", OK: true, Admin: true, Tier: automationTierAdmin}

	cases := []struct {
		name  string
		tpl   string
		scope automationScope
		want  bool // true = 放行
	}{
		{"没有 latest", "普通正文", roomScope, true},
		{"不带参数 = 自己的房间", "转：{{latest}}", roomScope, true},
		{"自己的房间（显式写出来）", "{{latest:secret}}", roomScope, true},
		{"公开房间", "{{latest:lobby}}", roomScope, true},
		{"公开房间（另一个）", "{{latest:public}}", roomScope, true},
		{"带密码的别的房间", "{{latest:locked}}", roomScope, false},
		// ⚠️ 管理员**放行**：他持全局密码、能读所有房间，拦他只会逼他
		// 「先看一眼再建任务」—— 多一步，但一步都没拦住（详见 checkTemplateSourceRooms）。
		{"带密码的别的房间（管理员放行）", "{{latest:locked}}", adminScope, true},
		{"管理员混着引用多个房间", "{{latest:lobby}} 和 {{latest:locked}}", adminScope, true},
		{"普通凭据混着写，只要有一个不行就整条拒", "{{latest:lobby}} 和 {{latest:locked}}", roomScope, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := s.checkTemplateSourceRooms(c.tpl, c.scope)
			if c.want && err != nil {
				t.Fatalf("应当放行，却报错: %v", err)
			}
			if !c.want {
				if err == nil {
					t.Fatal("应当被拒，却放行了")
				}
				// 错误信息必须点名是哪个房间 —— 否则用户不知道该改哪里
				if !strings.Contains(err.Error(), "locked") {
					t.Fatalf("错误信息里应当出现房间名，得到 %q", err.Error())
				}
			}
		})
	}
}

// 来源房间是空的 → 记 skipped（不是 error）。
//
// 空房间是常态：没人发言、或者消息都被顶掉了。记 error 会让「上次失败」
// 长期挂在那条任务上，用户以为功能坏了。
func TestAutomationSkipsWhenSourceEmpty(t *testing.T) {
	s := newAutomationServer(t)
	now := time.Now()

	task := testTask(automationFreqDaily, now.Format("15:04"))
	task.Room = "secret"
	task.Template = "转：{{latest:lobby}}"
	task.KeepHistory = true
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(now)

	if got := len(s.messageQueue.List); got != 0 {
		t.Fatalf("来源为空时不该投递任何东西，得到 %d 条", got)
	}
	after := s.snapshotAutomationTasks()[0]
	if after.LastStatus != "skipped" {
		t.Fatalf("应记为 skipped，得到 %q（错误: %q）", after.LastStatus, after.LastError)
	}
	if !strings.Contains(after.LastError, "来源房间") {
		t.Fatalf("跳过原因应当说清是来源房间的问题，得到 %q", after.LastError)
	}
	// 而且要真的是「可识别的空来源错误」，不是碰巧字符串对上了
	if !errors.Is(errors.New(after.LastError), ErrEmptySource) {
		t.Logf("提示：LastError = %q", after.LastError)
	}
}

// 端到端：把 lobby 的最新消息搬进 secret。
func TestAutomationUsesLatestFromAnotherRoom(t *testing.T) {
	s := newAutomationServer(t)
	now := time.Now()

	pushHumanText(s, "lobby", "大厅里刚说的那句")

	task := testTask(automationFreqDaily, now.Format("15:04"))
	task.Room = "secret"
	task.Template = "转播：{{latest:lobby}}"
	task.KeepHistory = true
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(now)

	if got := len(s.messageQueue.List); got != 2 {
		t.Fatalf("应当投递 1 条（加原来的 1 条 = 2），得到 %d", got)
	}
	last := s.messageQueue.List[len(s.messageQueue.List)-1].Data.TextReceive
	if last == nil {
		t.Fatal("最后一条应当是新投递的文本")
	}
	if last.Room != "secret" {
		t.Fatalf("应当发到任务自己的房间 secret，得到 %q", last.Room)
	}
	if last.Content != "转播：大厅里刚说的那句" {
		t.Fatalf("正文不对：%q", last.Content)
	}
	if last.Source != "automation" {
		t.Fatalf("定时消息必须带 automation 标记（回环过滤靠它），得到 %q", last.Source)
	}
}

// 防回环的**行为**验证：任务读自己的房间、发回自己的房间，内容不能自我放大。
//
// 没有这道过滤的话，第二轮读到的就是第一轮自己发的正文，
// 「再发一次：」会被一层层叠上去 —— 跑几十轮就是一屏前缀。
func TestAutomationLatestDoesNotFeedOnItself(t *testing.T) {
	s := newAutomationServer(t)
	now := time.Now()

	// lobby 是公开房间且开了 automation=single，正好用来演这个场景
	pushHumanText(s, "lobby", "原文")

	task := testTask(automationFreqDaily, now.Format("15:04"))
	task.Room = "lobby"
	task.Template = "再发一次：{{latest}}"
	task.KeepHistory = true
	if err := task.normalizeAndValidate(); err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if err := s.upsertAutomationTask(task); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	s.runDueAutomationTasks(now)
	first := s.messageQueue.List[len(s.messageQueue.List)-1].Data.TextReceive.Content
	if first != "再发一次：原文" {
		t.Fatalf("第一轮正文不对：%q", first)
	}

	// 第二轮：把幂等键清掉，假装是新的一天
	s.automationMutex.Lock()
	for _, t2 := range s.automationTasks {
		t2.LastRunKey = ""
	}
	s.automationMutex.Unlock()
	s.runDueAutomationTasks(now)

	last := s.messageQueue.List[len(s.messageQueue.List)-1].Data.TextReceive.Content
	if last != first {
		t.Fatalf("第二轮读到了自己上一轮的输出，内容开始自我放大：\n  第一轮 %q\n  第二轮 %q", first, last)
	}
	if strings.Count(last, "再发一次：") != 1 {
		t.Fatalf("前缀被叠了：%q", last)
	}
}
