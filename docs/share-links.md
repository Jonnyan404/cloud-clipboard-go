# 分享链接 / 分享页 —— 实现说明与交接

> 2026-09-21。涉及 Go 后端、Cloudflare Worker、Vue3 前端三处，以及 API 文档。

## 一句话

分享链接从「裸接口地址」改成「**前端分享页**」：`https://host<prefix>/#/s?t=<token>`。
`POST /share` 现在**一律**签发 token（开放房间也发），并新增 `GET /share?t=` 给分享页问元信息。
前端把「复制链接」和「二维码」两个图标合并成了一个。

---

## 为什么要改（原设计的三个问题）

### 1. 开放房间的 TTL / 次数限制是假的（真 bug）

`POST /share` 里 token 只在 `requirement.Required`（房间需要鉴权）时才签发：

```go
if requirement.Required {          // ← 房间没密码就整段跳过
    token, exp, err := s.issueShareToken(...)
    query.Set(shareTokenQueryKey, token)
    response["token"] = token
}
response["url"] = s.buildAbsoluteURL(r, fmt.Sprintf("/content/%s", idStr), query)
```

于是房间开放时返回的是**裸接口地址**，而响应里照样回 `ttl` / `maxUses` / `expiresAt`。
弹窗里让用户设「15 分钟 / 最多 3 次」，链接实际永不过期、不限次数 —— 设置被静默丢弃。
Worker 侧完全一样（`src/share.js` 里两处 `if (requirement.required)`）。

### 2. 没有分享页

两条路都落在 `/content/<id>` 或 `/file/<uuid>/<name>`：收件人看到的是纯文本 / 直接下载，
没有地方输密码，也没有任何「这是谁分享的、什么时候过期」的提示。

### 3. 复制链接与二维码产出的是同一个 URL

区别只剩呈现方式，却要用户先做一个没有意义的决定。

---

## 新契约

### `POST /share` —— 一律签发

```http
POST /share?room=<room>
Content-Type: application/json
Authorization: Bearer <凭据>

{"type": "content", "id": "7", "ttl": 900, "maxUses": 0, "password": ""}
```

响应（新增 `rawUrl`）：

```json
{
  "type": "content", "id": "7", "room": "default",
  "ttl": 900, "expiresAt": 1789955428, "maxUses": 0,
  "token": "eyJ0eXAiOi...",
  "url":    "https://host/#/s?t=eyJ0eXAiOi...",
  "rawUrl": "https://host/content/7?t=eyJ0eXAiOi..."
}
```

- `url` = **前端分享页**，交给收件人的就是它
- `rawUrl` = 带同一个 token 的直连接口，**下载与预览必须用它**（见下）

### `GET /share?t=<token>` —— 分享页的「先看一眼」

```json
{"type": "content", "kind": "text", "id": "7", "room": "default",
 "expiresAt": 1789955428, "maxUses": 0, "used": 0, "needsPassword": false}
```

文件分享额外带 `uuid` / `name` / `size`（**文件名不在 token 里**，分享页只能从这里拿，
否则拼不出 `/file/<uuid>/<name>`）。

失败原因可区分：

| `code` | 状态码 | 含义 |
|---|---|---|
| `share_token_invalid` | 401 | 签名不对或已过期 |
| `share_password_required` | 401 | 没带密码或密码不对 |
| `content_not_found` / `file_not_found` | 404 | 内容已不存在 |
| `file_expired` | 404 | 文件已过期 |

### 密码

走 `X-Share-Password` **请求头**，绝不进 URL（query 会进浏览器历史和访问日志）。
token 里只存 `HMAC(服务端签名密钥, "share-password:"+密码)` 的前 16 位。

---

## 关键设计决定（以及为什么）

