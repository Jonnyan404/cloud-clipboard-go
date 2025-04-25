package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log" // 导入 mime 包用于 Content-Type
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaolacci/murmur3"
	"github.com/ua-parser/uap-go/uaparser"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	deviceConnected = make(map[string]string)
)

var server_version = "go verion by Jonnyan404"
var build_git_hash = show_bin_info()
var config = load_config(config_path) // run before main()

var websockets = make(map[*websocket.Conn]bool)
var room_ws = make(map[*websocket.Conn]string)

var deviceHashSeed = murmur3.Sum32(random_bytes(32)) & 0xffffffff

// --------------- structs
type EventMsg struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Helper function to get IP (you might already have this or similar in auth.go)
func get_remote_ip(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		remoteAddr := r.RemoteAddr
		host, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = remoteAddr // Fallback if SplitHostPort fails
		}
	}
	// Handle potential multiple IPs in X-Forwarded-For
	ips := strings.Split(ip, ",")
	if len(ips) > 0 {
		ip = strings.TrimSpace(ips[0])
	}
	return ip
}

// Helper function to parse User-Agent (similar to ws_send_devices)
func parse_user_agent(uaString string) map[string]string {
	parser := uaparser.NewFromSaved() // Consider creating parser once globally
	client := parser.Parse(uaString)
	return map[string]string{
		"type":    client.Device.Family,                                                  // e.g., "Mac", "iPhone", "Other"
		"os":      fmt.Sprintf("%s %s", client.Os.Family, client.Os.Major),               // e.g., "Mac OS X 10", "iOS 15"
		"browser": fmt.Sprintf("%s %s", client.UserAgent.Family, client.UserAgent.Major), // e.g., "Chrome 100"
	}
}

// --------------- route handles
func handle_server(w http.ResponseWriter, r *http.Request) {
	need_auth := false
	if config.Server.Auth != "" && config.Server.Auth != false {
		need_auth = true
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"server": fmt.Sprintf("ws://%s%s/push", r.Host, config.Server.Prefix),
		"auth":   need_auth,
	})
}

func handle_text(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	room := r.URL.Query().Get("room")
	if len(body) > config.Text.Limit {
		http.Error(w, "文本长度不能超过 1MB", http.StatusBadRequest)
		return
	}
	bodyStr := string(body)

	// html encode & < > " '
	bodyStr = html.EscapeString(bodyStr)

	// --- Populate new fields ---
	senderIP := get_remote_ip(r)
	senderDevice := parse_user_agent(r.Header.Get("User-Agent"))
	timestamp := time.Now().Unix()
	// --- End populate ---

	message := PostEvent{
		Event: "receive",
		Data: ReceiveHolder{
			TextReceive: &TextReceive{
				ReceiveBase: ReceiveBase{ // Populate base struct
					// ID will be set by Append
					Type:         "text",
					Room:         room,
					Timestamp:    timestamp,
					SenderIP:     senderIP,
					SenderDevice: senderDevice,
				},
				Content: bodyStr,
			},
		},
	}
	messageQueue.Append(&message) // Append will set the ID
	fmt.Printf("DEBUG: Sending message data: %+v\n", message.Data)
	messageJSON, err := json.Marshal(message)
	if err != nil {
		http.Error(w, "无法编码消息", http.StatusInternalServerError)
		return
	}
	messageStr := string(messageJSON)
	broadcast_ws_msg(websockets, messageStr, room)
	save_history()
	// Enhance response to include URL (as done by enhanceHandleText)
	nextID := message.Data.ID() // Get the ID assigned by Append
	scheme := getScheme(r)
	contentURL := fmt.Sprintf("%s://%s%s/content/%d", scheme, r.Host, config.Server.Prefix, nextID)
	if room != "" {
		contentURL += fmt.Sprintf("?room=%s", room)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":   200,
		"msg":    "",
		"result": map[string]interface{}{"url": contentURL},
	})
}

