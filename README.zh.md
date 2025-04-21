# cloud-clipboard-go

<p>
  <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue.svg" alt="English Readme"></a>
  <a href="https://raw.githubusercontent.com/jonnyan404/cloud-clipboard-go-launcher/main/LICENSE">
    <img src="https://img.shields.io/github/license/jonnyan404/cloud-clipboard-go-launcher?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/v/release/jonnyan404/cloud-clipboard-go?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/jonnyan404/cloud-clipboard-go/releases/latest">
    <img src="https://img.shields.io/github/downloads/jonnyan404/cloud-clipboard-go/total?color=brightgreen&include_prereleases" alt="release">
  </a>
</p>

基于 [TransparentLC](https://github.com/TransparentLC/cloud-clipboard) 项目,用go复刻了一个

在 [yurenchen000](https://github.com/yurenchen000/cloud-clipboard) 基础上增加一些功能

## 截图

<details>
<summary>桌面端</summary>

![](https://ae01.alicdn.com/kf/Hfce3a9b69b3d404c8e3073ab0fffa913v.png)

</details>

<details>
<summary>移动端</summary>

![](https://ae01.alicdn.com/kf/Hbf859dd0e42c4406bf94a6b6f2f4658cf.png)

</details>

## 使用方法

这里是 go 版服务端

### 傻瓜式运行(UI辅助器,推荐小白用户们)

<details>
    <summary>点击查看预览图</summary>

![](https://github.com/Jonnyan404/cloud-clipboard-go-launcher/blob/main/demo.png)

</details>

去 [UI辅助器](https://github.com/Jonnyan404/cloud-clipboard-go-launcher/releases) 下载后,双击使用

### 使用 Docker 运行

```sh
# dockerhub镜像(二选一)
docker run -d --name=cloud-clipboard-go -p 9501:9501 -v /path/your/dir/data:/app/server-node/data jonnyan404/cloud-clipboard-go
# github镜像(二选一)
docker run -d --name=cloud-clipboard-go -p 9501:9501 -v /path/your/dir/data:/app/server-node/data ghcr.io/jonnyan404/cloud-clipboard-go
```

- vi docker-compose.yml

```

services:
    cloud-clipboard-go:
        container_name: cloud-clipboard-go
        restart: always
        ports:
            - "9501:9501"
        environment:
            - LISTEN_IP= #默认为0.0.0.0,可设置为 127.0.0.1 不懂勿动
            - LISTEN_PORT= #默认为9501,可设置为其他端口
            - PREFIX= #子路径,可配合nginx使用,格式: /cloud-clipboard
            - MESSAGE_NUM= #历史记录的数量,默认为10
            - AUTH_PASSWORD= #访问密码,默认为false,可自定义字符串密码
            - TEXT_LIMIT= #文本长度限制,默认为4096(2048个汉字),可设置为其他长度
            - FILE_EXPIRE= #文件过期时间,默认为3600(1小时),可设置为其他时间,单位为秒
            - FILE_LIMIT= #文件大小限制,默认为104857600(100MB),可设置为其他大小,单位为字节
        volumes:
            - /path/your/dir/data:/app/server-node/data #请注意修改为你自己的目录
        image: jonnyan404/cloud-clipboard-go:latest

```

- 访问主页: http://127.0.0.1:9501
- 访问 http://127.0.0.1:9501/content/latest 将永远返回最新的一条内容,可添加房间参数`?room=xxx`


### 使用 homebrew 运行

> 已知问题:brew services 无法tab补全,参考:https://github.com/orgs/Homebrew/discussions/6047#discussioncomment-12668536

默认配置文件分别在`homebrew`根目录下的`etc/cloud-clipboard-go`和`var`目录

```
brew update
#安装
brew install Jonnyan404/tap/cloud-clipboard-go
# 启动服务
brew services start cloud-clipboard-go
# 查看服务状态
brew services info cloud-clipboard-go
# 停止服务
brew services stop cloud-clipboard-go
# 重启服务
brew services restart cloud-clipboard-go
```

### 使用 OpenWrt 运行


✅ OpenWrt 24.10.0 测试通过

查看所属架构命令: `opkg print-architecture` (最后一行第二列就是)

去 https://github.com/Jonnyan404/cloud-clipboard-go/releases 的`pre-release`下载对应系统的`*平台.ipk`文件和`*_all.ipk`文件

然后在命令行执行下列命令
```
opkg install *平台.ipk
opkg install *_all.ipk
```

<details>
    <summary>点击预览luci界面</summary>

![](https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/openwrt/demo.png)

</details>


### 使用二进制文件运行

去项目 [release](https://github.com/Jonnyan404/cloud-clipboard-go/releases) 下载对应系统文件运行即可

> 参数优先级: 命令行参数 > 配置文件

- 命令行参数: `-host` 用来自定义服务器监听地址
- 命令行参数: `-port` 用来自定义服务器监听端口
- 命令行参数: `-auth` 用来自定义密码
- 命令行参数: `-config` 用来加载自定义配置文件
- 命令行参数: `-static` 用来加载自定义外部前端文件



### 使用源代码运行

- 需要安装 [Node.js>=22.12](https://nodejs.org)。
- 需要安装 [go>=1.22](https://go.dev/)

```bash
cd client
npm install
npm run build

# 运行服务端
cd ../cloud-clip
go mod tidy
go run .
```



### 配置文件说明

- 请查看: https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/cloud-clip/config.md

### HTTP API

- 请查看: https://github.com/Jonnyan404/cloud-clipboard-go/blob/main/cloud-clip/config.md

## 衍生项目

- 为 cloud-clipboard-go 制作的启动器,方便不想或不会使用终端的用户,[cloud-clipboard-go-launcher](https://github.com/Jonnyan404/cloud-clipboard-go-launcher)


