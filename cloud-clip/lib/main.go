package lib

import (
	"context" // 确保导入 embed 包
	"crypto/rand"
	"encoding/json"
	"sync"
	"flag"
	"fmt"
	"io/fs"
	"io"
	"log"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os" // 确保导入 os 包
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
)

var server_version = "go verion by Jonnyan404"
var build_git_hash = show_bin_info()

// NewClipboardServer 构造函数
func NewClipboardServer(cfg *Config) (*ClipboardServer, error) {
	logger := log.New(os.Stdout, "ClipboardServer: ", log.LstdFlags|log.Lshortfile)

	storageFolder := "./uploads"
	if cfg.Server.StorageDir != "" {
		storageFolder = cfg.Server.StorageDir
	}
	if err := os.MkdirAll(storageFolder, 0755); err != nil {
		logger.Printf("无法创建存储目录 %s: %v", storageFolder, err)
		// 根据需求，这里可以是致命错误
		// return nil, fmt.Errorf("无法创建存储目录 %s: %w", storageFolder, err)
	} else {
		logger.Printf("存储目录设置为: %s", storageFolder)
	}

	historyFilePath := filepath.Join(storageFolder, "history.json")
	if cfg.Server.HistoryFile != "" {
		historyFilePath = cfg.Server.HistoryFile
	} else {
		cfg.Server.HistoryFile = historyFilePath // 更新配置对象中的路径
		logger.Printf("历史文件路径未指定，使用默认: %s", historyFilePath)
	}
	logger.Printf("历史文件路径设置为: %s", historyFilePath)

	mqHistoryLen := 100 // 默认历史长度
	if cfg.Server.History > 0 {
		mqHistoryLen = cfg.Server.History
	}
	mq := NewMessageQueue(mqHistoryLen)

	uaParser := uaparser.NewFromSaved() // 初始化UA解析器

	// 处理认证：如果 cfg.Server.Auth 是布尔值 true，则生成随机密码
	if authBool, ok := cfg.Server.Auth.(bool); ok && authBool {
		randomPassword, err := generateRandomString(8)
		if err != nil {
			logger.Printf("警告: 生成随机密码失败: %v。认证可能无法正常工作。", err)
			// 根据策略，这里可以决定是否继续或返回错误
			// cfg.Server.Auth = "" // 清空，使其认证失败
		} else {
			cfg.Server.Auth = randomPassword // 将随机密码存回配置（内存中）
			logger.Printf("认证已启用，随机生成的密码为: %s", randomPassword)
			fmt.Printf("== \033[07m 认证密码 \033[0m: \033[33m%s\033[0m\n", randomPassword)
		}
	} else if authStr, ok := cfg.Server.Auth.(string); ok && authStr != "" {
		logger.Printf("认证已启用，使用配置的密码。")
	} else if authInt, ok := cfg.Server.Auth.(int); ok && authInt != 0 {
		// 将整数转换为字符串
		strPassword := strconv.Itoa(authInt)
		cfg.Server.Auth = strPassword
		logger.Printf("认证已启用，使用转换为字符串的整数密码: %s", strPassword)
	} else if authFloat, ok := cfg.Server.Auth.(float64); ok && authFloat != 0 {
		// JSON解析数字默认使用float64，需要将其转换为字符串
		strPassword := strconv.FormatFloat(authFloat, 'f', 0, 64)
		cfg.Server.Auth = strPassword
		logger.Printf("认证已启用，使用转换为字符串的数字密码: %s", strPassword)
	} else if authNumber, ok := cfg.Server.Auth.(json.Number); ok {
		// 处理json.Number类型（在一些JSON解析配置中可能会出现）
		strPassword := string(authNumber)
		cfg.Server.Auth = strPassword
		logger.Printf("认证已启用，使用转换为字符串的JSON数字密码: %s", strPassword)
	} else {
		logger.Printf("认证未启用。")
		cfg.Server.Auth = "" // 确保在未配置或配置为false时为空字符串
	}

	s := &ClipboardServer{
		config:          cfg,
		logger:          logger,
		messageQueue:    mq,
		websockets:      make(map[*websocket.Conn]bool),
		room_ws:         make(map[*websocket.Conn]string),
		uploadFileMap:   make(map[string]File),
		deviceConnected: make(map[string]DeviceMeta),
		storageFolder:   storageFolder,
		historyFilePath: historyFilePath,
		parser:          uaParser,
		connDeviceIDMap: make(map[*websocket.Conn]string),
		deviceHashSeed:  murmur3.Sum32(random_bytes(32)) & 0xffffffff, // 在此处初始化种子

		// 初始化房间管理相关字段
		roomStats:      make(map[string]*RoomStat),
		roomStatsMutex: sync.RWMutex{},
	}

	if err := s.loadHistoryData(); err != nil {
		s.logger.Printf("警告: 加载历史记录失败: %v. 将以空历史记录启动。", err)
	}

	// 如果启用了房间列表功能，启动房间清理任务
	if cfg.Server.RoomList {
		s.startRoomCleanup()
	}

	return s, nil
}

// --- ClipboardServer 方法 ---

