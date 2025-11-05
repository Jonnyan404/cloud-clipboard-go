# Cloud Clipboard Go

<p align="center">
  <a href="README.md"><img src="https://img.shields.io/badge/lang-简体中文-blue.svg" alt="中文 Readme"></a>
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
  <strong>A cross-platform cloud clipboard synchronization tool that supports real-time synchronization of text, images, and files to cloud or local servers.</strong>
</p>

---

## 📸 Screenshots

<details>
<summary><b>💻 Desktop</b></summary>

![Desktop Preview](https://ae01.alicdn.com/kf/Hfce3a9b69b3d404c8e3073ab0fffa913v.png)

</details>

<details>
<summary><b>📱 Mobile</b></summary>

![Mobile Preview](https://ae01.alicdn.com/kf/Hbf859dd0e42c4406bf94a6b6f2f4658cf.png)

</details>


---

## 🎯 Advantages

| Feature | Description |
|---------|-------------|
| 🔒 **Privacy** | Deploy locally or on your own server, full data control |
| 📦 **Easy Deploy** | Docker, Binary, Source code, Homebrew, OpenWrt, and more options |
| 🌍 **Cross-platform** | Windows, macOS, Linux, Android, iOS |
| ⚡ **Fast Sync** | Real-time synchronization, zero delay |
| 🔐 **Security** | Password and Token authentication support |
| 💾 **Flexible Storage** | Configurable history and file expiration |
| 🚀 **Lightweight** | Low resource usage, runs smoothly on low-end devices |
| 🔍 **Shortcuts** | Android/iOS shortcuts support |

---

## 🚀 Quick Start

### 1️⃣ Docker (Recommended)

```bash
# Option 1: Docker Compose (Recommended)
docker-compose up -d

# Option 2: Docker CLI
docker run -d \
  --name=cloud-clipboard-go \
  -p 9501:9501 \
  -v /path/to/data:/app/server-node/data \
  jonnyan404/cloud-clipboard-go
```

Then visit: `http://localhost:9501`

### 2️⃣ Binary Files

Download from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases):

```bash
# Linux/macOS
./cloud-clipboard-go -port 9501

# Windows
cloud-clipboard-go.exe -port 9501
```

### 3️⃣ Android Application (Mobile Devices)

For deploying server directly on Android phone/tablet:

1. Download `.apk` file from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases)
2. Install APK on your Android device
3. Open the app and set listening port (default 9501)
4. Set access password (optional)
5. Tap "Start Service"

Then access from other devices: `http://your-android-device-ip:9501`

**Advantages**:
- 📱 Run server directly on your phone without a computer
- 🚀 Ready to use, no additional dependencies
- 💾 Data persistence support

### 4️⃣ Homebrew (macOS)

```bash
brew install Jonnyan404/tap/cloud-clipboard-go
brew services start cloud-clipboard-go
```

### 5️⃣ OpenWrt (Router)

```bash
opkg update
opkg install cloud-clipboard-go_*_platform.ipk
opkg install cloud-clipboard-go_*_all.ipk
```

### 6️⃣ Build from Source

```bash
# Requirements: Node.js >= 22.12, Go >= 1.22

# 1. Build frontend
cd client
npm install
npm run build

# 2. Run backend
cd ../cloud-clip
go mod tidy
go run -tags embed .
```

### 7️⃣ Cloudflare Deployment

For cloud deployment scenarios, support one-click deployment to Cloudflare Workers + Pages:

```bash
# Requirements: Node.js >= 22.12, Wrangler CLI

# 1. Install Wrangler CLI
npm install -g wrangler

# 2. Login to Cloudflare
wrangler login

# 3. Run deployment script
cd cloudflare
./deploy.sh
```

**Deployment includes**:
- Cloudflare Workers (API backend)
- Cloudflare D1 (database)
- Cloudflare R2 (file storage)
- Cloudflare Pages (frontend interface)

**Advantages**:
- 🌐 Global CDN acceleration
- 🚀 No server maintenance required
- 💾 Automatic backup and scaling
- 🔒 Cloudflare security protection

**Important Notes**:
- Requires Cloudflare account
- Free tier usage (Workers: 100,000 requests/day, D1: 500MB storage, R2: 10GB storage)
- Access URL will be displayed after deployment

See: [Cloudflare Deployment Documentation](./cloudflare/README.md)

---

## 📋 Deployment Guide

### Docker Compose Configuration

Create `docker-compose.yml`:

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
      - LISTEN_IP= # Default is 0.0.0.0, can be set to 127.0.0.1. Don't change if unsure.
      - LISTEN_IP6= # Default is empty, can be set to ::. Don't change if unsure.
      - LISTEN_PORT= # Default is 9501, can be set to other ports.
      - PREFIX= # Subpath, can be used with nginx, format: /cloud-clipboard
      - MESSAGE_NUM= # Number of history records, default is 10.
      - AUTH_PASSWORD= # Access password, default is false, can be a custom string password.
      - TEXT_LIMIT= # Text length limit, default is 4096 (2048 Chinese characters).
      - FILE_EXPIRE= # File expiration time, default is 3600 (1 hour), unit is seconds.
      - FILE_LIMIT= # File size limit, default is 104857600 (100MB), unit is bytes.
      - MKCERT_DOMAIN_OR_IP= # The domain name or IP address for mkcert, defaults to empty. You can set it to other domain names or IPs. Multiple values can be separated by spaces. Wildcards are supported for domain names only.
      - MANUAL_KEY_PATH= # Manually set the path for the key. Defaults to empty. This parameter has higher priority than MKCERT_DOMAIN_OR_IP.
      - MANUAL_CERT_PATH= # Manually set the path for the certificate. Defaults to empty. This parameter has higher priority than MKCERT_DOMAIN_OR_IP.
      - ROOM_LIST= #是否启用房间列表展示功能,默认false
    volumes:
      - ./data:/app/server-node/data  # Data persistence
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9501"]
      interval: 30s
      timeout: 10s
      retries: 3
