package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaolacci/murmur3"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	deviceConnected = make(map[string]string)
)

var server_version = "go-1.22.x"
var build_git_hash = show_bin_info()

// 将配置相关变量声明为指针，不要立即初始化
// config_path 已在 config.go 中声明
var config *Config // 配置会在 main() 中加载

var websockets = make(map[*websocket.Conn]bool)
var room_ws = make(map[*websocket.Conn]string)

var deviceHashSeed = murmur3.Sum32(random_bytes(32)) & 0xffffffff

// --------------- structs
type EventMsg struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
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

	message := PostEvent{
		Event: "receive",
		Data: ReceiveHolder{
			TextReceive: &TextReceive{
				// ID:   messageQueue.nextid, // NOT thread-safe
				Type: "text",
				Room: room,

				Content: bodyStr,
			},
		},
	}
	messageQueue.Append(&message)
	messageJSON, err := json.Marshal(message)
	if err != nil {
		http.Error(w, "无法编码消息", http.StatusInternalServerError)
		return
	}
	messageStr := string(messageJSON)
	broadcast_ws_msg(websockets, messageStr, room)
	save_history()
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

// create new fileEntry in file_map
func handle_upload(w http.ResponseWriter, r *http.Request) {
	// filename := r.PostFormValue("filename")
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

	message := PostEvent{
		Event: "receive",
		Data: ReceiveHolder{
			FileReceive: &FileReceive{
				// ID:   messageQueue.nextid, // NOT thread-safe
				Type: "file",
				Room: room,

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
		if _, ok := uploadFileMap[uuid]; !ok {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, filepath.Join(storage_folder, uuid))

	case http.MethodDelete:
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
	// 首先解析命令行参数
	parseFlags()

	// 加载配置，应用命令行覆盖
	config = loadConfigWithOverrides()

	load_history()
	mkdir_uploads() // mkdir -p uplodas

	prefix := config.Server.Prefix

	// serve static
	server_static(prefix)

	// api
	http.HandleFunc(prefix+"/server", handle_server)
	http.HandleFunc(prefix+"/text", handle_text)
	http.HandleFunc(prefix+"/upload", handle_upload)
	http.HandleFunc(prefix+"/upload/chunk/", handle_chunk)
	http.HandleFunc(prefix+"/upload/finish/", handle_finish)
	http.HandleFunc(prefix+"/push", handle_push)
	http.HandleFunc(prefix+"/file/", handle_file)
	http.HandleFunc(prefix+"/revoke/", handle_revoke)
	http.HandleFunc(prefix+"/revoke/all", handleClearAll)

	// expire cleaner
	go clean_expire_files()

	// run
	listen_addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	fmt.Println("--- server run on", listen_addr, prefix)
	log.Fatal(http.ListenAndServe(listen_addr, nil))
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
