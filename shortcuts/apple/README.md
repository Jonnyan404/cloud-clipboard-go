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

## 构建

```bash
./build.sh                    # 构建全部源码并签名
./build.sh min                # 只构建文件名含 "min" 的源码
./build.sh --no-sign          # 只编译 + 注入问答（CI / 无界面环境）
./build.sh --verify-only      # 只跑校验
```

签名阶段会重启 Shortcuts.app，这是必须的——否则 `shortcuts sign` 会读到过期的动作定义。签名时打印的若干 `Unrecognized attribute string flag` 属正常噪音。

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

`Cloud-Clipboard-Send-Min` 是简化版：动作数 **121 → 68**，本地化类型名比较 **29 → 0**，客户端不再判断类型、改由服务端按魔数嗅探单点决策。

### Receive 验收（2026-09-16，首次）

`Cloud-Clipboard-Receive` 此前从未安装、从未验收。补上入口（原先 `WFWorkflowTypes` 只有 `[Watch, ShowInSearch]`，等于没有可用入口）后实测四类：

| # | 服务端内容 | 期望 | 实测 |
| :--- | :--- | :--- | :--- |
| R1 | 纯文本 | 进剪贴板 | ✅ `阿岚的文本验收 12345` |
| R2 | 富文本 HTML（服务端已剥成纯文本） | 进剪贴板、无标签 | ✅ `标题 Hello & 你好` |
| R3 | PNG 图片 | 进剪贴板（图像） | ✅ 剪贴板出现 `PNGf` / TIFF / JPEG / BMP 等类型 |
| R4 | 二进制文件 `u.bin` | 下载 + 保存 | ✅ 落盘 `~/Downloads/clipboard.bin`，SHA-256 与源文件一致 |

**4/4 通过，且全程无需人工点击。** `saveFilePrompt` 的「询问保存位置」默认为关，会直接存到 `~/Downloads` 而不弹对话框。

> 注意 `/content/latest` 只返回**最新一条**，所以逐条验收时必须在每个用例前重新上传该用例的内容。

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

