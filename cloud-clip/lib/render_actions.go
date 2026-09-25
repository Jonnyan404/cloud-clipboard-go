package lib

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

/**
*** FILE: render_actions.go
***   动作库在服务端的「可下沉子集」—— 定时任务能用的那部分动作。
**/

// 动作库（web-vue3/src/data/actions.js）现在有 45 个动作，`direction` 有 view（给「看」用）
// 和 insert（给「写」用）。定时任务是第三种用途：**给「发」用** —— 它是无人值守时被执行的动作，
// 所以只收同时满足下面全部条件的那一档。
//
// 判定标准从动作库自己已有的约定推出来，不新增教条：
//
//  1. **只收 `direction: 'view'`（变换类）。** `insert` 是「生成」（uuid / time / datetime），
//     它的语义是「往编辑区**插入**一块新内容」，在无人值守的管道里没有对应物 ——
//     放进链里就是「某一步突然把前面算出来的正文整个丢掉」。
//     ⚠️ 这不损失任何能力：生成类要的效果用**模板变量**更好
//     （`{{uuid}}` / `{{time}}` / `{{datetime}}` / `{{timestamp}}`），
//     变量是**内联**的（`订单号：{{uuid}}` 直接写在正文里），不会覆盖任何东西。
//  2. 纯函数，不碰 DOM / 浏览器专有 API
//  3. 只依赖入参和时钟，不依赖「此刻这台机器上的什么状态」
//  4. 不发网络请求（这条动作库本来就定了）
//
// 不在集合里的分四类，**都不是被砍掉，而是「只在客户端可执行」**：
//   - 产出 HTML 给「看」的：`format.markdown` / `format.code`
//   - 依赖浏览器里动态 import 的词典：`zh.pinyin*` / `zh.simplified` / `zh.traditional`
//   - 输出本身需要翻译、或判定依赖前端启发式的：`inspect.stats` / `inspect.detect`
//   - 上面第 1 条排除的生成类：`generate.*`
//
// ⚠️ 管理页**只列服务端下发的可用动作**，它没有「置灰不可用动作」这回事 ——
// 所以用户看不到「某个动作为什么不能用于定时任务」，只看到列表短了一截。
// 要真正交代清楚，得让服务端把「不可用 + 原因」也下发出去，那是另一件事。
//
// ⚠️ id **必须和前端 data/actions.js 里的逐字一致**。任务里存的就是 id，
// 两侧同名才能保证「同一个任务，谁执行结果都一样」。这条由契约测试守着（render_actions_test.go）。
//
// ⚠️ 不在表里的 id 一律**报错**，不静默跳过。静默跳过会让用户以为动作生效了，
// 等到消息真的发进房间才发现没跑 —— 而那时已经过去了几个小时甚至几天。

// renderActionSpec 一个可下沉动作。
//
// GroupKey / Key 是**前端 locale 文件里的 i18n key**（`actionGroupText` / `actionTrimLines` 之类），
// 前端拿它去自己的文案表里取人话名字。
//
// ⚠️ 服务端**只给 key，不给译文**。译文只应该有一份、放在前端的 locales 里；
// 在 Go 里再抄一份中文，就多出一个迟早会漂开的副本 —— 而「同一个动作在两个界面里叫不同名字」
// 正是用户最先会注意到的错。`render_actions_test.go` 会校验每个 key 都能在四份 locale 里查到。
type renderActionSpec struct {
	ID       string
	Group    string
	GroupKey string
	Key      string

	// Params 声明这个动作**需要哪些参数**（空 = 无参数动作，绝大多数）。
	//
	// ⚠️ 这里只有「长什么样」（key + 标签的 i18n key），参数的**值**存在链元素上
	// （见 task.go 的 AutomationChainStep.Params）—— 同一个动作可以在一条链上出现两次、
	// 两次用不同的参数（「替换 A→B」再接「替换 C→D」是合法意图，链本来就允许重复）。
	Params []renderActionParam

	Run func(text string, ctx renderContext, params map[string]string) (string, error)
}

// renderActionParam 一个参数的声明。
//
// LabelKey 是**前端 locale 文件里的 i18n key**，和动作名同一个约定：
// 服务端只给 key、不给译文，译文只留一份（见本文件头部）。
type renderActionParam struct {
	Key      string
	LabelKey string

	// Type 决定界面渲染成什么控件：空 / `text` = 单行输入（默认），`select` = 下拉。
	Type string

	// Options 只在 Type == "select" 时有意义。
	//
	// ⚠️ **第一项就是默认值** —— 不另设 default 字段：同一个意思写两处，迟早会不一致。
	Options []renderActionParamOption

	// VisibleWhen 让这个参数**只在另一个参数取了某个值时才显示**。
	// 目前只有「查找」用它：选了「数字 / 邮箱」这类模式之后，手填的查找词就没意义了，
	// 还留在界面上只会让人以为「填了不生效是 bug」。
	VisibleWhen *renderActionParamCondition
}

// renderActionParamOption 下拉里的一项。Value 是要存进链元素的值，LabelKey 是它的文案。
type renderActionParamOption struct {
	Value    string
	LabelKey string
}

// renderActionParamCondition 「当 Key 这个参数等于 Equals 时才显示」。
type renderActionParamCondition struct {
	Key    string
	Equals string
}

