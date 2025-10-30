# Clipboard Monitor

一个跨平台的剪贴板监控应用，支持自动上传剪贴板内容（文本、图片、文件）到指定的服务器。

## 📋 项目结构

```
clipboard-monitor/
├── src-tauri/
│   ├── src/
│   │   ├── main.rs           # 主入口，启动Tauri应用
│   │   ├── lib.rs            # 模块声明
│   │   ├── clipboard.rs      # 剪贴板监控逻辑
│   │   ├── uploader.rs       # 上传逻辑（大小检查、HTTP请求）
│   │   ├── config.rs         # 配置管理（读写JSON）
│   │   ├── tray.rs           # 系统托盘（菜单栏图标、菜单）
│   │   ├── commands.rs       # Tauri命令（前后端通信）
│   │   └── notifications.rs  # 系统通知
│   ├── Cargo.toml            # Rust依赖配置
│   ├── tauri.conf.json       # Tauri应用配置
│   └── static/
│       ├── config.html       # 配置界面
│       └── icons/            # 应用图标
├── build.ps1                 # PowerShell构建脚本
├── build.bat                 # Batch构建脚本
├── config.json               # 默认配置文件模板
└── README.md                 # 项目文档
```

## 🚀 系统要求

### Windows
- **操作系统**: Windows 10 或更高版本
- **内存**: 至少 2GB RAM
- **磁盘**: 至少 500MB 可用空间

### macOS
- **操作系统**: macOS 10.13 或更高版本
- **处理器**: Intel 或 Apple Silicon
- **内存**: 至少 2GB RAM

## 📦 构建依赖

### 1. Rust 工具链

**安装 Rust** (包含 Cargo)：
```bash
# 访问 https://rustup.rs/
# Windows: 下载并运行 rustup-init.exe
# macOS/Linux: 运行命令行安装器

# 验证安装
rustc --version
cargo --version
```

### 2. 系统依赖

#### Windows
- **Visual C++ 构建工具** (必需)
  ```bash
  # 选项 1: 安装 Visual Studio Community
  # https://visualstudio.microsoft.com/downloads/
  # 勾选 "Desktop development with C++"
  
  # 选项 2: 仅安装构建工具
  # https://visualstudio.microsoft.com/downloads/
  # 下载 "Visual Studio Build Tools"
  ```

#### macOS
```bash
# 安装 Xcode 命令行工具
xcode-select --install
```

### 3. Node.js (可选，用于前端资源处理)

```bash
# 访问 https://nodejs.org/
# 推荐 LTS 版本 (v18+)

# 验证安装
node --version
npm --version
```

### 4. Tauri CLI (可选)

```bash
# 通过 npm 安装
npm install -g @tauri-apps/cli

# 或通过 Cargo 安装
cargo install tauri-cli
```

## 🛠️ 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/yourusername/cloud-clipboard-go.git
cd cloud-clipboard-go/clipboard-monitor
```

### 2. 检查依赖

#### PowerShell (推荐)
```powershell
# 检查所有依赖是否已安装
.\build.ps1 -CheckOnly
```

#### Batch
```batch
build.bat
```

### 3. 开发模式运行

进入项目目录后：

```bash
cd src-tauri

# 方式 1: 使用 Tauri CLI
cargo tauri dev

# 方式 2: 直接使用 Cargo
cargo run --no-default-features
```

**功能**:
- 实时代码重新加载
- 调试模式日志输出
- 浏览器开发者工具

### 4. 构建发布版本

#### PowerShell (推荐)
```powershell
# Release 构建
.\build.ps1

# Debug 构建
.\build.ps1 -BuildType debug

# 查看帮助
.\build.ps1 -?
```

#### Batch
```batch
# Release 构建
build.bat

# Debug 构建
build.bat debug
```

#### 手动构建
```bash
cd src-tauri

# 构建 Windows 版本
cargo tauri build

# 构建特定架构
cargo tauri build --target x86_64-pc-windows-msvc
cargo tauri build --target aarch64-pc-windows-msvc
```

## 📥 构建输出

构建完成后，输出文件位置：

```
src-tauri/target/release/
├── bundle/
│   ├── msi/                          # Windows MSI 安装程序
│   │   └── Clipboard Monitor_*.msi
│   ├── nsis/                         # NSIS 安装程序 (可选)
│   └── portable/                     # 便携式可执行文件
├── Clipboard Monitor.exe             # 可执行文件
└── clipboard-monitor.exe
```

## ⚙️ 配置文件

首次运行时，应用会在用户目录创建 `config.json`：

```json
{
  "enable_monitoring": true,
  "enable_text": true,
  "enable_file": true,
  "max_file_size_mb": 50,
  "text_urls": [
    "http://localhost:3000/api/upload"
  ],
  "file_urls": [
    "http://localhost:3000/api/upload"
  ],
  "auth_token": "your-token-here"
}
```

**字段说明**:
- `enable_monitoring`: 启用/禁用剪贴板监控
- `enable_text`: 启用/禁用文本上传
- `enable_file`: 启用/禁用文件/图片上传
- `max_file_size_mb`: 最大上传文件大小（MB）
- `text_urls`: 文本上传的目标 URL 列表
- `file_urls`: 文件/图片上传的目标 URL 列表
- `auth_token`: 认证令牌 (可选)

## 🎯 功能特性

- ✅ 实时剪贴板监控
- ✅ 自动上传文本、图片、文件
- ✅ 支持多个上传端点
- ✅ Token 认证支持
- ✅ 文件大小限制
- ✅ 系统通知反馈
- ✅ 系统托盘集成
- ✅ 易用的配置界面

## 🐛 故障排除

### 构建失败：找不到 Rust

```powershell
# 安装 Rust
# 访问 https://rustup.rs/ 并按照说明安装
```

### 构建失败：Visual C++ 依赖

```powershell
# Windows 用户需要安装 Visual C++ 构建工具
# 访问 https://visualstudio.microsoft.com/downloads/
```

### 构建失败：Tauri 不兼容

```bash
# 清理构建缓存
cargo clean

# 重新构建
cargo tauri build
```

### 应用找不到配置文件

- 检查 `%APPDATA%\clipboard-monitor\config.json` (Windows)
- 检查 `~/.config/clipboard-monitor/config.json` (macOS/Linux)

## 📝 日志输出

应用会在控制台输出日志信息。设置日志级别：

```bash
# 开发模式 (默认 Info 级别)
RUST_LOG=debug cargo tauri dev

# 生产模式
set RUST_LOG=info  # Windows
export RUST_LOG=info  # macOS/Linux
```

## 🔄 跨平台构建

### 在 Windows 上为 macOS 构建

不直接支持。使用以下方案：

1. **GitHub Actions** (推荐) - 在云端自动构建所有平台
2. **虚拟机** - 在虚拟机中运行 macOS
3. **云服务** - 租用 macOS 云服务器 (MacStadium, AWS Mac)

### 在 Windows 上为 ARM64 构建

```bash
# 安装 ARM64 工具链
rustup target add aarch64-pc-windows-msvc

# 构建
cargo tauri build --target aarch64-pc-windows-msvc
```

## 📚 相关文档

- [Tauri 官方文档](https://tauri.app/docs/)
- [Rust 官方文档](https://doc.rust-lang.org/)
- [Cargo 文档](https://doc.rust-lang.org/cargo/)

## 📄 许可证

MIT License

## 🤝 贡献

欢迎提交 Issues 和 Pull Requests！

## 👨‍💻 开发者

Jonny - [GitHub](https://github.com/yourusername)

---

**最后更新**: 2025年10月30日