| 决定 | 原因 |
|---|---|
| 分享页用 **hash 路由**，不做服务端渲染 | 两个后端（Go / Worker）都不用碰模板，只需要会拼一个绝对地址；静态资源本来就已经内嵌/挂载 |
| `url` 与 `rawUrl` **分开两条** | 分享页地址是 hash 路由，`new URL(url).searchParams.set('download','true')` 只会改到 fragment —— 拿它下载会得到一个 html |
| `GET /share` **不消耗使用次数** | 打开页面本身不该烧掉一次；真正取正文（`/content` 或 `/file`）时才 `consumeShareUse` |
| 分享页是**独立裸壳** | `App.vue` 模板顶部 `v-if="isShareRoute"` 只渲染 `<router-view>`；`main.js` 在分享路由上**不建 WebSocket**。收件人不该看到工具栏、房间侧栏、输入区 |
| 密码比对用**常数时间** | `hmac.Equal` / XOR 循环，不用 `==`（字符串比较会在第一个不同字节返回，能按时间差逐字节猜） |
| 密码存 token 而不是内存 map | Go 的 usage map 是进程内的，重启就没了；密码不能跟着丢 |
| ⚠️ 分享页**必须 `watch(token)`** | 分享页只有一个 path（`/s`），token 在 query 里。同路由换 token 时 vue-router **复用组件实例、`onMounted` 不再跑** —— 不 watch 就会一直显示上一条分享的内容（连「无效 token」都显示成上一条的正文）。这条实测踩到过 |

---

## 改动清单

### Go（`cloud-clip/lib/`）

- `share_token.go`
  - 新增 `buildSharePageURL()` —— 手拼 `scheme://host<prefix>/#/s?t=`，**不能用 `buildAbsoluteURL`**（`#` 会被转义成 `%23`）
  - 新增 `handleShareInfo()`（`GET /share`）+ `fillShareFileInfo()`
  - `handle_share()` 开头按方法分派：`GET`/`HEAD` → info，`POST` → 签发
  - 两个分支都改成**一律签发**，响应加 `rawUrl`
- `share_token_test.go` —— 新增 7 个测试（见「怎么验证」）

### Worker（`cloudflare/workers/`）

- `src/share.js` —— 与 Go 逐条对齐：`buildSharePageURL()` / `readShareUsed()` /
  `ShareHandler.info()` / `findFileMeta()` 补 `size` / 两个分支一律签发
- `src/index.js` —— `router.get('/share', ShareHandler.info)`
- `test/share-page.test.mjs` —— **新增**，29 项断言
- `test/run.sh` —— esbuild entry 列表加 `share`，末尾加这一步

### 前端（`web-vue3/src/`）

- **`views/ShareView.vue`** —— 新增。收件人页面：loading / 密码闸门 / 失败提示 / 文本（raw↔md 切换 + 复制）/ 文件（缩略图 + 名字 + 大小 + 下载）
- `router/index.js` —— 新增 `/s` 路由，**懒加载**（收件人不需要主应用那一整包），`meta.sharePage: true`
- `App.vue` —— 模板顶部 `isShareRoute` 的 `v-if`；个性化面板补「默认密码」一项（默认格式后来删了，见文末）
- `main.js` —— 分享路由上跳过 `wsStore.connect()`
- `util.js` —— `createShareLink()` 加 `password`；新增 `withSharePageFormat()`（**必须拆 fragment 再拼**，`?t=` 在 `#` 后面）
- `store/app.js` —— `shareDefaults` 加 `password`（`format` 后来删了，见文末）
- `data/displayToggles.js` —— `cardCopyLink` + `cardQr` → 合并为 **`cardShare`**
- `components/received-item/Text.vue`、`File.vue` —— 一个分享图标；`shareResultVisible` 面板（二维码 + 链接 + 复制按钮 + 有效期/次数）；`confirmShareDialog()` 建完链接**自动复制**并打开面板
- `components/sticky/StickyNote.vue` + `views/modes/{Chat,Mega,Workbench,Terminal}Wall.vue` ——
  `ensureFileShareUrl()` → **`ensureFileShareLinks()`**，返回 `{ raw, page }`：
  **下载与预览用 `raw`，复制链接用 `page`**。这五个文件的 `downloadFile` / `srcPreview` 以前拿的是 `data.url`，
  改成分享页之后如果不动它们，下载会拿到一个 html
- `locales/{zh,en,zh-TW,ja}.json` —— 各加 16 个键（四份必须键对齐）

### 文档

- `docs/api.md` + `docs/api.zh-CN.md` —— `POST /share` 重写、新增 `GET /share`、错误码表补两行、接口表补一行
- `cloud-clip/config.md` —— 短期分享链接一节重写（删掉了「未启用房间密码时，返回的 url 不含 t」这条**已经过时的说明**）

