package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRoomSessionTokenAuth(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
	}
	s.config.Server.Auth = "room-pass"

	token, err := s.issueRoomSessionToken("default", 600, "")
	if err != nil {
		t.Fatalf("issue session token failed: %v", err)
	}
	if token == "" {
		t.Fatal("session token empty")
	}

	if !s.validateRoomSessionToken("default", token) {
		t.Fatal("expected valid room session token")
	}
	if s.validateRoomSessionToken("private", token) {
		t.Fatal("session should not validate for wrong room")
	}

	// 全局 scope 的会话令牌应通行所有房间
	globalToken, err := s.issueRoomSessionToken("default", 600, "global")
	if err != nil {
		t.Fatalf("issue global session token failed: %v", err)
	}
	if !s.validateRoomSessionToken("default", globalToken) {
		t.Fatal("global token should validate for the issuing room")
	}
	if !s.validateRoomSessionToken("private", globalToken) {
		t.Fatal("global token should validate for any room")
	}
	if !s.validateRoomSessionToken("finance", globalToken) {
		t.Fatal("global token should validate for any other room")
	}

	req := httptest.NewRequest(http.MethodGet, "/file/u/a", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if !s.canAccessFile(req, "default", "u") {
		t.Fatal("session token should allow access")
	}

	globalReq := httptest.NewRequest(http.MethodGet, "/file/u/a", nil)
	globalReq.Header.Set("Authorization", "Bearer "+globalToken)
	if !s.canAccessFile(globalReq, "finance", "u") {
		t.Fatal("global session token should allow access to any room")
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

func TestHandleAuthToken(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
		logger: log.New(io.Discard, "", 0),
	}
	s.config.Server.Auth = "room-pass"

	// Test successful token issuance
	reqBody := strings.NewReader(`{"password":"room-pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/token?room=default", reqBody)
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Token == "" {
		t.Fatal("token should not be empty")
	}
	if resp.Scope != "global" {
		t.Fatalf("expected global scope when using global password, got %q", resp.Scope)
	}

	// Verify the token can access the room
	if !s.validateRoomSessionToken("default", resp.Token) {
		t.Fatal("issued token should validate for the room")
	}
	// 全局密码换来的会话令牌应通行其他房间
	if !s.validateRoomSessionToken("finance", resp.Token) {
		t.Fatal("global-scope token should validate for other rooms")
	}

	// Test wrong password
	reqBody = strings.NewReader(`{"password":"wrong-pass"}`)
	req = httptest.NewRequest(http.MethodPost, "/auth/token?room=default", reqBody)
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", w.Code)
	}

	// Test empty password
	reqBody = strings.NewReader(`{"password":""}`)
	req = httptest.NewRequest(http.MethodPost, "/auth/token?room=default", reqBody)
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty password, got %d", w.Code)
	}

	// 房间专属密码换来的令牌应为房间专属 scope，且不能通行其他房间
	s.config.Server.RoomAuth = map[string]string{"private": "private-pass"}
	reqBody = strings.NewReader(`{"password":"private-pass"}`)
	req = httptest.NewRequest(http.MethodPost, "/auth/token?room=private", reqBody)
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for room password, got %d", w.Code)
	}
	var roomResp struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(w.Body).Decode(&roomResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if roomResp.Scope != "" {
		t.Fatalf("expected room-specific scope for room password, got %q", roomResp.Scope)
	}
	if !s.validateRoomSessionToken("private", roomResp.Token) {
		t.Fatal("room token should validate for its room")
	}
	if s.validateRoomSessionToken("finance", roomResp.Token) {
		t.Fatal("room token should NOT validate for other rooms")
	}
}

func TestHandleAuthTokenRefresh(t *testing.T) {
	s := &ClipboardServer{
		config: &Config{},
		logger: log.New(io.Discard, "", 0),
	}
	s.config.Server.Auth = "room-pass"

	// 先签发一个有效令牌
	req := httptest.NewRequest(http.MethodPost, "/auth/token?room=default", strings.NewReader(`{"password":"room-pass"}`))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var issued struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&issued); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if issued.Token == "" {
		t.Fatal("token should not be empty")
	}

	// 有效令牌 -> 续签成功
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/token/refresh?room=default", nil)
	refreshReq.Header.Set("Authorization", "Bearer "+issued.Token)
	w2 := httptest.NewRecorder()
	s.handleAuthTokenRefresh(w2, refreshReq)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for refresh, got %d", w2.Code)
	}

	var refreshed struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&refreshed); err != nil {
		t.Fatalf("failed to decode refresh response: %v", err)
	}
	if refreshed.Token == "" {
		t.Fatal("refreshed token should not be empty")
	}
	if refreshed.ExpiresAt <= time.Now().Unix() {
		t.Fatal("expiresAt should be in the future")
	}
	if !s.validateRoomSessionToken("default", refreshed.Token) {
		t.Fatal("refreshed token should validate for the room")
	}

	// 无效令牌 -> 401
	badReq := httptest.NewRequest(http.MethodPost, "/auth/token/refresh?room=default", nil)
	badReq.Header.Set("Authorization", "Bearer invalid-token")
	w3 := httptest.NewRecorder()
	s.handleAuthTokenRefresh(w3, badReq)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", w3.Code)
	}

	// 缺少令牌 -> 401
	w4 := httptest.NewRecorder()
	s.handleAuthTokenRefresh(w4, httptest.NewRequest(http.MethodPost, "/auth/token/refresh?room=default", nil))
	if w4.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", w4.Code)
	}
}
