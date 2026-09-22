package lib

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// roomSessionTTLSeconds 房间会话令牌的有效期，默认 1 小时。
const roomSessionTTLSeconds = 60 * 60

func (s *ClipboardServer) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	room := normalizeRoomName(r.URL.Query().Get("room"))
	if room == "" {
		room = "default"
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "Invalid request body", "无效的请求体")
		return
	}

	password := strings.TrimSpace(req.Password)
	if password == "" {
		writeError(w, http.StatusUnauthorized, "password_required", "Password required", "密码不能为空")
		return
	}

	if !s.tokenMatchesRoom(room, password) {
		writeError(w, http.StatusUnauthorized, "wrong_password", "Wrong password", "密码不正确")
		return
	}

	// 使用全局密码登录时，签发对所有房间有效的全局会话令牌
	scope := ""
	if globalPassword := normalizeAuthValue(s.config.Server.Auth); globalPassword != "" && password == globalPassword {
		scope = "global"
	}

	token, err := s.issueRoomSessionToken(room, roomSessionTTLSeconds, scope)
	if err != nil {
		s.logger.Printf("错误: 签发房间会话令牌失败: %v", err)
		writeError(w, http.StatusInternalServerError, "token_issue_failed", "Failed to issue token", "令牌签发失败")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token,
		"expiresAt": time.Now().Unix() + roomSessionTTLSeconds,
		"scope":     scope,
	})
}

// handleAuthTokenRefresh 使用仍有效的会话令牌签发新令牌，无需密码即可静默续期。
func (s *ClipboardServer) handleAuthTokenRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	room := normalizeRoomName(r.URL.Query().Get("room"))
	if room == "" {
		room = "default"
	}

	token := extractAuthToken(r)
	claims, ok := s.parseRoomSessionToken(token)
	if !ok || !s.validateRoomSessionToken(room, token) {
		writeError(w, http.StatusUnauthorized, "session_token_invalid", "Session token invalid or expired", "会话令牌无效或已过期")
		return
	}

	// 续签时保留原令牌的 scope，避免全局会话降级为房间专属
	newToken, err := s.issueRoomSessionToken(room, roomSessionTTLSeconds, claims.Scope)
	if err != nil {
		s.logger.Printf("错误: 续签房间会话令牌失败: %v", err)
		writeError(w, http.StatusInternalServerError, "token_refresh_failed", "Failed to refresh token", "令牌续签失败")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     newToken,
		"expiresAt": time.Now().Unix() + roomSessionTTLSeconds,
		"scope":     claims.Scope,
	})
}

func (s *ClipboardServer) handle_myip(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ip": get_remote_ip(r),
	})
}

