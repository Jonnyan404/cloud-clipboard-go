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
	"time"

	"github.com/gorilla/websocket"
)

func (s *ClipboardServer) handle_server(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("处理 /server 请求，来自: %s", get_remote_ip(r))
	authNeeded := false
	if authStr, ok := s.config.Server.Auth.(string); ok && authStr != "" {
		authNeeded = true
	} else if authBool, ok := s.config.Server.Auth.(bool); ok && authBool {
		// 如同 authMiddleware 中的注释，如果 auth: true 但没有密码，这是一种不明确状态。
		// 客户端可能需要知道是否需要认证。
		authNeeded = true
	}

	wsProtocol := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		wsProtocol = "wss"
	}

	response := map[string]interface{}{
		"server": fmt.Sprintf("%s://%s%s/push", wsProtocol, r.Host, s.config.Server.Prefix),
		"auth":   authNeeded,
		"config": map[string]interface{}{
			"server": map[string]interface{}{
				"roomList": s.config.Server.RoomList,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Printf("错误: 编码 /server 响应失败: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *ClipboardServer) handle_push(w http.ResponseWriter, r *http.Request) {
	ip := get_remote_ip(r)
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default" // 默认房间
	}
	s.logger.Printf("处理 /push WebSocket 连接请求，来自: %s, 房间: %s", ip, room)

	authNeeded := false
	var expectedPassword string
	// 从 s.config.Server.Auth 获取期望的密码（可能是随机生成的或配置的）
	if authStr, ok := s.config.Server.Auth.(string); ok && authStr != "" {
		authNeeded = true
		expectedPassword = authStr
	}
	// 注意：布尔型 true 的情况已在 NewClipboardServer 中处理并转换为字符串密码或空字符串

	if authNeeded {
		token := r.URL.Query().Get("auth")
		if expectedPassword == "" { // 这种情况理论上不应发生，因为 NewClipboardServer 会处理
			s.logger.Printf("WebSocket 认证失败: 服务器端未配置有效密码，但需要认证。来自 IP: %s, 房间: %s", ip, room)
			http.Error(w, "Unauthorized: Server authentication misconfiguration", http.StatusUnauthorized)
			return
		}
		if token == "" {
			s.logger.Printf("WebSocket 认证失败: 未提供 token。来自 IP: %s, 房间: %s", ip, room)
			http.Error(w, "Unauthorized: Missing token", http.StatusUnauthorized)
			return
		}
		if token != expectedPassword {
			s.logger.Printf("WebSocket 认证失败: 提供的 token '%s' 与期望的 '%s' 不匹配。来自 IP: %s, 房间: %s", token, expectedPassword, ip, room)
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}
		s.logger.Printf("WebSocket 认证成功。来自 IP: %s, 房间: %s", ip, room)
	}

	conn, err := upgrader.Upgrade(w, r, nil)
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
		Type:    clientUA.Device.Family,
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
			Prefix   string `json:"prefix"`
			RoomList bool   `json:"roomList"`
		}{
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
		http.Error(w, "文件未找到或已过期", http.StatusNotFound)
		return
	}

	// 检查文件是否已过期 (双重检查，因为 cleanExpiredFilesLoop 是异步的)
	if fileInfo.ExpireTime < time.Now().Unix() {
		s.logger.Printf("尝试访问已过期的文件: %s (UUID: %s)", fileInfo.Name, uuid)
		// 从 map 中移除并尝试删除文件
		s.runMutex.Lock()
		delete(s.uploadFileMap, uuid)
		s.runMutex.Unlock()
		go os.Remove(filepath.Join(s.storageFolder, uuid)) // 异步删除
		http.Error(w, "文件已过期", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(s.storageFolder, uuid)

	switch r.Method {
	case http.MethodGet:
		s.logger.Printf("提供文件下载: %s (UUID: %s), 路径: %s", fileInfo.Name, uuid, filePath)

		file, err := os.Open(filePath) // 打开文件以供 ServeContent 使用
		if err != nil {
			s.logger.Printf("错误: 打开文件失败: %v", err)
			http.Error(w, "文件在磁盘上未找到", http.StatusNotFound)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			s.logger.Printf("错误: 获取文件状态失败: %v", err)
			http.Error(w, "无法获取文件状态", http.StatusInternalServerError)
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
			http.Error(w, "删除文件失败", http.StatusInternalServerError)
			return
		}

		s.runMutex.Lock()
		delete(s.uploadFileMap, uuid)
		s.runMutex.Unlock()

		s.saveHistoryData()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "文件删除成功"})

	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

func (s *ClipboardServer) handle_text(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅允许 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Printf("错误: 读取 /text 请求体失败: %v", err)
		http.Error(w, "无法读取请求体", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	text := string(body)
	if s.config.Text.Limit > 0 && len(text) > s.config.Text.Limit {
		s.logger.Printf("错误: 文本内容超出限制 (%d > %d)", len(text), s.config.Text.Limit)
		http.Error(w, fmt.Sprintf("文本内容超出限制 (最大 %d 字符)", s.config.Text.Limit), http.StatusRequestEntityTooLarge)
		return
	}

	// 检查是否有 ID 参数用于覆盖
	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		// 尝试覆盖现有消息
		id, err := strconv.Atoi(idStr)
		if err != nil {
			s.logger.Printf("无效的 ID 参数: %s", idStr)
			http.Error(w, "无效的 ID 参数", http.StatusBadRequest)
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
			http.Error(w, "消息未找到或无法更新", http.StatusNotFound)
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
				s.messageQueue.List[i].Data.TextReceive.SenderDevice = s.parse_user_agent(r.UserAgent())

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

func (s *ClipboardServer) handle_upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅允许 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	// 获取请求路径和内容类型
	path := r.URL.Path
	contentType := r.Header.Get("Content-Type")
	s.logger.Printf("处理上传请求，路径: %s, 内容类型: %s, 来自: %s", path, contentType, get_remote_ip(r))

	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}

	// 处理 /upload/chunk 路径（文件名初始化请求）
	if strings.HasSuffix(path, "/upload/chunk") && contentType == "text/plain" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Printf("错误: 读取文件名失败: %v", err)
			http.Error(w, "无法读取请求体", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		filename := string(body)
		uuid := gen_UUID()
		s.logger.Printf("初始化分块上传: %s, 生成UUID: %s", filename, uuid)

		// 创建文件信息直接记录到 uploadFileMap 中
		expireTime := time.Now().Unix() + int64(s.config.File.Expire)
		s.runMutex.Lock()
		s.uploadFileMap[uuid] = File{
			Name:       filename,
			UUID:       uuid,
			Size:       0, // 初始大小为0
			ExpireTime: expireTime,
			UploadTime: time.Now().Unix(),
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
		http.Error(w, fmt.Sprintf("文件大小超出限制 (最大 %d 字节)", s.config.File.Limit), http.StatusRequestEntityTooLarge)
		return
	}

	err := r.ParseMultipartForm(int64(s.config.File.Limit)) // 使用文件大小限制作为 maxMemory
	if err != nil {
		s.logger.Printf("错误: 解析 multipart form 失败: %v", err)
		http.Error(w, "无法解析表单数据", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file") // "file" 是表单字段名
	if err != nil {
		s.logger.Printf("错误: 获取上传文件失败: %v", err)
		http.Error(w, "无法获取文件", http.StatusBadRequest)
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
		http.Error(w, "无法保存文件", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		s.logger.Printf("错误: 写入文件 %s 失败: %v", filePath, err)
		http.Error(w, "无法写入文件", http.StatusInternalServerError)
		return
	}

	timestamp := time.Now().Unix()
	expireTime := timestamp + int64(s.config.File.Expire)

	// 创建文件信息
	fileInfo := File{
		Name:       fileName,
		UUID:       uuid,
		Size:       fileSize,
		UploadTime: timestamp,
		ExpireTime: expireTime,
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
		http.Error(w, "仅允许 POST 请求", http.StatusMethodNotAllowed)
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
		http.Error(w, "无效的 UUID", http.StatusBadRequest)
		return
	}

	// 读取请求体中的数据
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Printf("错误: 读取分块数据失败: %v", err)
		http.Error(w, "无法读取分块数据", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// 更新文件大小
	newSize := fileInfo.Size + int64(len(data))
	s.logger.Printf("上传分块数据大小: %d, 累计大小: %d", len(data), newSize)

	// 检查文件大小是否超过限制
	if s.config.File.Limit > 0 && newSize > int64(s.config.File.Limit) {
		s.logger.Printf("错误: 文件大小已超过限制 (%d > %d)", newSize, s.config.File.Limit)
		http.Error(w, fmt.Sprintf("文件大小已超过限制 (最大 %d 字节)", s.config.File.Limit), http.StatusRequestEntityTooLarge)
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
		http.Error(w, "无法打开文件", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		s.logger.Printf("错误: 写入数据到文件 %s 失败: %v", filePath, err)
		http.Error(w, "无法写入文件", http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

func (s *ClipboardServer) handle_finish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅允许 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	// 从路径中提取 UUID
	uuid := strings.TrimPrefix(r.URL.Path, s.config.Server.Prefix+"/upload/finish/")
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}

	s.logger.Printf("处理上传完成请求, UUID: %s, 房间: %s, 来自: %s", uuid, room, get_remote_ip(r))

	s.runMutex.Lock()
	fileInfo, ok := s.uploadFileMap[uuid]
	s.runMutex.Unlock()

	if !ok {
		s.logger.Printf("错误: 无效的 UUID: %s", uuid)
		http.Error(w, "无效的 UUID", http.StatusBadRequest)
		return
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
			SenderDevice: s.parse_user_agent(r.UserAgent()),
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
		http.Error(w, "无效的撤销路径", http.StatusBadRequest)
		return
	}
	idStr := parts[len(parts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "无效的撤销 ID", http.StatusBadRequest)
		return
	}

	room := r.URL.Query().Get("room") // 撤销也可能需要房间上下文

	s.messageQueue.Lock()
	var foundMsg *PostEvent // 指向 PostEvent
	foundIndex := -1

	for i := range s.messageQueue.List { // 使用大写 L
		// 假设 PostEvent 的 ID 是通过其 Data 字段的 ID() 方法访问的
		if s.messageQueue.List[i].Data.ID() == id {
			// 检查房间匹配
			if room == "" || s.messageQueue.List[i].Data.Room() == "" || s.messageQueue.List[i].Data.Room() == room {
				foundMsg = &s.messageQueue.List[i]
				foundIndex = i
				break
			}
		}
	}
	// ...
	if foundMsg != nil {
		// 从消息队列中移除
		s.messageQueue.List = append(s.messageQueue.List[:foundIndex], s.messageQueue.List[foundIndex+1:]...) // 使用大写 L
	}
	s.messageQueue.Unlock()
	// ...
	if foundMsg == nil {
		s.logger.Printf("尝试撤销未找到的消息 ID: %d (房间: '%s')", id, room)
		http.Error(w, "消息未找到", http.StatusNotFound)
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
	s.broadcastWebSocketMessage(revokeWsMsg, room) // 使用新的广播函数
	s.saveHistoryData()
}

func (s *ClipboardServer) handleClearAll(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")
	normalizedRoom := normalizeRoomName(room) // 应用规范化：空字符串 -> "default"

	s.logger.Printf("处理 /revoke/all 请求 (房间: '%s', 规范化后: '%s')", room, normalizedRoom)

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
	if room == "" { // 清空所有文件
		for uuid := range s.uploadFileMap {
			filesToRemove = append(filesToRemove, uuid)
		}
		s.uploadFileMap = make(map[string]File) // 清空 map
	} else { // 只清空指定房间的文件 (需要消息中有房间信息来判断)
		// 这个逻辑比较复杂，因为 uploadFileMap 本身不直接关联房间。
		// 需要遍历原始消息（在它们被清除之前）来确定哪些文件属于该房间。
		// 或者，如果 PostEvent 中记录了文件UUID，可以在清除消息时收集这些UUID。
		// 简单起见，如果按房间清除，我们目前只清除消息，文件由过期机制处理。
		// 一个更完善的实现会跟踪与房间关联的文件。
		// 或者，在清除消息时，如果消息是文件类型且属于该房间，则记录其UUID并删除。
		// 这里我们假设，如果按房间清除，文件暂时不主动删除，依赖过期。
		// 如果是全局清除，则删除所有文件。
		if room == "" {
			for _, uuid := range filesToRemove {
				filePath := filepath.Join(s.storageFolder, uuid)
				if err := os.Remove(filePath); err != nil {
					if !os.IsNotExist(err) {
						s.logger.Printf("警告: 清除所有时删除文件 %s 失败: %v", filePath, err)
					}
				}
			}
		}
	}
	s.runMutex.Unlock()

	// 广播 clearAll 事件
	clearWsMsg := WebSocketMessage{
		Event: "clearAll",
		Data:  map[string]string{"room": room}, // 前端期望的载荷
	}
	s.broadcastWebSocketMessage(clearWsMsg, room) // 使用新的广播函数
	s.saveHistoryData()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "所有消息已清除")
}

func (s *ClipboardServer) handleContent(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 { // 至少需要 "content" 和 id
		http.Error(w, "无效的内容路径", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-1]

	// 检查是否是访问 "latest"，如果是，让专用处理函数处理
	if idStr == "latest" || idStr == "latest.json" {
		s.handleLatestContent(w, r)
		return
	}
	// 检查是否请求 JSON 格式的响应
	// 1. 通过 URL 后缀判断
	isJSONRequest := strings.HasSuffix(idStr, ".json")
	// 如果 idStr 带有 .json 后缀，需要去除后缀再转换为整数
	if isJSONRequest {
		idStr = strings.TrimSuffix(idStr, ".json")
	}
	// 2. 通过查询参数判断 (json=true 或 json=1)
	jsonParam := r.URL.Query().Get("json")
	if jsonParam == "true" || jsonParam == "1" {
		isJSONRequest = true
	}
	// 3. 通过 Accept 头判断 (会在特定情况下检查)

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.logger.Printf("无效的内容 ID: %s, 错误: %v", idStr, err)
		http.Error(w, "无效的内容 ID", http.StatusBadRequest)
		return
	}
	room := r.URL.Query().Get("room") // 可选的房间参数
	s.logger.Printf("处理内容请求, ID: %d, 房间: '%s', JSON请求: %t", id, room, isJSONRequest)

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	// 遍历消息列表寻找匹配的消息
	for _, msg := range s.messageQueue.List {
		// 检查ID是否匹配
		if msg.Data.ID() == id {
			// 检查房间是否匹配（如果指定了房间）
			if room == "" || msg.Data.Room() == "" || msg.Data.Room() == room {
				// 根据消息类型处理
				switch msg.Data.Type() {
				case "file":
					if msg.Data.FileReceive != nil {
						if isJSONRequest {
							// 返回JSON格式的文件信息
							fileReceive := msg.Data.FileReceive
							responseType := DetermineResponseType(fileReceive.Name)

							responseData := map[string]interface{}{
								"type":      responseType,
								"name":      fileReceive.Name,
								"size":      fileReceive.Size,
								"uuid":      fileReceive.Cache,
								"url":       fileReceive.URL,
								"id":        strconv.Itoa(msg.Data.ID()),
								"timestamp": fileReceive.Timestamp,
							}

							w.Header().Set("Content-Type", "application/json")
							json.NewEncoder(w).Encode(responseData)
							s.logger.Printf("以JSON格式返回文件信息, ID: %d", id)
							return
						} else {
							// 文件类型，重定向到文件URL
							cacheUUID := msg.Data.FileReceive.Cache
							filename := msg.Data.FileReceive.Name
							scheme := getScheme(r)
							encodedFilename := url.PathEscape(filename)

							fileURL := fmt.Sprintf("%s://%s%s/file/%s/%s",
								scheme,
								r.Host,
								s.config.Server.Prefix,
								cacheUUID,
								encodedFilename,
							)
							s.logger.Printf("找到文件内容, 重定向到: %s", fileURL)
							http.Redirect(w, r, fileURL, http.StatusFound)
							return
						}
					}
				case "text":
					if msg.Data.TextReceive != nil {
						// 返回格式判断优先级：1. isJSONRequest参数 2. Accept头
						if isJSONRequest || strings.Contains(r.Header.Get("Accept"), "application/json") {
							// JSON格式响应
							responseData := map[string]interface{}{
								"type":      "text",
								"content":   msg.Data.TextReceive.Content,
								"id":        strconv.Itoa(msg.Data.ID()),
								"timestamp": msg.Data.TextReceive.Timestamp,
							}

							w.Header().Set("Content-Type", "application/json")
							json.NewEncoder(w).Encode(responseData)
							s.logger.Printf("以JSON格式返回文本内容, ID: %d", id)
							return
						} else {
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
		}
	}

	// 内容未找到时的响应格式也遵循JSON请求参数
	s.logger.Printf("未找到内容 ID: %d (房间: '%s')", id, room)
	if isJSONRequest {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "内容未找到"})
	} else {
		http.Error(w, "内容未找到", http.StatusNotFound)
	}
}

func (s *ClipboardServer) handleLatestContent(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")

	// // 检查是否是 latest.json 请求
	isJSONRequest := strings.HasSuffix(r.URL.Path, "latest.json")
	jsonParam := r.URL.Query().Get("json")
	if jsonParam == "true" || jsonParam == "1" {
		isJSONRequest = true
	}

	s.logger.Printf("处理最新内容请求 (房间: '%s', JSON请求: %t)", room, isJSONRequest)

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	// 检查消息队列是否为空
	if len(s.messageQueue.List) == 0 {
		s.logger.Printf("没有可用的内容 (房间: '%s')", room)
		if isJSONRequest {
			// 如果是JSON请求，返回JSON格式的404响应
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "内容未找到"})
		} else {
			// 普通请求，返回普通404
			http.Error(w, "没有可用的内容", http.StatusNotFound)
		}
		return
	}

	// 从后向前查找匹配房间的最新消息
	for i := len(s.messageQueue.List) - 1; i >= 0; i-- {
		msg := s.messageQueue.List[i]

		// 检查房间匹配 (空房间参数表示匹配任何房间)
		if room != "" && msg.Data.Room() != "" && msg.Data.Room() != room {
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
				responseType = DetermineResponseType(fileReceive.Name)

				// 构建JSON响应
				responseData = map[string]interface{}{
					"type":      responseType,
					"name":      fileReceive.Name,
					"size":      fileReceive.Size,
					"uuid":      fileReceive.Cache,
					"url":       filepath.Join(fileReceive.URL, fileReceive.Name),
					"id":        strconv.Itoa(msg.Data.ID()),
					"timestamp": fileReceive.Timestamp,
				}
			} else if msg.Data.Type() == "text" && msg.Data.TextReceive != nil {
				responseType = "text"
				responseData = map[string]interface{}{
					"type":      responseType,
					"content":   msg.Data.TextReceive.Content,
					"id":        strconv.Itoa(msg.Data.ID()),
					"timestamp": msg.Data.TextReceive.Timestamp,
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
			s.logger.Printf("以JSON格式返回最新内容 (类型: %s, 房间: '%s')", responseType, room)
			return
		}

		// 非JSON请求，按原有逻辑处理
		if msg.Data.Type() == "file" && msg.Data.FileReceive != nil {
			// 文件类型，直接提供文件内容而不是重定向
			cacheUUID := msg.Data.FileReceive.Cache
			filename := msg.Data.FileReceive.Name

			// 构建文件路径
			filePath := filepath.Join(s.storageFolder, cacheUUID)

			file, err := os.Open(filePath)
			if err != nil {
				s.logger.Printf("错误: 打开文件失败: %v", err)
				http.Error(w, "文件在磁盘上未找到", http.StatusNotFound)
				return
			}
			defer file.Close()

			stat, err := file.Stat()
			if err != nil {
				s.logger.Printf("错误: 获取文件状态失败: %v", err)
				http.Error(w, "无法获取文件状态", http.StatusInternalServerError)
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
			// 文本类型，检查Accept头决定是否返回JSON
			acceptHeader := r.Header.Get("Accept")
			if strings.Contains(acceptHeader, "application/json") {
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

	s.logger.Printf("未找到匹配的最新内容 (房间: '%s')", room)
	if isJSONRequest {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "内容未找到"})
	} else {
		http.Error(w, "未找到匹配的内容", http.StatusNotFound)
	}
}

// handleRooms 处理房间列表请求
func (s *ClipboardServer) handleRooms(w http.ResponseWriter, r *http.Request) {
	// 添加 CORS 头
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// 处理预检请求
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "仅允许 GET 请求", http.StatusMethodNotAllowed)
		return
	}

	// 检查是否启用房间列表功能
	if !s.config.Server.RoomList {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Forbidden",
			"message": "房间列表功能未启用",
		})
		return
	}

	s.logger.Printf("处理房间列表请求，来自: %s", get_remote_ip(r))

	roomList := s.getRoomList()

	response := RoomListResponse{
		Rooms: roomList,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Printf("错误: 编码房间列表响应失败: %v", err)
		http.Error(w, "编码响应失败", http.StatusInternalServerError)
		return
	}

	s.logger.Printf("返回房间列表，包含 %d 个房间", len(roomList))
}