// 修改 handle_upload 函数处理表单上传
func handle_upload(w http.ResponseWriter, r *http.Request) {
	// 检查是否是表单上传
	contentType := r.Header.Get("Content-Type")

	// 如果是表单上传（multipart/form-data）
	if strings.Contains(contentType, "multipart/form-data") {
		fmt.Println("处理表单文件上传")

		// 解析表单
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB
			http.Error(w, "无法解析表单", http.StatusBadRequest)
			return
		}

		// 获取上传的文件
		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "无法获取文件", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// 获取房间参数
		room := r.URL.Query().Get("room")

		// 生成UUID
		uuid := gen_UUID()

		// 确保上传目录存在
		mkdir_uploads()

		// 保存文件
		filePath := filepath.Join(storage_folder, uuid)
		outFile, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "无法创建文件", http.StatusInternalServerError)
			return
		}
		defer outFile.Close()

		written, err := io.Copy(outFile, file)
		if err != nil {
			http.Error(w, "无法保存文件", http.StatusInternalServerError)
			os.Remove(filePath)
			return
		}

		// --- Populate new fields ---
		senderIP := get_remote_ip(r)
		senderDevice := parse_user_agent(r.Header.Get("User-Agent"))
		timestamp := time.Now().Unix()
		// --- End populate ---

		// 创建文件信息
		fileInfo := File{
			Name:       fileHeader.Filename,
			UUID:       uuid,
			Size:       int(written),
			UploadTime: timestamp, // Use the same timestamp
			ExpireTime: timestamp + int64(config.File.Expire),
		}
		uploadFileMap[uuid] = fileInfo

		// 获取下一个消息ID
		nextID := messageQueue.nextid

		// 创建文件消息
		message := PostEvent{
			Event: "receive",
			Data: ReceiveHolder{
				FileReceive: &FileReceive{
					ReceiveBase: ReceiveBase{ // Populate base struct
						// ID will be set by Append
						Type:         "file",
						Room:         room,
						Timestamp:    timestamp,
						SenderIP:     senderIP,
						SenderDevice: senderDevice,
					},
					Name:   fileInfo.Name,
					Size:   fileInfo.Size,
					Cache:  fileInfo.UUID,
					Expire: fileInfo.ExpireTime,
				},
			},
		}

		// 如果文件不太大，创建缩略图
		if written <= 32*1024*1024 { // 32MB
			thumbnail, err := gen_thumbnail(filePath)
			if err == nil {
				message.Data.FileReceive.Thumbnail = thumbnail
			}
		}

		// 添加到消息队列
		messageQueue.Append(&message)

		// 广播消息
		messageJSON, err := json.Marshal(message)
		if err == nil {
			broadcast_ws_msg(websockets, string(messageJSON), room)
		}

		// 保存历史记录
		save_history()

		// 构造内容访问URL
		scheme := getScheme(r)
		contentURL := fmt.Sprintf("%s://%s%s/content/%d",
			scheme, r.Host, config.Server.Prefix, nextID)
		if room != "" {
			contentURL += fmt.Sprintf("?room=%s", room)
		}

		// 返回与Node.js版本兼容的URL响应
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":   200,
			"msg":    "",
			"result": map[string]interface{}{"url": contentURL},
		})

		return
	}

	// 否则处理文件名上传（初始化大文件上传）
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "无法读取请求体", http.StatusBadRequest)
		return
	}
	filename := string(body)

	uuid := gen_UUID()

	fileInfo := File{
		Name:       filename,
		UUID:       uuid,
		Size:       0,
		UploadTime: time.Now().Unix(),
		ExpireTime: time.Now().Unix() + int64(config.File.Expire),
	}
	uploadFileMap[uuid] = fileInfo

	// 返回UUID响应（用于分片上传）
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":   200,
		"msg":    "",
		"result": map[string]interface{}{"uuid": uuid},
	})
}

