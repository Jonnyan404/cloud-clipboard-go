# Cloud Clipboard REST API

给第三方客户端（网页、App、脚本、嵌入式设备）用的接口说明。

服务端有两个实现，**共用这一份契约**：

| 实现 | 位置 | 说明 |
|---|---|---|
| Go | `cloud-clip/` | 自托管首选，单二进制，内嵌静态资源 |
| Cloudflare Worker | `cloudflare/workers/` | 无服务器部署，用 D1 + R2 |

下面凡有差异的地方都会单独标出；没标的表示两边行为一致。

> 契约有测试守着，改接口时会红：
> Go 侧 `lib/shortcut_contract_test.go`，Worker 侧 `test/shortcut-contract.test.mjs`。

---

## 1. 通用约定

### 1.1 Base URL 与子路径

服务默认监听 `9501`。若部署在子路径下（配置项 `server.prefix`，环境变量 `PREFIX`），
所有接口都要带上该前缀：

```
http://host:9501/text                    # 无前缀
https://host/cloud-clipboard/text        # PREFIX=/cloud-clipboard
```

### 1.2 鉴权

三种方式，任选其一：

| 方式 | 写法 | 说明 |
|---|---|---|
| Bearer 头 | `Authorization: Bearer <凭据>` | **推荐**。凭据可以是全局密码、房间密码，或 `/auth/token` 签发的会话令牌 |
| 查询串 | `?auth=<凭据>` | 仅为兼容保留（快捷指令在用）。**别把密码写进可分享的 URL** |
| 会话令牌 | 同上两种写法皆可 | 由 `/auth/token` 签发，默认 1 小时有效 |

**房间与凭据的对应关系**：

- `?room=` 留空 = `default` 房间；**完全不传** `room` 才是「不限房间」
- 房间要不要密码 = 全局 `auth` 与 `roomAuth` 里那一项共同决定：没配过 → 跟随全局；
  空串 → 也跟随全局（**不是**「开放」）；`{"open": true}` → **开放**，全局设了也不拦；
  非空密码 → 要密码，且全局密码**仍然有效**（多给一把钥匙，不是换锁）
- 服务端**按内容自己记录的房间**鉴权，不信客户端声明的 `?room=`
  （`/file/:uuid/:name` 尤其是这样，防伪造）

**症状对照**（排查时很有用）：

| 现象 | 含义 |
|---|---|
| `404 content_not_found` | 房间没对上（内容不在这个房间） |
| `401 unauthorized_invalid_token` | 房间对上了，但凭据是空或错 |
| `404 file_expired` | 内容存在但已过期 |

### 1.3 内容格式（`/content/*` 专用）

`/content/latest` 与 `/content/:id` 用 `?format=` 决定返回什么：

| 优先级 | 信号 | 效果 |
|---|---|---|
| 1 | `?format=raw\|json` | 显式指定，压过其他一切。**新代码一律用这个** |
| 2 | `.json` 路径后缀 | ⚠️ **兼容信号，即将下线**：已发布的捷径还在用，暂时保留 |
| 3 | `?json=1` / `?json=true` | ⚠️ **兼容信号，即将下线**，同上 |
| 4 | `Accept: application/json` | **只对文本生效** |
| 5 | 默认 | `raw` |

```bash
curl "http://localhost:9501/content/7?format=json"
# {"id":"7","type":"text","content":"foobar","timestamp":1758000000}

curl "http://localhost:9501/content/7?format=raw"
# foobar
```

两点要注意：

- **`Accept` 头对文件不生效**。下载链路上的 Accept 五花八门（浏览器、下载器、脚本各不同），
  所以文件分支只认显式信号 —— 否则「浏览器直接点开文件链接」会收到一坨 JSON。
- **不认识的 `format` 值返回 400**（`code: unsupported_format`），不会静默回落。

### 1.4 错误响应

**所有**错误路径返回同一种形状，`Content-Type: application/json; charset=utf-8`：

```json
{"code": "text_too_long", "error": "Text too long", "message": "文本内容超出限制 (最大 4096 字符)"}
```

| 字段 | 用途 |
|---|---|
| `code` | 机器码（snake_case），给程序判断。**发布后不要改** |
| `error` | 英文人话，给日志和英文用户 |
| `message` | 中文人话，给人看 |

