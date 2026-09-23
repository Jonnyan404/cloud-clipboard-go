package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	token, _, err := s.issueShareToken("content", "7", "default", 600, 2, "")
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

	token, _, err := s.issueShareToken("file", "uuid-x", "default", 600, 1, "")
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
	s.config.Server.RoomAuth = RoomAuthConfig{"private": RoomAuthEntry{Password: "private-pass"}}
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

// 分享密码：签进 token 的是 HMAC(签名密钥, 密码)，校验走请求头 X-Share-Password。
// 这里覆盖四种情况：没带 / 带错 / 带对 / 以及「本来就不需要密码的分享不受影响」。
func TestShareTokenPassword(t *testing.T) {
	s := &ClipboardServer{config: &Config{}}
	s.config.Server.Auth = "secret-pass"

	token, _, err := s.issueShareToken("content", "7", "default", 600, 0, "hunter2")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	newReq := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/content/7?t="+token, nil)
	}

	if s.validateShareToken(newReq(), "content", "7", "default") {
		t.Fatal("expected reject when no password is supplied")
	}

	req := newReq()
	req.Header.Set(sharePasswordHeader, "wrong")
	if s.validateShareToken(req, "content", "7", "default") {
		t.Fatal("expected reject with the wrong password")
	}

	req = newReq()
	req.Header.Set(sharePasswordHeader, "hunter2")
	if !s.validateShareToken(req, "content", "7", "default") {
		t.Fatal("expected accept with the correct password")
	}

	// 不带密码签发的分享：客户端多带一个密码头也不该被拦
	plain, _, err := s.issueShareToken("content", "8", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/content/8?t="+plain, nil)
	req.Header.Set(sharePasswordHeader, "whatever")
	if !s.validateShareToken(req, "content", "8", "default") {
		t.Fatal("a share without a password should not be affected by the header")
	}
}

// 密码不进 URL：token 里只有哈希，明文密码不该出现在 token 里。
func TestShareTokenPasswordNotInToken(t *testing.T) {
	s := &ClipboardServer{config: &Config{}}
	s.config.Server.Auth = "secret-pass"

	token, _, err := s.issueShareToken("content", "9", "default", 600, 0, "hunter2")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if strings.Contains(token, "hunter2") {
		t.Fatal("the plaintext password must not appear in the token")
	}
	claims, ok := s.parseShareToken(token)
	if !ok {
		t.Fatal("token should parse")
	}
	if claims.PwdHash == "" || claims.PwdHash == "hunter2" {
		t.Fatalf("expected a hashed password marker, got %q", claims.PwdHash)
	}
}

// ── 分享页（前端 hash 路由）──────────────────────────────────────────────
//
// 分享链接必须指向前端分享页，而不是裸接口地址。这里钉住三件事：
// 路径形状、`#` 没被转义、以及 URL 里带的 token 还能解析回来。