func (s *ClipboardServer) handle_server(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("处理 /server 请求，来自: %s", get_remote_ip(r))
	authNeeded := false
	authorized := true
	roomProtected := false
	globalPassword := normalizeAuthValue(s.config.Server.Auth)
	if _, hasRoom := r.URL.Query()["room"]; hasRoom {
		room := r.URL.Query().Get("room")
		authNeeded = s.resolveRoomAuth(room).Required
		authorized = s.canAccessRoom(room, extractAuthToken(r))
		roomProtected = s.hasRoomAuthEntry(room)
	} else if globalPassword != "" {
		authNeeded = true
		authorized = s.canAccessRoom("default", extractAuthToken(r))
	}

	wsProtocol := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		wsProtocol = "wss"
	}

	response := map[string]interface{}{
		"server":        fmt.Sprintf("%s://%s%s/push", wsProtocol, r.Host, s.config.Server.Prefix),
		"auth":          authNeeded,
		"authorized":    authorized,
		"roomProtected": roomProtected,
		"config": map[string]interface{}{
			"server": map[string]interface{}{
				"history":  s.config.Server.History,
				"roomList": s.config.Server.RoomList,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Printf("错误: 编码 /server 响应失败: %v", err)
		writeError(w, http.StatusInternalServerError, "encode_failed", "Failed to encode response", "Failed to encode response")
	}
}

func (s *ClipboardServer) handle_push(w http.ResponseWriter, r *http.Request) {
	ip := get_remote_ip(r)
	room := normalizeRoomName(r.URL.Query().Get("room"))
	s.logger.Printf("处理 /push WebSocket 连接请求，来自: %s, 房间: %s", ip, room)

	requirement := s.resolveRoomAuth(room)
	authNeeded := requirement.Required
	if authNeeded {
		token := extractWebSocketToken(r)
		if token == "" {
			s.logger.Printf("WebSocket 认证失败: 未提供 token。来自 IP: %s, 房间: %s", ip, room)
			writeError(w, http.StatusUnauthorized, "unauthorized_missing_token", "Missing auth token", "Unauthorized: Missing token")
			return
		}
		if !s.canAccessRoom(room, token) {
			s.logger.Printf("WebSocket 认证失败: 提供的 token 与房间 '%s' 的认证配置不匹配。来自 IP: %s", room, ip)
			writeError(w, http.StatusUnauthorized, "unauthorized_invalid_token", "Invalid auth token", "Unauthorized: Invalid token")
			return
		}
		s.logger.Printf("WebSocket 认证成功。来自 IP: %s, 房间: %s", ip, room)
	}

	// 回显客户端通过 Sec-WebSocket-Protocol 子协议提供的 token，避免 token 出现在 URL/访问日志中。
	// 浏览器要求服务端必须回选一个子协议，否则握手会被判定失败。
	respHeader := http.Header{}
	if proto := r.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		respHeader.Set("Sec-WebSocket-Protocol", strings.TrimSpace(strings.Split(proto, ",")[0]))
	}

	conn, err := upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		s.logger.Printf("错误: WebSocket 升级失败: %v", err)
		return
	}

	// 生成设备 ID 和元数据
	userAgent := r.Header.Get("User-Agent")
	deviceID := fmt.Sprintf("%d", hash_murmur3([]byte(fmt.Sprintf("%s %s", r.RemoteAddr, userAgent)), s.deviceHashSeed))

	clientUA := s.parser.Parse(userAgent)
	deviceMeta := DeviceMeta{
		ID:      deviceID,
		Type:    detectDeviceType(userAgent, clientUA.Os.Family),
		Name:    resolveDeviceName(r),
		Device:  strings.TrimSpace(fmt.Sprintf("%s %s %s", clientUA.Device.Brand, clientUA.Device.Model, clientUA.Os.Family)),
		OS:      fmt.Sprintf("%s %s", clientUA.Os.Family, clientUA.Os.Major),
		Browser: fmt.Sprintf("%s %s", clientUA.UserAgent.Family, clientUA.UserAgent.Major),
	}

	// 第一次加锁：注册连接和获取当前房间内的设备列表
	var devicesInRoom []DeviceMeta
	s.runMutex.Lock()
	s.websockets[conn] = true
	s.room_ws[conn] = room
	s.deviceConnected[deviceID] = deviceMeta
	s.connDeviceIDMap[conn] = deviceID
	s.updateRoomDeviceCount(room, deviceID, true)

	s.logger.Printf("新 WebSocket 客户端连接: %s (ID: %s), 房间: %s. 当前连接数: %d, 设备数: %d",
		conn.RemoteAddr(), deviceID, room, len(s.websockets), len(s.deviceConnected))

	// 获取房间内现有设备列表（排除当前设备）
	for _, existingDeviceID := range s.getDeviceIDsInRoomLocked(room, deviceID) {
		if devMeta, ok := s.deviceConnected[existingDeviceID]; ok {
			devicesInRoom = append(devicesInRoom, devMeta)
		}
	}
	s.runMutex.Unlock() // 尽早释放锁

	// 向新客户端发送房间内当前连接的设备列表（在锁外执行）
	for _, devMeta := range devicesInRoom {
		wsMsg := WebSocketMessage{
			Event: "connect",
			Data:  devMeta,
		}
		if err := conn.WriteJSON(wsMsg); err != nil {
			s.logger.Printf("错误: 发送现有设备 %s 信息到新客户端 %s 失败: %v", devMeta.ID, conn.RemoteAddr(), err)
			// 如果发送失败，清理连接并返回
			s.cleanupWebSocketConnection(conn, deviceID, room)
			return
		}
	}

	// 向房间内的其他客户端广播新设备连接（此函数内部会处理锁）
	newDeviceClientMsg := WebSocketMessage{
		Event: "connect",
		Data:  deviceMeta,
	}
	s.broadcastWebSocketMessageToRoomExcept(newDeviceClientMsg, room, conn)

	// 第二次加锁：获取历史消息（短时间持锁）
	var historyMessages []PostEvent
	s.messageQueue.Lock()
	for _, msg := range s.messageQueue.List {
		if msg.Data.Room() == "" || msg.Data.Room() == room {
			historyMessages = append(historyMessages, msg)
		}
	}
	s.messageQueue.Unlock() // 立即释放消息队列锁

	// 发送历史消息（在锁外执行）
	for _, msg := range historyMessages {
		var clientPayload interface{}
		if msg.Data.TextReceive != nil {
			clientPayload = msg.Data.TextReceive
		} else if msg.Data.FileReceive != nil {
			clientPayload = msg.Data.FileReceive
		} else {
			continue
		}

		wsMsg := WebSocketMessage{
			Event: "receive",
			Data:  clientPayload,
		}
		if err := conn.WriteJSON(wsMsg); err != nil {
			s.logger.Printf("错误: 发送历史消息到客户端 %s 失败: %v", conn.RemoteAddr(), err)
			s.cleanupWebSocketConnection(conn, deviceID, room)
			return
		}
	}
	s.logger.Printf("已发送 %d 条历史消息到客户端 %s (房间: %s)", len(historyMessages), conn.RemoteAddr(), room)

	// 发送配置信息给新连接的客户端
	clientConfigData := struct {
		Version string `json:"version"`
		Server  struct {
			History  int    `json:"history"`
			Prefix   string `json:"prefix"`
			RoomList bool   `json:"roomList"`
		} `json:"server"`
		Text struct {
			Limit int `json:"limit"`
		} `json:"text"`
		File struct {
			Expire int `json:"expire"`
			Chunk  int `json:"chunk"`
			Limit  int `json:"limit"`
		} `json:"file"`
		Auth bool `json:"auth"`
	}{
		Version: server_version,
		Server: struct {
			History  int    `json:"history"`
			Prefix   string `json:"prefix"`
			RoomList bool   `json:"roomList"`
		}{
			History:  s.config.Server.History,
			Prefix:   s.config.Server.Prefix,
			RoomList: s.config.Server.RoomList,
		},
		Text: s.config.Text,
		File: s.config.File,
		Auth: authNeeded,
	}

	configWsMsg := WebSocketMessage{
		Event: "config",
		Data:  clientConfigData,
	}
	if err := conn.WriteJSON(configWsMsg); err != nil {
		s.logger.Printf("错误: 发送配置信息到客户端 %s 失败: %v", conn.RemoteAddr(), err)
	} else {
		s.logger.Printf("已发送配置信息到客户端 %s", conn.RemoteAddr())
	}

	// 启动 WebSocket 消息读取 goroutine
	go func() {
		defer s.cleanupWebSocketConnection(conn, deviceID, room)

		done := make(chan struct{})
		defer close(done)

		var lastPingNano int64 // 原子访问；0 表示当前无待确认的 ping

		// 设置 pong 处理，测量局域网 RTT
		conn.SetPongHandler(func(appData string) error {
			nanos := atomic.SwapInt64(&lastPingNano, 0)
			if nanos != 0 {
				rtt := float64(time.Now().UnixNano()-nanos) / float64(time.Millisecond)
				s.latency.add(rtt)
			}
			return nil
		})

		// 定期发送 ping 以计算延迟
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					atomic.StoreInt64(&lastPingNano, time.Now().UnixNano())
					if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(2*time.Second)); err != nil {
						return
					}
				}
			}
		}()

		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					s.logger.Printf("错误: WebSocket 读取错误 (客户端: %s, ID: %s): %v", conn.RemoteAddr(), deviceID, err)
				} else {
					s.logger.Printf("WebSocket 连接正常关闭 (客户端: %s, ID: %s)", conn.RemoteAddr(), deviceID)
				}
				break
			}
			if len(p) > 0 {
				// Web 端延迟测量: 客户端发送 {"event":"ping","data":<clientMs>},
				// 这里原样回显 data, 客户端用 (Date.now()-data) 计算 RTT。
				var pingMsg WebSocketMessage
				if json.Unmarshal(p, &pingMsg) == nil && pingMsg.Event == "ping" {
					if t, ok := pingMsg.Data.(float64); ok {
						if err := conn.WriteJSON(WebSocketMessage{Event: "pong", Data: t}); err != nil {
							return
						}
						continue
					}
				}
				s.logger.Printf("收到来自 %s (ID: %s) 的 WebSocket 心跳消息: 类型 %d, 内容: %s",
					conn.RemoteAddr(), deviceID, messageType, string(p))
			}
		}
	}()
}

