package lib

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// 这个文件钉住 `/server` 响应里 `roomProtected` 的**含义**：
//
//	「这个房间实际要不要密码」
//
// 而**不是**「roomAuth 里有没有这一项」。
//
// 为什么值得一条测试：SPA 顶部那个房间 chip 的锁图标直接读它（`roomProtected` 进
// `roomProtectionCache`，见 web-vue3 的 store/websocket.js 与 PageToolbar.vue）。
// 判错的后果不是崩溃，而是**一把不存在的锁**：显式 `{open: true}` 的房间在配置里
// 有这一项、但不要密码，报成受保护就会给它挂锁；反过来「只配了全局密码、没有房间条目」
// 的房间要密码却报成公开。
//
// 同类 bug 已经出现过两次，都是**同一个混淆**：
//   - `/rooms` 的 `isProtected`：已经改成 resolveRoomAuth(...).Required（main.go 的
//     getRoomList 里有注释），当时漏改的就是这里；
//   - Cloudflare Worker 侧的两个调用点，见 workers/src/auth.js 里那段注释。
//
// 所以这里把「四种配置 × 要不要挂锁」全部按 URL 层的结果钉住，谁再改回
// 「有没有这一项」就会红。

// serverRoomProtected 打一次 /server?room=…，返回 roomProtected 与 auth 两个字段。
//
// 两个字段一起返回是有意的：SPA 把它们当**同一个问题**看 ——
// `auth` 决定「要不要弹密码框」，`roomProtected` 决定「chip 上挂不挂锁」。
// 对同一个房间来说这两者必须一致，否则会出现「没让输密码但显示锁」这种自相矛盾的界面。
func serverRoomProtected(t *testing.T, s *ClipboardServer, room string) (protected bool, authNeeded bool) {
	t.Helper()

	target := "/server"
	if room != "" {
		target += "?room=" + url.QueryEscape(room)
	}
	rec := httptest.NewRecorder()
	s.handle_server(rec, httptest.NewRequest(http.MethodGet, target, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/server%s 返回 %d，期望 200", target, rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 /server 响应失败: %v", err)
	}
	protected, _ = body["roomProtected"].(bool)
	authNeeded, _ = body["auth"].(bool)
	return protected, authNeeded
}

func TestServerRoomProtectedMeansNeedsPassword(t *testing.T) {
	cases := []struct {
		name   string
		config string
		room   string
		want   bool
		reason string
	}{
		{
			name:   "房间自带密码",
			config: `{"server": {"roomAuth": {"secret": {"password": "room-pass"}}}}`,
			room:   "secret",
			want:   true,
			reason: "有密码 → 挂锁",
		},
		{
			name: "显式开放的房间不该挂锁",
			config: `{"server": {
				"auth": "global-pass",
				"roomAuth": {"lobby": {"open": true}}
			}}`,
			room:   "lobby",
			want:   false,
			reason: "配置里有这一项，但它显式开放 —— 旧实现（按「有没有这一项」）在这里会挂一把不存在的锁",
		},
		{
			name:   "只写了 automation 的条目不代表要密码（无全局密码时）",
			config: `{"server": {"roomAuth": {"ops": {"automation": "single"}}}}`,
			room:   "ops",
			want:   false,
			reason: "条目本身不是锁；既没密码也没开放声明、又没有全局密码 → 房间里不需要钥匙",
		},
		{
			name: "只写了 automation 的条目要回落全局密码",
			config: `{"server": {
				"auth": "global-pass",
				"roomAuth": {"ops": {"automation": "single"}}
			}}`,
			room:   "ops",
			want:   true,
			reason: "条目没写密码也没写 open → 回落全局密码，仍然要钥匙",
		},
		{
			name:   "没配过的房间跟随全局密码",
			config: `{"server": {"auth": "global-pass"}}`,
			room:   "never-configured",
			want:   true,
			reason: "旧实现只认 roomAuth 条目，这种「全局加密 + 没有房间条目」的部署全线漏锁",
		},
		{
			name:   "既没全局密码也没配过",
			config: `{"server": {}}`,
			room:   "never-configured",
			want:   false,
			reason: "公开房间 → 地球图标",
		},
		{
			name: "空字符串 = 只接受全局 auth（旧语义）",
			config: `{"server": {
				"auth": "global-pass",
				"roomAuth": {"legacy": ""}
			}}`,
			room:   "legacy",
			want:   true,
			reason: "写空值不是「开放」，是「跟随全局」——别把这两种混成一个",
		},
		{
			name: "open 与 password 同时写错时按需要密码算",
			config: `{"server": {
				"roomAuth": {"contradiction": {"open": true, "password": "room-pass"}}
			}}`,
			room:   "contradiction",
			want:   true,
			reason: "宁可多要一次密码，也不能因为多打一个字段就把房间敞开",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newServerWithRoomAuth(t, tc.config)
			protected, authNeeded := serverRoomProtected(t, s, tc.room)

			if protected != tc.want {
				t.Errorf("房间 %q 的 roomProtected = %v，期望 %v（%s）",
					tc.room, protected, tc.want, tc.reason)
			}
			// chip 上的锁 与 弹不弹密码框 必须是同一个答案
			if protected != authNeeded {
				t.Errorf("房间 %q 自相矛盾：roomProtected=%v 但 auth=%v —— "+
					"前者决定 chip 上的锁，后者决定弹不弹密码框，两者只能同时真或同时假",
					tc.room, protected, authNeeded)
			}
		})
	}
}

// 不带 `?room=` 的 /server 是「服务端整体要不要密码」的老问题，没有具体房间可谈，
// 所以 roomProtected 保持 false —— 别顺手把它也改成「全局有没有密码」，
// 那会让前端在还没问到房间时先挂一把锁。
func TestServerWithoutRoomHasNoRoomProtection(t *testing.T) {
	s := newServerWithRoomAuth(t, `{"server": {"auth": "global-pass"}}`)

	protected, authNeeded := serverRoomProtected(t, s, "")
	if protected {
		t.Error("不带 ?room= 时 roomProtected 应当保持 false（没有具体房间可谈）")
	}
	if !authNeeded {
		t.Error("不带 ?room= 但有全局密码时 auth 应当为 true（旧行为，别改）")
	}
}
