# Cloud Clipboard for OpenWrt

## 安装方法

1. 根据您的 OpenWrt 版本下载对应格式的软件包
   - OpenWrt 25.12 及以上: 下载与设备 `cat /etc/apk/arch` 输出匹配的 APK 文件
   - OpenWrt 24.10 及以下: 下载与设备架构匹配的 IPK 文件
   - arm-7: 适用于大多数ARM路由器
   - aarch64: 适用于64位ARM设备（如部分高端路由器）
   - mips/mipsel: 适用于传统路由器
   - i386: 适用于32位x86设备
   - amd64: 适用于64位x86设备

2. 上传到OpenWrt设备

OpenWrt 25.12 及以上:

`scp cloud-clipboard-1.0.0-aarch64_cortex-a53.apk root@192.168.1.1:/tmp/`

OpenWrt 24.10 及以下:

`scp cloud-clipboard_1.0.0_arm_cortex-a7.ipk root@192.168.1.1:/tmp/`


3. 通过SSH连接到设备并安装

OpenWrt 25.12 及以上:

`apk add --allow-untrusted /tmp/cloud-clipboard-1.0.0-aarch64_cortex-a53.apk`

OpenWrt 24.10 及以下:

`opkg install /tmp/cloud-clipboard_1.0.0_arm_cortex-a7.ipk`

4. 配置服务

`vi /etc/config/cloud-clipboard`

将`option enabled`改为`1`以启用服务

如需使用按房间密码,请编辑 `/etc/cloud-clipboard/config.json`，在 `server` 下加入 `roomAuth`，例如：

```json
{
   "server": {
      "auth": true,
      "roomAuth": {
         "private": "",
         "finance": "finance-pass"
      }
   }
}
```

其中空字符串表示该房间沿用全局 `auth`，非空字符串表示该房间使用独立密码。

5. 启动服务

`/etc/init.d/cloud-clipboard start`


## 打包说明

如果您想自己构建 OpenWrt 包:

```bash
# 在项目根目录下执行
# 构建二进制文件
./openwrt/scripts/build.sh 版本号

# 打包 IPK
./openwrt/scripts/package-openwrt.sh 版本号 架构

# 打包 APK，需要 apk-tools 或 Docker
./openwrt/scripts/package-openwrt-apk.sh 版本号 二进制架构 OpenWrt包架构

# 例如在设备上 `cat /etc/apk/arch` 得到 aarch64_cortex-a53
./openwrt/scripts/package-openwrt-apk.sh 版本号 aarch64 aarch64_cortex-a53
```

OpenWrt APK 包架构不能直接等同于 Go 二进制架构，请以设备上的 `/etc/apk/arch` 为准。



## LuCI界面

Cloud Clipboard还提供了LuCI界面，可以方便地在OpenWrt管理页面进行控制和配置。

### 安装LuCI界面

1. 安装主应用包（如上方步骤所示）
2. 安装LuCI界面包
OpenWrt 25.12 及以上:
`apk add --allow-untrusted /tmp/luci-app-cloud-clipboard-1.0.0-noarch.apk`

OpenWrt 24.10 及以下:
`opkg install /tmp/luci-app-cloud-clipboard_1.0.0_all.ipk`
3. 重启LuCI
`/etc/init.d/uhttpd restart`
4. 在OpenWrt管理界面的"服务"菜单下找到"Cloud Clipboard"

### 通过LuCI控制服务

1. 登录OpenWrt管理界面
2. 进入"服务" > "Cloud Clipboard"
3. 设置服务参数并启用服务
4. 点击保存并应用