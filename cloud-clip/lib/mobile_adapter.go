package lib

import "strconv"

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
func (s *CloudClipboardService) StartServer(host string, port int, authPassword string,
	storageDir string, historyFile string) string {
	if s.isRunning {
		return "服务已在运行中"
	}

	// 创建配置
	cfg := defaultConfig()
	cfg.Server.Host = host
	cfg.Server.Port = port

	// 处理认证
	if authPassword != "" {
		cfg.Server.Auth = authPassword
	}

	// 设置存储目录
	if storageDir != "" {
		cfg.Server.StorageDir = storageDir
	}

	// 设置历史文件路径
	if historyFile != "" {
		cfg.Server.HistoryFile = historyFile
	}

	// 创建服务器实例
	server, err := NewClipboardServer(cfg)
	if err != nil {
		return "创建服务器失败: " + err.Error()
	}

	// 保存引用
	s.server = server
	s.config = cfg

	// 启动服务器
	if err := server.Start(); err != nil {
		return "启动服务器失败: " + err.Error()
	}

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

	host := "0.0.0.0"
	if hostStr, ok := s.config.Server.Host.(string); ok && hostStr != "" {
		host = hostStr
	}

	return host + ":" + strconv.Itoa(s.config.Server.Port)
}

// IsRunning 检查服务器是否在运行
func (s *CloudClipboardService) IsRunning() bool {
	return s.isRunning
}
