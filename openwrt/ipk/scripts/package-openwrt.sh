#!/bin/bash
# 打包cloud-clipboard为OpenWrt IPK

set -e

# 参数处理
VERSION=$1
ARCH=$2

if [ -z "$VERSION" ] || [ -z "$ARCH" ]; then
    echo "用法: $0 <版本号> <架构>"
    echo "支持的架构: x86_64, i386, arm_cortex-a7, arm_cortex-a15_neon-vfpv4, aarch64, mips_24kc, mipsel_24kc"
    exit 1
fi

# 目录定义
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
ROOT_DIR="$(dirname "$BASE_DIR")"
BINARY="$BASE_DIR/ipk/build/cloud-clipboard-$VERSION-$ARCH"
PKG_DIR="$BASE_DIR/ipk/build/pkg-$ARCH"
CONTROL_DIR="$BASE_DIR/ipk/control"
ROOTFS_DIR="$BASE_DIR/ipk/rootfs"
IPK_NAME="cloud-clipboard_${VERSION}_${ARCH}.ipk"

# 调试输出
echo "脚本目录: $SCRIPT_DIR"
echo "基础目录: $BASE_DIR"
echo "根目录: $ROOT_DIR"
echo "二进制文件: $BINARY"

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
mkdir -p "$PKG_DIR/CONTROL"

# 复制文件
echo "复制文件..."
cp "$BINARY" "$PKG_DIR/usr/bin/cloud-clipboard"
chmod 755 "$PKG_DIR/usr/bin/cloud-clipboard"

# 复制脚本和配置
echo "复制脚本和配置文件..."
if [ -d "$ROOTFS_DIR" ] && [ -f "$ROOTFS_DIR/etc/init.d/cloud-clipboard" ]; then
    cp "$ROOTFS_DIR/etc/init.d/cloud-clipboard" "$PKG_DIR/etc/init.d/"
    chmod 755 "$PKG_DIR/etc/init.d/cloud-clipboard"
    echo "✓ 已复制初始化脚本"
else
    echo "! 找不到初始化脚本，创建默认脚本"
    cat > "$PKG_DIR/etc/init.d/cloud-clipboard" << 'EOF'
#!/bin/sh /etc/rc.common

START=99
USE_PROCD=1
PROG=/usr/bin/cloud-clipboard
LOG_FILE="/var/log/cloud-clipboard.log"

start_service() {
    config_load cloud-clipboard
    
    local enabled
    local host
    local port
    local auth
    
    config_get enabled main enabled "0"
    [ "$enabled" = "0" ] && return 1
    
    config_get host main host "0.0.0.0"
    config_get port main port "9501"
    config_get auth main auth ""
    
    mkdir -p /tmp/cloud-clipboard
    touch "$LOG_FILE"
    
    procd_open_instance
    procd_set_param command $PROG -host $host -port $port
    [ -n "$auth" ] && procd_append_param command -auth "$auth"
    
    # 重定向输出到日志文件
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_set_param file "$LOG_FILE"
    procd_set_param respawn
    
    procd_close_instance
    
    echo "$(date): Cloud Clipboard started on $host:$port" >> "$LOG_FILE"
}

service_triggers() {
    procd_add_reload_trigger "cloud-clipboard"
}

EOF
    chmod 755 "$PKG_DIR/etc/init.d/cloud-clipboard"
    echo "✓ 已创建默认初始化脚本"
fi

if [ -d "$ROOTFS_DIR" ] && [ -f "$ROOTFS_DIR/etc/config/cloud-clipboard" ]; then
    cp "$ROOTFS_DIR/etc/config/cloud-clipboard" "$PKG_DIR/etc/config/"
    echo "✓ 已复制配置文件"
else
    echo "! 找不到配置文件，创建默认配置"
    cat > "$PKG_DIR/etc/config/cloud-clipboard" << 'EOF'
config cloud-clipboard 'main'
    option enabled '1'
    option host '0.0.0.0'
    option port '9501'
    option auth ''

EOF
    echo "✓ 已创建默认配置文件"
fi

# 处理控制文件
echo "准备控制文件..."
if [ -d "$CONTROL_DIR" ] && [ -f "$CONTROL_DIR/control" ]; then
    sed "s/{{VERSION}}/$VERSION/g; s/{{ARCH}}/$ARCH/g" \
        "$CONTROL_DIR/control" > "$PKG_DIR/CONTROL/control"
    echo "✓ 已处理控制文件"
else
    echo "! 找不到控制文件模板，创建默认控制文件"
    cat > "$PKG_DIR/CONTROL/control" << EOF
Package: cloud-clipboard
Version: $VERSION
Depends: libc
Source: https://github.com/jonnyan404/cloud-clipboard-go
License: MIT
Section: net
Architecture: $ARCH
Maintainer: jonnyan404
Description: Cloud clipboard application for transferring text and files between devices

EOF
    echo "✓ 已创建默认控制文件"
fi

if [ -d "$CONTROL_DIR" ] && [ -f "$CONTROL_DIR/postinst" ]; then
    cp "$CONTROL_DIR/postinst" "$PKG_DIR/CONTROL/postinst"
    chmod 755 "$PKG_DIR/CONTROL/postinst"
    echo "✓ 已复制postinst脚本"
else
    echo "! 找不到postinst脚本，创建默认脚本"
    cat > "$PKG_DIR/CONTROL/postinst" << 'EOF'
#!/bin/sh
/etc/init.d/cloud-clipboard enable
exit 0

EOF
    chmod 755 "$PKG_DIR/CONTROL/postinst"
    echo "✓ 已创建默认postinst脚本"
fi

if [ -d "$CONTROL_DIR" ] && [ -f "$CONTROL_DIR/prerm" ]; then
    cp "$CONTROL_DIR/prerm" "$PKG_DIR/CONTROL/prerm"
    chmod 755 "$PKG_DIR/CONTROL/prerm"
    echo "✓ 已复制prerm脚本"
else
    echo "! 找不到prerm脚本，创建默认脚本"
    cat > "$PKG_DIR/CONTROL/prerm" << 'EOF'
#!/bin/sh
/etc/init.d/cloud-clipboard disable
/etc/init.d/cloud-clipboard stop
exit 0

EOF
    chmod 755 "$PKG_DIR/CONTROL/prerm"
    echo "✓ 已创建默认prerm脚本"
fi

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