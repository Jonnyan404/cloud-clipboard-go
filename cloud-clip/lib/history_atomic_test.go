package lib

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

/**
*** FILE: history_atomic_test.go
***   钉住 history.json 的落盘行为：原子写、失败留档、并发写不损坏。
***
*** 背景（这三条都是既有代码里的真实缺陷）：
***   1. saveHistoryData 用 os.WriteFile 直接覆盖 —— 「截断 + 写入」不是原子操作，
***      进程在写入过程中被杀会留下半截 JSON。
***   2. loadHistoryData 解析失败时 os.Remove 整个文件 —— 半截文件 + 这次删除 =
***      用户的全部历史一次崩溃就没。
***   3. handler.go 里两处 `go s.saveHistoryData()` 会并发写同一个路径，而 messageQueue
***      的锁只保护内存切片、不保护文件。
***/

// newStorageTestServer 起一个「历史文件落在本地磁盘上」的实例。
//
// 这几个测试关心的就是落盘行为，所以必须拿到真实的 historyFilePath 直接驱动，
// 而不是绕 HTTP —— 走接口只能看到「最终能不能读回来」，看不到「中间有没有半截文件」。
func newStorageTestServer(t *testing.T) (*ClipboardServer, string) {
	t.Helper()

	dir := t.TempDir()
	cfg := &Config{}
	cfg.Server.StorageDir = filepath.Join(dir, "uploads")
	cfg.Server.HistoryFile = filepath.Join(dir, "history.json")
	cfg.Server.History = 50
	cfg.Server.RoomList = false // 别在测试里起房间清理 goroutine
	cfg.Text.Limit = 40960
	cfg.File.Limit = 204857600

	s, err := NewClipboardServer(cfg)
	if err != nil {
		t.Fatalf("构造服务器失败: %v", err)
	}
	s.logger = log.New(io.Discard, "", 0)
	return s, dir
}

// appendText 往队列里塞一条文本条目（ID 交给队列自己分配）。
//
// ⚠️ Type 必须显式写 "text"：History 落盘时存的是 ReceiveHolder，而它的反序列化
// （utils.go 的 UnmarshalJSON）**靠 type 字段区分 text / file**。留空串的话写出去的
// JSON 是合法的、读回来却报 "unknown message type"，往返测试会以很迷惑的方式失败。
func appendText(s *ClipboardServer, content string) {
	s.messageQueue.Append(&PostEvent{
		Event: "text",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{Type: "text", Room: "default", Timestamp: 1},
			Content:     content,
		}},
	})
}

// assertNoTempFiles 断言目录里没有原子写留下的残骸。
//
// 临时文件必须在**任何**路径上都被清掉（写失败、fsync 失败、chmod 失败、rename 失败），
// 否则每写一次盘就在用户的存储目录里攒一个文件 —— 这类问题要很久以后才会被发现。
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录失败: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("原子写留下了临时文件残骸: %s", e.Name())
		}
	}
}

// TestHistorySaveWritesAtomically 一次正常保存：文件是完整合法 JSON、权限没变、无残骸。
func TestHistorySaveWritesAtomically(t *testing.T) {
	s, dir := newStorageTestServer(t)

	appendText(s, "hello")
	s.saveHistoryData()

	data, err := os.ReadFile(s.historyFilePath)
	if err != nil {
		t.Fatalf("读 history.json 失败: %v", err)
	}
	var hist History
	if err := json.Unmarshal(data, &hist); err != nil {
		t.Fatalf("history.json 不是合法 JSON: %v", err)
	}
	if len(hist.Receive) != 1 {
		t.Fatalf("期望 1 条记录，实测 %d", len(hist.Receive))
	}
	if got := hist.Receive[0].TextReceive.Content; got != "hello" {
		t.Fatalf("正文不对: %q", got)
	}

	assertNoTempFiles(t, dir)

	// ⚠️ os.CreateTemp 建出来的是 0600，必须显式 chmod 回 0644 ——
	// rename 之后它就是正式文件了，权限不会自己变回来。
	// 不测这条的话，「文件权限悄悄从 0644 变成 0600」会一直没人发现，
	// 而某些部署（容器里换用户读）会因此读不到。
	info, err := os.Stat(s.historyFilePath)
	if err != nil {
		t.Fatalf("stat 失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("文件权限应当是 0644，实测 %o", perm)
	}
}

// TestHistoryConcurrentSaveKeepsFileValid 并发保存不会把文件写坏。
//
// 这是修复前最真实的故障：handler.go 里 updateTextMessage 和 handleContentColumn
// 都是 `go s.saveHistoryData()`，两个请求同时来就并发覆盖同一个路径。
//
// ⚠️ 两个「让这条测试真的有检测力」的细节，缺一个都会变成白绿：
//
//  1. 每个 goroutine 塞**长度不同**的正文。如果大家都写同样长的内容，即使两次 Write
//     交错，落盘的字节也一样、JSON 照样合法 —— 测试永远是绿的。
//  2. 循环多轮。实测单轮的损坏概率约 40%（用一个独立程序跑 30 轮命中 13 轮），
//     跑一次有六成机会侥幸通过。10 轮之后漏检概率降到千分之几。
func TestHistoryConcurrentSaveKeepsFileValid(t *testing.T) {
	s, dir := newStorageTestServer(t)

	const rounds = 10
	const workers = 16

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := 0; i < workers; i++ {
			go func(n int) {
				defer wg.Done()
				appendText(s, strings.Repeat("x", 100*(n+1)))
				s.saveHistoryData()
			}(i)
		}
		wg.Wait()

		data, err := os.ReadFile(s.historyFilePath)
		if err != nil {
			t.Fatalf("第 %d 轮：读 history.json 失败: %v", round, err)
		}
		var hist History
		if err := json.Unmarshal(data, &hist); err != nil {
			head := data
			if len(head) > 200 {
				head = head[:200]
			}
			t.Fatalf("第 %d 轮：并发写之后 history.json 损坏了: %v\n内容前 200 字节: %q", round, err, string(head))
		}
	}
	assertNoTempFiles(t, dir)
}

