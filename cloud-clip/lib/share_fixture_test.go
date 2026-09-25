package lib

// 分享/会话令牌的契约 fixture：把**固定的配置 + 固定的 claims** 从 Go 侧签出来，
// 连同派生出的签名密钥一起导出成 JSON，供 Rust 侧逐字节比对。
//
// 为什么需要它：分享令牌的形状（`base64url(json).base64url(hmac)`）、claims 的字段名与
// omitempty 效果、以及**密钥派生规则**（`SHA256("cloud-clipboard-share-v1" || 0 || 全局密码 || …)`）
// 三样合起来才是「Go 发的链接在 Rust 上仍然有效」。而这三样里有两样读代码根本看不出来
// —— 比如「空字段不写进 JSON」「房间按名字升序参与哈希」。
//
// 所以照 protocol_fixture_test.go 那套做：Go 生成 → Rust 断言
// （rust/crates/core/tests/share_tokens.rs）。
//
// 用法：
//
//	go test ./lib -run TestShareTokenFixtures                      # 校验
//	UPDATE_FIXTURES=1 go test ./lib -run TestShareTokenFixtures     # 重新生成
//
// ⚠️ 这个测试红了**先别改 fixture** —— 它红了意味着「新签出来的 token 和旧的不一样」，
// 也就是**所有已发出的分享链接会一起失效**。想清楚再决定。

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fixture 落点：仓库根的 cases/share/（`go test` 的 cwd 是 cloud-clip/lib）。
const shareFixtureDir = "../../cases/share"

type shareFixtureCase struct {
	Name string `json:"name"`
	// GlobalAuth 是 `server.auth` 的原始值（字符串 / 数字 / false 三态）。
	GlobalAuth interface{} `json:"globalAuth"`
	// RoomAuth 原样放进 JSON，Rust 侧用**自己的**配置解析器读它 ——
	// 顺带把「四种 roomAuth 写法」的解析也一起验了。
	RoomAuth map[string]interface{} `json:"roomAuth"`
	Claims   shareClaims            `json:"claims"`
	// KeyHex 是派生出的签名密钥。单独放一份是为了让失败信息能指出**是哪一步**不一样：
	// 密钥不一致 = 派生规则漂了；密钥一致但 token 不一致 = JSON 形状漂了。
	KeyHex string `json:"keyHex"`
	Token  string `json:"token"`
}

type shareFixtureFile struct {
	Note  string             `json:"note"`
	Cases []shareFixtureCase `json:"cases"`
}

func shareFixtureServer(t *testing.T, globalAuth interface{}, roomAuth map[string]interface{}) *ClipboardServer {
	t.Helper()
	cfg := &Config{}
	cfg.Server.Auth = globalAuth
	for room, value := range roomAuth {
		entry, err := parseRoomAuthEntryFixture(value)
		if err != nil {
			t.Fatalf("fixture 里的 roomAuth[%s] 解析失败: %v", room, err)
		}
		if cfg.Server.RoomAuth == nil {
			cfg.Server.RoomAuth = RoomAuthConfig{}
		}
		cfg.Server.RoomAuth[room] = entry
	}
	s := &ClipboardServer{config: cfg}
	s.initShareSigningKey()
	return s
}

func mustSignFixture(t *testing.T, s *ClipboardServer, claims shareClaims) string {
	t.Helper()
	token, err := s.signShareClaims(claims)
	if err != nil {
		t.Fatalf("签发 fixture token 失败: %v", err)
	}
	return token
}

// parseRoomAuthEntryFixture 把 fixture 里的 JSON 值还原成 RoomAuthEntry。
// 走的就是线上那条解析路径（UnmarshalJSON），别另写一份。
func parseRoomAuthEntryFixture(value interface{}) (RoomAuthEntry, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return RoomAuthEntry{}, err
	}
	var entry RoomAuthEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return RoomAuthEntry{}, err
	}
	return entry, nil
}

