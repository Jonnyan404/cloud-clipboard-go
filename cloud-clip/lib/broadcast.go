package lib

import (
	"net/http"
	"strings"
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

// messageSource 一条消息的来源信息。
//
// 为什么要有它：这些字段原本是直接从 *http.Request 上现取的，但定时任务**没有请求** ——
// 它由调度器在进程内发起。抽成这个结构之后，「HTTP 投递」和「定时投递」共用同一条
// 入队 → 房间统计 → 广播 → 持久化路径，不会出现「定时消息漏了房间统计」这种
// 只在其中一条路径上存在的差异（这类差异最难受：两边单看都对，对比才发现不一样）。
type messageSource struct {
	IP         string
	UserAgent  string
	DeviceName string
	ClientID   string

	// Source 标记来源类别（空 = 人发的）。目前只有 "automation"。
	Source string
	// ScheduledAt 是定时任务的**预定**触发时刻；Late 标记这是一条补发。
	ScheduledAt int64
	Late        bool
}

func messageSourceFromRequest(r *http.Request) messageSource {
	return messageSource{
		IP:         get_remote_ip(r),
		UserAgent:  r.UserAgent(),
		DeviceName: resolveDeviceName(r),
		// 前端每客户端持久 ID,用于气泡收发归属
		ClientID: strings.TrimSpace(r.URL.Query().Get("client")),
	}
}

// senderDevice 组装 SenderDevice。
//
// ⚠️ 没有 UA 的来源（定时任务）**不能**去调 parse_user_agent：那会拿到
// `"os": " "` / `"browser": " "` 这种带空格的脏值，而前端的 deviceLabel 取值顺序是
// name → os → type —— 于是定时消息会被显示成一个空格。
func (s *ClipboardServer) senderDevice(src messageSource) map[string]string {
	if strings.TrimSpace(src.UserAgent) == "" {
		return map[string]string{"name": src.DeviceName, "type": "Automation"}
	}
	return s.parse_user_agent(src.UserAgent, src.DeviceName)
}

// addMessageToQueueAndBroadcast 添加消息到队列并广播（HTTP 路径 —— 人发的消息）。
// 这是一个辅助函数，供 handle_text, handle_finish 等调用
func (s *ClipboardServer) addMessageToQueueAndBroadcast(dataType string, data interface{}, room string, r *http.Request) PostEvent {
	return s.deliverMessage(dataType, data, room, messageSourceFromRequest(r), true)
}

// deliverMessage 投递一条消息。
//
// keepHistory=false（定时消息的默认档）时**不入历史队列、不计房间统计**，只做实时广播。
// 为什么这是默认值：房间历史是**按房间**计数的（见 msg.go 的 trimRoomHistoryLocked），
// 额度就是 server.history（README 的 MESSAGE_NUM 默认 10，config.json 默认 100）。
// 一个每天 09:30 的任务，十几天就能把这个房间的历史全换成「今天是几号」，
// 用户翻记录什么都找不到了 —— 那不是「功能不好用」，是「把已有数据弄坏了」。
func (s *ClipboardServer) deliverMessage(dataType string, data interface{}, room string, src messageSource, keepHistory bool) PostEvent {
	// Create ReceiveBase first
	receiveBase := ReceiveBase{
		// ID will be set by PostList.Append（临时消息在下面单独分配）
		Type:           dataType, // This is the inner type for ReceiveHolder (e.g., "text", "file")
		Room:           room,
		Timestamp:      time.Now().Unix(),
		SenderIP:       src.IP,
		SenderDevice:   s.senderDevice(src),
		SenderClientID: src.ClientID,
		Source:         src.Source,
		ScheduledAt:    src.ScheduledAt,
		Late:           src.Late,
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
		fileRec.ReceiveBase = receiveBase // Set the common base
		rh.FileReceive = fileRec
	default:
		s.logger.Printf("警告: deliverMessage 收到未知数据类型: %s", dataType)
		return PostEvent{}
	}

	// 内部存储的事件
	storeEvent := PostEvent{
		Event: dataType, // "text" 或 "file"
		Data:  rh,       // ReceiveHolder
	}

	if keepHistory {
		s.messageQueue.Append(&storeEvent) // msg.go 处理这个 PostEvent
		s.updateRoomStats(room, 1)
	} else {
		// 临时消息也要有唯一 ID：前端拿它做列表 key，也会用它发起「复制 / 引用」。
		// 用**同一个计数器**分配（只是不进 List），ID 就不会和普通消息撞。
		storeEvent.Data.SetID(s.messageQueue.NextEphemeralID())
	}

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
		s.broadcastWebSocketMessage(wsMsg, room)
	}

	if keepHistory {
		s.saveHistoryData()
	}
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
