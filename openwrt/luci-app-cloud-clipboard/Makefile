include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-cloud-clipboard
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_MAINTAINER:=jonnyan404
PKG_LICENSE:=MIT

# 添加以下指定，确保使用新的LuCI结构
PKG_CONFIG_DEPENDS:=CONFIG_LUCI_JSMIN
PKG_BUILD_PARALLEL:=1

LUCI_TITLE:=LuCI support for Cloud Clipboard
LUCI_DEPENDS:=+luci-base +cloud-clipboard
LUCI_PKGARCH:=all
LUCI_DESCRIPTION:=LuCI interface for Cloud Clipboard service

define Package/$(PKG_NAME)/conffiles
/etc/config/cloud-clipboard
endef

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot signature