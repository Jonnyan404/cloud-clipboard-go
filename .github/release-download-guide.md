## 我该下哪个？

按「系统 + CPU」选。`x86_64` = Intel/AMD 64 位；`aarch64` = ARM 64 位（M 系列 Mac、树莓派 4/5、ARM 服务器）；`armv7` = 旧 ARM（树莓派 2/3）。

<div align="left">

| **系统** | **什么时候选它** | **点击直链下载** |
|:---:|:---|:---|
| <img src="https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/windows11/windows11-original.svg" alt="Windows" width="28"/> | 64 位 Windows 10/11<br>*(ARM 设备选右边那个)* | <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-windows-x86_64-v__VERSION__.zip"><img src="https://img.shields.io/badge/ZIP-x64-0078D7?logo=windows&logoColor=white&style=flat-square&labelColor=222222"></a> <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-windows-aarch64-v__VERSION__.zip"><img src="https://img.shields.io/badge/ZIP-ARM64-0078D7?logo=windows&logoColor=white&style=flat-square&labelColor=222222"></a> |
| <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/apple/apple-original.svg" alt="macOS" width="28"/> | M 系列芯片<br>*(Intel 选右边那个)* | <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-darwin-aarch64-v__VERSION__.tar.gz"><img src="https://img.shields.io/badge/TAR.GZ-Apple%20Silicon-000000?logo=apple&logoColor=white&style=flat-square&labelColor=222222"></a> <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-darwin-x86_64-v__VERSION__.tar.gz"><img src="https://img.shields.io/badge/TAR.GZ-Intel%20x64-00A9E0?logo=apple&logoColor=white&style=flat-square&labelColor=222222"></a> |
| <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/linux/linux-original.svg" alt="Linux" width="28"/> | 服务器与 PC<br>*(树莓派按右边两个选)* | <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-linux-x86_64-v__VERSION__.tar.gz"><img src="https://img.shields.io/badge/TAR.GZ-x64-f84e29?logo=linux&logoColor=white&style=flat-square&labelColor=222222"></a> <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-linux-aarch64-v__VERSION__.tar.gz"><img src="https://img.shields.io/badge/TAR.GZ-ARM64-f84e29?logo=linux&logoColor=white&style=flat-square&labelColor=222222"></a><br><a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-linux-armv7-v__VERSION__.tar.gz"><img src="https://img.shields.io/badge/TAR.GZ-ARMv7-f84e29?logo=linux&logoColor=white&style=flat-square&labelColor=222222"></a> |
| <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/android/android-original.svg" alt="Android" width="28"/> | 手机<br>*(拿不准就选 Universal)* | <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-android-arm64-v8a-v__VERSION__.apk"><img src="https://img.shields.io/badge/APK-ARMv8-32AF6A?logo=android&logoColor=white&style=flat-square&labelColor=222222"></a> <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-android-x86_64-v__VERSION__.apk"><img src="https://img.shields.io/badge/APK-x64-32AF6A?logo=android&logoColor=white&style=flat-square&labelColor=222222"></a><br><a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-android-armeabi-v7a-v__VERSION__.apk"><img src="https://img.shields.io/badge/APK-ARMv7-32AF6A?logo=android&logoColor=white&style=flat-square&labelColor=222222"></a> <a href="https://github.com/Jonnyan404/cloud-clipboard-go/releases/download/v__VERSION__/CCG_server-android-universal-v__VERSION__.apk"><img src="https://img.shields.io/badge/APK-Universal-32AF6A?logo=android&logoColor=white&style=flat-square&labelColor=222222"></a> |
| <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/docker/docker-original.svg" alt="Docker" width="28"/> | 不想手动装<br>*(x86 / ARM 都行)* | <a href="https://github.com/Jonnyan404/cloud-clipboard-go/pkgs/container/cloud-clipboard-go"><img src="https://img.shields.io/badge/ghcr.io-cloud--clipboard--go-2496ED?logo=docker&logoColor=white&style=flat-square&labelColor=222222"></a> |

</div>

> **English**: pick the file matching your OS and CPU. `x86_64` = Intel/AMD 64-bit, `aarch64` = ARM 64-bit, `armv7` = older ARM.

### 怎么跑

桌面版解压后直接执行，浏览器打开 <http://127.0.0.1:9501> 即可：

```bash
tar xzf CCG_server-linux-x86_64-v__VERSION__.tar.gz   # Windows 下是 .zip，解压即可
./cloud-clipboard-go
```

### OpenWrt 怎么选

OpenWrt 的包有二十多个（各种 CPU 架构 × 两种包格式），不放进上面那张表。规则两条：

- **包格式**看固件版本：OpenWrt **25.12 及以上**用 `.apk`（从这一版起 apk 取代了 opkg），**24.10 及更早**用 `.ipk`。
- **CPU 架构**在路由器上执行 `uname -m`，或对照型号。软路由（x86）选 `x86_64`；常见家用路由多在 `arm_cortex-a7` / `arm_cortex-a9` 一档。

只带 LuCI 界面、不含服务端的是 `CCG_server-luci-*` 那两个。

### 其他安装方式

- **Homebrew**（macOS）：`brew install jonnyan404/tap/cloud-clipboard-go`
- **Cloudflare Worker**：不占自己的服务器，部署见 README

---
