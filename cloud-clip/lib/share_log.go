package lib

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 分享记录（share log）：签发一条分享时落一条事实记录，分享页被真人打开时加一个计数。
//
// 为什么需要它：分享 token 是**无状态 HMAC 签名**，服务端不留任何痕迹 —— 于是
// 「我最近分享过什么」「这条链接被打开了几次」两个问题都答不上来。这里只补最小的事实：
// 签发时写一条，打开时加一。
//
// ⚠️ 谁能看到这份记录：GET /share/list 用**房间凭据**鉴权（与 POST /share 同一套规则）。
// 房间没配密码时 canAccessRoom 恒为 true，也就是**开放房间的记录列表对所有能访问服务器
// 的人可读**。这是有意的取舍：列表里的信息（哪个房间的哪条内容被分享过）在开放房间里
// 本来就公开；而且列表**不回 token**，拿到列表也取不到正文、无法重新取用别人的分享。
// 放敏感内容请给房间设密码 —— 那时列表同样需要该房间的凭据。
//
// 落盘位置：历史文件同目录下的 share-log.json。historyFilePath 为空（单元测试）时只留
// 内存，行为与分享功能本身一致 —— 统计是附加功能，任何写盘失败都不该影响签发。
const (
	// 记录条数上限。分享是高频动作，这份日志只回答「最近分享过什么」，不是审计系统；
	// 超出就丢最旧的（已过期的优先丢）。
	maxShareLogRecords    = 500
	defaultShareListLimit = 50
	maxShareListLimit     = 200
	// 同一访客对同一条分享的重复上报窗口（秒）：防「拿着链接刷数字」。
	shareVisitDedupeWindow = 600
	maxShareVisitDedupe    = 2048
)

