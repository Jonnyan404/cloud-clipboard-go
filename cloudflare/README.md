# Cloudflare 部署文档

本文档说明如何将 Cloud Clipboard 部署到 Cloudflare Workers（一体化：静态前端 + API），并与当前仓库中的自动化脚本保持一致。

## 部署内容

执行 [cloudflare/deploy.sh](cloudflare/deploy.sh) 后，会依次完成这些步骤：

1. 创建或复用 D1 数据库 `cloud-clipboard-db`
2. 创建或复用 R2 存储桶 `cloud-clipboard-files`
3. 构建 Vue3 前端 `web-vue3` 并拷贝为 Workers 静态资源 `cloudflare/workers/assets`
4. 基于 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 生成临时 `wrangler.toml`
5. 执行 [cloudflare/d1/schema.sql](cloudflare/d1/schema.sql) 远程迁移
6. 部署一体化 Worker（同时托管静态前端与 API）
7. 输出 Worker 访问地址

部署形态与自托管 Go 后端一致：**同一域名下内嵌静态前端，API 同源、无 `/api` 前缀**（`/server`、`/text`、`/upload`、`/push` 等），一套 Vue3 前端通用两种后端。

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

部署成功后，脚本会输出 Worker 地址，直接访问该地址即为前端页面。

## GitHub Actions 自动部署

仓库已提供手动触发的工作流 [.github/workflows/deploy-cloudflare.yml](../.github/workflows/deploy-cloudflare.yml)，可在 GitHub 仓库 **Actions → Deploy Cloudflare Worker → Run workflow** 一键部署，无需在本地登录 Wrangler。

该工作流会：

1. 构建 Vue3 前端并拷贝为 Worker 静态资源；
2. 通过 `wrangler d1 list --json` 动态解析 D1 数据库 ID；
3. 远程执行 D1 迁移（`cloudflare/d1/schema.sql`）；
4. 部署一体化 Worker，并用 GitHub Secrets 覆盖 `[vars]` 中的敏感变量。

### 配置 GitHub Secrets

在仓库 **Settings → Secrets and variables → Actions** 中添加：

| Secret 名称 | 必填 | 说明 |
| --- | --- | --- |
| `CF_API_TOKEN` | ✅ | Cloudflare API Token，权限需包含：Account → Workers Scripts: **Edit**、D1: **Edit**、R2: **Edit**、Durable Objects Namespace: **Edit**、Account Settings: **Read** |
| `CF_ACCOUNT_ID` | ✅ | Cloudflare 账号 ID（Dashboard 首页右下角或 URL 中可查看） |
| `AUTH_PASSWORD` | ✅ | 覆盖 `[vars].AUTH_PASSWORD`，即全局入口密码 |
| `ROOM_AUTH_JSON` | ✅ | 覆盖 `[vars].ROOM_AUTH_JSON`，房间级密码映射（需为合法 JSON 字符串，见下方 `roomAuth 说明`） |
| `ROOM_LIST` | ➖ | 可选，覆盖 `[vars].ROOM_LIST`；不设置则用模板默认值 |
| `HISTORY_LIMIT` | ➖ | 可选，覆盖 `[vars].HISTORY_LIMIT` |
| `TEXT_LIMIT` | ➖ | 可选，覆盖 `[vars].TEXT_LIMIT` |
| `FILE_LIMIT` | ➖ | 可选，覆盖 `[vars].FILE_LIMIT` |
| `FILE_EXPIRE` | ➖ | 可选，覆盖 `[vars].FILE_EXPIRE` |

> 说明：
> - 未设置的可选变量将使用 `wrangler.toml.template` 中的默认值，工作流会给出 warning 提示；
> - 强烈建议至少配置 `CF_API_TOKEN`、`CF_ACCOUNT_ID`、`AUTH_PASSWORD`、`ROOM_AUTH_JSON` 四个必填项，避免在生产使用模板中的占位密码。

## 静态资源与配额

Workers 通过 `assets` 配置托管前端静态资源：

```toml
[assets]
directory = "./assets"
not_found_handling = "single-page-application"
```

基于默认的资产优先路由（不要设置 `run_worker_first`）：

- **静态资源**（HTML/JS/CSS 等）与 **SPA 导航请求** 免费且不限量，不调用 Worker 代码、不计配额；
- **仅 API 请求**（前端通过 XHR/fetch 发起的非导航请求，如 `/server`、`/text`、`/upload`、`/push`）才会调用 Worker，计入每日配额。

## 可配置项

Cloudflare Workers 默认变量定义在 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)。

当前 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 里的 `vars` 目前包括这些变量：

例如：

```toml
[vars]
AUTH_PASSWORD = "123"
ROOM_AUTH_JSON = "{\"private\":\"\",\"finance\":\"finance-pass\"}"
ROOM_LIST = "false"
HISTORY_LIMIT = "50"
TEXT_LIMIT = "40960"
FILE_LIMIT = "204857600"
FILE_EXPIRE = "3600"
```

