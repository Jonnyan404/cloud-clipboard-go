# cloud-clipboard-go 交接清单

> 生成时间: 2026-09-16  |  交接背景: iOS 快捷指令版两个用户问题未最终验收，原执行人终止，交予接续者。

---

## 勘误与更新（2026-09-16 晚，接续者补记）

原文档整体质量高，§5「踩坑纪要」尤其有价值。以下 3 处经实测确认不准确，**请以本节为准**：

1. **§4.1 称 cherri「Swift 编写，`swift build` 后 link」——错误。** `go version -m /usr/local/bin/cherri` 显示它是 Go 模块：`github.com/electrikmilk/cherri v0.0.0-20260827234458-dc82114f346f`，go1.26.5 编译。照原文做会失败。正确重建：`go install github.com/electrikmilk/cherri@dc82114f346f`。
2. **§6 称 `config.json`「现在为空/被删，需按真实配置重建」——不实。** `cloud-clip/config.json` 完好存在（含 `server.auth` 与 `test` 房间密码），无需重建。
3. **§0 把 `.gitignore` / `lib/main.go` / `lib/utils.go` 标为「会话前既有，未深究」——不准确。** `main.go` 那 +2 行正是 `/upload/raw` 与 `/upload/base64` 的路由注册；`utils.go` 的 -2 行是 `DetermineResponseType` 里删掉 `text/*` 分支。二者与 `handler.go` 属同一套改动，**review 时不能单独回退**，否则新端点直接 404。

### 验收已执行（§6 矩阵）

服务端用隔离配置（去 auth、storageDir/historyFile 指向 /tmp、host 收窄 127.0.0.1），未污染仓库数据。结果：

| # | 场景 | 期望 | 原版 PK170 | 简化版 Min |
|---|---|---|---|---|
| a | 富文本/HTML 剪贴板 | 纯文本无标签 | ✅ `标题 Hello & 你好` | ✅ |
| b | 纯文本剪贴板（含中文） | 文本非空 | ✅ | ✅ |
| c | 链接剪贴板 | URL 原文 | ✅ | ✅ |
| d | `-i rightclick.txt` | 文本消息 | ❌ 存成文件 `clipboard.txt`（**稳定 4/4**） | ✅ 文本 |
| e | `-i GenericDocumentIcon.icns` | 文件 | ✅ | ✅ |
| f | `-i u.bin` | 文件 | ✅ | ✅ |

→ 用户问题 #2（HTML 泄漏）已修复；**#1（右键文本被当文件）在原版未修复，简化版已修复**。原版 5/6，简化版 6/6。

### ⚠️ 排查 b 场景前必读：`pbcopy` 写非 ASCII 会损坏剪贴板

原文 §7 建议「b/c 若空，优先排查剪贴板注入竞态」。**这个方向是错的，而且很容易把人带进沟里。**

本机 shell 的 `LANG` / `LC_ALL` 为空、`LC_CTYPE=C`。在这个环境下 `printf '中文' | pbcopy` 会写出**长度为 0 的剪贴板条目**：

```
$ printf '纯文本 你好' | pbcopy; osascript -e 'clipboard info'
«class utf8», 0, «class ut16», 2, string, 0, Unicode text, 0       ← 全是 0
$ osascript -e 'set the clipboard to "纯文本 你好"'; osascript -e 'clipboard info'
Unicode text, 12, string, 11, «class utf8», 16, «class ut16», 14   ← 正常
```

拿坏掉的剪贴板跑捷径，会看到「上传 0 字节」，**极易误判成「Shortcuts 不支持中文」**——本次接续时就一度这样误判并写进文档，随后被证伪：用 AppleScript 正确写入后，中文在**原版与简化版上都能正常上传**。

**所以：设置含非 ASCII 的剪贴板必须用 `osascript -e 'set the clipboard to "…"'`，不要用 `pbcopy`。**

→ 详细记录、构建管线与已知坑见 **`shortcuts/apple/README.md`**；构建用 `shortcuts/apple/build.sh`，校验用 `verify.py`。



## 0. 仓库未提交改动（全部在本机工作区，未 commit）