// shareRecord 一条分享的事实记录。字段以「能画出记录列表」为界，不做审计用途。
type shareRecord struct {
	JTI       string `json:"jti"`
	Type      string `json:"type"` // content | file
	ID        string `json:"id"`   // content id 或 file uuid
	Room      string `json:"room"`
	Kind      string `json:"kind,omitempty"` // 这条分享最终指向什么：text | file
	Name      string `json:"name,omitempty"` // 文件名，或文本首行摘要
	Size      int64  `json:"size,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	Exp       int64  `json:"exp"`
	MaxUses   int    `json:"maxUses"`
	Password  bool   `json:"password"`
	Visits    int    `json:"visits"` // 分享页被打开的次数
	Scans     int    `json:"scans"`  // 其中来自二维码的次数（链接带 ?q=1）
}

type shareLogFile struct {
	Records []*shareRecord `json:"records"`
}

func (s *ClipboardServer) shareLogPath() string {
	if strings.TrimSpace(s.historyFilePath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.historyFilePath), "share-log.json")
}

// ensureShareLogLocked 懒加载一次记录文件。调用方必须已持有 shareLogMutex。
// 读失败（不存在 / 损坏）一律当作空日志：统计丢了不是错误，不该拦住服务启动。
func (s *ClipboardServer) ensureShareLogLocked() {
	if s.shareLog != nil {
		return
	}
	s.shareLog = make(map[string]*shareRecord)

	path := s.shareLogPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // 首次运行时没有这个文件，属正常
	}
	var file shareLogFile
	if err := json.Unmarshal(data, &file); err != nil {
		if s.logger != nil {
			s.logger.Printf("分享记录 %s 解析失败，将以空记录继续: %v", path, err)
		}
		return
	}
	for _, rec := range file.Records {
		if rec == nil || rec.JTI == "" {
			continue
		}
		s.shareLog[rec.JTI] = rec
	}
}

// saveShareLogLocked 整份写盘。分享不是高频动作，全量写比增量简单且够快。
// 调用方必须已持有 shareLogMutex。写失败只记日志 —— 统计不该让分享签发失败。
func (s *ClipboardServer) saveShareLogLocked() {
	path := s.shareLogPath()
	if path == "" {
		return
	}

	records := make([]*shareRecord, 0, len(s.shareLog))
	for _, rec := range s.shareLog {
		if rec != nil {
			records = append(records, rec)
		}
	}

	now := time.Now().Unix()
	sort.Slice(records, func(i, j int) bool {
		expiredI := records[i].Exp > 0 && records[i].Exp <= now
		expiredJ := records[j].Exp > 0 && records[j].Exp <= now
		if expiredI != expiredJ {
			return !expiredI // 未过期的排在前面（优先保留）
		}
		return records[i].CreatedAt > records[j].CreatedAt
	})

	if len(records) > maxShareLogRecords {
		records = records[:maxShareLogRecords]
		// 内存同步裁掉，否则 map 会随运行时间无限增长
		kept := make(map[string]*shareRecord, len(records))
		for _, rec := range records {
			kept[rec.JTI] = rec
		}
		s.shareLog = kept
	}

	data, err := json.MarshalIndent(shareLogFile{Records: records}, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		if s.logger != nil {
			s.logger.Printf("写入分享记录 %s 时出错: %v", path, err)
		}
	}
}

// recordShare 记一条新分享。必须在 token 签发成功后调用（记录里的 jti 来自 claims）。
func (s *ClipboardServer) recordShare(rec *shareRecord) {
	if rec == nil || rec.JTI == "" {
		return
	}
	if rec.CreatedAt == 0 {
		rec.CreatedAt = time.Now().Unix()
	}

	s.shareLogMutex.Lock()
	defer s.shareLogMutex.Unlock()
	s.ensureShareLogLocked()
	s.shareLog[rec.JTI] = rec
	s.saveShareLogLocked()
}

// lookupShareRecord 取一条记录的副本（调用方不该直接改内部状态）。
func (s *ClipboardServer) lookupShareRecord(jti string) (shareRecord, bool) {
	if strings.TrimSpace(jti) == "" {
		return shareRecord{}, false
	}
	s.shareLogMutex.Lock()
	defer s.shareLogMutex.Unlock()
	s.ensureShareLogLocked()
	rec, ok := s.shareLog[jti]
	if !ok || rec == nil {
		return shareRecord{}, false
	}
	return *rec, true
}

// recordShareForTarget 把一条内容型分享记下来（POST /share 的 type=content 分支）。
// 名称：文件取文件名，文本取首行摘要 —— 记录列表里要能一眼认出「分享的是哪条」。
func (s *ClipboardServer) recordShareForTarget(claims *shareClaims, target shareTarget) {
	if claims == nil {
		return
	}
	rec := &shareRecord{
		JTI:       claims.JTI,
		Type:      claims.Type,
		ID:        claims.ID,
		Room:      claims.Room,
		Kind:      target.Kind,
		CreatedAt: time.Now().Unix(),
		Exp:       claims.Exp,
		MaxUses:   claims.MaxUses,
		Password:  claims.PwdHash != "",
	}
	if target.Kind == "file" {
		rec.Name, rec.Size = s.shareFileMeta(target.FileUUID, target.FileName, target.FileSize)
	} else {
		rec.Name = firstSummaryLine(target.Text, 60)
	}
	s.recordShare(rec)
}

// recordShareForFile 把一条文件型分享记下来（POST /share 的 type=file 分支）。
func (s *ClipboardServer) recordShareForFile(claims *shareClaims, fileUUID, name string, size int64) {
	if claims == nil {
		return
	}
	fileName, fileSize := s.shareFileMeta(fileUUID, name, size)
	s.recordShare(&shareRecord{
		JTI:       claims.JTI,
		Type:      claims.Type,
		ID:        claims.ID,
		Room:      claims.Room,
		Kind:      "file",
		Name:      fileName,
		Size:      fileSize,
		CreatedAt: time.Now().Unix(),
		Exp:       claims.Exp,
		MaxUses:   claims.MaxUses,
		Password:  claims.PwdHash != "",
	})
}

// shareFileMeta 以上传文件表为准补全文件名 / 大小（消息条目里的文件字段可能是空的）。
func (s *ClipboardServer) shareFileMeta(fileUUID, fallbackName string, fallbackSize int64) (string, int64) {
	name, size := fallbackName, fallbackSize
	if strings.TrimSpace(fileUUID) == "" {
		return name, size
	}
	s.runMutex.Lock()
	info, exists := s.uploadFileMap[fileUUID]
	s.runMutex.Unlock()
	if !exists {
		return name, size
	}
	if strings.TrimSpace(info.Name) != "" {
		name = info.Name
	}
	if info.Size > 0 {
		size = info.Size
	}
	return name, size
}

// markShareVisit 记一次「有人打开了这条分享」，返回累计值。
//
// viaQR：二维码链接带 ?q=1 —— 扫码和点链接打开的是同一个页面，这是唯一能把两者
// 分开的信号（前端从 URL 上带过来）。
//
// visitorKey：去重键（访客 IP）。同一条分享在同一窗口内的重复上报只算一次，
// 否则拿着链接连点就能把数字刷上去。
func (s *ClipboardServer) markShareVisit(jti string, viaQR bool, visitorKey string) (visits, scans int, tracked bool) {
	if strings.TrimSpace(jti) == "" {
		return 0, 0, false
	}

	s.shareLogMutex.Lock()
	defer s.shareLogMutex.Unlock()
	s.ensureShareLogLocked()

	rec, ok := s.shareLog[jti]
	if !ok || rec == nil {
		return 0, 0, false
	}

	now := time.Now().Unix()
	if s.shareVisitDedupe == nil {
		s.shareVisitDedupe = make(map[string]int64)
	}
	dedupeKey := jti + "|" + strings.TrimSpace(visitorKey)
	if last, seen := s.shareVisitDedupe[dedupeKey]; seen && now-last < shareVisitDedupeWindow {
		return rec.Visits, rec.Scans, false
	}
	if len(s.shareVisitDedupe) > maxShareVisitDedupe {
		for k, ts := range s.shareVisitDedupe {
			if now-ts >= shareVisitDedupeWindow {
				delete(s.shareVisitDedupe, k)
			}
		}
	}
	s.shareVisitDedupe[dedupeKey] = now

	rec.Visits++
	if viaQR {
		rec.Scans++
	}
	s.saveShareLogLocked()
	return rec.Visits, rec.Scans, true
}

// normalizeShareListLimit 夹一次 limit：默认 50，上限 200。
// 参数和响应里报出去的值必须是同一个，否则调用方看到的就是自己没拿到的数。
func normalizeShareListLimit(limit int) int {
	if limit <= 0 {
		return defaultShareListLimit
	}
	if limit > maxShareListLimit {
		return maxShareListLimit
	}
	return limit
}

// shareRecordsForRoom 返回某个房间的记录（新→旧）与该房间的记录总数。
func (s *ClipboardServer) shareRecordsForRoom(room string, limit int) ([]shareRecord, int) {
	room = normalizeRoomName(room)
	limit = normalizeShareListLimit(limit)

	s.shareLogMutex.Lock()
	s.ensureShareLogLocked()
	matched := make([]shareRecord, 0, len(s.shareLog))
	for _, rec := range s.shareLog {
		if rec == nil || normalizeRoomName(rec.Room) != room {
			continue
		}
		matched = append(matched, *rec)
	}
	s.shareLogMutex.Unlock()

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt > matched[j].CreatedAt
	})
	total := len(matched)
	if total > limit {
		matched = matched[:limit]
	}
	return matched, total
}

// shareUsageSnapshot 读一次「已用次数」。只读，不建 map、不写（同 handleShareInfo）。
func (s *ClipboardServer) shareUsageSnapshot(jti string) int {
	if strings.TrimSpace(jti) == "" {
		return 0
	}
	s.shareUsageMutex.Lock()
	defer s.shareUsageMutex.Unlock()
	if entry := s.shareTokenUsage[jti]; entry != nil {
		return entry.Used
	}
	return 0
}

// shareListEntry 是记录列表对外的那一版。刻意**不含 token**：token 是 bearer 凭据，
// 列表只是给创建者看统计的，不该变成「谁都能把别人的分享链接再抄一遍」的入口。
type shareListEntry struct {
	JTI       string `json:"jti"`
	Type      string `json:"type"`
	Kind      string `json:"kind,omitempty"`
	ID        string `json:"id"`
	Room      string `json:"room"`
	Name      string `json:"name,omitempty"`
	Size      int64  `json:"size,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	MaxUses   int    `json:"maxUses"`
	Used      int    `json:"used"`
	Visits    int    `json:"visits"`
	Scans     int    `json:"scans"`
	Password  bool   `json:"password"`
	Expired   bool   `json:"expired"`
}

