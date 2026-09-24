package lib

import (
	"fmt"
	"strings"
)

/**
*** FILE: automation_source.go
***   「把某个房间的最新消息当输入源」——读取与权限。
**/

// 这个功能听起来只是「再加一个变量」，但它同时打开了两条新的通路，必须一起想清楚：
//
//  1. **读别的房间**。任务是无人值守的，它没有密码可带。如果沿用「创作者当时的凭据」，
//     那就成了一个提权面：先在读得到的时候建任务，之后对方改密码也照读不误。
//     所以规则定成**从紧**：
//       · 不带参数的 {{latest}} —— 取任务自己的房间，永远允许（任务本来就属于那个房间）；
//       · 带参数的 {{latest:房间}} —— 那个房间必须**不需要凭据就能读**（公开房间）；
//       · **管理员例外**：持全局密码（或它换来的会话令牌）的人可以引用**任意房间**。
//         他本来就是「能读所有房间」的那个人，拦他一步都拦不住 —— 详见
//         checkTemplateSourceRooms 里那段说明。
//
//  2. **内容注入**。能往房间 B 发消息的人，就能决定房间 A 里那条定时消息说什么。
//     这是这个功能的**本质**，不是缺陷 —— 「把 lobby 的最新消息转播到 home」正是这么用的。
//     要防的不是注入本身，而是**未经授权**的注入：所以第 1 条那些规则必须真的生效。

// latestRoomText 取某个房间里最新一条**人发的**文本消息。
//
// 跳过两类：
//   - 文件消息 —— 正文要的是文本，文件名对「转播」没有意义；
//   - Source == "automation" 的消息 —— **防回环**。任务读 A 发回 A 的话，
//     每一轮的输出都会成为下一轮的输入，内容会自我放大成一堆垃圾。
//     定时消息本来就带这个标记（见 scheduler.go 的 deliverMessage），正好用上。
//
// 找的是**历史队列**里的消息，所以 keepHistory=false 的定时消息天然不在其中。
func (s *ClipboardServer) latestRoomText(room string) (string, bool) {
	normalized := normalizeRoomName(room)

	s.messageQueue.Lock()
	defer s.messageQueue.Unlock()

	for i := len(s.messageQueue.List) - 1; i >= 0; i-- {
		text := s.messageQueue.List[i].Data.TextReceive
		if text == nil {
			continue
		}
		if normalizeRoomName(text.Room) != normalized {
			continue
		}
		if text.Source == "automation" {
			continue
		}
		if strings.TrimSpace(text.Content) == "" {
			continue
		}
		return text.Content, true
	}
	return "", false
}

// checkTemplateSourceRooms 模板里引用了别的房间时，当前凭据得能读它。
//
// 为什么只在**建任务/试算**这一步判：运行时没有任何凭据在手，
// 所以「能读」这个判断只能在有请求上下文的时候做。风险因此是：
// 建完之后对方给那个房间加了密码，任务会照读不误。
// 这里选择**接受**这个风险，而不是编一套「记住授权来源并重判」的机制 ——
// 后者要往任务里存一个授权档位，而自托管场景下房间策略本来就是房主自己改的，
// 改完顺手看一眼任务比多一个字段更直接。真要收紧的话，把下面这处判断搬到
// scheduler 里、每次运行前用 resolveRoomAuth 重判即可。
func (s *ClipboardServer) checkTemplateSourceRooms(tpl string, scope automationScope) error {
	// 不带参数的 {{latest}} 用的是任务自己的房间，不用判 —— 任务本来就属于它。
	_, rooms := TemplateLatestRooms(tpl)
	if len(rooms) == 0 {
		return nil
	}

	for _, room := range rooms {
		if room == normalizeRoomName(scope.Room) {
			continue
		}
		// 管理员可以引用**任意房间**，包括带密码的。
		//
		// 为什么这里放行是安全的：管理员本来就是「能读所有房间」的那个人
		// （`canAccessRoom` 对全局密码 / 全局会话令牌恒真），拦他只会逼他
		// 「先用管理员身份看一眼、再去建任务」—— 多一步，但一步都没拦住。
		// 而运行时读的是**内存里的历史队列**（见 latestRoomText），那里本来就不分房间。
		if scope.Admin {
			continue
		}
		req := s.resolveRoomAuth(room)
		if !req.Required {
			continue // 公开房间：谁都能读，任务当然也能
		}
		return fmt.Errorf(
			"正文里的 {{latest:%s}} 读不了：房间 %s 需要密码，而定时任务是无人值守的、没有密码可带。"+
				"只有**不需要密码就能读**的房间（或任务自己的房间）能作为来源；"+
				"管理员（持全局密码）可以引用任意房间",
			room, room)
	}
	return nil
}
