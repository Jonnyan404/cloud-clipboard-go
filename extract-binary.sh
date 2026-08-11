#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${1:-}"
OUTPUT_DIR="${2:-./dist-bin}"
CONTAINER_BINARY_PATH="/app/server-node/cloud-clipboard-go"

# 检查 Docker 命令
if ! command -v docker &> /dev/null; then
    echo "错误: 系统中未找到 docker 命令！" >&2
    exit 1
fi

# 如果未提供镜像名称参数，默认从源码构建最新的 Docker 镜像
if [ -z "${IMAGE_NAME}" ]; then
    IMAGE_NAME="cloud-clipboard-go:build-tmp"
    echo "==> 未指定镜像名称，正在从当前源码构建最新 Docker 镜像 (${IMAGE_NAME})..."
    DOCKER_BUILDKIT=1 docker build -f cloud-clip/Dockerfile -t "${IMAGE_NAME}" .
else
    echo "==> 使用指定的镜像: ${IMAGE_NAME}"
fi

echo "==> 输出目录: ${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

CONTAINER_ID=""

# 确保脚本退出时清理临时容器
cleanup() {
    if [ -n "${CONTAINER_ID:-}" ]; then
        docker rm -f "${CONTAINER_ID}" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

# 创建临时容器（不运行）
CONTAINER_ID=$(docker create "${IMAGE_NAME}")

# 从容器中拷贝编译好的二进制文件
echo "==> 正在提取二进制文件..."
docker cp "${CONTAINER_ID}:${CONTAINER_BINARY_PATH}" "${OUTPUT_DIR}/cloud-clipboard-go"

chmod +x "${OUTPUT_DIR}/cloud-clipboard-go"

echo "==> 成功提取二进制文件到: ${OUTPUT_DIR}/cloud-clipboard-go"
ls -lh "${OUTPUT_DIR}/cloud-clipboard-go"
