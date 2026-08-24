

### 配置文件说明

`//` 开头的部分是注释，**并不需要写入配置文件中**，否则会导致读取失败。

```json
{
    "server": {
        // 监听的 IP 地址，省略或设为 null 则会监听所有网卡的IP地址
        "host": [
            "127.0.0.1"
        ],
        "port": 9501, // 端口号，falsy 值表示不监听
        "prefix": "", // 部署时的URL前缀，例如想要在 http://localhost/prefix/ 访问，则将这一项设为 /prefix
        "history": 10, // 消息历史记录的数量
        "auth": false, // 全局入口密码。只要设置了就对所有房间生效，保证旧密码升级后仍可用
        "roomAuth": {
            "private": "", // 空字符串表示该房间只接受全局 auth
            "finance": "finance-pass", // 非空字符串表示该房间额外接受独立密码
            "keep": {"password": "kp", "fileExpire": 0}, // 对象形式：password 为房间密码；fileExpire: 0=该房间文件永不过期，>0=覆盖 file.expire 秒数，不填=使用全局 file.expire
            "archive": {"fileExpire": 604800} // 也可以只配置 fileExpire（无独立密码）
        },
        "historyFile": null, // 自定义历史记录存储路径，默认为当前目录的 history.json
        "storageDir": null, // 自定义文件存储目录，默认为临时文件夹的.cloud-clipboard-storage目录
        "roomList": false, // 房间列表开关,默认false
        "roomCleanup": 3600 //房间清理周期(秒)，清理消息数0的房间
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
> 建议使用 nginx/caddy 来反向代理
>
> “密码认证”的说明：
>
> 如果启用“密码认证”，只有输入正确的密码才能连接到服务端并查看对应房间的剪贴板内容。
> 可以将 `server.auth` 字段设为 `true`（随机生成密码）或字符串（自定义全局密码）来启用这个功能。
> 如果设置了 `server.auth`，它始终作为全局入口密码，对所有房间生效。
> `server.roomAuth` 不会让 `server.auth` 失效；它只是给指定房间增加一个额外可用密码。
> `server.roomAuth` 中值为空字符串时，该房间只接受全局 `server.auth`；值为非空字符串时，该房间同时接受全局 `server.auth` 和该房间自己的密码。
> 值也可以是对象 `{ "password": "xx", "fileExpire": N }`：`fileExpire` 为 `0` 表示该房间上传的文件永不过期；大于 `0` 表示覆盖全局 `file.expire`（秒）；不填表示沿用全局。注意 `fileExpire` 只影响修改配置之后上传的文件；历史条数轮转删除不受其影响。
> 未通过认证的用户不会在房间列表里看到受保护房间。


### HTTP API

#### 获取内容

- 方式一: 
```
http://localhost:9501/content/latest  永远返回最新一条内容
http://localhost:9501/content/latest?room=test 永远返回指定房间的最新一条内容
```
- 方式二: 
```
http://localhost:9501/content/1   根据ID访问
http://localhost:9501/content/1?room=test   指定房间
```

#### 发送文本

```console
$ curl -H "Content-Type: text/plain" --data-binary "foobar" http://localhost:9501/text
{"id":"1","type":"text","url":"http://localhost:9501/content/1"}

$ curl http://localhost:9501/content/1
123

$ curl http://localhost:9501/content/1?json=true
{"content":"123","id":"1","timestamp":1748143093,"type":"text"}
```

注意：请求头中不能缺少 `Content-Type: text/plain`

#### 发送文件

```console
$ curl -F file=@image.png http://localhost:9501/upload
{"id":"2","type":"image","url":"http://localhost:9501/content/2"}

$ curl http://localhost:9501/content/2
<a href="http://localhost:9501/file/530a16de-07cb-4835-ba26-64f5e8e1f300/image.png">Found</a>.

$ curl http://localhost:9501/content/2?json=true
{"id":"2","name":"image.png","size":11361,"timestamp":1748175032,"type":"image","url":"http://localhost:9501/file/530a16de-07cb-4835-ba26-64f5e8e1f300","uuid":"530a16de-07cb-4835-ba26-64f5e8e1f300"}


$ curl -L http://localhost:9501/content/2
Warning: Binary output can mess up your terminal. Use "--output -" to tell curl to output it to your terminal anyway,
Warning: or consider "--output <FILE>" to save to a file.
```

#### 在设定房间的情况下发送文本或文件

```console
$ curl -H "Content-Type: text/plain" --data-binary @package.json http://localhost:9501/text?room=reisen-8fce
{"id":"3","type":"text","url":"http://localhost:9501/content/46?room=reisen-8fce"}

$ curl http://localhost:9501/content/3
Not Found

$ curl http://localhost:9501/content/3?room=suika-51ba
Not Found

