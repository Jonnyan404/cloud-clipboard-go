package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const expireTestUUID = "11111111-2222-3333-4444-555555555555"
const expireTestBody = "hello-expire"

// newExpireTestServer 造一个最小可用的服务器：不配鉴权（canAccessRoom 直接放行），
// 队列里塞一条文件记录，磁盘上放好对应的字节。
func newExpireTestServer(t *testing.T, expireAt int64) *ClipboardServer {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, expireTestUUID), []byte(expireTestBody), 0o600); err != nil {
		t.Fatalf("写测试文件失败: %v", err)
	}

	now := time.Now().Unix()
	s := &ClipboardServer{
		config:        &Config{},
		logger:        log.New(io.Discard, "", 0),
		messageQueue:  &PostList{},
		storageFolder: dir,
		uploadFileMap: map[string]File{},
	}
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{FileReceive: &FileReceive{
			ReceiveBase: ReceiveBase{ID: 1, Type: "file", Room: "default", Timestamp: now},
			Name:        "notes.txt",
			Size:        int64(len(expireTestBody)),
			Cache:       expireTestUUID,
			Expire:      expireAt,
			URL:         "http://example.test/file/" + expireTestUUID,
		}},
	})
	return s
}

// 取内容的五条路径。JSON 与否决定了响应体格式，但**过期与否与格式无关**——
// 这里全部列出，是因为曾经只有 JSON 分支做了过期检查，非 JSON 分支能把过期文件照常吐出来。
//
// wantJSON 如实记录现状，别按直觉改：
//   - 文件分支只认 `json=1` / `.json` 后缀，**不看 Accept 头**（四个文件分支一致）；
//   - 文本分支才会看 Accept（handler.go 中两处 `strings.Contains(Accept, "application/json")`）。
//
// 这是 main 上就有的设计，本次不动它：文件下载场景里 Accept 太不可靠，
// 所以 JSON 与否交给显式参数决定。客户端（快捷指令）两个信号都发，因此不受影响。
var contentPathCases = []struct {
	name     string
	path     string
	accept   string
	wantJSON bool
}{
	{"latest_json_param", "/content/latest?room=default&json=1", "", true},
	{"latest_accept_json", "/content/latest?room=default", "application/json", false},
	{"latest_accept_text", "/content/latest?room=default", "text/plain", false},
	{"byid_json_param", "/content/1?room=default&json=1", "", true},
	{"byid_no_accept", "/content/1?room=default", "", false},
}

func TestContentExpiredFileIsBlockedOnEveryPath(t *testing.T) {
	for _, tc := range contentPathCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newExpireTestServer(t, time.Now().Unix()-10)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			s.handleContent(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("过期文件应返回 404，实际 %d，响应体=%q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "文件已过期") {
				t.Fatalf("响应体应说明已过期，实际=%q", rec.Body.String())
			}
			// 最要命的一种退化：过期了还把字节吐出去。
			if strings.Contains(rec.Body.String(), expireTestBody) {
				t.Fatalf("过期文件的内容不应出现在响应里，实际=%q", rec.Body.String())
			}
			// JSON 路径的错误体必须能被客户端的「获取字典」解析，
			// 否则客户端会把错误文本当成内容，存成一个顶着原文件名的假文件。
			if tc.wantJSON {
				var payload map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatalf("JSON 错误体无法解析: %v，响应体=%q", err, rec.Body.String())
				}
				// 三个字段各司其职：code 给程序判断，error 给日志/英文用户，
				// message 给中文用户看。任一缺失或走样都算契约破坏。
				if payload["code"] != "file_expired" || payload["error"] == "" || payload["message"] != "文件已过期" {
					t.Fatalf("错误体字段不对: %v", payload)
				}
			}
		})
	}
}

func TestContentFreshFileIsServedOnEveryPath(t *testing.T) {
	for _, tc := range contentPathCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newExpireTestServer(t, time.Now().Unix()+3600)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			s.handleContent(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("未过期文件应返回 200，实际 %d，响应体=%q", rec.Code, rec.Body.String())
			}

			body := rec.Body.String()
			if tc.wantJSON {
				// JSON 路径要能被客户端的「获取字典」解析，并带上 expire 供其预判。
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(body), &payload); err != nil {
					t.Fatalf("JSON 响应无法解析: %v，响应体=%q", err, body)
				}
				if payload["name"] != "notes.txt" {
					t.Fatalf("name 字段不对: %v", payload["name"])
				}
				if _, ok := payload["expire"]; !ok {
					t.Fatalf("JSON 响应应含 expire 字段，实际=%v", payload)
				}
			} else if body != expireTestBody {
				t.Fatalf("非 JSON 路径应直接给出文件字节，实际=%q", body)
			}
		})
	}
}

// Expire == 0 表示永不过期，不能被误判成"早已过期"。
func TestContentZeroExpireMeansNeverExpires(t *testing.T) {
	for _, tc := range contentPathCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newExpireTestServer(t, 0)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			s.handleContent(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expire=0 应永不过期，实际 %d，响应体=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

// 文本消息没有过期概念，不能被文件那套逻辑牵连。
func TestContentTextMessageIgnoresExpire(t *testing.T) {
	s := &ClipboardServer{
		config:        &Config{},
		logger:        log.New(io.Discard, "", 0),
		messageQueue:  &PostList{},
		storageFolder: t.TempDir(),
		uploadFileMap: map[string]File{},
	}
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{ID: 2, Type: "text", Room: "default", Timestamp: time.Now().Unix()},
			Content:     "文本不该过期",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/content/latest?room=default&json=1", nil)
	rec := httptest.NewRecorder()
	s.handleContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("文本消息应始终可取，实际 %d，响应体=%q", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("JSON 响应无法解析: %v", err)
	}
	if payload["content"] != "文本不该过期" {
		t.Fatalf("content 字段不对: %v", payload["content"])
	}
}
