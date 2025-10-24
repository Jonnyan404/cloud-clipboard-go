package mobile

import (
	"github.com/jonnyan404/cloud-clipboard-go/cloud-clip/lib"
)

// Service 是移动平台使用的主要服务类
type Service struct {
	service *lib.CloudClipboardService
}

// NewService 创建并返回一个新的服务实例
func NewService() *Service {
	return &Service{
		service: lib.NewClipboardService(),
	}
}

// StartServer 启动云剪贴板服务器
// 返回空字符串表示成功，否则返回错误信息
func (s *Service) StartServer(configPath string, host string, port int, authPassword string,
	storageDir string, historyFile string) string {
	return s.service.StartServer(configPath, host, port, authPassword, storageDir, historyFile)
}

// StopServer 停止云剪贴板服务器
// 返回空字符串表示成功，否则返回错误信息
func (s *Service) StopServer() string {
	return s.service.StopServer()
}

// IsRunning 检查服务器是否正在运行
func (s *Service) IsRunning() bool {
	return s.service.IsRunning()
}

// GetServerAddress 获取服务器地址
func (s *Service) GetServerAddress() string {
	return s.service.GetServerAddress()
}

// GetVersion 获取服务器版本
func GetVersion() string {
	return lib.ServerVersion
}
