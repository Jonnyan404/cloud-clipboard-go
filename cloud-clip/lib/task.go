package lib

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

/**
*** FILE: task.go
***   定时自动化任务：模型、校验、tasks.json 持久化、下次触发计算。
**/

// 为什么任务必须是**声明式**的（存动作 id + 模板 + 时刻，而不是一段脚本）：
//  1. 执行载体可以有多个（Go 进程内的 ticker、Cloudflare 的 cron、将来的常驻客户端），
//     同一份定义谁跑都得出一样的结果 —— 存脚本就做不到这一点；
//  2. 脚本是「以服务端身份执行任意代码」，而模板 + 动作 id 是可枚举、可校验、可预览的，
//     用户能在保存前看到「下次触发会发出什么」。
//
// 落盘位置：历史文件同目录下的 tasks.json（与 share-log.json 同一范式）。
// historyFilePath 为空（单元测试）时只留内存。
const (
	automationFreqOnce   = "once"
	automationFreqDaily  = "daily"
	automationFreqWeekly = "weekly"
	automationFreqCron   = "cron"

	// 幂等键的格式。**精确到分钟**：调度精度就是分钟，秒级差异只是同一刻的不同表述，
	// 用它做键会让「同一分钟内的两次 tick」被当成两次不同的触发。
	automationRunKeyLayout = "2006-01-02T15:04"

	// 单个房间的任务条数上限（管理员不受限）。这不是安全边界（真正的边界是鉴权），
	// 是防止一个房间被塞满任务、把调度器变成一个人的广播台。
	automationMaxTasksPerRoom = 20

	defaultAutomationSender = "定时任务"

	// 任务名长度上限（rune）。列表要能一眼看完，太长的名字会被截断成没用的东西。
	automationNameMaxLen = 60
)

var (
	automationClockRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)

	// once 任务的时间写法：完整 RFC3339，或「本地写法」（按任务时区解释，保存时归一化成 RFC3339）。
	automationLocalTimeLayouts = []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05"}
)

// AutomationChainStep 动作链上的一步。
//
// 两种 JSON 形态都认，**读写都兼容**：
//
//	"text.trimLines"                                        ← 无参数（绝大多数）
//	{"id":"text.replace","params":{"find":"a","with":"b"}}  ← 带参数
//
// ⚠️ **必须两种都认**：链元素带参数是后加的能力，而 tasks.json 里的存量数据全是字符串形态。
// 只认对象的话，用户升一次级就会丢掉整条链 —— 「更新把用户数据弄坏」比功能缺失严重得多。
//
// ⚠️ 写回时**无参数一律写成字符串**：和存量数据保持同形，diff 也干净。
type AutomationChainStep struct {
	ID     string            `json:"id"`
	Params map[string]string `json:"params,omitempty"`
}

func (s *AutomationChainStep) UnmarshalJSON(data []byte) error {
	var id string
	if err := json.Unmarshal(data, &id); err == nil {
		s.ID = strings.TrimSpace(id)
		s.Params = nil
		return nil
	}
	// 用别名类型，否则会递归调回本方法
	type plain AutomationChainStep
	var obj plain
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("动作链上的每一步只能是动作 id 字符串，或 {id, params} 对象")
	}
	*s = AutomationChainStep(obj)
	s.ID = strings.TrimSpace(s.ID)
	return nil
}

func (s AutomationChainStep) MarshalJSON() ([]byte, error) {
	if len(s.Params) == 0 {
		return json.Marshal(s.ID)
	}
	type plain AutomationChainStep
	return json.Marshal(plain(s))
}

