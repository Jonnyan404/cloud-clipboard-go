package main

/**
*** FILE: history.go
***   handle history.json <===> messageQueue, uploadFileMap
**/

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// file item in File[]
type File struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Size       int    `json:"size"`
	UploadTime int64  `json:"uploadTime"`
	ExpireTime int64  `json:"expireTime"`
}

// History represents the entire JSON structure
type History struct {
	File    []File          `json:"file"`
	Receive []ReceiveHolder `json:"receive"` // Interface to handle both text and file types
}

// ----------------- history api
//
// 原来的默认值:
// var history_path = "history.json"
// var storage_folder = "./uploads"

// 修改为使用配置文件中的值:
var history_path = func() string {
	if config.Server.HistoryFile != "" {
		return config.Server.HistoryFile
	}
	return "history.json"
}()

var storage_folder = func() string {
	if config.Server.StorageDir != "" {
		// 确保目录存在
		os.MkdirAll(config.Server.StorageDir, 0755)
		return config.Server.StorageDir
	}
	// 确保默认目录存在
	os.MkdirAll("./uploads", 0755)
	return "./uploads"
}()

var uploadFileMap = make(map[string]File)
var messageQueue = &PostList{nextid: 0, history_len: config.Server.History}

func _load_history(hist_path string) *History {
	json_bytes, _ := os.ReadFile(hist_path)
	if len(json_bytes) == 0 {
		log.Println("++ history.json is empty or non-exist, will create")
		json_bytes = []byte("{}")
	}

	var payload History
	err := json.Unmarshal([]byte(json_bytes), &payload)
	if err != nil {
		// log.Fatalf("_load_history: Error parsing JSON: %v", err)
		log.Fatalf("_load_history: Error parsing JSON: %v", err)
	}

	return &payload
}
func _save_history(hist_path string, payload *History) {
	out, err := json.MarshalIndent(payload, "", "\t")
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	if err := os.WriteFile(hist_path, out, 0644); err != nil {
		log.Fatalf("Error writing JSON to file: %v", err)
	}
}

func append_msg(history *History, msg interface{}) {
	var holder ReceiveHolder
	switch v := msg.(type) {
	case TextReceive:
		holder.TextReceive = &v
	case FileReceive:
		holder.FileReceive = &v
	default:
		log.Fatalf("Unknown message type: %T", v)
	}
	history.Receive = append(history.Receive, holder)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// filter-out file expired or file no-exists msg
func filter_msg(history *History) {
	now := time.Now().Unix()

	// Create a new slice to hold the filtered messages
	var filteredReceive []ReceiveHolder

	for _, receiveItem := range history.Receive {
		if receiveItem.FileReceive != nil {
			fileReceive := receiveItem.FileReceive
			if fileReceive.Expire < now {
				continue // Skip expired files
			}
			if !pathExists(fileReceive.Name) {
				continue // Skip files that do not exist
			}
		}
		filteredReceive = append(filteredReceive, receiveItem)
	}

	// Update the payload with the filtered messages
	history.Receive = filteredReceive
}

// history.json => msg_que,uploadFileMap
func load_history() {
	history := _load_history(history_path)
	current_time := time.Now().Unix()

	// load to uploadFileMap
	for _, file := range history.File {
		if pathExists(filepath.Join(storage_folder, file.UUID)) && file.ExpireTime > current_time {
			uploadFileMap[file.UUID] = file
		} else {
			uuid := file.UUID
			fmt.Println("- del0:", uuid)
			os.Remove(filepath.Join(storage_folder, uuid))
		}
	}

	// load to msg_que
	for _, msg := range history.Receive {
		if msg.Type() == "file" && uploadFileMap[msg.FileReceive.Cache] == (File{}) {
			continue
		}

		// fmt.Println(" --load hist item:", msg)
		messageQueue.Append(&PostEvent{
			Event: "receive",
			Data:  msg,
		})
	}
}

func save_history() {
	current_time := time.Now().Unix()

	filteredFiles := []File{}
	for _, file := range uploadFileMap {
		if file.ExpireTime > current_time {
			filteredFiles = append(filteredFiles, file)
		}
	}

	filteredMessages := []ReceiveHolder{}
	for _, message := range messageQueue.List {
		if message.Data.Type() != "file" || message.Data.FileReceive.Expire > current_time {
			filteredMessages = append(filteredMessages, message.Data)
		}
	}
	historyData := History{
		File:    filteredFiles,
		Receive: filteredMessages,
	}

	file, _ := json.MarshalIndent(historyData, "", "\t")
	_ = os.WriteFile(history_path, file, 0644)
}

func get_all_msg(history *History) []interface{} {
	var msg_list []interface{}

	for _, receiveItem := range history.Receive {
		if receiveItem.TextReceive != nil {
			msg_list = append(msg_list, *receiveItem.TextReceive)
		} else if receiveItem.FileReceive != nil {
			msg_list = append(msg_list, *receiveItem.FileReceive)
		}
	}

	return msg_list
}

/*

func main() {
	// JSON data
	jsonData, err := os.ReadFile("history.test.json")
	if err != nil {
		log.Fatalf("Error reading JSON file: %v", err)
	}

	// Parse the JSON
	var payload Payload
	err := json.Unmarshal([]byte(jsonData), &payload)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	fmt.Println("--litter.dump:")
	litter.Dump(payload)

	// Store file data in file_list
	file_list := payload.File

	// Store receive data as concrete types in msg_list
	var msg_list []interface{} // Using interface{} to hold both TextReceive and FileReceive

	for _, receiveItem := range payload.Receive {
		if receiveItem.TextReceive != nil {
			msg_list = append(msg_list, *receiveItem.TextReceive)
		} else if receiveItem.FileReceive != nil {
			msg_list = append(msg_list, *receiveItem.FileReceive)
		}
	}

	// Print file_list and msg_list
	fmt.Println("File List:")
	for _, file := range file_list {
		fmt.Printf("%+v\n", file)
	}

	fmt.Println("\nMessage List:")
	for _, msg := range msg_list {
		switch v := msg.(type) {
		case TextReceive:
			fmt.Printf("Text Message: %+v\n", v)
		case FileReceive:
			fmt.Printf("File Message: %+v\n", v)
		}
	}

	// out, err := json.Marshal(payload)
	out, err := json.MarshalIndent(payload, "", "\t")
	fmt.Println("---json:", string(out))

	msg_list = append(msg_list, TextReceive{ReceiveBase: ReceiveBase{ID: 123, Type: "text", Room: "room1"}, Content: "ping"})
	out, err = json.MarshalIndent(msg_list, "", "\t")
	fmt.Println("---json:", string(out))
}


//*/