// save file & update fileEntry
func handle_chunk(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/upload/chunk/")
	fmt.Println("uuid:", uuid)
	if _, ok := uploadFileMap[uuid]; !ok {
		http.Error(w, "无效的 UUID", http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(r.Body)
	fileInfo := uploadFileMap[uuid]
	fileInfo.Size += len(data)
	uploadFileMap[uuid] = fileInfo

	// if fileInfo.Size > 10 {
	if fileInfo.Size > config.File.Limit {
		http.Error(w, "文件大小已超过限制", http.StatusBadRequest)
		return
	}

	file, _ := os.OpenFile(filepath.Join(storage_folder, uuid), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer file.Close()
	file.Write(data)
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

// finish fileEntry & broadcast
func handle_finish(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/upload/finish/")
	room := r.URL.Query().Get("room")
	if _, ok := uploadFileMap[uuid]; !ok {
		http.Error(w, "无效的 UUID", http.StatusBadRequest)
		return
	}
	fileInfo := uploadFileMap[uuid]

	// --- Populate new fields ---
	senderIP := get_remote_ip(r)
	senderDevice := parse_user_agent(r.Header.Get("User-Agent"))
	timestamp := time.Now().Unix() // Use finish time as the message time
	// Update expire time based on finish time if desired, or keep original
	// fileInfo.ExpireTime = timestamp + int64(config.File.Expire)
	// uploadFileMap[uuid] = fileInfo // Update map if ExpireTime changed
	// --- End populate ---

	message := PostEvent{
		Event: "receive",
		Data: ReceiveHolder{
			FileReceive: &FileReceive{
				ReceiveBase: ReceiveBase{ // Populate base struct
					// ID will be set by Append
					Type:         "file",
					Room:         room,
					Timestamp:    timestamp,
					SenderIP:     senderIP,
					SenderDevice: senderDevice,
				},
				Name:   fileInfo.Name,
				Size:   fileInfo.Size,
				Cache:  fileInfo.UUID,
				Expire: fileInfo.ExpireTime,
			},
		},
	}

	if fileInfo.Size <= 32*_MB {
		thumbnail, _ := gen_thumbnail(filepath.Join(storage_folder, uuid))
		message.Data.FileReceive.Thumbnail = thumbnail
	}

	messageQueue.Append(&message)
	messageJSON, err := json.Marshal(message)
	if err != nil {
		http.Error(w, "无法编码消息", http.StatusInternalServerError)
		return
	}
	messageStr := string(messageJSON)
	fmt.Println("")
	broadcast_ws_msg(websockets, messageStr, room)
	save_history()
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

func handle_push(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "-- ws conn fail", http.StatusInternalServerError)
		return
	}

	defer ws.Close()
	room_ws[ws] = room
	websockets[ws] = true
	ws.SetCloseHandler(func(code int, text string) error {
		delete(websockets, ws)
		delete(room_ws, ws)
		return nil
	})
	// remoteAddr := ws.RemoteAddr().String()
	ua := get_UA(r)
	ip, port := get_remote(r)
	remoteAddr := ip + ":" + port
	// fmt.Println("\n----- new conn:", ip, port, room)
	fmt.Println("\n----- new conn:", remoteAddr, room)

	auth := r.URL.Query().Get("auth")
	fmt.Println("---auth:", auth, config.Server.Auth)
	if auth != config.Server.Auth {
		forbid := `{"event":"forbidden","data":{}}`
		fmt.Println("---forbid:", "\033[37;41m", fmt.Sprintf("%-21s", remoteAddr), ua, "\033[0m")
		ws.WriteMessage(websocket.TextMessage, []byte(forbid))
		return
	}

	type ConfigData struct {
		Version string `json:"version"`
		Text    struct {
			Limit int `json:"limit"`
		} `json:"text"`
		File struct {
			Expire int `json:"expire"`
			Chunk  int `json:"chunk"`
			Limit  int `json:"limit"`
		} `json:"file"`
	}
	// type ConfigEvent EventMsg
	// config_event := ConfigEvent{
	config_event := EventMsg{
		Event: "config",
		Data: ConfigData{
			Version: server_version,
			Text:    config.Text,
			File:    config.File,
		},
	}

	config_event_json, _ := json.Marshal(config_event)
	ws.WriteMessage(websocket.TextMessage, config_event_json)

	ws_send_history(ws, room)
	deviceID, room := ws_send_devices(r, ws)

	for { //--- msg loop, recv no action
		_, _, err := ws.ReadMessage()
		if err != nil { //--- ws disconn
			disconn_event := EventMsg{
				Event: "disconnect",
				Data: map[string]interface{}{
					"id": deviceID,
				},
			}
			disconn_event_json, _ := json.Marshal(disconn_event)
			// broadcast_ws_msg(websockets, fmt.Sprintf(`{"event":"disconnect","data":{"id":"%s"}}`, deviceID), room)
			broadcast_ws_msg(websockets, string(disconn_event_json), room)
			delete(deviceConnected, deviceID)
			delete(websockets, ws)
			delete(room_ws, ws)
			break
		}
	}
}

func handle_file(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/file/")
	fmt.Println("==file request:", uuid, r.Method)

	switch r.Method {
	case http.MethodGet:
		fmt.Println("==get file:", uuid)
		fileInfo, ok := uploadFileMap[uuid] // 从 map 获取文件信息
		if !ok {
			http.Error(w, "File not found in map", http.StatusNotFound)
			return
		}

		filePath := filepath.Join(storage_folder, uuid)
		file, err := os.Open(filePath) // 打开文件以供 ServeContent 使用
		if err != nil {
			http.Error(w, "File not found on disk", http.StatusNotFound)
			// 可选：如果磁盘上没有但 map 中有，清理 map
			// delete(uploadFileMap, uuid)
			// save_history()
			return
		}
		defer file.Close()

		// 获取文件状态信息，主要是为了获取 ModTime
		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Cannot get file stat", http.StatusInternalServerError)
			return
		}

		// --- 设置 Content-Disposition 头 ---
		// 使用 fmt.Sprintf("%q", ...) 来为文件名添加必要的引号并转义特殊字符
		disposition := fmt.Sprintf("attachment; filename=%q", fileInfo.Name)
		w.Header().Set("Content-Disposition", disposition)
		// --- 结束设置 ---

		// 可选：手动设置 Content-Type，如果 ServeContent 的自动检测不够准确
		// contentType := mime.TypeByExtension(filepath.Ext(fileInfo.Name))
		// if contentType != "" {
		//     w.Header().Set("Content-Type", contentType)
		// }

		// 使用 http.ServeContent 提供文件内容
		// 它需要文件名（用于 Content-Type 检测）、修改时间 和 io.ReadSeeker
		http.ServeContent(w, r, fileInfo.Name, stat.ModTime(), file)

	case http.MethodDelete:
		// ... (DELETE 逻辑保持不变) ...
		fmt.Println("==del file:", uuid)
		if _, ok := uploadFileMap[uuid]; !ok {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		fmt.Println("-- rm file:", filepath.Join(storage_folder, uuid))
		os.Remove(filepath.Join(storage_folder, uuid))
		delete(uploadFileMap, uuid)
		save_history()
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "File deleted successfully"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handle_revoke(w http.ResponseWriter, r *http.Request) {
	messageIDStr := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/revoke/")
	messageID, err := strconv.Atoi(messageIDStr)
	room := r.URL.Query().Get("room")
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	idx := messageQueue.RemoveById(messageID)
	if idx < 0 {
		http.Error(w, "不存在的消息 ID", http.StatusBadRequest)
		return
	}

	revokeMessage := EventMsg{
		Event: "revoke",
		Data: map[string]interface{}{
			"id":   messageID,
			"room": room,
		},
	}

	revokeMessageJSON, err := json.Marshal(revokeMessage)
	if err != nil {
		http.Error(w, "Failed to marshal revoke message", http.StatusInternalServerError)
		return
	}

	broadcast_ws_msg(websockets, string(revokeMessageJSON), room)
	save_history()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

func show_bin_info() string {
	buildInfo, ok := debug.ReadBuildInfo()
	var gitHash string

	if !ok {
		// log.Fatal("Failed to read build info")
	} else {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				gitHash = setting.Value
				break
			}
		}

		if len(gitHash) > 7 {
			gitHash = gitHash[:7]
		}
	}

	// fmt.Println("== cloud-clip: ", server_version)
	fmt.Printf("== \033[07m cloud-clip \033[36m %s \033[0m     \033[35m %s  %s     %s\033[0m\n",
		server_version, gitHash, buildInfo.GoVersion, buildInfo.Main.Version)

	return gitHash
}

// make sure uploads exist
func mkdir_uploads() {
	uploadsDir := "uploads"
	if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
		err := os.MkdirAll(uploadsDir, 0755)
		if err != nil {
			log.Fatalf("Failed to create uploads directory: %v", err)
		}
		log.Println("++ uploads directory Created")
	} else {
		fmt.Println("== uploads directory Exists")
	}
}

// 清空所有消息的处理函数
func handleClearAll(w http.ResponseWriter, r *http.Request) {
	// 只允许 DELETE 方法
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取房间参数，但不要求必须有
	room := r.URL.Query().Get("room")

	// 清空消息队列
	messageQueue.ClearAll()

	// 构造清空消息事件
	clearAllMessage := EventMsg{
		Event: "clear",
		Data: map[string]interface{}{
			"room": room, // 即使为空也传递房间参数
			"time": time.Now().Unix(),
		},
	}

	clearAllMessageJSON, err := json.Marshal(clearAllMessage)
	if err != nil {
		http.Error(w, "Failed to marshal clear message", http.StatusInternalServerError)
		return
	}

	// 广播清空消息，即使 room 为空也能广播给所有客户端
	broadcast_ws_msg(websockets, string(clearAllMessageJSON), room)

	// 保存历史记录
	save_history()

	// 如果有文件存储相关的内容，可以同时清理
	// 这里根据您的实际情况添加清理文件存储的代码

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "All messages cleared",
	})
}

func main() {
	// 解析命令行参数，加载配置等
	load_history()
	mkdir_uploads()
	config = load_config(*flg_config)
	// 应用命令行参数，覆盖配置文件中的设置
	applyCommandLineArgs()
	prefix := config.Server.Prefix

	// 服务静态文件
	server_static(prefix)

	// 注册不需要认证的路由
	http.HandleFunc(prefix+"/server", handle_server)
	http.HandleFunc(prefix+"/push", handle_push)

	// 处理GET文件请求（不需要认证）和DELETE文件请求（需要认证）
	http.HandleFunc(prefix+"/file/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handle_file(w, r)
		} else {
			// 需要认证的操作
			authMiddleware(handle_file)(w, r)
		}
	})

	// 需要认证的路由
	http.HandleFunc(prefix+"/text", authMiddleware(handle_text))
	http.HandleFunc(prefix+"/upload", authMiddleware(handle_upload))
	http.HandleFunc(prefix+"/upload/chunk", authMiddleware(handle_upload))
	http.HandleFunc(prefix+"/upload/chunk/", authMiddleware(handle_chunk))
	http.HandleFunc(prefix+"/upload/finish/", authMiddleware(handle_finish))
	http.HandleFunc(prefix+"/revoke/", authMiddleware(handle_revoke))
	http.HandleFunc(prefix+"/revoke/all", authMiddleware(handleClearAll))

	// 注册内容访问路由
	http.HandleFunc(config.Server.Prefix+"/content/", authMiddleware(handleContent))

	// 添加最新内容路由
	http.HandleFunc(config.Server.Prefix+"/content/latest", authMiddleware(handleLatestContent))

	// 过期文件清理
	go clean_expire_files()

	// 修改此部分以支持多地址监听
	var hostList []string

	// 从配置中获取主机地址列表
	switch host := config.Server.Host.(type) {
	case string:
		// 单个地址
		hostList = []string{host}
	case []interface{}:
		// 地址数组
		for _, h := range host {
			if hostStr, ok := h.(string); ok {
				hostList = append(hostList, hostStr)
			}
		}
	case []string:
		// 已经是字符串数组
		hostList = host
	default:
		// 未知类型，使用默认值
		hostList = []string{"0.0.0.0"}
	}

	// 确保至少有一个地址
	if len(hostList) == 0 {
		hostList = []string{"0.0.0.0"}
	}

	// 创建错误通道和等待组
	errChan := make(chan error, len(hostList))

	// 打印服务器启动信息
	fmt.Printf("===== Cloud Clipboard Server %s =====\n", server_version)
	fmt.Printf("Storage: %s\n", storage_folder)

	// 启动多个监听器
	for _, host := range hostList {
		// IPv6 地址需要用方括号括起来
		formattedHost := host
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			formattedHost = "[" + host + "]"
		}

		listen_addr := fmt.Sprintf("%s:%d", formattedHost, config.Server.Port)
		fmt.Printf("--- 监听地址: %s%s\n", listen_addr, config.Server.Prefix)

		// 使用 goroutine 启动服务器
		go func(addr string) {
			errChan <- http.ListenAndServe(addr, nil)
		}(listen_addr)
	}

	// 等待任意一个服务器出错
	err := <-errChan
	log.Fatalf("服务器错误: %v", err)
}

// clean expire file
func clean_expire_files() {
	for {
		// time.Sleep(30 * time.Minute)
		time.Sleep(5 * time.Minute)
		// time.Sleep(30 * time.Second)

		currentTime := time.Now().Unix()
		var toRemove []string
		fmt.Println("--- clean_expire_files @", currentTime)

		for uuid, fileInfo := range uploadFileMap {
			if fileInfo.ExpireTime < currentTime {
				toRemove = append(toRemove, uuid)
			}
		}

		for _, uuid := range toRemove {
			fmt.Println("- del1:", uuid)
			delete(uploadFileMap, uuid)                    // rm key
			os.Remove(filepath.Join(storage_folder, uuid)) // rm file
		}
	}
}
