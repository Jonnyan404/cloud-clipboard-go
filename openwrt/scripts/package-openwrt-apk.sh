#!/bin/bash
# 打包 cloud-clipboard 为 OpenWrt APK

set -euo pipefail

CONTAINER_BASE_DIR=/workspace/openwrt
USE_DOCKER_APK=0

VERSION=${1:-}
ARCH=${2:-}
PACKAGE_ARCH=${3:-${OPENWRT_PKG_ARCH:-}}

if [ -z "$VERSION" ] || [ -z "$ARCH" ]; then
    echo "用法: $0 <版本号> <二进制架构> [OpenWrt APK架构]"
    echo "二进制架构: x86_64, i386, arm_cortex-a5, arm_cortex-a7, arm_cortex-a8, arm_cortex-a9, arm_cortex-a15_neon-vfpv4, aarch64, mips_24kc, mipsel_24kc"
    echo "示例: $0 0.1 aarch64 aarch64_cortex-a53"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$SCRIPT_DIR")"
EXPECTED_BINARY="$BASE_DIR/build/cloud-clipboard-$VERSION-$ARCH"
LEGACY_BINARY="$BASE_DIR/build/cloud-clipboard--$ARCH"
BINARY="$EXPECTED_BINARY"
PKG_DIR="$BASE_DIR/build/apk-$ARCH"
ROOTFS_DIR="$BASE_DIR/ipk/rootfs"
CONTROL_DIR="$BASE_DIR/ipk/control"

resolve_package_arch() {
    if [ -n "$PACKAGE_ARCH" ]; then
        return
    fi

    case "$ARCH" in
        x86_64|mips_24kc|mipsel_24kc|arm_cortex-a7|arm_cortex-a9|arm_cortex-a15_neon-vfpv4)
            PACKAGE_ARCH="$ARCH"
            ;;
        *)
            echo "错误: 无法从二进制架构 $ARCH 自动推断 OpenWrt APK 架构。"
            echo "请显式传入第三个参数，例如: $0 $VERSION $ARCH aarch64_cortex-a53"
            echo "可在设备上执行: cat /etc/apk/arch"
            exit 1
            ;;
    esac
}

resolve_package_arch

APK_NAME="cloud-clipboard-${VERSION}-${PACKAGE_ARCH}.apk"

to_container_path() {
    local host_path=$1
    printf '%s\n' "${host_path/$BASE_DIR/$CONTAINER_BASE_DIR}"
}

resolve_apk_runner() {
    if command -v apk >/dev/null 2>&1; then
        return
    fi

    if command -v docker >/dev/null 2>&1; then
        USE_DOCKER_APK=1
        echo "未找到本地 apk，改用 Docker 中的 Alpine apk-tools"
        return
    fi

    echo "错误: 未找到 apk 命令，也未找到 docker。请安装 apk-tools 或 Docker。"
    exit 1
}

run_apk_mkpkg() {
    if [ "$USE_DOCKER_APK" -eq 0 ]; then
        apk mkpkg "$@"
        return
    fi

    docker run --rm \
        -v "$BASE_DIR:$CONTAINER_BASE_DIR" \
        -w "$CONTAINER_BASE_DIR" \
        alpine:edge \
        sh -lc 'exec apk mkpkg "$@"' \
        sh "$@"
}

resolve_binary() {
    if [ -f "$EXPECTED_BINARY" ]; then
        BINARY="$EXPECTED_BINARY"
        return
    fi

    if [ -f "$LEGACY_BINARY" ]; then
        BINARY="$LEGACY_BINARY"
        echo "警告: 找到旧的无版本二进制 ${LEGACY_BINARY}，建议重新运行 build.sh 生成带版本号的产物"
        return
    fi
}

resolve_apk_runner
resolve_binary

echo "脚本目录: $SCRIPT_DIR"
echo "根目录: $BASE_DIR"
echo "二进制文件: $BINARY"
echo "APK包架构: $PACKAGE_ARCH"

if [ ! -f "$BINARY" ]; then
    echo "错误: 找不到二进制文件 $BINARY"
    echo "请先运行 build.sh 脚本构建二进制文件"
    exit 1
fi

echo "=== 打包 Cloud Clipboard $VERSION 为 OpenWrt APK ($ARCH) ==="

rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/etc/init.d"
mkdir -p "$PKG_DIR/etc/config"

echo "复制文件..."
cp "$BINARY" "$PKG_DIR/usr/bin/cloud-clipboard"
chmod 755 "$PKG_DIR/usr/bin/cloud-clipboard"

if [ -f "$ROOTFS_DIR/etc/init.d/cloud-clipboard" ]; then
    cp "$ROOTFS_DIR/etc/init.d/cloud-clipboard" "$PKG_DIR/etc/init.d/"
    chmod 755 "$PKG_DIR/etc/init.d/cloud-clipboard"
    echo "✓ 已复制初始化脚本"
else
    echo "错误: 找不到初始化脚本"
    exit 1
fi

if [ -f "$ROOTFS_DIR/etc/config/cloud-clipboard" ]; then
    cp "$ROOTFS_DIR/etc/config/cloud-clipboard" "$PKG_DIR/etc/config/"
    echo "✓ 已复制配置文件"
else
    echo "错误: 找不到配置文件"
    exit 1
fi

if [ ! -f "$CONTROL_DIR/postinst" ] || [ ! -f "$CONTROL_DIR/prerm" ]; then
    echo "错误: 找不到安装脚本模板"
    exit 1
fi

BUILD_TIME=$(date +%s)
APK_FILES_DIR=$PKG_DIR
APK_OUTPUT_PATH="$BASE_DIR/build/$APK_NAME"
APK_POSTINST="$CONTROL_DIR/postinst"
APK_PRERM="$CONTROL_DIR/prerm"

if [ "$USE_DOCKER_APK" -eq 1 ]; then
    APK_FILES_DIR=$(to_container_path "$PKG_DIR")
    APK_OUTPUT_PATH=$(to_container_path "$BASE_DIR/build/$APK_NAME")
    APK_POSTINST=$(to_container_path "$CONTROL_DIR/postinst")
    APK_PRERM=$(to_container_path "$CONTROL_DIR/prerm")
fi

run_apk_mkpkg \
    --files "$APK_FILES_DIR" \
    --output "$APK_OUTPUT_PATH" \
    --info "name:cloud-clipboard" \
    --info "version:$VERSION-r1" \
    --info "description:Cloud clipboard application for transferring text and files between devices" \
    --info "arch:$PACKAGE_ARCH" \
    --info "license:MIT" \
    --info "url:https://github.com/jonnyan404/cloud-clipboard-go" \
    --info "origin:cloud-clipboard" \
    --info "maintainer:jonnyan404" \
    --info "build-time:$BUILD_TIME" \
    --info "depends:libc" \
    --script "post-install:$APK_POSTINST" \
    --script "pre-deinstall:$APK_PRERM"

echo "=== APK打包完成: $BASE_DIR/build/$APK_NAME ==="