var serverRenderActions = []renderActionSpec{
	// ── 格式化 ──────────────────────────────────────────────────────
	//
	// ⚠️ `format.markdown` / `format.code` **刻意不在这里**：它们产出的是**给人看的 HTML**
	// （要 marked / highlight.js 那一套），而定时任务是「无人值守地发文本」——
	// 往房间里发一段 HTML 没有意义。这两个连同 pinyin / opencc 一起，
	// 在管理页里是**置灰**的（见 ServerRenderActionIDs 的用法）。
	{ID: "format.json.pretty", Group: "format", GroupKey: "actionGroupFormat", Key: "actionJsonPretty", Run: renderJSONPretty},
	{ID: "format.json.min", Group: "format", GroupKey: "actionGroupFormat", Key: "actionJsonMin", Run: renderJSONMin},

	// ── 文本 ────────────────────────────────────────────────────────
	{ID: "text.trimLines", Group: "text", GroupKey: "actionGroupText", Key: "actionTrimLines", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimSpace(line)
		}
		return strings.Join(lines, "\n"), nil
	}},
	{ID: "text.dropBlank", Group: "text", GroupKey: "actionGroupText", Key: "actionDropBlankLines", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		lines := strings.Split(s, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "\n"), nil
	}},
	// 空行不去重 —— 连着几个空行是有意的排版，去掉会改变结构（与前端同一条规则）。
	{ID: "text.dedupe", Group: "text", GroupKey: "actionGroupText", Key: "actionDedupeLines", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		seen := make(map[string]bool)
		lines := strings.Split(s, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) != "" && seen[line] {
				continue
			}
			seen[line] = true
			kept = append(kept, line)
		}
		return strings.Join(kept, "\n"), nil
	}},
	// ⚠️ 与前端有一处**已知差异**：前端用 `localeCompare(b, 'zh')`（中文拼音序），
	// 服务端用字节序。小数、纯英文、日期的排序两边一致，中文多音字场景会有出入。
	// 契约测试只冻结「排序稳定且行集合不变」，不冻结中文的具体次序 —— 与其做一个
	// 半吊子的拼音库，不如把差异写在这里。
	{ID: "text.sort", Group: "text", GroupKey: "actionGroupText", Key: "actionSortLines", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		lines := strings.Split(s, "\n")
		sort.Strings(lines)
		return strings.Join(lines, "\n"), nil
	}},
	{ID: "text.upper", Group: "text", GroupKey: "actionGroupText", Key: "actionUpperCase", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return strings.ToUpper(s), nil
	}},
	{ID: "text.lower", Group: "text", GroupKey: "actionGroupText", Key: "actionLowerCase", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return strings.ToLower(s), nil
	}},
	// 第一个**带参数**的动作（见 renderActionSpec.Params）。参数的值存在链元素上，
	// 所以同一条链上可以出现两次、两次用不同参数。
	{ID: "text.replace", Group: "text", GroupKey: "actionGroupText", Key: "actionReplace",
		Params: []renderActionParam{
			// ⚠️ 第一个选项就是默认值 = 字面替换，也就是这个动作原本的行为。
			{Key: "mode", LabelKey: "actionReplaceMode", Type: "select", Options: []renderActionParamOption{
				{Value: replaceModeText, LabelKey: "actionReplaceModeText"},
				{Value: replaceModeDigits, LabelKey: "actionReplaceModeDigits"},
				{Value: replaceModeLatin, LabelKey: "actionReplaceModeLatin"},
				{Value: replaceModeSpaces, LabelKey: "actionReplaceModeSpaces"},
				{Value: replaceModeEmail, LabelKey: "actionReplaceModeEmail"},
				{Value: replaceModeURL, LabelKey: "actionReplaceModeUrl"},
				{Value: replaceModePhone, LabelKey: "actionReplaceModePhone"},
				{Value: replaceModeIP, LabelKey: "actionReplaceModeIp"},
			}},
			// 「查找」只在「文本」模式下有意义 —— 其余模式自己决定匹配什么。
			// 留着不隐藏的话，用户填了发现不生效，只会以为功能坏了。
			{Key: "find", LabelKey: "actionReplaceFind", VisibleWhen: &renderActionParamCondition{
				Key: "mode", Equals: replaceModeText,
			}},
			{Key: "with", LabelKey: "actionReplaceWith"},
		},
		Run: renderReplaceLiteral},
	// 按**码点**反转（前端是 Array.from + reverse）—— 不是按字节，也不是按 UTF-16 单元：
	// 后者会把 emoji 的代理对拆开，变成两个乱码字符。
	{ID: "text.reverse", Group: "text", GroupKey: "actionGroupText", Key: "actionReverse", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		runes := []rune(s)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes), nil
	}},
	// 提取类：都是**启发式**（宁可多捞一个也不漏），输出按出现顺序、去重，每行一个。
	// 正则和前端 data/actions.js 的 XXX_RE 逐字对应，改一处要同步另一处。
	{ID: "text.extractUrl", Group: "text", GroupKey: "actionGroupText", Key: "actionExtractUrl", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return extractJoined(s, urlRe), nil
	}},
	{ID: "text.extractEmail", Group: "text", GroupKey: "actionGroupText", Key: "actionExtractEmail", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return extractJoined(s, emailRe), nil
	}},
	{ID: "text.extractPhone", Group: "text", GroupKey: "actionGroupText", Key: "actionExtractPhone", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return extractPhones(s), nil
	}},
	{ID: "text.extractIp", Group: "text", GroupKey: "actionGroupText", Key: "actionExtractIp", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return extractJoined(s, ipv4Re), nil
	}},
	{ID: "text.extractNumber", Group: "text", GroupKey: "actionGroupText", Key: "actionExtractNumber", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return extractJoined(s, numberRe), nil
	}},

	// ── 日期 ────────────────────────────────────────────────────────
	{ID: "date.add", Group: "date", GroupKey: "actionGroupDate", Key: "actionDateAdd", Run: renderDateAdd},
	{ID: "date.diff", Group: "date", GroupKey: "actionGroupDate", Key: "actionDateDiff", Run: renderDateDiff},

	// ── 编解码 ──────────────────────────────────────────────────────
	{ID: "encode.base64", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionBase64Encode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	}},
	{ID: "encode.base64.decode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionBase64Decode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		// ⚠️ 只认标准（带 =）的写法，**不做无填充回退**：前端用的是 atob，
		// 而 atob('abc') 会抛错（长度不是 4 的倍数）。回落一次确实能多收几个输入，
		// 但代价是「同一个动作在预览区报错、在定时任务里却跑出结果」——
		// 这种两侧不一致比少支持一种写法糟糕得多。
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil {
			return "", fmt.Errorf("不是合法的 Base64: %w", err)
		}
		return string(raw), nil
	}},
	{ID: "encode.url", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionUrlEncode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return encodeURIComponent(s), nil
	}},
	{ID: "encode.url.decode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionUrlDecode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		// ⚠️ 不能用 QueryUnescape：它会把 `+` 当成空格，而 decodeURIComponent 不会。
		out, err := urlPathUnescape(s)
		if err != nil {
			return "", fmt.Errorf("不是合法的 URL 编码: %w", err)
		}
		return out, nil
	}},
	{ID: "encode.hex", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionHexEncode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		bytes := []byte(s)
		parts := make([]string, len(bytes))
		for i, b := range bytes {
			parts[i] = hex.EncodeToString([]byte{b})
		}
		return strings.Join(parts, " "), nil
	}},
	{ID: "encode.hex.decode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionHexDecode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		clean := hexOnlyRe.ReplaceAllString(s, "")
		if len(clean)%2 != 0 {
			return "", fmt.Errorf("hex 长度必须是偶数")
		}
		raw, err := hex.DecodeString(clean)
		if err != nil {
			return "", fmt.Errorf("不是合法的 hex: %w", err)
		}
		return string(raw), nil
	}},
	{ID: "encode.html", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionHtmlEncode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return encodeHTMLEntities(s), nil
	}},
	{ID: "encode.html.decode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionHtmlDecode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		// ⚠️ 与前端有一处**已知差异**：前端拿 `<textarea>.innerHTML` 让浏览器解码，
		// 服务端用 html.UnescapeString。两者都认 HTML5 命名实体，但对
		// 「没有分号的写法」「不认识的实体名」的容忍度不完全一样。
		// 常见实体（&amp; &lt; &#39; &nbsp; …）两边一致；罕见写法可能一边解一边原样留。
		// 与其自己抄一张 2000 项的表（抄错一项就永远不知道），不如把差异写在这里。
		return html.UnescapeString(s), nil
	}},
	{ID: "encode.unicode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionUnicodeEncode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return toUnicodeEscapes(s), nil
	}},
	{ID: "encode.unicode.decode", Group: "encode", GroupKey: "actionGroupEncode", Key: "actionUnicodeDecode", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return fromUnicodeEscapes(s), nil
	}},

	// ── 中文 ────────────────────────────────────────────────────────
	//
	// 只收**不需要词典**的这四个：全角/半角是纯码点运算，标点是逐字查表，数字大写是算法。
	// `zh.pinyin*`（pinyin-pro 317KB）和 `zh.simplified` / `zh.traditional`（opencc 6MB）
	// 都靠浏览器里动态 import 的词典，服务端没有 —— 它们在管理页里是置灰的。
	{ID: "zh.fullwidth", Group: "zh", GroupKey: "actionGroupZh", Key: "actionFullWidth", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return toFullWidth(s), nil
	}},
	{ID: "zh.halfwidth", Group: "zh", GroupKey: "actionGroupZh", Key: "actionHalfWidth", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return toHalfWidth(s), nil
	}},
	{ID: "zh.punctuation", Group: "zh", GroupKey: "actionGroupZh", Key: "actionCnPunctuation", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return cnPunctuationToEn(s), nil
	}},
	{ID: "zh.number", Group: "zh", GroupKey: "actionGroupZh", Key: "actionNumberToChinese", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return numberToChinese(s)
	}},

	// ── 校验 / 时间戳 ───────────────────────────────────────────────
	//
	// ⚠️ `generate.*`（uuid / time / datetime）**刻意不在这里**，理由见文件头
	// 「为什么只收 direction: 'view'」那一段。一句话：它们是「生成」，
	// 放进链里会把前面算出来的正文**整个丢掉**；而它们要的效果用**模板变量**
	// （`{{uuid}}` / `{{time}}` / `{{datetime}}`）就能拿到，而且是内联的、不覆盖任何东西。
	{ID: "inspect.timestamp", Group: "inspect", GroupKey: "actionGroupInspect", Key: "actionTimestampToDate", Run: func(s string, ctx renderContext, _ map[string]string) (string, error) {
		return timestampToDateText(s, ctx)
	}},
	{ID: "inspect.dateToTimestamp", Group: "inspect", GroupKey: "actionGroupInspect", Key: "actionDateToTimestamp", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		return dateTextToTimestamp(s)
	}},
	{ID: "inspect.sha256", Group: "inspect", GroupKey: "actionGroupInspect", Key: "actionSha256", Run: func(s string, _ renderContext, _ map[string]string) (string, error) {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:]), nil
	}},
}

