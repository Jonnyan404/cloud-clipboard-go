package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ua-parser/uap-go/uaparser"
)

/**
*** FILE: type.go
***   handle receive type for messageQueue, history
**/
// WebSocketMessage 是专门用于通过 WebSocket 发送给前端的结构
type WebSocketMessage struct {
	Event string      `json:"event"` // 将是 "receive", "config", "connect", "disconnect", "revoke", "clearAll" 等
	Data  interface{} `json:"data"`  // 将是前端期望的直接载荷，如 *TextReceive, *FileReceive, DeviceMeta, map[string]string 等
}

type PostEvent struct {
	Event string        `json:"event"`
	Data  ReceiveHolder `json:"data"`
}

type PostList struct {
	sync.Mutex
	nextid      int
	history_len int

	List []PostEvent `json:"receive"`
}

type PostData struct {
	IP            string       `json:"ip,omitempty"`
	DeviceType    string       `json:"device_type,omitempty"`
	DeviceOS      string       `json:"device_os,omitempty"`
	Browser       string       `json:"browser,omitempty"`
	Text          string       `json:"text,omitempty"`
	FileReceive   *FileReceive `json:"fileReceive,omitempty"`
	TimestampUnix int64        `json:"timestamp"`
	// 用于设备连接/断开连接事件的字段
	DeviceConnection *DeviceMeta `json:"deviceConnection,omitempty"` // 用于连接事件
	DeviceID         string      `json:"deviceID,omitempty"`         // 用于断开连接事件
}

// DeviceMeta 保存连接设备的信息
type DeviceMeta struct {
	ID      string `json:"id"`      // 设备ID
	Type    string `json:"type"`    // 例如："Desktop", "Mobile"
	Device  string `json:"device"`  // 例如："Apple Mac", "iPhone"
	OS      string `json:"os"`      // 例如："macOS 14", "iOS 17"
	Browser string `json:"browser"` // 例如："Chrome 120"
}

// ClipboardServer 结构体定义
type ClipboardServer struct {
	config          *Config
	httpServer      *http.Server
	logger          *log.Logger
	messageQueue    *PostList
	websockets      map[*websocket.Conn]bool
	room_ws         map[*websocket.Conn]string
	uploadFileMap   map[string]File       // 从 history.go 的全局变量迁移过来
	deviceConnected map[string]DeviceMeta // 更改为将 deviceID 映射到 DeviceMeta
	storageFolder   string
	historyFilePath string
	isRunning       bool
	connDeviceIDMap map[*websocket.Conn]string
	runMutex        sync.Mutex
	parser          *uaparser.Parser // UA解析器实例
	deviceHashSeed  uint32           // 将 deviceHashSeed 添加到服务器实例
}

// file item in File[]
type File struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Size       int64  `json:"size"`
	UploadTime int64  `json:"uploadTime"`
	ExpireTime int64  `json:"expireTime"`
}

// History represents the entire JSON structure
type History struct {
	File    []File          `json:"file"`
	Receive []ReceiveHolder `json:"receive"`
	NextID  int             `json:"nextId,omitempty"` // 新增，用于保存消息队列的下一个ID
}

// ReceiveBase is the common structure for all receive types
type ReceiveBase struct {
	ID           int               `json:"id"`
	Type         string            `json:"type"`
	Room         string            `json:"room"`
	Timestamp    int64             `json:"timestamp"`    // Unix timestamp (seconds)
	SenderIP     string            `json:"senderIP"`     // 发送者 IP 地址
	SenderDevice map[string]string `json:"senderDevice"` // 发送者设备信息 (来自 User-Agent 解析)
}

// "text" type item in Receive[]
type TextReceive struct {
	ReceiveBase        // 嵌入基础结构
	Content     string `json:"content,omitempty"`
	// 为设备连接/断开事件添加字段
	DeviceConnection *DeviceMeta `json:"deviceConnection,omitempty"` // 新增字段
	DeviceID         string      `json:"deviceID,omitempty"`         // 新增字段 (用于断开连接)
}

