package lib

import (
	"fmt"
	"log"
	"net"
	// 引入 os 包
)

// 导出给移动平台使用的变量
var (
	ServerVersion        = server_version // 原server_version重新导出
	EffectiveUseEmbedded = true           // 默认启用嵌入式文件
)

// CloudClipboardService 为移动平台提供的服务接口
type CloudClipboardService struct {
	server    *ClipboardServer
	config    *Config
	isRunning bool
}

// NewClipboardService 创建一个新的服务实例
func NewClipboardService() *CloudClipboardService {
	return &CloudClipboardService{}
}

// StartServer 启动服务器
func (s *CloudClipboardService) StartServer(
	configPath string, // 新增 configPath 参数
	host string,
	port int,
	authPassword string,
	storageDir string,
	historyFile string) string {

	if s.isRunning {
		return "服务已在运行中"
	}

	// 尝试从路径加载配置
	cfg, err := load_config(configPath)
	if err != nil {
		log.Printf("警告: 从 %s 加载配置失败: %v。将使用默认值和参数。", configPath, err)
		cfg = defaultConfig() // 加载失败则使用默认配置
	} else {
		log.Printf("配置文件 %s 加载成功。", configPath)
	}

	// 无论配置文件是否加载成功，都使用从 Android UI 传递过来的核心参数覆盖
	// 这确保了主界面的设置具有最高优先级
	cfg.Server.Port = port
	if authPassword != "" {
		cfg.Server.Auth = authPassword
	} else {
		// 如果密码为空，确保禁用认证
		cfg.Server.Auth = false
	}
	if storageDir != "" {
		cfg.Server.StorageDir = storageDir
	}
	if historyFile != "" {
		cfg.Server.HistoryFile = historyFile
	}

	// 创建服务器实例
	server, err := NewClipboardServer(cfg)
	if err != nil {
		return fmt.Sprintf("创建服务器失败: %v", err)
	}

	// 保存引用
	s.server = server
	s.config = cfg

	// 在一个新的 goroutine 中启动服务器，以避免阻塞
	go func() {
		if err := server.Start(); err != nil {
			// 如果服务器启动失败，记录日志
			// 注意：这里的错误无法直接返回给调用者，因为已经异步
			log.Printf("错误: 异步启动服务器失败: %v", err)
			s.isRunning = false
		}
	}()

	s.isRunning = true
	return "" // 成功返回空字符串
}

// StopServer 停止服务器
func (s *CloudClipboardService) StopServer() string {
	if !s.isRunning || s.server == nil {
		return "服务未运行"
	}

	if err := s.server.Stop(); err != nil {
		return "停止服务器失败: " + err.Error()
	}

	s.isRunning = false
	return "" // 成功返回空字符串
}

// GetServerAddress 获取服务器地址
func (s *CloudClipboardService) GetServerAddress() string {
	if !s.isRunning || s.server == nil || s.config == nil {
		return ""
	}

	port := s.config.Server.Port

	// 获取本地 IPv4 地址
	localIP := getLocalIPv4()
	if localIP != "" {
		return fmt.Sprintf("http://%s:%d", localIP, port)
	}

	// 如果无法获取本地 IP,返回 0.0.0.0
	return fmt.Sprintf("http://0.0.0.0:%d", port)
}

// IsRunning 检查服务器是否在运行
func (s *CloudClipboardService) IsRunning() bool {
	return s.isRunning
}

// getLocalIPv4 获取本地 IPv4 地址
func getLocalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		// 检查是否是 IP 地址
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			// 只返回 IPv4 地址
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return ""
}