var hexOnlyRe = regexp.MustCompile(`[^0-9A-Fa-f]`)

var serverRenderActionIndex = func() map[string]renderActionSpec {
	idx := make(map[string]renderActionSpec, len(serverRenderActions))
	for _, spec := range serverRenderActions {
		idx[spec.ID] = spec
	}
	return idx
}()

// ServerRenderActionIDs 服务端能执行的动作 id（顺序稳定）。给 /server 下发、给管理页置灰判断用。
func ServerRenderActionIDs() []string {
	ids := make([]string, 0, len(serverRenderActions))
	for _, spec := range serverRenderActions {
		ids = append(ids, spec.ID)
	}
	return ids
}

// ServerRenderActionList 带分组、i18n key 与**参数声明**的动作清单，管理页按组渲染。
//
// 只给 key 不给译文，理由见 renderActionSpec 的注释。
//
// `params` 只在动作**需要参数**时才出现（无参数动作连这个键都没有）——
// 管理页据此决定「点一下直接加一步」还是「加一步 + 就地渲染两个输入框」。
func ServerRenderActionList() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(serverRenderActions))
	for _, spec := range serverRenderActions {
		item := map[string]interface{}{
			"id":       spec.ID,
			"group":    spec.Group,
			"groupKey": spec.GroupKey,
			"key":      spec.Key,
		}
		if len(spec.Params) > 0 {
			params := make([]map[string]interface{}, 0, len(spec.Params))
			for _, p := range spec.Params {
				param := map[string]interface{}{
					"key":      p.Key,
					"labelKey": p.LabelKey,
				}
				// 空 Type = 单行输入，是默认形态 —— 不写进响应，省得前端要认一个「默认值」
				if p.Type != "" {
					param["type"] = p.Type
				}
				if len(p.Options) > 0 {
					options := make([]map[string]string, 0, len(p.Options))
					for _, o := range p.Options {
						options = append(options, map[string]string{
							"value":    o.Value,
							"labelKey": o.LabelKey,
						})
					}
					param["options"] = options
				}
				if p.VisibleWhen != nil {
					param["visibleWhen"] = map[string]string{
						"key":    p.VisibleWhen.Key,
						"equals": p.VisibleWhen.Equals,
					}
				}
				params = append(params, param)
			}
			item["params"] = params
		}
		out = append(out, item)
	}
	return out
}

