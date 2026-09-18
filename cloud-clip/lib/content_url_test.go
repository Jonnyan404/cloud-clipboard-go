package lib

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /content/latest 的 JSON 分支曾经用 filepath.Join 拼 url。filepath.Join 内部会 Clean，
// 把 "http://host" 的双斜杠收成 "http:/host"，于是客户端拿到的 url 直接是坏的：
// 文本不受影响（内容内联在 JSON 里，不用再发请求），文件/图片却要靠这个 url 二次请求。
func TestLatestFileURLKeepsSchemeSlashes(t *testing.T) {
	// 复用过期测试里的最小服务器夹具（同一份 setup：队列一条文件 + 磁盘一份字节）
	s := newExpireTestServer(t, 0)

	req := httptest.NewRequest(http.MethodGet, "/content/latest?room=default&json=1", nil)
	rec := httptest.NewRecorder()
	s.handleContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d，响应体=%q", rec.Code, rec.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("JSON 无法解析: %v，响应体=%q", err, rec.Body.String())
	}

	got, _ := payload["url"].(string)
	want := "http://example.test/file/" + expireTestUUID + "/notes.txt"
	if got != want {
		t.Fatalf("url 不对\n  实际: %s\n  期望: %s", got, want)
	}
}

const roomGuardUUID = "99999999-8888-7777-6666-555555555555"

// 只设房间密码（全局 auth 为空）的最小服务器。这个组合最容易暴露「按 ?room= 鉴权」的问题：
// 攻击者只要把 room 说成 default，鉴权就会去查 default 的策略 —— 而 default 没设密码。
func newRoomGuardServer(t *testing.T) *ClipboardServer {
	t.Helper()

	s := &ClipboardServer{
		config:        &Config{},
		logger:        log.New(io.Discard, "", 0),
		messageQueue:  &PostList{},
		storageFolder: t.TempDir(),
		uploadFileMap: map[string]File{},
	}
	s.config.Server.Auth = ""
	s.config.Server.RoomAuth = RoomAuthConfig{
		"vault": {Password: "vaultpw"},
	}
	// 文件登记在 vault 房间
	s.uploadFileMap[roomGuardUUID] = File{
		Name: "secret.png",
		UUID: roomGuardUUID,
		Room: "vault",
	}
	return s
}

// 回归守卫：/file/ 的房间必须取文件自己记录的那个，不能听客户端传的 ?room=。
// 曾经 `?room=default` 就能把 vault 房间的文件读出来（default 无密码 → 直接放行）。
func TestFileRouteRoomCannotBeSpoofed(t *testing.T) {
	s := newRoomGuardServer(t)

	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	handler := s.authMiddleware(next)

	cases := []struct {
		name       string
		path       string
		headers    map[string]string
		wantStatus int
	}{
		{"谎报 room=default", "/file/" + roomGuardUUID + "/secret.png?room=default", nil, http.StatusUnauthorized},
		{"谎报 room=（空）", "/file/" + roomGuardUUID + "/secret.png?room=", nil, http.StatusUnauthorized},
		{"不传 room，也不带凭据", "/file/" + roomGuardUUID + "/secret.png", nil, http.StatusUnauthorized},
		{"带错密码", "/file/" + roomGuardUUID + "/secret.png?auth=wrong", nil, http.StatusUnauthorized},
		{"真房间 + 对密码", "/file/" + roomGuardUUID + "/secret.png?auth=vaultpw", nil, http.StatusOK},
		{"真房间 + Authorization 头", "/file/" + roomGuardUUID + "/secret.png", map[string]string{"Authorization": "Bearer vaultpw"}, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("实际 %d，期望 %d，响应体=%q", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

// 直接钉住房间推断本身：登记过的文件以记录为准，没登记过的才回落 ?room=。
func TestInferRequestRoomPrefersRecordedRoom(t *testing.T) {
	s := newRoomGuardServer(t)

	known := httptest.NewRequest(http.MethodGet, "/file/"+roomGuardUUID+"/secret.png?room=default", nil)
	if got := s.inferRequestRoom(known); got != "vault" {
		t.Fatalf("登记过的文件应以记录房间为准，实际=%q", got)
	}

	// 未登记的 uuid（例如刚创建、还没落 map 的上传）才允许信 ?room=
	unknown := httptest.NewRequest(http.MethodGet, "/file/unknown-uuid/secret.png?room=studio", nil)
	if got := s.inferRequestRoom(unknown); got != "studio" {
		t.Fatalf("未登记的文件应回落 ?room=，实际=%q", got)
	}

	// /text 这类非文件路径不受影响
	text := httptest.NewRequest(http.MethodPost, "/text?room=studio", nil)
	if got := s.inferRequestRoom(text); got != "studio" {
		t.Fatalf("/text 的 ?room= 应照常生效，实际=%q", got)
	}
}