// GET /share/list?room=<room>&limit=<n> —— 某个房间最近的分享记录。
//
// 鉴权刻意与「在该房间签发分享」完全一致（canAccessRoom），于是规则只有一条，
// 不会出现「能建分享但看不到自己建的分享」。开放房间 = 公开，理由见文件头部。
func (s *ClipboardServer) handleShareList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed", "仅允许 GET 请求")
		return
	}

	room := normalizeRoomName(r.URL.Query().Get("room"))
	if !s.canAccessRoom(room, extractAuthToken(r)) {
		writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
		return
	}

	limit := defaultShareListLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	limit = normalizeShareListLimit(limit)

	records, total := s.shareRecordsForRoom(room, limit)
	now := time.Now().Unix()
	entries := make([]shareListEntry, 0, len(records))
	for _, rec := range records {
		entries = append(entries, shareListEntry{
			JTI:       rec.JTI,
			Type:      rec.Type,
			Kind:      rec.Kind,
			ID:        rec.ID,
			Room:      rec.Room,
			Name:      rec.Name,
			Size:      rec.Size,
			CreatedAt: rec.CreatedAt,
			ExpiresAt: rec.Exp,
			MaxUses:   rec.MaxUses,
			Used:      s.shareUsageSnapshot(rec.JTI),
			Visits:    rec.Visits,
			Scans:     rec.Scans,
			Password:  rec.Password,
			Expired:   rec.Exp > 0 && rec.Exp <= now,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"room":    room,
		"total":   total,
		"limit":   limit,
		"records": entries,
	})
}