// ServerRenderActionI18nKeys 全部用到的 i18n key（动作名 + 分组名）。
// 契约测试用它去四份 locale 里逐个查 —— 少一个译文，界面上就会出现一个动作 id。
func ServerRenderActionI18nKeys() []string {
	keys := make([]string, 0, len(serverRenderActions)*2)
	seen := map[string]bool{}
	for _, spec := range serverRenderActions {
		// 动作名 + 分组名 + **每个参数的标签**都要有译文，
		// 少一个界面上就会出现一个裸 key（参数标签尤其容易漏 —— 它是后加的）。
		names := []string{spec.Key, spec.GroupKey}
		for _, p := range spec.Params {
			names = append(names, p.LabelKey)
			// ⚠️ 下拉选项的标签也要有译文 —— 漏了的话界面上会冒出一排裸 key。
			for _, o := range p.Options {
				names = append(names, o.LabelKey)
			}
		}
		for _, k := range names {
			if k != "" && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// IsServerRenderAction 这个 id 服务端能不能跑。
func IsServerRenderAction(id string) bool {
	_, ok := serverRenderActionIndex[strings.TrimSpace(id)]
	return ok
}

// applyRenderActions 按顺序跑一条动作链，某一步失败就停在那里。
//
// 「失败即停」与前端 runChain 一致：链上后一步的输入依赖前一步的输出，
// 硬着头皮跑下去得到的东西没有意义，反而会发出一条**看起来正常**的错误消息。
func applyRenderActions(text string, chain []AutomationChainStep, ctx renderContext) (string, error) {
	current := text
	for _, step := range chain {
		id := strings.TrimSpace(step.ID)
		if id == "" {
			continue
		}
		spec, ok := serverRenderActionIndex[id]
		if !ok {
			return "", fmt.Errorf("动作 %q 不能用于定时任务（服务端可执行的动作：%s）",
				id, strings.Join(ServerRenderActionIDs(), " / "))
		}
		out, err := spec.Run(current, ctx, step.Params)
		if err != nil {
			return "", fmt.Errorf("动作 %s 执行失败: %w", id, err)
		}
		current = out
	}
	return current, nil
}

// ── encodeURIComponent 的等价实现 ────────────────────────────────────
//
// 为什么不用 url.QueryEscape：它有两处不同 —— 空格编成 `+`（而非 `%20`），
// 且不转义 `!~*'()`（encodeURIComponent 会转义）。同一段文本在预览区（前端动作库）
// 和定时任务里编码出不同结果，是那种「看起来都对、对比一下才发现不一样」的错。
const uriComponentUnreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"

func encodeURIComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && strings.ContainsRune(uriComponentUnreserved, r) {
			b.WriteRune(r)
			continue
		}
		for _, by := range []byte(string(r)) {
			b.WriteString("%")
			b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{by})))
		}
	}
	return b.String()
}

// urlPathUnescape 等价于 decodeURIComponent：不把 `+` 当空格。
func urlPathUnescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '%' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("截断的百分号转义")
		}
		by, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("非法的百分号转义 %q", s[i:i+3])
		}
		b.WriteByte(byte(by))
		i += 3
	}
	return b.String(), nil
}

// ── 日期 ────────────────────────────────────────────────────────────
//
// 输入是**约定式**的（与前端同一套）：动作是单输入单输出，而日期计算天然要两个参数，
// 所以把它们写在同一段文本里。
//   · date.add ：`2026-01-01 +30d`（省略基准则从**触发时刻**算）
//   · date.diff：两行日期，或 `2026-01-01 ~ 2026-03-15`

func renderDateAdd(text string, ctx renderContext, _ map[string]string) (string, error) {
	m := dateAddInputRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", fmt.Errorf("写法：2026-01-01 +30d（单位 d/w/m/y，省略基准则从触发时刻算）")
	}
	baseRaw, sign, amountRaw, unitRaw := m[1], m[2], m[3], m[4]

	base := ctx.Now
	if strings.TrimSpace(baseRaw) != "" {
		parsed, ok := parseDateToken(baseRaw, ctx.Now.Location(), ctx.Now)
		if !ok {
			// 写了基准但认不出 → 报错。**别悄悄回落到「今天」** ——
			// 那会让用户拿到一个看着合理、其实完全不对的结果。
			return "", fmt.Errorf("认不出这个日期: %s", strings.TrimSpace(baseRaw))
		}
		base = parsed
	}

	amount, err := strconv.Atoi(amountRaw)
	if err != nil {
		return "", fmt.Errorf("无法解析数量 %q: %w", amountRaw, err)
	}
	if sign == "-" {
		amount = -amount
	}

	withTime := strings.Contains(baseRaw, ":")

	var result time.Time
	switch strings.ToLower(unitRaw) {
	case "m":
		result = addMonthsClamped(base, amount)
	case "y":
		result = addMonthsClamped(base, amount*12)
	case "w":
		result = base.AddDate(0, 0, amount*7)
	default:
		result = base.AddDate(0, 0, amount)
	}

	if withTime {
		return result.Format("2006-01-02 15:04"), nil
	}
	return result.Format("2006-01-02"), nil
}