| 文件 | 状态 | 性质 |
|---|---|---|
| `cloud-clip/lib/handler.go` | 已修改 | 服务端修复（见 §2），**已验证** |
| `cloud-clip/lib/unicode_text_test.go` | 新增 | 回归测试（见 §2），**已验证** |
| `shortcuts/apple/source/Cloud-Clipboard-Send.cherri` | 已修改 | 新设计（见 §3），**未验收** |
| `shortcuts/apple/source/*` 整体 | 未跟踪 | 快捷指令源码 + 助手脚本，属交付物 |
| `.gitignore` / `lib/main.go` / `lib/utils.go` | 已修改(会话前既有) | 未深究，提交前请逐项 review |
| `.workbuddy-ai/` | 未跟踪 | 非本项目产物（可能是其他工具的配置），勿提交 |

提交建议：服务端修复（handler.go + unicode_text_test.go）可单独提交；Send.cherri 待验收后再提交。

## 1. 要解决的原始问题（用户需求）

1. **Chrome 右键"选中文字"上传 → 被当文件存为 `clipboard.txt`**（期望：纯文本消息）
2. **网页/富文本剪贴板上传 → 消息里的内容是带 `<html>` 的 HTML 源码**（期望：纯文本）

约束：文件/图片/二进制仍必须是文件语义，不能被当成文本。

## 2. 服务端修复（已完成 + 已验证，Go 侧可靠）

均在 `cloud-clip/lib/handler.go`，运行 `go test ./lib/` 全绿：

- **UTF-16LE BOM 误判 MP3**：含 NUL 的载荷先走 `decodeUnicodeText` 归一化，再进魔数表。修复前 `0xFF 0xFE` 起步的 UTF-16LE 中文被 MP3 帧同步误判为 audio。回归用例 `TestSniffPayloadUTF16LEWithBOMNotMisclassifiedAsMP3`。
- **HTML→纯文本**：`storeRawText` 入口调 `htmlDocumentToPlainText`。仅当文本形如完整 HTML 文档（`<!doctype html|<html|<head|<body|<?xml` 开头）才剥标签/script/style/实体；普通文本和含 `<`、`>` 的代码原样保留。用例 `TestHTMLDocumentToPlainText`、`TestHTMLDocumentKeepsPlainTextAndCodes`、`TestHTMLDocumentStripsScriptAndStyle`。
- curl 实测：UTF-16LE+BOM 中文 → text；HTML 富文本 → `标题 Hello & 你好`；`code: a < b && c` 原样。

## 3. 快捷指令当前实现（PK170，最小化设计，**尚未跑验收矩阵**）

设计原则：**客户端不做任何文本转换**（getText/getName 在本机运行时不稳定，见 §5），只做"输入类型"路由，其余全交给服务器嗅探。

`Cloud-Clipboard-Send.cherri` 决策树：

```
if 来自快捷指令输入(ShareSheet/@fromInput):
   类型 ∉ {文本,text,字符串,string,url,URL}  →  as=file 原样上传（文件/图片保真）
   类型 ∈ 文本类                             →  setName(item,"clipboard.txt") 原样上传（服务器→文本消息；HTML 由服务器剥标签）
else（剪贴板）:
   typeOf ∈ 文件/图片/视频等黑名单           →  原样上传（文件保留）
   其余                                     →  setName(item,"clipboard.txt") 原样上传（纯文本/URL/富文本→服务器→文本；HTML 服务器剥标签）
```

要点：右手键文本输入不再 `as=file`（解决问题1）；富文本 HTML 由服务器剥标签成纯文本（解决问题2）；剪贴板文本即使 Shortcuts 无法 getText 也能通过 setName 原样上传，由服务器按字节嗅探正确分类。

打断点：`cherri` 对 `if @fromInput && (A || B)` **带括号的条件分组会死循环挂起**，必须改写成嵌套 if 或 `@flag` 变量（已规避，见当前源码注释风格）。

## 4. 机器当前状态

- **Shortcuts 库**：唯一相关条目 = PK170 `Cloud-Clipboard-Send`（active，最新最小版）；测试探针（160/163/165/166/167/168/169）全部 tombstone；`Cloud-Clipboard-Receive` **未安装**（需重新导入，源码在 `shortcuts/apple/source/Cloud-Clipboard-Receive.cherri`）。
- **服务器**：已停止；无残留进程/无 /tmp 测试文件。发布方式参考 §6。
- **数据**：`cloud-clip/history.json`、`cloud-clip/uploads/` 保留（含用户旧数据 pachong.py 等）；会话的测试消息仅新增 1 条。仓库根的历史/上传（纯测试数据）已删除。
- **测试产物已清理**：/tmp 下 ccgo-server、ccconfig、日志、测试素材（cldbg/typeq/clip2/t1/t2）、等待导入的 Send 编译产物全部删除。

