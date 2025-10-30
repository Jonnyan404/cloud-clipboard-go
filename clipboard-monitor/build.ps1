#Requires -Version 5.1

param(
    [ValidateSet('debug', 'release')]
    [string]$BuildType = 'release',
    
    [string]$Target = 'x86_64-pc-windows-msvc',
    
    [switch]$CheckOnly
)

$ErrorActionPreference = 'Stop'

# 颜色输出函数
function Write-Header {
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host $args -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
}

function Write-Success {
    Write-Host $args -ForegroundColor Green
}

function Write-ErrorMsg {
    Write-Host "❌ Error: $args" -ForegroundColor Red
}

function Write-Warning {
    Write-Host "⚠️  $args" -ForegroundColor Yellow
}

function Write-Info {
    Write-Host "ℹ️  $args" -ForegroundColor Blue
}

# 检查依赖
function Test-Dependencies {
    Write-Host ""
    Write-Info "Checking dependencies..."
    Write-Host ""
    
    # 检查 Rust
    try {
        $rustVersion = rustc --version 2>$null
        Write-Success "✓ $rustVersion"
    }
    catch {
        Write-ErrorMsg "Rust is not installed or not in PATH"
        Write-Info "Please install Rust from https://rustup.rs/"
        exit 1
    }
    
    # 检查 Node.js
    try {
        $nodeVersion = node --version 2>$null
        Write-Success "✓ Node.js $nodeVersion"
    }
    catch {
        Write-ErrorMsg "Node.js is not installed or not in PATH"
        Write-Info "Please install Node.js from https://nodejs.org/"
        exit 1
    }
    
    # 检查 Cargo
    try {
        $cargoVersion = cargo --version 2>$null
        Write-Success "✓ $cargoVersion"
    }
    catch {
        Write-ErrorMsg "Cargo is not installed"
        exit 1
    }
    
    Write-Host ""
}

# 主函数
function Build-App {
    Write-Header "Clipboard Monitor Build Script"
    
    Test-Dependencies
    
    if ($CheckOnly) {
        Write-Success "All dependencies are installed!"
        return
    }
    
    Write-Info "Build Configuration:"
    Write-Host "  Build Type: $BuildType"
    Write-Host "  Target: $Target"
    Write-Host ""
    
    # Tauri 2.0 的构建命令：cargo tauri build 默认就是 release
    # 如果需要 debug 构建，需要使用 cargo build --debug 然后手动处理
    if ($BuildType -eq 'release') {
        Write-Info "Running release build..."
        $buildArgs = @('tauri', 'build')
        $targetDir = 'release'
    }
    else {
        Write-Info "Running debug build..."
        # 对于 debug 构建，我们使用 cargo build 而不是 cargo tauri build
        $buildArgs = @('build', '--target', $Target)
        $targetDir = 'debug'
    }
    
    # 添加目标参数
    if ($BuildType -eq 'release') {
        $buildArgs += '--target'
        $buildArgs += $Target
    }
    
    Write-Host ""
    
    # 执行构建
    try {
        Write-Info "Executing: cargo $($buildArgs -join ' ')"
        Write-Host ""
        
        & cargo @buildArgs
        
        if ($LASTEXITCODE -ne 0) {
            Write-ErrorMsg "Build failed with exit code $LASTEXITCODE"
            exit 1
        }
    }
    catch {
        Write-ErrorMsg "Build process failed: $_"
        exit 1
    }
    
    # 输出结果
    Write-Host ""
    Write-Header "Build Completed Successfully!"
    
    $outputDir = "src-tauri\target\$targetDir\bundle"
    
    Write-Info "Output locations:"
    if ($BuildType -eq 'release') {
        Write-Host "  MSI installer: $outputDir\msi\"
    }
    Write-Host "  EXE file: src-tauri\target\$targetDir\"
    Write-Host ""
    
    # 打开输出目录
    if (Test-Path $outputDir) {
        $response = Read-Host "Open output directory? (Y/n)"
        if ($response -ne 'n' -and $response -ne 'N') {
            explorer $outputDir
        }
    }
}

# 执行主函数
try {
    Build-App
}
catch {
    Write-ErrorMsg $_
    exit 1
}