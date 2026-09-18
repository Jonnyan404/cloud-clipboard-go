#!/usr/bin/env bash
# 跑 Cloudflare Worker 的测试。
#
#   receive-path.test.mjs  —— 上传（/upload multipart 与 /text）→ /content/latest?json=1 → Receive 决策树
#   device-name.test.mjs   —— 设备名（?name=）落库与回读，含自愈加列路径
#   asset-routing.test.mjs —— 静态资源路由契约：/file 与 /content 不能被资源层回成 index.html
#
# 端到端测试需要先打包处理器（Worker 源码用打包器风格的无后缀导入，Node 直接加载不了），
# 并用 node:sqlite 充当 D1、Map 充当 R2，因此不需要 wrangler、不联网。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "── 打包处理器（供端到端测试导入）"
for entry in handlers/content handlers/file handlers/text durable-objects/websocket-room auth; do
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