// AutomationTask 一条定时任务。
//
// ⚠️ Room 是**服务端根据凭据推导出来**并写死的，不是客户端说了算的字段。
// 建好之后改房间 = 越权向别的房间投递，所以更新时**不允许改 Room**（见 upsert 的逻辑）。
type AutomationTask struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Freq    string `json:"freq"` // once | daily | weekly | cron
	Time    string `json:"time,omitempty"`

	// Cron 是 5 字段 cron 表达式（分 时 日 月 周），freq=cron 时用它。
	// 结构化字段（Time/ByWeekday）表达不了的排期走这里 —— 结构化那几种覆盖 90% 的用法，
	// 但「工作日每 30 分钟」「每月最后一个工作日」这类只有 cron 能直说。
	Cron string `json:"cron,omitempty"`

	// ByWeekday：0 = 周日 … 6 = 周六（与 time.Weekday、前端 getDay() 一致），weekly 用。
	ByWeekday []int  `json:"byWeekday,omitempty"`
	RunAt     string `json:"runAt,omitempty"` // once 用，RFC3339
	TZ        string `json:"tz,omitempty"`    // 空 = 服务端本地时区

	Room     string                `json:"room"`
	Template string                `json:"template"`
	Chain    []AutomationChainStep `json:"chain,omitempty"`

	// KeepHistory：这条任务的消息是否占房间历史额度。
	// 默认 false —— 房间历史是按房间计数的（见 msg.go 的 trimRoomHistoryLocked），
	// 一个每天 09:30 的任务十几天就能把房间里的历史全换成「今天是几号」。
	KeepHistory bool   `json:"keepHistory"`
	Sender      string `json:"sender,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`

	// LastRunKey 是**预定触发时刻**的幂等键（不是实际发送时刻）。
	// 补发（服务重启后补跑错过的）也写同一个键，所以不会重复发第二次。
	LastRunKey string `json:"lastRunKey,omitempty"`
	LastRunAt  int64  `json:"lastRunAt,omitempty"`
	LastStatus string `json:"lastStatus,omitempty"` // ok | error | skipped
	LastError  string `json:"lastError,omitempty"`
	LastOutput string `json:"lastOutput,omitempty"`

	// OwnerHash：「single 档房间」（通常是开放房间）里这条任务的钥匙的 SHA-256。
	// 开放房间的客户端**没有任何凭据**（resolveRoomAuth 返回 Required=false，
	// canAccessRoom 恒为 true），服务端无法区分谁是谁 —— 没有这个键，
	// 那条任务谁都能改、谁都能删。明文只在创建时返回一次。
	OwnerHash string `json:"ownerHash,omitempty"`
}

func (t *AutomationTask) location() (*time.Location, error) {
	tz := strings.TrimSpace(t.TZ)
	if tz == "" {
		// 兜底：正常路径上 HTTP 层已经把默认时区**写进任务**了（见 handleTaskUpsert）。
		// 这里只是为了「手改 tasks.json 把 tz 删了」这种情况别让调度器崩。
		return time.Local, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("无法识别的时区 %q（写法如 Asia/Shanghai）", tz)
	}
	return loc, nil
}

// defaultAutomationTZName 任务没写时区时用的默认值。理由见 config.go 的 defaultConfig。
const defaultAutomationTZName = "Asia/Shanghai"

// automationDefaultTZName 配置里的默认时区名；配置写坏了就退回内置默认。
//
// ⚠️ 必须校验：把这个名字直接填进任务的话，用户每次保存都会以「无法识别的时区」失败 ——
// 而错误指向任务，根因在服务器配置。降级 + 启动时警告，比让功能整个不能用好。
func (s *ClipboardServer) automationDefaultTZName() string {
	name := defaultAutomationTZName
	if s != nil && s.config != nil {
		if tz := strings.TrimSpace(s.config.Automation.DefaultTZ); tz != "" {
			name = tz
		}
	}
	if _, err := time.LoadLocation(name); err != nil {
		return defaultAutomationTZName
	}
	return name
}

