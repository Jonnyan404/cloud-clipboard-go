REM filepath: build-android.bat
@echo off
setlocal enabledelayedexpansion

REM 解析命令行参数
set "SKIP_AAR=0"
set "SKIP_APK=0"
set "ONLY_AAR=0"
set "ONLY_APK=0"
set "AUTO_INSTALL=0"

:parse_args
if "%~1"=="" goto :end_parse_args
if /i "%~1"=="-SkipAAR" set "SKIP_AAR=1"
if /i "%~1"=="-SkipAPK" set "SKIP_APK=1"
if /i "%~1"=="-OnlyAAR" set "ONLY_AAR=1"
if /i "%~1"=="-OnlyAPK" set "ONLY_APK=1"
if /i "%~1"=="-Install" set "AUTO_INSTALL=1"
if /i "%~1"=="-Help" goto :show_help
if /i "%~1"=="-h" goto :show_help
shift
goto :parse_args

:show_help
echo 用法: build-android.bat [选项]
echo.
echo 选项:
echo   -OnlyAAR      只编译 AAR 包,不编译 APK
echo   -OnlyAPK      只编译 APK,跳过 AAR 编译
echo   -SkipAAR      跳过 AAR 编译,直接编译 APK
echo   -SkipAPK      只编译 AAR,不编译 APK
echo   -Install      编译完成后自动安装到设备
echo   -Help         显示此帮助信息
echo.
echo 示例:
echo   build-android.bat                    # 完整构建流程
echo   build-android.bat -OnlyAAR           # 只编译 AAR
echo   build-android.bat -OnlyAPK           # 只编译 APK
echo   build-android.bat -SkipAAR           # 跳过 AAR,只编译 APK
echo   build-android.bat -Install           # 完整构建并自动安装
echo.
exit /b 0

:end_parse_args

echo ========================================
echo   构建 Cloud Clipboard Android 应用
echo ========================================
echo.

REM 参数验证
if "%ONLY_AAR%"=="1" if "%ONLY_APK%"=="1" (
    echo [错误] -OnlyAAR 和 -OnlyAPK 不能同时使用
    pause
    exit /b 1
)

if "%SKIP_AAR%"=="1" if "%ONLY_AAR%"=="1" (
    echo [错误] -SkipAAR 和 -OnlyAAR 不能同时使用
    pause
    exit /b 1
)

REM 确定执行哪些步骤
set "BUILD_AAR=1"
set "BUILD_APK=1"

if "%SKIP_AAR%"=="1" set "BUILD_AAR=0"
if "%ONLY_APK%"=="1" set "BUILD_AAR=0"
if "%SKIP_APK%"=="1" set "BUILD_APK=0"
if "%ONLY_AAR%"=="1" set "BUILD_APK=0"

REM 检查必要工具
echo [检查] 验证必要工具...

java -version >nul 2>&1
if errorlevel 1 (
    echo [错误] 未检测到 Java,请先安装 JDK 17
    echo 下载地址: https://adoptium.net/
    pause
    exit /b 1
)
echo [?] Java 已安装

if "%BUILD_AAR%"=="1" (
    go version >nul 2>&1
    if errorlevel 1 (
        echo [错误] 未检测到 Go,请先安装 Go 1.22+
        echo 下载地址: https://go.dev/dl/
        pause
        exit /b 1
    )
    echo [?] Go 已安装

    gomobile version >nul 2>&1
    if errorlevel 1 (
        echo [警告] gomobile 未安装,正在安装...
        go install golang.org/x/mobile/cmd/gomobile@latest
        if errorlevel 1 (
            echo [错误] gomobile 安装失败
            pause
            exit /b 1
        )
    )
    echo [?] gomobile 已安装
)

set "ROOT_DIR=%CD%"

REM ==================== 步骤 1: 编译 AAR ====================
if "%BUILD_AAR%"=="1" (
    echo.
    echo ========================================
    echo   步骤 1: 编译 Go AAR 包
    echo ========================================

    cd cloud-clip\mobile
    if errorlevel 1 (
        echo [错误] 找不到 cloud-clip\mobile 目录
        pause
        exit /b 1
    )

    echo [信息] 清理旧的 AAR 文件...
    if exist cloudclipservice.aar del /f /q cloudclipservice.aar
    if exist cloudclipservice-sources.jar del /f /q cloudclipservice-sources.jar

    echo [信息] 开始编译 AAR...
    echo [命令] gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w"

    set "JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8"
    echo [信息] 已设置 Java 编码为 UTF-8

    gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w" github.com/jonnyan404/cloud-clipboard-go/cloud-clip/mobile

    if errorlevel 1 (
        echo [错误] AAR 编译失败!
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo [?] AAR 编译成功: cloudclipservice.aar

    echo [信息] 复制 AAR 到 Android 项目...
    if not exist "%ROOT_DIR%\android\app\libs" mkdir "%ROOT_DIR%\android\app\libs"
    copy /y cloudclipservice.aar "%ROOT_DIR%\android\app\libs\"
    if errorlevel 1 (
        echo [错误] AAR 复制失败
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )
    echo [?] AAR 已复制到 android\app\libs\

    cd "%ROOT_DIR%"
) else (
    echo.
    echo [跳过] 跳过 AAR 编译
    
    if not exist "%ROOT_DIR%\android\app\libs\cloudclipservice.aar" (
        echo [错误] 找不到 AAR 文件: android\app\libs\cloudclipservice.aar
        echo 请先运行完整构建或使用 -OnlyAAR 参数编译 AAR
        pause
        exit /b 1
    )
    echo [?] 找到现有 AAR 文件
)

REM ==================== 步骤 2: 编译 APK ====================
if "%BUILD_APK%"=="1" (
    echo.
    echo ========================================
    echo   步骤 2: 编译 Android APK
    echo ========================================

    cd android
    if errorlevel 1 (
        echo [错误] 找不到 android 目录
        pause
        exit /b 1
    )

    echo [信息] 清理项目...
    call gradlew.bat clean
    if errorlevel 1 (
        echo [错误] Gradle clean 失败
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo [信息] 编译 Debug APK...
    call gradlew.bat assembleDebug
    if errorlevel 1 (
        echo [错误] APK 编译失败!
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo.
    echo ========================================
    echo   构建成功!
    echo ========================================
    echo.
    echo [?] APK 位置: android\app\build\outputs\apk\debug\app-debug.apk
    echo.

    if "%AUTO_INSTALL%"=="1" (
        echo [信息] 正在安装 APK...
        call gradlew.bat installDebug
        if errorlevel 1 (
            echo [警告] APK 安装失败,请手动安装
        ) else (
            echo [?] APK 已成功安装到设备
        )
    ) else (
        set /p INSTALL_CHOICE="是否安装到设备? (y/n): "
        if /i "!INSTALL_CHOICE!"=="y" (
            echo [信息] 正在安装 APK...
            call gradlew.bat installDebug
            if errorlevel 1 (
                echo [警告] APK 安装失败,请手动安装
            ) else (
                echo [?] APK 已成功安装到设备
            )
        )
    )

    cd "%ROOT_DIR%"
) else (
    echo.
    echo [跳过] 跳过 APK 编译
)

echo.
echo [完成] 构建流程结束
pause