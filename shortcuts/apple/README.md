# Apple 快捷指令（iOS / macOS）

两个原生快捷指令：**一键发送**剪贴板或分享的内容，**一键拉取**最新一条。

| 捷径 | 作用 | 怎么唤起 |
| :--- | :--- | :--- |
| **Cloud-Clipboard-Send** | 发送：剪贴板内容 / 分享的文件 | iOS 分享菜单、macOS Finder 右键 →「快速操作」、菜单栏 |
| **Cloud-Clipboard-Receive** | 拉取：把最新一条放进剪贴板（是文件则保存） | macOS 菜单栏、Spotlight 搜索、Apple Watch |

## 安装

1. 下载本目录下的 `Cloud-Clipboard-Send.shortcut` 与 `Cloud-Clipboard-Receive.shortcut`
2. 双击导入「快捷指令」App
3. 导入时会让你填三项：
   - **服务器地址** —— 如 `https://clip.example.com`，或局域网 `http://192.168.1.10:9501`
   - **房间名** —— 多设备收发同一内容时填相同值；默认 `default`
   - **密码** —— 服务端设了密码就填，没设就留空
4. **导入后在快捷指令 App 里手动各跑一次**，把网络与通知权限授权掉，之后命令行/自动化才能用

> 填错了随时能改：在捷径里找到最顶部那个「文本」动作，直接编辑。

## 怎么用

**发送**

- iOS —— 任意 App 里分享 → 选「Cloud-Clipboard-Send」
- macOS —— Finder 里右键文件 →「快速操作」→ Cloud-Clipboard-Send
- 只想发剪贴板 —— 先复制，再运行捷径（菜单栏或 Spotlight 都行）

**拉取** —— 运行 Cloud-Clipboard-Receive。最新一条会进剪贴板；如果是文件，会弹出保存对话框。

发送成功会弹通知；拉取成功也有通知（iOS 上是振动）。

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
| `POST /upload/raw` | 剪贴板路径（服务端按内容分流文字 / 图片） | ✅ | ✅ |
| `POST /upload` | 分享文件（multipart，part 自带真实文件名） | ✅ | ✅ |
| `GET /content/latest?json=1` | 拉取最新内容 | ✅ | ✅ |
| `GET /file/:uuid/:filename` | 下载文件 | ✅ | ✅ |

两种部署都支持，不需要额外配置。

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

依赖 [Cherri](https://github.com/electrikmilk/cherri)（**Go 写的，不是 Swift**）。
版本必须钉到 commit `dc82114f346f`，`build.sh` 会检查并在不一致时告警。

## 深入

判型是怎么做的、每个结论的实测数据、踩过哪些坑、哪些结论被推翻过——
都在 **[NOTES.md](./NOTES.md)**。README 只讲怎么用，那份讲为什么。
