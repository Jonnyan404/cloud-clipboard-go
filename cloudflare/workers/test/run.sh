#!/usr/bin/env bash
# 跑 Cloudflare Worker 的测试。
#
#   receive-path.test.mjs  —— 上传（/upload multipart 与 /text）→ /content/latest?json=1 → Receive 决策树
#   device-name.test.mjs   —— 设备名（?name=）落库与回读，含自愈加列路径
#   asset-routing.test.mjs —— 静态资源路由契约：/file 与 /content 不能被资源层回成 index.html
#   shortcut-contract.test.mjs —— Android 快捷指令真正发出的请求形状（?auth= 查询串等）
#   share-page.test.mjs    —— 分享页链路：POST /share 一律发 token + 指向 /#/s，GET /share 不消耗次数
#
# 端到端测试需要先打包处理器（Worker 源码用打包器风格的无后缀导入，Node 直接加载不了），
# 并用 node:sqlite 充当 D1、Map 充当 R2，因此不需要 wrangler、不联网。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "── 打包处理器（供端到端测试导入）"
for entry in handlers/content handlers/file handlers/text durable-objects/websocket-room auth share; do
  ./node_modules/.bin/esbuild "src/${entry}.js" \
    --bundle --format=esm --platform=neutral \
    --outfile="test/.build/$(basename "${entry}").mjs" --log-level=warning
done

echo
echo "── 上传与 Receive 链路（/upload multipart 与 /text → content/latest → 决策树）"
node --no-warnings test/receive-path.test.mjs

echo
echo "── 设备名链路（?name= → 落库 → content/latest 回读）"
node --no-warnings test/device-name.test.mjs

echo
echo "── 静态资源路由契约（[assets] 配置 + Worker 兜底）"
node --no-warnings test/asset-routing.test.mjs

echo
echo "── 历史消息文件名（不能被拼上图标）"
node --no-warnings test/history-name.test.mjs

echo
echo "── 房间鉴权（/file/ 的房间不可被客户端 ?room= 伪造；会话令牌密钥含房间密码）"
node --no-warnings test/auth-room.test.mjs

echo
echo "── Android 快捷指令的请求形状（?auth= 查询串 / latest.json 后缀 / 下载不带 room）"
node --no-warnings test/shortcut-contract.test.mjs

echo
echo "── 看板列（POST /content/:id/column：固定三列、落库、不动 timestamp）"
node --no-warnings test/board-column.test.mjs

echo
echo "── /text 的请求体形态（JSON / multipart / 纯文本；urlencoded 刻意不认）"
node --no-warnings test/text-body.test.mjs

echo
echo "── 分享页链路（一律发 token / 链接指向 /#/s / 元信息 / 密码闸门 / 看一眼不消耗次数）"
node --no-warnings test/share-page.test.mjs
