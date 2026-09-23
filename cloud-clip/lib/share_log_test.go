package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newShareLogTestServer(t *testing.T) *ClipboardServer {
	t.Helper()
	return &ClipboardServer{
		config:          &Config{},
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    &PostList{},
		uploadFileMap:   make(map[string]File),
		historyFilePath: filepath.Join(t.TempDir(), "history.json"),
	}
}

func (s *ClipboardServer) addTextForTest(id int, room, content string) {
	s.messageQueue.List = append(s.messageQueue.List, PostEvent{
		Event: "receive",
		Data: ReceiveHolder{TextReceive: &TextReceive{
			ReceiveBase: ReceiveBase{ID: id, Type: "text", Room: room, Timestamp: time.Now().Unix()},
			Content:     content,
		}},
	})
}

type shareCreateResponse struct {
	Token   string `json:"token"`
	JTI     string `json:"jti"`
	URL     string `json:"url"`
	PageURL string `json:"pageUrl"`
	Visits  int    `json:"visits"`
	Scans   int    `json:"scans"`
}

func createShareForTest(t *testing.T, s *ClipboardServer, body string) (int, shareCreateResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/share", strings.NewReader(body))
	req.Host = "clip.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	s.handle_share(w, req)

	var resp shareCreateResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	return w.Code, resp, w.Body.String()
}