func (s *ClipboardServer) loadHistoryData() error {
	s.logger.Printf("尝试从以下路径加载历史记录: %s", s.historyFilePath)

	if !pathExists(s.historyFilePath) { // pathExists 来自 utils.go 或 history.go
		s.logger.Println("历史文件不存在。将以空历史记录启动。")
		return nil
	}

	data, err := os.ReadFile(s.historyFilePath)
	if err != nil {
		return fmt.Errorf("无法读取历史文件 %s: %w", s.historyFilePath, err)
	}

	var loadedHist History // History struct from types.go
	if err := json.Unmarshal(data, &loadedHist); err != nil {
		s.logger.Printf("无法解析历史数据 %s: %v。将尝试删除损坏的历史文件。", s.historyFilePath, err)
		os.Remove(s.historyFilePath)
		return fmt.Errorf("无法解析历史数据 %s: %w", s.historyFilePath, err)
	}

	s.messageQueue.Lock()
	// 将 loadedHist.Receive ([]ReceiveHolder) 转换为 []PostEvent
	s.messageQueue.List = make([]PostEvent, 0, len(loadedHist.Receive))
	for _, rh := range loadedHist.Receive {
		s.messageQueue.List = append(s.messageQueue.List, PostEvent{
			Event: rh.Type(), // 从 ReceiveHolder 获取事件类型
			Data:  rh,        // ReceiveHolder 赋值给 PostEvent.Data
		})
	}

	// 确保 nextid 至少是加载的最后一个消息的 ID + 1
	if len(s.messageQueue.List) > 0 {
		lastID := s.messageQueue.List[len(s.messageQueue.List)-1].Data.ID()
		if s.messageQueue.nextid <= lastID {
			s.messageQueue.nextid = lastID + 1
		}
	}

	if len(s.messageQueue.List) > s.messageQueue.history_len {
		s.messageQueue.List = s.messageQueue.List[len(s.messageQueue.List)-s.messageQueue.history_len:]
	}
	s.messageQueue.Unlock()

	// 更新 uploadFileMap 的逻辑保持不变
	for _, rh := range loadedHist.Receive { // 遍历原始的 []ReceiveHolder
		if fileRec := rh.FileReceive; fileRec != nil && fileRec.Cache != "" {
			filePath := filepath.Join(s.storageFolder, fileRec.Cache)
			if _, statErr := os.Stat(filePath); statErr == nil {
				s.uploadFileMap[fileRec.Cache] = File{
					Name:       fileRec.Name,
					UUID:       fileRec.Cache,
					Size:       fileRec.Size,
					ExpireTime: fileRec.Expire,
					UploadTime: rh.Timestamp(), // 使用 ReceiveHolder 的 Timestamp 方法
				}
			} else {
				s.logger.Printf("历史记录中的文件 %s (UUID: %s) 在磁盘上未找到，将不加载到文件映射中。", fileRec.Name, fileRec.Cache)
			}
		}
	}
	s.filterHistoryMessages()

	s.logger.Printf("成功从历史记录加载 %d 条消息和 %d 个文件条目。", len(s.messageQueue.List), len(s.uploadFileMap))
	return nil
}

func (s *ClipboardServer) saveHistoryData() {
	s.logger.Printf("尝试将历史记录保存到: %s", s.historyFilePath)

	s.messageQueue.Lock()
	// s.filterHistoryMessagesLocked() // 需要在锁内部调用

	// 将 s.messageQueue.List ([]PostEvent) 转换为 []ReceiveHolder 以匹配 History 结构
	receiveHolders := make([]ReceiveHolder, len(s.messageQueue.List))
	for i, pe := range s.messageQueue.List {
		receiveHolders[i] = pe.Data // PostEvent.Data 是 ReceiveHolder
	}

	histToSave := History{
		// NextID:   s.messageQueue.nextid, // 如果 History 结构有 NextID 字段
		Receive: receiveHolders,
		// File 字段也需要填充，如果它与 uploadFileMap 相关
		// File: s.getFilesForHistory(), // 假设有这样一个辅助函数
	}
	// 如果 History 结构中也需要存储 File 列表 (s.uploadFileMap 的内容)
	// 你需要添加逻辑来填充 histToSave.File
	var filesForHistory []File
	for _, f := range s.uploadFileMap {
		filesForHistory = append(filesForHistory, f)
	}
	histToSave.File = filesForHistory

	s.messageQueue.Unlock() // 尽早解锁

	data, err := json.MarshalIndent(histToSave, "", "  ")
	if err != nil {
		s.logger.Printf("序列化历史记录以进行保存时出错: %v", err)
		return
	}

	if err := os.WriteFile(s.historyFilePath, data, 0644); err != nil {
		s.logger.Printf("写入历史文件 %s 时出错: %v", s.historyFilePath, err)
	} else {
		s.logger.Printf("历史记录已成功保存到 %s", s.historyFilePath)
	}
}

// filterHistoryMessagesLocked 过滤消息队列中的消息，移除无效或过期的文件消息
// 这个方法应该在 messageQueue 被锁定时调用
func (s *ClipboardServer) filterHistoryMessagesLocked() {
	if s.messageQueue.List == nil { // 确保使用大写 L
		return
	}
	var validMessages []PostEvent
	now := time.Now().Unix()
	for _, msg := range s.messageQueue.List { // 确保使用大写 L
		if msg.Data.FileReceive != nil {
			fileRec := msg.Data.FileReceive
			fileInfo, existsInMap := s.uploadFileMap[fileRec.Cache]
			if !existsInMap || fileInfo.ExpireTime < now {
				s.logger.Printf("从历史记录中过滤掉文件消息: %s (UUID: %s)，原因: 文件不存在或已过期。", fileRec.Name, fileRec.Cache)
				if existsInMap && fileInfo.ExpireTime < now {
					delete(s.uploadFileMap, fileRec.Cache)
				}
				continue
			}
		}
		validMessages = append(validMessages, msg)
	}
	s.messageQueue.List = validMessages // 确保使用大写 L
}

// filterHistoryMessages 是一个包装器，用于在需要时获取锁
func (s *ClipboardServer) filterHistoryMessages() {
	s.messageQueue.Lock()
	s.filterHistoryMessagesLocked()
	s.messageQueue.Unlock()
}

func hasEmbeddedStatic() bool {
    // 尝试打开 static 目录，如果成功说明有嵌入的文件
    if _, err := embed_static_fs.Open("static"); err == nil {
        return true
    }
    return false
}

