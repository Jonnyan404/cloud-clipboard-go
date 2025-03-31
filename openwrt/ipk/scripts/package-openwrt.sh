#!/bin/bash
# 打包cloud-clipboard为OpenWrt IPK

set -e

# 参数处理
VERSION=$1
ARCH=$2

if [ -z "$VERSION" ] || [ -z "$ARCH" ]; then
    echo "用法: $0 <版本号> <架构>"
    echo "支持的架构: amd64, i386, arm-7, aarch64, mips, mipsel"
    exit 1
fi

# 目录定义
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
BINARY="$BASE_DIR/ipk/build/cloud-clipboard-$VERSION-$ARCH"
PKG_DIR="$BASE_DIR/ipk/build/pkg-$ARCH"
CONTROL_DIR="$BASE_DIR/ipk/control"
ROOTFS_DIR="$BASE_DIR/ipk/rootfs"
IPK_NAME="cloud-clipboard_${VERSION}_${ARCH}.ipk"

# 检查二进制文件
if [ ! -f "$BINARY" ]; then
    echo "错误: 找不到二进制文件 $BINARY"
    echo "请先运行 build.sh 脚本构建二进制文件"
    exit 1
fi

echo "=== 打包 Cloud Clipboard $VERSION 为 OpenWrt IPK ($ARCH) ==="

# 清理并创建包目录
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/etc/init.d"
mkdir -p "$PKG_DIR/etc/config"
mkdir -p "$PKG_DIR/usr/share/cloud-clipboard"
mkdir -p "$PKG_DIR/CONTROL"

# 复制文件
echo "复制文件..."
cp "$BINARY" "$PKG_DIR/usr/bin/cloud-clipboard"
chmod 755 "$PKG_DIR/usr/bin/cloud-clipboard"

# 复制静态文件
cp -r "$BASE_DIR/../../cloud-clip/static" "$PKG_DIR/usr/share/cloud-clipboard/"

# 复制脚本和配置
cp "$ROOTFS_DIR/etc/init.d/cloud-clipboard" "$PKG_DIR/etc/init.d/"
chmod 755 "$PKG_DIR/etc/init.d/cloud-clipboard"
cp "$ROOTFS_DIR/etc/config/cloud-clipboard" "$PKG_DIR/etc/config/"

# 处理控制文件
echo "准备控制文件..."
sed "s/{{VERSION}}/$VERSION/g; s/{{ARCH}}/$ARCH/g" \
    "$CONTROL_DIR/control" > "$PKG_DIR/CONTROL/control"
cp "$CONTROL_DIR/postinst" "$PKG_DIR/CONTROL/postinst"
chmod 755 "$PKG_DIR/CONTROL/postinst"
cp "$CONTROL_DIR/prerm" "$PKG_DIR/CONTROL/prerm"
chmod 755 "$PKG_DIR/CONTROL/prerm"

# 打包
echo "创建IPK包..."
cd "$PKG_DIR"
tar -czf "$BASE_DIR/ipk/build/data.tar.gz" ./usr ./etc
cd "$PKG_DIR/CONTROL"
tar -czf "$BASE_DIR/ipk/build/control.tar.gz" ./*
cd "$BASE_DIR/ipk/build"
echo "2.0" > debian-binary
tar -czf "$IPK_NAME" ./debian-binary ./control.tar.gz ./data.tar.gz

# 清理
rm -f debian-binary control.tar.gz data.tar.gz

echo "=== IPK打包完成: $BASE_DIR/ipk/build/$IPK_NAME ==="