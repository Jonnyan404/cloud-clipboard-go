# Apple 快捷指令（iOS / macOS）

用 [Cherri](https://github.com/electrikmilk/cherri) 编写、编译并签名 `.shortcut` 的源码与构建管线。

## 目录

| 路径 | 说明 |
| :--- | :--- |
| `source/*.cherri` | **唯一可信源**。快捷指令的一切改动都在这里 |
| `source/inject_import_questions.py` | 编译后处理：补 `WFWorkflowImportQuestions.ActionIndex`（原因见下） |
| `build.sh` | 构建入口：编译 → 注入问答 → 签名 → 校验 |
| `verify.py` | 构建后断言：把已知的坑变成检查项 |
| `*.shortcut` | 签名产物（`--mode anyone`，iOS / macOS 均可导入） |

## 部署兼容性（重要）

快捷指令依赖的接口**只在 Go 服务端实现**。若部署的是 Cloudflare Worker 版本，请先看这张表：

| 接口 | 用途 | Go 服务端 | Cloudflare Worker |
| :--- | :--- | :--- | :--- |
| `POST /upload/raw` | Send 上传（body 即原始字节） | ✅ | ❌ **缺失** |
| `POST /upload/base64` | Send 上传的 base64 变体 | ✅ | ❌ **缺失** |
| `GET /content/latest?json=1` | Receive 拉取最新内容 | ✅ | ✅ |
| `GET /file/:uuid/:filename` | Receive 下载文件 | ✅ | ✅ |

**结论：Send 在 Cloudflare Worker 部署上不可用（会 404）；Receive 可用。**

Worker 的路由注释写着「无 /api 前缀，与自托管 Go 后端路径对齐」，新增的这两个端点打破了这个约定。
要在 Worker 上支持 Send，需在 `cloudflare/workers/src/index.js` 补上这两个路由，
并把内容嗅探、UTF-16/32 解码、HTML 降级这套逻辑用 JS 重写一遍。

> 说明：上表由**阅读路由定义**得出，未实际对 Worker 部署发起请求。若要确认，可对 Worker 的
> `/upload/raw` 发一次 POST，观察是否 404。

## 行为说明：上传什么会变成什么

客户端**不判断类型**，只把原始字节发出去；由服务端按内容魔数决定。实测结果：

| 你发的东西 | 服务端嗅探 | 接收端拿到 |
| :--- | :--- | :--- |
| 纯文本 | text | 文本消息，可直接粘贴 |
| **`.txt` 文件** | text（内容就是合法 UTF-8） | **文本消息（内容是文件里的文字），不是文件** |
| HTML / 富文本 | text（并剥掉标签） | 纯文本消息 |
| 图片（PNG / JPEG / …） | image | 进剪贴板的图片 |
| 其它二进制 | file | 可下载的文件，文件名退化为 `clipboard.<扩展名>` |

**两个容易被误当成 bug 的行为：**

1. **`.txt` 文件会变成文本消息，而不是文件。** 嗅探只看内容——`.txt` 的内容就是合法 UTF-8，于是被判为文本，文件性（文件名、可下载性）随之丢失。想要"文件就是文件"的语义，需要客户端显式传 `?as=file`，而当前客户端不判断类型，所以没有这个能力。
2. **文件名会退化。** 客户端不传 `?name=`，服务端只能按嗅探到的扩展名命名，原名（如 `u.bin`）会变成 `clipboard.bin`。

### 为什么类型判断放在服务端，而不是客户端

cherri 的高层语法**做不到可靠判断**：条件只支持字符串比较，唯一的类型手段 `typeOf` 返回**本地化字符串**（"文件"/"图像"/"文本"），非中英文系统上会静默失效——原版那 29 项字符串比较就是这么来的，也是它最大的隐患。

理论上可以用 `rawAction("...")` 手写一个「is a 内容类」条件（比较内容类标识符，语言环境无关），但那等于手写 plist。

**更关键的是 `getText` 本身不可靠。** 2026-09-16 用诊断捷径（分两步上报 `typeOf` 与 `getText` 的结果，只收到第一步即说明 `getText` 抛异常）实测：

| 输入 | `getText` 结果 |
| :--- | :--- |
| 纯 ASCII 剪贴板 | **抛异常**（复测仍失败，可复现） |
| 中文剪贴板 | **抛异常** |
| HTML / 富文本剪贴板 | ✅ 成功，且正确剥成纯文本 |
| `-i` `.txt` 文件（含中文） | ✅ 成功，中文完好 |
| `-i` `.png` | ⚠️ "成功"但只返回文件名 `test`，无意义 |
| `-i` `.html` 文件 | ⚠️ 成功但**乱码**（UTF-8 被按错误编码解码） |

→ **对"剪贴板里的纯文本条目" `getText` 直接抛异常**，所以客户端无法用一条无分支的路径处理文本。要用它就得先判断类型，而判断类型又只能靠 `typeOf`（本地化）——绕回原问题。

放在服务端还有两个独立好处：**一处实现、所有客户端受益**（Web / Android / curl 都吃到同样的嗅探与 HTML 降级），且**可测**（25 个 Go 测试无需 Apple 设备）。

> **不要把嗅探与 HTML 降级"优化"到客户端**，除非你愿意维护手写 plist 并放弃多客户端复用。

> 订正记录：曾推测「`getText` 不可靠」是 `pbcopy` 写坏剪贴板造成的假象。**该推测已被上表证伪**——剪贴板用 AppleScript 正确写入后依然失败。前人的判断是准确的。

## 构建

```bash
./build.sh                    # 构建全部源码并签名
./build.sh min                # 只构建文件名含 "min" 的源码
./build.sh --no-sign          # 只编译 + 注入问答（CI / 无界面环境）
./build.sh --verify-only      # 只跑校验
```

签名直接执行即可，**不需要先重启 Shortcuts.app**。曾照抄交接文档加入「`pkill Shortcuts` → 重开 → 再签名」的步骤，并断言「否则会拿到过期的动作定义」——那是没有依据的推测。2026-09-16 实测：不重启直接签名完全正常；而重启会扰动已安装的捷径（观察到被重命名、动作数 +1），因此已从 `build.sh` 移除。签名时打印的若干 `Unrecognized attribute string flag` 属正常噪音。

**cherri 的身份必须钉死。** 它当前是从源码构建的裸二进制，`cherri --version` 打印的 `v2.3.0` 只是版本常量，实际代码来自 commit `dc82114f346f`（`go version -m` 可验证，`build.sh` 会检查并在不一致时告警）。

> 注意：cherri **是 Go 编写的，不是 Swift**。网上流传的「Swift 编写、`swift build`」说法是错的，照做会失败。重建方式：
> ```bash
> go install github.com/electrikmilk/cherri@dc82114f346f
> ```

## 平台支持

一份源码可同时服务 iOS 与 macOS，靠 `#define from` 声明入口：

| define 值 | 生成的 `WFWorkflowTypes` | 出现位置 |
| :--- | :--- | :--- |
| `sharesheet` | `ActionExtension` | iOS 分享菜单 |
| `quickactions` | `QuickActions` | macOS Finder 快速操作 / 服务菜单 |
| `menubar` | `MenuBar` | macOS 菜单栏 |

逗号可叠加：`#define from sharesheet,quickactions,menubar`。合法取值共 9 个（`onscreen` `search` `spotlight` `menubar` `sharesheet` `sleepmode` `watch` `quickactions` `notifications`）。

**平台专属动作必须用 `@model` 守卫**，否则另一端会出错：

- `vibrate()` —— 仅 iOS / iPadOS
- `reveal()`（Reveal in Finder）—— 仅 macOS

`verify.py` 会提示 `WFWorkflowTypes` 是否漏了 macOS 入口。

## 已知的坑

### cherri v2.3.0

- **条件里不能内联函数调用**：`if count(@source) != 1` 会 `panic: interface conversion: interface {} is main.action, not main.varValue`。必须先落到变量：`@amount = count(@source)` 再比较。源码里那些看似冗余的变量赋值有一部分就是为绕开这个。
- **`if a && (b || c)` 带括号的条件分组会死循环挂起**，须改写成嵌套 if 或 `@flag` 变量。
- **输出文件名由 `#define name` 决定，`-o` 参数不生效**，且会静默覆盖同名产物。
- **不输出 `ActionIndex`**：`shortcutgen.go:1194` 读取 `q.actionIndex`，但全代码没有任何赋值点，恒为 0 并被 `omitempty` 省略 → 导入时的配置面板接不上。**因此 `inject_import_questions.py` 是长期必需，不是临时补丁。**

### Shortcuts 运行时

- **含非 ASCII 字符的文本条目无法上传**（见下方「当前状态」）。这是最影响中文用户的一条。
- **`typeOf` 会闪变**：同一个 icns 文件多次运行会在 文件/图像/文本 之间飘，不能作为唯一判据。
- **`getText` / `getName` 在本机不稳定**：`getText(剪贴板文本)` 曾返回空或抛 `无法将文本转换为 NSString`。
- 剪贴板注入后必须 `sleep 1` 再运行捷径，否则读到旧值。
- `shortcuts run` 是后台化的，立即返回；上传是否成功要轮询 `/content/latest` 确认。
- 新导入的捷径**首次 CLI 运行前需在界面点一次信任**。

### 产物比对

不要用文件大小比对：patcher 用 `plistlib.dump` 重写后体积会变（XML 序列化差异）。应比对**动作数 + 动作类型分布 + ActionIndex**——`verify.py` 就是干这个的。

## 当前状态（2026-09-16）

验收矩阵与实测结果（服务端为本地隔离实例，无鉴权）：

| # | 场景 | 期望 | 原版 `Cloud-Clipboard-Send` | `Cloud-Clipboard-Send-Min` |
| :--- | :--- | :--- | :--- | :--- |
| a | HTML / 富文本剪贴板 | 纯文本无标签 | ✅ | ✅ |
| b | 纯文本剪贴板（含中文） | 文本非空 | ✅ | ✅ |
| c | 链接剪贴板 | URL 原文 | ✅ | ✅ |
| d | 右键选中文本（`.txt`） | 文本消息 | ❌ 存成文件 `clipboard.txt` | ✅ 文本 |
| e | `.icns` 文件 | 文件 | ✅ | ✅ |
| f | 二进制文件 | 文件 | ✅ | ✅ |

**原版 5/6，简化版 6/6。** 简化版修好了 d（把 `.txt` 当文件存的那个回归），其余场景行为一致。

> **2026-09-16：原版 `Cloud-Clipboard-Send` 已退役并从仓库删除**（源码与签名产物均已移除，需要时可在 git 历史里找回）。
> 上表的原版列保留作为对照记录。它的 29 项本地化类型名比较、4 路分支与 `?as=file` 协议都不再维护。

`Cloud-Clipboard-Send-Min` 是简化版：动作数 **121 → 68**，本地化类型名比较 **29 → 0**，客户端不再判断类型、改由服务端按魔数嗅探单点决策。

### 发送结果用通知，不用结果卡片

原先用 `show()`（Show Result），每次发送都会在屏幕中央弹一张卡片，打断视线。现改为：

```cherri
showNotification("已发送 {@savedID}", "Cloud Clipboard", false)
```

cherri 的动作名是 **`showNotification(body, title, playSound, attachment)`**，不是 `notification`。`playSound` 传 `false`，连续发送时不会反复响铃。两个 Send 源码同步修改，动作数不变。

**验证状态：已完整验证。** 跑通后服务端正常收到消息（产物里 `notification=1`、`showresult=0`），且已在界面上确认通知确实弹出。

> 自查记录：这一步最初只能验到"流程没被破坏"，通知是否真显示无法自证——系统日志被沙箱挡住（`log: Cannot run while sandboxed`），通知数据库需要全盘访问权限，ncprefs 里的位掩码含义未确认。最后是人工目视确认的。**无法自证时如实标注为未验证，比含糊过去强。**

### Receive 验收（2026-09-16，首次）

`Cloud-Clipboard-Receive-Min`（2026-09-16 由 `Cloud-Clipboard-Receive` 改名，与 Send 保持 `-Min` 命名一致）此前从未安装、从未验收。补上入口（原先 `WFWorkflowTypes` 只有 `[Watch, ShowInSearch]`，等于没有可用入口）后实测四类：

| # | 服务端内容 | 期望 | 实测 |
| :--- | :--- | :--- | :--- |
| R1 | 纯文本 | 进剪贴板 | ✅ `阿岚的文本验收 12345` |
| R2 | 富文本 HTML（服务端已剥成纯文本） | 进剪贴板、无标签 | ✅ `标题 Hello & 你好` |
| R3 | PNG 图片 | 进剪贴板（图像） | ✅ 剪贴板出现 `PNGf` / TIFF / JPEG / BMP 等类型 |
| R4 | 二进制文件 `u.bin` | 下载 + 保存 | ✅ 落盘 `~/Downloads/clipboard.bin`，SHA-256 与源文件一致 |

**R1–R3 无需人工干预；R4 会弹出保存对话框，需要点一次。**

`saveFilePrompt`（`documentpicker.save`）**会弹出保存对话框并阻塞执行**，确认后才写入 `~/Downloads`。
产物里没有 `WFAskWhereToSave` 键，而 Shortcuts 在该键缺失时的默认行为正是「询问保存位置」——这也解释了为什么 cherri 的 `saveFilePrompt(file, overwrite)` 两个参数都不涉及这个开关。

验证方式（毫秒级轮询）：触发 Receive 后每 0.2 秒检查一次 `~/Downloads`，**30 秒内没有任何新文件出现**，同时界面上确实有对话框在等待 → 确认是阻塞式弹窗。

两个衍生的行为：

- **重复拉取会累积文件**：`overwrite` 传 `false`，所以第二次存成 `clipboard-2.bin`、第三次 `clipboard-3.bin`…… 不会覆盖。
- **文件名会退化**：客户端不传 `?name=`，服务端只能按嗅探到的扩展名命名，原名（如 `u.bin`）会变成 `clipboard.bin`。

### 静默保存（已实现，路径格式待验）

Receive 新增第 4 个导入问答 `qSaveDir`：

- **填了保存目录**（绝对路径，如 `/Users/你的用户名/Downloads`）→ 收到文件时**静默直存**，不弹对话框。
- **留空** → 回退到原来的「询问保存位置」。

实现方式：cherri 里其实有**两个**保存动作——

```
action default 'documentpicker.save' saveFilePrompt(file, ?overwrite)                        ← 弹对话框
action 'documentpicker.save' saveFile(file: 'WFFileDestinationPath', content, ?overwrite) {
    "WFAskWhereToSave": false                                                                ← 不弹，直存
}
```

所以**不需要 patcher**，`saveFile` 本身就带 `WFAskWhereToSave: false`。产物已验证包含该键与 `WFFileDestinationPath`。

**待验**：`WFFileDestinationPath` 接受何种路径格式（绝对路径 `/Users/…/Downloads`，还是文件提供者命名空间内的相对路径）**尚未真机验证**。

#### 为什么不做「导入时浏览选目录」

按 Apple 文档，导入问答是绑定到动作的**可用参数**上的（在 App 里从参数列表挑选）。但**目录在 Shortcuts 里是安全作用域 bookmark（不透明的二进制 blob）**，无法随捷径分发；而 `saveFile` 的路径来自变量，没有静态参数值可挂问题。**所以导入时浏览选目录做不到。**

三个可行方案，取舍不同：

| 方案 | 操作 | 结果 |
| :--- | :--- | :--- |
| **A. 填路径**（当前） | 导入时手打绝对路径 | 静默保存，不弹窗 |
| **B. 留空**（当前回退） | 什么都不填 | 每次弹**原生保存对话框**——那本身就是个浏览选择器，只是每次都要问 |
| **C. App 内点选**（推荐给不熟路径的人） | 导入后打开捷径，在「存储文件」动作里点选目标文件夹 | 静默保存且无需打路径；**一次性操作** |

> 方案 C 的代价：安装好的捷径会与 `.cherri` 源码不一致，**重新构建导入会覆盖掉这次点选**。
> 另外方案 C 能否点选我无法在无界面环境验证，属预期行为。

> 注意 `/content/latest` 只返回**最新一条**，逐条验收时必须在每个用例前重新上传该用例的内容。

> **排查教训（重要）**：判断某个动作是否弹窗，不能靠"我看到弹了"，**更不能靠"文件出现了所以没弹"——后者会被用户的点击污染**。
> 本次为此连续误判两次：第一次直接断定"不弹窗"，第二次做了所谓"对照实验"，却没确认用户真的没点，
> 于是把用户点出来的文件当成了静默保存的证据。可靠做法是**测量阻塞**（毫秒级轮询 + 确认界面状态）或**直接检查产物参数**。

### ⚠️ 测试手法陷阱：不要用 `pbcopy` 写非 ASCII 内容

本机 shell 的 `LANG` / `LC_ALL` 为空、`LC_CTYPE=C`。在这个环境下 `printf '中文' | pbcopy` 会写出**损坏的剪贴板条目**：

```
$ printf '纯文本 你好' | pbcopy; osascript -e 'clipboard info'
«class utf8», 0, «class ut16», 2, string, 0, Unicode text, 0       ← 长度全是 0
$ osascript -e 'set the clipboard to "纯文本 你好"'; osascript -e 'clipboard info'
Unicode text, 12, string, 11, «class utf8», 16, «class ut16», 14   ← 正常
```

拿这个坏掉的剪贴板去跑捷径，会看到「上传 0 字节」，**极易误判为 Shortcuts 不支持中文**。实测证明是假象：用 AppleScript 正确写入后，中文文本在原版与简化版上都能正常上传。

**因此设置含非 ASCII 的剪贴板时，必须用 `osascript -e 'set the clipboard to "…"'`，不要用 `pbcopy`。**

> 订正记录：曾据此误判「Shortcuts 对含非 ASCII 的文本条目发不出字节」，并一度写进本文档与 `docs/handover.md`。**该结论已证伪**（2026-09-16 晚）。

