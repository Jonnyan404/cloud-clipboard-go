# syntax=docker/dockerfile:1

# 直接下载 GitHub Release 中已构建好的静态二进制（已内嵌前端静态资源），
# 不再在镜像内编译前端/后端，大幅加快镜像生成。
# 需要 build-arg: VERSION  (例如 v1.0.0)；
# TARGETARCH / TARGETVARIANT 由 docker buildx 自动注入。

FROM alpine:latest

ARG VERSION
ARG TARGETARCH
ARG TARGETVARIANT

# 运行时依赖
RUN apk add --no-cache ca-certificates netcat-openbsd git tzdata \
    && apk add --repository=https://dl-cdn.alpinelinux.org/alpine/edge/testing mkcert

WORKDIR /app/server-node

COPY cloud-clip/entrypoint.sh /app/entrypoint.sh

# 按平台映射 Release 归档名并解压出二进制
RUN chmod +x /app/entrypoint.sh \
 && case "${TARGETARCH}-${TARGETVARIANT}" in \
      amd64-*) B="Linux_x86_64";; \
      arm64-*) B="Linux_aarch64";; \
      arm-v7)  B="Linux_armv7";; \
      *) echo "不支持的平台: ${TARGETARCH}-${TARGETVARIANT}"; exit 1;; \
    esac \
 && wget -q -O - "https://github.com/jonnyan404/cloud-clipboard-go/releases/download/${VERSION}/cloud-clipboard-go_${B}.tar.gz" \
      | tar -xz -C /app/server-node/ cloud-clipboard-go \
 && chmod +x /app/server-node/cloud-clipboard-go

# 暴露端口
EXPOSE 9501

HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s \
    CMD nc -z 127.0.0.1 "${LISTEN_PORT:-9501}" || exit 1

# 运行应用
ENTRYPOINT ["/app/entrypoint.sh"]