func (t *AutomationTask) parseClock() (int, int, error) {
	m := automationClockRe.FindStringSubmatch(strings.TrimSpace(t.Time))
	if m == nil {
		return 0, 0, fmt.Errorf("时间写法应为 HH:MM（收到 %q）", t.Time)
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	if hour > 23 || minute > 59 {
		return 0, 0, fmt.Errorf("时间超出范围（收到 %q）", t.Time)
	}
	return hour, minute, nil
}

func (t *AutomationTask) matchesWeekday(wd time.Weekday) bool {
	if len(t.ByWeekday) == 0 {
		// weekly 却没写周几：按「每天」处理。校验会拦住这种写法，
		// 这里的宽容只是为了「配置被手改坏了也别让调度器崩」。
		return true
	}
	for _, d := range t.ByWeekday {
		if d == int(wd) {
			return true
		}
	}
	return false
}

// cronSpecOf 解析任务里的 cron 表达式。
//
// 每次调用都重新解析，不做缓存：解析是微秒级，而缓存一个 *cronSpec 就要加锁 ——
// 调度器（后台 goroutine）和 HTTP 处理器会并发读同一个 *AutomationTask，
// 为了省几微秒引入一把锁和一类数据竞争不划算。
func (t *AutomationTask) cronSpecOf() (*cronSpec, bool) {
	if strings.TrimSpace(t.Cron) == "" {
		return nil, false
	}
	spec, err := ParseCron(t.Cron)
	if err != nil {
		return nil, false
	}
	return spec, true
}

// nextRunAfter 返回严格晚于 after 的下一次触发时刻（用于展示「下次触发时间」）。
func (t *AutomationTask) nextRunAfter(after time.Time) (time.Time, bool) {
	loc, err := t.location()
	if err != nil {
		return time.Time{}, false
	}

	if t.Freq == automationFreqCron {
		spec, ok := t.cronSpecOf()
		if !ok {
			return time.Time{}, false
		}
		return spec.Next(after.In(loc))
	}

	if t.Freq == automationFreqOnce {
		at, ok := t.onceInstant(loc)
		if !ok || !at.After(after) {
			return time.Time{}, false
		}
		return at, true
	}

	hour, minute, err := t.parseClock()
	if err != nil {
		return time.Time{}, false
	}

	local := after.In(loc)
	// 7 天足够覆盖 weekly 的最坏情况（今天刚过 + 本周最后一个可执行日）
	for i := 0; i <= 7; i++ {
		candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc).AddDate(0, 0, i)
		if !candidate.After(after) {
			continue
		}
		if t.Freq == automationFreqWeekly && !t.matchesWeekday(candidate.Weekday()) {
			continue
		}
		return candidate, true
	}
	return time.Time{}, false
}

// dueOccurrence 返回**不晚于 now 的最近一次**计划触发时刻（用于调度器判到期）。
//
// 为什么取「最近的过去」而不是「下一次未来」：服务可能重启、机器可能合盖，
// 用「下一次未来」判断会让错过的那些**永远没人发现**（now 一直在往后走，
// 每次都算出下一个未来时刻，于是那一趟就静静消失了）。取最近的过去 + 幂等键，
// 才既能发现「该跑了」，又不会重复跑。
func (t *AutomationTask) dueOccurrence(now time.Time) (time.Time, bool) {
	loc, err := t.location()
	if err != nil {
		return time.Time{}, false
	}

	if t.Freq == automationFreqCron {
		spec, ok := t.cronSpecOf()
		if !ok {
			return time.Time{}, false
		}
		return spec.Prev(now.In(loc))
	}

	if t.Freq == automationFreqOnce {
		at, ok := t.onceInstant(loc)
		if !ok || at.After(now) {
			return time.Time{}, false
		}
		return at, true
	}

	hour, minute, err := t.parseClock()
	if err != nil {
		return time.Time{}, false
	}

	local := now.In(loc)
	for i := 0; i <= 7; i++ {
		candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc).AddDate(0, 0, -i)
		if candidate.After(now) {
			continue
		}
		if t.Freq == automationFreqWeekly && !t.matchesWeekday(candidate.Weekday()) {
			continue
		}
		return candidate, true
	}
	return time.Time{}, false
}