常见 `code` 见文末[错误码表](#8-错误码表)。

> **不要按 `Accept` 头分叉成两种响应体**：Apple 快捷指令的「获取URL内容」既不发 `Accept`、
> 也不把 HTTP 状态码暴露给捷径，客户端只能读响应体。同一状态码两种形状 = 每个客户端写两套解析。

### 1.5 请求体类型

- 文本类接口收**纯文本**（`Content-Type: text/plain`），**不是 JSON**
- 文件类接口收 `multipart/form-data`
- 鉴权类接口（`/auth/token`、`/share`）收 JSON

---

## 2. 端点总览

| 方法 | 路径 | 用途 | 鉴权 |
|---|---|---|---|
| GET | `/server` | 服务信息与限制 | 否 |
| GET | `/myip` | 客户端出口 IP | 否 |
| GET | `/health` | 健康检查（**仅 Worker**） | 否 |
| POST | `/auth/token` | 用密码换会话令牌 | 密码 |
| POST | `/auth/token/refresh` | 续签会话令牌 | 令牌 |
| POST | `/text` | 发送文本 | 是 |
| POST | `/upload` | 上传文件 | 是 |
| POST | `/upload/chunk/:uuid` | 分块上传（**仅 Go**） | 是 |
| POST | `/upload/finish/:uuid` | 分块完成（**仅 Go**） | 是 |
| POST | `/upload/multipart/*` | R2 分片上传（**仅 Worker**） | 是 |
| GET | `/content/latest` | 取最新一条 | 是 |
| GET | `/content/:id` | 按 ID 取一条 | 是 |
| GET | `/file/:uuid/:name` | 下载文件 | 是 |
| GET | `/rooms` | 房间列表 | 是 |
| POST | `/share` | 创建分享令牌 | 是 |
| GET | `/share?t=` | 分享页元信息（不消耗次数） | 否 |
| DELETE | `/revoke/:id` | 删除一条 | 是 |
| DELETE | `/revoke/all` | 清空房间 | 是 |
| WS | `/push` | 实时推送 | 是 |

---

## 3. 服务信息

### GET /server

无需鉴权。客户端启动时先调它拿限制值，别把限制写死在客户端。

```json
{
  "version": "5.0.8",
  "server": { "prefix": "", "history": 100, "roomList": false },
  "text": { "limit": 4096 },
  "file": { "limit": 268435456, "expire": 3600, "chunk": 1048576 }
}
```

`authNeeded` / `authorized` 等字段会反映当前鉴权状态，前端据此决定是否弹密码框。

### GET /myip

```json
{ "ip": "203.0.113.7" }
```

### GET /health（仅 Worker）

返回纯文本 `OK`。Go 版没有这个端点 —— 它用 `/server` 探活。

---

## 4. 鉴权

### POST /auth/token

```http
POST /auth/token?room=default
Content-Type: application/json

{"password": "your-password"}
```

响应：

```json
{"token": "eyJ...", "expiresAt": 1758003600, "scope": "global"}
```

- `scope` 为 `global` 表示用全局密码登录，令牌对所有房间有效；房间密码登录则为 `""`
- 令牌默认 1 小时有效

### POST /auth/token/refresh

带旧令牌即可静默续期，无需再输密码：

```http
POST /auth/token/refresh?room=default
Authorization: Bearer <旧令牌>
```

响应同 `/auth/token`。续签会保留原令牌的 `scope`，不会把全局会话降级成房间专属。

---

## 5. 发送

### POST /text

```http
POST /text?room=default&name=iPhone&client=<client-id>
Content-Type: text/plain
Authorization: Bearer <凭据>

要发送的文本内容
```

| 参数 | 位置 | 说明 |
|---|---|---|
| `room` | query | 房间名，留空 = `default` |
| `name` | query | **设备显示名**，写进消息的 `senderDevice.name`。最多 32 字符，控制字符会被剔除；留空则由服务端按 User-Agent 推断 |
| `client` | query | **客户端唯一 ID**，用于「这条是不是我发的」判断（聊天气泡归属）。与 `name` 是两件事，不能互相替代 |
| `id` | query | 传了就**覆盖**这条已有消息，而不是新建 |

响应：

```json
{"id": "7", "type": "text", "url": "http://localhost:9501/content/7"}
```

超限时返回 `413` + `code: text_too_long`（上限见 `/server` 的 `text.limit`）。

**正文有三种形态**，按 `Content-Type` 分：

| `Content-Type` | 正文 |
|---|---|
| `text/plain`、不声明、或其它 | **整个请求体就是正文** |
| `application/json` | `{"content": "要发送的文本"}` |
| `multipart/form-data` | 表单字段 `content` |

后两种是给**快捷指令**用的：它把字符串变量当请求体发出去时字节会变成 UTF-16，
而结构化请求体是按 UTF-8 序列化的。

⚠️ `application/x-www-form-urlencoded` **刻意不认**，继续走「整个请求体是正文」那一条 ——
它是 `curl --data-binary` 之类不带 `-H` 时的默认类型，把它当表单解析会让这类请求**静默存成空串**。

声明了 `application/json` 但正文不是合法 JSON → `400` + `code: invalid_body`。

**纯文本那一条还会认 UTF-16**（带 BOM，或字节形态能看出是 UTF-16）并解码 —— 快捷指令发的就是它；
认不出就按 UTF-8 存原文，**不做任何转义**。

### POST /upload

```http
POST /upload?room=default&name=iPhone
Authorization: Bearer <凭据>
Content-Type: multipart/form-data

file=@photo.png
```

表单字段名固定为 **`file`**。响应含 `uuid` 与 `url`：

```json
{
  "id": "8", "type": "file", "name": "photo.png", "size": 20480,
  "uuid": "11111111-2222-3333-4444-555555555555",
  "url": "http://localhost:9501/file/11111111-.../photo.png",
  "expire": 1758003600
}
```

> **文件是两次请求，两次都要带凭据。** `/content/*` 返回的 `url` 只是一个地址，不含凭据；
> 客户端必须自己把凭据加在下载那一次请求上。文本没有这一步（内容内联在 JSON 里），
> 所以漏带凭据的表现很像「文本正常、文件 401」。

**大文件**：

- **Go**：`POST /upload/chunk/:uuid` 逐块追加 → `POST /upload/finish/:uuid` 收尾
- **Worker**：R2 multipart —— `create` → `PUT /upload/multipart/:partNumber` → `complete`
  （`DELETE /upload/multipart` 可中止）

---

## 6. 接收

### GET /content/latest

取该房间**最新一条**（可能是文本也可能是文件记录）。

```bash
curl "http://localhost:9501/content/latest?room=default&format=json" -H "Authorization: Bearer xxx"
```

```json
{
  "id": "7", "type": "text", "content": "foobar",
  "timestamp": 1758000000,
  "senderDevice": { "name": "iPhone", "type": "mobile", "os": "iOS 18" },
  "senderIP": "203.0.113.7"
}
```

文件类型时返回 `uuid` / `name` / `size` / `url` / `expire`，不含内容字节。

> **「最新」的定义**：`timestamp` 是**秒级**，同一秒里有多条时取**后插入**的那条。
> Worker 侧对应 `ORDER BY timestamp DESC, id DESC`。

### GET /content/:id

同上，按 ID 精确取。⚠️ `.json` 路径后缀是**即将下线的兼容信号**，新代码请用 `?format=json`。

### GET /file/:uuid/:name

下载文件字节。

| 参数 | 说明 |
|---|---|
| `?auth=` | 凭据（**房间以文件自己记录的为准**，客户端传的 `room` 不作数） |
| `?download=true` | 加 `Content-Disposition: attachment`，浏览器直接下载而不是内联显示 |

---

## 7. 房间与管理

### GET /rooms

返回房间列表（需服务端开启 `roomList`）：

```json
{
  "rooms": [
    { "name": "default", "messageCount": 12, "isProtected": false, "isActive": true }
  ]
}
```

### POST /content/:id/column

把一条内容挪到看板的某一列。看板是**同一批条目的一个视图**，不是第二份数据 ——
这里只是在条目上改一个字段，别的什么都不动：

```http
POST /content/7/column?room=default
Content-Type: application/json
Authorization: Bearer <凭据>

{"column": "doing"}
```

| 取值 | 含义 |
|---|---|
| `todo` | 待办 —— 也是默认值：`column` 缺失或空串都会归一成它 |
| `doing` | 进行中 |
| `done` | 已完成 |

响应：

```json
{"id": "7", "type": "text", "column": "doing"}
```

- 三列是**固定的** —— 没有按房间配置列，也没有列内顺序。挪动只改「在哪一列」。
- ⚠️ **不动 `timestamp`。** `POST /text?id=` 改正文时会刷新时间戳，但挪卡片**不能**让它在时间流里
  跳到最前面 —— 否则拖一张卡就把整个列表重排了。
- 文本条目和文件条目都能上板。
- 鉴权用**房间密码**。分享 token **不行**：那是只读凭据。
- 会在房间的 WebSocket 上广播 `update` 事件，其他客户端也会跟着挪。
- 错误码：`invalid_column`（400）、`invalid_body`（400）、`invalid_content_id`（400）、
  `content_not_found`（404）、`method_not_allowed`（405）。

### POST /share

为单条内容创建**短期分享令牌**，让拿到链接的人可以访问：

```http
POST /share
Content-Type: application/json
Authorization: Bearer <凭据>

{"type": "content", "id": "7", "ttl": 900, "maxUses": 0, "password": ""}
```

- `type`：`content` 或 `file`
- `file` 类型用 `uuid` 而不是 `id`
- `ttl` 秒，默认 900（15 分钟），范围 60 ~ 86400
- `maxUses` 为 `0` 表示不限次数
- `password` 可选；一旦设置，收件人必须提供（见下）

令牌**一律签发** —— 开放房间也会拿到一个，因为有效期、次数限制和密码全都装在它里面。
响应给出两个地址：

```json
{
  "url": "https://host/#/s?t=<token>",
  "rawUrl": "https://host/content/7?t=<token>",
  "token": "<token>",
  "expiresAt": 1750000000,
  "maxUses": 0
}
```

- `url` 是**分享页** —— 交给收件人的就是它
- `rawUrl` 带同一个令牌直连内容 / 文件接口（下载链路用）

### GET /share?t=&lt;token&gt;

分享页在取正文之前先问一次这里：类型、文件名与大小、剩余有效期、以及是否需要密码。
**不消耗使用次数** —— 打开页面本身不该烧掉一次。

```json
{"type": "content", "kind": "text", "id": "7", "room": "default",
 "expiresAt": 1750000000, "maxUses": 0, "used": 0, "needsPassword": false}
```

失败原因可区分，分享页据此给出对应提示：

| `code` | 状态码 | 含义 |
|---|---|---|
| `share_token_invalid` | 401 | 签名不对或已过期 |
| `share_password_required` | 401 | 没带密码或密码不对 |
| `content_not_found` / `file_not_found` | 404 | 内容已不存在 |
| `file_expired` | 404 | 文件已过期 |

**密码走 `X-Share-Password` 请求头，绝不进 URL** —— query 会进浏览器历史和服务器访问日志。
令牌里只存 `HMAC(服务端密钥, 密码)`。

### DELETE /revoke/:id

删除指定消息。**Worker 侧走 `DELETE`，Go 侧同样支持 `DELETE`。**

### DELETE /revoke/all

清空当前房间的全部消息（会通过 WebSocket 广播 `clearAll`）。

---

## 8. 实时推送

### WS /push

```
ws://localhost:9501/push?room=default&token=<令牌>
```

连接后，该房间的新消息会实时推给所有连接。事件形如：

```json
{"event": "newMessage", "data": { ...与 /content/:id 的 JSON 同构... }}
```

断线重连是客户端自己的责任（网页端会带指数退避重试）。

---

## 9. 错误码表

| `code` | 典型状态码 | 含义 |
|---|---|---|
| `unauthorized` | 401 | 缺凭据 |
| `unauthorized_invalid_token` | 401 | 凭据无效 |
| `room_forbidden` | 401 | 无权访问该房间 |
| `method_not_allowed` | 405 | 方法不对 |
| `invalid_request_body` | 400 | 请求体不是合法 JSON |
| `content_not_found` | 404 | 内容不存在 |
| `no_content` | 404 | 该房间还没有任何内容 |
| `invalid_content_id` | 400 | 内容 ID 不是数字 |
| `file_not_found` | 404 | 文件不存在 |
| `file_expired` | 404 | 文件已过期 |
| `file_too_large` | 413 | 文件超限 |
| `text_too_long` | 413 | 文本超限 |
| `unsupported_format` | 400 | `?format=` 给了不认识的值 |
| `form_parse_failed` | 400 | multipart 解析失败 |
| `invalid_uuid` | 400 | UUID 格式不对 |
| `missing_id` / `missing_type` / `missing_uuid` | 400 | 分享接口缺参数 |
| `unsupported_type` | 400 | 分享类型不支持 |
| `password_required` | 401 | 密码为空 |
| `wrong_password` | 401 | 密码不对 |
| `share_token_invalid` | 401 | 分享令牌无效或已过期 |
| `share_password_required` | 401 | 分享密码没带或不对 |
| `internal_error` | 500 | 服务端内部错误 |

---

## 10. 客户端实现建议

1. **先调 `/server`** 拿限制值，不要写死。
2. **限制是动态的**：超限错误里的数字来自服务端配置（`text.limit` / `file.limit`），
   客户端应原样展示服务端给的 `message`，而不是自己拼一句。
3. **错误只解析一套**：读 `message`（中文）或 `error`（英文），别按 `Accept` 分叉。
4. **文件下载记得带凭据**，且**别信自己的 `room`**。
5. **`name` 与 `client` 别混用**：前者给人看，后者给程序判归属。
6. **别依赖「不传 room = default」**：不传 `room` 在部分端点意味着「不限房间」，
   想指定默认房间就显式写 `?room=default`。
