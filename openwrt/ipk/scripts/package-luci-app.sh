#!/bin/bash
# 打包LuCI应用为IPK

set -e

VERSION=$1
if [ -z "$VERSION" ]; then
    echo "用法: $0 <版本号>"
    exit 1
fi

# 目录定义
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
LUCI_DIR="$BASE_DIR/luci-app-cloud-clipboard"
PKG_DIR="$BASE_DIR/ipk/build/luci-app"
IPK_NAME="luci-app-cloud-clipboard_${VERSION}_all.ipk"

echo "=== 打包 LuCI 应用 $VERSION 为 OpenWrt IPK ==="

# 清理并创建包目录
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/controller"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/model/cbi"
mkdir -p "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard"
mkdir -p "$PKG_DIR/CONTROL"

# 复制文件
echo "复制文件..."
cp "$LUCI_DIR/luasrc/controller/cloud-clipboard.lua" "$PKG_DIR/usr/lib/lua/luci/controller/"
cp "$LUCI_DIR/luasrc/model/cbi/cloud-clipboard.lua" "$PKG_DIR/usr/lib/lua/luci/model/cbi/"
cp "$LUCI_DIR/luasrc/view/cloud-clipboard/status.htm" "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard/"
cp "$LUCI_DIR/luasrc/view/cloud-clipboard/log.htm" "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard/"

# 创建控制文件
echo "准备控制文件..."
cat > "$PKG_DIR/CONTROL/control" << EOF
Package: luci-app-cloud-clipboard
Version: $VERSION
Depends: luci-base, cloud-clipboard
Source: https://github.com/jonnyan404/cloud-clipboard-go
License: MIT
Section: luci
Architecture: all
Maintainer: jonnyan404
Description: LuCI support for Cloud Clipboard
EOF

# 打包
echo "创建IPK包..."
cd "$PKG_DIR"
tar -czf "$BASE_DIR/ipk/build/data.tar.gz" ./usr
cd "$PKG_DIR/CONTROL"
tar -czf "$BASE_DIR/ipk/build/control.tar.gz" ./control
cd "$BASE_DIR/ipk/build"
echo "2.0" > debian-binary
tar -czf "$IPK_NAME" ./debian-binary ./control.tar.gz ./data.tar.gz

# 清理
rm -f debian-binary control.tar.gz data.tar.gz

echo "=== LuCI应用打包完成: $BASE_DIR/ipk/build/$IPK_NAME ==="