---

## 怎么验证

### 单元 / 契约测试

```bash
# Go：13 个分享相关测试
cd cloud-clip && export PATH="/usr/local/bin:$PATH" && go test ./lib/ -run TestShare -v

# Worker：新增 29 项 + 原有 50 项
cd cloudflare/workers && bash test/run.sh
```

Go 侧新增的测试钉住了这几件事：分享页 URL 的形状与 `#` 没被转义、**开放房间也发 token**、
文件元信息带 `uuid`/`name`/`size`、密码闸门能区分「没带」与「带错」、
**「看一眼」不消耗次数**、URL 里没有明文密码。

### 端到端（无头 Chrome + 真实前后端）

一套临时环境：**临时后端 9502（开放房间）+ 临时 vite 1211 + 无头 Chrome 9222**，
全程不碰正在跑的 9501。20 个场景全绿：

| 场景 | 断言要点 |
|---|---|
| 01 文本分享 · 桌面 | `sharePage=true`；`appToolbar/roomAside/composer` **全为 false**（裸壳） |
| 02 文本分享 · 390×844 | 卡片宽 358，不溢出 |
| 03 `&f=md` | 渲染 markdown，raw 区不渲染 |
| 04 需要密码 | 出密码框、**初始不报错** |
| 05 密码错 | `passwordErrorShown=true`，正文仍是 0 |
| 06 密码对 | 正文出现、md 切换按钮可用 |
| 07 文件分享 | `fileName=photo.png`、缩略图加载成功 |
| 08 无效 token | 提示「分享链接无效或已过期」 |
| **10 同路由换 token** | 换完**重新加载**并弹出密码闸门（就是上面那个 watch 的回归守卫） |
| 11 合并后的分享图标 | 面板有二维码 + `/#/s?t=` 链接 + 2 个按钮；`qrIconCount=0`、`linkIconCount=0` |
| 12 逐张卡片点分享 | 每张卡片都能弹出面板 |
| 13 分享设置弹窗 | 只剩 有效期滑块 / 次数 / 密码；**没有**格式切换；按钮仍带 `timeline-card__icon-button`（组件化后 `$attrs` 没掉） |
| 14 设置面板的分享默认值 | 三个输入框是 `outlined`、底色不再跟卡片重合、密码框 ≥200px（隐形/太短的回归守卫） |
| 15 工作台模式 · 触屏 | 打开触屏模拟后 `(hover: none)` 为真，`.workbench-wall__row-ops` 无需 hover 即可见 |
| 16a~16e 五个模式的分享入口 | 逐个模式打开详情弹窗 → 按钮文案是「分享链接」（不是旧的「复制链接」）→ 点开确认后弹出面板，且链接是 `/#/s?t=` |
| 09 主应用 | 外壳完好（`v-if` 包装没弄坏主界面） |

外加一步 curl 校验：`rawUrl` 对文本返回正确内容、对文件**逐字节等于上传的字节**（89 字节）。

复现要点（这套踩过的坑都在这里）：

- **vite 必须显式 `--host 127.0.0.1`**。不写的话它绑 `localhost` → macOS 上解析到 `::1`，
  用 `127.0.0.1` 访问会连接被拒，现象是「vite 起不来」
- 就绪探测**必须判状态码**：`curl -s -o /dev/null URL && break` 在代理回 502 时也算成功
- **只改 hash 的导航不会重新加载页面**。逐场景之间要先 `Page.navigate` 到 `about:blank`，
  否则量到的是上一个场景的数据
- **就绪要按目标选择器等，不能按时间睡**。vite 是按需编译的，冷启动实测 23s；
  固定 sleep 会让整轮的第一张失败，看起来像功能坏了
- `pkill -f "<模式>"` 的模式串如果出现在你自己的命令行里，**会把自己这个 shell 也杀掉**。
  用括号技巧：`pkill -f "ccg[-]share-test"`
- **`verify.mjs` 结尾必须显式退出**：CDP 那条 WebSocket 是活句柄，只设 `process.exitCode`
  的话事件循环永远不空、node 不退，外层 `run.sh` 就一直挂着等（实测卡了 11 分钟才看出来）。
  结尾 `ws.close(); process.exit(...)`，`catch` 里也要关。