// POST /share/visit —— 分享页被**真人**打开时上报一次。
//
// 为什么由前端上报、而不是在 /s/<token> 落地页里直接计数：落地页是给社交平台的
// **抓取程序**看的（贴一次链接，微信/Telegram/Slack 都会去抓，且平台侧会按自己的
// 节奏重抓）。在那里计数会把「机器抓取」算成「有人打开」。只有执行了 JS 的分享页
// 能证明是真人打开了。
//
// 鉴权：只需要 token 本身（未认证接口）—— 你拿着链接才能上报，接口也只回**这一条**
// 分享的计数。同一访客十分钟内重复上报会被去重（见 markShareVisit）。
func (s *ClipboardServer) handleShareVisit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	var req struct {
		Token string `json:"token"`
		QR    bool   `json:"qr"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "Invalid request body", "无效的请求体")
		return
	}
	// token 也允许走 query：落地页可以顺手把访问者带过来（保留 POST body 为主）
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = extractShareToken(r)
	}
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing_token", "Missing token", "缺少 token")
		return
	}

	claims, ok := s.parseShareToken(token)
	if !ok {
		writeError(w, http.StatusUnauthorized, "share_token_invalid", "Share link is invalid or expired", "分享链接无效或已过期")
		return
	}
	// 已过期的链接不计数：访客看到的是错误页，不该算作「打开了一次分享」
	if claims.Exp > 0 && claims.Exp <= time.Now().Unix() {
		writeError(w, http.StatusUnauthorized, "share_token_invalid", "Share link is invalid or expired", "分享链接无效或已过期")
		return
	}

	visits, scans, tracked := s.markShareVisit(claims.JTI, req.QR || r.URL.Query().Get("q") == "1", get_remote_ip(r))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"tracked": tracked,
		"visits":  visits,
		"scans":   scans,
	})
}
