# cloud-clipboard-go

<p>
  <a href="README.md"><img src="https://img.shields.io/badge/lang-简体中文-blue.svg" alt="中文 Readme"></a>
  <a href="https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go-launcher/main/LICENSE">
    <img src="https://img.shields.io/github/license/jonnyan404/cloud-clipboard-go-launcher?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/v/release/jonnyan404/cloud-clipboard-go?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/downloads/jonnyan404/cloud-clipboard-go/total?color=brightgreen&include_prereleases" alt="release">
  </a>
</p>

A Go remake based on the [TransparentLC/cloud-clipboard](https://github.com/TransparentLC/cloud-clipboard) project.

Added some features based on [yurenchen000/cloud-clipboard](https://github.com/yurenchen000/cloud-clipboard).

## Screenshots

<details>
<summary>Desktop</summary>

![](https://ae01.alicdn.com/kf/Hfce3a9b69b3d404c8e3073ab0fffa913v.png)

</details>

<details>
<summary>Mobile</summary>

![](https://ae01.alicdn.com/kf/Hbf859dd0e42c4406bf94a6b6f2f4658cf.png)

</details>

## Usage

This is the Go version of the server.

### Android Shortcuts (Imitating iOS Shortcuts)

- quire: server version ≥ v4.5.10

1. [Download http-shortcuts](https://github.com/Waboodoo/HTTP-Shortcuts/releases)
2. [Download cloud-clipboard-shortcuts.zip](https://raw.githubusercontent.com/Jonnyan404/cloud-clipboard-go/refs/heads/main/shortcuts/cloud-clipboard-shortcuts.zip)
3. Open `http-shortcuts`, click the three-dot menu in the top right corner --> Import/Export --> Import from file --> Select the file downloaded in step 2.
4. Click the three-dot menu in the top right corner --> Variables --> Modify the `url` value to your server's IP and port (If there is a prefix parameter, it needs to be added after the port.); `room` is optional, default value is empty; `auth` is optional, default value is empty; do not change anything else.


### Easy Run (UI Launcher, Recommended for Beginners)

<details>
    <summary>Click to view preview</summary>

![](https://github.com/Jonnyan404/cloud-clipboard-go-launcher/blob/main/demo.png)

</details>

Go to [UI Launcher Releases](https://github.com/Jonnyan404/cloud-clipboard-go-launcher/releases), download, and double-click to use.

### Run with Docker

```sh
# Docker Hub Image (choose one)
docker run -d --name=cloud-clipboard-go -p 9501:9501 -v /path/your/dir/data:/app/server-node/data jonnyan404/cloud-clipboard-go
# GitHub Container Registry Image (choose one)
docker run -d --name=cloud-clipboard-go -p 9501:9501 -v /path/your/dir/data:/app/server-node/data ghcr.io/jonnyan404/cloud-clipboard-go
```

- `vi docker-compose.yml`

```yaml
services:
    cloud-clipboard-go:
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
        volumes:
            - /path/your/dir/data:/app/server-node/data # Please change to your own directory
        image: jonnyan404/cloud-clipboard-go:latest # Or ghcr.io/jonnyan404/cloud-clipboard-go:latest
```

- Access homepage: http://127.0.0.1:9501
- Accessing http://127.0.0.1:9501/content/latest will always return the latest content. You can add the room parameter `?room=xxx`.

### Run with Homebrew

> Known Issue: `brew services` tab completion doesn't work. Reference: https://github.com/orgs/Homebrew/discussions/6047#discussioncomment-12668536

Default configuration files are located in the `etc/cloud-clipboard-go` and `var` directories under the `homebrew` root directory.

```bash
brew update
# Install
brew install Jonnyan404/tap/cloud-clipboard-go
# Start the service
brew services start cloud-clipboard-go
# Check service status
brew services info cloud-clipboard-go
# Stop the service
brew services stop cloud-clipboard-go
# Restart the service
brew services restart cloud-clipboard-go
```

### Run on OpenWrt

✅ Tested on OpenWrt 24.10.0

Check your architecture with: `opkg print-architecture` (the second column of the last line)

Go to the `pre-release` section of https://github.com/Jonnyan404/cloud-clipboard-go/releases and download the `*platform.ipk` file corresponding to your system and the `*_all.ipk` file.

Then execute the following commands in the terminal:
```bash
opkg install *platform.ipk
opkg install *_all.ipk
```

<details>
    <summary>Click to preview LuCI interface</summary>

![](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/openwrt/demo.png)

</details>

### Run with Binary File

Go to the project [Releases](https://github.com/Jonnyan404/cloud-clipboard-go/releases), download the file corresponding to your system, and run it.

> Parameter priority: Command-line arguments > Configuration file

- Command-line argument: `-host` to customize the server listening address.
- Command-line argument: `-port` to customize the server listening port.
- Command-line argument: `-auth` to customize the password.
- Command-line argument: `-config` to load a custom configuration file.
- Command-line argument: `-static` to load custom external frontend files.

### Run from Source Code

- Requires [Node.js>=22.12](https://nodejs.org).
- Requires [Go>=1.22](https://go.dev/).

```bash
# Build frontend
cd client
npm install
npm run build

# Run backend
cd ../cloud-clip
go mod tidy
go run -tags embed .
```

### Configuration File Description

- Please see: https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/cloud-clip/config.md

### HTTP API

- Please see: https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/cloud-clip/config.md

## Derivative Projects

- A launcher made for cloud-clipboard-go, convenient for users who don't want to or can't use the terminal: [cloud-clipboard-go-launcher](https://github.com/Jonnyan404/cloud-clipboard-go-launcher)
