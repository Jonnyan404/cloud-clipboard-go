

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
            "public": {"open": true}, // 显式声明该房间**开放**：不要密码，即使全局 auth 设了也一样
            "keep": {"password": "kp", "fileExpire": 0}, // 对象形式：password 为房间密码；fileExpire: 0=该房间文件永不过期，>0=覆盖 file.expire 秒数，不填=使用全局 file.expire
            "archive": {"fileExpire": 604800}, // 也可以只配置 fileExpire（无独立密码）
            "home": {"password": "hp", "automation": "room"}, // automation: 该房间的定时任务策略，见下方说明
            "lobby": {"open": true, "automation": "single"}, // 开放房间 + 只允许一条定时任务（需要创建者持有 task token）
            "ops": {"password": "op", "automation": "none"} // 配置层关闭：连全局密码也开不了，只能改这个文件
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
    },
    "automation": {
        "enabled": true, // 定时自动化总开关。默认 true —— 但「可用」不等于「放开」，能不能给某个房间装任务由 roomAuth 的 automation 决定
        "tickSeconds": 30, // 扫描到期任务的间隔（秒）。触发精度是分钟，间隔只影响最多晚多久发出去
        "graceSeconds": 600, // 错过触发窗口多久之内还补发。超时只记 skipped，不补发（服务重启 10 分钟内会补，宕机一夜不会）
        "defaultTZ": "Asia/Shanghai" // 任务没写时区时的默认值。默认上海而不是「服务器本地」—— 容器里那通常是 UTC，一个「每天 09:30」会变成 17:30 发出去，而且只有真到了那天才看得出来
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
> 想让某个房间在**全局加密**的前提下保持开放，用 `{ "open": true }`：该房间不要密码，且**不**回落全局密码。这样两种混合都能表达 —— 全局加密 + 个别房间开放，或全局开放 + 个别房间加密。
> `open` 是单独一个字段，不是「把值留空」：空字符串在这份配置里**已经有含义**（只接受全局密码），改掉它会静默改变现有配置 —— 某个房间会悄悄敞开。而且 `open` 还能和 `fileExpire` 一起用（`{"open": true, "fileExpire": 0}` = 开放且文件永不过期），空字符串表达不了这个。
> 同时写了 `open` 和非空 `password` 属于配置写错：**按需要密码处理**（宁可多要一次密码，也不能因为多打了一个字段把房间敞开）。
> 值也可以是对象 `{ "password": "xx", "fileExpire": N }`：`fileExpire` 为 `0` 表示该房间上传的文件永不过期；大于 `0` 表示覆盖全局 `file.expire`（秒）；不填表示沿用全局。注意 `fileExpire` 只影响修改配置之后上传的文件；历史条数轮转删除不受其影响。
> 未通过认证的用户不会在房间列表里看到受保护房间。
>
> **定时自动化（`automation` / `roomAuth.*.automation`）的说明：**
>
> 定时任务能「以服务端的身份、在没人看着的时候」往房间投递内容，所以它是**写权限的代理**——谁能建任务，谁就获得了「无人值守地在这个房间说话」的能力。因此策略是按**房间**分档的，写在 `roomAuth` 里：
>
> | `automation` 取值 | 客户端凭据 | 面板 | 条数 | 可操作范围 |
> |---|---|---|---|---|
> | 不写 | 跟随房间鉴权档位：有房间密码 → `room`；公开房间 → `none` | — | — | — |
> | `none` | 任意（含全局密码） | 隐藏 | 0 | 只能改这个配置文件 |
> | `single` | 无（另有 task token） | 显示 | 1 | 只能改删自己那条 |
> | `room` | 房间密码 / 房间令牌 | 显示 | 不限（默认上限 20） | 仅本房间 |
>
> 几个关键约定：
> - **持有全局密码 = 管理员**，可以给任意房间建任务（改 `?room=` 即可）。这不是代码决定的，是部署约定：如果 `server.auth` 是全家共用的一把钥匙，那在这个模型里「所有人都是管理员」。需要更严的话，用 `automation: "none"` 把敏感房间摘出去 —— 这条配置**连全局密码也改不了**，只能改配置文件，也就是「配置层 > 运行时最高权限」。
> - **公开房间默认关闭**，但**可以显式打开**（`{"open": true, "automation": "single"}`）。没有硬编码成「公开房间一律禁止」：家里没设密码的房间恰恰最需要每日提醒。
> - **`single` 档必须配 task token**：公开房间的客户端没有任何凭据可证明身份（`canAccessRoom` 恒为真），没有钥匙的话那条任务谁都能改、谁都能删。开启后，创建任务时响应里会返回 `taskToken`，明文只出现这一次。
> - 房间**不能**由请求体声明：任务的作用房间来自鉴权上下文，请求体里带 `room` 会被忽略（否则一条已授权的任务就能把消息发到别的房间）。
> - 定时消息默认 **不占房间历史额度**（只广播、不落历史）。原因见 `history` 一项：房间历史是按房间计数的，一个每天发一次的任务十几天就能把房间里的历史全顶掉。任务里把 `keepHistory` 设为 `true` 可以改成占额度。
> - 任务的**排期**有四种写法：`daily` / `weekly` / `once` / `cron`。前三种是结构化的（`time` + `byWeekday`），覆盖绝大多数用法；`cron` 收 5 字段表达式（分 时 日 月 周），用来表达前三种说不清的排期 —— 比如 `*/30 9-18 * * 1-5`（工作日每半小时）、`0 0 1 */3 *`（每季度第一天）。cron 里同时限制「日」和「星期」时按标准 cron 的 **OR** 语义（任一匹配即触发）。
> - 任务**没写时区**时会自动填上 `automation.defaultTZ`（默认 `Asia/Shanghai`），并且**写进任务本身**而不是运行时兜底 —— 任务定义要是自洽的，换一台服务器不该换个时刻触发。
> - ⚠️ **不要用多个副本共享同一个数据目录**。调度器是**进程内**的（每个实例各扫各的），判重用的幂等键（`lastRunKey`）落在 `tasks.json`、读进各自的内存 —— A 写盘之后 B 内存里还是旧值，于是同一分钟两边都会认为「该发了」，用户收到两条。自托管单实例不受影响；要横向扩就得先给调度加一把跨进程的锁，那是另一件事。
> - ⚠️ **仅 Go 端**。Cloudflare Worker 部署没有这一族接口（没有 `/tasks`、没有 `/automation`、也没有调度器），它的 `/server` 会明确下发 `automation.enabled = false`，SPA 工具栏上的入口据此不显示。


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

#### 声明设备名称

发送文本、文件或连接 WebSocket（`/push`）时，可以用 `name` 参数声明发送端的显示名；不传则按 `User-Agent` 推断。

```console
$ curl -H "Content-Type: text/plain" --data-binary "来自树莓派" "http://localhost:9501/text?name=RaspberryPi"
$ curl -F file=@image.png "http://localhost:9501/upload?name=RaspberryPi"
```

名字会写进消息的 `senderDevice.name`，Web 端优先显示它，没有才回落 `os` / `type`。主要给快捷指令、脚本这类 `User-Agent` 认不出来的来源用。

- 最多 32 个字符，超出按字符截断（不会截坏多字节字符）
- 控制字符会被剔除，首尾空白会被裁掉
- 名字由客户端随每次请求带着走，服务端不存储、不记忆

#### 密码认证

```console
$ curl -H "Content-Type: text/plain" --data-binary "foobar" http://localhost:9501/text
{"code":"unauthorized","error":"Authentication required","message":"需要认证令牌"}

$ curl -H "Authorization: Bearer xxxx" -H "Content-Type: text/plain" --data-binary "foobar" http://localhost:9501/text
{"id":"7","type":"text","url":"http://localhost:9501/content/7"}

$ curl http://localhost:9501/content/1
{"code":"unauthorized","error":"Authentication required","message":"需要认证令牌"}

$ curl -H "Authorization: Bearer xxxx" http://localhost:9501/content/1
foobar

$ curl  http://localhost:9501/content/1?auth=xxx
foobar
```

> 推荐 API / 脚本使用 `Authorization: Bearer`。`?auth=` 仅为兼容保留，不建议把房间密码写进可分享 URL。

#### 内容格式（`/content/*`）

`/content/<id>` 和 `/content/latest` 用 `?format=raw|json` 决定返回什么：

```console
$ curl "http://localhost:9501/content/7?format=json"
{"id":"7","type":"text","content":"foobar","timestamp":1758000000}

$ curl "http://localhost:9501/content/7?format=raw"
foobar
```

判定优先级（高到低）：

1. **`?format=raw|json`** —— 显式指定，压过其他一切信号
2. `.json` 路径后缀 —— `/content/latest.json`（Android 捷径在用，**不能动**）
3. `?json=1` / `?json=true` —— 旧信号，保留兼容
4. `Accept` 头含 `application/json` —— **只对文本生效**
5. 默认 —— `raw`（纯文本或文件字节）

两点要注意：

- **`Accept` 头对文件不生效**。下载链路上的 Accept 太不可靠（浏览器、下载器、脚本五花八门），
  所以文件分支只认显式信号。否则「浏览器直接点开文件链接」会突然收到一坨 JSON。
- **不认识的 `format` 值返回 400**（`code: unsupported_format`），不会静默回落成 raw ——
  客户端以为拿到 HTML、实际拿到原文，是要出事的。

#### 错误响应

**所有**错误路径都返回同一种形状，`Content-Type: application/json; charset=utf-8`，状态码保持常规语义：

```json
{"code": "text_too_long", "error": "Text too long", "message": "文本内容超出限制 (最大 4096 字符)"}
```

| 字段 | 用途 |
|---|---|
| `code` | 机器码（snake_case），给程序判断。**发布后不要改** |
| `error` | 英文人话，给日志和英文用户看 |
| `message` | 中文人话，给人看 |

Go 与 Cloudflare Worker 两个实现共用这一份契约。**不要按 `Accept` 头分叉成两种响应体**：Apple 快捷指令的「获取URL内容」既不发 `Accept`、也不把 HTTP 状态码暴露给捷径，客户端只能读响应体 —— 同一状态码两种形状等于要求每个客户端各写两套解析逻辑。

常见 `code`：`text_too_long`、`file_too_large`、`content_not_found`、`no_content`、`file_expired`、`unauthorized`、`unauthorized_invalid_token`、`room_forbidden`、`method_not_allowed`、`invalid_request_body`。

**文件/图片是两次请求，两次都要带凭据。** `/content/*` 返回的 `url` 只是一个地址，不含凭据；
客户端必须自己把凭据加在这一次请求上。文本没有这一步（内容内联在 JSON 里），
所以漏带凭据的表现非常像「文本正常、文件/图片 401」。

```console
# 第一次：拿元数据（带凭据）
$ curl -H "Authorization: Bearer xxxx" "http://localhost:9501/content/latest?room=default&json=1"
{"type":"image","name":"m.png","uuid":"<uuid>","url":"http://localhost:9501/file/<uuid>/m.png",...}

# 第二次：按 url 取字节（同样要带凭据，否则 401）
$ curl -H "Authorization: Bearer xxxx" "http://localhost:9501/file/<uuid>/m.png"
```

`/file/<uuid>/<name>` 按**文件自己记录的房间**鉴权，不看你传的 `?room=` —— 传了也不作数。

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
{"type":"content","id":"7","room":"default","ttl":900,"expiresAt":1710000000,"token":"...","url":"http://localhost:9501/s/...","pageUrl":"http://localhost:9501/s/...","rawUrl":"http://localhost:9501/content/7?t=..."}

$ curl -H "Authorization: Bearer xxxx" \
  -H "Content-Type: application/json" \
  -d '{"type":"file","uuid":"530a16de-07cb-4835-ba26-64f5e8e1f300","ttl":600}' \
  http://localhost:9501/share
{"type":"file","uuid":"530a16de-...","room":"default","ttl":600,"expiresAt":1710000000,"token":"...","url":"http://localhost:9501/s/...","pageUrl":"http://localhost:9501/s/...","rawUrl":"http://localhost:9501/file/530a16de-.../image.png?t=..."}

# 用短期 token 直连（无需房间密码）
$ curl "http://localhost:9501/content/7?t=..."
$ curl -L "http://localhost:9501/file/530a16de-.../image.png?t=..." -o image.png

# 旧 API 仍然有效
$ curl -H "Authorization: Bearer xxxx" \
  "http://localhost:9501/file/530a16de-.../image.png" -o image.png
```

说明：
- **`url` 就是分享页地址**（`<服务地址><prefix>/s/<token>`），交给收件人的就是它。
  服务端对这条路径返回 SPA 外壳、并把 Open Graph 标签直接注入它的 `<head>` ——
  聊天软件拿这个地址展开预览能拿到真实卡片，真人打开**同一个**地址直接进分享页，
  没有第二跳、也没有第二个地址。
- `pageUrl` 为兼容保留，目前与 `url` **同值**（只认 `pageUrl` 的老客户端照常工作）；
  `rawUrl` 才是带同一个 token 的直连接口地址，下载链路用。
- **一律签发 token**，房间没开密码也发 —— 有效期、次数限制、密码都装在 token 里。
  以前开放房间返回的是裸 `/content/<id>`，`ttl` / `maxUses` 会被静默丢弃。
- `ttl` 可选，默认 900 秒（15 分钟），范围 60～86400
- `maxUses` 可选，默认 `0`（不限次数）；正整数表示最多完整访问次数，范围 1～1000
  - 一次完整 GET（无 Range，或 `bytes=0-...`）计 1 次
  - 视频/文件的 Range 续传（`bytes>0`）与 HEAD 不计入次数
  - 次数在服务端按 token 的 `jti` 计数（Go 进程内存；Cloudflare 优先 D1）
- `password` 可选：设了之后收件人必须提供。**走 `X-Share-Password` 请求头，不进 URL**
  （query 会进浏览器历史和访问日志）；token 里只存 `HMAC(服务端密钥, 密码)` 的前 16 位
- 短期 token 仅授予对应 content/file 的读取权限，不能用于上传/删除

分享页在取正文之前会先问一次 `GET /share?t=<token>`，拿类型 / 文件名 / 大小 / 剩余有效期 /
是否需要密码。**这一步不消耗次数** —— 打开页面本身不该烧掉一次。失败原因可区分：
`share_token_invalid`（无效或过期）、`share_password_required`（没带或带错密码）、
`content_not_found` / `file_not_found`、`file_expired`。

```console
# 15 分钟、最多打开 3 次
$ curl -H "Authorization: Bearer xxxx" -H "Content-Type: application/json" \
  -d '{"type":"content","id":"7","ttl":900,"maxUses":3}' \
  http://localhost:9501/share

# 15 分钟、最多 3 次、且需要密码 hunter2
$ curl -H "Authorization: Bearer xxxx" -H "Content-Type: application/json" \
  -d '{"type":"content","id":"7","ttl":900,"maxUses":3,"password":"hunter2"}' \
  http://localhost:9501/share
```
