# Cloudflare 部署文档

本文档说明如何将 Cloud Clipboard 部署到 Cloudflare Workers + Pages，并与当前仓库中的自动化脚本保持一致。

## 部署内容

执行 [cloudflare/deploy.sh](cloudflare/deploy.sh) 后，会依次完成这些步骤：

1. 创建或复用 D1 数据库 `cloud-clipboard-db`
2. 创建或复用 R2 存储桶 `cloud-clipboard-files`
3. 基于 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 生成临时 `wrangler.toml`
4. 执行 [cloudflare/d1/schema.sql](cloudflare/d1/schema.sql) 远程迁移
5. 部署 Workers API
6. 基于 [cloudflare/pages/client/src/config.js.template](cloudflare/pages/client/src/config.js.template) 生成临时 `config.js`
7. 构建并部署 Cloudflare Pages 前端
8. 输出 Worker API 地址和 Pages 访问地址

## 前置要求

- Node.js
- npm
- Cloudflare 账号
- 已安装或可自动安装 Wrangler CLI

建议先确认：

```bash
node -v
npm -v
wrangler --version
```

如果未登录 Wrangler：

```bash
wrangler login
```

也可以手动确认当前登录状态：

```bash
wrangler whoami
```

## 一键部署

在仓库根目录执行：

```bash
cd cloudflare
bash deploy.sh
```

部署成功后，脚本会输出两类地址：

- Worker API 地址
- Cloudflare Pages 前端地址

## 可配置项

Cloudflare Workers 默认变量定义在 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)。

当前 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 里的 `vars` 目前包括这些变量：

例如：

```toml
[vars]
AUTH_PASSWORD = "123"
ROOM_AUTH = "{\"private\":\"\",\"finance\":\"finance-pass\"}"
ROOM_LIST = "false"
HISTORY_LIMIT = "50"
TEXT_LIMIT = "40960"
FILE_LIMIT = "204857600"
FILE_EXPIRE = "3600"
```

| 变量 | 默认值 | 类型 | 说明 |
| --- | --- | --- | --- |
| `AUTH_PASSWORD` | `"123"` | 字符串或布尔语义 | 全局入口密码。只要设置了就对所有房间生效，保证旧密码升级后仍可用 |
| `ROOM_AUTH` | `{"private":"","finance":"finance-pass"}` | JSON 字符串 | 房间级密码映射。不会让 `AUTH_PASSWORD` 失效，而是为指定房间增加额外可用密码 |
| `ROOM_LIST` | `"false"` | 布尔语义字符串 | 是否启用房间列表功能，支持 `1`、`true`、`yes`、`on` |
| `HISTORY_LIMIT` | `"50"` | 整数字符串 | 每个房间保留的历史消息条数 |
| `TEXT_LIMIT` | `"40960"` | 整数字符串 | 单条文本消息最大长度 |
| `FILE_LIMIT` | `"204857600"` | 整数字符串 | 单个文件上传大小上限，单位字节 |
| `FILE_EXPIRE` | `"3600"` | 整数字符串 | 文件过期时间，单位秒 |

### roomAuth 说明

`ROOM_AUTH` 需要是一个 JSON 字符串，对应后端的 `server.roomAuth`。

示例：

```toml
ROOM_AUTH = "{\"private\":\"\",\"finance\":\"finance-pass\",\"ops\":\"ops-pass\"}"
```

含义：

- `private: ""` 表示 `private` 房间只接受全局 `AUTH_PASSWORD`
- `finance: "finance-pass"` 表示 `finance` 房间同时接受全局 `AUTH_PASSWORD` 和 `finance-pass`
- `ops: "ops-pass"` 表示 `ops` 房间同时接受全局 `AUTH_PASSWORD` 和 `ops-pass`

如果你想修改这些变量，有两种方式：

1. 在部署前直接编辑 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)，然后重新执行 [cloudflare/deploy.sh](cloudflare/deploy.sh)
2. 部署完成后，在 Cloudflare Dashboard 的 Workers 设置中修改变量

注意：除了这些 `vars`，模板里还有几类不是“环境变量”的部署配置：

- D1 绑定：`DB`
- R2 绑定：`R2_BUCKET`
- Durable Object 绑定：`WEBSOCKET_ROOM`

这些绑定项同样是运行所必需的，但它们不属于 `vars`，通常由部署脚本自动处理，不需要像密码或限制值那样日常调整。

## 前端配置来源

Pages 前端运行时会使用临时生成的 `config.js`，其模板来自 [cloudflare/pages/client/src/config.js.template](cloudflare/pages/client/src/config.js.template)。

部署脚本会自动把 Worker 地址写入：

- `apiBaseURL`
- `wsBaseURL`

因此正常情况下不需要手动修改前端配置。

## 数据库迁移

数据库结构定义在 [cloudflare/d1/schema.sql](cloudflare/d1/schema.sql)。

部署脚本会自动执行：

- 远程 D1 迁移：始终执行
- 本地 D1 迁移：仅在环境允许时执行

如果你后续修改了 schema，可以重新执行部署脚本，或单独运行：

```bash
cd cloudflare/workers
wrangler d1 execute cloud-clipboard-db --file=../d1/schema.sql --remote
```

## macOS 12 注意事项

当前脚本已经兼容较老的 macOS，但如果你使用的是 macOS 13.5 以下版本，本地 D1 迁移会被自动跳过，因为 `workerd` 本地运行有系统版本要求。

这不会影响远程部署。

如果你想显式跳过本地 D1 迁移，可以这样执行：

```bash
cd cloudflare
SKIP_LOCAL_D1=1 bash deploy.sh
```

## 重新部署

如果你只修改了 Workers 变量或逻辑，通常重新执行即可：

```bash
cd cloudflare
bash deploy.sh
```

脚本会自动：

- 复用已存在的 D1 数据库
- 复用已存在的 R2 存储桶
- 重新部署 Worker 和 Pages

## 常见问题

### 1. `wrangler whoami` 提示未登录

先执行：

```bash
wrangler login
```

### 2. 本地 D1 迁移失败

如果是 macOS 版本较低，可直接跳过本地迁移：

```bash
SKIP_LOCAL_D1=1 bash deploy.sh
```

### 3. 修改了 `wrangler.toml` 或 `config.js`，但文件又消失了

这是正常行为。

部署脚本会在运行时临时生成：

- `cloudflare/workers/wrangler.toml`
- `cloudflare/pages/client/src/config.js`

部署结束后会自动清理。

如果你要改默认值，请修改模板文件，而不是改临时生成文件：

- [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)
- [cloudflare/pages/client/src/config.js.template](cloudflare/pages/client/src/config.js.template)

### 4. 修改了密码或 `ROOM_AUTH` 后未生效

确认你修改的是模板文件 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 或 Cloudflare Dashboard 中的 Worker Variables，然后重新部署。

### 5. Pages 能打开，但 API 或 WebSocket 连接异常

优先检查：

1. Worker 是否部署成功
2. Pages 生成的 `config.js` 是否已写入正确的 Worker URL
3. Worker 变量中的 `AUTH_PASSWORD` / `ROOM_AUTH` / `ROOM_LIST` 是否符合预期
4. D1 schema 是否已经迁移到远程数据库

## 相关文件

- [cloudflare/deploy.sh](cloudflare/deploy.sh)
- [cloudflare/d1/schema.sql](cloudflare/d1/schema.sql)
- [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)
- [cloudflare/pages/client/src/config.js.template](cloudflare/pages/client/src/config.js.template)
- [cloudflare/workers/src/index.js](cloudflare/workers/src/index.js)
