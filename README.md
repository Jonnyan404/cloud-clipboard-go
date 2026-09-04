<h1 align="center"> Cloud Clipboard Go </h1>

<p align="center">
  <a href="README.zh-CN.md"><img src="https://img.shields.io/badge/lang-简体中文-blue.svg" alt="Chinese Readme"></a>
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
  <strong>A cross-platform cloud clipboard tool for sending text, images, and files in real-time across multiple devices and platforms.</strong>
</p>

---

## 📸 Screenshots

<details>
<summary><b>💻 Desktop</b></summary>

![Desktop Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/desktop.png)

</details>

<details>
<summary><b>📱 Mobile</b></summary>

![Mobile Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/mobile.png)

</details>

<details>
<summary><b>📡 Router (OpenWrt)</b></summary>

![OpenWrt Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/openwrt/demo.png)

</details>

---

## 🎯 Highlights

| Feature | Description |
|---|---|
| 🔒 **Privacy & Security** | Deploy on your local machine or private server; keep full ownership of your data |
| 📦 **Easy Deployment** | Supports Docker, binaries, Homebrew, OpenWrt, Serverless, and source builds |
| 🌍 **Cross-platform** | Available for Windows, macOS, Linux, Android, and iOS |
| ⚡ **Real-time Sync** | Instant bidirectional synchronization via WebSocket |
| 🔐 **Authentication** | Password protection and per-room access control |
| 💾 **Flexible Storage** | Configurable history capacity and file expiration policies |
| 🚀 **Lightweight** | Minimal resource consumption, runs smoothly even on routers or low-spec hardware |
| 🔍 **Shortcuts Support** | Android and iOS shortcut integration for one-tap sharing |

---

## 🚀 Quick Start

### 🐳 Using Docker (Recommended)

#### Quick Run
```bash
docker run -d \
  --name cloud-clipboard-go \
  -p 9501:9501 \
  -v /path/to/data:/app/server-node/data \
  jonnyan404/cloud-clipboard-go:latest
```

#### Docker Compose
<details open>
<summary><b>Expand to view docker-compose.yml example</b></summary>

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
      - /path/your/dir/data:/app/server-node/data # Data persistence directory (modify to your local path)
    environment:
      LISTEN_IP: ${LISTEN_IP:-}                 # Listening IPv4 address, default 0.0.0.0 (use 127.0.0.1 for local only)
      LISTEN_IP6: ${LISTEN_IP6:-}               # Listening IPv6 address, default empty (use :: for IPv6)
      LISTEN_PORT: ${LISTEN_PORT:-}             # Server port, default 9501
      PREFIX: ${PREFIX:-}                       # Subpath prefix for reverse proxies (e.g., /cloud-clipboard)
      MESSAGE_NUM: ${MESSAGE_NUM:-}             # Number of history records to keep, default 10
      AUTH_PASSWORD: ${AUTH_PASSWORD:-}         # Global access password, leave empty for no password
      ROOM_AUTH_JSON: '${ROOM_AUTH_JSON:-{}}'   # Room-level auth and policy JSON, e.g. {"finance":"pass","keep":{"password":"kp","fileExpire":0}}
      TEXT_LIMIT: ${TEXT_LIMIT:-}               # Max text length in characters, default 4096 (approx. 2048 Chinese characters)
      FILE_EXPIRE: ${FILE_EXPIRE:-}             # File retention period in seconds, default 3600 (1 hour), 0 for no expiration
      FILE_LIMIT: ${FILE_LIMIT:-}               # Max file size in bytes, default 104857600 (100MB)
      ROOM_LIST: ${ROOM_LIST:-}                 # Display public room list in UI, default false
      MKCERT_DOMAIN_OR_IP: ${MKCERT_DOMAIN_OR_IP:-} # Domains/IPs for automatic mkcert SSL certs (space separated)
      MANUAL_KEY_PATH: ${MANUAL_KEY_PATH:-}     # Manual SSL private key path (overrides mkcert)
      MANUAL_CERT_PATH: ${MANUAL_CERT_PATH:-}   # Manual SSL certificate path (overrides mkcert)
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 \"${LISTEN_PORT:-9501}\" || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

Run:
```bash
docker compose up -d
```

</details>

Then open in your browser: `http://localhost:9501`

---

## 📦 Other Deployment Methods

<details>
<summary><b>1️⃣ Standalone Binaries (Linux / macOS / Windows)</b></summary>

Download precompiled binaries for your platform from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases):

```bash
# Linux / macOS
./cloud-clipboard-go -port 9501 -auth mypassword123

# Windows
cloud-clipboard-go.exe -port 9501
```