func renderDateDiff(text string, ctx renderContext, _ map[string]string) (string, error) {
	loc := ctx.Now.Location()
	parts := dateDiffSepRe.Split(strings.TrimSpace(text), -1)
	if len(parts) < 2 {
		parts = strings.Split(strings.TrimSpace(text), "\n")
	}
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) < 2 {
		return "", fmt.Errorf("写法：两行日期，或 2026-01-01 ~ 2026-03-15")
	}

	a, okA := parseDateToken(cleaned[0], loc, ctx.Now)
	b, okB := parseDateToken(cleaned[1], loc, ctx.Now)
	if !okA || !okB {
		return "", fmt.Errorf("认不出这个日期")
	}

	// 用**本地零点**算差，不能用时间戳差：夏令时那天只有 23 小时，直接除会少一天。
	days := int((startOfDay(b).Unix() - startOfDay(a).Unix()) / 86400)
	abs := days
	if abs < 0 {
		abs = -abs
	}
	weeks := abs / 7
	rest := abs % 7

	lines := []string{fmt.Sprintf("%d 天", days)}
	if weeks > 0 {
		lines = append(lines, fmt.Sprintf("%d 周 %d 天", weeks, rest))
	}
	return strings.Join(lines, "\n"), nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// 一个「日期 token」的三种写法，与前端 parseDateToken 认的一致：
//
//	ISO / 斜杠：2026-09-23、2026/9/23（可跟 10:30 或 T10:30:00）
//	中文：      2026年09月23日
//	紧凑：      20260923
var dateTokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?)?$`),
	regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日?(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?)?$`),
	regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`),
}

// 日期关键词 → 相对今天的天数偏移。
var dateKeywords = map[string]int{
	"今天": 0, "明天": 1, "昨天": -1,
	"today": 0, "tomorrow": 1, "yesterday": -1,
}

// parseDateToken 解析一个日期 token，返回**任务时区**下的时刻；认不出返回 false。
//
// ⚠️ 必须用 time.Date(..., loc) 构造，不能 time.Parse(time.RFC3339, ...) ——
// 后者会当成 UTC，在东八区会整体差 8 小时，日期计算直接错一天。
//
// ⚠️ `now` 是**基准时刻**（调用方传 ctx.Now），**不要**在函数里用 time.Now()：
// 「今天 / 明天 / 昨天」要按它算。用真实时间的话，**试算和实发会得到不同结果** ——
// 试算的 ctx.Now 是「下次触发时刻」（未来），实发时才是当前时刻。
// 而且测试里注入的固定 Now 也会失效，于是断言跟着真实日期漂（每天早上红一次）。
func parseDateToken(raw string, loc *time.Location, now time.Time) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}

	if offset, ok := dateKeywords[strings.ToLower(s)]; ok {
		n := now.In(loc)
		// 关键词按「今天」算：归一化到零点再加偏移，避免把当前时分秒带进去
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, offset), true
	}

	for _, re := range dateTokenPatterns {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		hour, minute, second := 0, 0, 0
		// ⚠️⚠️ **下标必须按捕获组个数守一遍**：紧凑写法（`20260923`）那条正则
		// **没有**时间捕获组，它的 `m` 只有 4 项 —— 直接取 `m[4]` 会下标越界 panic。
		//
		// 这个 panic 不是理论上的（2026-09-25 实测撞到）：`date.add` 的输入是**用户写的正文**，
		// 于是「任务正文里写 `20260924 +1d`」→ 模板渲染 → 链上 date.add → panic。
		// 而它跑在**调度器 goroutine** 里，`lib` 里一个 recover 都没有 —— 那就是整个进程退出。
		// （`handleTaskPreview` 那条路是 HTTP，net/http 会按连接兜住，所以只崩一个请求。）
		//
		// 原来的测试只覆盖了 `2026-02-31 +1d`（走的是下面那条**回读校验**的错路），
		// 紧凑写法这条**成功路径**没人写过用例，所以它一直没被发现。
		//
		// 修法刻意选「按个数守」而不是「给第三条正则也补上时间捕获组」：后者等于顺手把
		// 紧凑写法扩展成能带时间（`20260924T10:30`），那是**行为扩展**，不是修 bug ——
		// 要扩也得单独一次决定，别混在修 panic 里。
		hasTimeGroups := len(m) > 4
		if hasTimeGroups && m[4] != "" {
			hour, _ = strconv.Atoi(m[4])
		}
		if hasTimeGroups && m[5] != "" {
			minute, _ = strconv.Atoi(m[5])
		}
		if hasTimeGroups && m[6] != "" {
			second, _ = strconv.Atoi(m[6])
		}

		date := time.Date(year, time.Month(month), day, hour, minute, second, 0, loc)
		// ⚠️ 回读校验：`2026-02-31` 会被 time.Date **悄悄滚到** 3 月 3 日。
		// 不校验的话，用户写错了日期却拿到一个「看起来正常」的结果 —— 比直接报错糟糕得多。
		if date.Year() != year || int(date.Month()) != month || date.Day() != day {
			return time.Time{}, false
		}
		return date, true
	}

	return time.Time{}, false
}

// ── JSON ────────────────────────────────────────────────────────────
//
// ⚠️ 用 `json.Indent` / `json.Compact`（对**原始字节**重排空白），
// **不是** `Unmarshal` 再 `Marshal`。后者会把对象的键**按字典序重排**
// （Go 的 map 序列化就是这么做的），而前端的 `JSON.stringify` 保持解析时的顺序 ——
// 那会让「预览区看到的」和「定时任务发出去的」键顺序不一样，一眼就能看出来。
//
// 代价是数字/字符串的**写法**会原样保留（`1.0` 不会变成 `1`，`\u0041` 不会变成 `A`），
// 而前端会规范化。这个方向是安全的：输出仍然是合法 JSON，只是更贴近原文。

func renderJSONPretty(s string, _ renderContext, _ map[string]string) (string, error) {
	trimmed := strings.TrimSpace(s)
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err != nil {
		return "", fmt.Errorf("不是合法的 JSON: %w", err)
	}
	out := buf.String()
	// 与前端同一条约定：**结果和原文一样就算失败**（抛错而不是返回原样）——
	// 「点了没反应的按钮」比没有这个按钮更糟，用户分不清是没可压的还是功能坏了。
	if out == trimmed {
		return "", fmt.Errorf("已经是美化过的 JSON")
	}
	return out, nil
}

func renderJSONMin(s string, _ renderContext, _ map[string]string) (string, error) {
	trimmed := strings.TrimSpace(s)
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return "", fmt.Errorf("不是合法的 JSON: %w", err)
	}
	out := buf.String()
	if out == trimmed {
		return "", fmt.Errorf("已经是一行，压不动了")
	}
	return out, nil
}

// ── HTML 实体 ───────────────────────────────────────────────────────
//
// ⚠️ 不用 `html.EscapeString`：它连单引号也转（`'` → `&#39;`），而前端只转四个
// （`& < > "`）。转多了会让「同一个动作在两个界面输出不同」。
var htmlEntityReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&quot;",
)

