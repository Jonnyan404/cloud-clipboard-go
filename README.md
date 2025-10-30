<h1 align="center"> Cloud Clipboard Go </h1>

<p align="center">
  <a href="README.en.md"><img src="https://img.shields.io/badge/lang-English-blue.svg" alt="English Readme"></a>
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
  <strong>一个跨平台的云剪贴板同步工具，支持文本、图片、文件实时同步到云端或本地服务器。</strong>
</p>

---

## 📸 截图预览

<details>
<summary><b>💻 桌面端</b></summary>

![Desktop Preview](https://ae01.alicdn.com/kf/Hfce3a9b69b3d404c8e3073ab0fffa913v.png)

</details>

<details>
<summary><b>📱 移动端</b></summary>

![Mobile Preview](https://ae01.alicdn.com/kf/Hbf859dd0e42c4406bf94a6b6f2f4658cf.png)

</details>

---

## ✨ 主要功能

- 🔄 **实时同步** - 剪贴板内容实时同步到云端
- 📝 **多种内容类型** - 支持文本、图片、文件上传
- 🌐 **多平台支持** - Windows、macOS、Linux、Android、iOS
- 🔐 **安全认证** - 支持密码/Token认证
- 🚀 **灵活部署** - Docker、源代码、二进制、Homebrew、OpenWrt
- 📦 **多端点支持** - 支持配置多个上传地址
- 💾 **历史记录** - 可配置的历史消息保留数量
- 🔍 **快捷指令** - Android/iOS 快捷指令支持

---

## 🚀 快速开始

### 1️⃣ 使用 Docker（最推荐）

```bash
# 方式一：Docker Compose（推荐）
docker-compose up -d

# 方式二：Docker 命令行
docker run -d \
  --name=cloud-clipboard-go \
  -p 9501:9501 \
  -v /path/to/data:/app/server-node/data \
  jonnyan404/cloud-clipboard-go
```

然后访问：`http://localhost:9501`

### 2️⃣ 使用二进制文件

前往 [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases) 下载对应平台的文件：

```bash
# Linux/macOS
./cloud-clipboard-go -port 9501

# Windows
cloud-clipboard-go.exe -port 9501
```

### 3️⃣ 使用 Android 应用（移动设备）

对于在 Android 手机/平板上直接部署服务器的场景：

1. 前往 [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases) 下载 `.apk` 文件
2. 在 Android 设备上安装 APK
3. 打开应用，设置监听端口（默认 9501）
4. 设置访问密码（可选）
5. 点击"启动服务"

然后在其他设备访问：`http://你的安卓设备IP:9501`

**优点**：
- 📱 无需电脑，在手机上直接运行服务器
- 🚀 开箱即用，无需额外依赖
- 💾 支持数据持久化

### 4️⃣ 使用 Homebrew（macOS）

```bash
brew install Jonnyan404/tap/cloud-clipboard-go
brew services start cloud-clipboard-go
```

### 5️⃣ 使用 OpenWrt（路由器）

```bash
opkg update
opkg install cloud-clipboard-go_*_platform.ipk
opkg install cloud-clipboard-go_*_all.ipk
```

### 6️⃣ 从源代码构建

```bash
# 前置要求：Node.js >= 22.12、Go >= 1.22

# 1. 构建前端
cd client
npm install
npm run build

# 2. 运行后端
cd ../cloud-clip
go mod tidy
go run -tags embed .
```

---

## 📋 部署指南

### Docker Compose 配置

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  cloud-clipboard-go:
    image: jonnyan404/cloud-clipboard-go:latest
    container_name: cloud-clipboard-go
    restart: always
    ports:
      - "9501:9501"
    environment:
      - LISTEN_IP= #默认为0.0.0.0,可设置为 127.0.0.1 不懂勿动
      - LISTEN_IP6= #默认为空,ipv6地址,可设置为::,不懂勿动
      - LISTEN_PORT= #默认为9501,可设置为其他端口
      - PREFIX= #子路径,可配合nginx使用,格式: /cloud-clipboard
      - MESSAGE_NUM= #历史记录的数量,默认为10
      - AUTH_PASSWORD= #访问密码,默认为false,可自定义字符串密码
      - TEXT_LIMIT= #文本长度限制,默认为4096(2048个汉字),可设置为其他长度
      - FILE_EXPIRE= #文件过期时间,默认为3600(1小时),可设置为其他时间,单位为秒
      - FILE_LIMIT= #文件大小限制,默认为104857600(100MB),可设置为其他大小,单位为字节
      - MKCERT_DOMAIN_OR_IP= #mkcert域名或IP,默认为空,可设置为其他域名或IP,多个用空格分隔,仅域名支持通配符*
      - MANUAL_KEY_PATH= #手动设置证书路径,默认为空,该参数优先级高于MKCERT_DOMAIN_OR_IP
      - MANUAL_CERT_PATH= #手动设置证书路径,默认为空,该参数优先级高于MKCERT_DOMAIN_OR_IP
      - ROOM_LIST= #是否启用房间列表展示功能,默认false
    volumes:
      - ./data:/app/server-node/data  # 数据持久化
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9501"]
      interval: 30s
      timeout: 10s
      retries: 3
```

运行：

```bash
docker-compose up -d
```

### 二进制文件参数

```bash
# 参数优先级：命令行 > 配置文件 > 默认值

-host string
    服务器监听地址 (默认 "0.0.0.0")

-port int
    服务器监听端口 (默认 9501)

-auth string
    访问密码

-config string
    配置文件路径

-static string
    外部前端文件路径
```

示例：

```bash
./cloud-clipboard-go -host 127.0.0.1 -port 8080 -auth mypassword123
```

---

## 📱 客户端使用

### 📲 Android 快捷指令

1. 下载 [HTTP Shortcuts](https://github.com/Waboodoo/HTTP-Shortcuts/releases)
2. 下载 [快捷指令文件](https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go/refs/heads/main/shortcuts/cloud-clipboard-shortcuts.zip)
3. 在 HTTP Shortcuts 中导入文件
4. 配置变量：
   - `url`: 你的服务器地址 (如：`http://192.168.1.100:9501`)
   - `room`: 房间名称（可选）
   - `auth`: 认证密码（可选）

### 🖥️ 桌面端应用

- **Clipboard Monitor**（推荐）
  - 自动监控剪贴板
  - 支持 Windows/macOS/Linux
  - 详见：[clipboard-monitor 文档](./clipboard-monitor/README.md)

### 💻 UI 辅助工具

下载 [Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher/releases)，无需命令行操作。

---

## 🌐 API 接口

### 获取最新内容

```bash
GET /content/latest
```

返回最新的一条剪贴板内容。

**参数**：
- `room` (可选)：房间名称

**示例**：

```bash
curl http://localhost:9501/content/latest
curl http://localhost:9501/content/latest?room=work
```

完整 API 文档：[API.md](./cloud-clip/config.md)

---

## 🐳 Docker 镜像

### 镜像来源

| 来源 | 仓库 |
|------|------|
| Docker Hub | `jonnyan404/cloud-clipboard-go` |
| GitHub Container Registry | `ghcr.io/jonnyan404/cloud-clipboard-go` |

### 拉取最新镜像

```bash
docker pull jonnyan404/cloud-clipboard-go:latest
```

---

## 📚 详细文档

- 📖 [配置文件说明](./cloud-clip/config.md)
- 🔌 [HTTP API 文档](./cloud-clip/config.md)
- 🖥️ [Clipboard Monitor 文档](./clipboard-monitor/README.md)
- 📱 [客户端部署指南](#-客户端使用)

---

## 🔄 支持的平台

| 平台 | 二进制 | Docker | 源代码 | 说明 |
|------|---------|--------|--------|------|
| Linux | ✅ | ✅ | ✅ | 主要支持 |
| macOS | ✅ | ✅ | ✅ | Intel/Apple Silicon |
| Windows | ✅ | ✅ | ✅ | 需要 Visual C++ Build Tools |
| Android | ✅ | - | - | APK/快捷指令 |
| iOS | - | - | - | 快捷指令 |
| OpenWrt | ✅ | - | ✅ | 路由器系统 |

---

## 🐛 故障排除

### Docker 容器无法启动

```bash
# 查看日志
docker logs cloud-clipboard-go

# 检查端口是否被占用
netstat -tuln | grep 9501

# 重启容器
docker restart cloud-clipboard-go
```

### 无法访问 Web 界面

- 检查防火墙是否阻止了 9501 端口
- 确认容器正在运行：`docker ps | grep cloud-clipboard-go`
- 尝试本地访问：`http://localhost:9501`

### 文件上传失败

- 检查磁盘空间是否充足
- 检查 `FILE_LIMIT` 环境变量设置
- 确保数据目录有写入权限：`chmod 777 ./data`

详见：[完整故障排除指南](./docs/troubleshooting.md)

---

## 🎯 优势特性

| 特性 | 说明 |
|------|------|
| 🔒 **隐私安全** | 可部署在本地或自有服务器，数据完全可控 |
| 📦 **易于部署** | 支持 Docker、二进制、源代码等多种方式 |
| 🌍 **跨平台** | 支持 Windows、macOS、Linux、Android、iOS |
| ⚡ **高效同步** | 实时同步，无延迟 |
| 🔐 **认证保护** | 支持密码和 Token 认证 |
| 💾 **灵活存储** | 支持配置历史记录和文件过期时间 |
| 🚀 **轻量高效** | 资源占用少，即使在低配设备也能流畅运行 |

---

## 📦 衍生项目

- **[Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher)** - UI 辅助工具，方便不使用终端的用户
- **[Clipboard Monitor](./clipboard-monitor/)** - 桌面端监控应用

---

## 🙏 致谢

本项目基于以下开源项目开发：

- [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard)
- [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard)

---

## 📊 Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=Jonnyan404/cloud-clipboard-go&type=Date)](https://www.star-history.com/#Jonnyan404/cloud-clipboard-go&Date)

---

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE)

---

## 💬 交流反馈

- 📝 提交 [Issues](https://github.com/jonnyan404/cloud-clipboard-go/issues)
- 🔀 贡献 [Pull Requests](https://github.com/jonnyan404/cloud-clipboard-go/pulls)
- 💡 讨论 [Discussions](https://github.com/jonnyan404/cloud-clipboard-go/discussions)

---

**最后更新**: 2025年10月30日 | 📖 [English Version](README.en.md)
