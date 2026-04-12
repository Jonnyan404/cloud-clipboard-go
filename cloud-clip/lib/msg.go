package lib

import "log" // 新增：导入 log 包

/**
*** FILE: msg.go
***   handle messageQueue
**/

// 修改：增加 logger 参数
func NewMessageQueue(historyLen int, logger *log.Logger) *PostList {
	return &PostList{
		nextid:      1, // Start IDs from 1
		history_len: historyLen,
		List:        make([]PostEvent, 0, historyLen),
		logger:      logger, // 新增：赋值 logger
	}
}

func (m *PostList) Append(item *PostEvent) {
	m.Lock()
	defer m.Unlock()

	m.appendLocked(*item)
}

func (m *PostList) appendLocked(item PostEvent) {
	if item.Data.ID() <= 0 { //fill uniq id, thread-safe way
		item.Data.SetID(m.nextid)
	}
	m.List = append(m.List, item)
	m.trimRoomHistoryLocked(item.Data.Room())

	itemID := item.Data.ID()
	if m.nextid <= itemID {
		m.nextid = itemID + 1
	}
}

func (m *PostList) trimRoomHistoryLocked(room string) {
	if m.history_len <= 0 {
		m.List = []PostEvent{}
		return
	}

	normalizedRoom := normalizeRoomName(room)
	roomCount := 0
	for _, msg := range m.List {
		if normalizeRoomName(msg.Data.Room()) == normalizedRoom {
			roomCount++
		}
	}

	for roomCount > m.history_len {
		evictedIndex := -1
		for i, msg := range m.List {
			if normalizeRoomName(msg.Data.Room()) == normalizedRoom {
				m.logEvictedMessage(msg)
				evictedIndex = i
				break
			}
		}

		if evictedIndex == -1 {
			return
		}

		m.List = append(m.List[:evictedIndex], m.List[evictedIndex+1:]...)
		roomCount--
	}
}

func (m *PostList) logEvictedMessage(evicted PostEvent) {
	if m.logger == nil {
		return
	}

	var content string
	if evicted.Data.TextReceive != nil {
		content = evicted.Data.TextReceive.Content
	} else if evicted.Data.FileReceive != nil {
		content = "[文件] " + evicted.Data.FileReceive.Name
	}

	runes := []rune(content)
	if len(runes) > 30 {
		content = string(runes[:30]) + "..."
	}

	m.logger.Printf("房间消息队列已满(%d)，淘汰旧消息: ID=%d, 房间=[%s], 类型=%s, 内容=[%s]",
		m.history_len, evicted.Data.ID(), normalizeRoomName(evicted.Data.Room()), evicted.Event, content)
}

func (m *PostList) ClearAll() {
	m.Lock()
	defer m.Unlock()

	// 清空列表
	m.List = []PostEvent{}
}

// remove array item by index
func (m *PostList) Remove(index int) {
	// m.Lock()
	// defer m.Unlock()
	if index < 0 || index >= len(m.List) {
		return
	}
	m.List = append(m.List[:index], m.List[index+1:]...)
}

// find item index which m.Data[index].Data.ID() == msgId
func (m *PostList) FindId(msgId int) int {
	// m.Lock()
	// defer m.Unlock()
	for i, msg := range m.List {
		if msg.Data.ID() == msgId {
			return i
		}
	}
	return -1 // Return -1 if the message ID is not found
}

// find item by id and remove it from array
func (m *PostList) RemoveById(msgId int) int {
	m.Lock()
	defer m.Unlock()

	index := m.FindId(msgId)
	if index != -1 {
		m.Remove(index)
	}

	return index
}
