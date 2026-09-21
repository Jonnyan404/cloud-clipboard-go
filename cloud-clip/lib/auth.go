package lib

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type RoomAuthRequirement struct {
	Room     string
	Required bool
	Password string
	// FileExpire: nil=使用全局 file.expire；0=该房间文件永不过期；>0=覆盖过期秒数
	FileExpire *int64
}

// RoomAuthEntry 单个房间的认证与文件留存配置。
// 配置值支持三种 JSON 形式（UnmarshalJSON 兼容）：
//
//	"password"                       -> 仅密码（旧格式）
//	12345                            -> 数字密码
//	{"password": "x", "fileExpire": 0} -> 密码 + 文件过期覆盖（fileExpire: 0=永不过期，>0=秒数，<0=回退全局）
//	{"open": true, "fileExpire": 0}  -> **开放房间**：不要密码，即使全局 server.auth 设了也一样
//
// `open` 单独一个字段、而不是拿「空密码」当信号，有两个原因：
//  1. 空字符串在这份配置里**已经有含义**（只接受全局 auth，见 config.md）——
//     改掉它会静默改变所有现有配置的含义，某个房间会悄悄敞开且不报错。安全设置不能这么反转。
//  2. 空密码和「压根没配过这个房间」在 JSON 里长得一样，而这两者的意图正好相反。
type RoomAuthEntry struct {
	Password   string `json:"password"`
	FileExpire *int64 `json:"fileExpire"`
	Open       bool   `json:"open"`
}