func TestShareTokenFixtures(t *testing.T) {
	cases := make([]shareFixtureCase, 0, 4)
	servers := make([]*ClipboardServer, 0, 4)

	// 1. 只有全局密码：最常见的部署。
	s1 := shareFixtureServer(t, "global-pw", nil)
	claims1 := shareClaims{
		Type:    "content",
		ID:      "7",
		Room:    "default",
		Exp:     1789955428,
		JTI:     "9f2c1d3b4a5e6f708192a3b4c5d6e7f8",
		MaxUses: 3,
	}
	cases = append(cases, shareFixtureCase{
		Name:       "global-password-only",
		GlobalAuth: "global-pw",
		RoomAuth:   map[string]interface{}{},
		Claims:     claims1,
		Token:      mustSignFixture(t, s1, claims1),
	})
	servers = append(servers, s1)

	// 2. 房间密码 + 开放房间：两种 roomAuth 写法都进派生输入，
	//    而且参与哈希的顺序是**房间名升序**（public < work）。
	roomAuth2 := map[string]interface{}{
		"work":   "work-pw",
		"public": map[string]interface{}{"open": true},
	}
	s2 := shareFixtureServer(t, "global-pw", roomAuth2)
	claims2 := shareClaims{
		Type: "file",
		ID:   "3f2a70c8-1f4b-4a5e-9c1d-2b3a4c5d6e7f",
		Room: "work",
		Exp:  1789955428,
	}
	cases = append(cases, shareFixtureCase{
		Name:       "room-password-and-open-room",
		GlobalAuth: "global-pw",
		RoomAuth:   roomAuth2,
		Claims:     claims2,
		Token:      mustSignFixture(t, s2, claims2),
	})
	servers = append(servers, s2)

	// 3. 带密码的分享：顺带钉住**密码哈希**（HMAC 十六进制前 16 位）的算法。
	s3 := shareFixtureServer(t, "global-pw", nil)
	claims3 := shareClaims{
		Type:    "content",
		ID:      "42",
		Room:    "default",
		Exp:     1789955428,
		JTI:     "00112233445566778899aabbccddeeff",
		MaxUses: 1,
		PwdHash: s3.sharePasswordHash("hunter2"),
	}
	cases = append(cases, shareFixtureCase{
		Name:       "content-share-with-password",
		GlobalAuth: "global-pw",
		RoomAuth:   map[string]interface{}{},
		Claims:     claims3,
		Token:      mustSignFixture(t, s3, claims3),
	})
	servers = append(servers, s3)

	// 4. 全局作用域的会话令牌：`sc` 字段与 `id` / `room` 的关系也一起钉住。
	s4 := shareFixtureServer(t, "global-pw", nil)
	claims4 := shareClaims{
		Type:  "room_session",
		ID:    "default",
		Room:  "default",
		Scope: "global",
		Exp:   1789959028,
	}
	cases = append(cases, shareFixtureCase{
		Name:       "session-token-global-scope",
		GlobalAuth: "global-pw",
		RoomAuth:   map[string]interface{}{},
		Claims:     claims4,
		Token:      mustSignFixture(t, s4, claims4),
	})
	servers = append(servers, s4)

	// KeyHex 属于「服务端的派生结果」，不是入参，所以签名之后再取。
	for i, s := range servers {
		cases[i].KeyHex = hex.EncodeToString(s.shareSigningKey)
	}

	writeAndCheckShareFixtures(t, cases)
}

func writeAndCheckShareFixtures(t *testing.T, cases []shareFixtureCase) {
	t.Helper()

	file := shareFixtureFile{
		Note:  "由 cloud-clip/lib/share_fixture_test.go 生成，**不要手工编辑**。Rust 侧逐字节比对签出来的 token。",
		Cases: cases,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("编码 fixture 失败: %v", err)
	}
	data = append(data, '\n')

	path := filepath.Join(shareFixtureDir, "tokens.json")
	if os.Getenv("UPDATE_FIXTURES") == "1" {
		if err := os.MkdirAll(shareFixtureDir, 0o755); err != nil {
			t.Fatalf("创建 %s 失败: %v", shareFixtureDir, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("写入 %s 失败: %v", path, err)
		}
		t.Logf("已更新 %s", path)
		return
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败（首次生成请用 UPDATE_FIXTURES=1）: %v", path, err)
	}

	var want shareFixtureFile
	if err := json.Unmarshal(existing, &want); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	if len(want.Cases) != len(cases) {
		t.Fatalf("fixture 有 %d 条，这次算出 %d 条 —— 用例增删了，确认后用 UPDATE_FIXTURES=1 重新生成",
			len(want.Cases), len(cases))
	}
	for i := range cases {
		if want.Cases[i].Name != cases[i].Name {
			t.Fatalf("第 %d 条用例名不一致：fixture=%q，本次=%q", i, want.Cases[i].Name, cases[i].Name)
		}
		if want.Cases[i].KeyHex != cases[i].KeyHex {
			t.Errorf("用例 %s：**密钥派生**变了（所有已发出的分享链接会失效）\n  fixture=%s\n  本次   =%s",
				cases[i].Name, want.Cases[i].KeyHex, cases[i].KeyHex)
		}
		if want.Cases[i].Token != cases[i].Token {
			t.Errorf("用例 %s：**token 字节**变了\n  fixture=%s\n  本次   =%s",
				cases[i].Name, want.Cases[i].Token, cases[i].Token)
		}
	}
}
