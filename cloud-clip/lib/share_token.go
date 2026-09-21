package lib

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultShareTTLSeconds = 15 * 60
	maxShareTTLSeconds     = 24 * 60 * 60
	minShareTTLSeconds     = 60
	maxShareMaxUses        = 1000
	shareTokenQueryKey     = "t"

	// 分享密码走的请求头。**不要放 URL** —— query 会进浏览器历史和服务器访问日志。
	// 分享页是前端那一条路由，发请求时带这个头即可。
	sharePasswordHeader = "X-Share-Password"
	// 存进 token 的是 HMAC(服务端签名密钥, 密码) 的十六进制前 16 位。
	// 用密钥而不是裸 SHA256：token 在 URL 里，裸哈希能被离线爆破；带密钥的算不出来。
	// 存 token 里而不是内存 map：usage map 是进程内的，重启就没了，密码不能跟着丢。
	sharePasswordHashLen = 16
)

type shareClaims struct {
	Type    string `json:"typ"`          // content | file | room_session
	ID      string `json:"id"`           // content id or file uuid or room
	Room    string `json:"room"`         // 绑定的房间（默认房间为空串）
	Scope   string `json:"sc,omitempty"` // room_session 的 scope: ""=房间专属, "global"=全局所有房间
	Exp     int64  `json:"exp"`
	JTI     string `json:"jti,omitempty"` // token id when usage-limited
	MaxUses int    `json:"mu,omitempty"`  // 0 = unlimited
	// 非空表示这条分享需要密码；值是 HMAC(签名密钥, 密码) 的前若干位
	PwdHash string `json:"p,omitempty"`
}

type shareRequest struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	UUID    string `json:"uuid"`
	TTL     int    `json:"ttl"`
	MaxUses int    `json:"maxUses"`
	// 可选：给这条分享加密码。空串 = 不需要密码
	Password string `json:"password"`
}

type shareUsageEntry struct {
	Used    int
	MaxUses int
	Exp     int64
}

func (s *ClipboardServer) initShareSigningKey() {
	if len(s.shareSigningKey) > 0 {
		return
	}

	// 优先从认证配置派生稳定密钥，避免进程重启后未过期的分享链接全部失效
	h := sha256.New()
	_, _ = h.Write([]byte("cloud-clipboard-share-v1"))
	if s.config != nil {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(normalizeAuthValue(s.config.Server.Auth)))
		if len(s.config.Server.RoomAuth) > 0 {
			rooms := make([]string, 0, len(s.config.Server.RoomAuth))
			for room := range s.config.Server.RoomAuth {
				rooms = append(rooms, room)
			}
			sort.Strings(rooms)
			for _, room := range rooms {
				_, _ = h.Write([]byte{0})
				_, _ = h.Write([]byte(room))
				_, _ = h.Write([]byte{0})
				_, _ = h.Write([]byte(s.config.Server.RoomAuth[room].Password))
			}
		}
	}

	derived := h.Sum(nil)
	// 若完全没有认证材料，再混入随机盐，避免固定空密钥
	if s.config == nil || (normalizeAuthValue(s.config.Server.Auth) == "" && len(s.config.Server.RoomAuth) == 0) {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err == nil {
			h2 := sha256.New()
			_, _ = h2.Write(derived)
			_, _ = h2.Write(salt)
			derived = h2.Sum(nil)
		} else {
			h2 := sha256.New()
			_, _ = h2.Write(derived)
			_, _ = h2.Write([]byte(fmt.Sprintf("%d|%d", s.deviceHashSeed, time.Now().UnixNano())))
			derived = h2.Sum(nil)
		}
	}
	s.shareSigningKey = derived
}

func (s *ClipboardServer) ensureShareUsageMap() {
	if s.shareTokenUsage == nil {
		s.shareTokenUsage = make(map[string]*shareUsageEntry)
	}
}

func extractShareToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get(shareTokenQueryKey))
}

func normalizeShareTTL(ttl int) int {
	if ttl <= 0 {
		return defaultShareTTLSeconds
	}
	if ttl < minShareTTLSeconds {
		return minShareTTLSeconds
	}
	if ttl > maxShareTTLSeconds {
		return maxShareTTLSeconds
	}
	return ttl
}

// normalizeShareMaxUses: 0 = unlimited; clamp positive values to [1, maxShareMaxUses]
func normalizeShareMaxUses(maxUses int) int {
	if maxUses <= 0 {
		return 0
	}
	if maxUses > maxShareMaxUses {
		return maxShareMaxUses
	}
	return maxUses
}

