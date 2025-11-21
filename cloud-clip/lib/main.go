package lib

import (
	"context" // 确保导入 embed 包
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os" // 确保导入 os 包
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
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

	// 转换为绝对路径用于日志显示
	absStorageFolder, err := filepath.Abs(storageFolder)
	if err != nil {
		logger.Printf("警告: 无法获取存储目录的绝对路径: %v，使用原始路径: %s", err, storageFolder)
		absStorageFolder = storageFolder
	}

	if err := os.MkdirAll(storageFolder, 0755); err != nil {
		logger.Printf("无法创建存储目录 %s: %v", absStorageFolder, err)
		// 根据需求，这里可以是致命错误
		// return nil, fmt.Errorf("无法创建存储目录 %s: %w", absStorageFolder, err)
	} else {
		logger.Printf("存储目录设置为: %s", absStorageFolder)
	}

	historyFilePath := filepath.Join(storageFolder, "history.json")
	if cfg.Server.HistoryFile != "" {
		historyFilePath = cfg.Server.HistoryFile
	} else {
		cfg.Server.HistoryFile = historyFilePath // 更新配置对象中的路径
		logger.Printf("历史文件路径未指定，使用默认: %s", historyFilePath)
	}

	// 转换为绝对路径用于日志显示
	absHistoryFilePath, err := filepath.Abs(historyFilePath)
	if err != nil {
		logger.Printf("警告: 无法获取历史文件的绝对路径: %v，使用原始路径: %s", err, historyFilePath)
		absHistoryFilePath = historyFilePath
	}
	logger.Printf("历史文件路径设置为: %s", absHistoryFilePath)

	mqHistoryLen := 100 // 默认历史长度
	if cfg.Server.History > 0 {
		mqHistoryLen = cfg.Server.History
	}
	// 修改：传入 logger 以便在淘汰消息时打印日志
	mq := NewMessageQueue(mqHistoryLen, logger)

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

	// 显示绝对路径
	absStorageFolder, err1 := filepath.Abs(s.storageFolder)
	if err1 != nil {
		absStorageFolder = s.storageFolder
	}
	s.logger.Printf("存储目录: %s", absStorageFolder)

	absHistoryFilePath, err2 := filepath.Abs(s.historyFilePath)
	if err2 != nil {
		absHistoryFilePath = s.historyFilePath
	}
	s.logger.Printf("历史文件: %s", absHistoryFilePath)

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

// hash_murmur3 函数 (假设可用，例如来自 random.go 或工具文件)
// 如果没有，需要定义或导入。例如：
func hash_murmur3(data []byte, seed uint32) uint32 {
	h := murmur3.New32WithSeed(seed) // murmur3 来自 "github.com/spaolacci/murmur3"
	h.Write(data)
	return h.Sum32()
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
