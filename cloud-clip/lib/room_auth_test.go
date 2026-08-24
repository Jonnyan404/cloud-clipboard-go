package lib

import (
	"encoding/json"
	"io"
	"log"
	"testing"
)

func TestRoomAuthEntryUnmarshalString(t *testing.T) {
	var entry RoomAuthEntry
	if err := json.Unmarshal([]byte(`"finance-pass"`), &entry); err != nil {
		t.Fatalf("unmarshal string failed: %v", err)
	}
	if entry.Password != "finance-pass" {
		t.Fatalf("unexpected password: %q", entry.Password)
	}
	if entry.FileExpire != nil {
		t.Fatal("fileExpire should be nil for plain string form")
	}
}

func TestRoomAuthEntryUnmarshalNumber(t *testing.T) {
	var entry RoomAuthEntry
	if err := json.Unmarshal([]byte(`12345`), &entry); err != nil {
		t.Fatalf("unmarshal number failed: %v", err)
	}
	if entry.Password != "12345" {
		t.Fatalf("unexpected password: %q", entry.Password)
	}
}

func TestRoomAuthEntryUnmarshalObject(t *testing.T) {
	var entry RoomAuthEntry
	if err := json.Unmarshal([]byte(`{"password":"p1","fileExpire":0}`), &entry); err != nil {
		t.Fatalf("unmarshal object failed: %v", err)
	}
	if entry.Password != "p1" {
		t.Fatalf("unexpected password: %q", entry.Password)
	}
	if entry.FileExpire == nil || *entry.FileExpire != 0 {
		t.Fatalf("expected fileExpire=0, got %v", entry.FileExpire)
	}
}

func TestRoomAuthEntryUnmarshalObjectWithoutFileExpire(t *testing.T) {
	var entry RoomAuthEntry
	if err := json.Unmarshal([]byte(`{}`), &entry); err != nil {
		t.Fatalf("unmarshal empty object failed: %v", err)
	}
	if entry.Password != "" {
		t.Fatalf("unexpected password: %q", entry.Password)
	}
	if entry.FileExpire != nil {
		t.Fatal("fileExpire should be nil when omitted")
	}
}

func newServerWithRoomAuth(t *testing.T, raw string) *ClipboardServer {
	t.Helper()
	cfg := &Config{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		t.Fatalf("config unmarshal failed: %v", err)
	}
	return &ClipboardServer{config: cfg, logger: discardLogger()}
}

func discardLogger() *log.Logger {
	if testDiscardLogger == nil {
		testDiscardLogger = log.New(io.Discard, "", 0)
	}
	return testDiscardLogger
}

var testDiscardLogger *log.Logger

func TestResolveFileExpireSeconds(t *testing.T) {
	s := newServerWithRoomAuth(t, `{
		"file": {"expire": 3600},
		"server": {
			"roomAuth": {
				"keep": {"password": "kp", "fileExpire": 0},
				"slow": {"password": "sp", "fileExpire": 86400},
				"legacy": "old-pass",
				"bad": {"password": "bp", "fileExpire": -5}
			}
		}
	}`)

	cases := []struct {
		room string
		want int64
	}{
		{"default", 3600}, // 未配置 -> 全局
		{"keep", 0},       // 0 -> 永不过期
		{"slow", 86400},   // 覆盖全局
		{"legacy", 3600},  // 字符串旧格式 -> 全局
		{"bad", 3600},     // 负数非法 -> 回退全局
	}
	for _, tc := range cases {
		if got := s.resolveFileExpireSeconds(tc.room); got != tc.want {
			t.Errorf("resolveFileExpireSeconds(%q) = %d, want %d", tc.room, got, tc.want)
		}
	}
}

func TestResolveRoomAuthKeepsPasswordAndPolicy(t *testing.T) {
	s := newServerWithRoomAuth(t, `{
		"server": {
			"roomAuth": {"finance": {"password": "finance-pass", "fileExpire": 0}}
		}
	}`)

	req := s.resolveRoomAuth("finance")
	if !req.Required || req.Password != "finance-pass" {
		t.Fatalf("auth requirement wrong: %+v", req)
	}
	if req.FileExpire == nil || *req.FileExpire != 0 {
		t.Fatalf("expected fileExpire=0, got %v", req.FileExpire)
	}

	open := s.resolveRoomAuth("private")
	if open.Required {
		t.Fatalf("empty-password room should not require auth: %+v", open)
	}
}
