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

// 「全局加密 + 个别房间开放」用 `{"open": true}` 表达。
//
// ⚠️ 是**显式字段**而不是「把值留空」：空字符串在这份配置里已经有含义（只接受全局 auth），
// 改掉它会静默改变现有配置 —— 某个房间会悄悄敞开。四种组合都在这里钉住，别改回去。
func TestOpenRoomOverridesGlobalAuth(t *testing.T) {
	s := newServerWithRoomAuth(t, `{
		"server": {
			"auth": "global-pass",
			"roomAuth": {
				"public": {"open": true},
				"locked": "room-pass",
				"legacy-empty": ""
			}
		}
	}`)

	// 显式开放：不要密码，也不回落全局密码
	if req := s.resolveRoomAuth("public"); req.Required {
		t.Fatalf("open 房间不该要密码: %+v", req)
	}
	if !s.canAccessRoom("public", "") {
		t.Fatal("开放房间不带凭据也该能进")
	}

	// 房间自己的密码生效，**且全局密码仍然有效**（旧行为，别改回去）
	locked := s.resolveRoomAuth("locked")
	if !locked.Required || locked.Password != "room-pass" {
		t.Fatalf("房间密码应当生效: %+v", locked)
	}
	for _, token := range []string{"room-pass", "global-pass"} {
		if !s.canAccessRoom("locked", token) {
			t.Fatalf("凭据 %q 应当能进 locked", token)
		}
	}
	for _, token := range []string{"", "wrong"} {
		if s.canAccessRoom("locked", token) {
			t.Fatalf("凭据 %q 不该进 locked", token)
		}
	}

	// 空字符串 = 只接受全局 auth（旧语义，没变）
	legacy := s.resolveRoomAuth("legacy-empty")
	if !legacy.Required || legacy.Password != "global-pass" {
		t.Fatalf("空字符串应当回落全局密码: %+v", legacy)
	}

	// 没配过的房间也回落全局密码
	if req := s.resolveRoomAuth("never-configured"); !req.Required || req.Password != "global-pass" {
		t.Fatalf("没配过的房间应当继承全局密码: %+v", req)
	}
}

// open 和 password 同时给 = 配置写错。按**需要密码**处理：
// 宁可多要一次密码，也不能因为多打了一个字段把房间敞开。
func TestOpenWithPasswordPrefersPassword(t *testing.T) {
	s := newServerWithRoomAuth(t, `{
		"server": {
			"auth": "global-pass",
			"roomAuth": {"contradiction": {"open": true, "password": "room-pass"}}
		}
	}`)

	req := s.resolveRoomAuth("contradiction")
	if !req.Required || req.Password != "room-pass" {
		t.Fatalf("open + password 同时出现时应当按需要密码处理: %+v", req)
	}
	if s.canAccessRoom("contradiction", "") {
		t.Fatal("不该因为配置里多写了 open 就把房间敞开")
	}
}