// TestCorruptHistoryIsQuarantinedNotDeleted 损坏的历史文件要**留档**，不能删。
//
// 修复前是 os.Remove：一次崩溃（半截文件）+ 下一次启动（解析失败）= 全部历史消失，
// 而且现场也没了，用户连「为什么没了」都查不出来。
func TestCorruptHistoryIsQuarantinedNotDeleted(t *testing.T) {
	s, _ := newStorageTestServer(t)

	// 造一份半截 JSON —— 正是「写到一半被杀」会留下的东西
	corrupt := []byte(`{"receive":[{"id":1,"type":"text","content":"半截`)
	if err := os.WriteFile(s.historyFilePath, corrupt, 0o644); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}

	if err := s.loadHistoryData(); err == nil {
		t.Fatal("损坏的历史文件应当返回错误")
	}

	// 原路径不该还有文件（已经被改名移走）
	if _, err := os.Stat(s.historyFilePath); !os.IsNotExist(err) {
		t.Fatalf("损坏的历史文件应当被改名移走，原路径实测 err=%v", err)
	}

	// 必须留下 .corrupt-* 存档，且内容就是那份损坏数据
	parent := filepath.Dir(s.historyFilePath)
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("读目录失败: %v", err)
	}
	var archived string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "history.json.corrupt-") {
			archived = filepath.Join(parent, e.Name())
		}
	}
	if archived == "" {
		t.Fatal("没有找到 .corrupt-* 存档 —— 损坏的数据被删掉了")
	}
	kept, err := os.ReadFile(archived)
	if err != nil {
		t.Fatalf("读存档失败: %v", err)
	}
	if string(kept) != string(corrupt) {
		t.Fatalf("存档内容和损坏数据不一致:\n存档: %q\n原始: %q", kept, corrupt)
	}

	// 服务端照旧以空历史启动 —— 行为没变，变的只是「文件保住了」
	if n := len(s.messageQueue.List); n != 0 {
		t.Fatalf("解析失败后应当以空历史启动，实测 %d 条", n)
	}
}

// TestHistorySaveLoadRoundTrip 保存后能原样读回来（原子写没改变语义）。
func TestHistorySaveLoadRoundTrip(t *testing.T) {
	s, _ := newStorageTestServer(t)

	appendText(s, "第一条")
	appendText(s, "第二条")
	s.saveHistoryData()

	// 换一个实例读同一个文件 —— 走的就是真实启动路径
	cfg := &Config{}
	cfg.Server.StorageDir = s.storageFolder
	cfg.Server.HistoryFile = s.historyFilePath
	cfg.Server.History = 50
	cfg.Server.RoomList = false
	cfg.Text.Limit = 40960
	cfg.File.Limit = 204857600

	restored, err := NewClipboardServer(cfg)
	if err != nil {
		t.Fatalf("重新构造服务器失败: %v", err)
	}
	restored.logger = log.New(io.Discard, "", 0)

	if n := len(restored.messageQueue.List); n != 2 {
		t.Fatalf("期望读回 2 条，实测 %d", n)
	}
	if got := restored.messageQueue.List[0].Data.TextReceive.Content; got != "第一条" {
		t.Fatalf("第一条正文不对: %q", got)
	}
	if got := restored.messageQueue.List[1].Data.TextReceive.Content; got != "第二条" {
		t.Fatalf("第二条正文不对: %q", got)
	}
}
