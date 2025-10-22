package lib

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

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
	s.logger.Printf("广播 WebSocket 消消息 (类型: %s) 到房间 '%s'", message.Event, room)

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
