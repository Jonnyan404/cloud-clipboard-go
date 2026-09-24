package lib

/**
*** FILE: config.go
***   handle config.json <===> Config
**/

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	Server struct {
		Host        interface{} `json:"host"`        //done
		Port        int         `json:"port"`        //done
		Prefix      string      `json:"prefix"`      //done
		History     int         `json:"history"`     //done
		HistoryFile string      `json:"historyFile"` // 添加历史文件路径
		StorageDir  string      `json:"storageDir"`  // 添加存储目录路径
		// Auth    string `json:"auth"`
		Auth     interface{}    `json:"auth"` //done
		RoomAuth RoomAuthConfig `json:"roomAuth"`
		Cert     string         `json:"cert"`
		Key      string         `json:"key"`

		// 添加房间相关配置
		RoomList    bool `json:"roomList"`    // 是否启用房间列表功能
		RoomCleanup int  `json:"roomCleanup"` // 房间清理间隔（秒）
	} `json:"server"`
	Text struct {
		Limit int `json:"limit"` //done
	} `json:"text"`
	File struct {
		Expire int `json:"expire"` //done
		Chunk  int `json:"chunk"`  //done, but no limit
		Limit  int `json:"limit"`  //done
	} `json:"file"`
	// Automation 定时自动化（tasks.json）。
	//
	// ⚠️ Enabled 默认 true，但「默认可用」不等于「默认放开」：能不能给**某个房间**装任务
	// 由 roomAuth[x].automation 决定，公开房间默认是 none（见 auth.go 的
	// resolveAutomationPolicy）。所以默认开着不会让任何房间凭空多出自动化能力。
	Automation struct {
		Enabled      bool   `json:"enabled"`
		TickSeconds  int    `json:"tickSeconds"`
		GraceSeconds int    `json:"graceSeconds"`
		DefaultTZ    string `json:"defaultTZ"`
	} `json:"automation"`
}

// var config_path = "config.json"

func load_config(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config.json" // 默认配置文件名
	}
	log.Printf("尝试从以下路径加载配置文件: %s\n", configPath)

	defaultConf := defaultConfig()
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("无法读取配置文件 '%s': %v. 将使用默认配置.\n", configPath, err)
		// 将默认配置写入文件，如果它不存在或无法读取
		defaultData, marshalErr := json.MarshalIndent(defaultConf, "", "  ")
		if marshalErr == nil {
			if writeErr := os.WriteFile(configPath, defaultData, 0644); writeErr == nil {
				log.Printf("已将默认配置写入: %s\n", configPath)
			} else {
				log.Printf("无法写入默认配置文件 '%s': %v\n", configPath, writeErr)
			}
		}
		return defaultConf, nil // 返回默认配置，不视为致命错误
	}

	// 使用默认配置作为基础，然后用文件中的值覆盖它
	conf := defaultConfig()
	if err := json.Unmarshal(data, conf); err != nil {
		log.Printf("无法解析配置文件 '%s': %v. 将使用默认配置.\n", configPath, err)
		return defaultConf, nil // 返回默认配置，不视为致命错误
	}

	log.Printf("配置文件 %s 加载成功。\n", configPath)
	return conf, nil
}

func defaultConfig() *Config {
	// 检测是否在 OpenWrt 环境中运行
	historyFile := "history.json"
	storageDir := "./uploads"

	// 如果配置文件路径包含 /etc/cloud-clipboard，则认为是 OpenWrt 环境
	if isOpenWrtEnv() {
		historyFile = "/etc/cloud-clipboard/data/history.json"
		storageDir = "/etc/cloud-clipboard/data/upload"
	}

	return &Config{
		Server: struct {
			Host        interface{}    `json:"host"`
			Port        int            `json:"port"`
			Prefix      string         `json:"prefix"`
			History     int            `json:"history"`
			HistoryFile string         `json:"historyFile"`
			StorageDir  string         `json:"storageDir"`
			Auth        interface{}    `json:"auth"`
			RoomAuth    RoomAuthConfig `json:"roomAuth"`
			Cert        string         `json:"cert"`
			Key         string         `json:"key"`
			RoomList    bool           `json:"roomList"`
			RoomCleanup int            `json:"roomCleanup"`
		}{
			Host:        []string{"0.0.0.0"},
			Port:        9501,
			Prefix:      "",
			History:     100,
			HistoryFile: historyFile,
			StorageDir:  storageDir,
			Auth:        false,
			RoomAuth:    RoomAuthConfig{},
			Cert:        "",
			Key:         "",
			RoomList:    false, // 默认关闭房间列表功能
			RoomCleanup: 3600,  // 默认1小时清理一次空房间
		},
		Text: struct {
			Limit int `json:"limit"`
		}{
			Limit: 4096,
		},
		File: struct {
			Expire int `json:"expire"`
			Chunk  int `json:"chunk"`
			Limit  int `json:"limit"`
		}{
			Expire: 3600,
			Chunk:  1 * _MB,
			Limit:  256 * _MB,
		},
		Automation: struct {
			Enabled      bool   `json:"enabled"`
			TickSeconds  int    `json:"tickSeconds"`
			GraceSeconds int    `json:"graceSeconds"`
			DefaultTZ    string `json:"defaultTZ"`
		}{
			Enabled:      true,
			TickSeconds:  30,
			GraceSeconds: 600,
			// 默认上海：这个项目的用户绝大多数在 +08:00，而「服务器本地时区」在容器里
			// 常常是 UTC —— 一个「每天 09:30」的任务会因此在 17:30 发出去，
			// 而且只有真到了那一天才看得出来。默认值选「用户很可能在的时区」，
			// 比默认「服务器的时区」安全得多。
			DefaultTZ: defaultAutomationTZName,
		},
	}
}

// 检测是否在 OpenWrt 环境
func isOpenWrtEnv() bool {
	// 方法1: 检查特定文件是否存在
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		return true
	}

	// 方法2: 检查环境变量
	if os.Getenv("OPENWRT_ENV") == "1" {
		return true
	}

	// 方法3: 检查是否从标准路径启动
	if _, err := os.Stat("/etc/config/cloud-clipboard"); err == nil {
		return true
	}

	return false
}
