# Apple 快捷指令（iOS / macOS）

两个原生快捷指令：**一键发送**剪贴板或分享的内容，**一键拉取**最新一条。

| 捷径 | 作用 | 怎么唤起 |
| :--- | :--- | :--- |
| **Cloud-Clipboard-Send-Text** | 发送**文本** | iOS 分享菜单、macOS Finder 右键 →「快速操作」、菜单栏 |
| **Cloud-Clipboard-Send-File** | 发送**文件 / 图片** | 同上 |
| **Cloud-Clipboard-Receive** | 拉取**最新一条**，放进剪贴板（是文件则保存） | macOS 菜单栏、Spotlight 搜索、Apple Watch |
| **Cloud-Clipboard-Receive-By-ID** | 拉取**指定 ID** 的那一条 | 同上 |

> **发送为什么要分成两个？** 因为剪贴板条目的「类型」在客户端判不准——它的名字反映的是
> **剪贴板格式**（带 HTML 味就变 `Clipboard <日期>.html`、图片变 `.png`、纯文本没后缀），
> 不是内容。让用户自己选发什么，比让程序猜可靠。

## 安装

1. 下载本目录下的 `Cloud-Clipboard-Send-Text.shortcut`、`Cloud-Clipboard-Send-File.shortcut`、`Cloud-Clipboard-Receive.shortcut` 与 `Cloud-Clipboard-Receive-By-ID.shortcut`
2. 双击导入「快捷指令」App
3. 导入时会让你填几项：
   - **服务器地址** —— 如 `https://clip.example.com`，或局域网 `http://192.168.1.10:9501`
   - **房间名** —— 多设备收发同一内容时填相同值；默认 `default`
   - **密码** —— 服务端设了密码就填，没设就留空
   - **设备名称**（仅两个「发送」有）—— 接收端用它显示消息来自哪台设备，默认 `快捷指令`。
     多台设备建议填成不同的名字（如「我的 iPhone」「办公室 Mac」），
     接收端一眼就能看出是哪台发的；留空则按系统信息推断
4. **导入后在快捷指令 App 里手动各跑一次**，把网络与通知权限授权掉，之后命令行/自动化才能用

> 填错了随时能改：在捷径里找到最顶部那个「文本」动作，直接编辑。

## 怎么用

**发送**

- 发**文件/图片** —— iOS 分享菜单选「发送文件」；macOS Finder 右键 →「快速操作」→ 发送文件
- 发**文字** —— 复制好，运行「发送文本」（菜单栏或 Spotlight 都行）
- 从浏览器/编辑器里分享选中的文字 —— 选「发送文本」

**拉取**

- **最新一条** —— 运行「接收最新」，直接进剪贴板
- **指定某一条** —— 运行「按 ID 接收」，填上 ID
  - ID 从哪来：**发送成功的通知里就有**（`已发送 <ID>`）
  - 也可以在导入时把 ID 填进问答里固定住；留空则每次运行时弹框问

如果是文件，会弹出保存对话框（见下方「已知限制」）。

发送/拉取完成后的反馈：

- **发送成功** —— 两个平台都弹通知（内容 `已发送 <ID>`），iOS 上**再振一下**。
  通知不能省：**ID 只在这里出现**，「按 ID 接收」要靠它。
- **拉取成功** —— macOS 弹通知，iOS 振动。这里两边各给一个等价信号就够了。

## 发什么会变成什么

| 你发的东西 | 接收端拿到 |
| :--- | :--- |
| 剪贴板里的文字 | 文本消息，可直接粘贴 |
| 剪贴板里的富文本 / 网页选中文字 | 纯文本（HTML 标签会自动剥掉） |
| 剪贴板里的图片 | 图片 |
| 分享的任意文件（`.md` / `.sh` / `.png` / `.pdf` …） | **文件，文件名和扩展名原样保留** |

**文件名是保真的。** 分享 `测试笔记.md`，对方拿到的就是 `测试笔记.md`——不会变成 `clipboard.txt`。

## 服务端要求

| 接口 | 用途 | Go 服务端 | Cloudflare Worker |
| :--- | :--- | :--- | :--- |
| `POST /text` | 发文本 | ✅ | ✅ |
| `POST /upload` | 发文件（multipart，part 自带真实文件名） | ✅ | ✅ |
| `GET /content/latest?json=1` | 拉取最新内容 | ✅ | ✅ |
| `GET /file/:uuid/:filename` | 下载文件 | ✅ | ✅ |

**只需要这四个接口，两种部署都支持，不需要额外配置。**

> 服务端不再需要「按内容猜类型」那套逻辑（`/upload/raw` 已于 2026-09-17 删除）——
> 判型交给用户之后，那些代码就没有存在理由了。

## 已知限制

- **拉取文件只能每次询问保存位置。** Shortcuts 的文件夹选择器作用域限于「文件提供者命名空间」（iCloud 云盘 / 我的 Mac），
  `/Users/…/Downloads` 这类绝对路径用不了——所以「静默保存到指定目录」在 Shortcuts 里**无法表达**。
- **重复拉取同一文件会累积**：存成 `clipboard-2.bin`、`clipboard-3.bin`……不会覆盖。
- 如果发送端不是本项目的客户端（没带上文件名），接收端拿到的会退化成 `clipboard.<扩展名>`。

## 开发者

```bash
./build.sh                # 构建全部源码并签名
./build.sh Send           # 只构建文件名含 Send 的源码
./build.sh --no-sign      # 只编译 + 后处理，不签名（CI / 无界面环境）
./build.sh --verify-only  # 只跑校验，不重新构建
```

| 路径 | 说明 |
| :--- | :--- |
| `source/*.cherri` | **唯一可信源**，一切改动都在这里 |
| `source/inject_import_questions.py` | 编译后处理：补 `ActionIndex`（cherri 不输出，长期必需） |
| `source/patch_shortcut.py` | 编译后处理：补齐 cherri 表达不了的 plist 结构 |
| `build.sh` | 构建入口：编译 → 两个后处理 → 签名 → 校验 |
| `verify.py` | 构建后断言，把踩过的坑变成检查项 |
| `*.shortcut` | 签名产物（`--mode anyone`，iOS / macOS 均可导入） |

当前产物规模：Receive **123** 动作 / Receive-By-ID **141** / Send-Text **83** / Send-File **81**。
`build.sh` 每次都会打印动作数、导入问答的 ActionIndex 与校验结果。

依赖 [Cherri](https://github.com/electrikmilk/cherri)（**Go 写的，不是 Swift**）。
版本必须钉到 commit `dc82114f346f`，`build.sh` 会检查并在不一致时告警。

## 深入

判型是怎么做的、每个结论的实测数据、踩过哪些坑、哪些结论被推翻过——
都在 **[NOTES.md](./NOTES.md)**。README 只讲怎么用，那份讲为什么。