func (s *ClipboardServer) setupRoutes() {
	s.logger.Println("正在设置路由...")
	prefix := s.config.Server.Prefix
	mux := http.NewServeMux()
	if *flg_static_dir != "" { // 检查配置中的外部静态目录
		s.logger.Printf("从外部目录提供静态文件: %s", *flg_static_dir)
		if _, statErr := os.Stat(*flg_static_dir); os.IsNotExist(statErr) {
			s.logger.Printf("警告: 配置的外部静态目录 %s 不存在。将不提供前端服务。", *flg_static_dir)
		} else {
			mux.Handle(prefix+"/", http.StripPrefix(prefix, compressionMiddleware(http.FileServer(http.Dir(*flg_static_dir)))))
		}
    } else if hasEmbeddedStatic() { // 直接检测是否有嵌入的静态文件
        s.logger.Println("使用嵌入式静态文件。")
        fsys, err := fs.Sub(embed_static_fs, "static")
        if err != nil {
            s.logger.Fatalf("错误: 无法从 embed_static_fs 获取 'static' 子目录: %v", err)
        }
        mux.Handle(prefix+"/", http.StripPrefix(prefix, compressionMiddleware(http.FileServer(http.FS(fsys)))))
    } else {
        s.logger.Println("警告: 未使用嵌入式静态文件，也未配置外部静态目录。将不提供前端服务。")
    }

	// HTTP 路由
	mux.HandleFunc(prefix+"/server", s.handle_server)
	mux.HandleFunc(prefix+"/push", s.handle_push)
	mux.HandleFunc(prefix+"/rooms", s.handleRooms) 
	mux.HandleFunc(prefix+"/file/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			s.handle_file(w, r)
		} else {
			s.authMiddleware(s.handle_file)(w, r)
		}
	})
	mux.HandleFunc(prefix+"/text", s.authMiddleware(s.handle_text))
	mux.HandleFunc(prefix+"/upload", s.authMiddleware(s.handle_upload))
	mux.HandleFunc(prefix+"/upload/chunk", s.authMiddleware(s.handle_upload))
	mux.HandleFunc(prefix+"/upload/chunk/", s.authMiddleware(s.handle_chunk))
	mux.HandleFunc(prefix+"/upload/finish/", s.authMiddleware(s.handle_finish))
	mux.HandleFunc(prefix+"/revoke/", s.authMiddleware(s.handle_revoke))
	mux.HandleFunc(prefix+"/revoke/all", s.authMiddleware(s.handleClearAll))
	mux.HandleFunc(prefix+"/content/", s.authMiddleware(s.handleContent))

	s.httpServer = &http.Server{
		Handler: mux,
	}
}

func (s *ClipboardServer) Start() error {
	s.runMutex.Lock()
	if s.isRunning {
		s.runMutex.Unlock()
		s.logger.Println("服务器已在运行。")
		return fmt.Errorf("服务器已在运行")
	}

	s.setupRoutes() // 在这里设置 s.httpServer.Handler

	hostList := []string{"0.0.0.0"} // 默认
	// 从配置中解析 Host 字段
	if hostCfg, ok := s.config.Server.Host.([]interface{}); ok {
		var parsedHosts []string
		for _, h := range hostCfg {
			if hostStr, isStr := h.(string); isStr && hostStr != "" {
				parsedHosts = append(parsedHosts, hostStr)
			}
		}
		if len(parsedHosts) > 0 {
			hostList = parsedHosts
		}
	} else if hostStr, isStr := s.config.Server.Host.(string); isStr && hostStr != "" { // 处理单个字符串的情况
		hostList = []string{hostStr}
	} else if hostsArray, isArray := s.config.Server.Host.([]string); isArray && len(hostsArray) > 0 { // 处理已经是 []string 的情况
		hostList = hostsArray
	}

	s.logger.Printf("===== Cloud Clipboard Server %s =====", server_version)
	s.logger.Printf("存储目录: %s", s.storageFolder)
	s.logger.Printf("历史文件: %s", s.historyFilePath)

	// 显示所有将要监听的地址
	s.logger.Printf("将监听以下地址: %v", hostList)

	if len(hostList) == 0 {
		s.runMutex.Unlock()
		return fmt.Errorf("没有配置有效的监听地址")
	}

	// 创建多个监听器
	listeners := make([]net.Listener, 0, len(hostList))
	for _, host := range hostList {
		// 处理IPv6地址
		formattedHost := host
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") { // IPv6
			formattedHost = "[" + host + "]"
		}

		listenAddr := fmt.Sprintf("%s:%d", formattedHost, s.config.Server.Port)
		ln, err := net.Listen("tcp", listenAddr)
		if err != nil {
			s.logger.Printf("警告: 无法在 %s 上监听: %v", listenAddr, err)
			continue
		}

		listeners = append(listeners, ln)
		s.logger.Printf("--- 监听地址: %s%s", listenAddr, s.config.Server.Prefix)
	}

	if len(listeners) == 0 {
		s.runMutex.Unlock()
		return fmt.Errorf("无法在任何配置的地址上启动监听")
	}

	s.isRunning = true
	s.runMutex.Unlock()

	go s.cleanExpiredFilesLoop()

	// 为每个监听器创建一个单独的HTTP服务器并启动goroutine
	errChan := make(chan error, len(listeners))
	for i, ln := range listeners {
		// 克隆原始的HTTP服务器配置
		server := &http.Server{
			Handler:      s.httpServer.Handler,
			ReadTimeout:  s.httpServer.ReadTimeout,
			WriteTimeout: s.httpServer.WriteTimeout,
			IdleTimeout:  s.httpServer.IdleTimeout,
		}

		// 确保至少有一个实例被赋值给s.httpServer以便Stop()方法可以使用
		if i == 0 {
			s.httpServer = server
		}

		go func(srv *http.Server, listener net.Listener) {
			var err error
			addr := listener.Addr().String()

			if s.config.Server.Cert != "" && s.config.Server.Key != "" {
				s.logger.Printf("启动 HTTPS 服务器于 %s", addr)
				err = srv.ServeTLS(listener, s.config.Server.Cert, s.config.Server.Key)
			} else {
				s.logger.Printf("启动 HTTP 服务器于 %s", addr)
				err = srv.Serve(listener)
			}

			if err != nil && err != http.ErrServerClosed {
				s.logger.Printf("HTTP 服务器在 %s 上的 Serve/ServeTLS 错误: %v", addr, err)
				errChan <- err
			} else {
				s.logger.Printf("HTTP 服务器在 %s 上正常关闭", addr)
			}
		}(server, ln)
	}

	// 等待任何一个服务器出错或全部正常关闭
	var err error
	select {
	case err = <-errChan:
		s.logger.Printf("一个或多个 HTTP 服务器出错: %v", err)
		// 尝试优雅关闭所有服务器
		s.Stop()
	}

	s.runMutex.Lock()
	s.isRunning = false
	s.runMutex.Unlock()

	return err
}

