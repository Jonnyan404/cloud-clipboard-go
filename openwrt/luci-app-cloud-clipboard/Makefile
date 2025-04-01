include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-cloud-clipboard
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_MAINTAINER:=jonnyan404
PKG_LICENSE:=MIT

LUCI_TITLE:=LuCI support for Cloud Clipboard
LUCI_DEPENDS:=+luci-base +cloud-clipboard +rpcd +luci-compat
LUCI_PKGARCH:=all

# 添加自定义安装目录
define Package/$(PKG_NAME)/install
    $(INSTALL_DIR) $(1)/usr/lib/lua/luci/controller
    $(INSTALL_DATA) ./luasrc/controller/cloud-clipboard.lua $(1)/usr/lib/lua/luci/controller/
    
    $(INSTALL_DIR) $(1)/usr/lib/lua/luci/model/cbi
    $(INSTALL_DATA) ./luasrc/model/cbi/cloud-clipboard.lua $(1)/usr/lib/lua/luci/model/cbi/
    
    $(INSTALL_DIR) $(1)/usr/lib/lua/luci/view/cloud-clipboard
    $(INSTALL_DATA) ./luasrc/view/cloud-clipboard/*.htm $(1)/usr/lib/lua/luci/view/cloud-clipboard/
    
    $(INSTALL_DIR) $(1)/etc/config
    $(INSTALL_CONF) ./root/etc/config/cloud-clipboard $(1)/etc/config/
    
    $(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
    $(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-cloud-clipboard.json $(1)/usr/share/rpcd/acl.d/
endef

# 这两行标准的LUCI构建命令
include $(TOPDIR)/feeds/luci/luci.mk
# call BuildPackage - OpenWrt buildroot signature