func newShareJTI() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// 把分享密码算成可存进 token 的短串。
// 空密码返回空串（表示这条分享不需要密码）。
func (s *ClipboardServer) sharePasswordHash(password string) string {
	password = strings.TrimSpace(password)
	if password == "" {
		return ""
	}
	s.initShareSigningKey()
	mac := hmac.New(sha256.New, s.shareSigningKey)
	_, _ = mac.Write([]byte("share-password:" + password))
	return hex.EncodeToString(mac.Sum(nil))[:sharePasswordHashLen]
}

// 从请求里取分享密码（请求头）。
func extractSharePassword(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(sharePasswordHeader))
}

func (s *ClipboardServer) signShareClaims(claims shareClaims) (string, error) {
	s.initShareSigningKey()

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.shareSigningKey)
	_, _ = mac.Write([]byte(payloadPart))
	sigPart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadPart + "." + sigPart, nil
}

func (s *ClipboardServer) parseShareToken(token string) (*shareClaims, bool) {
	s.initShareSigningKey()

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, false
	}

	mac := hmac.New(sha256.New, s.shareSigningKey)
	_, _ = mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return nil, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}

	var claims shareClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}

	claims.Type = strings.TrimSpace(claims.Type)
	claims.ID = strings.TrimSpace(claims.ID)
	claims.Room = normalizeRoomName(claims.Room)
	claims.JTI = strings.TrimSpace(claims.JTI)
	if claims.Type == "" || claims.ID == "" || claims.Exp <= 0 {
		return nil, false
	}
	if time.Now().Unix() > claims.Exp {
		return nil, false
	}
	if claims.MaxUses < 0 {
		claims.MaxUses = 0
	}
	if claims.MaxUses > 0 && claims.JTI == "" {
		return nil, false
	}

	return &claims, true
}

// issueRoomSessionToken 签发房间会话令牌。
// scope 为 "global" 时签发全局会话令牌（对所有房间有效），否则按 room 绑定。
func (s *ClipboardServer) issueRoomSessionToken(room string, ttlSeconds int, scope string) (string, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = 60 * 60
	}
	if ttlSeconds > 24*60*60 {
		ttlSeconds = 24 * 60 * 60
	}

	claims := shareClaims{
		Type:  "room_session",
		ID:    normalizeRoomName(room),
		Room:  normalizeRoomName(room),
		Scope: scope,
		Exp:   time.Now().Unix() + int64(ttlSeconds),
	}
	return s.signShareClaims(claims)
}

// parseRoomSessionToken 解析并校验会话令牌（不含房间匹配），返回 claims。
func (s *ClipboardServer) parseRoomSessionToken(token string) (*shareClaims, bool) {
	if strings.TrimSpace(token) == "" {
		return nil, false
	}
	claims, ok := s.parseShareToken(token)
	if !ok {
		return nil, false
	}
	if claims.Type != "room_session" {
		return nil, false
	}
	return claims, true
}

func (s *ClipboardServer) validateRoomSessionToken(room, token string) bool {
	claims, ok := s.parseRoomSessionToken(token)
	if !ok {
		return false
	}
	// 全局会话令牌对所有房间有效
	if claims.Scope == "global" {
		return true
	}
	if normalizeRoomName(room) != claims.Room {
		return false
	}
	if normalizeRoomName(claims.ID) != normalizeRoomName(room) {
		return false
	}
	return true
}

// shouldConsumeShareUse decides whether this HTTP request should count against maxUses.
// Range continuations (bytes starting > 0) and non-GET methods do not consume.
func shouldConsumeShareUse(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	// HEAD is used for probes; do not burn uses.
	if r.Method == http.MethodHead {
		return false
	}
	rangeHeader := strings.TrimSpace(r.Header.Get("Range"))
	if rangeHeader == "" {
		return true
	}
	// Only count the first segment of a download / media open.
	// e.g. "bytes=0-1023" or "bytes=0-" consumes; "bytes=1024-" does not.
	lower := strings.ToLower(rangeHeader)
	if !strings.HasPrefix(lower, "bytes=") {
		return true
	}
	spec := strings.TrimSpace(rangeHeader[len("bytes="):])
	if spec == "" {
		return true
	}
	// multi-range: treat as consume to be safe
	if strings.Contains(spec, ",") {
		return true
	}
	startPart := strings.SplitN(spec, "-", 2)[0]
	startPart = strings.TrimSpace(startPart)
	if startPart == "" || startPart == "0" {
		return true
	}
	return false
}