func (s *ClipboardServer) Stop() error {
	s.runMutex.Lock()
	defer s.runMutex.Unlock()

	if !s.isRunning || s.httpServer == nil {
		s.logger.Println("服务器未运行或未初始化。")
		return fmt.Errorf("服务器未运行")
	}
    // 停止房间清理任务
    s.stopRoomCleanup()
	s.logger.Println("正在停止服务器...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := s.httpServer.Shutdown(ctx)
	// isRunning 状态由 Start 中的 defer/finally 处理
	if err != nil {
		s.logger.Printf("HTTP 服务器关闭错误: %v", err)
		return err
	}
	s.logger.Println("服务器已成功关闭。")
	return nil
}

func (s *ClipboardServer) cleanExpiredFilesLoop() {
	// 确保配置中 File.Expire > 0 才启动清理
	if s.config.File.Expire <= 0 {
		s.logger.Println("文件过期时间设置为0或负数，不启动过期文件清理任务。")
		return
	}
	// 清理间隔可以配置，例如 s.config.File.ExpireCheckInterval，默认为5分钟
	checkInterval := 5 * time.Minute
	s.logger.Printf("后台过期文件清理任务已启动，检查间隔: %v", checkInterval)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		<-ticker.C // 等待下一个 tick
		s.performCleanExpiredFiles()
	}
}

func (s *ClipboardServer) performCleanExpiredFiles() {
	s.logger.Println("正在运行过期文件清理任务...")
	currentTime := time.Now().Unix()
	var toRemove []string

	// 注意：并发访问 s.uploadFileMap 需要加锁
	// s.mapMutex.Lock() // 假设有一个用于保护 map 的锁
	for uuid, fileInfo := range s.uploadFileMap {
		if fileInfo.ExpireTime < currentTime {
			toRemove = append(toRemove, uuid)
		}
	}
	// s.mapMutex.Unlock()

	if len(toRemove) > 0 {
		s.logger.Printf("发现 %d 个过期文件需要移除。", len(toRemove))
		removedCount := 0
		for _, uuid := range toRemove {
			filePath := filepath.Join(s.storageFolder, uuid)
			if err := os.Remove(filePath); err != nil {
				if !os.IsNotExist(err) { // 如果文件不存在，则不是一个错误
					s.logger.Printf("移除文件 %s 时出错: %v", filePath, err)
				}
			} else {
				s.logger.Printf("已移除过期文件: %s (UUID: %s)", filePath, uuid)
			}
			// s.mapMutex.Lock()
			delete(s.uploadFileMap, uuid) // 从 map 中移除
			// s.mapMutex.Unlock()
			removedCount++
		}
		if removedCount > 0 {
			// 文件被移除后，历史记录中可能还存在对这些文件的引用
			// 调用 saveHistoryData 会触发 filterHistoryMessagesLocked 清理这些引用
			s.saveHistoryData()
		}
	} else {
		s.logger.Println("没有发现过期文件。")
	}
}

// parse_user_agent 现在使用 s.parser
func (s *ClipboardServer) parse_user_agent(uaString string) map[string]string {
	client := s.parser.Parse(uaString) // 使用实例化的解析器
	return map[string]string{
		"type":    client.Device.Family,
		"os":      fmt.Sprintf("%s %s", client.Os.Family, client.Os.Major),
		"browser": fmt.Sprintf("%s %s", client.UserAgent.Family, client.UserAgent.Major),
	}
}

// get_remote_ip (保持不变)
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
			ip = remoteAddr
		}
	}
	ips := strings.Split(ip, ",")
	if len(ips) > 0 {
		ip = strings.TrimSpace(ips[0])
	}
	return ip
}

// getScheme (保持不变)
func getScheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

// --- main 函数 ---
func Main() {
	// 确保标志只解析一次。如果 flags.go 中的 init() 调用了 flag.Parse()，这里可以省略。
	// 为安全起见，检查一下。
	if !flag.Parsed() {
		flag.Parse()
	}

	initialCfg, err := load_config(*flg_config) // flg_config 来自 flags.go
	if err != nil {
		log.Printf("警告: 加载初始配置失败: %v。将使用默认值继续。", err)
		initialCfg = defaultConfig() // 确保 defaultConfig() 返回一个有效的 Config 实例
	}
	if initialCfg == nil { // 双重检查
		initialCfg = defaultConfig()
	}

	applyCommandLineArgs(initialCfg) // applyCommandLineArgs 来自 flags.go

	server, err := NewClipboardServer(initialCfg)
	if err != nil {
		log.Fatalf("创建剪贴板服务器失败: %v", err)
	}

	if err := server.Start(); err != nil {
		server.logger.Fatalf("服务器启动失败: %v", err)
	}
	server.logger.Println("主函数退出。")
}