func (t *AutomationTask) onceInstant(loc *time.Location) (time.Time, bool) {
	raw := strings.TrimSpace(t.RunAt)
	if raw == "" {
		return time.Time{}, false
	}
	if at, err := time.Parse(time.RFC3339, raw); err == nil {
		return at, true
	}
	for _, layout := range automationLocalTimeLayouts {
		if at, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

func (t *AutomationTask) runKey(occurrence time.Time) string {
	loc, err := t.location()
	if err != nil {
		loc = time.Local
	}
	return occurrence.In(loc).Format(automationRunKeyLayout)
}

// NextRunAt 给管理页展示用：下一次触发时刻（Unix 秒）；没有后续触发返回 0。
func (t *AutomationTask) NextRunAt(now time.Time) int64 {
	next, ok := t.nextRunAfter(now)
	if !ok {
		return 0
	}
	return next.Unix()
}

// ── 校验 ────────────────────────────────────────────────────────────

// normalizeAndValidate 把客户端传来的任务整理成可落盘的形态，顺带挡掉写错的配置。
//
// ⚠️ 校验要发生在**保存时**，不是首次触发时。后者意味着用户写完要等一整个周期
// 才知道时区名拼错了、动作 id 服务端跑不了 —— 而那时房间里已经（或者永远没有）消息了。
func (t *AutomationTask) normalizeAndValidate() error {
	t.Name = strings.TrimSpace(t.Name)
	t.Freq = strings.ToLower(strings.TrimSpace(t.Freq))
	t.Time = strings.TrimSpace(t.Time)
	t.Cron = strings.TrimSpace(t.Cron)
	t.TZ = strings.TrimSpace(t.TZ)
	t.Template = strings.ReplaceAll(t.Template, "\r\n", "\n")
	t.Sender = sanitizeDeviceName(t.Sender)

	if t.Template == "" {
		return fmt.Errorf("正文不能为空")
	}
	if err := ValidateTemplate(t.Template); err != nil {
		return err
	}

	loc, err := t.location()
	if err != nil {
		return err
	}

	switch t.Freq {
	case automationFreqOnce:
		at, ok := t.onceInstant(loc)
		if !ok {
			return fmt.Errorf("「仅一次」需要 runAt（RFC3339 或 2006-01-02T15:04）")
		}
		// 归一化成带时区的 RFC3339，落盘之后不再有「按谁的时区解释」这个问题
		t.RunAt = at.Format(time.RFC3339)
		t.Time = ""
		t.ByWeekday = nil

	case automationFreqDaily:
		if _, _, err := t.parseClock(); err != nil {
			return err
		}
		t.RunAt = ""
		t.ByWeekday = nil

	case automationFreqWeekly:
		if _, _, err := t.parseClock(); err != nil {
			return err
		}
		if len(t.ByWeekday) == 0 {
			return fmt.Errorf("「每周」需要至少选一天")
		}
		seen := make(map[int]bool, len(t.ByWeekday))
		days := make([]int, 0, len(t.ByWeekday))
		for _, d := range t.ByWeekday {
			if d < 0 || d > 6 {
				return fmt.Errorf("星期取值应在 0（周日）到 6（周六）之间，收到 %d", d)
			}
			if seen[d] {
				continue
			}
			seen[d] = true
			days = append(days, d)
		}
		sort.Ints(days)
		t.ByWeekday = days
		t.RunAt = ""

	case automationFreqCron:
		spec, err := ParseCron(t.Cron)
		if err != nil {
			return err
		}
		// 能解析 ≠ 能触发：`0 0 30 2 *`（2 月 30 日）语法完全合法，但永远等不到。
		// 在保存时就算一次未来 —— 否则用户会在几个月后才发现任务从没跑过。
		if _, ok := spec.Next(time.Now().In(loc)); !ok {
			return fmt.Errorf("cron 表达式 %q 在未来算不出任何触发时刻（检查「日」和「月」是不是不可能的组合）", spec.expr)
		}
		// 归一化：验证通过后写的这份就是唯一写法，落盘之后不再有「同一表达式两种字符串」
		t.Cron = spec.expr
		t.Time = ""
		t.RunAt = ""
		t.ByWeekday = nil

	default:
		return fmt.Errorf("频率只能是 once / daily / weekly / cron（收到 %q）", t.Freq)
	}

	for _, step := range t.Chain {
		if step.ID == "" {
			continue
		}
		if !IsServerRenderAction(step.ID) {
			return fmt.Errorf("动作 %q 不能用于定时任务（服务端可执行的动作：%s）",
				step.ID, strings.Join(ServerRenderActionIDs(), " / "))
		}
	}

	if t.Name == "" {
		t.Name = deriveTaskName(t.Template)
	}
	if runes := []rune(t.Name); len(runes) > automationNameMaxLen {
		t.Name = string(runes[:automationNameMaxLen])
	}
	if t.Sender == "" {
		t.Sender = defaultAutomationSender
	}
	return nil
}

func deriveTaskName(tpl string) string {
	for _, line := range strings.Split(tpl, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if runes := []rune(line); len(runes) > 20 {
				return string(runes[:20]) + "…"
			}
			return line
		}
	}
	return defaultAutomationSender
}

// ── 凭据（single 档房间的钥匙）──────────────────────────────────────

func newAutomationToken() string {
	return uuid.NewString()
}

func hashAutomationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// automationTokenMatches 常数时间比较，避免用任务 token 做计时侧信道。
func automationTokenMatches(token, hash string) bool {
	if token == "" || hash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashAutomationToken(token)), []byte(hash)) == 1
}