func (e *RoomAuthEntry) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	var strVal string
	if err := json.Unmarshal(trimmed, &strVal); err == nil {
		e.Password = strings.TrimSpace(strVal)
		return nil
	}

	var numVal json.Number
	if err := json.Unmarshal(trimmed, &numVal); err == nil {
		e.Password = numVal.String()
		return nil
	}

	var obj struct {
		Password   interface{} `json:"password"`
		FileExpire *float64    `json:"fileExpire"`
		Open       bool        `json:"open"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return err
	}
	if obj.Password != nil {
		e.Password = normalizeAuthValue(obj.Password)
	}
	if obj.FileExpire != nil {
		v := int64(*obj.FileExpire)
		e.FileExpire = &v
	}
	e.Open = obj.Open
	return nil
}

type RoomAuthConfig map[string]RoomAuthEntry

func normalizeAuthValue(auth interface{}) string {
	switch value := auth.(type) {
	case string:
		return value
	case int:
		if value != 0 {
			return strconv.Itoa(value)
		}
	case float64:
		if value != 0 {
			return strconv.FormatFloat(value, 'f', 0, 64)
		}
	case json.Number:
		return string(value)
	}

	return ""
}

func normalizeRoomAuthConfig(roomAuth RoomAuthConfig) RoomAuthConfig {
	if len(roomAuth) == 0 {
		return RoomAuthConfig{}
	}

	normalized := make(RoomAuthConfig, len(roomAuth))
	for room, entry := range roomAuth {
		normalized[normalizeRoomName(room)] = entry
	}

	return normalized
}

func extractAuthToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
		return authHeader
	}

	return r.URL.Query().Get("auth")
}

// extractWebSocketToken 提取 WebSocket 握手使用的 token。
// 优先取 Authorization / ?auth= 以兼容旧客户端，其次取 Sec-WebSocket-Protocol 子协议，
// 避免凭据出现在 URL 中泄漏到访问日志。
func extractWebSocketToken(r *http.Request) string {
	if token := extractAuthToken(r); token != "" {
		return token
	}
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			return p
		}
	}
	return ""
}

func extractAuthTokens(r *http.Request) []string {
	tokens := []string{}
	pushToken := func(token string) {
		normalized := strings.TrimSpace(token)
		if normalized == "" {
			return
		}
		for _, existing := range tokens {
			if existing == normalized {
				return
			}
		}
		tokens = append(tokens, normalized)
	}

	pushToken(extractAuthToken(r))

	extraHeader := strings.TrimSpace(r.Header.Get("X-Room-Auth-Tokens"))
	if extraHeader == "" {
		return tokens
	}

	var parsed []string
	if err := json.Unmarshal([]byte(extraHeader), &parsed); err == nil {
		for _, token := range parsed {
			pushToken(token)
		}
		return tokens
	}

	for _, token := range strings.Split(extraHeader, ",") {
		pushToken(token)
	}

	return tokens
}

func (s *ClipboardServer) resolveRoomAuth(room string) RoomAuthRequirement {
	normalizedRoom := normalizeRoomName(room)
	globalPassword := normalizeAuthValue(s.config.Server.Auth)
	entry, hasEntry := s.config.Server.RoomAuth[normalizedRoom]

	// 房间自己带密码 → 用它。全局密码**仍然有效**（见 tokenMatchesRoom），
	// 所以 roomAuth 是「多给一把钥匙」，不是「换锁」。
	if hasEntry && entry.Password != "" {
		// ⚠️ 同时写了 open 和 password 是配置写错了。**密码优先** ——
		// 宁可多要一次密码，也不能因为配置里多打了一个字段就把房间敞开。
		return RoomAuthRequirement{Room: normalizedRoom, Required: true, Password: entry.Password, FileExpire: entry.FileExpire}
	}

	// 显式开放：**不**回落全局 auth。这就是「全局加密 + 个别房间开放」的表达方式。
	if hasEntry && entry.Open {
		return RoomAuthRequirement{Room: normalizedRoom, FileExpire: entry.FileExpire}
	}

	// 没配过、或配了个空密码 → 回落全局 auth（旧行为，别改回去）。
	if globalPassword != "" {
		return RoomAuthRequirement{Room: normalizedRoom, Required: true, Password: globalPassword, FileExpire: entry.FileExpire}
	}

	return RoomAuthRequirement{Room: normalizedRoom, FileExpire: entry.FileExpire}
}

// resolveFileExpireSeconds 返回指定房间文件上传生效的过期秒数：0 表示永不过期，>0 为秒数
func (s *ClipboardServer) resolveFileExpireSeconds(room string) int64 {
	global := int64(s.config.File.Expire)
	fileExpire := s.resolveRoomAuth(room).FileExpire
	if fileExpire == nil {
		return global
	}
	if *fileExpire < 0 {
		s.logger.Printf("配置警告: 房间 '%s' 的 roomAuth.fileExpire 为负数 (%d)，已回退为全局 file.expire", normalizeRoomName(room), *fileExpire)
		return global
	}
	return *fileExpire
}

func (s *ClipboardServer) tokenMatchesRoom(room string, token string) bool {
	if token == "" {
		return false
	}

	if s.validateRoomSessionToken(room, token) {
		return true
	}

	globalPassword := normalizeAuthValue(s.config.Server.Auth)
	if globalPassword != "" && token == globalPassword {
		return true
	}

	normalizedRoom := normalizeRoomName(room)
	if entry, ok := s.config.Server.RoomAuth[normalizedRoom]; ok && entry.Password != "" {
		return token == entry.Password
	}

	return false
}

func (s *ClipboardServer) canAccessRoom(room string, token string) bool {
	requirement := s.resolveRoomAuth(room)
	if !requirement.Required {
		return true
	}

	return s.tokenMatchesRoom(room, token)
}

func (s *ClipboardServer) hasRoomAuthEntry(room string) bool {
	normalizedRoom := normalizeRoomName(room)
	_, ok := s.config.Server.RoomAuth[normalizedRoom]
	return ok

}

func (s *ClipboardServer) getUploadedFileRoom(uuid string) (string, bool) {
	s.runMutex.Lock()
	defer s.runMutex.Unlock()

	fileInfo, ok := s.uploadFileMap[uuid]
	if !ok {
		return "", false
	}

	return normalizeRoomName(fileInfo.Room), true
}

func (s *ClipboardServer) inferRequestRoom(r *http.Request) string {
	filePrefix := s.config.Server.Prefix + "/file/"
	chunkPrefix := s.config.Server.Prefix + "/upload/chunk/"
	finishPrefix := s.config.Server.Prefix + "/upload/finish/"

	var uuid string
	switch {
	case strings.HasPrefix(r.URL.Path, filePrefix):
		pathPart := strings.TrimPrefix(r.URL.Path, filePrefix)
		uuid = strings.SplitN(pathPart, "/", 2)[0]
	case strings.HasPrefix(r.URL.Path, chunkPrefix):
		uuid = strings.TrimPrefix(r.URL.Path, chunkPrefix)
	case strings.HasPrefix(r.URL.Path, finishPrefix):
		uuid = strings.TrimPrefix(r.URL.Path, finishPrefix)
	}

	// 文件已经登记过就以它自己记录的房间为准，**不能信客户端传的 ?room=**。
	// 否则 `GET /file/<uuid>/<name>?room=default` 就能把受保护房间的文件读出来：
	// 鉴权会去查 default 房间的策略，而 default 往往没设密码，直接放行。
	// ?room= 只留给「文件还没登记」的情况（例如刚创建、尚未落 map 的上传），
	// 那种情况下 handle_file 自己也找不到文件，会 404，不会泄字节。
	if uuid != "" {
		if room, ok := s.getUploadedFileRoom(uuid); ok {
			return room
		}
	}

	if _, hasRoom := r.URL.Query()["room"]; hasRoom {
		return normalizeRoomName(r.URL.Query().Get("room"))
	}

	return "default"
}

// 错误响应统一走 handler.go 的 writeError —— 这里不再单独实现一份，
// 免得同一个状态码在两条路径上给出不同的响应体形状。
