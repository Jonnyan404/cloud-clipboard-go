param(
    [switch]$SkipAAR,      # 跳过 AAR 编译
    [switch]$SkipAPK,      # 跳过 APK 编译
    [switch]$OnlyAAR,      # 只编译 AAR
    [switch]$OnlyAPK,      # 只编译 APK
    [switch]$Install,      # 自动安装到设备
    [switch]$Help          # 显示帮助
)

$ErrorActionPreference = "Stop"

# 显示帮助信息
function Show-Help {
    Write-Host "用法: .\build-android.ps1 [选项]" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "选项:" -ForegroundColor Yellow
    Write-Host "  -OnlyAAR      只编译 AAR 包,不编译 APK"
    Write-Host "  -OnlyAPK      只编译 APK,跳过 AAR 编译"
    Write-Host "  -SkipAAR      跳过 AAR 编译,直接编译 APK"
    Write-Host "  -SkipAPK      只编译 AAR,不编译 APK"
    Write-Host "  -Install      编译完成后自动安装到设备"
    Write-Host "  -Help         显示此帮助信息"
    Write-Host ""
    Write-Host "示例:" -ForegroundColor Green
    Write-Host "  .\build-android.ps1                    # 完整构建流程"
    Write-Host "  .\build-android.ps1 -OnlyAAR           # 只编译 AAR"
    Write-Host "  .\build-android.ps1 -OnlyAPK           # 只编译 APK"
    Write-Host "  .\build-android.ps1 -SkipAAR           # 跳过 AAR,只编译 APK"
    Write-Host "  .\build-android.ps1 -Install           # 完整构建并自动安装"
    Write-Host ""
}