$ curl http://localhost:9501/content/3?room=reisen-8fce
{
  "name": "cloud-clipboard-server-node",
  ...
}
```

#### 密码认证

```console
$ curl -H "Content-Type: text/plain" --data-binary "foobar" http://localhost:9501/text
{"error":"Unauthorized","message":"需要认证令牌"}

$ curl -H "Authorization: Bearer xxxx" -H "Content-Type: text/plain" --data-binary "foobar" http://localhost:9501/text
{"id":"7","type":"text","url":"http://localhost:9501/content/7"}

$ curl http://localhost:9501/content/1
{"error":"Unauthorized","message":"需要认证令牌"}

$ curl -H "Authorization: Bearer xxxx" http://localhost:9501/content/1
foobar

$ curl  http://localhost:9501/content/1?auth=xxx
foobar
```

> 推荐 API / 脚本使用 `Authorization: Bearer`。`?auth=` 仅为兼容保留，不建议把房间密码写进可分享 URL。

#### 房间会话令牌

Web 前端会用密码换取短期会话令牌，只缓存令牌而非密码，到期前自动续签。

```console
# 用房间密码换取会话令牌（有效期 1 小时）
$ curl -H "Content-Type: application/json" \
  -d '{"password":"room-pass"}' \
  "http://localhost:9501/auth/token?room=default"
{"token":"...","expiresAt":1710003600,"scope":"global"}

# 用仍有效的令牌续签（无需密码），浏览器在到期前 60 秒自动调用
$ curl -H "Authorization: Bearer <token>" \
  -X POST "http://localhost:9501/auth/token/refresh?room=default"
{"token":"...","expiresAt":1710007200,"scope":"global"}
```

说明：
- `POST /auth/token` 仅接受 JSON `{"password":"..."}`，密码正确返回 `token` 与 `expiresAt`
- `scope` 字段表示令牌作用域：用**全局密码**（`server.auth`）登录返回 `"global"`，该令牌对所有房间有效，进入任意受保护房间无需再次输入密码；用**房间专属密码**（`roomAuth`）登录返回 `""`（房间专属）
- 令牌续签时保留原作用域，全局令牌不会在刷新时降级为房间专属
- `POST /auth/token/refresh` 通过 `Authorization: Bearer` 携带当前令牌，有效则签发新令牌；无效或缺失返回 401
- 会话令牌等价于对应房间密码，仅用于浏览器内部，勿放入可分享 URL
- WebSocket 握手（`/push`）优先使用 `Sec-WebSocket-Protocol` 子协议传递令牌（避免凭据进入 URL/访问日志）；`?auth=` 与 `Authorization` 仍作为兼容兜底

#### 短期分享链接

用于浏览器复制链接 / 二维码 / `<a href>` 下载，**不暴露房间密码**。  
现有 API 下载方式不变：继续可用房间密码（Header 或 `?auth=`）。

```console
# 需要已拥有房间访问权限（Authorization）
$ curl -H "Authorization: Bearer xxxx" \
  -H "Content-Type: application/json" \
  -d '{"type":"content","id":"7"}' \
  http://localhost:9501/share
{"type":"content","id":"7","room":"default","ttl":900,"expiresAt":1710000000,"token":"...","url":"http://localhost:9501/content/7?t=..."}

$ curl -H "Authorization: Bearer xxxx" \
  -H "Content-Type: application/json" \
  -d '{"type":"file","uuid":"530a16de-07cb-4835-ba26-64f5e8e1f300","ttl":600}' \
  http://localhost:9501/share
{"type":"file","uuid":"530a16de-...","room":"default","ttl":600,"expiresAt":1710000000,"token":"...","url":"http://localhost:9501/file/530a16de-.../image.png?t=..."}

# 使用短期 token 访问（无需房间密码）
$ curl "http://localhost:9501/content/7?t=..."
$ curl -L "http://localhost:9501/file/530a16de-.../image.png?t=..." -o image.png

# 旧 API 仍然有效
$ curl -H "Authorization: Bearer xxxx" \
  "http://localhost:9501/file/530a16de-.../image.png" -o image.png
```

说明：
- `ttl` 可选，默认 900 秒（15 分钟），范围 60～86400
- `maxUses` 可选，默认 `0`（不限次数）；正整数表示最多完整访问次数，范围 1～1000
  - 一次完整 GET（无 Range，或 `bytes=0-...`）计 1 次
  - 视频/文件的 Range 续传（`bytes>0`）与 HEAD 不计入次数
  - 次数在服务端按 token 的 `jti` 计数（Go 进程内存；Cloudflare 优先 D1）
- 未启用房间密码时，返回的 `url` 不含 `t`
- 短期 token 仅授予对应 content/file 的读取权限，不能用于上传/删除

```console
# 15 分钟、最多打开 3 次
$ curl -H "Authorization: Bearer xxxx" -H "Content-Type: application/json" \
  -d '{"type":"content","id":"7","ttl":900,"maxUses":3}' \
  http://localhost:9501/share
```
