package lib

// `GET /content` 的 `limit` 取值规则：**有效上限 = min(server.history, 硬上限)**。
//
// 为什么单拎出来测：这条规则有三层（配置值、硬上限、请求参数），而它错了的表现是
// **静默的** —— 要么一次返回太多（把「一次推 2MB」从 WS 挪到 HTTP，等于这个变更白做），
// 要么比配置少（SPA 突然少看一截历史）。它是个纯函数，直接测比起服务端便宜得多。

import "testing"

func TestResolveContentListLimit(t *testing.T) {
	cases := []struct {
		name    string
		history int
		raw     string
		want    int
	}{
		// 缺省值 = 有效上限（同一根旋钮）
		{"缺省 → 跟配置（配置小于硬上限）", 50, "", 50},
		{"缺省 → 被硬上限夹住（配置大于硬上限）", 10000, "", contentListHardCap},
		{"恰好等于硬上限", contentListHardCap, "", contentListHardCap},

		// 显式 limit
		{"显式小于配置", 50, "20", 20},
		{"显式等于配置", 50, "50", 50},
		{"显式超过配置 → 夹到配置", 50, "999999", 50},
		{"显式超过硬上限 → 夹到硬上限", 10000, "999999", contentListHardCap},

		// 非法值一律退回有效上限，**不报错**
		{"非数字", 50, "abc", 50},
		{"负数", 50, "-1", 50},
		{"零", 50, "0", 50},
		{"空白", 50, "   ", 50},

		// 配置本身是病态的（0 / 负）→ 一条都不返回，别悄悄放大
		{"配置为 0", 0, "", 0},
		{"配置为负", -1, "", 0},
		{"配置为 0 但显式要 10", 0, "10", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.Server.History = c.history
			s := &ClipboardServer{config: cfg}
			if got := s.resolveContentListLimit(c.raw); got != c.want {
				t.Fatalf("history=%d limit=%q: 得到 %d，想要 %d", c.history, c.raw, got, c.want)
			}
		})
	}
}

// 硬上限必须**真的**比内置缺省大 —— 否则默认部署的行为会变（§2.1 的「与改动前逐条一致」）。
func TestContentListHardCapLeavesDefaultDeploymentAlone(t *testing.T) {
	if got := defaultConfig().Server.History; got > contentListHardCap {
		t.Fatalf("内置缺省 %d 已经超过硬上限 %d —— 默认部署会少看历史", got, contentListHardCap)
	}
}
