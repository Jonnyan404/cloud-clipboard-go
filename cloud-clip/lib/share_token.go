package lib

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	shareTokenQueryKey     = "t"
)

type shareClaims struct {
	Type string `json:"typ"` // content | file
	ID   string `json:"id"`  // content id or file uuid
	Room string `json:"room"`
	Exp  int64  `json:"exp"`
}

type shareRequest struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	UUID string `json:"uuid"`
	TTL  int    `json:"ttl"`
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
				_, _ = h.Write([]byte(s.config.Server.RoomAuth[room]))
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
	if claims.Type == "" || claims.ID == "" || claims.Exp <= 0 {
		return nil, false
	}
	if time.Now().Unix() > claims.Exp {
		return nil, false
	}

	return &claims, true
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

func (s *ClipboardServer) handle_share(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAuthJSONError(w, http.StatusMethodNotAllowed, "仅允许 POST 请求")
		return
	}

	var req shareRequest
	decoder := json.NewDecoder(r.Body)
	// 与 authMiddleware 保持一致的 CORS 行为，便于前后端分离调用
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Room-Auth-Tokens")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
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
			// 未指定 room 时再尝试一次全表匹配失败即为不存在
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
		}

		if requirement.Required {
			token, err := s.signShareClaims(shareClaims{
				Type: "content",
				ID:   idStr,
				Room: room,
				Exp:  expiresAt,
			})
			if err != nil {
				writeAuthJSONError(w, http.StatusInternalServerError, "生成分享令牌失败")
				return
			}
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
		}

		if requirement.Required {
			token, err := s.signShareClaims(shareClaims{
				Type: "file",
				ID:   fileUUID,
				Room: room,
				Exp:  expiresAt,
			})
			if err != nil {
				writeAuthJSONError(w, http.StatusInternalServerError, "生成分享令牌失败")
				return
			}
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