func TestSharePageURLUsesFrontendRoute(t *testing.T) {
	s := &ClipboardServer{config: &Config{}}
	s.config.Server.Auth = "secret-pass"
	s.config.Server.Prefix = "/cc"

	token, _, err := s.issueShareToken("content", "7", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/share", nil)
	req.Host = "clip.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := s.buildSharePageURL(req, token)
	// 分享地址就是落地页地址：抓取程序和真人共用一条（见 share_landing.go 文件头）。
	if !strings.HasPrefix(got, "https://clip.example.com/cc/s/") {
		t.Fatalf("unexpected share page url: %s", got)
	}
	// 不再是 hash 地址：token 必须在**路径**里，否则服务端读不到（抓取程序也读不到）。
	if strings.Contains(got, "#") || strings.Contains(got, "?t=") {
		t.Fatalf("the token must live in the path now: %s", got)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("share page url should parse: %v", err)
	}
	// 分享地址不能再带 fragment：`#` 之后的部分服务端收不到，OG 就无从注入。
	if parsed.Fragment != "" {
		t.Fatalf("the share url must not carry a fragment any more, got %q", parsed.Fragment)
	}
	claims, ok := s.parseShareToken(strings.TrimPrefix(parsed.Path, "/cc/s/"))
	if !ok {
		t.Fatal("the token carried in the share page url should still parse")
	}
	if claims.ID != "7" || claims.Type != "content" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

// 房间开放时也**必须**签发 token。
//
// 曾经只在 requirement.Required 时才发，于是开放房间的分享链接是裸接口地址：
// 弹窗里让用户设的 TTL / 次数限制被静默丢弃，而响应里照样回 ttl/maxUses。
func TestShareAlwaysIssuesTokenOnOpenRoom(t *testing.T) {
	s := &ClipboardServer{
		config:       &Config{},
		logger:       log.New(io.Discard, "", 0),
		messageQueue: &PostList{},
	}
	// 没有 Server.Auth、没有 RoomAuth —— 这就是「开放房间」
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{ID: 42, Type: "text", Room: "default", Timestamp: time.Now().Unix()},
			Content:     "hello",
		}},
	})

	body := strings.NewReader(`{"type":"content","id":"42","ttl":60,"maxUses":3}`)
	req := httptest.NewRequest(http.MethodPost, "/share", body)
	req.Host = "clip.example.com"
	w := httptest.NewRecorder()
	s.handle_share(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token     string `json:"token"`
		URL       string `json:"url"`
		RawURL    string `json:"rawUrl"`
		MaxUses   int    `json:"maxUses"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("an open room must still get a share token, otherwise ttl/maxUses are silently dropped")
	}
	if !strings.Contains(resp.URL, "/s/") {
		t.Fatalf("share url should point at the share page itself, got %s", resp.URL)
	}
	// 分享页里取不了正文 —— 下载/取正文要另一条带同一个 token 的接口地址
	if !strings.Contains(resp.RawURL, "/content/42") || !strings.Contains(resp.RawURL, "t=") {
		t.Fatalf("rawUrl should reach the content endpoint with the same token, got %s", resp.RawURL)
	}
	if resp.MaxUses != 3 {
		t.Fatalf("expected maxUses 3, got %d", resp.MaxUses)
	}
	if resp.ExpiresAt <= time.Now().Unix() {
		t.Fatal("expiresAt should be in the future")
	}

	// 次数限制这次真的生效
	next := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/content/42?t="+resp.Token, nil)
	}
	for i := 1; i <= 3; i++ {
		if !s.validateShareToken(next(), "content", "42", "default") {
			t.Fatalf("use %d should succeed", i)
		}
	}
	if s.validateShareToken(next(), "content", "42", "default") {
		t.Fatal("the 4th use should be rejected when maxUses=3")
	}
}

func newShareInfoTestServer(t *testing.T, files map[string]File) *ClipboardServer {
	t.Helper()
	return &ClipboardServer{
		config:        &Config{},
		logger:        log.New(io.Discard, "", 0),
		messageQueue:  &PostList{},
		uploadFileMap: files,
	}
}

type shareInfoResponse struct {
	Type          string `json:"type"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	UUID          string `json:"uuid"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	Room          string `json:"room"`
	ExpiresAt     int64  `json:"expiresAt"`
	MaxUses       int    `json:"maxUses"`
	Used          int    `json:"used"`
	NeedsPassword bool   `json:"needsPassword"`
}

func TestShareInfoForFileShare(t *testing.T) {
	s := newShareInfoTestServer(t, map[string]File{
		"uuid-1": {Name: "photo.png", UUID: "uuid-1", Size: 4096, ExpireTime: time.Now().Unix() + 600},
	})

	token, _, err := s.issueShareToken("file", "uuid-1", "default", 600, 0, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	w := httptest.NewRecorder()
	s.handle_share(w, httptest.NewRequest(http.MethodGet, "/share?t="+token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp shareInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Type != "file" || resp.Kind != "file" {
		t.Fatalf("unexpected type/kind: %+v", resp)
	}
	if resp.UUID != "uuid-1" || resp.Name != "photo.png" || resp.Size != 4096 {
		t.Fatalf("the share page needs uuid/name/size to build the download link, got %+v", resp)
	}
	if resp.NeedsPassword {
		t.Fatal("this share has no password")
	}
}

// 分享页在取正文之前必须先能问出「这条分享到底要不要密码」——
// 否则收件人只会拿到一个笼统的 401，不知道该输什么。
func TestShareInfoPasswordGate(t *testing.T) {
	s := newShareInfoTestServer(t, map[string]File{
		"uuid-2": {Name: "a.txt", UUID: "uuid-2", Size: 10, ExpireTime: time.Now().Unix() + 600},
	})

	token, _, err := s.issueShareToken("file", "uuid-2", "default", 600, 0, "hunter2")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	w := httptest.NewRecorder()
	s.handle_share(w, httptest.NewRequest(http.MethodGet, "/share?t="+token, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without the password, got %d", w.Code)
	}
	var errResp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if errResp.Code != "share_password_required" {
		t.Fatalf("expected share_password_required, got %q", errResp.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/share?t="+token, nil)
	req.Header.Set(sharePasswordHeader, "hunter2")
	w2 := httptest.NewRecorder()
	s.handle_share(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 with the correct password, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestShareInfoRejectsBadToken(t *testing.T) {
	s := newShareInfoTestServer(t, nil)

	w := httptest.NewRecorder()
	s.handle_share(w, httptest.NewRequest(http.MethodGet, "/share?t=not-a-token", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a malformed token, got %d", w.Code)
	}

	expired, err := s.signShareClaims(shareClaims{
		Type: "file", ID: "uuid-1", Room: "default", Exp: time.Now().Unix() - 10,
	})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	w2 := httptest.NewRecorder()
	s.handle_share(w2, httptest.NewRequest(http.MethodGet, "/share?t="+expired, nil))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an expired token, got %d", w2.Code)
	}
}

// 「看一眼」不消耗次数：打开分享页本身不该烧掉一次，真正取正文时才算。
func TestShareInfoDoesNotConsumeUses(t *testing.T) {
	s := newShareInfoTestServer(t, map[string]File{
		"uuid-3": {Name: "a.txt", UUID: "uuid-3", Size: 10, ExpireTime: time.Now().Unix() + 600},
	})

	token, _, err := s.issueShareToken("file", "uuid-3", "default", 600, 1, "")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		s.handle_share(w, httptest.NewRequest(http.MethodGet, "/share?t="+token, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("peek %d should succeed, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	next := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/file/uuid-3/a.txt?t="+token, nil)
	}
	if !s.validateShareToken(next(), "file", "uuid-3", "default") {
		t.Fatal("the first real fetch should succeed after any number of peeks")
	}
	if s.validateShareToken(next(), "file", "uuid-3", "default") {
		t.Fatal("the second real fetch should fail with maxUses=1")
	}
}

// 分享页地址里不该出现明文密码（密码只走请求头，且 token 里只有哈希）。
func TestSharePageURLHasNoPassword(t *testing.T) {
	s := &ClipboardServer{config: &Config{}}
	s.config.Server.Auth = "secret-pass"

	token, _, err := s.issueShareToken("content", "7", "default", 600, 0, "hunter2")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/share", nil)
	req.Host = "clip.example.com"
	if got := s.buildSharePageURL(req, token); strings.Contains(got, "hunter2") {
		t.Fatalf("the share page url must not carry the plaintext password: %s", got)
	}
}
