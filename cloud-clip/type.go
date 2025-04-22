package main

import (
	"encoding/json"
	"fmt"
)

/**
*** FILE: type.go
***   handle receive type for messageQueue, history
**/

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
	Content     string `json:"content"`
}

// "file" type item in Receive[]
type FileReceive struct {
	ReceiveBase        // 嵌入基础结构
	Name        string `json:"name"`
	Size        int    `json:"size"`
	Cache       string `json:"cache"` // Cache 通常就是 UUID
	Expire      int64  `json:"expire"`
	Thumbnail   string `json:"thumbnail"`
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
