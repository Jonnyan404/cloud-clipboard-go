package lib

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ua-parser/uap-go/uaparser"
)

// TestResolveDeviceName 覆盖 ?name= 的读取与清洗。
// 空串是「客户端未声明」的信号，调用方据此回落 UA，所以这里必须严格区分
// 「没传」「传了空白」「传了正常值」三种情况。
func TestResolveDeviceName(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"未传参数", "", ""},
		{"传了空值", "?name=", ""},
		{"只传空白", "?name=%20%20", ""},
		{"正常英文", "?name=My%20MacBook", "My MacBook"},
		{"正常中文", "?name=iOS%20%E5%BF%AB%E6%8D%B7%E6%8C%87%E4%BB%A4", "iOS 快捷指令"},
		{"首尾空白被裁掉", "?name=%20%20iPad%20%20", "iPad"},
		{"控制字符被剔除", "?name=a%0Ab%0Dc%00d", "abcd"},
		{"超长按字符截断", "?name=" + strings.Repeat("%E5%BF%AB", 40), strings.Repeat("快", deviceNameMaxLen)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/text"+tc.query, nil)
			if got := resolveDeviceName(r); got != tc.want {
				t.Fatalf("resolveDeviceName(%q) = %q, 期望 %q", tc.query, got, tc.want)
			}
		})
	}
}

// TestSanitizeDeviceNameMultibyteSafe 确认截断不会把多字节字符切成半个，
// 否则前端会渲染出乱码方块。
func TestSanitizeDeviceNameMultibyteSafe(t *testing.T) {
	got := sanitizeDeviceName(strings.Repeat("岚", deviceNameMaxLen+5))
	if !strings.HasPrefix(got, "岚") {
		t.Fatalf("截断结果首字符异常: %q", got)
	}
	if n := len([]rune(got)); n != deviceNameMaxLen {
		t.Fatalf("截断后字符数 = %d, 期望 %d", n, deviceNameMaxLen)
	}
}

// TestDetectDeviceType 锁住归类规则。
// 重点是把「iOS 17 + desktop」这个自相矛盾的组合钉死 —— 快捷指令的 UA 是
// BackgroundShortcutRunner，不含任何移动端关键词，曾经因此被判成桌面设备。
func TestDetectDeviceType(t *testing.T) {
	cases := []struct {
		name     string
		ua       string
		osFamily string
		want     string
	}{
		{
			name:     "快捷指令不再被误判成桌面",
			ua:       "BackgroundShortcutRunner/2607.0.0 CFNetwork/1498.700.2 Darwin/23.6.0",
			osFamily: "iOS",
			want:     "smartphone",
		},
		{
			name:     "Chrome on macOS 仍是桌面",
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/151.0.0.0 Safari/537.36",
			osFamily: "Mac OS X",
			want:     "desktop",
		},
		{
			name:     "curl 仍是桌面",
			ua:       "curl/8.7.1",
			osFamily: "Other",
			want:     "desktop",
		},
		{
			name:     "关键词命中优先于 OS 兜底",
			ua:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile/15E148",
			osFamily: "iOS",
			want:     "smartphone",
		},
		{
			name:     "iPad 仍是平板",
			ua:       "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) Mobile/15E148",
			osFamily: "iOS",
			want:     "tablet",
		},
		{
			name:     "Android 仍是手机",
			ua:       "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/151.0.0.0 Mobile Safari/537.36",
			osFamily: "Android",
			want:     "smartphone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectDeviceType(tc.ua, tc.osFamily); got != tc.want {
				t.Fatalf("detectDeviceType() = %q, 期望 %q", got, tc.want)
			}
		})
	}
}

// TestParseUserAgentNameFallback 确认未声明名字时不写入 name 字段，
// 让旧客户端的载荷与改动前逐字一致。
func TestParseUserAgentNameFallback(t *testing.T) {
	s := newExpireTestServer(t, 0)
	// newExpireTestServer 造的是最小服务器，没有 UA 解析器，这里补上
	s.parser = uaparser.NewFromSaved()
	ua := "curl/8.7.1"

	without := s.parse_user_agent(ua, "")
	if _, ok := without["name"]; ok {
		t.Fatalf("未声明名字时不应出现 name 字段, 实际: %v", without)
	}

	with := s.parse_user_agent(ua, "我的树莓派")
	if got := with["name"]; got != "我的树莓派" {
		t.Fatalf("name = %q, 期望 %q", got, "我的树莓派")
	}
}