// "file" type item in Receive[]
type FileReceive struct {
	ReceiveBase        // 嵌入基础结构
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Cache       string `json:"cache"` // Cache 通常就是 UUID
	Expire      int64  `json:"expire"`
	Thumbnail   string `json:"thumbnail"`
	URL         string `json:"url,omitempty"` // 新增 URL 字段
	// 也可以在这里为设备事件添加字段以保持对称性，如果需要的话
	// DeviceConnection *DeviceMeta `json:"deviceConnection,omitempty"`
	// DeviceID         string      `json:"deviceID,omitempty"`
}

// holds either a TextReceive or a FileReceive
type ReceiveHolder struct {
	TextReceive *TextReceive
	FileReceive *FileReceive
}

// ----------------- json enc/dec

// custom unmarshalling for ReceiveBaseHolder
func (r *ReceiveHolder) UnmarshalJSON(data []byte) error {
	// unmarshall for type field
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// "type" field decide TextReceive or FileReceive
	switch raw["type"] {
	case "text":
		var textReceive TextReceive
		if err := json.Unmarshal(data, &textReceive); err != nil {
			return err
		}
		r.TextReceive = &textReceive
	case "file":
		var fileReceive FileReceive
		if err := json.Unmarshal(data, &fileReceive); err != nil {
			return err
		}
		r.FileReceive = &fileReceive
	default:
		// Try unmarshalling into ReceiveBase just to check if it's a valid base structure
		var base ReceiveBase
		if errBase := json.Unmarshal(data, &base); errBase == nil && base.Type != "" {
			// It might be a type we don't specifically handle here, but has the base fields.
			// Decide if you want to allow this or return an error.
			// For now, let's return an error for unknown specific types.
			return fmt.Errorf("unknown specific message type: %v", raw["type"])
		}
		return fmt.Errorf("unknown message type or invalid structure: %v", raw["type"])

	}

	return nil
}

// Custom JSON marshaler for ReceiveHolder
func (r ReceiveHolder) MarshalJSON() ([]byte, error) {
	if r.TextReceive != nil {
		return json.Marshal(r.TextReceive)
	} else if r.FileReceive != nil {
		return json.Marshal(r.FileReceive)
	}
	// Return null or an empty object instead of an error if appropriate
	// return []byte("null"), nil
	return nil, fmt.Errorf("no valid receive type found in ReceiveHolder")
}

// --- Helper methods for ReceiveHolder ---

func (r *ReceiveHolder) SetID(id int) int {
	if r.TextReceive != nil {
		r.TextReceive.ID = id
		return id
	} else if r.FileReceive != nil {
		r.FileReceive.ID = id
		return id
	}
	return -1
}

func (r *ReceiveHolder) ID() int {
	if r.TextReceive != nil {
		return r.TextReceive.ID
	} else if r.FileReceive != nil {
		return r.FileReceive.ID
	}
	return -1
}

func (r *ReceiveHolder) Type() string {
	if r.TextReceive != nil {
		return r.TextReceive.Type
	} else if r.FileReceive != nil {
		return r.FileReceive.Type
	}
	return ""
}

func (r *ReceiveHolder) Room() string {
	if r.TextReceive != nil {
		return r.TextReceive.Room
	} else if r.FileReceive != nil {
		return r.FileReceive.Room
	}
	return ""
}

// Add getters for the new fields if needed, accessing via the embedded ReceiveBase
func (r *ReceiveHolder) Timestamp() int64 {
	if r.TextReceive != nil {
		return r.TextReceive.Timestamp
	} else if r.FileReceive != nil {
		return r.FileReceive.Timestamp
	}
	return 0
}

func (r *ReceiveHolder) SenderIP() string {
	if r.TextReceive != nil {
		return r.TextReceive.SenderIP
	} else if r.FileReceive != nil {
		return r.FileReceive.SenderIP
	}
	return ""
}

func (r *ReceiveHolder) SenderDevice() map[string]string {
	if r.TextReceive != nil {
		return r.TextReceive.SenderDevice
	} else if r.FileReceive != nil {
		return r.FileReceive.SenderDevice
	}
	return nil
}