// show_bin_info (保持不变)
func show_bin_info() string {
	buildInfo, ok := debug.ReadBuildInfo()
	var gitHash string
	if !ok {
		// log.Printf("无法读取构建信息")
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
	fmt.Printf("== \033[07m cloud-clip \033[36m %s \033[0m     \033[35m %s  %s     %s\033[0m\n",
		server_version, gitHash, buildInfo.GoVersion, buildInfo.Main.Version)
	return gitHash
}


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

// 辅助函数：获取特定房间内的设备ID，排除某个设备
// 必须在 s.runMutex 锁定时调用
func (s *ClipboardServer) getDeviceIDsInRoomLocked(room string, excludeDeviceID string) []string {
	var deviceIDs []string
	for conn, clientRoom := range s.room_ws { // Iterate through connections and their rooms
		if clientRoom == room { // If the connection is in the target room
			if devID, ok := s.connDeviceIDMap[conn]; ok { // Get the deviceID for this connection
				if devID != excludeDeviceID { // Don't include the excluded device itself
					deviceIDs = append(deviceIDs, devID)
				}
			}
		}
	}
	return deviceIDs
}

// 辅助函数：清理 WebSocket 连接并通知其他人
func (s *ClipboardServer) cleanupWebSocketConnection(conn *websocket.Conn, deviceID string, room string) {
    // 第一步：在锁内进行状态清理，但不关闭连接
    var shouldBroadcast bool
    s.runMutex.Lock()
    delete(s.websockets, conn)
    delete(s.room_ws, conn)
    delete(s.connDeviceIDMap, conn)
    
    if deviceID != "" {
        delete(s.deviceConnected, deviceID)
        s.updateRoomDeviceCount(room, deviceID, false)
        shouldBroadcast = true
        s.logger.Printf("WebSocket 客户端断开连接: %s (ID: %s), 房间: %s. 当前连接数: %d, 设备数: %d", 
            conn.RemoteAddr(), deviceID, room, len(s.websockets), len(s.deviceConnected))
    } else {
        s.logger.Printf("WebSocket 客户端断开连接 (无有效DeviceID): %s, 房间: %s. 当前连接数: %d", 
            conn.RemoteAddr(), room, len(s.websockets))
    }
    s.runMutex.Unlock()

    // 第二步：在锁外关闭连接
    conn.Close()

    // 第三步：广播断开连接事件
    if shouldBroadcast {
        disconnectWsMsg := WebSocketMessage{
            Event: "disconnect",
            Data:  map[string]string{"id": deviceID},
        }
        s.broadcastWebSocketMessage(disconnectWsMsg, room)
    }
}

// broadcastMessageToRoomExcept 将消息广播到房间中的所有客户端，除了一个特定的连接。
func (s *ClipboardServer) broadcastMessageToRoomExcept(message PostEvent, room string, exceptConn *websocket.Conn) {
    // 第一步：在锁内收集需要发送的连接
    var targetConnections []*websocket.Conn
    s.runMutex.Lock()
    for client, clientRoom := range s.room_ws {
        if client == exceptConn {
            continue
        }
        if room == "" || clientRoom == room {
            targetConnections = append(targetConnections, client)
        }
    }
    s.runMutex.Unlock()

    // 第二步：在锁外进行网络操作
    var failedConnections []*websocket.Conn
    for _, client := range targetConnections {
        if err := client.WriteJSON(message); err != nil {
            s.logger.Printf("错误: 写入消息到 WebSocket 客户端 %s 失败: %v。计划移除客户端。", client.RemoteAddr(), err)
            failedConnections = append(failedConnections, client)
        }
    }

    // 第三步：清理失败的连接
    if len(failedConnections) > 0 {
        s.runMutex.Lock()
        for _, client := range failedConnections {
            client.Close()
            delete(s.websockets, client)
            delete(s.room_ws, client)
            if deviceID, ok := s.connDeviceIDMap[client]; ok {
                delete(s.connDeviceIDMap, client)
                delete(s.deviceConnected, deviceID)
            }
        }
        s.runMutex.Unlock()
    }
}

// hash_murmur3 函数 (假设可用，例如来自 random.go 或工具文件)
// 如果没有，需要定义或导入。例如：
func hash_murmur3(data []byte, seed uint32) uint32 {
	h := murmur3.New32WithSeed(seed) // murmur3 来自 "github.com/spaolacci/murmur3"
	h.Write(data)
	return h.Sum32()
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

	s.logger.Printf("收到文本消息 (房间: %s): %s", room, text)
	event := s.addMessageToQueueAndBroadcast("text", text, room, r)

	// 响应 (可以效仿 auth.go 中的 enhanceHandleText 返回内容 URL)
	scheme := getScheme(r)
	contentURL := fmt.Sprintf("%s://%s%s/content/%d", scheme, r.Host, s.config.Server.Prefix, event.Data.ID()) // [!code word]
	if room != "default" {                                                                                     // 只有非默认房间才添加 room 参数到 URL
		contentURL += fmt.Sprintf("?room=%s", room)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url":  contentURL,
		"id":   strconv.Itoa(event.Data.ID()),
		"type": "text", // 添加 type 参数
	})
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
	room := r.URL.Query().Get("room") // 可选，如果只想清空特定房间

	s.logger.Printf("处理 /revoke/all 请求 (房间: '%s')", room)

	s.messageQueue.Lock()
	var newMsgList []PostEvent
	var revokedIDs []int
	if room == "" { // 清空所有房间
		s.messageQueue.List = []PostEvent{}
		// nextid 可以不清零，或者根据需求决定是否重置
	} else { // 只清空指定房间的消息
		for _, msg := range s.messageQueue.List {
			if msg.Data.Room() != room {
				newMsgList = append(newMsgList, msg)
			} else {
				revokedIDs = append(revokedIDs, msg.Data.ID())
			}
		}
		s.messageQueue.List = newMsgList
	}
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

func (s *ClipboardServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 添加 CORS 头，允许跨域请求
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 快速路径：如果不需要认证，直接调用下一个处理函数
		authNeeded := false
		var expectedPassword string

		// 处理所有可能的类型：string、bool、int、float64
		switch auth := s.config.Server.Auth.(type) {
		case string:
			if auth != "" {
				authNeeded = true
				expectedPassword = auth
			}
		case bool:
			// bool true 的情况已在 NewClipboardServer 中处理为随机密码
			if auth {
				authNeeded = true
				// expectedPassword 应该已经在 NewClipboardServer 中设置
				if authStr, ok := s.config.Server.Auth.(string); ok {
					expectedPassword = authStr
				}
			}
		case int:
			authNeeded = true
			expectedPassword = strconv.Itoa(auth)
		case float64:
			authNeeded = true
			expectedPassword = strconv.FormatFloat(auth, 'f', 0, 64)
		}

		if !authNeeded {
			next.ServeHTTP(w, r)
			return
		}

		// 获取认证令牌 - 先检查 Authorization 头，再检查查询参数
		token := ""

		// 检查 Authorization 头
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				token = parts[1]
			} else {
				// 尝试将整个头部作为令牌（向后兼容）
				token = authHeader
			}
		}

		// 如果头部没有令牌，尝试从查询参数获取
		if token == "" {
			token = r.URL.Query().Get("auth")
		}

		clientIP := get_remote_ip(r)

		// 验证令牌
		if token == "" {
			s.logger.Printf("认证失败: 未提供令牌。来自 IP: %s, 路径: %s", clientIP, r.URL.Path)

			// 返回结构化的 JSON 错误响应
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Unauthorized",
				"message": "需要认证令牌",
			})
			return
		}

		if expectedPassword == "" {
			s.logger.Printf("认证失败: 服务器认证配置错误。来自 IP: %s", clientIP)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "ServerError",
				"message": "服务器认证配置错误",
			})
			return
		}

		if token != expectedPassword {
			s.logger.Printf("认证失败: 无效令牌。来自 IP: %s, 路径: %s,token:%s,server:%s", clientIP, r.URL.Path, token, expectedPassword)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Unauthorized",
				"message": "无效的认证令牌",
			})
			return
		}

		// 认证成功
		s.logger.Printf("认证成功: IP: %s, 路径: %s", clientIP, r.URL.Path)
		next.ServeHTTP(w, r)
	}
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}