if ($Help) {
    Show-Help
    exit 0
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  构建 Cloud Clipboard Android 应用" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 参数验证
if ($OnlyAAR -and $OnlyAPK) {
    Write-Host "[错误] -OnlyAAR 和 -OnlyAPK 不能同时使用" -ForegroundColor Red
    exit 1
}

if ($SkipAAR -and $OnlyAAR) {
    Write-Host "[错误] -SkipAAR 和 -OnlyAAR 不能同时使用" -ForegroundColor Red
    exit 1
}

# 确定执行哪些步骤
$BuildAAR = -not ($SkipAAR -or $OnlyAPK)
$BuildAPK = -not ($SkipAPK -or $OnlyAAR)

# 函数:检查命令是否存在
function Test-Command {
    param($Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

# 函数:显示错误并退出
function Exit-WithError {
    param($Message)
    Write-Host "[错误] $Message" -ForegroundColor Red
    Read-Host "按任意键退出"
    exit 1
}

# 检查必要工具
Write-Host "[检查] 验证必要工具..." -ForegroundColor Yellow

if (-not (Test-Command "java")) {
    Exit-WithError "未检测到 Java,请先安装 JDK 17`n下载地址: https://adoptium.net/"
}
Write-Host "[✓] Java 已安装" -ForegroundColor Green

if ($BuildAAR) {
    if (-not (Test-Command "go")) {
        Exit-WithError "未检测到 Go,请先安装 Go 1.22+`n下载地址: https://go.dev/dl/"
    }
    Write-Host "[✓] Go 已安装" -ForegroundColor Green

    if (-not (Test-Command "gomobile")) {
        Write-Host "[警告] gomobile 未安装,正在安装..." -ForegroundColor Yellow
        go install golang.org/x/mobile/cmd/gomobile@latest
        if ($LASTEXITCODE -ne 0) {
            Exit-WithError "gomobile 安装失败"
        }
    }
    Write-Host "[✓] gomobile 已安装" -ForegroundColor Green

    # 检查并初始化 gomobile
    Write-Host "[检查] 验证 gomobile 初始化状态..." -ForegroundColor Yellow
    $GobindPath = Join-Path $env:GOPATH "pkg\gomobile"
    if (-not (Test-Path $GobindPath)) {
        Write-Host "[信息] gomobile 需要初始化,这可能需要几分钟..." -ForegroundColor Yellow
        Write-Host "[信息] 正在下载 Android NDK 和工具链..." -ForegroundColor Yellow
        & gomobile init
        if ($LASTEXITCODE -ne 0) {
            Exit-WithError "gomobile 初始化失败"
        }
        Write-Host "[✓] gomobile 初始化成功" -ForegroundColor Green
    } else {
        Write-Host "[✓] gomobile 已初始化" -ForegroundColor Green
    }
}

$RootDir = Get-Location

# ==================== 步骤 1: 编译 AAR ====================
if ($BuildAAR) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  步骤 1: 编译 Go AAR 包" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan

    # 检查 mobile 目录是否存在
    $MobileDir = Join-Path $RootDir "cloud-clip\mobile"
    if (-not (Test-Path $MobileDir)) {
        Exit-WithError "找不到 cloud-clip\mobile 目录: $MobileDir"
    }

    # 进入 mobile 目录
    Push-Location $MobileDir

    Write-Host "[信息] 当前目录: $(Get-Location)" -ForegroundColor Gray
    Write-Host "[信息] 清理旧的 AAR 文件..." -ForegroundColor Yellow
    Remove-Item -Path "cloudclipservice.aar" -ErrorAction SilentlyContinue
    Remove-Item -Path "cloudclipservice-sources.jar" -ErrorAction SilentlyContinue

    Write-Host "[信息] 开始编译 AAR..." -ForegroundColor Yellow
    Write-Host "[命令] gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags=`"-s -w`"" -ForegroundColor Gray

    # 设置 Java 编译器使用 UTF-8 编码
    $env:JAVA_TOOL_OPTIONS = "-Dfile.encoding=UTF-8"
    Write-Host "[信息] 已设置 Java 编码为 UTF-8" -ForegroundColor Gray

    & gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w" github.com/jonnyan404/cloud-clipboard-go/cloud-clip/mobile

    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        Exit-WithError "AAR 编译失败!"
    }

    Write-Host "[✓] AAR 编译成功: cloudclipservice.aar" -ForegroundColor Green

    # 复制 AAR 到 Android 项目
    Write-Host "[信息] 复制 AAR 到 Android 项目..." -ForegroundColor Yellow
    $LibsDir = Join-Path $RootDir "android\app\libs"
    if (-not (Test-Path $LibsDir)) {
        New-Item -ItemType Directory -Path $LibsDir | Out-Null
    }
    Copy-Item -Path "cloudclipservice.aar" -Destination $LibsDir -Force
    Write-Host "[✓] AAR 已复制到 android\app\libs\" -ForegroundColor Green

    Pop-Location
} else {
    Write-Host ""
    Write-Host "[跳过] 跳过 AAR 编译" -ForegroundColor Yellow
    
    # 检查 AAR 文件是否存在
    $AARPath = Join-Path $RootDir "android\app\libs\cloudclipservice.aar"
    if (-not (Test-Path $AARPath)) {
        Exit-WithError "找不到 AAR 文件: $AARPath`n请先运行完整构建或使用 -OnlyAAR 参数编译 AAR"
    }
    Write-Host "[✓] 找到现有 AAR 文件" -ForegroundColor Green
}

# ==================== 步骤 2: 编译 APK ====================
if ($BuildAPK) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  步骤 2: 编译 Android APK" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan

    # 检查 android 目录是否存在
    $AndroidDir = Join-Path $RootDir "android"
    if (-not (Test-Path $AndroidDir)) {
        Exit-WithError "找不到 android 目录: $AndroidDir"
    }

    Push-Location $AndroidDir

    Write-Host "[信息] 当前目录: $(Get-Location)" -ForegroundColor Gray
    Write-Host "[信息] 清理项目..." -ForegroundColor Yellow
    & .\gradlew.bat clean
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        Exit-WithError "Gradle clean 失败"
    }

    Write-Host "[信息] 编译 Debug APK..." -ForegroundColor Yellow
    & .\gradlew.bat assembleDebug
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        Exit-WithError "APK 编译失败!"
    }

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  构建成功!" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "[✓] APK 位置: android\app\build\outputs\apk\debug\app-debug.apk" -ForegroundColor Green
    Write-Host ""

    # 自动安装或询问
    if ($Install) {
        Write-Host "[信息] 正在安装 APK..." -ForegroundColor Yellow
        & .\gradlew.bat installDebug
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[警告] APK 安装失败,请手动安装" -ForegroundColor Yellow
        } else {
            Write-Host "[✓] APK 已成功安装到设备" -ForegroundColor Green
        }
    } else {
        $InstallChoice = Read-Host "是否安装到设备? (y/n)"
        if ($InstallChoice -eq "y" -or $InstallChoice -eq "Y") {
            Write-Host "[信息] 正在安装 APK..." -ForegroundColor Yellow
            & .\gradlew.bat installDebug
            if ($LASTEXITCODE -ne 0) {
                Write-Host "[警告] APK 安装失败,请手动安装" -ForegroundColor Yellow
            } else {
                Write-Host "[✓] APK 已成功安装到设备" -ForegroundColor Green
            }
        }
    }

    Pop-Location
} else {
    Write-Host ""
    Write-Host "[跳过] 跳过 APK 编译" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "[完成] 构建流程结束" -ForegroundColor Green
Read-Host "按回车键退出"