**Common CLI flags**:
- `-host string`: Listening host (default `0.0.0.0`)
- `-port int`: Listening port (default `9501`)
- `-auth string`: Access authentication password
- `-config string`: Path to configuration file
- `-static string`: Path to external static assets directory
</details>

<details>
<summary><b>2️⃣ Homebrew (macOS)</b></summary>

```bash
brew install Jonnyan404/tap/cloud-clipboard-go
brew services start cloud-clipboard-go
```
> Default config path: `/usr/local/etc/cloud-clipboard-go/config.json`
</details>

<details>
<summary><b>3️⃣ Android Host (Run Server on Phone)</b></summary>

No PC required; your mobile device acts as the clipboard server:
1. Download and install the `.apk` from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases).
2. Open the app, configure the port/password, and tap **"Start Service"**.
3. Access `http://phone-lan-ip:9501` from any device on the same local network.
</details>

<details>
<summary><b>4️⃣ OpenWrt Router</b></summary>

```bash
# Check your router architecture
cat /etc/apk/arch

# OpenWrt 25.12+ (APK package manager)
apk add --allow-untrusted ./cloud-clipboard-<version>-<apk-arch>.apk
apk add --allow-untrusted ./luci-app-cloud-clipboard-<version>-noarch.apk

# OpenWrt 24.10 and earlier (OPKG package manager)
opkg install ./cloud-clipboard_<version>_<arch>.ipk
opkg install ./luci-app-cloud-clipboard_<version>_all.ipk
```
Manage and configure directly in the OpenWrt LuCI WebUI.
</details>

<details>
<summary><b>5️⃣ Cloudflare Workers (Serverless Cloud Hosting)</b></summary>

Built on Cloudflare Workers + D1 database + R2 storage with global CDN acceleration and generous free tiers.

- Supports GitHub Actions automated deployments
- Supports local script deployment

See detailed instructions: [Cloudflare Deployment Guide](./cloudflare/README.md)
</details>

<details>
<summary><b>6️⃣ Build from Source</b></summary>

> Prerequisites: Node.js >= 22.12, Go >= 1.25

```bash
# 1. Build frontend
cd web-vue3
npm install && npm run build

# 2. Build and run backend
cd ../cloud-clip
go mod tidy
go run -tags embed .
```
</details>

---

## 📱 Clients & Utilities

| Tool / Type | Supported Platforms | Description |
| :--- | :--- | :--- |
| **Web UI** | Modern Browsers | Built-in responsive web app with PWA desktop installation support |
| **HTTP Shortcuts** | Android / iOS | Send & receive clipboard content via system share sheet with [shortcuts package](./shortcuts/) |
| **Clipboard Sync** | Win / Mac / Linux | Silent bidirectional clipboard synchronization desktop app (donors only) |
| **Cloud Clipboard Go Launcher** | Win / Mac / Linux | [Desktop GUI launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher) without terminal |
| **Universal Shell** | Win / Mac / Linux | [GUI utility helper](https://github.com/Jonnyan404/universal-shell/releases) |

---

## 🌐 API Examples

```bash
# Get latest clipboard entry
curl http://localhost:9501/content/latest

# Get latest clipboard entry from a specific room
curl http://localhost:9501/content/latest?room=work
```

For full API specifications and configuration schema: 📖 [Configuration & API Reference](./cloud-clip/config.md)

---

## ☕ Support the Project

If this project helps you, consider supporting us:

### 💰 Sponsorship & Donations

Your support keeps this project maintained and actively developed!

| Method | QR Code |
|---|---|
| **WeChat Pay** | <img src="https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/wechat.png" width="280" alt="WeChat Pay"> |

### 🌟 Other Ways to Support

- [Tencent Cloud 2C2G from ¥99/yr](https://cloud.tencent.com/act/cps/redirect?redirect=6150&cps_key=0b1dfaf9bb573dac05abef76202dc8cc&from=console)
- [Alibaba Cloud 2C2G from ¥99/yr](https://www.aliyun.com/daily-act/ecs/activity_selection?userCode=79h2wrag)
- ⭐ **Star this repository** on GitHub
- 🐛 **Report issues** to help us improve
- 💡 **Suggest features** in GitHub Discussions
- 🔀 **Submit Pull Requests** to contribute code
- 📢 **Share the project** with your friends and colleagues

### 📝 Donors & Supporters

Special thanks to:
- 🥇 DOYO (¥20)
- 🥈 xxxxxxxx (¥99)
- 🥉 xxxxxxxx (¥50)

*(Leave your nickname in the donation note if you want to be listed here!)*

---

## 🙏 Acknowledgments

This project's frontend and backend are forked and evolved from the following open-source works:

- [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard)
- [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard)

---

## 📄 License

MIT License - See [LICENSE](LICENSE) for details.