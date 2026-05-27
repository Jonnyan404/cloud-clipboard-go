#!/bin/bash
# 打包 LuCI 应用为 OpenWrt APK

set -euo pipefail

CONTAINER_BASE_DIR=/workspace/openwrt
USE_DOCKER_APK=0

VERSION=${1:-}
if [ -z "$VERSION" ]; then
    echo "用法: $0 <版本号>"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$SCRIPT_DIR")"
LUCI_DIR="$BASE_DIR/luci-app-cloud-clipboard"
PKG_DIR="$BASE_DIR/build/luci-app-apk"
APK_NAME="luci-app-cloud-clipboard-${VERSION}-noarch.apk"
TEMP_DIR="$BASE_DIR/build/.tmp-luci-app-apk-$$"

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

resolve_apk_runner

cleanup() {
    rm -rf "$TEMP_DIR"
}

trap cleanup EXIT

rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR"

echo "=== 打包 LuCI 应用 $VERSION 为 OpenWrt APK ==="
echo "SCRIPT_DIR: $SCRIPT_DIR"
echo "BASE_DIR: $BASE_DIR"
echo "LUCI_DIR: $LUCI_DIR"

if [ ! -d "$LUCI_DIR" ]; then
    echo "错误: LuCI应用目录不存在: $LUCI_DIR"
    exit 1
fi

if [ ! -d "$LUCI_DIR/luasrc" ]; then
    echo "错误: LuCI应用luasrc目录不存在: $LUCI_DIR/luasrc"
    exit 1
fi

rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/model/cbi"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/controller"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard"

echo "复制LuCI应用文件..."
cp -v "$LUCI_DIR/luasrc/model/cbi/"*.lua "$PKG_DIR/usr/lib/lua/luci/model/cbi/"
cp -v "$LUCI_DIR/luasrc/controller/"*.lua "$PKG_DIR/usr/lib/lua/luci/controller/"
cp -v "$LUCI_DIR/luasrc/view/cloud-clipboard/"*.htm "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard/"

if [ -d "$LUCI_DIR/root" ]; then
    cp -r "$LUCI_DIR/root/"* "$PKG_DIR/"
    rm -f "$PKG_DIR/etc/config/cloud-clipboard"
    echo "✓ 已复制root目录结构"
else
    echo "! 警告: 找不到root目录结构"
fi

if [ -d "$LUCI_DIR/htdocs" ]; then
    mkdir -p "$PKG_DIR/www"
    cp -r "$LUCI_DIR/htdocs/"* "$PKG_DIR/www/"
    echo "✓ 已复制htdocs目录结构"
else
    echo "! 警告: 找不到htdocs目录结构"
fi

cat > "$TEMP_DIR/post-install" << 'EOF'
#!/bin/sh
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache/
/etc/init.d/rpcd reload 2>/dev/null || true
exit 0
EOF
chmod 755 "$TEMP_DIR/post-install"

cat > "$TEMP_DIR/pre-deinstall" << 'EOF'
#!/bin/sh
rm -f /tmp/luci-indexcache
rm -rf /tmp/luci-modulecache/
exit 0
EOF
chmod 755 "$TEMP_DIR/pre-deinstall"

cp "$TEMP_DIR/post-install" "$TEMP_DIR/post-upgrade"
chmod 755 "$TEMP_DIR/post-upgrade"

BUILD_TIME=$(date +%s)
APK_FILES_DIR=$PKG_DIR
APK_OUTPUT_PATH="$BASE_DIR/build/$APK_NAME"
APK_POSTINST="$TEMP_DIR/post-install"
APK_PRERM="$TEMP_DIR/pre-deinstall"
APK_POSTUPGRADE="$TEMP_DIR/post-upgrade"

if [ "$USE_DOCKER_APK" -eq 1 ]; then
    APK_FILES_DIR=$(to_container_path "$PKG_DIR")
    APK_OUTPUT_PATH=$(to_container_path "$BASE_DIR/build/$APK_NAME")
    APK_POSTINST=$(to_container_path "$TEMP_DIR/post-install")
    APK_PRERM=$(to_container_path "$TEMP_DIR/pre-deinstall")
    APK_POSTUPGRADE=$(to_container_path "$TEMP_DIR/post-upgrade")
fi

run_apk_mkpkg \
    --files "$APK_FILES_DIR" \
    --output "$APK_OUTPUT_PATH" \
    --info "name:luci-app-cloud-clipboard" \
    --info "version:$VERSION-r1" \
    --info "description:LuCI support for Cloud Clipboard" \
    --info "arch:noarch" \
    --info "license:MIT" \
    --info "url:https://github.com/jonnyan404/cloud-clipboard-go" \
    --info "origin:luci-app-cloud-clipboard" \
    --info "maintainer:jonnyan404" \
    --info "build-time:$BUILD_TIME" \
    --info "depends:libc" \
    --info "depends:luci-base" \
    --info "depends:cloud-clipboard" \
    --info "depends:rpcd" \
    --info "depends:luci-compat" \
    --script "post-install:$APK_POSTINST" \
    --script "pre-deinstall:$APK_PRERM" \
    --script "post-upgrade:$APK_POSTUPGRADE"

echo "=== LuCI APK打包完成: $BASE_DIR/build/$APK_NAME ==="