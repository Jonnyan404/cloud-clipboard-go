<h1 align="center"> Cloud Clipboard Go </h1>

<p align="center">
  <a href="README.md"><img src="https://img.shields.io/badge/lang-简体中文-blue.svg" alt="Chinese Readme"></a>
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
  <strong>A cross-platform cloud clipboard tool for sending text, images, and files to a cloud or local server in real time.</strong>
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
<summary><b>📡 Router</b></summary>

![OpenWrt Preview](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/openwrt/demo.png)

</details>

---

## 🎯 Highlights

| Feature | Description |
|---------|-------------|
| 🔒 **Privacy & Security** | Deploy locally or on your own server and keep full control of your data |
| 📦 **Easy Deployment** | Supports Docker, source builds, binaries, Homebrew, OpenWrt, and more |
| 🌍 **Cross-platform** | Works on Windows, macOS, Linux, Android, and iOS |
| ⚡ **Real-time Sync** | Instant synchronization with no noticeable delay |
| 🔐 **Authentication** | Supports password and token-based protection |
| 💾 **Flexible Storage** | Configurable history retention and file expiration |
| 🚀 **Lightweight** | Low resource usage, suitable even for low-spec devices |
| 🔍 **Shortcuts Support** | Android and iOS shortcuts are supported |

---

## 🚀 Quick Start

### 1️⃣ Docker (Recommended)

