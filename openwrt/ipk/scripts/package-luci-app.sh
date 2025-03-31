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
ROOT_DIR="$(dirname "$BASE_DIR")"
LUCI_DIR="$ROOT_DIR/openwrt/luci-app-cloud-clipboard"
PKG_DIR="$BASE_DIR/ipk/build/luci-app"
IPK_NAME="luci-app-cloud-clipboard_${VERSION}_all.ipk"

echo "=== 打包 LuCI 应用 $VERSION 为 OpenWrt IPK ==="

# 检查LuCI应用目录是否存在
if [ ! -d "$LUCI_DIR" ]; then
    echo "错误: LuCI应用目录不存在: $LUCI_DIR"
    exit 1
fi

# 清理并创建IPK目录结构
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/CONTROL"

# 复制LuCI应用程序文件
echo "复制LuCI应用文件..."

# 复制root目录结构（如果存在）
if [ -d "$LUCI_DIR/root" ]; then
    cp -r "$LUCI_DIR/root/"* "$PKG_DIR/"
    echo "✓ 已复制root目录结构"
else
    echo "! 警告: 找不到root目录结构"
fi

# 复制htdocs目录结构（如果存在）
if [ -d "$LUCI_DIR/htdocs" ]; then
    mkdir -p "$PKG_DIR/www"
    cp -r "$LUCI_DIR/htdocs/"* "$PKG_DIR/www/"
    echo "✓ 已复制htdocs目录结构"
else
    echo "! 警告: 找不到htdocs目录结构"
fi

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

# 创建安装后脚本
cat > "$PKG_DIR/CONTROL/postinst" << 'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT}" ] || {
    rm -f /tmp/luci-indexcache
    rm -rf /tmp/luci-modulecache/
    exit 0
}
EOF
chmod 755 "$PKG_DIR/CONTROL/postinst"

# 创建卸载前脚本
cat > "$PKG_DIR/CONTROL/prerm" << 'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT}" ] || {
    rm -f /tmp/luci-indexcache
    rm -rf /tmp/luci-modulecache/
    exit 0
}
EOF
chmod 755 "$PKG_DIR/CONTROL/prerm"

# 打包
echo "创建IPK包..."
cd "$PKG_DIR"
tar -czf "$BASE_DIR/ipk/build/data.tar.gz" ./usr ./www ./etc 2>/dev/null || tar -czf "$BASE_DIR/ipk/build/data.tar.gz" ./usr ./www 2>/dev/null || tar -czf "$BASE_DIR/ipk/build/data.tar.gz" ./usr 2>/dev/null
cd "$PKG_DIR/CONTROL"
tar -czf "$BASE_DIR/ipk/build/control.tar.gz" ./*
cd "$BASE_DIR/ipk/build"
echo "2.0" > debian-binary
tar -czf "$IPK_NAME" ./debian-binary ./control.tar.gz ./data.tar.gz

# 清理
rm -f debian-binary control.tar.gz data.tar.gz

echo "=== LuCI应用打包完成: $BASE_DIR/ipk/build/$IPK_NAME ==="