func (s *ClipboardServer) handle_file(w http.ResponseWriter, r *http.Request) {
	// 修改 UUID 提取逻辑
	pathPart := strings.TrimPrefix(r.URL.Path, s.config.Server.Prefix+"/file/")
	pathSegments := strings.SplitN(pathPart, "/", 2) // 最多分割成两部分
	uuid := pathSegments[0]                          // 第一部分总是 UUID

	s.logger.Printf("处理文件请求: %s, 方法: %s", uuid, r.Method)

	s.runMutex.Lock() // 保护 uploadFileMap 的读取
	fileInfo, ok := s.uploadFileMap[uuid]
	s.runMutex.Unlock()

	if !ok {
		s.logger.Printf("文件未找到或已过期: %s", uuid)
		writeError(w, http.StatusNotFound, "file_not_found", "File not found or expired", "文件未找到或已过期")
		return
	}

	// 检查文件是否已过期 (双重检查，因为 cleanExpiredFilesLoop 是异步的)
	// ExpireTime <= 0 表示永不过期
	if fileInfo.ExpireTime > 0 && fileInfo.ExpireTime < time.Now().Unix() {
		s.logger.Printf("尝试访问已过期的文件: %s (UUID: %s)", fileInfo.Name, uuid)
		// 从 map 中移除并尝试删除文件
		s.runMutex.Lock()
		delete(s.uploadFileMap, uuid)
		s.runMutex.Unlock()
		go os.Remove(filepath.Join(s.storageFolder, uuid)) // 异步删除
		writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
		return
	}

	filePath := filepath.Join(s.storageFolder, uuid)

	switch r.Method {
	case http.MethodGet:
		s.logger.Printf("提供文件下载: %s (UUID: %s), 路径: %s", fileInfo.Name, uuid, filePath)

		file, err := os.Open(filePath) // 打开文件以供 ServeContent 使用
		if err != nil {
			s.logger.Printf("错误: 打开文件失败: %v", err)
			writeError(w, http.StatusNotFound, "file_missing_on_disk", "File missing on disk", "文件在磁盘上未找到")
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			s.logger.Printf("错误: 获取文件状态失败: %v", err)
			writeError(w, http.StatusInternalServerError, "file_stat_failed", "Cannot read file info", "无法获取文件状态")
			return
		}

		// 设置 Content-Disposition
		dispositionType := "inline" // 默认为内联显示
		if r.URL.Query().Get("download") == "true" {
			dispositionType = "attachment"
		}
		disposition := fmt.Sprintf("%s; filename=%q", dispositionType, fileInfo.Name)
		w.Header().Set("Content-Disposition", disposition)

		// 使用 http.ServeContent 提供文件内容
		http.ServeContent(w, r, fileInfo.Name, stat.ModTime(), file)

	case http.MethodDelete:
		// 需要认证才能删除文件，此处已有 authMiddleware 保护
		s.logger.Printf("删除文件: %s (UUID: %s)", fileInfo.Name, uuid)

		err := os.Remove(filePath)
		if err != nil && !os.IsNotExist(err) {
			s.logger.Printf("错误: 删除文件失败: %v", err)
			writeError(w, http.StatusInternalServerError, "file_delete_failed", "Failed to delete file", "删除文件失败")
			return
		}

		s.runMutex.Lock()
		delete(s.uploadFileMap, uuid)
		s.runMutex.Unlock()

		s.saveHistoryData()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "文件删除成功"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "方法不允许")
	}
}

func (s *ClipboardServer) handle_text(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	room := normalizeRoomName(r.URL.Query().Get("room"))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Printf("错误: 读取 /text 请求体失败: %v", err)
		writeError(w, http.StatusInternalServerError, "body_read_failed", "Cannot read request body", "无法读取请求体")
		return
	}
	defer r.Body.Close()

	text := string(body)
	if s.config.Text.Limit > 0 && len(text) > s.config.Text.Limit {
		s.logger.Printf("错误: 文本内容超出限制 (%d > %d)", len(text), s.config.Text.Limit)
		writeError(w, http.StatusRequestEntityTooLarge, "text_too_long", "Text too long", fmt.Sprintf("文本内容超出限制 (最大 %d 字符)", s.config.Text.Limit))
		return
	}

	// 检查是否有 ID 参数用于覆盖
	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		// 尝试覆盖现有消息
		id, err := strconv.Atoi(idStr)
		if err != nil {
			s.logger.Printf("无效的 ID 参数: %s", idStr)
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid id parameter", "无效的 ID 参数")
			return
		}

		// 查找并更新消息
		if updated := s.updateTextMessage(id, text, room, r); updated {
			w.Header().Set("Content-Type", "application/json")
			// 构建内容 URL
			scheme := getScheme(r)
			contentURL := fmt.Sprintf("%s://%s%s/content/%s", scheme, r.Host, s.config.Server.Prefix, idStr)
			if room != "default" {
				contentURL += fmt.Sprintf("?room=%s", room)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"url":  contentURL,
				"id":   idStr,
				"type": "text",
			})
			return
		} else {
			s.logger.Printf("未找到可更新的文本消息 ID: %d (房间: %s)", id, room)
			writeError(w, http.StatusNotFound, "message_not_updatable", "Message not found or not updatable", "消息未找到或无法更新")
			return
		}
	}

	s.logger.Printf("收到文本消息 (房间: %s): %s", room, text)
	event := s.addMessageToQueueAndBroadcast("text", text, room, r)

	// 响应 (可以效仿 auth.go 中的 enhanceHandleText 返回内容 URL)
	scheme := getScheme(r)
	contentURL := fmt.Sprintf("%s://%s%s/content/%d", scheme, r.Host, s.config.Server.Prefix, event.Data.ID())
	if room != "default" {
		contentURL += fmt.Sprintf("?room=%s", room)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url":  contentURL,
		"id":   strconv.Itoa(event.Data.ID()),
		"type": "text",
	})
}

// updateTextMessage 更新指定 ID 的文本消息
func (s *ClipboardServer) updateTextMessage(id int, newContent string, room string, r *http.Request) bool {
	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	for i, msg := range s.messageQueue.List {
		if msg.Data.ID() == id && msg.Data.Type() == "text" && msg.Data.Room() == room {
			if msg.Data.TextReceive != nil {
				// 检查更新内容是否与原内容相同
				if msg.Data.TextReceive.Content == newContent {
					s.logger.Printf("文本消息 ID %d 内容未改变，无需更新 (房间: %s)", id, room)
					return true // 内容相同，直接返回，避免频繁触发写入操作
				}

				// 获取原内容用于日志
				originalContent := msg.Data.TextReceive.Content
				// 更新内容和时间戳（使用索引 i 修改原数组）
				s.messageQueue.List[i].Data.TextReceive.Content = newContent
				s.messageQueue.List[i].Data.TextReceive.Timestamp = time.Now().Unix()
				s.messageQueue.List[i].Data.TextReceive.SenderIP = get_remote_ip(r)
				s.messageQueue.List[i].Data.TextReceive.SenderDevice = s.parse_user_agent(r.UserAgent(), resolveDeviceName(r))

				// 广播更新事件
				wsMsg := WebSocketMessage{
					Event: "update",
					Data:  s.messageQueue.List[i].Data.TextReceive,
				}
				go s.broadcastWebSocketMessage(wsMsg, room)

				// 保存历史数据
				go s.saveHistoryData()

				s.logger.Printf("文本消息 ID %d 已更新 (房间: %s) - 原内容: '%s', 新内容: '%s'", id, room, originalContent, newContent)
				return true
			}
		}
	}
	return false
}

// 看板的列。**固定三列**，不做用户自建 —— 这是最小实现，见 handleContentColumn。
var boardColumns = map[string]bool{"todo": true, "doing": true, "done": true}

// normalizeBoardColumn 校验列名。空串归一成 todo（新条目默认落在待办）。
func normalizeBoardColumn(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "todo", true
	}
	if boardColumns[value] {
		return value, true
	}
	return "", false
}

