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
| `POST /upload/raw` | Send 上传（body 即原始字节） | ✅ | ✅ 2026-09-17 补齐 |
| `POST /upload/base64` | Send 上传的 base64 变体 | ✅ | ✅ 2026-09-17 补齐 |
| `GET /content/latest?json=1` | Receive 拉取最新内容 | ✅ | ✅ |
| `GET /file/:uuid/:filename` | Receive 下载文件 | ✅ | ✅ |

**两端现在都可用。**

Worker 侧的实现（2026-09-17）：

- `src/sniff.js` —— 内容嗅探与文本规范化，**与 Go 版逐条对齐**；刻意零依赖，因此可被 Node 直接单测
- `src/handlers/raw-upload.js` —— 只做「嗅探 + 请求包装」，存储逻辑**委托**给既有的
  `TextHandler.create` / `FileHandler.upload`，不复制 D1/R2 写入、清理与广播
- 路由加在 `src/index.js`
- 测试：`cd cloudflare/workers && npm test` —— 25 项单测（用例与 Go 的 `unicode_text_test.go` 逐条对应）
  + 28 项端到端（HTTP → 嗅探 → 委托 → D1/R2 → 响应 JSON），用 `node:sqlite` 充当 D1、Map 充当 R2，
  **不需要 wrangler、不联网**

> 说明：端到端测试跑的是处理器的真实调用路径，但没有经过 wrangler 的完整运行时
> （本地 wrangler 因网络限制未能启动）。若要验证线上行为，需实际部署后请求一次。

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

#### 为什么「让客户端区分文件和文本」做不到

设想：客户端判断输入是文件就传 `?as=file`，是文本就不传——这样 `.txt` 文件能保持文件语义，而复制文字仍走文本。**但这条路走不通**，两条独立证据：

**证据一：`typeOf` 无法区分。** 诊断捷径实测：

| 输入 | `typeOf` |
| :--- | :--- |
| 剪贴板纯文本 | `文本` |
| **`.txt` 文件** | **`文本`** ← 与剪贴板文本完全相同 |
| HTML / 富文本剪贴板 | `多信息文本` |
| `.html` 文件 | `多信息文本` |
| `.png` | `图像` |

Shortcuts 会把 `.txt` 文件**强制转换成字符串**，所以在类型层面它与剪贴板里的文字无从分辨。

**证据二：没有「是某种类型」这种条件。** 三份独立来源一致：

