#!/usr/bin/env bash
# 跑 Cloudflare Worker 的测试。
#
#   1) sniff.test.mjs         —— 纯逻辑单测，用例与 Go 版 cloud-clip/lib/unicode_text_test.go 逐条对齐
#   2) upload-e2e.test.mjs    —— /upload/raw 的端到端胶水测试 + /upload（multipart）文件名保真
#   3) receive-path.test.mjs  —— Receive 依赖的链路：上传 → /content/latest?json=1 → 决策树
#
# 端到端测试需要先打包处理器（Worker 源码用打包器风格的无后缀导入，Node 直接加载不了），
# 并用 node:sqlite 充当 D1、Map 充当 R2，因此不需要 wrangler、不联网。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "── 打包处理器（供端到端测试导入）"
for entry in raw-upload content file; do
  ./node_modules/.bin/esbuild "src/handlers/${entry}.js" \
    --bundle --format=esm --platform=neutral \
    --outfile="test/.build/${entry}.mjs" --log-level=warning
done

echo
echo "── 1/3 嗅探与文本规范化（对照 Go 版 unicode_text_test.go）"
node --no-warnings test/sniff.test.mjs

echo
echo "── 2/3 /upload/raw 端到端 + /upload multipart 文件名保真"
node --no-warnings test/upload-e2e.test.mjs

echo
echo "── 3/3 Receive 链路（上传 → content/latest → 决策树）"
node --no-warnings test/receive-path.test.mjs