func encodeHTMLEntities(s string) string {
	// NewReplacer 是**单遍**替换（不会把刚替换出来的 `&amp;` 再转一次），
	// 和前端那串链式 .replace() 等价。
	return htmlEntityReplacer.Replace(s)
}

// ── Unicode 转义 ────────────────────────────────────────────────────

// toUnicodeEscapes 把非 ASCII / 不可打印字符转成 `\uXXXX`。
//
// ⚠️ 必须按 **UTF-16 单元**遍历，不能按 rune：emoji（一个码点 = 两个单元）按 rune
// 处理会被当成单字符，输出 `\ud83e` 把低半截丢掉，而且**往返不回来**。
// 前端在这里踩过同一个坑（`岚🪶` → `岚\ud83e`），两边现在走的是同一套单元遍历。
func toUnicodeEscapes(s string) string {
	var b strings.Builder
	for _, unit := range utf16.Encode([]rune(s)) {
		if unit > 0x7e || unit < 0x20 {
			fmt.Fprintf(&b, "\\u%04x", unit)
			continue
		}
		b.WriteRune(rune(unit))
	}
	return b.String()
}

var unicodeEscapeRe = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})`)

// fromUnicodeEscapes 是 toUnicodeEscapes 的逆运算。
//
// 先把整串收集成 **UTF-16 单元序列**再一次性解码 —— 这样相邻的两个 `\uXXXX`
// 如果正好是一个代理对，能拼回原来的字符（逐个 fromCharCode 拼也能，但逐个解码不行）。
func fromUnicodeEscapes(s string) string {
	units := make([]uint16, 0, len(s))
	appendRunes := func(part string) {
		for _, r := range part {
			units = append(units, utf16.Encode([]rune{r})...)
		}
	}

	last := 0
	for _, m := range unicodeEscapeRe.FindAllStringSubmatchIndex(s, -1) {
		appendRunes(s[last:m[0]])
		if v, err := strconv.ParseUint(s[m[2]:m[3]], 16, 32); err == nil {
			units = append(units, uint16(v))
		}
		last = m[1]
	}
	appendRunes(s[last:])
	return string(utf16.Decode(units))
}

// ── 提取 ────────────────────────────────────────────────────────────
//
// ⚠️ 这几个正则必须和前端 `data/actions.js` 里的 `XXX_RE` **逐字对应**：
// 差一个字符就会「预览区捞出来的和定时任务发出去的不是同一批」。
//
// ⚠️ 前端的手机号正则用了 `(?!\d)` 前瞻，而 Go 的 RE2 **不支持前瞻/后顾**，
// 所以那边改成了「先按裸号码找、再手工检查左右边界」（见 extractPhones）。
var (
	// ⚠️ `(?i)` 对应前端的 `gi` 里的 `i`。
	urlRe = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"'，。；：、（）【】《》「」“”]+|\bwww\.[^\s<>"'，。；：、（）【】《》「」“”]+`)
	// ⚠️ 字符类里的 `-` 放在**末尾**就不用转义（`[A-Za-z0-9-]`），别写成 `[A-Za-z0-9\-]` ——
	// 这条正则被「替换」的 email 模式复用，而契约测试要和前端 `EMAIL_RE.source` **逐字比对**，
	// 多一个反斜杠就红了。
	emailRe   = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	ipv4Re    = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)\b`)
	numberRe  = regexp.MustCompile(`-?\d[\d,]*(?:\.\d+)?`)
	cnPhoneRe = regexp.MustCompile(`1[3-9]\d{9}`)
)

// extractJoined 捞一遍、**去重并保持出现顺序**，每行一个。
// 保序很重要：用户要的是「按原文出现顺序的清单」，按长度或字典序排都对不上原文。
func extractJoined(s string, re *regexp.Regexp) string {
	matches := re.FindAllString(s, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return strings.Join(out, "\n")
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// extractPhones 中国大陆手机号：1 开头、第二位 3-9、共 11 位。
//
// ⚠️ 两边的边界**不能省**：不判的话 `13800138000 12` 里的长数字串会被截出前 11 位。
// 前端用 `(?:^|\D)...(?!\d)` 表达，Go 的 RE2 没有前瞻，所以这里先按裸号码找、
// 再逐个检查左右字符 —— 结果等价，而且不会像「消费分隔符」的写法那样漏掉相邻号码。
func extractPhones(s string) string {
	locs := cnPhoneRe.FindAllStringIndex(s, -1)
	seen := make(map[string]bool, len(locs))
	out := make([]string, 0, len(locs))
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if start > 0 && isASCIIDigit(s[start-1]) {
			continue
		}
		if end < len(s) && isASCIIDigit(s[end]) {
			continue
		}
		phone := s[start:end]
		if seen[phone] {
			continue
		}
		seen[phone] = true
		out = append(out, phone)
	}
	return strings.Join(out, "\n")
}

// ── 替换 ────────────────────────────────────────────────────────────

// 「查找」的常用模式。
const (
	replaceModeText   = "text" // 字面（用用户填的 find）
	replaceModeDigits = "digits"
	replaceModeLatin  = "latin"
	replaceModeSpaces = "spaces"
	replaceModeEmail  = "email"
	replaceModeURL    = "url"
	replaceModePhone  = "phone"
	replaceModeIP     = "ip"
)

// replaceModes 模式 → 正则。**这个切片的顺序就是下拉里的顺序，第一项是默认值。**
//
// ⚠️ 为什么不给用户一个正则输入框：前后端各有一份实现，而 JS 的 RegExp 与 Go 的 RE2
// 在前瞻/后顾、替换引用语法（`$1` vs `${1}`）、多行标志上都会分叉 —— 症状是
// 「预览区替换掉了、定时任务里没替换」，正是这个模块一直在防的那类错。
// 固定的正则把两边都锁死，而且**逐字一致**这件事可以被契约测试钉住
// （见 render_actions_test.go 的 TestReplaceModesMatchFrontend）。
//
// ⚠️ 表里的写法必须**两端语法相同**：
//   - `\t` 两边都认；
//   - 别用 `\uXXXX`（Go 的 RE2 不认这个写法，要写 `\x{XXXX}`）—— 全角空格那类直接写字符；
//   - `text` 的正则是空的，表示「不匹配，用用户填的字面文本」。
//
// ⚠️ `[ \t]{2,}` 只压**水平**空白：用 `\s{2,}` 会把换行也算进去，
// 跨行的缩进被一起吃掉，结构就压坏了。
var replaceModes = []struct {
	Key      string
	Source   string
	CaseFold bool // 对应 JS 的 `i` 标志（Go 这边写成 `(?i)` 前缀）
}{
	{Key: replaceModeText, Source: ""},
	{Key: replaceModeDigits, Source: `[0-9]+`},
	{Key: replaceModeLatin, Source: `[A-Za-z]+`},
	{Key: replaceModeSpaces, Source: `[ \t]{2,}`},
	// 这三条**复用提取区那几个正则**（同一个模式不该有两份写法）。
	// ⚠️ 它们的写法必须和前端 `EMAIL_RE.source` 等**逐字一致** —— 契约测试比对的就是这个。
	{Key: replaceModeEmail, Source: emailRe.String()},
	// ⚠️ url 手写而不是复用 urlRe：JS 正则字面量里 `/` 必须写成 `\/`，
	// 而 Go 的写法是 `/` —— 契约测试是逐字比对的，手写一份才能让两边完全一致。
	{Key: replaceModeURL, CaseFold: true,
		Source: `\bhttps?://[^\s<>"'，。；：、（）【】《》「」“”]+|\bwww\.[^\s<>"'，。；：、（）【】《》「」“”]+`},
	{Key: replaceModePhone, Source: cnPhoneRe.String()},
	{Key: replaceModeIP, Source: ipv4Re.String()},
}