// broadcastMessage 向所有连接的 WebSocket 客户端（可选地，特定房间）广播消息。
// 这个方法需要是线程安全的，因为它会被多个 goroutine 调用。
func (s *ClipboardServer) broadcastMessage(message PostEvent, room string) {
    s.logger.Printf("广播消息 (ID: %d, 类型: %s) 到房间 '%s'", message.Data.ID(), message.Event, room)
    
    // 第一步：在锁内收集需要发送的连接
    var targetConnections []*websocket.Conn
    s.runMutex.Lock()
    for client, clientRoom := range s.room_ws {
        if room == "" || clientRoom == room {
            targetConnections = append(targetConnections, client)
        }
    }
    s.runMutex.Unlock()

    // 第二步：在锁外进行网络操作
    var failedConnections []*websocket.Conn
    for _, client := range targetConnections {
        if err := client.WriteJSON(message); err != nil {
            s.logger.Printf("错误: 写入消息到 WebSocket 客户端 %s 失败: %v。移除客户端。", client.RemoteAddr(), err)
            failedConnections = append(failedConnections, client)
        }
    }

    // 第三步：清理失败的连接
    if len(failedConnections) > 0 {
        s.runMutex.Lock()
        for _, client := range failedConnections {
            client.Close()
            delete(s.websockets, client)
            delete(s.room_ws, client)
            if deviceID, ok := s.connDeviceIDMap[client]; ok {
                delete(s.connDeviceIDMap, client)
                delete(s.deviceConnected, deviceID)
            }
        }
        s.runMutex.Unlock()
    }
}

// addMessageToQueueAndBroadcast 添加消息到队列并广播
// 这是一个辅助函数，供 handle_text, handle_finish 等调用
func (s *ClipboardServer) addMessageToQueueAndBroadcast(dataType string, data interface{}, room string, r *http.Request) PostEvent {
	ip := get_remote_ip(r)
	ua := s.parse_user_agent(r.UserAgent())

	// Create ReceiveBase first
	receiveBase := ReceiveBase{
		// ID will be set by PostList.Append
		Type:         dataType, // This is the inner type for ReceiveHolder (e.g., "text", "file")
		Room:         room,
		Timestamp:    time.Now().Unix(),
		SenderIP:     ip,
		SenderDevice: ua,
	}

	// Create ReceiveHolder
	var rh ReceiveHolder
	switch dataType {
	case "text":
		rh.TextReceive = &TextReceive{
			ReceiveBase: receiveBase,
			Content:     data.(string),
		}
	case "file":
		fileRec := data.(*FileReceive)
		// Ensure FileReceive's own ReceiveBase is also populated if it's not already
		// For now, assuming data.(*FileReceive) might already have its ReceiveBase fields set,
		// or we can overwrite/set them here.
		// Let's assume data.(*FileReceive) is mostly complete except for common base fields.
		fileRec.ReceiveBase = receiveBase // Set the common base
		rh.FileReceive = fileRec
	default:
		// Handle unknown dataType if necessary, though current calls are "text" or "file"
		s.logger.Printf("警告: addMessageToQueueAndBroadcast 收到未知数据类型: %s", dataType)
		// Return an empty or error PostEvent
		return PostEvent{}
	}

	// 内部存储的事件
	storeEvent := PostEvent{
		Event: dataType, // "text" 或 "file"
		Data:  rh,       // ReceiveHolder
	}
	s.messageQueue.Append(&storeEvent) // msg.go 处理这个 PostEvent
    // 更新房间消息统计
    s.updateRoomStats(room, 1)
	// 准备发送给客户端的 WebSocket 消息
	var clientPayload interface{}
	if rh.TextReceive != nil {
		clientPayload = rh.TextReceive
	} else if rh.FileReceive != nil {
		clientPayload = rh.FileReceive
	}

	if clientPayload != nil {
		wsMsg := WebSocketMessage{
			Event: "receive",     // 前端期望的事件名
			Data:  clientPayload, // 前端期望的直接数据
		}
		s.broadcastWebSocketMessage(wsMsg, room) // 新的广播函数
	}

	s.saveHistoryData()
	return storeEvent // 返回内部事件，例如用于获取ID
}

