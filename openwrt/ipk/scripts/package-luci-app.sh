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
mkdir -p "$PKG_DIR/usr/share/luci/menu.d"
mkdir -p "$PKG_DIR/usr/share/rpcd/acl.d"
mkdir -p "$PKG_DIR/www/luci-static/resources/view/cloud-clipboard"
mkdir -p "$PKG_DIR/usr/libexec/rpcd/luci"

# 复制文件
echo "复制文件..."
cp "$LUCI_DIR/luasrc/controller/cloud-clipboard.lua" "$PKG_DIR/usr/lib/lua/luci/controller/"
cp "$LUCI_DIR/luasrc/model/cbi/cloud-clipboard.lua" "$PKG_DIR/usr/lib/lua/luci/model/cbi/"
cp "$LUCI_DIR/luasrc/view/cloud-clipboard/status.htm" "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard/"
cp "$LUCI_DIR/luasrc/view/cloud-clipboard/log.htm" "$PKG_DIR/usr/lib/lua/luci/view/cloud-clipboard/"

# 添加新的菜单描述文件
cat > "$PKG_DIR/usr/share/luci/menu.d/luci-app-cloud-clipboard.json" << EOF
{
    "admin/services/cloud-clipboard": {
        "title": "Cloud Clipboard",
        "order": 100,
        "action": {
            "type": "alias",
            "path": "admin/services/cloud-clipboard/settings"
        }
    },
    "admin/services/cloud-clipboard/settings": {
        "title": "Settings",
        "order": 10,
        "action": {
            "type": "view",
            "path": "cloud-clipboard/settings"
        }
    }
}

EOF

# 添加 ACL 文件
cat > "$PKG_DIR/usr/share/rpcd/acl.d/luci-app-cloud-clipboard.json" << EOF
{
    "luci-app-cloud-clipboard": {
        "description": "Grant access to cloud-clipboard settings",
        "read": {
            "file": {
                "/var/log/cloud-clipboard.log": [ "read" ]
            },
            "uci": [ "cloud-clipboard" ],
            "ubus": {
                "service": [ "list" ]
            }
        },
        "write": {
            "uci": [ "cloud-clipboard" ]
        }
    }
}

EOF

# 添加 RPCD 脚本
cat > "$PKG_DIR/usr/libexec/rpcd/luci/cloud-clipboard" << 'EOF'
#!/bin/sh
case "$1" in
    list)
        echo '{"status":{"description":"Get cloud-clipboard service status"}}'
    ;;
    call)
        case "$2" in
            status)
                echo '{"status":'
                if pgrep -f cloud-clipboard >/dev/null; then
                    echo '"running"}'
                else
                    echo '"stopped"}'
                fi
            ;;
        esac
    ;;
esac
EOF

chmod +x "$PKG_DIR/usr/libexec/rpcd/luci/cloud-clipboard"

# 添加设置页面脚本
cat > "$PKG_DIR/www/luci-static/resources/view/cloud-clipboard/settings.js" << 'EOF'
'use strict';
'require view';
'require form';
'require uci';
'require fs';

return view.extend({
    render: function() {
        var m, s, o;

        m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), _('Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。'));

        s = m.section(form.TypedSection, 'cloud-clipboard', _('基本设置'));
        s.anonymous = true;

        o = s.option(form.Flag, 'enabled', _('启用'));
        o.rmempty = false;

        o = s.option(form.Value, 'host', _('监听地址'));
        o.datatype = 'ip4addr';
        o.default = '0.0.0.0';
        o.rmempty = false;

        o = s.option(form.Value, 'port', _('监听端口'));
        o.datatype = 'port';
        o.default = '9501';
        o.rmempty = false;

        o = s.option(form.Value, 'auth', _('访问密码'));
        o.password = true;
        o.rmempty = true;
        o.description = _('如果设置，访问时需要输入此密码。留空表示不需要密码。');

        return m.render();
    }
});
EOF

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