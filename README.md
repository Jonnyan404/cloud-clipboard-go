# cloud-clipboard-go

基于 https://github.com/TransparentLC/cloud-clipboard 项目,用go复刻了一个

在 https://github.com/yurenchen000/cloud-clipboard 基础上增加一些功能

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

### go 版服务端

#### 使用二进制文件运行

去项目 [release](https://github.com/Jonnyan404/cloud-clipboard-go/releases) 下载对应系统文件运行即可

- 命令行参数: `-config` 用来自定义配置文件
- 命令行参数: `-static` 用来自定义加载外部前端文件

#### 使用 Docker 运行

```sh
docker run -d --name=cloud-clipboard-go -p 9502:9502 -v /path/your/dir/data:/app/server-node/data jonnyan404/cloud-clipboard-go
或者
docker run -d --name=cloud-clipboard -p 9502:9502 -v /path/your/dir/data:/app/server-node/data ghcr.io/jonnyan404/cloud-clipboard-go
```

- vi docker-compose.yml

```

services:
    cloud-clipboard-go:
        container_name: cloud-clipboard-go
        restart: always
        ports:
            - "9502:9502"
        environment:
            - LISTEN_IP= #默认为0.0.0.0,可设置为 127.0.0.1 不懂勿动
            - LISTEN_PORT= #默认为9502,可设置为其他端口
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

然后访问 http://127.0.0.1:9502


#### mac用户从 homebrew 运行

```
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


#### 从源代码运行

需要安装 [Node.js>=22.12](https://nodejs.org)。
需要安装 [go>=1.22]

```bash
cd client
npm install
npm run build

# 运行服务端
cd ../cloud-clip
go mod tidy
go run .
```

配置文件是按照以下顺序尝试读取的：

* 和 `main.go` 放在同一目录的 `config.json`
* 在命令行中指定：`./cloud-clipboard-go -config=/tmp/config.json`

服务端默认会监听本机所有网卡的 IP 地址（也可以自己设定），并在终端中显示前端界面所在的网址，使用浏览器打开即可使用。

### 配置文件说明

`//` 开头的部分是注释，**并不需要写入配置文件中**，否则会导致读取失败。

```json
{
    "server": {
        // 监听的 IP 地址，省略或设为 null 则会监听所有网卡的IP地址
        "host": [
            "127.0.0.1",
            "::1"
        ],
        "port": 9501, // 端口号，falsy 值表示不监听
        "uds": "/var/run/cloud-clipboard.sock", // UNIX domain socket 路径，可以后接“:666”设定权限（默认666），falsy 值表示不监听
        "prefix": "", // 部署时的URL前缀，例如想要在 http://localhost/prefix/ 访问，则将这一项设为 /prefix
        "key": "localhost-key.pem", // HTTPS 私钥路径
        "cert": "localhost.pem", // HTTPS 证书路径
        "history": 10, // 消息历史记录的数量
        "auth": false, // 是否在连接时要求使用密码认证，falsy 值表示不使用
        "historyFile": null, // 自定义历史记录存储路径，默认为当前目录的 history.json
        "storageDir": null // 自定义文件存储目录，默认为临时文件夹的.cloud-clipboard-storage目录
    },
    "text": {
        "limit": 4096 // 文本的长度限制
    },
    "file": {
        "expire": 3600, // 上传文件的有效期，超过有效期后自动删除，单位为秒
        "chunk": 1048576, // 上传文件的分片大小，不能超过 5 MB，单位为 byte
        "limit": 104857600 // 上传文件的大小限制，单位为 byte
    }
}
```
> HTTPS 的说明：
>
> 如果使用了 Nginx 等软件的反向代理，且这些软件已经提供了 HTTPS 连接，则无需在这里设定。
>
> “密码认证”的说明：
>
> 如果启用“密码认证”，只有输入正确的密码才能连接到服务端并查看剪贴板内容。
> 可以将 `server.auth` 字段设为 `true`（随机生成六位密码）或字符串（自定义密码）来启用这个功能，启动服务端后终端会以 `Authorization code: ******` 的格式输出当前使用的密码。

# cloud-clipboard-launcher(画饼)

为 cloud-clipboard-go 制作的启动器,方便不想或不会使用终端的用户


# luci-app-cloud-clipboard-go(画饼)

移植为openwrt项目