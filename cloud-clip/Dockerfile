# 前端构建阶段
FROM node:22.12-alpine3.21 AS frontend
ENV NODE_OPTIONS=--openssl-legacy-provider
WORKDIR /app

# 优先复制 package*.json 利用层缓存
COPY client/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci || npm install

# 复制前端源码并构建
COPY client/ ./
RUN chmod +x node_modules/.bin/* 2>/dev/null || true
RUN npm run build

# 后端构建阶段
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 优先复制 Go 模块依赖文件利用层缓存
COPY cloud-clip/go.mod cloud-clip/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制 Go 源码
COPY cloud-clip/ ./

# 创建静态文件目录
RUN mkdir -p lib/static

# 复制前端构建结果
COPY --from=frontend /app/dist lib/static/

# 编译应用，启用 Go 编译缓存与 Go module 缓存
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -tags=embed -ldflags="-s -w" -o cloud-clipboard-go .

# 最终镜像
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache ca-certificates netcat-openbsd git tzdata \
    && apk add --repository=https://dl-cdn.alpinelinux.org/alpine/edge/testing mkcert

# 创建应用目录
WORKDIR /app/server-node

# 从构建阶段复制编译好的应用与启动脚本
COPY --from=builder /app/cloud-clipboard-go .
COPY cloud-clip/entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh

# 暴露端口
EXPOSE 9501

HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s \
    CMD nc -z 127.0.0.1 "${LISTEN_PORT:-9501}" || exit 1

# 运行应用
ENTRYPOINT ["/app/entrypoint.sh"]