- cherri 源码：条件只把比较值路由到 `WFDate` / `WFNumberValue` / `WFConditionalActionString`
- 逆向工程文档 [drewburchfield/shortcuts-toolkit](https://github.com/drewburchfield/shortcuts-toolkit)：条件仅 `Equals` / `Contains` / `Begins With` / `Ends With` / `Is Greater Than` / `Is Less Than` / `Is Between` / `Has Any Value` / `Does Not Have Any Value`
- 本机 14 个在用捷径：无一个使用内容类条件

→ **所以 `.txt` 该当文件还是文本，只能二选一定死，不能按输入类型分：**

| 方案 | 效果 | 代价 |
| :--- | :--- | :--- |
| 一律当文本（**现状**） | 分享 `.txt` 得到文字 | 拿不到文件本体 |
| 一律当文件（客户端对 share-sheet 输入加 `?as=file`） | 分享 `.txt` 得到文件 | **Chrome 右键"选中文字"也会变成文件——即回归用户问题 #1** |

> 唯一可能解开的线索：老版本问题 #1 记录里那个文件叫 `clipboard.txt`，而这个名字是**客户端自己 `setName` 起的**——真文件会自带文件名。若成立，则 Chrome 传的是字符串、`.txt` 分享传的是文件，理论上可区分；但这与上表 `typeOf` 的实测矛盾（两者都是 `文本`），**需要真机再验一次才能定论**。

**两个容易被误当成 bug 的行为：**

1. **`.txt` 文件会变成文本消息，而不是文件。** 嗅探只看内容——`.txt` 的内容就是合法 UTF-8，于是被判为文本，文件性（文件名、可下载性）随之丢失。想要"文件就是文件"的语义，需要客户端显式传 `?as=file`，而当前客户端不判断类型，所以没有这个能力。
2. **文件名会退化。** 客户端不传 `?name=`，服务端只能按嗅探到的扩展名命名，原名（如 `u.bin`）会变成 `clipboard.bin`。

### 为什么类型判断放在服务端，而不是客户端

cherri 的高层语法**做不到可靠判断**：条件只支持字符串比较，唯一的类型手段 `typeOf` 返回**本地化字符串**（"文件"/"图像"/"文本"），非中英文系统上会静默失效——原版那 29 项字符串比较就是这么来的，也是它最大的隐患。

`rawAction("...")` 能注入任意动作 plist，但**前提是存在可注入的目标**——而「is a 内容类」这种条件在三份独立来源里都查不到（见上文「为什么让客户端区分文件和文本做不到」），所以它救不了场。

### `getName` 能区分文件与文本，`getText` 则不能（2026-09-17 实测）

用带分享菜单入口的诊断捷径（`CC-Diag-Share`）实测四种来源：

| 来源 | `typeOf` | `getName` | `getText` |
| :--- | :--- | :--- | :--- |
| 剪贴板（无输入） | `文本` | `Clipboard 2026年9月17日 09.47` | 执行成功，**返回值未观测到** |
| **分享 `.txt` 文件** | `文本` | **`requirements`** ← 真实文件名 | ✅ 返回文件内容 |
| **分享 `.html` 文件** | `URL` | **`Cloud Clipboard · OpenWrt LuCI 界面重设计预览`** ← 真实文件名 | ✅ 返回文件内容 |
| **分享文本（字符串）** | `文本` | **`django==4.2.*`** ← **内容首行** | ✅ 返回文本 |

**两条结论：**

1. **分享文件确实保留真实文件名**（此前只是假设，现已验证）。
2. **判据是 `getName` 是否等于内容的开头**：字符串的名字取自内容本身，文件的名字是文件名。
   剪贴板路径的名字是 `Clipboard <日期时间>`，需靠 `ShortcutInput` 是否为空单独排除。
3. **`getText` 可用与否不能用来区分"文件 vs 文本"**——四种来源都能成功执行。

### ⚠️ 订正：`getText` 并不抛异常

先前根据"第二步上报未到达"推断"`getText` 对剪贴板文本抛异常"，**该结论是错的**。

用固定标记 `S3-GETTEXT-RETURNED` 把「`getText` 的执行」与「上报其结果」解耦后，**四次运行该标记全部到达**，包括剪贴板路径那次。
→ **`getText` 从未抛异常**；失败一直发生在**上报环节**——把文本值当 `File` body 上传时 Shortcuts 会把它物化成文件，
若物化出的名字非法就报 **`未能存储该项目，因为文件名称无效`**，且**该请求根本不会发出**。

→ 仍未观测到的：剪贴板路径下 `getText` 的**返回值**（其结果上报失败）。前人记录"返回空"**既未证实也未证伪**。

> **方法论教训**：用"分步上报"定位失败时，**必须检查错误信息本身**——否则会把"上报失败"误读成"被测动作失败"。

### 用 `getName` 判别：可行，但风险要认

判据本身可用，但有两点必须说明：

1. 该命名约定是 **Shortcuts 的内部实现，未公开、可能随版本变化**（实测中文环境下 `Clipboard ` 仍是英文，非本地化）。
2. 需要在分享路径上调用 `getName` + `getText`。**若某类输入让它们抛异常，捷径会直接挂掉**（Shortcuts 没有 try/catch）——**比"分类错误"更糟**。实测 3/3 分享运行正常，但样本很小。
3. 失败模式是安全的：判错时退化为"当文本"，**等于现在的行为，不会更糟**。

→ 因此**备选方案是弹菜单让用户选**（零歧义、无崩溃风险，代价是分享时多一次点击）。两者取舍见下。

放在服务端还有两个独立好处：**一处实现、所有客户端受益**（Web / Android / curl 都吃到同样的嗅探与 HTML 降级），且**可测**（25 个 Go 测试无需 Apple 设备）。


> **不要把嗅探与 HTML 降级"优化"到客户端**，除非你愿意维护手写 plist 并放弃多客户端复用。

> **订正记录**（本项目反复修正的地方，留档以免重走）：
> - 曾把"第 2 步上报未到达"读成"`getText` 抛异常"——**证据不足**，Shortcuts 报的错误指向上传环节。
> - 曾推测「`getText` 不可靠」是 `pbcopy` 写坏剪贴板的假象——该推测已证伪（剪贴板用 AppleScript 写入后仍复现），但**失败原因至今未定论**。
> - 曾把 `WFFileDestinationPath` 当作绝对路径来设计——实测是文件提供者命名空间，绝对路径不可用。
> - 曾把 `show()` 当作无害的提示——它会在屏幕中央弹结果卡片打断视线，已改为通知。

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

| # | 场景 | 期望 | 原版（已退役） | 现行 `Cloud-Clipboard-Send` |
| :--- | :--- | :--- | :--- | :--- |
| a | HTML / 富文本剪贴板 | 纯文本无标签 | ✅ | ✅ |
| b | 纯文本剪贴板（含中文） | 文本非空 | ✅ | ✅ |
| c | 链接剪贴板 | URL 原文 | ✅ | ✅ |
| d | 右键选中文本（`.txt`） | 文本消息 | ❌ 存成文件 `clipboard.txt` | ✅ 文本 |
| e | `.icns` 文件 | 文件 | ✅ | ✅ |
| f | 二进制文件 | 文件 | ✅ | ✅ |

**原版 5/6，现行版 6/6。** 现行版修好了 d（把 `.txt` 当文件存的那个回归），其余场景行为一致。

### 2026-09-17：类型判断改到客户端（方案 A）

上表是"客户端不判断类型、全交给服务端嗅探"的结果。它的代价是 **`.txt` 文件会变成文本消息**（见上文「行为说明」）。
09-17 改成：**客户端用 `getName` 判别，分享文件时显式传 `?as=file`。**

设计要点（两条路径**完全分叉**，互不共享新原语）：

- **剪贴板路径**：逐动作复刻 09-16 已验证的版本，**零改动、零风险**
- **分享路径**：判据三层
  1. `getName` 含 `Clipboard ` → 文本（系统为无名字符串生成的名字）
  2. 否则若 `getText` 的结果以 `getName` 开头 → 文本（分享字符串的名字 = 内容首行）
  3. 否则 → `?as=file`（真文件保留自己的文件名）
- **误判安全**：图片/二进制即使被判成文本，服务端仍按魔数嗅探重新分流，所以判错无害

**验收矩阵（2026-09-17，全绿）**：

| # | 用例 | 结果 | 判定 |
| :--- | :--- | :--- | :--- |
| 1 | 剪贴板 ASCII | `type=text` | ✅ |
| 2 | 剪贴板中文 | `type=text` | ✅ |
| 3 | **`-i .txt` 文件** | **`type=file`** | ✅ **由"文本"变为"文件"——本次改动的目标** |
| 4 | `-i .png` | `type=image` | ✅ |
| 5 | `-i .html` 文件 | `type=file` | ✅ |
| 6 | URL scheme 文本（走分享分支） | `type=text` | ✅ |

> 用例 6 很关键：URL scheme 传字符串时 `ShortcutInput` **是设置的**（走分享分支），
> 而它的名字是 `Clipboard <日期时间>`——**只有第 1 层判据能接住它**。
> 早期一版没有这层，会把它当文件发出去。

**已知遗留**：客户端不传 `?name=`，服务端仍按嗅探结果命名，所以文件名会退化（`.txt` → `clipboard.txt`，`.html` → `clipboard.txt`）。
要保留原文件名，需客户端补传 `?name=`，并让服务端在名字缺扩展名时补上嗅探到的扩展名——**尚未实施**。

> **2026-09-16：原版已退役并从仓库删除**（源码与签名产物均已移除，需要时可在 git 历史里找回）。
> 上表的原版列保留作为对照记录。它的 29 项本地化类型名比较、4 路分支与 `?as=file` 协议都不再维护。
> **注意**：原版曾占用 `Cloud-Clipboard-Send` 这个名字，该名字现已由现行的简化版接手，所以下表与提交历史里同名指的不是同一个东西。

现行的 `Cloud-Clipboard-Send` 是简化重写版：动作数 **121 → 68**，本地化类型名比较 **29 → 0**，客户端不再判断类型、改由服务端按魔数嗅探单点决策。

### 发送结果用通知，不用结果卡片

原先用 `show()`（Show Result），每次发送都会在屏幕中央弹一张卡片，打断视线。现改为：

```cherri
showNotification("已发送 {@savedID}", "Cloud Clipboard", false)
```

cherri 的动作名是 **`showNotification(body, title, playSound, attachment)`**，不是 `notification`。`playSound` 传 `false`，连续发送时不会反复响铃。动作数不变（原版 121 / 现行 68）。

**验证状态：已完整验证。** 跑通后服务端正常收到消息（产物里 `notification=1`、`showresult=0`），且已在界面上确认通知确实弹出。

> 自查记录：这一步最初只能验到"流程没被破坏"，通知是否真显示无法自证——系统日志被沙箱挡住（`log: Cannot run while sandboxed`），通知数据库需要全盘访问权限，ncprefs 里的位掩码含义未确认。最后是人工目视确认的。**无法自证时如实标注为未验证，比含糊过去强。**

### Receive 验收（2026-09-16，首次）

`Cloud-Clipboard-Receive` 此前从未安装、从未验收，2026-09-16 首次补测。先补上入口（原先 `WFWorkflowTypes` 只有 `[Watch, ShowInSearch]`，等于没有可用入口）后实测四类：

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

### 拉取成功的反馈（各平台）

`vibrate()` 是 **iOS 专属**动作，macOS 上不可用，所以源码里它一直包在 `if @model != "Mac"` 里。
**但此前没人给 macOS 补等价反馈**，导致文本与图片两条分支在 Mac 上静默完成——拉取成功与否完全看不出来。
2026-09-17 补上通知：

| 分支 | macOS | iOS |
| :--- | :--- | :--- |
| 文字 | `showNotification("已接收文字，已复制到剪贴板")` | `vibrate()` |
| 图片 | `showNotification("已接收图片，已复制到剪贴板")` | `vibrate()` |
| 文件 | 保存对话框 + `reveal()` 打开 Finder（本身即反馈，未再加通知） | `saveFilePrompt` + `vibrate()` |

> 通知需要系统授权。若 macOS 上看不到，检查 **系统设置 → 通知 → 快捷指令**。

### 保存位置：只能每次询问（结论）

**「静默保存到指定目录」做不到**，所以 Receive 只保留「询问保存位置」一种行为，不提供保存目录配置项。

实测经过（2026-09-16）：

1. cherri 里确实有**两个**保存动作，`saveFile` 自带 `WFAskWhereToSave: false`，理论上可以静默直存：
   ```
   action default 'documentpicker.save' saveFilePrompt(file, ?overwrite)                        ← 弹对话框
   action 'documentpicker.save' saveFile(file: 'WFFileDestinationPath', content, ?overwrite) {
       "WFAskWhereToSave": false                                                                ← 不弹，直存
   }
   ```
   于是加了第 4 个导入问答 `qSaveDir`（填目录→静默直存，留空→询问）。
2. 在捷径 App 里关掉「询问保存位置」并**点选文件夹**后实测：**文件落到了 iCloud 云盘，而不是所选的真实路径。**

**根因**：Shortcuts 的文件夹选择器作用域限于**文件提供者命名空间**（iCloud 云盘 / 我的 Mac），`WFFileDestinationPath` 是**该命名空间内的路径**，不是真实文件系统路径。因此 `/Users/…/Downloads` 这类绝对路径不可用——**「静默保存到任意本地目录」在 Shortcuts 里无法表达**。

→ 已撤回 `qSaveDir` 与静默保存分支：Receive 由 126 动作回到 **113**，导入问答由 4 个回到 **3** 个，保存动作只剩 1 个（询问式）。

**「询问保存位置」本身就是原生保存对话框，也就是浏览选择器**——只是每次都要确认一次。这是当前唯一可行的行为。

> 备选（未采用）：若确实要静默，只能存到 iCloud 云盘内的固定位置，接受落点不在本地。

> 教训：`WFFileDestinationPath` 的路径语义我最初是靠猜的（还写进过文档），直到真机点选才发现是提供者命名空间。**涉及平台语义的参数，别靠推断，早点实测。**

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