// broadcastWebSocketMessage 向所有连接的 WebSocket 客户端（可选地，特定房间）广播 WebSocketMessage。
func (s *ClipboardServer) broadcastWebSocketMessage(message WebSocketMessage, room string) {
    s.logger.Printf("广播 WebSocket 消息 (类型: %s) 到房间 '%s'", message.Event, room)
    
    // 第一步：在锁内收集需要发送的连接
    var targetConnections []*websocket.Conn
    s.runMutex.Lock()
    for client, clientRoom := range s.room_ws {
        if room == "" || clientRoom == room {
            targetConnections = append(targetConnections, client)
        }
    }
    s.runMutex.Unlock()

    // 第二步：在锁外进行网络操作
    var failedConnections []*websocket.Conn
    for _, client := range targetConnections {
        if err := client.WriteJSON(message); err != nil {
            s.logger.Printf("错误: 写入 WebSocketMessage 到客户端 %s 失败: %v。计划移除客户端。", client.RemoteAddr(), err)
            failedConnections = append(failedConnections, client)
        }
    }

    // 第三步：清理失败的连接
    if len(failedConnections) > 0 {
        s.runMutex.Lock()
        for _, client := range failedConnections {
            client.Close()
            delete(s.websockets, client)
            delete(s.room_ws, client)
            // 从 connDeviceIDMap 中查找并删除对应的设备ID
            if deviceID, ok := s.connDeviceIDMap[client]; ok {
                delete(s.connDeviceIDMap, client)
                delete(s.deviceConnected, deviceID)
            }
        }
        s.runMutex.Unlock()
    }
}

// broadcastWebSocketMessageToRoomExcept 将 WebSocketMessage 广播到房间中的所有客户端，除了一个特定的连接。
func (s *ClipboardServer) broadcastWebSocketMessageToRoomExcept(message WebSocketMessage, room string, exceptConn *websocket.Conn) {
    // 第一步：在锁内收集需要发送的连接
    var targetConnections []*websocket.Conn
    s.runMutex.Lock()
    for client, clientRoom := range s.room_ws {
        if client == exceptConn {
            continue
        }
        if room == "" || clientRoom == room {
            targetConnections = append(targetConnections, client)
        }
    }
    s.runMutex.Unlock()

    // 第二步：在锁外进行网络操作
    var failedConnections []*websocket.Conn
    for _, client := range targetConnections {
        if err := client.WriteJSON(message); err != nil {
            s.logger.Printf("错误: 写入 WebSocketMessage (except) 到客户端 %s 失败: %v。", client.RemoteAddr(), err)
            failedConnections = append(failedConnections, client)
        }
    }

    // 第三步：清理失败的连接
    if len(failedConnections) > 0 {
        s.runMutex.Lock()
        for _, client := range failedConnections {
            client.Close()
            delete(s.websockets, client)
            delete(s.room_ws, client)
            if deviceID, ok := s.connDeviceIDMap[client]; ok {
                delete(s.connDeviceIDMap, client)
                delete(s.deviceConnected, deviceID)
            }
        }
        s.runMutex.Unlock()
    }
}
// --- 房间管理相关方法 ---

// updateRoomStats 更新房间统计信息
func (s *ClipboardServer) updateRoomStats(room string, messageCount int) {
    if !s.config.Server.RoomList {
        return // 如果没有启用房间列表功能，不统计
    }

    // 统一房间名称
    normalizedRoom := normalizeRoomName(room)

    s.roomStatsMutex.Lock()
    defer s.roomStatsMutex.Unlock()

    if s.roomStats[normalizedRoom] == nil {
        s.roomStats[normalizedRoom] = &RoomStat{
            MessageCount: 0,
            LastActive:   time.Now().Unix(),
            DeviceIDs:    make(map[string]bool),
        }
    }

    stat := s.roomStats[normalizedRoom]
    if messageCount > 0 {
        stat.MessageCount += messageCount
    }
    stat.LastActive = time.Now().Unix()
}

// updateRoomDeviceCount 更新房间设备数量
func (s *ClipboardServer) updateRoomDeviceCount(room string, deviceID string, connected bool) {
    if !s.config.Server.RoomList {
        return
    }

    // 统一房间名称
    normalizedRoom := normalizeRoomName(room)

    s.roomStatsMutex.Lock()
    defer s.roomStatsMutex.Unlock()

    if s.roomStats[normalizedRoom] == nil {
        s.roomStats[normalizedRoom] = &RoomStat{
            MessageCount: 0,
            LastActive:   time.Now().Unix(),
            DeviceIDs:    make(map[string]bool),
        }
    }

    stat := s.roomStats[normalizedRoom]
    if connected {
        stat.DeviceIDs[deviceID] = true
    } else {
        delete(stat.DeviceIDs, deviceID)
    }
    stat.LastActive = time.Now().Unix()
}