```

Run:

```bash
docker-compose up -d
```

### Binary Command-line Parameters

```bash
# Priority: Command-line > Config file > Default values

-host string
    Server listening address (default "0.0.0.0")

-port int
    Server listening port (default 9501)

-auth string
    Access password

-config string
    Configuration file path

-static string
    External frontend file path
```

Example:

```bash
./cloud-clipboard-go -host 127.0.0.1 -port 8080 -auth mypassword123
```

---

## 📱 Client Usage

### 📲 Android Shortcuts

1. Download [HTTP Shortcuts](https://github.com/Waboodoo/HTTP-Shortcuts/releases)
2. Download [Shortcuts file](https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go/refs/heads/main/shortcuts/cloud-clipboard-shortcuts.zip)
3. Import into HTTP Shortcuts
4. Configure variables:
   - `url`: Your server address (e.g., `http://192.168.1.100:9501`)
   - `room`: Room name (optional)
   - `auth`: Authentication password (optional)

### 🖥️ Desktop Application

- **Clipboard Monitor** (Recommended)
  - Auto clipboard monitoring
  - Supports Windows/macOS/Linux
  - See: [Clipboard Monitor Documentation](./clipboard-monitor/README.md)

### 💻 UI Launcher Tool

Download [Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher/releases) - no command line needed.

---

## 🌐 API Endpoints

### Get Latest Content

```bash
GET /content/latest
```

Returns the latest clipboard content.

**Parameters**:
- `room` (optional): Room name

**Examples**:

```bash
curl http://localhost:9501/content/latest
curl http://localhost:9501/content/latest?room=work
```

Full API Documentation: [API.md](./cloud-clip/config.md)

---

## 🐳 Docker Images

### Image Sources

| Source | Repository |
|--------|------------|
| Docker Hub | `jonnyan404/cloud-clipboard-go` |
| GitHub Container Registry | `ghcr.io/jonnyan404/cloud-clipboard-go` |

### Pull Latest Image

```bash
docker pull jonnyan404/cloud-clipboard-go:latest
```

---

## 📚 Documentation

- 📖 [Configuration Guide](./cloud-clip/config.md)
- 🔌 [HTTP API Documentation](./cloud-clip/config.md)
- 🖥️ [Clipboard Monitor Guide](./clipboard-monitor/README.md)
- 📱 [Client Deployment](#-client-usage)

---

## 🔄 Supported Platforms

| Platform | Binary | Docker | Source | Notes |
|----------|--------|--------|--------|-------|
| Linux | ✅ | ✅ | ✅ | Primary support |
| macOS | ✅ | ✅ | ✅ | Intel/Apple Silicon |
| Windows | ✅ | ✅ | ✅ | Requires Visual C++ Build Tools |
| Android | ✅ | - | ✅ | Server APK/Shortcuts |
| iOS | - | - | - | Shortcuts |
| OpenWrt | ✅ | - | ✅ | Router systems |

---

## 🐛 Troubleshooting

### Docker Container Won't Start

```bash
# Check logs
docker logs cloud-clipboard-go

# Check if port is in use
netstat -tuln | grep 9501

# Restart container
docker restart cloud-clipboard-go
```

### Can't Access Web Interface

- Check firewall isn't blocking port 9501
- Verify container is running: `docker ps | grep cloud-clipboard-go`
- Try local access: `http://localhost:9501`

### File Upload Fails

- Check disk space availability
- Verify `FILE_LIMIT` environment variable setting
- Ensure data directory is writable: `chmod 777 ./data`

Full Guide: [Troubleshooting](./docs/troubleshooting.md)



## 📦 Related Projects

- **[Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher)** - UI launcher tool for easier usage
- **[Clipboard Monitor](./clipboard-monitor/)** - Desktop monitoring application

---

## 🙏 Acknowledgments

This project is based on:

- [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard)
- [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard)

---

## 📊 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Jonnyan404/cloud-clipboard-go&type=Date)](https://www.star-history.com/#Jonnyan404/cloud-clipboard-go&Date)

---

## 📄 License

MIT License - See [LICENSE](LICENSE) for details

---

## ☕ Support the Project

If this project has been helpful to you, please consider supporting us:

### 💰 Donation

Your support motivates us to continue maintaining and improving this project!

| Method | QR Code |
|--------|---------|
| **WeChat** | <img src="https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/wechat.png" width="300" alt="WeChat"> |

### 🌟 Other Ways to Support

- ⭐ **Star** - Give us a star if you like the project
- 🐛 **Report Issues** - Help us improve by reporting bugs
- 💡 **Suggestions** - Share your ideas in Discussions
- 🔀 **Contribute Code** - Submit Pull Requests
- 📢 **Share** - Tell others about this project

### 📝 Supporters

Thanks to those who have supported us:

- 🥇 xxxxxxxx (¥199)
- 🥈 xxxxxxxx (¥99)
- 🥉 xxxxxxxx (¥50)

> If you'd like to appear here, please leave your name or nickname when donating!

---

## 💬 Community & Feedback

- 📝 Report [Issues](https://github.com/jonnyan404/cloud-clipboard-go/issues)
- 🔀 Submit [Pull Requests](https://github.com/jonnyan404/cloud-clipboard-go/pulls)
- 💡 Join [Discussions](https://github.com/jonnyan404/cloud-clipboard-go/discussions)

---

**Last Updated**: October 30, 2025 | 📖 [中文版本](README.md)
