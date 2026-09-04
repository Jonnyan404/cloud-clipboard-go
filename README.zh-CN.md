<h1 align="center"> Cloud Clipboard Go </h1>

<p align="center">
  <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue.svg" alt="English Readme"></a>
  <a href="https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go-launcher/main/LICENSE">
    <img src="https://img.shields.io/github/license/jonnyan404/cloud-clipboard-go-launcher?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/v/release/jonnyan404/cloud-clipboard-go?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/downloads/jonnyan404/cloud-clipboard-go/total?color=brightgreen&include_prereleases" alt="downloads">
  </a>
</p>

<p align="center">
  <a href="https://ko-fi.com/jonnyan404">
    <img src="https://ko-fi.com/img/githubbutton_sm.svg" alt="Buy Me a Coffee at ko-fi.com">
  </a>
</p>

<p align="center">
  <strong>一个跨平台的云剪贴板工具，支持文本、图片、文件实时发送到云端或本地服务器。</strong>
</p>

---

## 📸 截图预览

<details>
<summary><b>💻 桌面端</b></summary>

![Desktop Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/desktop.png)

</details>

<details>
<summary><b>📱 移动端</b></summary>

![Mobile Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/mobile.png)

</details>

<details>
<summary><b>📡 路由器</b></summary>

![OpenWrt Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/openwrt/demo.png)

</details>

---

## 🎯 优势特性

| 特性 | 说明 |
|------|------|
| 🔒 **隐私安全** | 可部署在本地或自有服务器，数据完全可控 |
| 📦 **易于部署** | 支持 Docker、源代码、二进制、Homebrew、OpenWrt等多种方式 |
| 🌍 **跨平台** | 支持 Windows、macOS、Linux、Android、iOS |
| ⚡ **高效同步** | 实时同步，无延迟 |
| 🔐 **认证保护** | 支持密码和 Token 认证 |
| 💾 **灵活存储** | 支持配置历史记录和文件过期时间 |
| 🚀 **轻量高效** | 资源占用少，即使在低配设备也能流畅运行 |
| 🔍 **快捷指令** | Android/iOS 快捷指令支持 |

---

## 🚀 快速开始

### 🐳 使用 Docker（推荐）

#### 极简启动
```bash
docker run -d \
  --name cloud-clipboard-go \
  -p 9501:9501 \
  -v /path/to/data:/app/server-node/data \
  jonnyan404/cloud-clipboard-go:latest
```

#### Docker Compose
<details open>
<summary><b>展开查看 docker-compose.yml 示例</b></summary>

```yaml
services:
  cloud-clipboard-go:
    image: ghcr.io/jonnyan404/cloud-clipboard-go:latest
    #image: jonnyan404/cloud-clipboard-go:latest
    container_name: cloud-clipboard-go
    restart: always
    ports:
      - "9501:9501"
    volumes:
      - /path/your/dir/data:/app/server-node/data # 数据持久化目录（请修改为本地目录）
    environment:
      LISTEN_IP: ${LISTEN_IP:-}                 # 默认为 0.0.0.0，可设置为 127.0.0.1 仅限本地访问
      LISTEN_IP6: ${LISTEN_IP6:-}               # 默认为空，IPv6 监听地址，可设置为 ::
      LISTEN_PORT: ${LISTEN_PORT:-}             # 服务监听端口，默认为 9501
      PREFIX: ${PREFIX:-}                       # 子路径反代前缀（配合 Nginx 使用），例如 /cloud-clipboard
      MESSAGE_NUM: ${MESSAGE_NUM:-}             # 历史记录保留条数，默认为 10
      AUTH_PASSWORD: ${AUTH_PASSWORD:-}         # 全局访问密码，留空即无需密码
      ROOM_AUTH_JSON: '${ROOM_AUTH_JSON:-{}}'   # 房间独立密码与策略 JSON，如 {"finance":"pass","keep":{"password":"kp","fileExpire":0}}
      TEXT_LIMIT: ${TEXT_LIMIT:-}               # 文本最大长度，默认为 4096（约 2048 个汉字）
      FILE_EXPIRE: ${FILE_EXPIRE:-}             # 上传文件过期时间（秒），默认为 3600（1小时），0 为不过期
      FILE_LIMIT: ${FILE_LIMIT:-}               # 上传文件大小限制（字节），默认为 104857600（100MB）
      ROOM_LIST: ${ROOM_LIST:-}                 # 是否在前端开启公开房间列表展示，默认 false
      MKCERT_DOMAIN_OR_IP: ${MKCERT_DOMAIN_OR_IP:-} # mkcert 域名/IP（自动生成自签证书），多个以空格分隔
      MANUAL_KEY_PATH: ${MANUAL_KEY_PATH:-}     # 自定义 SSL 私钥文件绝对路径（优先级高于 mkcert）
      MANUAL_CERT_PATH: ${MANUAL_CERT_PATH:-}   # 自定义 SSL 证书文件绝对路径（优先级高于 mkcert）
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 \"${LISTEN_PORT:-9501}\" || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

启动命令：
```bash
docker compose up -d
```

</details>

启动后在浏览器打开：`http://localhost:9501`

---

## 📦 其它部署方式

<details>
<summary><b>1️⃣ 二进制文件直接运行（Linux / macOS / Windows）</b></summary>

前往 [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases) 下载对应系统的预编译文件：

```bash
# Linux / macOS
./cloud-clipboard-go -port 9501 -auth mypassword123

# Windows
cloud-clipboard-go.exe -port 9501
```