// handleContentColumn 把一条内容挪到看板的某一列：`POST /content/<id>/column`。
//
// 看板的最小实现：**固定三列**（todo / doing / done），卡片就是剪贴板条目本身 ——
// 不建新表、不做「列内顺序」，条目上多一个 `column` 字段就够了（空 = 待办）。
// 列是**视图属性**：所有模式看的是同一批条目，只是看板按列摆。
//
// ⚠️ **故意不动 timestamp**。`updateTextMessage` 改正文时会把时间戳刷成现在，
// 但「把卡片挪到另一列」不该让它在时间流里跳到最前面 —— 挪个位置就重排整个列表太突然。
func (s *ClipboardServer) handleContentColumn(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content id", "无效的内容 ID")
		return
	}

	var body struct {
		Column string `json:"column"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body", "请求体不是合法的 JSON")
		return
	}
	column, columnOK := normalizeBoardColumn(body.Column)
	if !columnOK {
		writeError(w, http.StatusBadRequest, "invalid_column", "Unknown board column", "未知的看板列（只支持 todo / doing / done）")
		return
	}

	_, hasRequestedRoom := r.URL.Query()["room"]
	requestedRoom := normalizeRoomName(r.URL.Query().Get("room"))

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	for i, msg := range s.messageQueue.List {
		if msg.Data.ID() != id {
			continue
		}
		messageRoom := normalizeRoomName(msg.Data.Room())
		if hasRequestedRoom && messageRoom != requestedRoom {
			continue
		}
		// 与 handleContent 同一条鉴权：按**条目自己记录的房间**，不信客户端传的 ?room=
		if !s.canAccessContent(r, messageRoom, id) {
			writeError(w, http.StatusUnauthorized, "room_auth_required", "Room authentication required", "无权访问该房间")
			return
		}

		s.messageQueue.List[i].Data.SetColumn(column)

		// 广播载荷要和 updateTextMessage 一致：客户端 `case 'update'` 是
		// `{...app.received[i], ...data}` 原地合并，所以给**具体那一支**（含 id），不是外层 holder。
		var payload interface{}
		if msg.Data.TextReceive != nil {
			payload = s.messageQueue.List[i].Data.TextReceive
		} else {
			payload = s.messageQueue.List[i].Data.FileReceive
		}
		go s.broadcastWebSocketMessage(WebSocketMessage{Event: "update", Data: payload}, messageRoom)
		go s.saveHistoryData()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":     strconv.Itoa(id),
			"type":   msg.Data.Type(),
			"column": column,
		})
		return
	}

	writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "内容未找到")
}

func (s *ClipboardServer) handle_upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	// 获取请求路径和内容类型
	path := r.URL.Path
	contentType := r.Header.Get("Content-Type")
	s.logger.Printf("处理上传请求，路径: %s, 内容类型: %s, 来自: %s", path, contentType, get_remote_ip(r))

	room := normalizeRoomName(r.URL.Query().Get("room"))

	// 处理 /upload/chunk 路径（文件名初始化请求）
	if strings.HasSuffix(path, "/upload/chunk") && contentType == "text/plain" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Printf("错误: 读取文件名失败: %v", err)
			writeError(w, http.StatusBadRequest, "body_read_failed", "Cannot read request body", "无法读取请求体")
			return
		}
		defer r.Body.Close()

		filename := string(body)
		uuid := gen_UUID()
		s.logger.Printf("初始化分块上传: %s, 生成UUID: %s", filename, uuid)

		// 创建文件信息直接记录到 uploadFileMap 中
		// 房间级 fileExpire 覆盖全局 file.expire（0=永不过期）
		var expireTime int64
		if expireSeconds := s.resolveFileExpireSeconds(room); expireSeconds > 0 {
			expireTime = time.Now().Unix() + expireSeconds
		} else {
			expireTime = 0
		}
		s.runMutex.Lock()
		s.uploadFileMap[uuid] = File{
			Name:       filename,
			UUID:       uuid,
			Size:       0, // 初始大小为0
			ExpireTime: expireTime,
			UploadTime: time.Now().Unix(),
			Room:       room,
		}
		s.runMutex.Unlock()

		// 返回UUID响应
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]string{"uuid": uuid},
		})
		return
	}

	// 处理常规文件上传 (/upload 路径)
	// 检查文件大小限制
	if s.config.File.Limit > 0 && r.ContentLength > int64(s.config.File.Limit) {
		s.logger.Printf("错误: 文件大小 (%d) 超出限制 (%d)", r.ContentLength, s.config.File.Limit)
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File too large", fmt.Sprintf("文件大小超出限制 (最大 %d 字节)", s.config.File.Limit))
		return
	}

	err := r.ParseMultipartForm(int64(s.config.File.Limit)) // 使用文件大小限制作为 maxMemory
	if err != nil {
		s.logger.Printf("错误: 解析 multipart form 失败: %v", err)
		writeError(w, http.StatusBadRequest, "form_parse_failed", "Cannot parse form data", "无法解析表单数据")
		return
	}

	file, handler, err := r.FormFile("file") // "file" 是表单字段名
	if err != nil {
		s.logger.Printf("错误: 获取上传文件失败: %v", err)
		writeError(w, http.StatusBadRequest, "file_field_missing", "Cannot read uploaded file", "无法获取文件")
		return
	}
	defer file.Close()

	fileName := handler.Filename
	fileSize := handler.Size
	s.logger.Printf("收到文件上传: %s, 大小: %d, 房间: %s", fileName, fileSize, room)

	// 生成唯一文件名 (UUID)
	uuid := gen_UUID()
	filePath := filepath.Join(s.storageFolder, uuid)

	// 保存文件
	dst, err := os.Create(filePath)
	if err != nil {
		s.logger.Printf("错误: 创建文件 %s 失败: %v", filePath, err)
		writeError(w, http.StatusInternalServerError, "file_save_failed", "Cannot save file", "无法保存文件")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		s.logger.Printf("错误: 写入文件 %s 失败: %v", filePath, err)
		writeError(w, http.StatusInternalServerError, "file_write_failed", "Cannot write file", "无法写入文件")
		return
	}

	timestamp := time.Now().Unix()
	// 房间级 fileExpire 覆盖全局 file.expire（0=永不过期）
	var expireTime int64
	if expireSeconds := s.resolveFileExpireSeconds(room); expireSeconds > 0 {
		expireTime = timestamp + expireSeconds
	} else {
		expireTime = 0
	}

	// 创建文件信息
	fileInfo := File{
		Name:       fileName,
		UUID:       uuid,
		Size:       fileSize,
		UploadTime: timestamp,
		ExpireTime: expireTime,
		Room:       room,
	}

	s.runMutex.Lock() // 保护 uploadFileMap
	s.uploadFileMap[uuid] = fileInfo
	s.runMutex.Unlock()

	fileReceiveData := &FileReceive{
		Name:   fileName,
		Size:   fileSize,
		Expire: expireTime,
		Cache:  uuid,
		URL:    fmt.Sprintf("%s://%s%s/file/%s", getScheme(r), r.Host, s.config.Server.Prefix, uuid),
	}

	// 如果文件不太大，创建缩略图
	if fileSize <= 32*1024*1024 { // 32MB
		thumbnail, err := gen_thumbnail(filePath)
		if err == nil {
			s.logger.Printf("已为文件 %s 生成缩略图", fileName)
			fileReceiveData.Thumbnail = thumbnail
		} else {
			s.logger.Printf("生成缩略图失败: %v,文件类型可能不受支持", err)
		}
	}

	event := s.addMessageToQueueAndBroadcast("file", fileReceiveData, room, r)

	// 响应
	scheme := getScheme(r)
	contentURL := fmt.Sprintf("%s://%s%s/content/%d", scheme, r.Host, s.config.Server.Prefix, event.Data.ID())
	if room != "default" {
		contentURL += fmt.Sprintf("?room=%s", room)
	}
	responseType := DetermineResponseType(fileInfo.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url":  contentURL,
		"id":   strconv.Itoa(event.Data.ID()),
		"type": responseType,
	})
}

func (s *ClipboardServer) handle_chunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	// 从路径中提取 UUID
	uuid := strings.TrimPrefix(r.URL.Path, s.config.Server.Prefix+"/upload/chunk/")
	s.logger.Printf("处理分块上传请求, UUID: %s, 来自: %s", uuid, get_remote_ip(r))

	s.runMutex.Lock()
	fileInfo, ok := s.uploadFileMap[uuid]
	s.runMutex.Unlock()

	if !ok {
		s.logger.Printf("错误: 无效的 UUID: %s", uuid)
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid UUID", "无效的 UUID")
		return
	}

	// 读取请求体中的数据
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Printf("错误: 读取分块数据失败: %v", err)
		writeError(w, http.StatusInternalServerError, "chunk_read_failed", "Cannot read chunk data", "无法读取分块数据")
		return
	}
	defer r.Body.Close()

	// 更新文件大小
	newSize := fileInfo.Size + int64(len(data))
	s.logger.Printf("上传分块数据大小: %d, 累计大小: %d", len(data), newSize)

	// 检查文件大小是否超过限制
	if s.config.File.Limit > 0 && newSize > int64(s.config.File.Limit) {
		s.logger.Printf("错误: 文件大小已超过限制 (%d > %d)", newSize, s.config.File.Limit)
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "File too large", fmt.Sprintf("文件大小已超过限制 (最大 %d 字节)", s.config.File.Limit))
		return
	}

	// 更新文件信息
	fileInfo.Size = newSize
	s.runMutex.Lock()
	s.uploadFileMap[uuid] = fileInfo
	s.runMutex.Unlock()

	// 追加数据到文件
	filePath := filepath.Join(s.storageFolder, uuid)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		s.logger.Printf("错误: 打开文件 %s 失败: %v", filePath, err)
		writeError(w, http.StatusInternalServerError, "file_open_failed", "Cannot open file", "无法打开文件")
		return
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		s.logger.Printf("错误: 写入数据到文件 %s 失败: %v", filePath, err)
		writeError(w, http.StatusInternalServerError, "file_write_failed", "Cannot write file", "无法写入文件")
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

func (s *ClipboardServer) handle_finish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed", "仅允许 POST 请求")
		return
	}

	// 从路径中提取 UUID
	uuid := strings.TrimPrefix(r.URL.Path, s.config.Server.Prefix+"/upload/finish/")
	room := normalizeRoomName(r.URL.Query().Get("room"))

	s.logger.Printf("处理上传完成请求, UUID: %s, 房间: %s, 来自: %s", uuid, room, get_remote_ip(r))

	s.runMutex.Lock()
	fileInfo, ok := s.uploadFileMap[uuid]
	s.runMutex.Unlock()

	if !ok {
		s.logger.Printf("错误: 无效的 UUID: %s", uuid)
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid UUID", "无效的 UUID")
		return
	}

	if room == "default" && fileInfo.Room != "" {
		room = normalizeRoomName(fileInfo.Room)
	}

	// 生成消息相关信息
	timestamp := time.Now().Unix()

	filePath := filepath.Join(s.storageFolder, uuid)

	fileReceiveData := &FileReceive{
		ReceiveBase: ReceiveBase{
			Type:         "file",
			Room:         room,
			Timestamp:    timestamp,
			SenderIP:     get_remote_ip(r),
			SenderDevice: s.parse_user_agent(r.UserAgent(), resolveDeviceName(r)),
		},
		Name:   fileInfo.Name,
		Size:   fileInfo.Size,
		Cache:  uuid,
		Expire: fileInfo.ExpireTime,
		URL:    fmt.Sprintf("%s://%s%s/file/%s", getScheme(r), r.Host, s.config.Server.Prefix, uuid),
	}

	// 如果文件不太大，创建缩略图
	if fileInfo.Size <= 32*1024*1024 { // 32MB
		thumbnail, err := gen_thumbnail(filePath)
		if err == nil {
			s.logger.Printf("已为文件 %s 生成缩略图", fileInfo.Name)
			fileReceiveData.Thumbnail = thumbnail
		} else {
			s.logger.Printf("生成缩略图失败: %v,文件类型可能不受支持", err)
		}
	}

	// 添加消息到队列并广播
	event := s.addMessageToQueueAndBroadcast("file", fileReceiveData, room, r)
	s.logger.Printf("文件 %s (UUID: %s) 上传完成, 大小: %d, 房间: %s", fileInfo.Name, uuid, fileInfo.Size, room)

	// 构建响应
	scheme := getScheme(r)
	contentURL := fmt.Sprintf("%s://%s%s/content/%d", scheme, r.Host, s.config.Server.Prefix, event.Data.ID())
	if room != "default" {
		contentURL += fmt.Sprintf("?room=%s", room)
	}
	responseType := DetermineResponseType(fileInfo.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url":  contentURL,
		"id":   strconv.Itoa(event.Data.ID()),
		"type": responseType,
	})
}

func (s *ClipboardServer) handle_revoke(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 1 {
		writeError(w, http.StatusBadRequest, "invalid_revoke_path", "Invalid revoke path", "无效的撤销路径")
		return
	}
	idStr := parts[len(parts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_revoke_id", "Invalid revoke id", "无效的撤销 ID")
		return
	}

	token := extractAuthToken(r)
	requestedRoom, hasRequestedRoom := r.URL.Query()["room"]
	_ = requestedRoom

	s.messageQueue.Lock()
	var foundMsg PostEvent
	foundIndex := -1
	unauthorized := false

	for i := range s.messageQueue.List { // 使用大写 L
		if s.messageQueue.List[i].Data.ID() == id {
			messageRoom := normalizeRoomName(s.messageQueue.List[i].Data.Room())
			if hasRequestedRoom && messageRoom != normalizeRoomName(r.URL.Query().Get("room")) {
				continue
			}
			if !s.canAccessRoom(messageRoom, token) {
				unauthorized = true
				continue
			}
			foundMsg = s.messageQueue.List[i]
			foundIndex = i
			break
		}
	}
	if foundIndex != -1 {
		// 从消息队列中移除
		s.messageQueue.List = append(s.messageQueue.List[:foundIndex], s.messageQueue.List[foundIndex+1:]...) // 使用大写 L
	}
	s.messageQueue.Unlock()
	if foundIndex == -1 {
		if unauthorized {
			writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
			return
		}
		s.logger.Printf("尝试撤销未找到的消息 ID: %d", id)
		writeError(w, http.StatusNotFound, "message_not_found", "Message not found", "消息未找到")
		return
	}

	// 如果是文件消息，则删除文件并从 uploadFileMap 中移除
	if foundMsg.Data.Type() == "file" && foundMsg.Data.FileReceive != nil {
		uuid := foundMsg.Data.FileReceive.Cache
		s.runMutex.Lock() // 保护 uploadFileMap
		delete(s.uploadFileMap, uuid)
		s.runMutex.Unlock()

		filePath := filepath.Join(s.storageFolder, uuid)
		if err := os.Remove(filePath); err != nil {
			if !os.IsNotExist(err) {
				s.logger.Printf("警告: 撤销时删除文件 %s (UUID: %s) 失败: %v", filePath, uuid, err)
			}
		} else {
			s.logger.Printf("已删除与撤销消息关联的文件: %s (UUID: %s)", filePath, uuid)
		}
	}

	// 广播撤销事件
	revokeWsMsg := WebSocketMessage{
		Event: "revoke",
		Data:  map[string]int{"id": id}, // 前端期望的载荷
	}
	s.broadcastWebSocketMessage(revokeWsMsg, foundMsg.Data.Room()) // 使用新的广播函数
	s.saveHistoryData()
}

func (s *ClipboardServer) handleClearAll(w http.ResponseWriter, r *http.Request) {
	normalizedRoom := normalizeRoomName(r.URL.Query().Get("room"))
	if !s.canAccessRoom(normalizedRoom, extractAuthToken(r)) {
		writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
		return
	}

	s.logger.Printf("处理 /revoke/all 请求 (规范化后: '%s')", normalizedRoom)

	s.messageQueue.Lock()
	var newMsgList []PostEvent
	var revokedIDs []int

	// 始终只清空指定房间（规范化后的房间名），不再支持通过空字符串清空所有
	for _, msg := range s.messageQueue.List {
		if normalizeRoomName(msg.Data.Room()) != normalizedRoom {
			newMsgList = append(newMsgList, msg)
		} else {
			revokedIDs = append(revokedIDs, msg.Data.ID())
		}
	}
	s.messageQueue.List = newMsgList
	s.messageQueue.Unlock()

	// 删除关联的文件
	s.runMutex.Lock() // 保护 uploadFileMap
	var filesToRemove []string
	for uuid, fileInfo := range s.uploadFileMap {
		if normalizeRoomName(fileInfo.Room) == normalizedRoom {
			filesToRemove = append(filesToRemove, uuid)
			delete(s.uploadFileMap, uuid)
		}
	}
	s.runMutex.Unlock()

	for _, uuid := range filesToRemove {
		filePath := filepath.Join(s.storageFolder, uuid)
		if err := os.Remove(filePath); err != nil {
			if !os.IsNotExist(err) {
				s.logger.Printf("警告: 清理房间 %s 时删除文件 %s 失败: %v", normalizedRoom, filePath, err)
			}
		}
	}

	// 广播 clearAll 事件
	clearWsMsg := WebSocketMessage{
		Event: "clearAll",
		Data:  map[string]string{"room": normalizedRoom}, // 前端期望的载荷
	}
	s.broadcastWebSocketMessage(clearWsMsg, normalizedRoom) // 使用新的广播函数
	s.saveHistoryData()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "所有消息已清除")
}

// writeError 是所有错误响应的唯一出口：一律返回 JSON。
//
// 三个字段各有分工，别混用：
//
//	code    机器码，snake_case，给程序判断用（前端据此分支、测试据此断言）。
//	        一旦发布就不要改，改了等于破坏契约。
//	error   英文人话，给「会看英文的人」和日志用。与 Worker 侧保持同一风格。
//	message 中文人话，给人看。已分发的捷径、Android 快捷方式、前端都在展示它。
//
// 为什么需要它：Apple 快捷指令的「获取URL内容」**不暴露 HTTP 状态码**，只能读响应体，
// 而它用 getDictionary 解析。纯文本错误会让它解析不出字段，走到兜底分支；
// 更糟的是文件分支会拿错误文本去 setName + saveFilePrompt，
// **保存出一个顶着原文件名的假文件**。
//
// 为什么不再按 Accept 分叉（曾经的做法，已废弃）：捷径根本不发 Accept 头，
// 于是「文本超限」这类错误仍然是纯文本 —— 捷径读不到 error，把 413 误报成
// 「服务器未确认保存，请检查部署地址及服务器状态」，把人往部署/网络方向带。
// 同一状态码对应两种响应体形状，等于要求每个客户端各写两套解析逻辑。
// 统一成 JSON 后客户端只需要一套。
func writeError(w http.ResponseWriter, status int, code, errText, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"code":    code,
		"error":   errText,
		"message": message,
	})
}

// resolveContentFormat 读**显式**的格式信号：?format= > .json 后缀 > ?json=1。
//
// 为什么要有这个函数：同一件事以前有三种表达，谁优先、哪个算数只能靠读代码。
// 现在统一成 ?format= 优先，其余两个保留为兼容信号 —— 已发布的捷径走
// /content/latest.json、Android 端同样用后缀，一个字都不能改。
//
// 返回 "" 表示调用方没显式要格式，由分支自己决定（文本分支会再看 Accept 头，
// 文件分支不看 —— 见 wantsJSON 的注释）。
//
// 第二个返回值 false 表示 format 给了不认识的值（比如 ?format=html）：必须报错、
// 不能回落，否则客户端以为拿到 HTML、实际拿到原文。
func resolveContentFormat(r *http.Request, hasJSONSuffix bool) (string, bool) {
	if explicit := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))); explicit != "" {
		switch explicit {
		case "json":
			return "json", true
		case "raw", "text", "plain":
			return "raw", true
		default:
			return "", false
		}
	}
	if hasJSONSuffix {
		return "json", true
	}
	if v := r.URL.Query().Get("json"); v == "true" || v == "1" {
		return "json", true
	}
	return "", true
}

// wantsJSON 决定**文本**响应给不给 JSON：显式格式优先，没显式时才看 Accept 头。
//
// 文件分支不走这里。下载链路上的 Accept 头太不可靠（浏览器、下载器、脚本五花八门），
// 所以文件分支历来只认显式信号 —— 这是既有设计，别为了「统一」合并掉：
// 合并的后果是「浏览器直接点开文件链接」会突然收到一坨 JSON。
func wantsJSON(explicitFormat string, r *http.Request) bool {
	switch explicitFormat {
	case "json":
		return true
	case "raw":
		return false
	default:
		return strings.Contains(r.Header.Get("Accept"), "application/json")
	}
}

func (s *ClipboardServer) handleContent(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 { // 至少需要 "content" 和 id
		writeError(w, http.StatusBadRequest, "invalid_content_path", "Invalid content path", "无效的内容路径")
		return
	}

	// /content/<id>/column —— 看板把卡片挪到另一列。必须先分派：下面按「最后一段是 id」
	// 取 id，`/content/7/column` 会被当成 id="column" 然后 Atoi 失败。
	if len(parts) == 3 && parts[2] == "column" {
		s.handleContentColumn(w, r, parts[1])
		return
	}

	idStr := parts[len(parts)-1]

	// 检查是否是访问 "latest"，如果是，让专用处理函数处理
	if idStr == "latest" || idStr == "latest.json" {
		s.handleLatestContent(w, r)
		return
	}
	// 后缀无论格式如何都要剥掉：/content/999.json 的 id 就是 999，哪怕调用方
	// 用 ?format=raw 显式要原文。
	hasJSONSuffix := strings.HasSuffix(idStr, ".json")
	idStr = strings.TrimSuffix(idStr, ".json")

	explicitFormat, formatOK := resolveContentFormat(r, hasJSONSuffix)
	if !formatOK {
		writeError(w, http.StatusBadRequest, "unsupported_format", "Unsupported format", "不支持的格式（只支持 raw / json）")
		return
	}
	// 文件分支只看显式信号；文本分支还会看 Accept（见下方 wantsJSON）
	isJSONRequest := explicitFormat == "json"

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.logger.Printf("无效的内容 ID: %s, 错误: %v", idStr, err)
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content id", "无效的内容 ID")
		return
	}
	_, hasRequestedRoom := r.URL.Query()["room"]
	requestedRoom := normalizeRoomName(r.URL.Query().Get("room"))
	s.logger.Printf("处理内容请求, ID: %d, 房间参数存在: %t, JSON请求: %t", id, hasRequestedRoom, isJSONRequest)

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()
	unauthorized := false

	// 遍历消息列表寻找匹配的消息
	for _, msg := range s.messageQueue.List {
		// 检查ID是否匹配
		if msg.Data.ID() == id {
			messageRoom := normalizeRoomName(msg.Data.Room())
			if hasRequestedRoom && messageRoom != requestedRoom {
				continue
			}
			// 房间密码 或 针对该 content 的短期分享 token
			if !s.canAccessContent(r, messageRoom, id) {
				unauthorized = true
				continue
			}

			// 根据消息类型处理
			switch msg.Data.Type() {
			case "file":
				if msg.Data.FileReceive != nil {
					if isJSONRequest {
						// 返回JSON格式的文件信息
						fileReceive := msg.Data.FileReceive

						// 过期检查必须放在返回之前：否则客户端会拿着一条已过期的记录去下载，
						// 拿到 404 的错误文本，然后**存成一个顶着原文件名的假文件**。
						if fileReceive.Expire > 0 && fileReceive.Expire < time.Now().Unix() {
							s.logger.Printf("尝试访问已过期的文件: %s (ID: %d)", fileReceive.Name, id)
							writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
							return
						}

						responseType := DetermineResponseType(fileReceive.Name)

						responseData := map[string]interface{}{
							"type":      responseType,
							"name":      fileReceive.Name,
							"size":      fileReceive.Size,
							"uuid":      fileReceive.Cache,
							"url":       fileReceive.URL,
							"id":        strconv.Itoa(msg.Data.ID()),
							"timestamp": fileReceive.Timestamp,
							"expire":    fileReceive.Expire,
							// 空串 = 待办（看板列，见 handleContentColumn）
							"column": fileReceive.Column,
						}

						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(responseData)
						s.logger.Printf("以JSON格式返回文件信息, ID: %d", id)
						return
					}

					// 非 JSON 分支同样要拦过期：记录已声明过期，却还能把字节吐出去，
					// 浏览器直连 /content/<id> 就能绕过上面的检查拿到已过期文件。
					if msg.Data.FileReceive.Expire > 0 && msg.Data.FileReceive.Expire < time.Now().Unix() {
						s.logger.Printf("尝试访问已过期的文件: %s (ID: %d)", msg.Data.FileReceive.Name, id)
						writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
						return
					}

					filePath := filepath.Join(s.storageFolder, msg.Data.FileReceive.Cache)
					file, openErr := os.Open(filePath)
					if openErr != nil {
						s.logger.Printf("错误: 打开文件失败: %v", openErr)
						writeError(w, http.StatusNotFound, "file_expired", "File expired or cleaned up", "文件已过期或已被清理")
						return
					}
					defer file.Close()

					stat, statErr := file.Stat()
					if statErr != nil {
						s.logger.Printf("错误: 获取文件状态失败: %v", statErr)
						writeError(w, http.StatusInternalServerError, "file_stat_failed", "Cannot read file info", "无法读取文件状态")
						return
					}

					dispositionType := "inline"
					if r.URL.Query().Get("download") == "true" {
						dispositionType = "attachment"
					}
					w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", dispositionType, msg.Data.FileReceive.Name))
					http.ServeContent(w, r, msg.Data.FileReceive.Name, stat.ModTime(), file)
					return
				}
			case "text":
				if msg.Data.TextReceive != nil {
					// 文本分支：显式格式优先，没显式时才看 Accept（文件分支不看，见 wantsJSON）
					if wantsJSON(explicitFormat, r) {
						// JSON格式响应
						responseData := map[string]interface{}{
							"type":      "text",
							"content":   msg.Data.TextReceive.Content,
							"id":        strconv.Itoa(msg.Data.ID()),
							"timestamp": msg.Data.TextReceive.Timestamp,
							// 空串 = 待办（看板列，见 handleContentColumn）
							"column": msg.Data.TextReceive.Column,
						}

						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(responseData)
						s.logger.Printf("以JSON格式返回文本内容, ID: %d", id)
						return
					}

					// 默认返回纯文本
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					content := msg.Data.TextReceive.Content
					if !strings.HasSuffix(content, "\n") {
						content += "\n"
					}
					w.Write([]byte(content))
					s.logger.Printf("以纯文本格式返回文本内容, ID: %d", id)
					return
				}
			}
		}
	}

	// 鉴权失败优先于“未找到”，避免把缺密码/过期分享 token 误报成内容不存在
	if unauthorized {
		writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
		return
	}
	s.logger.Printf("未找到内容 ID: %d", id)
	writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "内容未找到")
}

func (s *ClipboardServer) handleLatestContent(w http.ResponseWriter, r *http.Request) {
	token := extractAuthToken(r)
	_, hasRequestedRoom := r.URL.Query()["room"]
	requestedRoom := normalizeRoomName(r.URL.Query().Get("room"))

	explicitFormat, formatOK := resolveContentFormat(r, strings.HasSuffix(r.URL.Path, "latest.json"))
	if !formatOK {
		writeError(w, http.StatusBadRequest, "unsupported_format", "Unsupported format", "不支持的格式（只支持 raw / json）")
		return
	}
	isJSONRequest := explicitFormat == "json"

	s.logger.Printf("处理最新内容请求 (房间参数存在: %t, JSON请求: %t)", hasRequestedRoom, isJSONRequest)

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	// 检查消息队列是否为空
	if len(s.messageQueue.List) == 0 {
		s.logger.Printf("没有可用的内容")
		writeError(w, http.StatusNotFound, "no_content", "No content available", "没有可用的内容")
		return
	}

	// 从后向前查找匹配房间的最新消息
	// latest 不支持分享 token（无稳定资源 id），仅房间密码
	unauthorized := false
	for i := len(s.messageQueue.List) - 1; i >= 0; i-- {
		msg := s.messageQueue.List[i]
		messageRoom := normalizeRoomName(msg.Data.Room())

		// 检查房间匹配 (空房间参数表示匹配任何房间)
		if hasRequestedRoom && messageRoom != requestedRoom {
			continue
		}
		if !s.canAccessRoom(messageRoom, token) {
			unauthorized = true
			continue
		}

		// 如果是JSON请求，始终以JSON格式返回
		if isJSONRequest {
			w.Header().Set("Content-Type", "application/json")

			var responseType string
			var responseData map[string]interface{}

			if msg.Data.Type() == "file" && msg.Data.FileReceive != nil {
				// 确定文件类型
				fileReceive := msg.Data.FileReceive

				// 过期检查必须放在返回之前：否则客户端会拿着一条已过期的记录去下载，
				// 拿到 404 的错误文本，然后**存成一个顶着原文件名的假文件**。
				if fileReceive.Expire > 0 && fileReceive.Expire < time.Now().Unix() {
					s.logger.Printf("尝试访问已过期的文件: %s (ID: %d)", fileReceive.Name, msg.Data.ID())
					writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
					return
				}

				responseType = DetermineResponseType(fileReceive.Name)

				// 构建JSON响应
				responseData = map[string]interface{}{
					"type": responseType,
					"name": fileReceive.Name,
					"size": fileReceive.Size,
					"uuid": fileReceive.Cache,
					// 不能用 filepath.Join：它内部会 Clean，把 "http://host" 里的双斜杠
					// 收成 "http:/host"，客户端拿到的 url 直接是坏的。
					"url":       fileReceive.URL + "/" + url.PathEscape(fileReceive.Name),
					"id":        strconv.Itoa(msg.Data.ID()),
					"timestamp": fileReceive.Timestamp,
					"expire":    fileReceive.Expire,
					// 空串 = 待办（看板列，见 handleContentColumn）
					"column": fileReceive.Column,
				}
			} else if msg.Data.Type() == "text" && msg.Data.TextReceive != nil {
				responseType = "text"
				responseData = map[string]interface{}{
					"type":      responseType,
					"content":   msg.Data.TextReceive.Content,
					"id":        strconv.Itoa(msg.Data.ID()),
					"timestamp": msg.Data.TextReceive.Timestamp,
					// 空串 = 待办（看板列，见 handleContentColumn）
					"column": msg.Data.TextReceive.Column,
				}
			} else {
				// 未知类型，提供基本信息
				responseType = "unknown"
				responseData = map[string]interface{}{
					"type":  responseType,
					"id":    strconv.Itoa(msg.Data.ID()),
					"error": "不支持的内容类型",
				}
			}

			json.NewEncoder(w).Encode(responseData)
			s.logger.Printf("以JSON格式返回最新内容 (类型: %s, 房间: '%s')", responseType, messageRoom)
			return
		}

		// 非JSON请求，按原有逻辑处理
		if msg.Data.Type() == "file" && msg.Data.FileReceive != nil {
			// 文件类型，直接提供文件内容而不是重定向
			// 与 JSON 分支保持一致：过期记录不能再吐出字节（浏览器直连走的就是这条路）。
			if msg.Data.FileReceive.Expire > 0 && msg.Data.FileReceive.Expire < time.Now().Unix() {
				s.logger.Printf("尝试访问已过期的文件: %s (ID: %d)", msg.Data.FileReceive.Name, msg.Data.ID())
				writeError(w, http.StatusNotFound, "file_expired", "File expired", "文件已过期")
				return
			}

			cacheUUID := msg.Data.FileReceive.Cache
			filename := msg.Data.FileReceive.Name

			// 构建文件路径
			filePath := filepath.Join(s.storageFolder, cacheUUID)

			file, err := os.Open(filePath)
			if err != nil {
				s.logger.Printf("错误: 打开文件失败: %v", err)
				writeError(w, http.StatusNotFound, "file_missing_on_disk", "File missing on disk", "文件在磁盘上未找到")
				return
			}
			defer file.Close()

			stat, err := file.Stat()
			if err != nil {
				s.logger.Printf("错误: 获取文件状态失败: %v", err)
				writeError(w, http.StatusInternalServerError, "file_stat_failed", "Cannot read file info", "无法获取文件状态")
				return
			}

			// 设置响应头，根据文件类型确定内容类型
			contentType := mime.TypeByExtension(filepath.Ext(filename))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			w.Header().Set("Content-Type", contentType)

			// 根据查询参数决定是否作为附件下载
			dispositionType := "inline" // 默认内联显示
			if r.URL.Query().Get("download") == "true" {
				dispositionType = "attachment"
			}
			disposition := fmt.Sprintf("%s; filename=%q", dispositionType, filename)
			w.Header().Set("Content-Disposition", disposition)

			// 提供文件内容
			s.logger.Printf("直接提供最新文件内容: %s", filename)
			http.ServeContent(w, r, filename, stat.ModTime(), file)
			return

		} else if msg.Data.Type() == "text" && msg.Data.TextReceive != nil {
			// 同上：文本分支显式优先，没显式才看 Accept
			if wantsJSON(explicitFormat, r) {
				// 客户端请求JSON格式
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(msg)
				s.logger.Printf("以JSON格式返回最新文本内容")
				return
			} else {
				// 默认返回纯文本
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				content := msg.Data.TextReceive.Content
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				w.Write([]byte(content))
				s.logger.Printf("以纯文本格式返回最新文本内容")
				return
			}
		}
	}

	if unauthorized {
		writeError(w, http.StatusUnauthorized, "room_forbidden", "No access to this room", "无权访问该房间")
		return
	}
	s.logger.Printf("未找到匹配的最新内容")
	writeError(w, http.StatusNotFound, "content_not_found", "Content not found", "未找到匹配的内容")
}

// handleRooms 处理房间列表请求
func (s *ClipboardServer) handleRooms(w http.ResponseWriter, r *http.Request) {
	// 添加 CORS 头
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Room-Auth-Tokens")

	// 处理预检请求
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed", "仅允许 GET 请求")
		return
	}

	// 检查是否启用房间列表功能
	if !s.config.Server.RoomList {
		writeError(w, http.StatusForbidden, "room_list_disabled", "Room list disabled", "房间列表功能未启用")
		return
	}

	s.logger.Printf("处理房间列表请求，来自: %s", get_remote_ip(r))

	roomList := s.getRoomList(extractAuthTokens(r))

	response := RoomListResponse{
		Rooms: roomList,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Printf("错误: 编码房间列表响应失败: %v", err)
		writeError(w, http.StatusInternalServerError, "encode_failed", "Failed to encode response", "编码响应失败")
		return
	}

	s.logger.Printf("返回房间列表，包含 %d 个房间", len(roomList))
}
