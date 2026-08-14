package lib

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShareTokenSignAndValidate(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "secret-pass"

	token, err := s.signShareClaims(shareClaims{
		Type: "content",
		ID:   "12",
		Room: "default",
		Exp:  time.Now().Unix() + 600,
	})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	req := httptest.NewRequest(http.MethodGet, "/content/12?t="+token, nil)
	if !s.validateShareToken(req, "content", "12", "default") {
		t.Fatal("expected valid share token")
	}
	if s.validateShareToken(req, "file", "12", "default") {
		t.Fatal("type mismatch should fail")
	}
	if s.validateShareToken(req, "content", "99", "default") {
		t.Fatal("id mismatch should fail")
	}
}

func TestShareTokenExpired(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "secret-pass"

	token, err := s.signShareClaims(shareClaims{
		Type: "file",
		ID:   "uuid-1",
		Room: "private",
		Exp:  time.Now().Unix() - 10,
	})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/file/uuid-1/name?t="+token, nil)
	if s.validateShareToken(req, "file", "uuid-1", "private") {
		t.Fatal("expired token should fail")
	}
}

func TestShareTokenMaxUses(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "secret-pass"

	token, _, err := s.issueShareToken("content", "7", "default", 600, 2)
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodGet, "/content/7?t="+token, nil)
	if !s.validateShareToken(req1, "content", "7", "default") {
		t.Fatal("first use should succeed")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/content/7?t="+token, nil)
	if !s.validateShareToken(req2, "content", "7", "default") {
		t.Fatal("second use should succeed")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/content/7?t="+token, nil)
	if s.validateShareToken(req3, "content", "7", "default") {
		t.Fatal("third use should fail")
	}
}

func TestShareTokenRangeContinuationDoesNotConsume(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "secret-pass"

	token, _, err := s.issueShareToken("file", "uuid-x", "default", 600, 1)
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	// first full GET consumes the only use
	req1 := httptest.NewRequest(http.MethodGet, "/file/uuid-x/a.mp4?t="+token, nil)
	if !s.validateShareToken(req1, "file", "uuid-x", "default") {
		t.Fatal("initial get should succeed")
	}

	// range continuation should not require another use
	req2 := httptest.NewRequest(http.MethodGet, "/file/uuid-x/a.mp4?t="+token, nil)
	req2.Header.Set("Range", "bytes=1024-")
	if !s.validateShareToken(req2, "file", "uuid-x", "default") {
		t.Fatal("range continuation should not consume extra use")
	}

	// another full GET should fail
	req3 := httptest.NewRequest(http.MethodGet, "/file/uuid-x/a.mp4?t="+token, nil)
	if s.validateShareToken(req3, "file", "uuid-x", "default") {
		t.Fatal("second full get should fail after maxUses=1")
	}
}

func TestCanAccessRoomStillWorksWithoutShareToken(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "room-pass"

	req := httptest.NewRequest(http.MethodGet, "/file/u/a?auth=room-pass", nil)
	if !s.canAccessFile(req, "default", "u") {
		t.Fatal("password auth should still work")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/file/u/a", nil)
	req2.Header.Set("Authorization", "Bearer room-pass")
	if !s.canAccessFile(req2, "default", "u") {
		t.Fatal("bearer auth should still work")
	}
}