// ── 持久化 ──────────────────────────────────────────────────────────

type automationFile struct {
	Tasks []*AutomationTask `json:"tasks"`
}

func (s *ClipboardServer) automationPath() string {
	if strings.TrimSpace(s.historyFilePath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.historyFilePath), "tasks.json")
}

// ensureAutomationLocked 懒加载一次任务文件。调用方必须已持有 automationMutex。
//
// 读失败（不存在 / 损坏）一律当作空：任务文件坏了不该拦住服务启动 ——
// 它坏了最多是「自动化不跑了」，而服务起不来是「整个剪贴板不能用」。
func (s *ClipboardServer) ensureAutomationLocked() {
	if s.automationLoaded {
		return
	}
	s.automationLoaded = true
	s.automationTasks = nil

	path := s.automationPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // 首次运行没有这个文件，属正常
	}
	var file automationFile
	if err := json.Unmarshal(data, &file); err != nil {
		if s.logger != nil {
			s.logger.Printf("定时任务 %s 解析失败，将以空任务继续: %v", path, err)
		}
		return
	}

	kept := make([]*AutomationTask, 0, len(file.Tasks))
	for _, task := range file.Tasks {
		if task == nil || strings.TrimSpace(task.ID) == "" {
			continue
		}
		task.Room = normalizeRoomName(task.Room)
		kept = append(kept, task)
	}
	s.automationTasks = kept
}

// saveAutomationLocked 整份原子写盘。调用方必须已持有 automationMutex。
//
// 用 writeFileAtomic 而不是 os.WriteFile：任务文件是启动时唯一的任务来源，
// 半截 JSON 会让用户辛苦配的任务全丢（见 utils.go 里那段论证）。
func (s *ClipboardServer) saveAutomationLocked() {
	path := s.automationPath()
	if path == "" {
		return // 单元测试场景：只留内存
	}
	data, err := json.MarshalIndent(automationFile{Tasks: s.automationTasks}, "", "  ")
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("序列化定时任务失败: %v", err)
		}
		return
	}
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		if s.logger != nil {
			s.logger.Printf("写入定时任务文件 %s 失败: %v", path, err)
		}
	}
}

