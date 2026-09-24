package lib

// 契约 fixture：把样例值从 **Go 侧**导出成 JSON，供 Rust 侧反序列化断言。
//
// 为什么值得单独一个文件做这件事：
//
//	这套契约的权威是 docs/api.md，但**字段名与 omitempty 的实际效果**只有 Go 的
//	encoding/json 说了算。「读代码觉得一致」和「真的一致」是两件事 —— 后者只能靠
//	拿真实输出校验。所以：Go 生成 → Rust 断言（rust/crates/protocol/tests/go_fixtures.rs）。
//
// 为什么 fixture 入库（cases/protocol/）而不是丢进 Rust 目录：
//
//	它属于**契约**，和 docs/api.md 一个层级。放在仓库根，Go 与 Rust 的测试读同一份
//	（见 clip-sync/ARCHITECTURE.md §5.4 与 §10.3）—— 复制一份到两边必然漂。
//
// 用法：
//
//	go test ./lib -run TestProtocolFixtures           # 校验
//	UPDATE_FIXTURES=1 go test ./lib -run TestProtocolFixtures   # 重新生成
//
// ⚠️ 改了 type.go 的字段名或 omitempty 之后这个测试会红 —— 那是有意的，
// 它是在提醒你：**这是一次契约变更**，Rust 侧要同步，可能还要通知存量客户端。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fixture 落点：仓库根的 cases/protocol/（`go test` 的 cwd 是 cloud-clip/lib）。
const protocolFixtureDir = "../../cases/protocol"

func sampleDeviceMeta() DeviceMeta {
	return DeviceMeta{
		ID:      "dev-1",
		Type:    "Desktop",
		Name:    "客厅的 Mac",
		Device:  "Apple Mac",
		OS:      "macOS 14",
		Browser: "Chrome 120",
	}
}

func sampleTextReceive() TextReceive {
	return TextReceive{
		ReceiveBase: ReceiveBase{
			ID:             7,
			Type:           "text",
			Room:           "work",
			Timestamp:      1758700000,
			SenderIP:       "192.168.1.20",
			SenderClientID: "client-abc",
			SenderDevice: map[string]string{
				"device":  "Apple Mac",
				"os":      "macOS 14",
				"browser": "Chrome 120",
				"type":    "Desktop",
			},
		},
		Content: "hello\nworld",
	}
}

func sampleFileReceive() FileReceive {
	return FileReceive{
		ReceiveBase: ReceiveBase{
			ID:        8,
			Type:      "file",
			Room:      "default",
			Timestamp: 1758700001,
			SenderIP:  "10.0.0.5",
			SenderDevice: map[string]string{
				"device": "iPhone",
				"os":     "iOS 17",
			},
		},
		Name:      "screenshot.png",
		Size:      20480,
		Cache:     "6f1c9d2e-0000-4000-8000-000000000001",
		Expire:    1758703601,
		Thumbnail: "data:image/png;base64,iVBORw0KGgo=",
		URL:       "/file/6f1c9d2e-0000-4000-8000-000000000001/screenshot.png",
	}
}

// fixtures 返回「文件名 → 样例值」。文件名即 Rust 侧的目标类型（见 Rust 测试）。
func protocolFixtures() map[string]any {
	// 最小文本条目：所有 omitempty 字段都是零值 —— 用来钉住「哪些 key 会消失」。
	minimal := TextReceive{
		ReceiveBase: ReceiveBase{ID: 1, Type: "text"},
	}

	// 定时任务发的消息：source / scheduledAt / late / column 全上。
	automation := TextReceive{
		ReceiveBase: ReceiveBase{
			ID:          9,
			Type:        "text",
			Room:        "work",
			Timestamp:   1758700200,
			SenderIP:    "127.0.0.1",
			Column:      "doing",
			Source:      "automation",
			ScheduledAt: 1758700000,
			Late:        true,
		},
		Content: "每日提醒",
	}

	// 看板挪列过的文件条目：column 在两个分支上都要能序列化。
	fileWithColumn := sampleFileReceive()
	fileWithColumn.Column = "done"

	// 设备断开事件：走的是 TextReceive 的 deviceID 分支。
	disconnect := TextReceive{
		ReceiveBase: ReceiveBase{ID: 10, Type: "text", Room: "default", Timestamp: 1758700300},
		DeviceID:    "dev-1",
	}

	// nil senderDevice：Go 会写成 `null`（不是省略、也不是 `{}`）—— 三种情况必须分得开。
	nilSenderDevice := TextReceive{
		ReceiveBase: ReceiveBase{
			ID:           11,
			Type:         "text",
			Room:         "default",
			Timestamp:    1758700400,
			SenderIP:     "192.168.1.9",
			SenderDevice: nil,
		},
		Content: "no device info",
	}

	return map[string]any{
		"device_meta":             sampleDeviceMeta(),
		"device_meta_no_name":     DeviceMeta{ID: "dev-2", Type: "Mobile", Device: "iPhone", OS: "iOS 17", Browser: "Safari"},
		"text_receive":            sampleTextReceive(),
		"text_receive_min":        minimal,
		"text_receive_auto":       automation,
		"text_receive_disconnect": disconnect,
		"text_receive_nil_device": nilSenderDevice,
		"file_receive":            sampleFileReceive(),
		"file_receive_column":     fileWithColumn,
		"post_event_text":         PostEvent{Event: "receive", Data: ReceiveHolder{TextReceive: ptr(sampleTextReceive())}},
		"post_event_file":         PostEvent{Event: "receive", Data: ReceiveHolder{FileReceive: ptr(sampleFileReceive())}},
		"history": History{
			File:    []File{{Name: "a.png", UUID: "u-1", Size: 12, UploadTime: 1, ExpireTime: 2, Room: "work"}},
			Receive: []ReceiveHolder{{TextReceive: ptr(sampleTextReceive())}, {FileReceive: ptr(sampleFileReceive())}},
			NextID:  12,
		},
		// nil 切片：Go 写成 `null`，Rust 侧必须能读进来（读得宽松）。
		"history_empty": History{},
		"room_info": RoomInfo{
			Name: "work", MessageCount: 3, DeviceCount: 2, LastActive: 1758700000,
			IsActive: true, IsProtected: true,
		},
		"room_list": RoomListResponse{Rooms: []RoomInfo{
			{Name: "default", MessageCount: 1, DeviceCount: 1, LastActive: 1758700000, IsActive: true},
			{Name: "work", MessageCount: 3, DeviceCount: 0, LastActive: 1758700100, IsProtected: true},
		}},
		"ws_connect": WebSocketMessage{Event: "connect", Data: sampleDeviceMeta()},
	}
}

