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
)

type shareClaims struct {
	Type    string `json:"typ"`           // content | file | room_session
	ID      string `json:"id"`            // content id or file uuid or room
	Room    string `json:"room"`          // 绑定的房间（默认房间为空串）
	Scope   string `json:"sc,omitempty"`  // room_session 的 scope: ""=房间专属, "global"=全局所有房间
	Exp     int64  `json:"exp"`
	JTI     string `json:"jti,omitempty"` // token id when usage-limited
	MaxUses int    `json:"mu,omitempty"`  // 0 = unlimited
}

type shareRequest struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	UUID    string `json:"uuid"`
	TTL     int    `json:"ttl"`
	MaxUses int    `json:"maxUses"`
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

func (s *ClipboardServer) issueShareToken(shareType, id, room string, ttl, maxUses int) (token string, expiresAt int64, err error) {
	expiresAt = time.Now().Unix() + int64(ttl)
	claims := shareClaims{
		Type:    shareType,
		ID:      id,
		Room:    room,
		Exp:     expiresAt,
		MaxUses: maxUses,
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

func (s *ClipboardServer) handle_share(w http.ResponseWriter, r *http.Request) {
	// 与 authMiddleware 保持一致的 CORS 行为，便于前后端分离调用
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Room-Auth-Tokens")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeAuthJSONError(w, http.StatusMethodNotAllowed, "仅允许 POST 请求")
		return
	}

	var req shareRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && err.Error() != "EOF" {
		writeAuthJSONError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	shareType := strings.ToLower(strings.TrimSpace(req.Type))
	if shareType == "" {
		writeAuthJSONError(w, http.StatusBadRequest, "缺少 type")
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
			writeAuthJSONError(w, http.StatusBadRequest, "缺少 id")
			return
		}
		contentID, err := strconv.Atoi(idStr)
		if err != nil || contentID < 0 {
			writeAuthJSONError(w, http.StatusBadRequest, "无效的 id")
			return
		}

		room, _, _, found := s.findContentForShare(contentID, requestedRoom, hasRequestedRoom)
		if !found {
			writeAuthJSONError(w, http.StatusNotFound, "内容未找到")
			return
		}
		if !s.canAccessRoom(room, authToken) {
			writeAuthJSONError(w, http.StatusUnauthorized, "无权访问该房间")
			return
		}

		query := url.Values{}
		if room != "default" {
			query.Set("room", room)
		}

		requirement := s.resolveRoomAuth(room)
		expiresAt := time.Now().Unix() + int64(ttl)
		response := map[string]interface{}{
			"type":      "content",
			"id":        idStr,
			"room":      room,
			"ttl":       ttl,
			"expiresAt": expiresAt,
			"maxUses":   maxUses,
		}

		if requirement.Required {
			token, exp, err := s.issueShareToken("content", idStr, room, ttl, maxUses)
			if err != nil {
				writeAuthJSONError(w, http.StatusInternalServerError, "生成分享令牌失败")
				return
			}
			expiresAt = exp
			response["expiresAt"] = expiresAt
			query.Set(shareTokenQueryKey, token)
			response["token"] = token
		}

		response["url"] = s.buildAbsoluteURL(r, fmt.Sprintf("/content/%s", idStr), query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return

	case "file":
		fileUUID := strings.TrimSpace(req.UUID)
		if fileUUID == "" {
			fileUUID = strings.TrimSpace(req.ID)
		}
		if fileUUID == "" {
			writeAuthJSONError(w, http.StatusBadRequest, "缺少 uuid")
			return
		}

		s.runMutex.Lock()
		fileInfo, exists := s.uploadFileMap[fileUUID]
		s.runMutex.Unlock()
		if !exists {
			writeAuthJSONError(w, http.StatusNotFound, "文件未找到或已过期")
			return
		}
		if fileInfo.ExpireTime > 0 && fileInfo.ExpireTime < time.Now().Unix() {
			writeAuthJSONError(w, http.StatusNotFound, "文件已过期")
			return
		}

		room := normalizeRoomName(fileInfo.Room)
		if hasRequestedRoom && room != requestedRoom {
			writeAuthJSONError(w, http.StatusNotFound, "文件未找到或已过期")
			return
		}
		if !s.canAccessRoom(room, authToken) {
			writeAuthJSONError(w, http.StatusUnauthorized, "无权访问该房间")
			return
		}

		filename := fileInfo.Name
		if filename == "" {
			filename = "file"
		}
		query := url.Values{}
		requirement := s.resolveRoomAuth(room)
		expiresAt := time.Now().Unix() + int64(ttl)
		response := map[string]interface{}{
			"type":      "file",
			"uuid":      fileUUID,
			"room":      room,
			"ttl":       ttl,
			"expiresAt": expiresAt,
			"maxUses":   maxUses,
		}

		if requirement.Required {
			token, exp, err := s.issueShareToken("file", fileUUID, room, ttl, maxUses)
			if err != nil {
				writeAuthJSONError(w, http.StatusInternalServerError, "生成分享令牌失败")
				return
			}
			expiresAt = exp
			response["expiresAt"] = expiresAt
			query.Set(shareTokenQueryKey, token)
			response["token"] = token
		}

		response["url"] = s.buildAbsoluteURL(r, fmt.Sprintf("/file/%s/%s", fileUUID, url.PathEscape(filename)), query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return

	default:
		writeAuthJSONError(w, http.StatusBadRequest, "不支持的 type")
	}
}