// snapshotAutomationTasks 返回任务切片的浅拷贝。
//
// 为什么要拷贝：调度器要在**不持锁**的情况下执行任务（投递会写 WebSocket、
// 可能阻塞），持锁做 IO 会顶住所有 /tasks 请求。拷贝一次指针列表，
// 执行完再回到锁内写状态。
func (s *ClipboardServer) snapshotAutomationTasks() []*AutomationTask {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()
	return append([]*AutomationTask(nil), s.automationTasks...)
}

func (s *ClipboardServer) findAutomationTask(id string) (*AutomationTask, bool) {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()
	for _, task := range s.automationTasks {
		if task.ID == id {
			return task, true
		}
	}
	return nil, false
}

func (s *ClipboardServer) countAutomationTasksInRoomLocked(room string) int {
	count := 0
	normalized := normalizeRoomName(room)
	for _, task := range s.automationTasks {
		if normalizeRoomName(task.Room) == normalized {
			count++
		}
	}
	return count
}

// upsertAutomationTask 新增或整体替换一条任务（按 ID）。
//
// ⚠️ Room 由调用方（鉴权层）注入，并且**替换时保留原任务的 Room**：
// 允许改 Room 等于允许一条已授权的任务把投递目标换到别的房间。
func (s *ClipboardServer) upsertAutomationTask(task *AutomationTask) error {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()

	now := time.Now().Unix()
	for i, existing := range s.automationTasks {
		if existing.ID != task.ID {
			continue
		}
		task.Room = existing.Room
		task.CreatedAt = existing.CreatedAt
		task.OwnerHash = existing.OwnerHash
		task.UpdatedAt = now
		// 换了触发规则要把幂等键清掉：否则「把每天 09:30 改成 10:00」之后，
		// 10:00 的第一次触发会被当成 09:30 那次而跳过。
		if existing.Freq != task.Freq || existing.Time != task.Time ||
			existing.RunAt != task.RunAt || !sameIntSlice(existing.ByWeekday, task.ByWeekday) {
			task.LastRunKey = ""
		} else {
			task.LastRunKey = existing.LastRunKey
			task.LastRunAt = existing.LastRunAt
			task.LastStatus = existing.LastStatus
			task.LastError = existing.LastError
			task.LastOutput = existing.LastOutput
		}
		s.automationTasks[i] = task
		s.saveAutomationLocked()
		return nil
	}

	task.CreatedAt = now
	task.UpdatedAt = now
	s.automationTasks = append(s.automationTasks, task)
	s.saveAutomationLocked()
	return nil
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *ClipboardServer) deleteAutomationTask(id string) bool {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()

	for i, task := range s.automationTasks {
		if task.ID != id {
			continue
		}
		s.automationTasks = append(s.automationTasks[:i], s.automationTasks[i+1:]...)
		s.saveAutomationLocked()
		return true
	}
	return false
}

// recordAutomationRun 落一次执行结果。执行发生在锁外，所以状态更新要单独回锁。
func (s *ClipboardServer) recordAutomationRun(id, runKey string, status, errText, output string, disable bool) {
	s.automationMutex.Lock()
	defer s.automationMutex.Unlock()
	s.ensureAutomationLocked()

	for _, task := range s.automationTasks {
		if task.ID != id {
			continue
		}
		task.LastRunKey = runKey
		task.LastRunAt = time.Now().Unix()
		task.LastStatus = status
		task.LastError = errText
		if output != "" {
			task.LastOutput = output
		}
		if disable {
			// 「仅一次」跑完自禁用：留着 enabled=true 会让它每次 tick 都被重新评估，
			// 而 once 的 dueOccurrence 恒等于那个过去时刻 —— 幂等键挡住了重复发送，
			// 但状态会一直显示「已启用」，让人以为还有下一次。
			task.Enabled = false
		}
		task.UpdatedAt = time.Now().Unix()
		s.saveAutomationLocked()
		return
	}
}