func (s *ClipboardServer) consumeShareUse(claims *shareClaims) bool {
	if claims == nil || claims.MaxUses <= 0 {
		return true
	}
	if claims.JTI == "" {
		return false
	}

	now := time.Now().Unix()
	s.shareUsageMutex.Lock()
	defer s.shareUsageMutex.Unlock()
	s.ensureShareUsageMap()

	// opportunistic cleanup of a few expired entries
	if len(s.shareTokenUsage) > 256 {
		for k, v := range s.shareTokenUsage {
			if v == nil || v.Exp <= now {
				delete(s.shareTokenUsage, k)
			}
		}
	}

	entry, ok := s.shareTokenUsage[claims.JTI]
	if !ok || entry == nil {
		entry = &shareUsageEntry{
			Used:    0,
			MaxUses: claims.MaxUses,
			Exp:     claims.Exp,
		}
		s.shareTokenUsage[claims.JTI] = entry
	} else {
		if entry.Exp < claims.Exp {
			entry.Exp = claims.Exp
		}
		if entry.MaxUses < claims.MaxUses {
			entry.MaxUses = claims.MaxUses
		}
	}

	if entry.Exp > 0 && entry.Exp <= now {
		delete(s.shareTokenUsage, claims.JTI)
		return false
	}
	if entry.Used >= entry.MaxUses {
		return false
	}
	entry.Used++
	return true
}

func (s *ClipboardServer) validateShareToken(r *http.Request, expectedType, expectedID, expectedRoom string) bool {
	claims, ok := s.parseShareToken(extractShareToken(r))
	if !ok {
		return false
	}

	if claims.Type != expectedType {
		return false
	}
	if claims.ID != strings.TrimSpace(expectedID) {
		return false
	}
	if normalizeRoomName(expectedRoom) != claims.Room {
		return false
	}

	// 需要密码的分享：请求头里必须带对。用 hmac.Equal 做常数时间比较，
	// 别用 == —— 字符串比较会在第一个不同的字节就返回，能按时间差逐字节猜。
	if claims.PwdHash != "" {
		if !hmac.Equal([]byte(s.sharePasswordHash(extractSharePassword(r))), []byte(claims.PwdHash)) {
			return false
		}
	}

	if claims.MaxUses > 0 && shouldConsumeShareUse(r) {
		if !s.consumeShareUse(claims) {
			return false
		}
	}
	return true
}

func (s *ClipboardServer) canAccessContent(r *http.Request, room string, contentID int) bool {
	token := extractAuthToken(r)
	if s.canAccessRoom(room, token) {
		return true
	}
	return s.validateShareToken(r, "content", strconv.Itoa(contentID), room)
}

func (s *ClipboardServer) canAccessFile(r *http.Request, room string, fileUUID string) bool {
	token := extractAuthToken(r)
	if s.canAccessRoom(room, token) {
		return true
	}
	return s.validateShareToken(r, "file", fileUUID, room)
}

