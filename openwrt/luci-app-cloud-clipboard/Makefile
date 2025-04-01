include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-cloud-clipboard
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_MAINTAINER:=jonnyan404
PKG_LICENSE:=MIT

LUCI_TITLE:=LuCI support for Cloud Clipboard
LUCI_DEPENDS:=+luci-base +cloud-clipboard +rpcd +luci-compat
LUCI_PKGARCH:=all

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot signature