| 变量 | 默认值 | 类型 | 说明 |
| --- | --- | --- | --- |
| `AUTH_PASSWORD` | `"123"` | 字符串或布尔语义 | 全局入口密码。只要设置了就对所有房间生效，保证旧密码升级后仍可用 |
| `ROOM_AUTH_JSON` | `{"private":"","finance":"finance-pass"}` | JSON 字符串 | 房间级密码映射。不会让 `AUTH_PASSWORD` 失效，而是为指定房间增加额外可用密码。值也支持对象形式 `{ "password": "xx", "fileExpire": N }`，可同时配置该房间文件的过期策略（见下方 roomAuth 说明） |
| `ROOM_LIST` | `"false"` | 布尔语义字符串 | 是否启用房间列表功能，支持 `1`、`true`、`yes`、`on` |
| `HISTORY_LIMIT` | `"50"` | 整数字符串 | 每个房间保留的历史消息条数 |
| `TEXT_LIMIT` | `"40960"` | 整数字符串 | 单条文本消息最大长度 |
| `FILE_LIMIT` | `"204857600"` | 整数字符串 | 单个文件上传大小上限，单位字节 |
| `FILE_EXPIRE` | `"3600"` | 整数字符串 | 文件过期时间，单位秒 |

### roomAuth 说明

`ROOM_AUTH_JSON` 需要是一个 JSON 字符串，对应后端的 `server.roomAuth`。

每个房间的值支持两种形式：

1. **字符串/数字**（旧格式）：仅作为该房间的额外密码；
2. **对象** `{ "password": "xx", "fileExpire": N }`：同时配置密码与该房间文件的过期策略。

示例：

```toml
ROOM_AUTH_JSON = "{\"finance\":{\"password\":\"finance-pass\",\"fileExpire\":0},\"archive\":{\"fileExpire\":604800},\"private\":\"\",\"ops\":\"ops-pass\"}"
```

含义：

- `finance` 房间额外密码 `finance-pass`，且该房间上传的**文件永不过期**（`fileExpire: 0`）；
- `archive` 房间无独立密码，文件保留 7 天（覆盖全局 `FILE_EXPIRE`）；
- `private: ""` 表示 `private` 房间只接受全局 `AUTH_PASSWORD`
- `ops: "ops-pass"` 表示 `ops` 房间同时接受全局 `AUTH_PASSWORD` 和 `ops-pass`

`fileExpire` 取值：

| 取值 | 含义 |
| --- | --- |
| 不填 | 使用全局 `FILE_EXPIRE`（默认行为） |
| `0` | 该房间上传的文件**永不过期** |
| `> 0` | 覆盖全局 `FILE_EXPIRE`，单位秒 |
| `< 0` 或非法 | 视为配置错误，回退全局 `FILE_EXPIRE` 并输出警告日志 |

注意事项：

- **`fileExpire` 只影响修改配置之后上传的文件**：过期时间在上传瞬间写入 R2 元数据和 D1 记录，之后修改 ROOM_AUTH_JSON 不会回溯变更已有文件；
- 历史条数轮转删除不受 `fileExpire` 影响：房间消息数超过 `HISTORY_LIMIT` 时，最旧的文件仍会被清理。低频房间配合 `fileExpire: 0` 约等于永久保存；
- 前端会把永久文件显示为“永久有效”。

如果你想修改这些变量，有两种方式：

1. 在部署前直接编辑 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)，然后重新执行 [cloudflare/deploy.sh](cloudflare/deploy.sh)
2. 部署完成后，在 Cloudflare Dashboard 的 Workers 设置中修改变量

注意：除了这些 `vars`，模板里还有几类不是“环境变量”的部署配置：

- D1 绑定：`DB`
- R2 绑定：`R2_BUCKET`
- Durable Object 绑定：`WEBSOCKET_ROOM`

这些绑定项同样是运行所必需的，但它们不属于 `vars`，通常由部署脚本自动处理，不需要像密码或限制值那样日常调整。

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
- 重新构建前端并部署一体化 Worker

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

### 3. 修改了 `wrangler.toml`，但文件又消失了

这是正常行为。部署脚本会在运行时临时生成 `cloudflare/workers/wrangler.toml`，部署结束后自动清理。

如果你要改默认值，请修改模板文件 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)，而不是临时生成文件。

### 4. 修改了密码或 `ROOM_AUTH_JSON` 后未生效

确认你修改的是模板文件 [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template) 或 Cloudflare Dashboard 中的 Worker Variables，然后重新部署。

### 5. 前端能打开，但 API 或 WebSocket 连接异常

优先检查：

1. Worker 是否部署成功
2. Worker 变量中的 `AUTH_PASSWORD` / `ROOM_AUTH_JSON` / `ROOM_LIST` 是否符合预期
3. D1 schema 是否已经迁移到远程数据库
4. R2 存储桶与 Durable Object 绑定是否正确

## 相关文件

- [cloudflare/deploy.sh](cloudflare/deploy.sh)
- [cloudflare/d1/schema.sql](cloudflare/d1/schema.sql)
- [cloudflare/workers/wrangler.toml.template](cloudflare/workers/wrangler.toml.template)
- [cloudflare/workers/src/index.js](cloudflare/workers/src/index.js)
- [cloudflare/workers/src/handlers/file.js](cloudflare/workers/src/handlers/file.js)
- [web-vue3/dist](../web-vue3/dist)（前端静态资源构建产物）