// getRoomList 获取房间列表
func (s *ClipboardServer) getRoomList() []RoomInfo {
    if !s.config.Server.RoomList {
        return []RoomInfo{}
    }

    // 第一步：快速收集当前连接信息
    currentRooms := make(map[string]map[string]bool)
    s.runMutex.Lock()
    for conn, room := range s.room_ws {
        if deviceID, ok := s.connDeviceIDMap[conn]; ok {
            normalizedRoom := normalizeRoomName(room)
            if currentRooms[normalizedRoom] == nil {
                currentRooms[normalizedRoom] = make(map[string]bool)
            }
            currentRooms[normalizedRoom][deviceID] = true
        }
    }
    s.runMutex.Unlock()

    // 第二步：快速收集消息信息
    roomMessageCounts := make(map[string]int)
    s.messageQueue.Lock()
    for _, msg := range s.messageQueue.List {
        normalizedRoom := normalizeRoomName(msg.Data.Room())
        roomMessageCounts[normalizedRoom]++
    }
    s.messageQueue.Unlock()

    // 第三步：快速收集房间统计信息
    roomStatsSnapshot := make(map[string]RoomStat)
    s.roomStatsMutex.RLock()
    for room, stat := range s.roomStats {
        // 创建副本，避免长时间持有锁
        roomStatsSnapshot[room] = RoomStat{
            MessageCount: stat.MessageCount,
            LastActive:   stat.LastActive,
            DeviceIDs:    make(map[string]bool),
        }
        // 复制 DeviceIDs
        for deviceID, active := range stat.DeviceIDs {
            roomStatsSnapshot[room].DeviceIDs[deviceID] = active
        }
    }
    s.roomStatsMutex.RUnlock()

    // 第四步：在无锁状态下处理数据
    allRooms := make(map[string]bool)
    
    // 添加有消息的房间
    for room := range roomMessageCounts {
        allRooms[room] = true
    }
    
    // 添加有连接的房间
    for room := range currentRooms {
        allRooms[room] = true
    }
    
    // 添加统计中的房间
    for room := range roomStatsSnapshot {
        allRooms[room] = true
    }

    var roomList []RoomInfo
    for room := range allRooms {
        // 显示时转换：default 显示为空字符串
        displayRoom := room
        if room == "default" {
            displayRoom = ""
        }

        deviceCount := 0
        if devices, ok := currentRooms[room]; ok {
            deviceCount = len(devices)
        }

        messageCount := roomMessageCounts[room]

        var lastActive int64
        if stat, ok := roomStatsSnapshot[room]; ok {
            lastActive = stat.LastActive
        }

        // 如果有活跃连接，更新最后活跃时间
        if deviceCount > 0 {
            lastActive = time.Now().Unix()
        }

        roomInfo := RoomInfo{
            Name:         displayRoom,
            MessageCount: messageCount,
            DeviceCount:  deviceCount,
            LastActive:   lastActive,
            IsActive:     deviceCount > 0,
        }

        roomList = append(roomList, roomInfo)
    }

    // 排序：活跃房间优先，然后按最后活跃时间排序
    for i := 0; i < len(roomList)-1; i++ {
        for j := i + 1; j < len(roomList); j++ {
            if roomList[i].IsActive != roomList[j].IsActive {
                if roomList[j].IsActive {
                    roomList[i], roomList[j] = roomList[j], roomList[i]
                }
            } else {
                if roomList[i].LastActive < roomList[j].LastActive {
                    roomList[i], roomList[j] = roomList[j], roomList[i]
                }
            }
        }
    }

    return roomList
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

// startRoomCleanup 启动房间清理任务
func (s *ClipboardServer) startRoomCleanup() {
    if s.config.Server.RoomCleanup <= 0 {
        s.logger.Println("房间清理间隔设置为0或负数，不启动房间清理任务")
        return
    }

    interval := time.Duration(s.config.Server.RoomCleanup) * time.Second
    s.roomCleanupTicker = time.NewTicker(interval)
    s.logger.Printf("房间清理任务已启动，清理间隔: %v", interval)

    go func() {
        for range s.roomCleanupTicker.C {
            s.cleanupEmptyRooms()
        }
    }()
}

// stopRoomCleanup 停止房间清理任务
func (s *ClipboardServer) stopRoomCleanup() {
    if s.roomCleanupTicker != nil {
        s.roomCleanupTicker.Stop()
        s.roomCleanupTicker = nil
        s.logger.Println("房间清理任务已停止")
    }
}

// cleanupEmptyRooms 清理空房间
func (s *ClipboardServer) cleanupEmptyRooms() {
    if !s.config.Server.RoomList {
        return
    }

    s.logger.Println("开始清理空房间...")

    // 第一步：快速收集活跃房间信息
    activeRooms := make(map[string]bool)
    s.runMutex.Lock()
    for _, room := range s.room_ws {
        normalizedRoom := normalizeRoomName(room)
        activeRooms[normalizedRoom] = true
    }
    s.runMutex.Unlock()

    // 第二步：快速收集有消息的房间
    roomsWithMessages := make(map[string]bool)
    s.messageQueue.Lock()
    for _, msg := range s.messageQueue.List {
        normalizedRoom := normalizeRoomName(msg.Data.Room())
        roomsWithMessages[normalizedRoom] = true
    }
    s.messageQueue.Unlock()

    // 第三步：确定要删除的房间
    var roomsToDelete []string
    currentTime := time.Now().Unix()
    
    s.roomStatsMutex.Lock()
    for room, stat := range s.roomStats {
        hasConnections := activeRooms[room]
        hasMessages := roomsWithMessages[room]
        
        // 不要删除默认房间的统计
        if room == "default" {
            continue
        }
        
        if !hasConnections && !hasMessages {
            timeSinceLastActive := currentTime - stat.LastActive
            if timeSinceLastActive > int64(s.config.Server.RoomCleanup) {
                roomsToDelete = append(roomsToDelete, room)
            }
        }
    }

    // 第四步：删除房间统计
    for _, room := range roomsToDelete {
        delete(s.roomStats, room)
        s.logger.Printf("已清理空房间统计: %s", room)
    }
    s.roomStatsMutex.Unlock()

    if len(roomsToDelete) > 0 {
        s.logger.Printf("房间清理完成，共清理 %d 个空房间", len(roomsToDelete))
    } else {
        s.logger.Println("房间清理完成，没有发现需要清理的空房间")
    }
}

// normalizeRoomName 统一房间名称处理
// 空字符串和"default"都转换为"default"，其他保持不变
func normalizeRoomName(room string) string {
    if room == "" {
        return "default"
    }
    return room
}