// replaceModeByKey 查模式：返回（正则、是否忽略大小写、是否存在）。
func replaceModeByKey(key string) (string, bool, bool) {
	for _, m := range replaceModes {
		if m.Key == key {
			return m.Source, m.CaseFold, true
		}
	}
	return "", false, false
}

// ReplaceModeKeys 模式 key 的稳定顺序，给契约测试和界面用。
func ReplaceModeKeys() []string {
	out := make([]string, 0, len(replaceModes))
	for _, m := range replaceModes {
		out = append(out, m.Key)
	}
	return out
}

// renderReplaceLiteral 按模式替换（「文本」模式下就是字面替换）。
//
// ⚠️ 替换文本一律当**字面**用，不做组引用（`$1` 之类会被原样写进去）：
// 用户填的是「替换成什么文字」，把它当模板会让 `$` 变成危险字符。
// Go 用 ReplaceAllLiteralString、前端用 `replace(re, () => with)`，两边同一条语义。
//
// ⚠️「文本」模式下「查找」为空直接报错，不能当成「在每个字符之间插入」处理：
// 用户只会看到一串乱码，完全不知道发生了什么。前端的 replaceLiteral 是同一条规则。
func renderReplaceLiteral(s string, _ renderContext, params map[string]string) (string, error) {
	mode := strings.TrimSpace(params["mode"])
	if mode == "" {
		mode = replaceModeText
	}
	pattern, caseFold, ok := replaceModeByKey(mode)
	if !ok {
		return "", fmt.Errorf("未知的查找模式 %q", mode)
	}

	with := params["with"]

	if pattern == "" {
		find := params["find"]
		if find == "" {
			return "", fmt.Errorf("「查找」不能为空")
		}
		// ReplaceAll 是**字面**替换，和前端的 split(find).join(with) 等价
		return strings.ReplaceAll(s, find, with), nil
	}

	if caseFold {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		// 模式表是我们自己写死的，编译不过就是代码 bug（契约测试会先一步拦住）
		return "", fmt.Errorf("模式 %s 的正则写错了: %w", mode, err)
	}
	return re.ReplaceAllLiteralString(s, with), nil
}

