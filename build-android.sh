#!/bin/bash
# filepath: build-android.sh

# 当任何命令失败时立即退出
set -e

# --- 配置 ---
# 定义颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
GRAY='\033[0;90m'
NC='\033[0m' # 无颜色

# --- 默认标志 ---
BUILD_AAR=true
BUILD_APK=true
INSTALL_APK=false
SHOW_HELP=false

# --- 函数定义 ---
show_help() {
    echo -e "${CYAN}用法: ./build-android.sh [选项]${NC}"
    echo ""
    echo -e "${YELLOW}选项:${NC}"
    echo "  --only-aar      只编译 AAR 包,不编译 APK"
    echo "  --only-apk      只编译 APK,跳过 AAR 编译"
    echo "  --install       编译完成后自动安装到设备"
    echo "  --help          显示此帮助信息"
    echo ""
    echo -e "${GREEN}示例:${NC}"
    echo "  ./build-android.sh                    # 完整构建流程"
    echo "  ./build-android.sh --only-aar         # 只编译 AAR"
    echo "  ./build-android.sh --only-apk         # 只编译 APK"
    echo "  ./build-android.sh --install          # 完整构建并自动安装"
}

exit_with_error() {
    echo -e "${RED}[错误] $1${NC}" >&2
    exit 1
}

# --- 参数解析 ---
for arg in "$@"; do
  case $arg in
    --only-aar)
      BUILD_APK=false
      shift
      ;;
    --only-apk)
      BUILD_AAR=false
      shift
      ;;
    --install)
      INSTALL_APK=true
      shift
      ;;
    --help)
      SHOW_HELP=true
      shift
      ;;
    *)
      # 未知选项
      ;;
  esac
done

if [ "$SHOW_HELP" = true ]; then
    show_help
    exit 0
fi

# --- 主逻辑 ---
echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  构建 Cloud Clipboard Android 应用${NC}"
echo -e "${CYAN}========================================${NC}"
echo ""

# 检查必要工具
echo -e "${YELLOW}[检查] 验证必要工具...${NC}"
if ! command -v java &> /dev/null; then
    exit_with_error "未检测到 Java,请先安装 JDK 17"
fi
echo -e "${GREEN}[✓] Java 已安装${NC}"

if [ "$BUILD_AAR" = true ]; then
    if ! command -v go &> /dev/null; then
        exit_with_error "未检测到 Go,请先安装 Go 1.22+"
    fi
    echo -e "${GREEN}[✓] Go 已安装${NC}"

    if ! command -v gomobile &> /dev/null; then
        echo -e "${YELLOW}[警告] gomobile 未安装,正在安装...${NC}"
        go install golang.org/x/mobile/cmd/gomobile@latest
    fi
    echo -e "${GREEN}[✓] gomobile 已安装${NC}"

    echo -e "${YELLOW}[检查] 验证 gomobile 初始化状态...${NC}"
    GOPATH=$(go env GOPATH)
    if [ ! -d "$GOPATH/pkg/gomobile" ]; then
        echo -e "${YELLOW}[信息] gomobile 需要初始化,这可能需要几分钟...${NC}"
        gomobile init
    fi
    echo -e "${GREEN}[✓] gomobile 已初始化${NC}"
fi

ROOT_DIR=$(pwd)

# ==================== 步骤 1: 编译 AAR ====================
if [ "$BUILD_AAR" = true ]; then
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  步骤 1: 编译 Go AAR 包${NC}"
    echo -e "${CYAN}========================================${NC}"

    MOBILE_DIR="$ROOT_DIR/cloud-clip/mobile"
    if [ ! -d "$MOBILE_DIR" ]; then
        exit_with_error "找不到 cloud-clip/mobile 目录: $MOBILE_DIR"
    fi

    pushd "$MOBILE_DIR" > /dev/null

    echo -e "${GRAY}[信息] 当前目录: $(pwd)${NC}"
    echo -e "${YELLOW}[信息] 清理旧的 AAR 文件...${NC}"
    rm -f cloudclipservice.aar cloudclipservice-sources.jar

    echo -e "${YELLOW}[信息] 开始编译 AAR...${NC}"
    echo -e "${GRAY}[命令] gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags=\"-s -w\" github.com/jonnyan404/cloud-clipboard-go/cloud-clip/mobile${NC}"
    
    gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w" github.com/jonnyan404/cloud-clipboard-go/cloud-clip/mobile

    echo -e "${GREEN}[✓] AAR 编译成功: cloudclipservice.aar${NC}"

    echo -e "${YELLOW}[信息] 复制 AAR 到 Android 项目...${NC}"
    LIBS_DIR="$ROOT_DIR/android/app/libs"
    mkdir -p "$LIBS_DIR"
    cp cloudclipservice.aar "$LIBS_DIR/"
    echo -e "${GREEN}[✓] AAR 已复制到 android/app/libs/${NC}"

    popd > /dev/null
else
    echo ""
    echo -e "${YELLOW}[跳过] 跳过 AAR 编译${NC}"
    AAR_PATH="$ROOT_DIR/android/app/libs/cloudclipservice.aar"
    if [ ! -f "$AAR_PATH" ]; then
        exit_with_error "找不到 AAR 文件: $AAR_PATH\n请先运行完整构建或使用 --only-aar 参数编译 AAR"
    fi
    echo -e "${GREEN}[✓] 找到现有 AAR 文件${NC}"
fi

# ==================== 步骤 2: 编译 APK ====================
if [ "$BUILD_APK" = true ]; then
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  步骤 2: 编译 Android APK${NC}"
    echo -e "${CYAN}========================================${NC}"

    ANDROID_DIR="$ROOT_DIR/android"
    if [ ! -d "$ANDROID_DIR" ]; then
        exit_with_error "找不到 android 目录: $ANDROID_DIR"
    fi

    pushd "$ANDROID_DIR" > /dev/null

    echo -e "${GRAY}[信息] 当前目录: $(pwd)${NC}"
    echo -e "${YELLOW}[信息] 清理项目...${NC}"
    ./gradlew clean

    echo -e "${YELLOW}[信息] 编译 Debug APK...${NC}"
    ./gradlew assembleDebug

    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  构建成功!${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
    echo -e "${GREEN}[✓] APK 位置: android/app/build/outputs/apk/debug/app-debug.apk${NC}"
    echo ""

    if [ "$INSTALL_APK" = true ]; then
        echo -e "${YELLOW}[信息] 正在安装 APK...${NC}"
        ./gradlew installDebug
        echo -e "${GREEN}[✓] APK 已成功安装到设备${NC}"
    else
        read -p "是否安装到设备? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo -e "${YELLOW}[信息] 正在安装 APK...${NC}"
            ./gradlew installDebug
            echo -e "${GREEN}[✓] APK 已成功安装到设备${NC}"
        fi
    fi

    popd > /dev/null
else
    echo ""
    echo -e "${YELLOW}[跳过] 跳过 APK 编译${NC}"
fi

echo ""
echo -e "${GREEN}[完成] 构建流程结束${NC}"