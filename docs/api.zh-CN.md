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

常见 `code` 见文末[错误码表](#9-错误码表)。

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
| GET/POST | `/tasks` | 定时自动化任务：列表 / 创建或更新（**仅 Go**） | 房间 |
| POST | `/tasks/preview` | 试算一条还没保存的任务（**仅 Go**） | 房间 |
| DELETE | `/tasks/:id` | 删除一条定时任务（**仅 Go**） | 房间 |
| POST | `/tasks/:id/run` | 试跑（默认）或立即发送（`?send=1`）（**仅 Go**） | 房间 |
| GET | `/tasks/cron` | 校验 cron 表达式并给出未来几次触发时刻（**仅 Go**） | 房间 |
| POST | `/tasks/:id/toggle` | 启用 / 停用一条任务（**仅 Go**） | 房间 |
| GET | `/automation` | 自动化管理页（HTML）（**仅 Go**） | 否 |
| POST | `/share` | 创建分享令牌 | 是 |
| GET | `/share?t=` | 分享页元信息（不消耗次数） | 否 |
| GET | `/share/list` | 某房间最近的分享记录（含打开次数） | 房间 |
| POST | `/share/visit` | 上报「有人打开了这条分享」 | 否 |
| GET | `/s/:token` | 分享页：SPA 外壳 + 注入的 Open Graph 标签（HTML） | 否 |
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
响应给出地址：

```json
{
  "url": "https://host/s/<token>",
  "pageUrl": "https://host/s/<token>",
  "rawUrl": "https://host/content/7?t=<token>",
  "token": "<token>",
  "jti": "9f2c…",
  "expiresAt": 1750000000,
  "maxUses": 0,
  "visits": 0,
  "scans": 0
}
```

- `url` **就是**分享页：同一个地址同时服务抓取程序和真人。服务端对 `/s/<token>` 返回 SPA 外壳，
  并把 Open Graph 标签直接注入它的 `<head>` —— 聊天软件拿这个地址展开预览能拿到真实卡片，
  真人打开**同一个**地址直接进分享页，没有第二跳、没有第二个地址
- `pageUrl` 为兼容保留，目前与 `url` **同值**（只认 `pageUrl` 的客户端照常工作）
- `rawUrl` 带同一个令牌直连内容 / 文件接口（下载链路用）
- `jti` 是这条分享在服务端记录里的编号；`visits` / `scans` 初始为 0

### GET /share/list?room=&limit=

这个房间最近的分享，以及每条被打开了多少次。鉴权与「在该房间签发分享」完全一致
（`room` 默认 `default`，`limit` 默认 50、最多 200）。

```json
{
  "room": "default",
  "total": 3,
  "limit": 50,
  "records": [
    {
      "jti": "9f2c…", "type": "content", "kind": "text", "id": "7", "room": "default",
      "name": "正文首行摘要", "size": 0,
      "createdAt": 1749999000, "expiresAt": 1750000000,
      "maxUses": 0, "used": 0, "visits": 2, "scans": 1,
      "password": false, "expired": false
    }
  ]
}
```

> **列表里永远没有 token 本身**。它是 bearer 凭据，把列表做成「能再抄一遍链接」的入口，
> 就等于让任何能读这个房间记录的人取用别人的分享。
>
> **谁能读**：能在该房间签发分享的人。房间没设密码时就是所有能访问服务器的人 ——
> 记录记的是「这个房间分享过什么」，而这个房间的内容本来就已经公开。
> 需要保护这份记录就给房间设密码。

### POST /share/visit

上报「有**真人**打开了分享页」。分享页调一次；服务端自己验 token
（无效或已过期一律 401，且不计数）。

```http
POST /share/visit
Content-Type: application/json

{"token": "<token>", "qr": true}
```

```json
{ "ok": true, "tracked": true, "visits": 3, "scans": 1 }
```

- 同一访客十分钟内重复上报时 `tracked` 为 `false` —— 重复上报不该把数字刷上去
- `qr: true`（或 `?q=1`）在「打开」之外另计一次扫码；二维码那条地址写成 `/s/<token>?q=1`，
  分享页直接从打开时的 query 上读这个标记 —— 抓取程序和真人共用一个地址，不需要谁再转手透传
- **不需要鉴权**：拿着链接就是上报的凭据，而且响应只描述这一条分享
- 计数**只走这一个接口**（分享页挂载时调一次）。响应 `/s/<token>` 本身永不计数 ——
  聊天软件的抓取程序反复访问它也刷不出数字：抓取程序不执行页面，也就不会上报

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

## 8. 定时自动化（`/tasks`）

> **仅 Go 实现。** Worker 侧尚未实现这一族接口。

定时任务让服务端在**没人看着的时候**按时刻把一段文本投进房间。它是**写权限的代理** ——
能建任务的人就获得了「无人值守地在这个房间说话」的能力，所以策略按**房间**分档，
写在 `roomAuth` 里（完整说明见 [配置说明](../cloud-clip/config.md)）。

### 8.1 名额分档

| 房间策略 `roomAuth[x].automation` | 客户端凭据 | 可建条数 | 可操作范围 |
|---|---|---|---|
| 不写（默认） | 跟随房间鉴权：有房间密码 → `room`；公开房间 → `none` | — | — |
| `none` | 任意（含全局密码） | 0 | 只能改配置文件 |
| `single` | 无（另有 task token） | 1 | 只能改删自己那条 |
| `room` | 房间密码 / 房间令牌 | 20 | 仅本房间 |
| 任意策略 **+ 全局密码** | 全局密码 | 不限 | 任意房间（用 `?room=` 指定） |

`GET /server?room=R` 会把当前凭据的能力一并下发，客户端据此决定是否渲染自动化入口：

```json
{
  "automation": {
    "enabled": true,
    "room": "home",
    "tier": "room",
    "allowed": true,
    "admin": false,
    "max": 20,
    "defaultTZ": "Asia/Shanghai",
    "vars": ["date", "weekday", "time", "datetime", "timestamp", "uuid", "task", "room"],
    "actions": [
      {"id": "text.trimLines", "group": "text", "groupKey": "actionGroupText", "key": "actionTrimLines"}
    ]
  }
}
```

| 字段 | 说明 |
|---|---|
| `tier` | `admin` / `room` / `single` / `none` |
| `allowed` | 当前凭据能不能用这个房间的自动化 |
| `max` | 该房间可建条数上限，`0` = 不限 |
| `defaultTZ` | 任务不写时区时会被填入的默认值 |
| `vars` | 可用变量名（**不含** `{{ }}`，客户端自己拼） |
| `actions` | 服务端可执行的动作。`key` / `groupKey` 是**前端 locale 文件里的 i18n key**（如 `actionTrimLines` / `actionGroupText`）—— 客户端拿它去自己的文案表取人话名字。直接把 `text.trimLines` 这种 id 显示给用户是没有意义的 |

> ⚠️ 服务端**只下发 i18n key，不下发译文**。译文的唯一来源是前端的 `locales/*.json`；
> 在服务端再放一份，迟早会漂成「同一个动作在两个界面里叫不同的名字」。

### 8.1.1 凭据：会话令牌，不是密码

`/automation` 管理页**不存密码**：输入后立刻 `POST /auth/token` 换成一枚 1 小时有效的会话令牌，
之后所有请求走 `Authorization: Bearer <令牌>`，并在到期前自动 `POST /auth/token/refresh`。

令牌存在 `sessionStorage['roomAuthCache']`，**与 SPA 是同一个键、同一种结构**：

```
{ "<房间>": { token: "...", expiresAt: 1790225666 } }
```

- 房间键：`default` 房间用 `__default__`，其余用房间名本身；
- 用**全局密码**换来的令牌 `scope=global`，放在 `__global__` 下，对任意房间有效；
- `expiresAt` 是 Unix **秒**（不是毫秒）。

于是：

- 在**同一个标签页**里从 SPA 跳到 `/automation`（或反向）**不需要二次登录**；
- 关掉标签页令牌即失效，不会长期留在磁盘上；
- 两边共用同一套续签规则，不会出现一边过期、另一边还能用。

> **入口**：SPA 工具条上的自动化按钮会带着当前房间跳到 `/automation?room=<房间>`，
> 管理页右上角的「返回主界面」带回同一个房间。这一跳是**同标签页**的，不是新窗口 ——
> 新标签页的 `sessionStorage` 是空的，用户会被再要一次密码。所以这个链接**不能加
> `target="_blank"`**，有测试盯着（`TestSpaToolbarEntryStaysInSameTab`）。

> 语言设置同理共用一个键：`localStorage['locale']`，取值 `zh` / `zh-TW` / `en` / `ja`。
> 语言优先级：`?lang=` > `localStorage['locale']` > `navigator.language`（判断规则与 SPA 一致）。
> 在任一边切换语言，另一边也会跟着变 —— 不会出现「主界面英文、管理页中文」这种割裂。

> 唯一留在 `localStorage` 的是 `single` 档房间的 **task token**（键 `ccgAutomationTaskToken:<房间>`）：
> 它必须在关标签页之后依然存在，否则用户下次回来就再也改不了自己建的那条任务。
> 它只对一条任务有效、和密码无关，风险面比会话令牌小得多。

> ⚠️ 客户端**不要**自己判断「这个房间有没有密码」来决定要不要显示面板 —— 那会和服务端策略
> 漂开（`{"open": true}` 的房间没密码，但策略可以是 `none`）。能力声明只有一个来源，就是 `/server`。
> 反过来，**隐藏面板不是权限边界**：接口照样会拦（`automation_forbidden`）。

### 8.2 房间不由请求体决定

`POST /tasks` 的请求体里**没有 `room` 字段**，带了也会被忽略。任务的作用房间来自鉴权上下文
（`?room=` 必须通过房间鉴权）。理由和 `/file/:uuid/:name` 那条完全一样：只要存在一个可篡改的
入口，`?room=default` 之类的绕过迟早会被找到。已建好的任务**不允许改房间**。

### 8.3 变量

正文是模板，`{{ }}` 在**触发那一刻**求值。偏移语法与动作库的 `date.add` 一致（`[+-]N[dwmy]`）：

| 变量 | 含义 | 示例 |
|---|---|---|
| `{{date}}` | 基准日的日期 | `2026-09-24` |
| `{{date:+1d}}` | 加一天（`w` / `m` / `y` 同理，单位可省 = 天） | `2026-09-25` |
| `{{weekday}}` | 基准日是周几 | `周四` |
| `{{weekday:+1d}}` | **明天**是周几 | `周五` |
| `{{weekday:en\|+1d}}` | 样式：`zh` / `zh-short` / `en` / `en-short`，与偏移可换序 | `Friday` |
| `{{time}}` | 触发时刻 `HH:MM` | `09:30` |
| `{{datetime}}` | `YYYY-MM-DD HH:MM` | `2026-09-24 09:30` |
| `{{timestamp}}` | Unix 秒 | `1790219280` |
| `{{uuid}}` | 每次求值一个新 UUID | — |
| `{{task}}` / `{{room}}` | 任务名 / 房间名 | — |
| `{{latest}}` | **本房间**最新一条人发的文本 | — |
| `{{latest:房间名}}` | **指定房间**最新一条人发的文本 | — |

⚠️ **偏移挂在变量自己身上**，不是全局共享一个「今天」。要得到「明天是 9-25（周五）」必须写
`{{date:+1d}}` 配 `{{weekday:+1d}}`；把周几写成 `{{weekday}}` 会得到「明天是 9-25（周四）」
这种自相矛盾的文案，而且很难一眼看出来。

未知变量、写错的偏移在**保存时**就报 `invalid_task`，不静默放过 —— 否则用户只会在几小时后
从房间里的乱码发现写错了。求值同样是「全成功或全失败」，不会返回替换了一半的正文。

#### `{{latest}}`：把某个房间的最新消息当输入源

它是全部变量里**唯一一个会读外部状态**的（其余只依赖入参和时钟），所以有三条规则要记住：

1. **只取人发的文本**。文件消息跳过；`source == "automation"` 的消息也跳过 ——
   这是**防回环**：任务读 A 发回 A 时，若不过滤，每一轮的输出都会成为下一轮的输入，
   「前缀」会被一层层叠上去。因此这条任务读到的永远是最新的**人话**。
2. **房间权限按「无人值守」判，从紧**：能用**不需要密码就能读的房间**（公开房间），
   以及**任务自己的房间**。带密码的房间读不了，因为任务是无人值守的、没有密码可带，
   若按「创建者当时能读」授权，就成了提权面（先在读得到的时候建任务，之后对方改密码也照读不误）。
   **例外：管理员**（持全局密码，或用它换来的会话令牌）**可以引用任意房间** ——
   他本来就是 `canAccessRoom` 恒真的那个人，拦他一步都拦不住。
   建任务与试算时会校验，越界返回 `source_room_forbidden`。
3. **来源为空记 `skipped`，不是 `error`**。空房间是常态（没人发言、或消息被顶掉了），
   记 error 会让「上次失败」长期挂在那条任务上，用户以为功能坏了。
   手动 `POST /tasks/:id/run` 是交互式动作，那种情况下直接返回错误原因。

### 8.4 动作链

`chain` 是**一串步骤**，按顺序作用在渲染后的正文上，**某一步失败就停在那里**（与前端 `runChain`
一致：后一步的输入依赖前一步的输出，硬跑下去只会得到一条看起来正常的错误消息）。

每一步有两种写法，**都认**：

```json
"chain": ["text.trimLines", {"id": "text.replace", "params": {"find": "内网", "with": "外网"}}]
```

- **字符串** —— 无参数步骤（绝大多数）；
- **`{id, params}`** —— 带参数步骤。`params` 是**这一步自己的**，所以同一个动作可以在一条链上
  出现两次、两次用不同参数（「替换 A→B」再接「替换 C→D」是合法意图，链本来就允许重复）。

> ⚠️ 无参数的步骤**写回时仍是字符串**。老版本存的 `tasks.json` 里全是字符串形态，
> 读进来再写出去不会变形 —— 升级不会动到已有数据。

服务端只实现「无人值守也能跑」的那一档 —— 纯函数、只依赖入参和时钟、不发请求：

| 分组 | 动作 |
|---|---|
| 格式化 | `format.json.pretty` `format.json.min` |
| 文本 | `text.trimLines` `text.dropBlank` `text.dedupe` `text.sort` `text.upper` `text.lower` `text.replace` `text.reverse` `text.extractUrl` `text.extractEmail` `text.extractPhone` `text.extractIp` `text.extractNumber` |
| 编解码 | `encode.base64` `encode.base64.decode` `encode.url` `encode.url.decode` `encode.hex` `encode.hex.decode` `encode.html` `encode.html.decode` `encode.unicode` `encode.unicode.decode` |
| 中文 | `zh.fullwidth` `zh.halfwidth` `zh.punctuation` `zh.number` |
| 日期 | `date.add` `date.diff` |
| 校验 / 时间戳 | `inspect.sha256` `inspect.timestamp` `inspect.dateToTimestamp` |

**不在这一档的四类**（都不是被砍掉，是「只在客户端可执行」）：

- 产出 HTML 给「看」的：`format.markdown` `format.code`；
- 依赖浏览器里动态 import 的词典：`zh.pinyin*` `zh.simplified` `zh.traditional`；
- 输出本身需要翻译、或判定依赖前端启发式的：`inspect.stats` `inspect.detect`；
- **「生成」类**（`generate.uuid` / `generate.time` / `generate.datetime`）：它们在链里会把前面
  算出来的正文**整个丢掉**。要这些值请用**模板变量**（`{{uuid}}` / `{{time}}` / `{{datetime}}`），
  那是内联的（`订单号：{{uuid}}`），不会覆盖任何东西。

> `text.replace` 是**字面**替换，**不支持正则**：前后端各有一份实现，而 JS 的 RegExp 与 Go 的
> RE2 语义差得远（前瞻、反向引用、Unicode 属性转义……），承诺「两边行为一致」是守不住的。
> `params.find` 为空会报错（不会当成「在每个字符之间插入」）。
>
> 传了不在表里的 id 会 400 并列出可用集合，**不静默跳过**。

### 8.5 创建 / 更新

```bash
curl -X POST "http://localhost:9501/tasks?room=home" \
  -H "Authorization: Bearer <房间密码>" -H "Content-Type: application/json" \
  -d '{
        "name": "值班提醒",
        "freq": "daily",
        "time": "09:30",
        "tz": "Asia/Shanghai",
        "template": "今天是 {{date}}，明天是 {{date:+1d}}（{{weekday:+1d}}）",
        "chain": ["text.trimLines"],
        "keepHistory": false
      }'
```

| 字段 | 说明 |
|---|---|
| `id` | 省略 = 新建；带上 = 整体更新 |
| `name` | 省略则取正文首行 |
| `enabled` | 默认 `true` |
| `freq` | `once` / `daily` / `weekly` / `cron` |
| `time` | `daily` / `weekly` 用，`HH:MM` |
| `cron` | `cron` 用，5 字段表达式（分 时 日 月 周），见 8.6 |
| `byWeekday` | `weekly` 用，`0` = 周日 … `6` = 周六（与 `Date.getDay()` 一致），可多选 |
| `runAt` | `once` 用，RFC3339 或 `2026-10-01T09:30`（按 `tz` 解释）；保存时归一化成 RFC3339 |
| `tz` | 省略则填入 `automation.defaultTZ`（默认 `Asia/Shanghai`），并**写进任务本身** |
| `template` | 正文模板，必填 |
| `chain` | 步骤数组，可选。元素是动作 id 字符串，或 `{id, params}`（见 8.4） |
| `keepHistory` | 默认 `false`：只广播、**不占房间历史额度**（见下） |
| `sender` | 发送者显示名，默认「定时任务」 |

响应 `{"task": {...}}`。房间策略为 `single` 时还会多一个 **`taskToken`**：

```json
{ "task": {"id": "…", "room": "lobby"}, "taskToken": "f4a2…" }
```

> `taskToken` 的明文**只出现这一次**。公开房间的客户端没有别的凭据可用，它是之后修改 / 删除
> 那条任务的唯一钥匙（`X-Task-Token` 头或 `?taskToken=`），丢了就只能在服务端删文件。
> `ownerHash` **永不外发**。

只读字段：`nextRunAt`、`lastRunAt`、`lastStatus`（`ok` / `error` / `skipped`）、`lastError`、`lastOutput`、
以及 cron 任务的 `desc`（见 8.6）。

> **管理页的编辑器只暴露 `cron` 和 `once` 两种写法。** `daily` / `weekly` 仍然被服务端接受
> （老任务要能跑，API 调用方也用得上），但界面统一用 cron 表达：
> `每天 09:30` = `30 9 * * *`，`每周一 10:00` = `0 10 * * 1`。
> 编辑一条老的 `daily` / `weekly` 任务时，界面会把它换算成等价的 cron 表达式，
> 保存后 `freq` 就变成 `cron` —— 触发时刻完全不变，但看任务定义会看到写法变了。
>
> 界面上那五个框（`[框]分 [框]时 [框]日 [框]月 [框]周`）只是**一个表达式的拆装视图**，
> 默认全是 `*`，在客户端拼成 `"<分> <时> <日> <月> <周>"` 后才发出来 —— 协议与本例一致。
> 「常用」按钮只改「日 / 月 / 周」三段，不会替调用方猜时刻。

### 8.6 cron 表达式

`freq: "cron"` 配 `cron: "<分> <时> <日> <月> <周>"`，用来表达结构化字段说不清的那些排期。

| 写法 | 含义 |
|---|---|
| `*` / `?` | 任意（`?` 是 Quartz 的写法，从网上抄来的表达式常常带着它） |
| `5` | 单个值 |
| `1-5` | 范围 |
| `*/10` | 步长 |
| `1-30/5` | 带步长的范围 |
| `1,15,30` | 列表（列表项本身也可以是范围或带步长） |
| `MON-FRI` / `JAN-DEC` | 月份、星期的英文前三个字母，大小写都认 |

| 常用表达式 | 含义 |
|---|---|
| `0 9 * * *` | 每天 09:00 |
| `*/30 9-18 * * 1-5` | 工作日 09:00–18:00 每半小时 |
| `0 10 * * 1` | 每周一 10:00 |
| `0 0 1 * *` | 每月 1 日 00:00 |
| `0 0 29 2 *` | 闰年 2 月 29 日 |
| `0 9 * JAN MON` | 每年 1 月的每个周一 09:00 |

三条要记住的规则：

1. **只认 5 个字段**。带秒的 6 字段表达式会被明确拒绝，并告诉你该怎么改 —— 静默忽略第一段
   会让 `0 */5 * * * *`（本意每 5 分钟）变成「每小时第 0 分」，而用户要过一整天才会发现少发了。
2. **「日」和「星期」是 OR**：两个都被限制时，任一匹配就触发（标准 cron 的语义）。
   `0 9 1 * 1` 是「每月 1 号**或**每周一」。按 AND 理解的话这条一年只触发几次，
   用户会以为任务坏了。
3. **保存时会先算一次未来**：像 `0 0 30 2 *`（2 月 30 日）这种语法合法、却永远等不到的表达式
   会被拒。否则用户会在几个月后才发现任务从没跑过。

校验与预览：

```bash
curl "http://localhost:9501/tasks/cron?room=home&expr=*/30%209-18%20*%20*%201-5&tz=Asia/Shanghai" \
  -H "Authorization: Bearer <房间密码>"
# → {"valid":true,"tz":"Asia/Shanghai","expr":"*/30 9-18 * * 1-5",
#    "next":[...], "nextFormatted":["2026-09-24 09:00 Thu", ...],
#    "desc":{"mode":"everyNMinutes","n":30,"hours":"9-18","day":"weekly",
#            "weekdays":[1,2,3,4,5]}}

# 不合法时**仍然是 200**，只是 valid:false + error：
#   {"valid":false,"error":"只支持 5 个字段（分 时 日 月 周），收到的看起来带「秒」…"}
```

> ⚠️ 表达式不合法返回 **200 + `valid:false`**，不是 400：这个接口的用途就是校验，
> 不合法是它的正常输出之一。客户端会在每次输入时调它，把它当错误抛会让人以为页面坏了。

`desc` 是给「翻译」那一行用的**结构化描述**，`GET /tasks` 里的每条 cron 任务也带同一个字段。
它只回答「长什么样」，**不含任何成句文案** —— 管理页有 zh / zh-TW / en / ja 四份语言，
服务端要是拼一句中文，英文和日文用户就会看到中文。句子由客户端用本地文案表拼。

| `mode` | 含义 | 附带字段 |
|---|---|---|
| `everyMinute` | 每分钟 | 小时被限制时另有 `hours` |
| `everyNMinutes` | 每 N 分钟 | `n`，小时被限制时另有 `hours` |
| `everyNHours` | 每 N 小时的第 M 分 | `n`、`minutes` |
| `minutesEachHour` | 每小时的第 M 分 | `minutes`，小时被限制时另有 `hours` |
| `times` | 具体时刻 | `times`（`HH:MM` 列表），被截断时 `more: true` |
| `unknown` | 归纳不出可靠说法 | 客户端应退回「只看具体时刻」 |

日期部分由 `day` 配 `weekdays` / `dom` / `month` / `dayN` 表达：

| `day` | 含义 | 附带字段 |
|---|---|---|
| `daily` | 每天 | — |
| `weekly` | 每周某几天 | `weekdays`（已展开成具体星期几） |
| `monthly` | 每月某几天 | `dom` |
| `monthlyOrWeekly` | 每月某几天**或**每周某几天 | `dom` + `weekdays` |
| `everyNDays` | 每隔 N 天 | `dayN` |
| `everyNDaysOrWeekly` | 每隔 N 天**或**每周某几天 | `dayN` + `weekdays` |

`monthlyOrWeekly` / `everyNDaysOrWeekly` 对应「日」和「星期」都被限制时的 OR 语义 ——
少这两档就会把「每月 1 号**或**每周一」说成「每月 1 号**且**每周一」，把一年十几次说成一年一次。

> ⚠️ **`everyNDays` 是近似说法**，客户端要把它当近似处理：标准 5 字段 cron 里的 `*/3` 落在「日」上
> 是「每月的 1、4、7…31 日」，跨月时会从 31 号直接跳到下月 1 号 —— **间隔只有 1 天**。
> cron 表达不出真正等距的「每 N 天」。服务端只有在**列不全**具体日期时才用这一档
> （`*/15` 展开只有 3 个日期，就会精确地说 `monthly` + `dom: "1,16,31"`）；
> 管理页在对应的预设按钮上挂了悬停说明，把这件事讲清楚。

> ⚠️ **`desc` 里只会出现人能读的东西**（`1-5`、`1,15`、展开后的 `5,15,25,35,45,55`），
> **绝不会出现 `*`、`/`、`?` 这些表达式符号**。带步长、带英文月份名的字段（`*/3`、`JAN`）
> 会先展开成具体值；展开后太长（超过 6 个，例如 `*/3` 在「日」上是 11 个）就返回 `unknown`，
> 而不是把原文念进句子 —— 「每月 1,4,7…31 日」不是描述，是把代码读了一遍。
> 客户端可以拿这条当不变量来断言。

> ⚠️ `desc` 是**辅助**，不能替代 `nextFormatted`。归纳总有说不全的时候
> （`*/30 9-18 * * 1-5` 说成「每 30 分钟」就漏掉了 9-18 点这个限制），
> 所以界面上两者并排给，用户核对的始终是具体时刻。

### 8.7 试算 / 试跑 / 立即发送 / 开关

```bash
# 试算一条**还没保存**的任务：不落盘、不发送、无副作用
curl -X POST "http://localhost:9501/tasks/preview?room=home&at=2026-09-24T09:30:00%2B08:00" \
  -H "Authorization: Bearer <房间密码>" -H "Content-Type: application/json" \
  -d '{"freq":"daily","time":"09:30","tz":"Asia/Shanghai","template":"明天是 {{date:+1d}}（{{weekday:+1d}}）"}'
# → {"preview":true,"output":"明天是 2026-09-25（周五）", ...}

# 试跑一条已保存的任务（不带 send 就是试跑）
curl -X POST "http://localhost:9501/tasks/<id>/run?room=home" -H "Authorization: Bearer <房间密码>"

# 真发一次
curl -X POST "http://localhost:9501/tasks/<id>/run?room=home&send=1" -H "Authorization: Bearer <房间密码>"

# 启用 / 停用。只动开关，不碰模板和动作链 —— 也**不清幂等键**，
# 所以「关掉再打开」不会补发一次。
curl -X POST "http://localhost:9501/tasks/<id>/toggle?room=home&enabled=0" -H "Authorization: Bearer <房间密码>"
# 不带 enabled 参数就是翻转；也可以用 body {"enabled": true}
```

试算的基准时刻默认取**下次触发时刻**，而不是「现在」：否则下午配一个「每天 09:30、正文写
`{{date:+1d}}`」的任务，试算看到的是今天 +1，而明早真正发出的会是明天 +1 —— 预览反而误导人。
`?at=` 可显式指定（RFC3339，或 `2026-09-25` / `2026-09-25 09:30`）。

### 8.8 调度语义

- 触发精度是**分钟**，扫描间隔是 `automation.tickSeconds`。判定是「现在 ≥ 某个计划时刻」，
  所以重启、休眠不会让整个触发凭空消失。
- 每个计划时刻有一个**幂等键**（任务 id + 该时刻，精确到分钟）：同一趟不会重复发送，
  多个实例 / 多个标签页也不会。
- 错过触发窗口超过 `automation.graceSeconds` 就**跳过不补发**（记 `skipped`）：
  服务重启 10 分钟内会补，宕机一夜不会 —— 补发一条昨天的提醒是纯噪音。
- 补发时正文用**预定的那个时刻**求值，不是实际发送时刻；消息上带 `late: true`。
- `once` 任务跑完（或错过）自动停用。
- 定时消息在载荷上带 `source: "automation"`、`scheduledAt`、`late` 三个字段，客户端可据此加角标。
- 定时消息默认**不占房间历史额度**：只走 WebSocket 实时广播，不进历史队列。原因很实际 ——
  房间历史是按房间计数的（`server.history`），一个每天发一次的任务十几天就能把房间里的历史
  全换成「今天是几号」。要留存就把 `keepHistory` 设为 `true`。

---

## 9. 实时推送

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

## 10. 错误码表

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
| `automation_disabled` | 404 | 服务端未启用定时自动化（`automation.enabled = false`） |
| `automation_forbidden` | 403 | 该房间未开放自动化 |
| `task_not_found` | 404 | 定时任务不存在 |
| `task_forbidden` | 403 | 这条任务不属于你（或属于别的房间） |
| `task_limit_reached` | 400 | 该房间的定时任务条数已达上限 |
| `invalid_task` | 400 | 任务定义不合法（变量写错、时间格式错、动作不可用…） |
| `render_failed` | 400 | 试算时求值失败 |
| `invalid_reference` | 400 | `?at=` 给的基准时刻无法解析 |
| `source_room_forbidden` | 400 | 正文里的 `{{latest:房间}}` 指向一个需要密码的房间（无人值守读不了） |
| `invalid_timezone` | 400 | 时区名无法识别 |
| `internal_error` | 500 | 服务端内部错误 |

---

## 11. 客户端实现建议

1. **先调 `/server`** 拿限制值，不要写死。
2. **限制是动态的**：超限错误里的数字来自服务端配置（`text.limit` / `file.limit`），
   客户端应原样展示服务端给的 `message`，而不是自己拼一句。
3. **错误只解析一套**：读 `message`（中文）或 `error`（英文），别按 `Accept` 分叉。
4. **文件下载记得带凭据**，且**别信自己的 `room`**。
5. **`name` 与 `client` 别混用**：前者给人看，后者给程序判归属。
6. **别依赖「不传 room = default」**：不传 `room` 在部分端点意味着「不限房间」，
   想指定默认房间就显式写 `?room=default`。