- [Tencent Cloud: 2 vCPU / 2 GB server from CNY 99/year](https://cloud.tencent.com/act/cps/redirect?redirect=6150&cps_key=0b1dfaf9bb573dac05abef76202dc8cc&from=console)
- [Alibaba Cloud: 2 vCPU / 2 GB server from CNY 99/year](https://www.aliyun.com/daily-act/ecs/activity_selection?userCode=79h2wrag)

```bash
# Option 1: Docker Compose (Recommended)
docker compose up -d

# Option 2: Docker CLI
docker run -d \
  --name=cloud-clipboard-go \
  -p 9501:9501 \
  -e AUTH_PASSWORD='global-pass' \
  -e ROOM_AUTH_JSON='{"finance":"finance-pass","private":""}' \
  -v /path/to/data:/app/server-node/data \
  jonnyan404/cloud-clipboard-go
```

Then visit: `http://localhost:9501`

### 2️⃣ Binary Files

Download the package for your platform from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases):

```bash
# Linux/macOS
./cloud-clipboard-go -port 9501

# Windows
cloud-clipboard-go.exe -port 9501
```

### 3️⃣ Android App (Mobile Devices)

For running the server directly on an Android phone or tablet:

1. Download the `.apk` file from [Releases](https://github.com/jonnyan404/cloud-clipboard-go/releases)
2. Install the APK on your Android device
3. Open the app and set the listening port, default `9501`
4. Set an access password if needed
5. Tap `Start Service`

Then open it from another device: `http://your-android-device-ip:9501`

**Advantages**:
- 📱 Run the server directly on your phone without a computer
- 🚀 Ready to use with no extra dependencies
- 💾 Persistent local data storage

### 4️⃣ Homebrew (macOS)

```bash
brew install Jonnyan404/tap/cloud-clipboard-go
brew services start cloud-clipboard-go
```

### 5️⃣ OpenWrt (Routers)

```bash
# OpenWrt 25.12+
cat /etc/apk/arch
apk add --allow-untrusted ./cloud-clipboard-<version>-<apk-arch>.apk
apk add --allow-untrusted ./luci-app-cloud-clipboard-<version>-noarch.apk

# OpenWrt 24.10 and earlier
opkg install ./cloud-clipboard_<version>_<arch>.ipk
opkg install ./luci-app-cloud-clipboard_<version>_all.ipk
```

### 6️⃣ Build from Source

```bash
# Requirements: Node.js >= 22.12, Go >= 1.22

# 1. Build the frontend
cd client
npm install
npm run build

# 2. Run the backend
cd ../cloud-clip
go mod tidy
go run -tags embed .
```

### 7️⃣ Cloudflare Deployment

For cloud hosting, you can deploy to Cloudflare Workers + Pages in one go:

```bash
# Requirements: Node.js >= 22.12, Wrangler CLI

# 1. Install Wrangler CLI
npm install -g wrangler

# 2. Log in to Cloudflare
wrangler login

# 3. Run the deployment script
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
- 🚀 No server maintenance
- 💾 Automatic backup and scaling
- 🔒 Cloudflare security protection

**Notes**:
- A Cloudflare account is required
- Can be used within the free tier limits (Workers: 100,000 requests/day, D1: 500 MB storage, R2: 10 GB storage)
- The access URL is displayed after deployment

See also: [Cloudflare Deployment Documentation](./cloudflare/README.md)

---

## 📋 Deployment Guide

### Docker Compose Configuration

Use the existing `docker-compose.yml` in the repository root as the source of truth. The image entrypoint generates the runtime configuration from these environment variables, so the documentation should follow the same naming.

Adjust the root `docker-compose.yml` as needed:

```yaml
services:
  cloud-clipboard-go:
    container_name: cloud-clipboard-go
    restart: always
    ports:
      - "9501:9501"
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 \"${LISTEN_PORT:-9501}\" || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    environment:
      LISTEN_IP: ${LISTEN_IP:-} # Defaults to 0.0.0.0. Leave unchanged unless you know you need 127.0.0.1.
      LISTEN_IP6: ${LISTEN_IP6:-} # Defaults to empty. Set :: for IPv6 if needed.
      LISTEN_PORT: ${LISTEN_PORT:-} # Defaults to 9501. Change if you need another port.
      PREFIX: ${PREFIX:-} # Subpath, useful with nginx. Example: /cloud-clipboard
      MESSAGE_NUM: ${MESSAGE_NUM:-} # History record count, default 10.
      AUTH_PASSWORD: ${AUTH_PASSWORD:-} # Global access password. Default false.
      ROOM_AUTH_JSON: '${ROOM_AUTH_JSON:-{}}' # Room password JSON, for example {"finance":"finance-pass","ops":""}
      TEXT_LIMIT: ${TEXT_LIMIT:-} # Text length limit, default 4096.
      FILE_EXPIRE: ${FILE_EXPIRE:-} # File expiration in seconds, default 3600.
      FILE_LIMIT: ${FILE_LIMIT:-} # File size limit in bytes, default 104857600.
      MKCERT_DOMAIN_OR_IP: ${MKCERT_DOMAIN_OR_IP:-} # mkcert domain or IP. Separate multiple values with spaces. Wildcards are supported for domains only.
      MANUAL_KEY_PATH: ${MANUAL_KEY_PATH:-} # Manual key path. Higher priority than MKCERT_DOMAIN_OR_IP.
      MANUAL_CERT_PATH: ${MANUAL_CERT_PATH:-} # Manual certificate path. Higher priority than MKCERT_DOMAIN_OR_IP.
      ROOM_LIST: ${ROOM_LIST:-} # Whether to enable room list display. Default false.
    volumes:
      - /path/your/dir/data:/app/server-node/data # Replace with your own directory
    image: jonnyan404/cloud-clipboard-go:latest
```

Run:

```bash
docker compose up -d
```

`ROOM_AUTH_JSON` must be a valid JSON object. If a room value is an empty string, that room falls back to `AUTH_PASSWORD`.

Additional notes:

- The Docker Compose variable name is now standardized as `ROOM_AUTH_JSON`.
- The entrypoint still accepts the legacy variable name `ROOM_AUTH`, but the Compose example and later docs use `ROOM_AUTH_JSON` consistently.
- If you use a `.env` file, keep the same `${VAR:-}` mapping and only fill in the actual values.
- The image explicitly installs `nc`, and the Compose health check only verifies the listening port inside the container. It is unrelated to `PREFIX` or HTTP/HTTPS settings.

Example:

```yaml
environment:
  AUTH_PASSWORD: 'global-pass'
  ROOM_AUTH_JSON: '{"finance":"finance-pass","private":""}'
```

If you want to change `roomAuth` through variables dynamically, use a `.env` file:

```env
AUTH_PASSWORD=global-pass
ROOM_AUTH_JSON={"finance":"finance-pass","private":""}
```

Then run:

```bash
docker compose up -d
```

You can also override it temporarily:

```bash
ROOM_AUTH_JSON='{"finance":"new-pass","ops":"ops-pass"}' docker compose up -d
```

Note: the Docker image only auto-generates [cloud-clip/config.json](./cloud-clip/config.json) when it does not already exist. If you mounted an existing `config.json`, updating environment variables will not rewrite it automatically. In that case, delete the file and recreate the container, or edit `server.roomAuth` manually.

### Binary Command-line Parameters

```bash
# Priority: command line > config file > default value

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
2. Download the [shortcuts package](https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go/refs/heads/main/shortcuts/cloud-clipboard-shortcuts.zip)
3. Import it into HTTP Shortcuts
4. Configure the variables:
   - `url`: your server address, for example `http://192.168.1.100:9501`
   - `room`: room name, optional
   - `auth`: authentication password, optional

### 🖥️ Desktop Application

- **Clipboard Sync** (available only to donors)
  - Two-way clipboard sync
  - Supports Windows, macOS, and Linux

### 💻 UI Launcher Tool

Download [Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher/releases) if you prefer not to use the command line.

---

## 🌐 API

### Get Latest Content

```bash
GET /content/latest
```

Returns the latest clipboard item.

**Parameters**:
- `room` (optional): room name

**Examples**:

```bash
curl http://localhost:9501/content/latest
curl http://localhost:9501/content/latest?room=work
```

Full API documentation: [API.md](./cloud-clip/config.md)

---

## 🐳 Docker Images

### Image Sources

| Source | Repository |
|--------|------------|
| Docker Hub | `jonnyan404/cloud-clipboard-go` |
| GitHub Container Registry | `ghcr.io/jonnyan404/cloud-clipboard-go` |

### Pull the Latest Image

```bash
docker pull jonnyan404/cloud-clipboard-go:latest
```

---

## 📚 Detailed Documentation

- 📖 [Configuration Guide](./cloud-clip/config.md)
- 🔌 [HTTP API Documentation](./cloud-clip/config.md)
- 📱 [Client Deployment Guide](#-client-usage)

---

## 🔄 Supported Platforms

| Platform | Binary | Docker | Source | Notes |
|----------|--------|--------|--------|-------|
| Linux | ✅ | ✅ | ✅ | Primary support |
| macOS | ✅ | ✅ | ✅ | Intel and Apple Silicon |
| Windows | ✅ | ✅ | ✅ | Requires Visual C++ Build Tools |
| Android | ✅ | - | ✅ | Server APK and shortcuts |
| iOS | - | - | - | Shortcuts |
| OpenWrt | ✅ | ✅ | ✅ | Router system |

---

## 📦 Related Projects

- **[Cloud Clipboard Go Launcher](https://github.com/jonnyan404/cloud-clipboard-go-launcher)** - A UI helper tool for users who do not want to work in a terminal

---

## ☕ Support the Project

If this project helps you, you can support it in the following ways:

### 💰 Donations

Your support helps keep the project maintained and improved.

| Method | QR Code |
|--------|---------|
| **WeChat** | <img src="https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/wechat.png" width="300" alt="WeChat donation QR code"> |

### 🌟 Other Ways to Support

- [Tencent Cloud: 2 vCPU / 2 GB server from CNY 99/year](https://cloud.tencent.com/act/cps/redirect?redirect=6150&cps_key=0b1dfaf9bb573dac05abef76202dc8cc&from=console)
- [Alibaba Cloud: 2 vCPU / 2 GB server from CNY 99/year](https://www.aliyun.com/daily-act/ecs/activity_selection?userCode=79h2wrag)
- ⭐ **Star the project** if you find it useful
- 🐛 **Report issues** to help improve it
- 💡 **Share suggestions** in Discussions
- 🔀 **Contribute code** through Pull Requests
- 📢 **Share the project** with others who may need it

### 📝 Supporters

Thanks to the following supporters:

> Thanks for the support, and PRs are always welcome.
- 🥇 DOYO (donated CNY 20) Thanks for building clipboard go. It is super useful, and now I want to submit a PR.

- 🥈 xxxxxxxx (donated CNY 99)
- 🥉 xxxxxxxx (donated CNY 50)

> If you want to be listed here too, leave your name or nickname with your donation.

---

## 🙏 Acknowledgments

The frontend (client) and backend (cloud-clip) in this project are forked and adapted from the following open-source projects:

- [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard)
- [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard)

---

## 📊 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Jonnyan404/cloud-clipboard-go&type=Date)](https://www.star-history.com/#Jonnyan404/cloud-clipboard-go&Date)

---

## 📄 License

MIT License - See [LICENSE](LICENSE) for details