**常用命令行参数**：
- `-host string`: 监听地址（默认 `0.0.0.0`）
- `-port int`: 监听端口（默认 `9501`）
- `-auth string`: 访问认证密码
- `-config string`: 配置文件路径（默认读取当前目录或系统目录配置）
- `-static string`: 外部静态文件目录路径
</details>

<details>
<summary><b>2️⃣ Homebrew（macOS）</b></summary>

```bash
brew install Jonnyan404/tap/cloud-clipboard-go
brew services start cloud-clipboard-go
```
> 默认配置文件路径: `/usr/local/etc/cloud-clipboard-go/config.json`
</details>

<details>
<summary><b>3️⃣ Android 手机一键开服</b></summary>

无需电脑，手机即是剪贴板服务器：
1. 前往 [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases) 下载 `.apk` 文件并在手机安装。
2. 打开 App，配置监听端口与密码后点击**“启动服务”**。
3. 同局域网内其他设备访问 `http://手机局域网IP:9501` 即可。
</details>

<details>
<summary><b>4️⃣ OpenWrt 路由器</b></summary>

```bash
# 查看架构信息
cat /etc/apk/arch
# OpenWrt 25.12+ (APK 包管理器)
apk add --allow-untrusted ./cloud-clipboard-<version>-<apk-arch>.apk
apk add --allow-untrusted ./luci-app-cloud-clipboard-<version>-noarch.apk

# OpenWrt 24.10 及以下 (OPKG 包管理器)
opkg install ./cloud-clipboard_<version>_<arch>.ipk
opkg install ./luci-app-cloud-clipboard_<version>_all.ipk
```
安装后可在 OpenWrt LuCI 管理后台中直接启用与配置。
</details>

<details>
<summary><b>5️⃣ Cloudflare Workers（云端免运维 Serverless）</b></summary>

基于 Cloudflare Workers + D1 数据库 + R2 存储，享受全球 CDN 加速与免费额度。

- 支持 github actions 部署
- 支持本地脚本部署

详见：[Cloudflare 部署指南](./cloudflare/README.md)
</details>

<details>
<summary><b>6️⃣ 源代码手动构建</b></summary>

> 前置依赖：Node.js >= 22.12、Go >= 1.25

```bash
# 1. 编译前端
cd web-vue3
npm install && npm run build

# 2. 编译并运行后端
cd ../cloud-clip
go mod tidy
go run -tags embed .
```
</details>

---

## 📱 客户端与辅助工具

| 工具 / 形式 | 支持平台 | 说明 |
| :--- | :--- | :--- |
| **Web 前端** | 全平台浏览器 | 内置开箱即用，响应式 UI，支持 PWA 安装至桌面 |
| **HTTP Shortcuts 快捷指令** | Android / iOS | 配合 [快捷指令包](./shortcuts/) 实现系统分享与快速发送 |
| **Clipboard Sync** | Win / Mac / Linux | 桌面双向静默剪贴板同步工具（捐赠用户专享） |
| **Cloud Clipboard Go Launcher** | Win / Mac / Linux | [图形化启动器](https://github.com/jonnyan404/cloud-clipboard-go-launcher)，无需接触命令行 |
| **Universal Shell** | Win / Mac / Linux | [图形化辅助运行工具](https://github.com/Jonnyan404/universal-shell/releases) |

---

## 🌐 API 接口示例

```bash
# 获取最新一条剪贴板内容
curl http://localhost:9501/content/latest

# 获取指定房间的最新内容
curl http://localhost:9501/content/latest?room=work
```

完整接口规范与配置参数说明请参考：📖 [配置文件与 API 文档](./cloud-clip/config.md)

---

## ☕ 支持项目

如果这个项目对你有帮助，欢迎通过以下方式支持我们：

### 💰 赞赏捐助

你的支持是我们继续维护和改进项目的动力！

| 方式 | 二维码 |
|------|--------|
| **微信** | <img src="https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/wechat.png" width="300" alt="微信赞赏码"> |



### 🌟 其他支持方式

- [【腾讯云】2核2G云服务器新老同享 99元/年，续费同价](https://cloud.tencent.com/act/cps/redirect?redirect=6150&cps_key=0b1dfaf9bb573dac05abef76202dc8cc&from=console)
- [【阿里云】2核2G云服务器新老同享 99元/年，续费同价](https://www.aliyun.com/daily-act/ecs/activity_selection?userCode=79h2wrag)
- ⭐ **Star 项目** - 如果觉得项目不错，请给个 Star
- 🐛 **报告问题** - 提交 Issues 帮助我们改进
- 💡 **提出建议** - 在 Discussions 中分享你的想法
- 🔀 **贡献代码** - 提交 Pull Requests 帮助项目发展
- 📢 **分享项目** - 告诉更多需要的人

### 📝 赞赏者名单

感谢以下用户的支持：

> 首先感谢赞赏,欢迎提 pr 的
- 🥇 DOYO（赞赏 ¥20）谢谢老大写的 clipboard go捏,很好用!想提 pr 了 (

- 🥈 xxxxxxxx（赞赏 ¥99）
- 🥉 xxxxxxxx（赞赏 ¥50）

> 如果你也想出现在这里，请在赞赏时备注你的名字或昵称！

---

## 🙏 致谢

本项目前端(client)和后端(cloud-clip) fork以下开源项目修改而来：

- [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard)
- [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard)

---

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE)