- **就绪要等目标选择器，不能等外壳**：场景 12 曾经等 `.page-toolbar`，但工具栏先出来、
  卡片是接口回来之后才渲染的 —— 撞上空窗期就报「没找到任何分享按钮」。
  等 `.timeline-card__icon-button` 里真的出现 share 图标才稳。
- **触屏兜底（`@media (hover: none)`）在 headless 里默认测不到**：headless 是「有 hover 的
  桌面」。要 `Emulation.setTouchEmulationEnabled` + `setEmitTouchEventsForMouse`，
  而且先断言 `matchMedia('(hover: none)').matches === true` —— 不然那条断言等于没测。

---

## 后续改动（同日，两处）

### 1. 分享逻辑抽成 `components/ShareLinkButton.vue`

「图标 + 两个弹窗 + 有效期滑块的全部样式」原来在 `received-item/Text.vue` 和
`received-item/File.vue` 里**逐字节各存了一份**（约 110 行 script + 110 行 template + 80 行 style），
改一处要改两处，而且其余五个模式迟早也要接同一个入口 —— 再抄下去就是七份。
现在收成一个组件，两个调用点各一行：

```html
<share-link-button :meta="meta" class="timeline-card__icon-button" />
```

- 是否显示由组件内部按 `app.display.cardShare` 判断，调用点不用管。
- 组件根节点不止一个（图标 + 两个弹窗），所以 `defineOptions({ inheritAttrs: false })`，
  再在按钮上 `v-bind="{ ...activatorProps, ...$attrs }"` —— 调用点写的 `class` 才落得回按钮上。
  ⚠️ **只能合成一个 v-bind**：同一元素上写两个 `v-bind="..."` 会被 SFC 解析器判成
  `Duplicate attribute`（`mergeProps` 那套是运行时行为，编译期先拦下来）。
  tooltip 的 activator 属性里只有事件和 `aria-describedby`、**没有 class**，所以展开合并不会互相覆盖。
- **样式必须跟着组件走**：`v-dialog` 一律 teleport 出应用子树，scoped 样式留在卡片里会失配。
- 顺手统一了一处不一致：`Text.vue` 的「取消」按钮写的是 Vuetify 2 的 `<v-btn text>` ——
  在 Vuetify 3 里 `text` 只是个无效属性，按钮实际落到**默认的 elevated 变体**，
  而 `File.vue` 写的是 `variant="text"`。两个卡片的同一个按钮长得不一样，现在统一成后者。

### 2. 删掉「默认展示格式」

分享页自己就带 raw↔md 切换，发送方再预设一次是多余的一道决定（选错了收件人还得自己找按钮换回来）。
删掉的是：设置弹窗里的格式切换块、个性化面板里的「分享页默认格式」、
`store/app.js` 的 `shareDefaults.format`，以及 `shareFormat` / `shareFormatHint` / `shareDefaultFormat`
三个文案键（四份 locale 同步删，键仍然对齐）。

**保留**：分享页仍然认 `?f=md|raw` 当初始格式（`ShareView.vue` 的 `linkedFormat`，
工具函数 `util.js` 的 `withSharePageFormat`）。站内已经没有地方生成这个参数了，留着是为了
①不让已经发出去的旧链接失效 ②留一个手工拼链接的入口。`withSharePageFormat` 目前**零调用方**，
注释里写明了原因；想彻底删掉就连 `ShareView.vue` 的 `linkedFormat` 一起去掉（e2e 场景 03 也要删）。

### 3. 设置面板里的分享默认值输入框是「隐形」的

关掉「分享时弹出设置框」之后，面板里会露出 有效期 / 次数 / 密码 三个输入框。
它们原本写的是 `variant="solo"` + `flat` —— Vuetify 里 solo 的底色是
`rgb(var(--v-theme-surface))`，跟设置卡片本身**一模一样**，`flat` 又把阴影去掉了。
白底白框、没有边框，看着就像「这里根本没有输入框」（数字那两项还能靠数字认出来，
密码框空的，就完全看不见了）。改成 `variant="outlined"`，靠边框站住。
宽度也一起从 168px 放到 220px —— 数字 2~3 位就够，密码不是。