func (s *ClipboardServer) buildAbsoluteURL(r *http.Request, path string, query url.Values) string {
	scheme := getScheme(r)
	prefix := strings.TrimRight(s.config.Server.Prefix, "/")
	normalizedPath := "/" + strings.TrimLeft(path, "/")
	if prefix != "" {
		normalizedPath = prefix + normalizedPath
	}

	u := url.URL{
		Scheme: scheme,
		Host:   r.Host,
		Path:   normalizedPath,
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func (s *ClipboardServer) findContentForShare(contentID int, preferredRoom string, hasPreferredRoom bool) (room string, msgType string, fileUUID string, ok bool) {
	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	for _, msg := range s.messageQueue.List {
		if msg.Data.ID() != contentID {
			continue
		}
		messageRoom := normalizeRoomName(msg.Data.Room())
		if hasPreferredRoom && messageRoom != preferredRoom {
			continue
		}
		switch msg.Data.Type() {
		case "text":
			return messageRoom, "text", "", true
		case "file":
			uuid := ""
			if msg.Data.FileReceive != nil {
				uuid = msg.Data.FileReceive.Cache
			}
			return messageRoom, "file", uuid, true
		default:
			return messageRoom, msg.Data.Type(), "", true
		}
	}
	return "", "", "", false
}

func (s *ClipboardServer) issueShareToken(shareType, id, room string, ttl, maxUses int, password string) (token string, expiresAt int64, err error) {
	expiresAt = time.Now().Unix() + int64(ttl)
	claims := shareClaims{
		Type:    shareType,
		ID:      id,
		Room:    room,
		Exp:     expiresAt,
		MaxUses: maxUses,
		PwdHash: s.sharePasswordHash(password),
	}
	if maxUses > 0 {
		jti, jerr := newShareJTI()
		if jerr != nil {
			return "", 0, jerr
		}
		claims.JTI = jti
	}
	token, err = s.signShareClaims(claims)
	if err != nil {
		return "", 0, err
	}
	return token, expiresAt, nil
}

// buildSharePageURL 拼前端分享页地址。
//
// 为什么不用 buildAbsoluteURL：那个函数把路径交给 url.URL 处理，`#` 会被转义成 %23，
// 而分享页走的是 hash 路由，必须保留字面量 `#`。
func (s *ClipboardServer) buildSharePageURL(r *http.Request, token string) string {
	scheme := getScheme(r)
	prefix := strings.TrimRight(s.config.Server.Prefix, "/")
	return fmt.Sprintf("%s://%s%s/#/s?t=%s", scheme, r.Host, prefix, url.QueryEscape(token))
}

// handleShareInfo 处理 GET /share?t=...：分享页在取正文之前先问一次这里。
//
// 为什么不直接让分享页去调 /content 或 /file：
//   - 文件场景必须先知道**文件名**才能拼出 /file/<uuid>/<name>，而 token 里没有这个名字；
//   - 分享页要在取正文之前就把「类型 / 大小 / 剩余有效期 / 剩余次数」渲染出来；
//   - 「token 无效」「已过期」「需要密码」三种情况要能分开报，取正文的接口分不出来。
//
// **不消耗使用次数**：打开页面本身不该烧掉一次，真正取正文时才消耗（见 validateShareToken）。
func (s *ClipboardServer) handleShareInfo(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.parseShareToken(extractShareToken(r))
	if !ok {
		writeError(w, http.StatusUnauthorized, "share_token_invalid", "Share link is invalid or expired", "分享链接无效或已过期")
		return
	}

	if claims.PwdHash != "" {
		if !hmac.Equal([]byte(s.sharePasswordHash(extractSharePassword(r))), []byte(claims.PwdHash)) {
			writeError(w, http.StatusUnauthorized, "share_password_required", "Share password required", "需要分享密码")
			return
		}
	}

	response := map[string]interface{}{
		"type":          claims.Type,
		"room":          claims.Room,
		"expiresAt":     claims.Exp,
		"maxUses":       claims.MaxUses,
		"needsPassword": claims.PwdHash != "",
	}
	if claims.MaxUses > 0 && claims.JTI != "" {
		// 只读一次，不建 map、不写
		s.shareUsageMutex.Lock()
		if entry := s.shareTokenUsage[claims.JTI]; entry != nil {
			response["used"] = entry.Used
		}
		s.shareUsageMutex.Unlock()
	}

	switch claims.Type {
	case "content":
		contentID, err := strconv.Atoi(claims.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "内容未找到")
			return
		}
		room, msgType, fileUUID, found := s.findContentForShare(contentID, claims.Room, true)
		if !found {
			writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "内容未找到")
			return
		}
		response["id"] = claims.ID
		response["room"] = room
		response["kind"] = msgType
		if msgType == "file" && !s.fillShareFileInfo(w, response, fileUUID) {
			return
		}
	case "file":
		response["kind"] = "file"
		if !s.fillShareFileInfo(w, response, claims.ID) {
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unsupported_type", "Unsupported share type", "不支持的分享类型")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// fillShareFileInfo 往响应里补文件元信息，顺带做存在性 / 过期检查。
// 返回 false 表示已经写过错误响应，调用方直接 return 即可。
func (s *ClipboardServer) fillShareFileInfo(w http.ResponseWriter, response map[string]interface{}, fileUUID string) bool {
	s.runMutex.Lock()
	fileInfo, exists := s.uploadFileMap[fileUUID]
	s.runMutex.Unlock()
	if !exists {
		writeError(w, http.StatusNotFound, "file_not_found", "File not found or expired", "文件未找到或已过期")
		return false
	}
	if fileInfo.ExpireTime > 0 && fileInfo.ExpireTime < time.Now().Unix() {
		writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
		return false
	}
	name := fileInfo.Name
	if name == "" {
		name = "file"
	}
	response["uuid"] = fileUUID
	response["name"] = name
	response["size"] = fileInfo.Size
	return true
}

func (s *ClipboardServer) handle_share(w http.ResponseWriter, r *http.Request) {
	// 与 authMiddleware 保持一致的 CORS 行为，便于前后端分离调用
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Room-Auth-Tokens, "+sharePasswordHeader)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// GET /share?t=... 是给前端分享页用的「先看一眼」，POST 才是签发。同一条路径两种方法。
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		s.handleShareInfo(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	var req shareRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "Invalid request body", "无效的请求体")
		return
	}

	shareType := strings.ToLower(strings.TrimSpace(req.Type))
	if shareType == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "Missing type", "缺少 type")
		return
	}

	requestedRoom := normalizeRoomName(r.URL.Query().Get("room"))
	_, hasRequestedRoom := r.URL.Query()["room"]
	ttl := normalizeShareTTL(req.TTL)
	maxUses := normalizeShareMaxUses(req.MaxUses)
	authToken := extractAuthToken(r)

	switch shareType {
	case "content":
		idStr := strings.TrimSpace(req.ID)
		if idStr == "" {
			writeError(w, http.StatusBadRequest, "missing_id", "Missing id", "缺少 id")
			return
		}
		contentID, err := strconv.Atoi(idStr)
		if err != nil || contentID < 0 {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id", "无效的 id")
			return
		}

		room, _, _, found := s.findContentForShare(contentID, requestedRoom, hasRequestedRoom)
		if !found {
			writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "内容未找到")
			return
		}
		if !s.canAccessRoom(room, authToken) {
			writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
			return
		}

		// 一律签发 token：TTL / 次数限制 / 密码都由它承载，房间是否需要鉴权不再影响这件事。
		// 曾经只在 requirement.Required 时才发 —— 结果是开放房间的分享链接永不过期、不限次数，
		// 弹窗里让用户设的值被静默丢弃，而响应里却照样回 ttl/maxUses，会骗到调用方。
		token, expiresAt, err := s.issueShareToken("content", idStr, room, ttl, maxUses, req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share_token_failed", "Failed to generate share token", "生成分享令牌失败")
			return
		}
		// rawUrl 是「直接拿字节」的地址（带同一个 token），给分享页里的下载按钮和
		// 前端自己的下载链路用 —— 分享页地址是 hash 路由，取不了正文。
		rawQuery := url.Values{}
		if room != "default" {
			rawQuery.Set("room", room)
		}
		rawQuery.Set(shareTokenQueryKey, token)
		response := map[string]interface{}{
			"type":      "content",
			"id":        idStr,
			"room":      room,
			"ttl":       ttl,
			"expiresAt": expiresAt,
			"maxUses":   maxUses,
			"token":     token,
			"url":       s.buildSharePageURL(r, token),
			"rawUrl":    s.buildAbsoluteURL(r, fmt.Sprintf("/content/%s", idStr), rawQuery),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return

	case "file":
		fileUUID := strings.TrimSpace(req.UUID)
		if fileUUID == "" {
			fileUUID = strings.TrimSpace(req.ID)
		}
		if fileUUID == "" {
			writeError(w, http.StatusBadRequest, "missing_uuid", "Missing uuid", "缺少 uuid")
			return
		}

		s.runMutex.Lock()
		fileInfo, exists := s.uploadFileMap[fileUUID]
		s.runMutex.Unlock()
		if !exists {
			writeError(w, http.StatusNotFound, "file_not_found", "File not found or expired", "文件未找到或已过期")
			return
		}
		if fileInfo.ExpireTime > 0 && fileInfo.ExpireTime < time.Now().Unix() {
			writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
			return
		}

		room := normalizeRoomName(fileInfo.Room)
		if hasRequestedRoom && room != requestedRoom {
			writeError(w, http.StatusNotFound, "file_not_found", "File not found or expired", "文件未找到或已过期")
			return
		}
		if !s.canAccessRoom(room, authToken) {
			writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
			return
		}

		// 同 content 分支：一律签发 token，文件名不再进分享页地址 ——
		// 分享页会先问一次 GET /share 拿到它，再拼 /file/<uuid>/<name>。
		token, expiresAt, err := s.issueShareToken("file", fileUUID, room, ttl, maxUses, req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share_token_failed", "Failed to generate share token", "生成分享令牌失败")
			return
		}
		filename := fileInfo.Name
		if filename == "" {
			filename = "file"
		}
		rawQuery := url.Values{}
		rawQuery.Set(shareTokenQueryKey, token)
		response := map[string]interface{}{
			"type":      "file",
			"uuid":      fileUUID,
			"room":      room,
			"ttl":       ttl,
			"expiresAt": expiresAt,
			"maxUses":   maxUses,
			"token":     token,
			"url":       s.buildSharePageURL(r, token),
			"rawUrl":    s.buildAbsoluteURL(r, fmt.Sprintf("/file/%s/%s", fileUUID, url.PathEscape(filename)), rawQuery),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return

	default:
		writeError(w, http.StatusBadRequest, "unsupported_type", "Unsupported type", "不支持的 type")
	}
}
