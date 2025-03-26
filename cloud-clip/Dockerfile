# 前端构建阶段
FROM node:22.12-alpine3.21 AS frontend
ENV NODE_OPTIONS=--openssl-legacy-provider
COPY client /app
WORKDIR /app
RUN npm install
RUN npm run build

# 后端构建阶段
FROM golang:1.22-alpine AS builder

# 设置工作目录
WORKDIR /app
# 复制 Go 模块文件（确保这些文件在 cloud-clip 目录下）
COPY cloud-clip/go.mod cloud-clip/go.sum ./
RUN go mod download

# 复制源代码
COPY cloud-clip/*.go ./

# 创建静态文件目录
RUN mkdir -p ./static

# 复制前端构建结果
COPY --from=frontend /app/dist ./static/

# 编译应用，启用嵌入式静态文件
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.useEmbedded=true" -o cloud-clipboard-go .

# 最终镜像
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache ca-certificates

# 创建应用目录
WORKDIR /app/server-node

# 从构建阶段复制编译好的应用
COPY --from=builder /app/cloud-clipboard-go .
COPY cloud-clip/entrypoint.sh /app/
RUN apk add --no-cache git tzdata \
    && chmod +x /app/entrypoint.sh
# 暴露端口
EXPOSE 9502

# 运行应用
ENTRYPOINT ["/app/entrypoint.sh"]