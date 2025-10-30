@echo off
setlocal enabledelayedexpansion

echo.
echo ========================================
echo   Clipboard Monitor Build Script
echo ========================================
echo.

REM 检查是否安装了 Rust
rustc --version >nul 2>&1
if errorlevel 1 (
    echo Error: Rust is not installed or not in PATH
    echo Please install Rust from https://rustup.rs/
    exit /b 1
)

REM 检查是否安装了 Node.js
node --version >nul 2>&1
if errorlevel 1 (
    echo Error: Node.js is not installed or not in PATH
    echo Please install Node.js from https://nodejs.org/
    exit /b 1
)

echo Rust version:
rustc --version
echo.
echo Node.js version:
node --version
echo.

REM 设置构建目标
set TARGET=x86_64-pc-windows-msvc
set BUILD_TYPE=%1
if "!BUILD_TYPE!"=="" set BUILD_TYPE=release

echo Building for Windows (!BUILD_TYPE!)...
echo Target: !TARGET!
echo.

REM 构建应用
if "!BUILD_TYPE!"=="debug" (
    echo Running debug build...
    cargo tauri build --target !TARGET!
) else (
    echo Running release build...
    cargo tauri build --target !TARGET!
)

if errorlevel 1 (
    echo.
    echo Error: Build failed!
    exit /b 1
)

echo.
echo ========================================
echo Build completed successfully!
echo ========================================
echo.
echo Output locations:
echo - MSI installer: src-tauri\target\release\bundle\msi\
echo - EXE file: src-tauri\target\release\
echo.

endlocal
pause