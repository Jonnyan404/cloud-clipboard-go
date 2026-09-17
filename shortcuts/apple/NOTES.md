# 工程笔记

> README 讲**怎么用**，这份讲**为什么**——判型的演进、每个结论的实测数据、踩过的坑、
> 以及被推翻过的判断。**留档的目的是别重走。**

## 目录

- [判型：怎么决定「当文本还是当文件」](#判型怎么决定当文本还是当文件)
- [实测数据](#实测数据)
- [订正记录（被推翻的结论）](#订正记录被推翻的结论)
- [cherri 的坑](#cherri-的坑)
- [Shortcuts 运行时的坑](#shortcuts-运行时的坑)
- [构建与产物比对](#构建与产物比对)
- [测试手法](#测试手法)
- [平台入口](#平台入口)
- [Cloudflare Worker 侧的实现](#cloudflare-worker-侧的实现)
- [验收矩阵历史](#验收矩阵历史)
- [保存位置：为什么只能每次询问](#保存位置为什么只能每次询问)

---

## ⚠️ 2026-09-17 定稿：判型交给用户，`/upload/raw` 已删除

**最终方案**：发送拆成两个捷径 —— 「发送文本」和「发送文件」，**用户自己选发什么**。
客户端不做任何类型判断，服务端也不再需要「按内容猜类型」。

```
POST /text     发文本（快捷指令：getText → 从多信息文本获取 Markdown → POST）
POST /upload   发文件（multipart，part 自带真实文件名）
```

**为什么判型必须交给用户**：剪贴板条目的名字反映的是**剪贴板格式**而非内容 ——
纯文本是 `Clipboard <日期>`、带 HTML 味是 `Clipboard <日期>.html`、图片是 `Clipboard <日期>.png`。
所以"扩展名含字母 = 文件"会把 HTML 味的文本误判成文件（实测踩过）。
而客户端又看不到内容，判不了。**用户本来就知道自己要发什么。**

**删掉的东西**（都只为「服务端替客户端判型」而存在）：
`/upload/raw` 端点、`sniffPayload`、`normalizeText`、`decodeUnicodeText`、
`htmlDocumentToPlainText`、`storeRawText`，以及 Worker 侧的 `sniff.js` 和那 25 项对齐测试。

**验收（四项全绿）**：

| 用例 | 结果 |
| :--- | :--- |
| 剪贴板纯文本（真实 ⌘C 复制） | 内容正确 |
| 剪贴板富文本 | 纯文本无标签 |
| 剪贴板图片 | 图片 |
| 分享 `.md` | 文件、名字保真 |

> ⚠️ **测试陷阱**：`osascript` 只能造**单一 flavor** 的剪贴板，与真实 ⌘C 的多 flavor 不同。
> 用它测剪贴板路径会得到假象 —— 我因此误判过"纯文本发出去是空的"。

---

## 判型：怎么决定「当文本还是当文件」

### 为什么需要判型

客户端要决定三件事：发**文本消息**、发**文件**、还是发**图片**。而 Shortcuts 给出的信号并不直接。

### 走过的三条路

**第一条（原版）**：用 `typeOf` 比较 29 个本地化类型名。
→ 致命缺陷：`typeOf` 返回**本地化字符串**（`文件`/`图像`/`文本`），非中英文系统上**静默失效**。已废弃。

**第二条**：客户端不判，全交给服务端按内容魔数嗅探。
（**剪贴板路径至今仍是这一条**——不是因为懒，是因为客户端读不到剪贴板的文字，
见文末「为什么剪贴板路径必须留在服务端」。）
→ 简单、可测（Go 侧 25 个测试无需 Apple 设备），但代价是 **`.txt`/`.md` 这类文本文件会被存成文本消息**，
文件名和可下载性一起丢失。

**第三条（现行）**：**先看扩展名，再用名字启发式兜底**。

```
扩展名 ≠ txt        → 文件        （.md/.sh/.png… 全走这条，与系统语言无关）
扩展名 = txt        → 再看名字是不是内容开头
                        是   → 文本（分享字符串会被物化成 <首行>.txt）
                        不是 → 文件（真的 .txt 文件）
名字以 Clipboard 开头 → 文本（系统为无名字符串生成的名字）
取不到扩展名（空）   → 落回名字启发式，不会更糟
```

**为什么需要扩展名这一层**：纯名字启发式（名字 = 内容首行 → 文本）**会被 markdown 骗过**——
`getText` 把 `# 测试笔记` 渲染成 `测试笔记`，于是「内容以开头名字」成立，`.md` 被误判成文本。

### 扩展名是怎么拿到的

动作：`is.workflow.actions.properties.files`，参数 `WFContentItemPropertyName = "File Extension"`
（捷径 App 里叫「获取文件详情 → 扩展名」）。

三个注意点：

1. **cherri 的动作表里没有它**，只能用 `rawAction("is.workflow.actions.properties.files", {...})` 调。
2. **它没有输入参数**，吃「上一个动作的输出」。但实测**紧跟条目之后仍然拿不到值（恒为空）**，
   **必须显式给 `WFInput`**。→ 由 `source/patch_shortcut.py` 在构建后补上。
3. **`rawAction` 的标识符覆盖不稳定**：cherri 源码 `action.go` 里确实会用第一个参数覆盖
   `overrideIdentifier`，但实测同一个写法有时生效、有时不生效，产物里会留下占位符
   `is.workflow.actions.rawaction`。→ 同一个 patcher 按参数特征（含 `WFContentItemPropertyName`）还原。

### 误判为什么是安全的

图片 / 二进制即使被判成文本，服务端仍会按魔数嗅探重新分流。所以判错最多退化成"文本消息"，
不会产生坏数据。

---

## 实测数据

### 各输入的信号（2026-09-17，`CC-Diag-Ext3`）

| 输入 | `typeOf` | `getName` | `getText` | `EXT` |
| :--- | :--- | :--- | :--- | :--- |
| 剪贴板纯文本 | `文本` | `Clipboard 2026年9月17日 09.47` | 成功（返回值未观测到） | `28`（日期的小数部分，**纯数字**） |
| 剪贴板富文本 | `多信息文本` | — | 成功且正确剥成纯文本 | — |
| **剪贴板图片** | `图像` | `Clipboard <日期>` | — | **`png`** |
| 分享 **`.md` 文件** | `文件` | `测试笔记` | 渲染后的纯文本 | **`md`** |
| 分享 **`.sh` 文件** | `文本` | `build` | 文件内容 | **`sh`** |
| 分享 **`.txt` 文件** | `文本` | `notes` | 文件内容 | **`txt`** |
| 分享 **`.png`** | `图像` | `test` | 只返回名字，无意义 | **`png`** |
| 分享**字符串**（多行/单行/含点） | `文本` | 内容首行 | 文本本身 | **恒为 `txt`** |

**读法**：`typeOf` 不可靠（`.sh`/`.txt` 它说是"文本"，而 `.md` 又说是"文件"，且本地化）；
**`EXT` 才是可靠信号** —— 真文件给真扩展名，分享字符串恒为 `txt`，剪贴板文本是纯数字。

### 早期数据（`CC-Diag-Share`，2026-09-17）

| 来源 | `typeOf` | `getName` |
| :--- | :--- | :--- |
| 剪贴板（无输入） | `文本` | `Clipboard 2026年9月17日 09.47` |
| 分享 `.txt` 文件 | `文本` | `requirements` ← 真实文件名 |
| 分享 `.html` 文件 | `URL` | `Cloud Clipboard · OpenWrt LuCI 界面重设计预览` ← 真实文件名 |
| 分享文本（字符串） | `文本` | `django==4.2.*` ← **内容首行** |

→ 结论：**分享文件确实保留真实文件名**；字符串的名字取自内容本身。

### 为什么「让客户端区分文件和文本」一度被认为做不到

两条当时的证据（**后来被第三条路推翻了**，但推理过程值得留档）：

**证据一：`typeOf` 无法区分。** 剪贴板纯文本与 `.txt` 文件的 `typeOf` **都是 `文本`**——
Shortcuts 会把 `.txt` 文件强制转成字符串，类型层面无从分辨。

**证据二：当时认为没有「是某种类型」这种条件。** 三份来源一致：

- cherri 源码：条件只把比较值路由到 `WFDate` / `WFNumberValue` / `WFConditionalActionString`
- 逆向文档 [drewburchfield/shortcuts-toolkit](https://github.com/drewburchfield/shortcuts-toolkit)：
  条件仅 `Equals` / `Contains` / `Begins With` / `Ends With` / `Is Greater Than` / `Is Less Than` /
  `Is Between` / `Has Any Value` / `Does Not Have Any Value`
- 本机 14 个在用捷径：无一个使用内容类条件

**教训**：这三处都没有「取文件扩展名」这种**动作**（不是条件）——
我在错误的方向上找，于是得出"客户端做不到"的结论。
**后来在捷径 App 里手工建一个样本、读它的 plist，一分钟就找到了。**
→ **遇到"某能力不存在"的判断，优先考虑"从 UI 里做个样本读 plist"。**

---

## 订正记录（被推翻的结论）

留档以免重走。这些都是**当时看起来证据充分**的判断：

| 曾经的结论 | 实际情况 |
| :--- | :--- |
| 「`getText` 对剪贴板文本抛异常」 | **部分成立**。分享路径上它不抛（固定标记 4/4 到达）；但**剪贴板路径上确实会抛** `无法将文本转换为NSString`，且对**中文返回乱码**。准确说法：**`getText` 在剪贴板路径上不可靠** |
| 「`getText` 从未抛异常」（我后来推翻上一条时下的结论） | **同样太绝对**。见上一条——两个方向都被实测推翻过，最终结论是"剪贴板路径不可靠" |
| 「`getText` 不可靠是 `pbcopy` 写坏剪贴板的假象」 | **该推测被证伪**：剪贴板用 AppleScript 正确写入后依然复现 |
| 「`WFFileDestinationPath` 是绝对路径」 | **错**。实测是**文件提供者命名空间**内的路径，`/Users/…` 用不了 |
| 「`show()` 是无害提示」 | **错**。它会在屏幕中央弹结果卡片打断视线，已改为通知 |
| 「签名前必须重启 Shortcuts.app」 | **无依据**。不重启直接签名完全正常；重启反而会扰动已安装的捷径 |
| 「Shortcuts 对含非 ASCII 的文本条目发不出字节」 | **错**，是 `pbcopy` 写坏剪贴板的假象（见[测试手法](#测试手法)） |
| 「客户端没法区分文件与文本」 | **错**。扩展名动作存在，只是 cherri 的动作表里没有 |
| 「客户端读不到剪贴板文字」 | **错**。用三动作捷径在界面上实测：中文/英文/富文本**都返回正常纯文本**。我的诊断失败在 text 包装与上报环节——**把工具的故障算成了被测动作的故障** |
| 「剪贴板路径也能本地分流」 | **错（做不到）**。分类 ✅、读取 ✅ 都没问题，卡在**编码**（客户端读到的文本是 UTF-16 字节）——见下节。**但后来 Jonny 提出「判型交给用户、拆成两个捷径」，绕开了整个问题**，见本文开头「定稿」一节 |

> **通用教训**：用"分步上报"定位失败时，**必须检查错误信息本身**——
> 否则会把"上报失败"误读成"被测动作失败"。
> 同理：**同一个动作在不同输入路径上的行为可能完全不同**，别把一条路径的结论推广到另一条。

### （历史）剪贴板路径为什么一度被认为必须留在服务端：卡在「编码」

> ⚠️ **这一节是当时的过程记录，结论已被后面的「判型交给用户」方案取代。**
> 保留它是为了说明"为什么当时走不通"，不是当前设计。

问题：剪贴板路径能不能也本地分流，从而删掉 `/upload/raw`？

**分类可以** —— 扩展名就能分开，且与系统语言无关：

| 剪贴板内容 | `EXT` |
| :--- | :--- |
| 纯文本 | `49` / `50` …（名字里日期的小数部分，**纯数字**） |
| 图片 | **`png`** |

**读取也可以** —— 2026-09-17 用「获取剪贴板 → 从输入获取文本 → 显示结果」三动作捷径**在界面上直接验证**：
中文、英文、富文本**都返回正常纯文本**（富文本的标签已被剥掉）。

> ⚠️ **我之前判断"客户端读不到剪贴板文字"是错的。**
> 我的诊断捷径里失败的是 `getText` **外面那层 `text()` 包装**（报错原文是「**文本**失败…无法将文本转换为 NSString」）
> 以及上报环节 —— 我把工具的问题算到了被测动作头上。
> **教训：分步上报这种手法，会把"测量工具自身的故障"伪装成"被测对象故障"。**
> 要验证某个动作的能力，**优先用不经过上报的手段**（在捷径里直接「显示结果」看一眼）。

**卡在"发"** —— 而且根因是**编码**，不是读取。

2026-09-17 实测（诊断捷径分两步上报 + curl 对照）：

```
id=139  TEXTDIAG-AFTER-GETTEXT                 ← 标记到达：getText 没抛异常
id=140  -N�ejR4�\x7fg�Q�[A\x00B\x00C\x00        ← 文本到达，但乱码
```

**注意末尾的 `A\x00B\x00C\x00`** —— 那是 **UTF-16LE** 的铁证（字符后跟一个 NUL）。

用 curl 把**同一份 UTF-16LE 字节**分别发给两个端点，机制就清楚了：

| 端点 | 结果 |
| :--- | :--- |
| `/text`（只按 UTF-8 解析） | `-N�eKmՋA\x00B\x00C\x00` ❌ 乱码 |
| **`/upload/raw`**（`normalizeText` 解码 UTF-16/32） | **`中文测试ABC`** ✅ 正确 |

→ **客户端从剪贴板读到的文本是 UTF-16 字节**；Shortcuts 在界面里能正常显示它，
但落成 HTTP 请求体就是 UTF-16。客户端**没有"转成 UTF-8"的动作**。

**当时的结论是"`/upload/raw` 必须保留"**（承担编码转换）。
**这个结论后来被推翻了**：Jonny 指出可以过一道文本类动作在客户端完成编码规范化，
再把判型交给用户 —— 于是 `/upload/raw` 整个删掉了。

> 这也解释了为什么当前设计一直很稳：客户端发**原始条目**，服务端 `normalizeText` 解码。
> 无论发条目还是发 `getText` 的结果，字节都是 UTF-16，都需要服务端解码。

---

## cherri 的坑

版本：**v2.3.0**，实际代码来自 commit `dc82114f346f`（`cherri --version` 只打印版本常量）。
**它是 Go 写的，不是 Swift**——网上流传的「Swift 编写、`swift build`」说法是错的，照做会失败。
重建：`go install github.com/electrikmilk/cherri@dc82114f346f`

### 条件里不能内联函数调用

```cherri
if count(@source) != 1 { ... }      // ✗ panic: interface conversion: interface {} is main.action, not main.varValue
@amount = count(@source)
if @amount != 1 { ... }             // ✓
```

**推论**：源码里那些看似冗余的变量赋值，有一部分是绕这个坑必需的。

### 带括号的条件分组会死循环挂起

`if a && (b || c)` 会挂起。改写为嵌套 if 或 `@flag` 变量。

### `getName` / `replaceText` 没有返回类型标注

直接参与 `!=` 比较会报 `Invalid type '' for conditional '!='`。绕法是用字符串字面量包一层：

```cherri
@a = getName(@item)
@aw = "{@a}"          // 经 gettext 动作，推断为 text
if @aw != @bw { ... }
```

**真值判断（`if @x`）不受影响**，只有 `==` / `!=` 这种带类型的比较才需要。

### 自定义动作的 `variable` 参数不能传字面量

```cherri
myAction("报告内容 {@value}")   // ✗ Invalid value "..." (text) for argument 'body' (variable)
@step = "报告内容 {@value}"
myAction(@step)                 // ✓
```

### `dictionary!` 参数只收内联字面量

`formRequest` / `jsonRequest` 的 `body` 和 `headers` 都是 `dictionary!`（`!` = 只收字面量），
传变量会报 `Shortcuts does not allow variable values for this argument`。

### 输出文件名由 `#define name` 决定

`-o` 参数不生效，且会静默覆盖同名产物。构建脚本按 `#define name` 推导产物名。

### 不输出 `ActionIndex`

`shortcutgen.go:1194` 读取 `q.actionIndex`，但全代码没有任何赋值点，恒为 0 并被 `omitempty` 省略
→ 导入时的配置面板接不上。**因此 `inject_import_questions.py` 是长期必需，不是临时补丁。**

### `rawAction` 的标识符覆盖不稳定

见上文[扩展名是怎么拿到的](#扩展名是怎么拿到的)。

---

## Shortcuts 运行时的坑

- **`typeOf` 会闪变**：同一个 icns 文件多次运行会在 文件/图像/文本 之间飘，**不能作为唯一判据**。
- **`getText` / `getName` 在剪贴板路径上不可靠**：`getText(剪贴板文本)` 的返回值未观测到
  （其结果上报总是失败），`getName` 同样不稳。
- **含非 ASCII 的剪贴板条目必须用 AppleScript 写入**，不能用 `pbcopy`（见[测试手法](#测试手法)）。
- **剪贴板注入后必须 `sleep 1` 再运行捷径**，否则读到旧值。
- **`shortcuts run` 是后台化的**，立即返回；上传是否成功要轮询 `/content/latest` 确认。
- **新导入的捷径首次 CLI 运行前需在界面点一次信任**，否则报
  `WFBackgroundShortcutRunnerErrorDomain 错误 1`。
- **`/content/latest` 只返回最新一条**，逐条验收时必须在每个用例前重新上传。
- **重启 Shortcuts.app 会扰动已安装的捷径**（观察到被重命名、动作数 +1）。

### 平台的反馈差异

`vibrate()` 是 **iOS 专属**动作，所以源码里一直包在 `if @model != "Mac"` 里。
**但此前没人给 macOS 补等价反馈**，导致文本与图片两条分支在 Mac 上静默完成——拉取成功与否完全看不出来。
2026-09-17 补上通知（见 README 的「怎么用」）。

> **教训**：平台专属动作被条件守卫时，一定要问一句"另一个平台的等价反馈是什么"。
> `if @model != "Mac" { vibrate() }` 只处理了"iOS 有"，没处理"macOS 没有"——**静默失败比报错更难查**。

---

## 构建与产物比对

- **签名直接执行即可，不需要先重启 Shortcuts.app**。签名时打印的若干
  `Unrecognized attribute string flag` 属正常噪音。
- **不要用文件大小比对产物**：patcher 用 `plistlib.dump` 重写后体积会变（XML 序列化差异）。
  应比对**动作数 + 动作类型分布 + ActionIndex** —— `verify.py` 就是干这个的。

### 两个后处理脚本为什么长期必需

| 脚本 | 补什么 | 为什么 cherri 做不到 |
| :--- | :--- | :--- |
| `inject_import_questions.py` | `WFWorkflowImportQuestions.ActionIndex` | cherri 恒输出 0 并被省略 |
| `patch_shortcut.py` | 扩展名动作的显式 `WFInput`；表单里的「文件」字段 | `dictionary!` 只收字面量，字典只能生成 text token |

`patch_shortcut.py` 补的表单文件字段结构（取自手工样本）：

```json
"WFItemType": 5,                       // 5 = 文件（文本字段没有这个键）
"WFValue": {
  "Value": {"Value": {"Type": "Variable", "VariableName": "item"},
            "WFSerializationType": "WFTextTokenAttachment"},
  "WFSerializationType": "WFTokenAttachmentParameterState"    // 不是 WFTextTokenString
}
```

漏掉 `WFItemType: 5` → 字段被当成文本 → 请求体不是合法 multipart → 服务端报「无法解析表单数据」。

---

## 测试手法

### 三条输入通道（覆盖捷径里不同分支）

| 想模拟 | 命令 |
| :--- | :--- |
| **剪贴板**（无输入） | `osascript -e 'set the clipboard to "…"'` 然后 `shortcuts run "Name"` |
| **文件输入**（分享文件） | `shortcuts run "Name" -i /path/to/file` |
| **文本输入**（分享字符串） | `open "shortcuts://run-shortcut?name=Name&input=text&text=…"` |

第三种用 **URL scheme**，是唯一能从命令行喂**字符串**的办法（`-i` 只能传文件）。
注意：**不要加 `-g`**（后台打开会被忽略），且**比预期慢**（实测 5 秒不够，约 6–10 秒）。

**三种输入会走不同分支，必须分别测。**

### ⚠️ 不要用 `pbcopy` 写非 ASCII 内容

本机 shell 的 `LANG`/`LC_ALL` 为空、`LC_CTYPE=C`。此环境下：

```
$ printf '纯文本 你好' | pbcopy; osascript -e 'clipboard info'
«class utf8», 0, «class ut16», 2, string, 0, Unicode text, 0       ← 长度全是 0
$ osascript -e 'set the clipboard to "纯文本 你好"'; osascript -e 'clipboard info'
Unicode text, 12, string, 11, «class utf8», 16, «class ut16», 14   ← 正常
```

拿坏掉的剪贴板跑捷径会看到「上传 0 字节」，**极易误判为「Shortcuts 不支持中文」**。
**正确做法：`osascript -e 'set the clipboard to "…"'`。**

### 诊断捷径：分步上报

Shortcuts 没有 try/catch，所以让捷径把中间结果逐步 POST 到服务端：

```
上报 "S1-TYPE=[{@typeOf结果}]"
执行被测动作
上报 "S2-NAME=[{@getName结果}]"
```

**关键**：被测动作若抛异常，后面的上报不会到达——但**「上报失败」和「动作失败」是两件事**，
必须看错误信息本身（这个坑踩过，见[订正记录](#订正记录被推翻的结论)）。

### 判断"是否弹出对话框"

**不能靠观察，更不能靠"文件出现了所以没弹"——后者会被用户的点击污染。**
可靠做法：**测量阻塞**（毫秒级轮询副作用 + 确认界面状态）或**直接检查产物参数**。

---

## 平台入口

一份源码可同时服务 iOS 与 macOS，靠 `#define from` 声明入口：

| define 值 | 生成的 `WFWorkflowTypes` | 出现位置 |
| :--- | :--- | :--- |
| `sharesheet` | `ActionExtension` | iOS 分享菜单 |
| `quickactions` | `QuickActions` | macOS Finder 快速操作 / 服务菜单 |
| `menubar` | `MenuBar` | macOS 菜单栏 |

逗号可叠加：`#define from sharesheet,quickactions,menubar`。
合法取值共 9 个：`onscreen` `search` `spotlight` `menubar` `sharesheet` `sleepmode` `watch` `quickactions` `notifications`。

**平台专属动作必须用 `@model` 守卫**：`vibrate()` 仅 iOS/iPadOS，`reveal()`（Reveal in Finder）仅 macOS。

`verify.py` 会提示 `WFWorkflowTypes` 是否漏了 macOS 入口。

---

## Cloudflare Worker 侧的实现

两个上传端点的分工：

- **`/upload/raw`** —— ~~给剪贴板路径用~~ **已于 2026-09-17 删除**。它承担的三件事
  （按内容判型、UTF-16 解码、HTML 降级）分别被"判型交给用户"和"客户端过一道文本动作"取代。
- **`/upload`** —— 给**分享路径的文件**用。multipart 的 part 自带真实文件名（含扩展名），
  服务端直接用它——**扩展名不需要猜**。

Worker 侧代码：

- `src/sniff.js` —— 内容嗅探与文本规范化，**与 Go 版逐条对齐**；刻意零依赖，可被 Node 直接单测
- `src/handlers/raw-upload.js` —— 只做「嗅探 + 请求包装」，存储逻辑**委托**给既有的
  `TextHandler.create` / `FileHandler.upload`，不复制 D1/R2 写入、清理与广播
- 测试：`cd cloudflare/workers && npm test` —— 25 项单测（用例与 Go 的 `unicode_text_test.go` 逐条对应）
  + 30 项端到端 + 19 项 Receive 链路，用 `node:sqlite` 充当 D1、Map 充当 R2，**不需要 wrangler、不联网**

> 端到端测试跑的是处理器的真实调用路径，但**没有经过 wrangler 的完整运行时**
> （本地 wrangler 因网络限制未能启动）。要验证线上行为，需实际部署后请求一次。

**已知局限（Go 版同样存在，为保持一致未改动）**：含 NUL 的随机字节若恰好能解成
"全是可打印字符"的 UTF-16，会被判成文本——`validUnicodeRunes` 只挡 `< 0x20` 的控制字符。

---

## 验收矩阵历史

### 现行版（2026-09-17，9/9 全绿）

| # | 用例 | 结果 |
| :--- | :--- | :--- |
| 1 | 剪贴板 ASCII / 中文 | `type=text` ✅ |
| 2 | 剪贴板富文本 | `type=text`，`标题 Hello & 你好 第二段`（标签已剥、实体已解码）✅ |
| 3 | 分享字符串（多行 / 单行 / 含点） | `type=text` ✅ |
| 4 | **`-i .md` 文件** | **`type=file` `测试笔记.md`** ✅ |
| 5 | `-i .sh` 文件 | `type=file` `build.sh` ✅ |
| 6 | `-i .txt` 文件 | `type=file` `notes.txt` ✅ |
| 7 | `-i .png` | `type=image` **`test.png`** ✅ |

### 更早的验收（2026-09-16，服务端嗅探版）

| # | 场景 | 期望 | 原版（已退役） | 当时的现行版 |
| :--- | :--- | :--- | :--- | :--- |
| a | HTML / 富文本剪贴板 | 纯文本无标签 | ✅ | ✅ |
| b | 纯文本剪贴板（含中文） | 文本非空 | ✅ | ✅ |
| c | 链接剪贴板 | URL 原文 | ✅ | ✅ |
| d | 右键选中文本（`.txt`） | 文本消息 | ❌ 存成文件 `clipboard.txt` | ✅ 文本 |
| e | `.icns` 文件 | 文件 | ✅ | ✅ |
| f | 二进制文件 | 文件 | ✅ | ✅ |

**原版 5/6，当时的现行版 6/6。**
> 原版已退役并从仓库删除（源码与签名产物均已移除，需要时可在 git 历史里找回）。
> 它的 29 项本地化类型名比较、4 路分支与 `?as=file` 协议都不再维护。
> **注意**：原版曾占用 `Cloud-Clipboard-Send` 这个名字，该名字现已由现行的重写版接手，
> 所以文档与提交历史里同名指的不是同一个东西。

### Receive 验收（2026-09-16 首次）

| # | 服务端内容 | 期望 | 实测 |
| :--- | :--- | :--- | :--- |
| R1 | 纯文本 | 进剪贴板 | ✅ |
| R2 | 富文本 HTML（服务端已剥成纯文本） | 进剪贴板、无标签 | ✅ `标题 Hello & 你好` |
| R3 | PNG 图片 | 进剪贴板（图像） | ✅ 剪贴板出现 `PNGf` / TIFF / JPEG / BMP 等类型 |
| R4 | 二进制文件 | 下载 + 保存 | ✅ 落盘 `~/Downloads/clipboard.bin`，SHA-256 与源文件一致 |

**R1–R3 无需人工干预；R4 会弹出保存对话框，需要点一次。**

### 发送结果用通知，不用结果卡片

原先用 `show()`，每次发送都会在屏幕中央弹一张卡片打断视线。现改为：

```cherri
showNotification("已发送 {@savedID}", "Cloud Clipboard", false)
```

cherri 的动作名是 **`showNotification(body, title, playSound, attachment)`**，不是 `notification`。
`playSound` 传 `false`，连续发送时不会反复响铃。

> 自查记录：这一步最初只能验到"流程没被破坏"，通知是否真显示无法自证——
> 系统日志被沙箱挡住（`log: Cannot run while sandboxed`），通知数据库需要全盘访问权限。
> 最后是人工目视确认的。**无法自证时如实标注为未验证，比含糊过去强。**

---

## 保存位置：为什么只能每次询问

**「静默保存到指定目录」做不到**，所以 Receive 只保留「询问保存位置」一种行为。

实测经过（2026-09-16）：

1. cherri 里确实有**两个**保存动作，`saveFile` 自带 `WFAskWhereToSave: false`，理论上可静默直存：
   ```
   action default 'documentpicker.save' saveFilePrompt(file, ?overwrite)      ← 弹对话框
   action 'documentpicker.save' saveFile(file: 'WFFileDestinationPath', …) {
       "WFAskWhereToSave": false                                              ← 不弹，直存
   }
   ```
   于是加了第 4 个导入问答 `qSaveDir`（填目录→静默直存，留空→询问）。
2. 在捷径 App 里关掉「询问保存位置」并**点选文件夹**后实测：**文件落到了 iCloud 云盘，而不是所选的真实路径。**

**根因**：Shortcuts 的文件夹选择器作用域限于**文件提供者命名空间**（iCloud 云盘 / 我的 Mac），
`WFFileDestinationPath` 是**该命名空间内的路径**，不是真实文件系统路径。

→ 已撤回 `qSaveDir` 与静默保存分支（Receive 由 126 动作回到 113，问答由 4 个回到 3 个）。
**「询问保存位置」本身就是原生保存对话框，也就是浏览选择器**——只是每次都要确认一次。

> **教训**：涉及平台语义的参数别靠推断。`WFFileDestinationPath` 的路径语义我最初是靠猜的
> （还写进过文档），直到真机点选才发现是提供者命名空间。