// ── 全角 / 半角 ─────────────────────────────────────────────────────
//
// ASCII 可见字符（0x21-0x7E）与全角（U+FF01-U+FF5E）相差 0xFEE0。
//
// ⚠️ 空格必须**单独**处理：0x20 + 0xFEE0 = U+FF00 是个**未分配字符**，
// 不是全角空格（全角空格是 U+3000）。踩过这个坑的话表现是「转换后空格变成方块/问号」。
func toFullWidth(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '!' && r <= '~':
			b.WriteRune(r + 0xfee0)
		case r == ' ':
			b.WriteRune('\u3000')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func toHalfWidth(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '\uff01' && r <= '\uff5e':
			b.WriteRune(r - 0xfee0)
		case r == '\u3000':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── 中文标点 ────────────────────────────────────────────────────────
//
// 逐字符查表。（`——` 这类双字符标点会被拆成两个 `-`，可接受 —— 为它做长串匹配不值得。）
var cnPunctMap = map[rune]rune{
	'，': ',', '。': '.', '、': ',', '；': ';', '：': ':',
	'？': '?', '！': '!', '（': '(', '）': ')', '【': '[', '】': ']',
	'《': '<', '》': '>', '「': '"', '」': '"', '『': '\'', '』': '\'',
	'“': '"', '”': '"', '‘': '\'', '’': '\'', '～': '~',
	'…': '.', '—': '-', '－': '-', '　': ' ',
}

func cnPunctuationToEn(s string) string {
	var b strings.Builder
	for _, r := range s {
		if to, ok := cnPunctMap[r]; ok {
			b.WriteRune(to)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ── 数字 → 中文大写 ─────────────────────────────────────────────────
//
// ⚠️ 必须**分节**处理（4 位一节 + 万/亿），不能逐位查表：中文的单位是**组合**的
// （十万 = 十 + 万），逐位法根本表达不出来。
// 节内的「零」也要合并：`1001` → 一千零一（不是一千零零一），`10000` → 一万（不是一万零）。
var (
	cnDigits      = []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	cnSmallUnits  = []string{"", "十", "百", "千"}
	cnBigUnits    = []string{"", "万", "亿", "万亿"}
	cnNumberRe    = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	cnTrailZeroRe = regexp.MustCompile(`零+$`)
)

func cnSectionToText(section string) string {
	var b strings.Builder
	zeroPending := false
	for i := 0; i < len(section); i++ {
		digit := int(section[i] - '0')
		unit := cnSmallUnits[len(section)-1-i]
		if digit == 0 {
			zeroPending = true
			continue
		}
		if zeroPending && b.Len() > 0 {
			b.WriteString(cnDigits[0])
		}
		zeroPending = false
		b.WriteString(cnDigits[digit])
		b.WriteString(unit)
	}
	return b.String()
}

func numberToChinese(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !cnNumberRe.MatchString(s) {
		return "", fmt.Errorf("不是合法数字")
	}

	negative := strings.HasPrefix(s, "-")
	body := strings.TrimPrefix(s, "-")
	intPart, decPart, _ := strings.Cut(body, ".")

	intText := cnDigits[0]
	trimmed := strings.TrimLeft(intPart, "0")
	if trimmed != "" {
		// 从右往左每 4 位切一节，chunks[0] 是**最高**节
		var chunks []string
		for i := len(trimmed); i > 0; i -= 4 {
			start := i - 4
			if start < 0 {
				start = 0
			}
			chunks = append([]string{trimmed[start:i]}, chunks...)
		}

		var b strings.Builder
		for idx, chunk := range chunks {
			bigUnit := cnBigUnits[len(chunks)-1-idx]
			sectionText := cnSectionToText(chunk)
			if sectionText == "" {
				// 整节是 0：只有前面已经有内容才补零（`10000` 不该变成「一万零」）
				if b.Len() > 0 && !strings.HasSuffix(b.String(), cnDigits[0]) {
					b.WriteString(cnDigits[0])
				}
				continue
			}
			// 这节有内容、但首位是 0（数值不满千）且前面已有输出 → 中间要补零
			// （`10000001` → 一千万**零**一）
			if b.Len() > 0 && chunk[0] == '0' && !strings.HasSuffix(b.String(), cnDigits[0]) {
				b.WriteString(cnDigits[0])
			}
			b.WriteString(sectionText)
			b.WriteString(bigUnit)
		}
		intText = cnTrailZeroRe.ReplaceAllString(b.String(), "")
		// 中文习惯说「十」「十二」，不说「一十」「一十二」；但「一百一十」要保留
		// （只换开头的那个，所以不能用 ReplaceAll）
		if strings.HasPrefix(intText, "一十") {
			intText = "十" + intText[len("一十"):]
		}
	}

	out := intText
	if decPart != "" {
		var b strings.Builder
		b.WriteString("点")
		for i := 0; i < len(decPart); i++ {
			b.WriteString(cnDigits[int(decPart[i]-'0')])
		}
		out += b.String()
	}
	if negative {
		out = "负" + out
	}
	return out, nil
}

// ── 时间戳 ⇄ 日期 ───────────────────────────────────────────────────

var unixTimestampRe = regexp.MustCompile(`^\d{9,13}$`)

// timestampToDateText 把 Unix 时间戳（秒或毫秒）渲染成日期时间。
//
// ⚠️ 用**任务时区**渲染（前端用的是浏览器本地时区）。定时任务的正文必须跟任务自己的
// 时区走 —— 否则「一个按 Asia/Shanghai 排的任务跑在 UTC 容器里」会发出差 8 小时的时间，
// 而那只有真到了那一天才看得出来。
func timestampToDateText(raw string, ctx renderContext) (string, error) {
	s := strings.TrimSpace(raw)
	if !unixTimestampRe.MatchString(s) {
		return "", fmt.Errorf("不是 10 位（秒）或 13 位（毫秒）时间戳")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return "", fmt.Errorf("时间戳超出可表示范围")
	}
	if len(s) < 13 {
		n *= 1000
	}
	return time.UnixMilli(n).In(ctx.Now.Location()).Format("2006-01-02 15:04:05"), nil
}

// dateTextToTimestamp 日期字符串 → Unix 时间戳（秒）。
//
// ⚠️ 必须按 **UTC** 解析，和前端 `Date.parse('2026-09-23')` 保持一致 ——
// JS 对「不带时间的 ISO 日期」按 UTC 解释（ES5 起的规定，`Date.parse` 在
// `2026-09-23` 与 `2026-09-23T00:00` 上都走这条路）。按本地解析会整体差一个时区，
// 东八区就是 8 小时，日期还可能跟着错一天。
func dateTextToTimestamp(raw string) (string, error) {
	// 前端把 `2026/09/23` 的斜杠归一成 `-`（Safari 对斜杠的解析行为不一致）
	s := strings.TrimSpace(strings.ReplaceAll(raw, "/", "-"))
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return strconv.FormatInt(t.Unix(), 10), nil
		}
	}
	return "", fmt.Errorf("认不出这个日期")
}