func ptr[T any](v T) *T { return &v }

// TestProtocolFixtures 生成 / 校验上面那份 fixture。
func TestProtocolFixtures(t *testing.T) {
	update := os.Getenv("UPDATE_FIXTURES") == "1"

	if err := os.MkdirAll(protocolFixtureDir, 0o755); err != nil {
		t.Fatalf("创建 fixture 目录失败: %v", err)
	}

	for name, value := range protocolFixtures() {
		t.Run(name, func(t *testing.T) {
			// ⚠️ 用 json.Marshal（默认行为，含 HTML 转义）—— 这就是**服务端真实吐出来的字节**。
			// 不去 SetEscapeHTML(false)：那样 fixture 就不再是「Go 实际输出」，
			// 测出来的东西也就没那么可信了。
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}
			raw = append(raw, '\n')

			path := filepath.Join(protocolFixtureDir, name+".json")
			existing, readErr := os.ReadFile(path)

			if readErr != nil {
				// 新 fixture：直接落盘，然后**失败一次**提醒跑第二遍确认。
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatalf("写入 %s 失败: %v", path, err)
				}
				t.Fatalf("fixture %s 不存在，已生成。请重新运行本测试确认它稳定。", path)
			}

			if string(existing) == string(raw) {
				return
			}

			if update {
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatalf("更新 %s 失败: %v", path, err)
				}
				t.Logf("已更新 %s", path)
				return
			}

			t.Fatalf("%s 与 Go 当前输出不一致 —— **这是一次契约变更**，不是测试坏了。\n"+
				"  旧: %s\n  新: %s\n"+
				"确认这是有意的之后，用 UPDATE_FIXTURES=1 go test ./lib -run TestProtocolFixtures 重新生成，"+
				"并同步 rust/crates/protocol 的字段。",
				path, string(existing), string(raw))
		})
	}
}

// TestProtocolFixtureRoundTripInGo 顺带钉住「Go 自己能读回自己写的」。
//
// 单独一个测试而不是塞进上面：上面只验证**输出形状**，
// 这里验证**形状是可解析的** —— 少一个 key、多一个 null 都可能让反序列化炸，
// 而那种错在只做 Marshal 的测试里是看不见的。
func TestProtocolFixtureRoundTripInGo(t *testing.T) {
	for name, value := range protocolFixtures() {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}

			switch name {
			case "post_event_text", "post_event_file":
				var back PostEvent
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("PostEvent 读不回来: %v", err)
				}
			case "history", "history_empty":
				var back History
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("History 读不回来: %v", err)
				}
			case "room_list":
				var back RoomListResponse
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("RoomListResponse 读不回来: %v", err)
				}
			case "ws_connect":
				var back WebSocketMessage
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("WebSocketMessage 读不回来: %v", err)
				}
			case "room_info":
				var back RoomInfo
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("RoomInfo 读不回来: %v", err)
				}
			case "device_meta", "device_meta_no_name":
				var back DeviceMeta
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("DeviceMeta 读不回来: %v", err)
				}
			default: // text_* / file_*
				var back ReceiveHolder
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatalf("ReceiveHolder 读不回来: %v", err)
				}
				if back.Type() == "" {
					t.Fatal("读回来之后 type 是空的 —— ReceiveHolder 分派坏了")
				}
			}
		})
	}
}
