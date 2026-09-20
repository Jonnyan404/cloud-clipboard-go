## 我该下哪个？

按「系统 + CPU」选。`x86_64` = Intel/AMD 64 位；`aarch64` = ARM 64 位（M 系列 Mac、树莓派 4/5、ARM 服务器）；`armv7` = 旧 ARM（树莓派 2/3）。

| 装在哪 | 下载 |
| --- | --- |
| Windows 10/11 | `CCG_server-windows-x86_64-v__VERSION__.zip` |
| Windows on ARM | `CCG_server-windows-aarch64-v__VERSION__.zip` |
| macOS（M 系列芯片） | `CCG_server-darwin-aarch64-v__VERSION__.tar.gz` |
| macOS（Intel 芯片） | `CCG_server-darwin-x86_64-v__VERSION__.tar.gz` |
| Linux x86_64 | `CCG_server-linux-x86_64-v__VERSION__.tar.gz` |
| Linux ARM64（树莓派 4/5） | `CCG_server-linux-aarch64-v__VERSION__.tar.gz` |
| Linux ARMv7（树莓派 2/3） | `CCG_server-linux-armv7-v__VERSION__.tar.gz` |
| Android 手机 | `CCG_server-android-arm64-v8a-v__VERSION__.apk`（拿不准就下 `universal`，包大但都能装） |
| OpenWrt 路由器 | 见下方「OpenWrt 怎么选」 |
| 不想自己装 | 见下方「其他安装方式」 |

> **English**: pick the file matching your OS and CPU. `x86_64` = Intel/AMD 64-bit, `aarch64` = ARM 64-bit, `armv7` = older ARM.

### 怎么跑

桌面版解压后直接执行，浏览器打开 <http://127.0.0.1:9501> 即可：

```bash
tar xzf CCG_server-linux-x86_64-v__VERSION__.tar.gz   # Windows 下是 .zip，解压即可
./cloud-clipboard-go
```

### OpenWrt 怎么选

- **包格式**看固件版本：OpenWrt **25.12 及以上**用 `.apk`（从这一版起 apk 取代了 opkg），**24.10 及更早**用 `.ipk`。
- **CPU 架构**在路由器上执行 `uname -m`，或对照型号。软路由（x86）选 `x86_64`；常见家用路由多在 `arm_cortex-a7` / `arm_cortex-a9` 一档。
- 只带 LuCI 界面、不含服务端的是 `CCG_server-luci-*` 那两个。

### 其他安装方式

- **Docker**：`ghcr.io/jonnyan404/cloud-clipboard-go`
- **Homebrew**（macOS）：`brew install jonnyan404/tap/cloud-clipboard-go`
- **Cloudflare Worker**：不占自己的服务器，部署见 README

---