// 签发分享时要留下档案：jti、类型、名称（文本取首行摘要）都要记下来，
// 否则「我最近分享过什么」这个问题永远答不上来。
func TestShareCreateRecordsLogEntry(t *testing.T) {
	s := newShareLogTestServer(t)
	s.addTextForTest(42, "default", "第一条摘要\n第二行\n第三行")

	code, resp, raw := createShareForTest(t, s, `{"type":"content","id":"42","ttl":600}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, raw)
	}
	if resp.JTI == "" {
		t.Fatal("a share with maxUses=0 must still carry a jti, otherwise it cannot be logged or counted")
	}
	if !strings.HasPrefix(resp.PageURL, "https://clip.example.com/s/") {
		t.Fatalf("pageUrl should be the share page itself, got %q", resp.PageURL)
	}
	// 两个字段收敛成同一个地址：同一个分享只有一个身份（抓取程序与真人共用）。
	if resp.URL != resp.PageURL {
		t.Fatalf("url and pageUrl must be the same address now, got %q vs %q", resp.URL, resp.PageURL)
	}

	rec, ok := s.lookupShareRecord(resp.JTI)
	if !ok {
		t.Fatal("expected a share log entry for the new jti")
	}
	if rec.Kind != "text" || rec.Room != "default" || rec.Type != "content" || rec.ID != "42" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.Name != "第一条摘要" {
		t.Fatalf("expected the first line as the record name, got %q", rec.Name)
	}
	if rec.Visits != 0 || rec.Scans != 0 {
		t.Fatalf("a fresh share should start at zero, got visits=%d scans=%d", rec.Visits, rec.Scans)
	}
}

func TestShareVisitIncrementsAndDeduplicates(t *testing.T) {
	s := newShareLogTestServer(t)
	s.addTextForTest(7, "default", "hi")
	_, resp, raw := createShareForTest(t, s, `{"type":"content","id":"7","ttl":600}`)
	if resp.JTI == "" {
		t.Fatalf("no jti in response: %s", raw)
	}

	visit := func(body, remoteAddr string) (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodPost, "/share/visit", strings.NewReader(body))
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		s.handleShareVisit(w, req)
		var out map[string]interface{}
		_ = json.NewDecoder(w.Body).Decode(&out)
		return w.Code, out
	}

	body := `{"token":"` + resp.Token + `"}`
	code, out := visit(body, "203.0.113.9:5555")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if tracked, _ := out["tracked"].(bool); !tracked {
		t.Fatalf("first visit should be tracked: %v", out)
	}
	if visits, _ := out["visits"].(float64); visits != 1 {
		t.Fatalf("expected visits=1, got %v", out["visits"])
	}

	// 同一访客重复上报会被去重 —— 否则拿着链接连点就能把数字刷上去
	_, out = visit(body, "203.0.113.9:5555")
	if tracked, _ := out["tracked"].(bool); tracked {
		t.Fatal("a repeat report from the same visitor should be deduplicated")
	}
	if visits, _ := out["visits"].(float64); visits != 1 {
		t.Fatalf("deduplicated report must not increase the count, got %v", out["visits"])
	}

	// 换一个访客就是新的一次打开
	_, out = visit(body, "203.0.113.10:5555")
	if visits, _ := out["visits"].(float64); visits != 2 {
		t.Fatalf("expected visits=2 after a second visitor, got %v", out["visits"])
	}

	// 二维码（?q=1 / qr:true）额外计一次扫码
	_, out = visit(`{"token":"`+resp.Token+`","qr":true}`, "203.0.113.11:5555")
	if scans, _ := out["scans"].(float64); scans != 1 {
		t.Fatalf("expected scans=1, got %v", out["scans"])
	}
	if visits, _ := out["visits"].(float64); visits != 3 {
		t.Fatalf("expected visits=3, got %v", out["visits"])
	}

	rec, _ := s.lookupShareRecord(resp.JTI)
	if rec.Visits != 3 || rec.Scans != 1 {
		t.Fatalf("record out of sync: %+v", rec)
	}
}

func TestShareVisitRejectsExpiredAndUnknownTokens(t *testing.T) {
	s := newShareLogTestServer(t)
	s.config.Server.Auth = "secret-pass"

	expired, err := s.signShareClaims(shareClaims{Type: "content", ID: "1", Room: "default", Exp: time.Now().Unix() - 5})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	for name, token := range map[string]string{"expired": expired, "garbage": "not-a-token"} {
		req := httptest.NewRequest(http.MethodPost, "/share/visit", strings.NewReader(`{"token":"`+token+`"}`))
		w := httptest.NewRecorder()
		s.handleShareVisit(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d (%s)", name, w.Code, w.Body.String())
		}
	}
}

func TestShareListScopedToRoomAndNeverReturnsTokens(t *testing.T) {
	s := newShareLogTestServer(t)
	s.config.Server.RoomAuth = RoomAuthConfig{"private": {Password: "pw"}}
	s.addTextForTest(1, "default", "开放房间的文本")

	_, openShare, raw := createShareForTest(t, s, `{"type":"content","id":"1","ttl":600}`)
	if openShare.Token == "" {
		t.Fatalf("default share failed: %s", raw)
	}
	// 受保护房间的那条直接入档：这里测的是列表的鉴权与隔离，不是签发。
	s.recordShare(&shareRecord{
		JTI: "private-jti", Type: "content", ID: "2", Room: "private", Kind: "text",
		Name: "私密房间的文本", CreatedAt: time.Now().Unix(), Exp: time.Now().Unix() + 600, Password: true,
	})

	list := func(query string, headers map[string]string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/share/list"+query, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		s.handleShareList(w, req)
		return w.Code, w.Body.String()
	}

	// 没凭据就读不到受保护房间的记录
	if code, _ := list("?room=private", nil); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a room token, got %d", code)
	}

	roomToken, err := s.issueRoomSessionToken("private", 3600, "")
	if err != nil {
		t.Fatalf("issue room session failed: %v", err)
	}

	code, raw := list("?room=private", map[string]string{"Authorization": "Bearer " + roomToken})
	if code != http.StatusOK {
		t.Fatalf("expected 200 with a room token, got %d: %s", code, raw)
	}
	if !strings.Contains(raw, "私密房间的文本") {
		t.Fatalf("expected the private room's own record, got %s", raw)
	}
	if strings.Contains(raw, "开放房间的文本") {
		t.Fatalf("the list must not cross rooms, got %s", raw)
	}
	// 列表是给创建者看统计的，把 token 回给调用方等于又发一遍 bearer 凭据
	if strings.Contains(raw, openShare.Token) {
		t.Fatalf("the list must not hand out share tokens: %s", raw)
	}

	var parsed struct {
		Total   int              `json:"total"`
		Records []shareListEntry `json:"records"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if parsed.Total != 1 || len(parsed.Records) != 1 {
		t.Fatalf("expected exactly one record for the private room, got %+v", parsed)
	}
	if !parsed.Records[0].Password {
		t.Fatal("the record should remember that this share is password protected")
	}
	// 开放房间的列表不需要凭据（和「在该房间签发分享」一致），但只该看到本房间的记录
	code, raw = list("?room=default", nil)
	if code != http.StatusOK || !strings.Contains(raw, "开放房间的文本") {
		t.Fatalf("an open room should be listable, got %d: %s", code, raw)
	}
	if parsed.Records[0].JTI == "" || parsed.Records[0].ExpiresAt == 0 {
		t.Fatalf("record is missing identity/expiry: %+v", parsed.Records[0])
	}
}

func TestShareLogPersistsAcrossRestart(t *testing.T) {
	s := newShareLogTestServer(t)
	s.addTextForTest(5, "default", "落盘的记录")
	if code, _, raw := createShareForTest(t, s, `{"type":"content","id":"5","ttl":600}`); code != http.StatusOK {
		t.Fatalf("share failed: %d %s", code, raw)
	}

	// 同一份 historyFilePath = 同一个数据目录
	restarted := &ClipboardServer{
		config:          &Config{},
		logger:          log.New(io.Discard, "", 0),
		messageQueue:    &PostList{},
		uploadFileMap:   make(map[string]File),
		historyFilePath: s.historyFilePath,
	}
	records, total := restarted.shareRecordsForRoom("default", 10)
	if total != 1 || len(records) != 1 {
		t.Fatalf("expected the record to survive a restart, got %+v", records)
	}
	if records[0].Name != "落盘的记录" {
		t.Fatalf("unexpected restored record: %+v", records[0])
	}
}

// 记录条数要有上限，否则这份文件会随运行时间无限长大；已过期的优先丢。
func TestShareLogTrimsToCapWithExpiredFirst(t *testing.T) {
	s := newShareLogTestServer(t)
	now := time.Now().Unix()

	for i := 0; i < maxShareLogRecords+20; i++ {
		// 前 30 条标成已过期 —— 被丢掉的应该是它们，而不是新的
		exp := now + 3600
		if i < 30 {
			exp = now - 60
		}
		s.recordShare(&shareRecord{JTI: "jti-" + strconv.Itoa(i), Type: "content", ID: "1", Room: "default", CreatedAt: now + int64(i), Exp: exp})
	}

	if len(s.shareLog) != maxShareLogRecords {
		t.Fatalf("expected the log to be trimmed to %d entries, got %d", maxShareLogRecords, len(s.shareLog))
	}
	// 本次只超了 20 条，而更早的过期条目有 30 条 —— 被丢的必须全部是过期的，
	// 没有一条还能用的分享被牺牲。
	for i := 30; i < maxShareLogRecords+20; i++ {
		if _, kept := s.shareLog["jti-"+strconv.Itoa(i)]; !kept {
			t.Fatalf("unexpired entry jti-%d must not be dropped while expired ones remain", i)
		}
	}
	for i := 0; i < 20; i++ {
		if _, still := s.shareLog["jti-"+strconv.Itoa(i)]; still {
			t.Fatalf("the oldest expired entry jti-%d should have gone first", i)
		}
	}
	if _, kept := s.shareLog["jti-"+strconv.Itoa(maxShareLogRecords+19)]; !kept {
		t.Fatal("the newest entry must survive trimming")
	}
}

func TestShareListCLampsLimit(t *testing.T) {
	s := newShareLogTestServer(t)
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		s.recordShare(&shareRecord{JTI: "jti-" + strconv.Itoa(i), Room: "default", CreatedAt: now + int64(i), Exp: now + 3600})
	}

	req := httptest.NewRequest(http.MethodGet, "/share/list?room=default&limit=99999", nil)
	w := httptest.NewRecorder()
	s.handleShareList(w, req)

	var parsed struct {
		Limit   int              `json:"limit"`
		Total   int              `json:"total"`
		Records []shareListEntry `json:"records"`
	}
	if err := json.NewDecoder(w.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if parsed.Limit != maxShareListLimit {
		t.Fatalf("limit should be clamped to %d, got %d", maxShareListLimit, parsed.Limit)
	}
	if parsed.Total != 5 || len(parsed.Records) != 5 {
		t.Fatalf("expected 5 records, got %+v", parsed)
	}
	// 新→旧
	if parsed.Records[0].JTI != "jti-4" {
		t.Fatalf("expected newest first, got %+v", parsed.Records)
	}
}

// 不限次数的分享以前没有 jti，于是既入不了档也计不了数 —— 这条钉住新契约。
func TestNewShareClaimsAlwaysCarriesJTI(t *testing.T) {
	s := &ClipboardServer{config: &Config{}}
	s.config.Server.Auth = "secret-pass"

	for _, maxUses := range []int{0, 5} {
		claims, _, err := s.newShareClaims("content", "1", "default", 60, maxUses, "")
		if err != nil {
			t.Fatalf("newShareClaims failed: %v", err)
		}
		if claims.JTI == "" {
			t.Fatalf("maxUses=%d must still get a jti", maxUses)
		}
	}
}
