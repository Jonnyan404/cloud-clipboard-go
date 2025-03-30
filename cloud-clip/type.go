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
// type ReceiveBase struct {
// 	ID   int    `json:"id"`
// 	Type string `json:"type"`
// 	Room string `json:"room"`
// }

// "text" type item in Receive[]
type TextReceive struct {
	// ReceiveBase
	ID   int    `json:"id"`
	Type string `json:"type"`
	Room string `json:"room"`

	Content string `json:"content"`
}

// "file" type item in Receive[]
type FileReceive struct {
	// ReceiveBase
	ID   int    `json:"id"`
	Type string `json:"type"`
	Room string `json:"room"`

	Name      string `json:"name"`
	UUID      string `json:"uuid"`
	Size      int    `json:"size"`
	Cache     string `json:"cache"`
	Expire    int64  `json:"expire"`
	Thumbnail string `json:"thumbnail"`
	URL       string `json:"url,omitempty"`
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
		return fmt.Errorf("unknown type: %v", raw["type"])
	}

	return nil
}

// Custom JSON marshaler for ReceiveBaseHolder
func (r ReceiveHolder) MarshalJSON() ([]byte, error) {
	if r.TextReceive != nil {
		return json.Marshal(r.TextReceive)
	} else if r.FileReceive != nil {
		return json.Marshal(r.FileReceive)
	}
	return nil, fmt.Errorf("no valid receive type found in ReceiveBaseHolder")
}
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
