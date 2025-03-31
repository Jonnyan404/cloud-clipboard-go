# Cloud Clipboard for OpenWrt

## 安装方法

1. 下载与您的OpenWrt设备架构相匹配的IPK文件
   - arm-7: 适用于大多数ARM路由器
   - aarch64: 适用于64位ARM设备（如部分高端路由器）
   - mips/mipsel: 适用于传统路由器
   - i386: 适用于32位x86设备
   - amd64: 适用于64位x86设备

2. 上传到OpenWrt设备

`scp cloud-clipboard_1.0.0_arm-7.ipk root@192.168.1.1:/tmp/`


3. 通过SSH连接到设备并安装

`opkg install /tmp/cloud-clipboard_1.0.0_arm-7.ipk`

4. 配置服务

`vi /etc/config/cloud-clipboard`

将`option enabled`改为`1`以启用服务

5. 启动服务

`/etc/init.d/cloud-clipboard start`


## 打包说明

如果您想自己构建IPK包：

```bash
# 在项目根目录下执行
# 构建二进制文件
./openwrt/ipk/scripts/build.sh 版本号

# 打包IPK
./openwrt/ipk/scripts/package-openwrt.sh 版本号 架构
```

支持的架构：amd64, i386, arm-7, aarch64, mips, mipsel



## LuCI界面

Cloud Clipboard还提供了LuCI界面，可以方便地在OpenWrt管理页面进行控制和配置。

### 安装LuCI界面

1. 安装主应用包（如上方步骤所示）
2. 安装LuCI界面包
`opkg install luci-app-cloud-clipboard_1.0.0_all.ipk`
3. 重启LuCI
`/etc/init.d/uhttpd restart`
4. 在OpenWrt管理界面的"服务"菜单下找到"Cloud Clipboard"

### 通过LuCI控制服务

1. 登录OpenWrt管理界面
2. 进入"服务" > "Cloud Clipboard"
3. 设置服务参数并启用服务
4. 点击保存并应用