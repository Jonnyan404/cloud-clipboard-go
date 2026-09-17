#!/usr/bin/env bash
# 跑 Cloudflare Worker 的测试。
#
#   1) sniff.test.mjs        —— 纯逻辑单测，用例与 Go 版 cloud-clip/lib/unicode_text_test.go 逐条对齐
#   2) upload-e2e.test.mjs   —— /upload/raw 与 /upload/base64 的端到端胶水测试
#
# 端到端测试需要先打包处理器（Worker 源码用打包器风格的无后缀导入，Node 直接加载不了），
# 并用 node:sqlite 充当 D1、Map 充当 R2，因此不需要 wrangler、不联网。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "── 打包处理器（供端到端测试导入）"
./node_modules/.bin/esbuild src/handlers/raw-upload.js \
  --bundle --format=esm --platform=neutral \
  --outfile=test/.build/raw-upload.mjs --log-level=warning

echo
echo "── 1/2 嗅探与文本规范化（对照 Go 版 unicode_text_test.go）"
node --no-warnings test/sniff.test.mjs

echo
echo "── 2/2 /upload/raw 与 /upload/base64 端到端"
node --no-warnings test/upload-e2e.test.mjs