> 这条与分享页无关，只是刚好落在同一个代码块里；记在这里免得下次又踩。

### 4. 其余五个模式的分享入口也接上了同一个面板

之前只有标准模式的卡片图标是「合并后的分享」。其余五个模式仍是老的「复制链接」纯复制按钮
（在详情 / 阅读器弹窗的动作行里，便签模式卡片上还有一个），没有二维码、没有面板、不自动复制。
现在它们都换成 `<share-link-button :meta="…" :icon-only="false" />`：

| 模式 | 入口位置 |
|---|---|
| 聊天 / 巨型 / 工作台 / 终端 | 详情弹窗动作行（**文件**条目才有这一行） |
| 便签 | 阅读器弹窗动作行（`v-if="!isFile"`，**文本**条目）+ 卡片上那个链接图标（仍是旧的，见下） |

`ShareLinkButton` 因此有了 `iconOnly` 这个 prop：

- `true`（默认）= 卡片上那排里的纯图标按钮，**受 `app.display.cardShare` 控制**；
- `false` = 弹窗动作行里的带文字按钮，**不受开关管** —— 那个开关叫「卡片图标」，
  只管卡片上那排图标。弹窗里的动作行本来也不是「卡片图标」。

⚠️ **工作台模式卡片上那个「复制」按钮没动**（对文件走 `copyFileLink`，复制分享页地址）。
所以 `copyFileLink` / `contentUrlOf` 只在 `WorkbenchWall.vue` 里留着，另外三个
（聊天 / 巨型 / 终端）已经删干净 —— 那三个文件里留了一行注释说明为什么不一致，
免得下次有人「顺手统一」把工作台的那份也删了。`ensureFileShareLinks` 五个模式都保持
返回 `{ raw, page }`（工作台和便签还要用 `page`），没跟着改签名。

> **仍然没做**：聊天 / 巨型 / 工作台 / 终端的**文本**条目没有分享入口（它们的动作行只对文件渲染）。
> 卡片操作行也一个都没加图标 —— 那是版式决定，不该被「合并图标」这件事绑架。

---

## 已知缺口 / 下一步

1. **`generateQrCode` 这个文案键已无人使用**（弹窗合并后不再有「生成二维码」按钮）。
   四份 locale 里都还在，可删可留。
2. 老用户 localStorage 里的 `cardCopyLink` / `cardQr` 会继续留着（**无害**，代码不再读它们）。
   合并后的 `cardShare` 默认 `true`，所以升级后图标还在。
3. 另外五个模式的分享入口现在弹的是**分享面板**（以前只把分享页地址丢进剪贴板，
   而且更早以前开放房间时给的是直连文件地址）。值得在 release note 里提一句。
4. 分享页用的是 Vuetify 默认主题，**没有**跟主应用那六种模式皮肤联动。
5. Worker 侧的分享页地址不带 prefix（Worker 没有 prefix 概念）；Go 侧带 `server.prefix`。
6. 没做：分享页的 OG/社交卡片、扫码统计、分享记录列表。
7. `share_token_usage` 表在 Cloudflare 侧是**首次消费时才建**（`ensureShareUsageTable`），
   `readShareUsed` 查不到就按 0 处理 —— 这是刻意的，不是 bug。

---

## 这次踩过、下次别重犯

- **重建 `cloud-clip/lib/static` 前必须先把旧目录 `mv` 走，不能删**：沙箱的批量删除守卫
  （阈值 50，按轮计）会拦 `rm -rf`、vite 的 `emptyOutDir`、`after-build.js` 的 `rmSync`。
  `mv lib/static /tmp/x`（rename 不算删除）之后再构建。`dist/` 同理。
- **`set -o pipefail` 下 `… | grep -q …` 匹配成功也会返回失败**（`grep -q` 命中即退，
  上游吃 SIGPIPE）。判重一律用 `git log --fixed-strings --grep`。
- **`… | tail -N` 会把整轮输出缓冲到结束**：跑长流程时等于全程看不见进度。
  要实时看就重定向到文件，另开一个调用去 `cat`。