## 4.1 工具/编译环境（本机安装位置）

| 工具 | 位置 | 版本/说明 |
|---|---|---|
| **go** | `/usr/local/bin/go`（符号链接 → `/usr/local/Cellar/go/1.26.5/bin/go`） | go1.26.5 darwin/amd64，Homebrew 安装；`GOROOT=/usr/local/Cellar/go/1.26.5/libexec`，`GOPATH=/Users/jonny/go` |
| **cherri** | `/usr/local/bin/cherri`（独立可执行文件，非 brew 安装） | v2.3.0 |
| **codesign/sign** | `/usr/bin/codesign` | 系统自带 |

- 命令行直接用：`go`、`cherri`、`shortcuts`（均在 PATH）。
- cherri 若缺失/需重装：源码 https://github.com/electrikmilk/cherri（Swift 编写，`swift build` 后把产物 link/复制到 `/usr/local/bin/cherri`），或直接放置其 release 二进制。
- go 若缺失：`brew install go`。<br><br>

## 5. 踩坑纪要（对接续者重要）

- **不要在客户端依赖 getText/getName 的正确性**：本机多次实测 getText(剪贴板文本) 返回空或抛 `无法将文本转换为NSString`；getName 在剪贴板路径同样不可靠。v160（getText 全量）曾全绿，v163（加入始终 run 的 getName）b 场景(纯文本剪贴板)必空；结论是这两个动作本身在本机 flaky。
- **typeOf 会闪变**：同一 icns 文件多次运行 typeOf 在 文件/图像/文本 之间飘，别拿它做唯一判据。
- **macOS Shortcuts 导入流程**：`cherri Cloud-Clipboard-Send.cherri --skip-sign` → `python3 inject_import_questions.py <source.cherri> <unsigned.shortcut>`（3 个 question 动作 [0,2,4]）→ `pkill -x Shortcuts; sleep 2; open -a Shortcuts; sleep 6` → `shortcuts sign -i <u> -o <s> --mode anyone` → `open <s>` → 等用户"添加"。新导入的捷径**首次 CLI 运行前需用户在界面点一次信任**。
- **CLI 运行**：`shortcuts run "Cloud-Clipboard-Send"` 后台化立即返回；上传成功需轮询服务端 `content/latest` 确认。偶发 `WFBackgroundShortcutRunnerErrorDomain错误1` 属系统瞬时，重试即可。
- **剪贴板注入**：设剪贴板后必须 `sleep 1` 再运行捷径，否则读到旧值。
- 中文字面量与 == 比较在重启前曾偶发 false（运行时 flake），重启后正常。

## 6. 验收矩阵（接续者需用 PK170 重跑）

服务器：`cd cloud-clip && go build -o /tmp/ccgo-server . && /tmp/ccgo-server -config <repo>/config.json`（仓库 `config.json` 内容在会话中曾含 `"auth":"456"`，现在为空/被删，需按真实配置重建；鉴权 header 在捷径里对应 `@authorization`）。
（注：本次会话用 `sed` 去掉 auth 生成临时配置存 /tmp，已删除——接续者必须按仓库真实配置重建。）

| # | 场景 | 期望 |
|---|---|---|
| a | 网页/富文本剪贴板共享 | 文本消息，**纯文本无 HTML 标签** |
| b | 纯文本剪贴板 | 文本消息，内容非空 |
| c | 链接剪贴板 | 文本消息（URL 原文） |
| d | `-i rightclick.txt`（右键选中文本） | 文本消息 |
| e | `-i GenericDocumentIcon.icns` | 文件（as=file） |
| f | `-i u.bin`（二进制） | 文件（as=file） |

观察：`curl http://127.0.0.1:9501/content/latest?room=default&json=1`。

## 7. 接续者下一步建议

1. `cd cloud-clip && go test ./lib/` 确认服务端修复在；重建真实 config.json（含鉴权）并起服务器。
2. 用 PK170 跑 §6 矩阵（b/c 若空，优先排查剪贴板注入竞态——sleep 1；仍空再考虑回退 v160 结构：剪贴板路径完全无 getName + getText 上传）。
3. 验收后 `shortcuts` 提交：handler.go 修复 + unicode_text_test.go + Send.cherri。
4. 如需完整功能，重新导入 `Cloud-Clipboard-Receive.shortcut`（源码同目录，复刻同款签名/导入流程）。