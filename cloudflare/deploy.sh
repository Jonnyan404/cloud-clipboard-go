#!/bin/bash
# filepath: cloudflare/deploy.sh

set -e  # 遇到错误立即停止
# 兼容 macOS 和 Linux 的 sed -i 参数
if [[ "$(uname -s)" == "Darwin" ]]; then
    SED_INPLACE=(-i '')
else
    SED_INPLACE=(-i)
fi

echo "=== 部署 Cloud Clipboard 到 Cloudflare ==="
echo ""
echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
echo "!!! 部署前请先检查并修改 workers/wrangler.toml.template !!!"
echo "!!! 特别是 AUTH_PASSWORD、ROOM_AUTH_JSON、ROOM_LIST 等变量    !!!"
echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
echo ""

read -rp "请确认已修改完毕并按下 Enter 键继续..."

# 颜色输出函数
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

version_lt() {
    [[ "$1" == "$2" ]] && return 1
    [[ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)" == "$1" ]]
}

should_run_local_d1() {
    if [[ "${SKIP_LOCAL_D1:-}" == "1" ]]; then
        return 1
    fi

    if [[ "$(uname -s)" != "Darwin" ]]; then
        return 0
    fi

    local macos_version
    macos_version=$(sw_vers -productVersion)

    if version_lt "$macos_version" "13.5.0"; then
        warn "检测到 macOS $macos_version，低于 workerd 本地运行要求的 13.5.0，跳过本地 D1 迁移。"
        warn "如需强制跳过本地 D1，可在执行前设置 SKIP_LOCAL_D1=1。"
        return 1
    fi

    return 0
}

# 检查必要工具
check_requirements() {
    info "检查必要工具..."
    
    if ! command -v npm &> /dev/null; then
        error "npm 未安装，请先安装 Node.js"
        exit 1
    fi
    
    if ! command -v wrangler &> /dev/null; then
        info "安装 Wrangler CLI..."
        npm install -g wrangler
    fi
    
    info "检查 Wrangler 登录状态..."
    if printf '%s' "$(wrangler whoami)" | grep -Eqi 'not|unauthenticated|please run `wrangler login`|not authenticated|not logged in'; then
        warn "请先登录 Wrangler: wrangler login"
        exit 1
    fi
}

# 步骤 1: 创建 D1 数据库并获取 ID
create_d1_database() {
    info "=== 步骤 1: 创建 D1 数据库 ==="
    
    cd d1 || exit 1
    
    # 尝试创建数据库
    info "创建 D1 数据库..."
    DB_OUTPUT=$(wrangler d1 create cloud-clipboard-db 2>&1 || echo "database may exist")
    
    # 提取数据库 ID
    if echo "$DB_OUTPUT" | grep -q "database_id"; then
        D1_DATABASE_ID=$(echo "$DB_OUTPUT" | grep "database_id" | cut -d'"' -f4)
        info "D1 数据库创建成功，ID: $D1_DATABASE_ID"
    else
        # 如果创建失败，尝试获取现有数据库 ID
        warn "数据库可能已存在，尝试获取现有数据库信息..."
        LIST_OUTPUT=$(wrangler d1 list 2>&1)
        if echo "$LIST_OUTPUT" | grep -q "cloud-clipboard-db"; then
            D1_DATABASE_ID=$(echo "$LIST_OUTPUT" | grep "cloud-clipboard-db" | awk '{print $2}' | head -1)
            info "找到现有数据库，ID: $D1_DATABASE_ID"
        else
            error "无法获取数据库 ID，请手动检查"
            exit 1
        fi
    fi
    
    
    cd ..
    
    # 保存 D1 ID 到临时文件
    echo "$D1_DATABASE_ID" > .d1_database_id
}

# 步骤 2: 创建 R2 存储桶
create_r2_bucket() {
    info "=== 步骤 2: 创建 R2 存储桶 ==="
    
    wrangler r2 bucket create cloud-clipboard-files || warn "存储桶可能已存在"
}

# 步骤 3: 更新 Worker 配置并部署
deploy_worker() {
    info "=== 步骤 3: 部署 Cloudflare Workers ==="
    
    cd workers || exit 1
    cp wrangler.toml.template wrangler.toml

    # 读取 D1 数据库 ID
    D1_DATABASE_ID=$(cat ../.d1_database_id)
    
    # 更新 wrangler.toml 中的数据库 ID
    info "更新 wrangler.toml 配置..."
    sed "${SED_INPLACE[@]}" "s/database_id = \"your-d1-database-id\"/database_id = \"$D1_DATABASE_ID\"/" wrangler.toml
    
    # 执行数据库迁移（远程生产数据库）
    info "执行远程数据库迁移..."
    wrangler d1 execute cloud-clipboard-db --file=../d1/schema.sql --remote
    
    # 可选：同时迁移本地数据库用于开发
    if should_run_local_d1; then
        info "执行本地数据库迁移..."
        wrangler d1 execute cloud-clipboard-db --file=../d1/schema.sql --local
    else
        info "已跳过本地数据库迁移，不影响远程部署。"
    fi

   

    # 安装依赖
    info "安装 Worker 依赖..."
    npm install
    
    # 部署 Worker
    info "部署 Worker..."
    DEPLOY_OUTPUT=$(wrangler deploy --env="" 2>&1)
    
    # 提取 Worker URL
    if echo "$DEPLOY_OUTPUT" | grep -q "workers.dev"; then
        WORKER_URL=$(echo "$DEPLOY_OUTPUT" | grep -o 'https://[^[:space:]]*\.workers\.dev' | head -1)
        # 移除可能的尾部斜杠
        WORKER_URL=${WORKER_URL%/}
        info "Worker 部署成功: $WORKER_URL"
    else
        error "无法获取 Worker URL，请检查部署输出"
        echo "$DEPLOY_OUTPUT"
        exit 1
    fi
    
    cd ..
    
    # 保存 Worker URL 到临时文件
    echo "$WORKER_URL" > .worker_url
}

# 步骤 4: 构建 Vue3 前端并拷贝为 Worker 静态资源
build_frontend_assets() {
    info "=== 步骤 4: 构建 Vue3 前端 (web-vue3) ==="

    if [ ! -d "../web-vue3" ]; then
        error "未找到 web-vue3 目录，请确保在仓库根目录的 cloudflare/ 下执行本脚本"
        exit 1
    fi

    cd ../web-vue3 || exit 1

    if [ ! -d "node_modules" ]; then
        info "安装前端依赖..."
        npm install
    fi

    info "构建前端..."
    npm run build

    info "拷贝构建产物到 Worker 静态资源目录..."
    local ASSETS_DIR="../cloudflare/workers/assets"
    rm -rf "$ASSETS_DIR"
    mkdir -p "$ASSETS_DIR"

    # 拷贝 dist 内容，排除 .gz/.br 压缩副本（Workers Static Assets 自动协商压缩）
    for item in dist/*; do
        local base
        base=$(basename "$item")
        case "$base" in
            *.gz|*.br) continue ;;
        esac
        cp -R "$item" "$ASSETS_DIR/"
    done

    info "前端静态资源已就绪，共 $(find "$ASSETS_DIR" -type f | wc -l | tr -d ' ') 个文件"

    cd ../cloudflare || exit 1
}

# 清理临时文件
cleanup() {
    info "清理临时文件..."
    rm -f .d1_database_id .worker_url .pages_url
    rm -f workers/wrangler.toml
}

# 显示部署结果
show_results() {
    info "=== 部署完成! ==="
    
    WORKER_URL=$(cat .worker_url 2>/dev/null || echo "请手动检查")
    
    echo ""
    echo "🎉 部署结果:"
    echo "  - 一体化 Worker (静态 + API): $WORKER_URL"
    echo ""
    echo "📝 后续步骤:"
    echo "  1. 访问 $WORKER_URL 测试功能"
    echo "  2. 如需自定义域名，请在 Cloudflare Dashboard 中配置"
    echo "  3. 可在 Worker 设置中配置环境变量（如认证密码）"
    echo ""
    echo "🔧 环境变量配置:"
    echo "  - 脚本执行的默认环境变量在 cloudflare/workers/wrangler.toml.template 中定义。可在此文件内更改后,重新执行脚本."
    echo "  - 也可部署完成后,去 workers 后台设置-变量-处更改"
    echo ""
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    echo "!!! 如果你在部署前没有修改 wrangler.toml.template        !!!"
    echo "!!! 部署完成后务必去 Cloudflare Workers 后台检查并修改变量 !!!"
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    echo "  - 已支持 /content/latest、/content/:id 及对应 .json 路由（无 /api 前缀，与自托管 Go 对齐）"
    echo "  - 已支持分块上传 /upload/chunk、/upload/chunk/:uuid、/upload/finish/:uuid"
    echo "  - 静态资源与 SPA 导航请求免费，仅 API 请求计费"
}

# 错误处理
trap cleanup EXIT

# 主执行流程
main() {
    check_requirements
    create_d1_database
    create_r2_bucket
    build_frontend_assets
    deploy_worker
    show_results
}

# 执行主函数
main "$@"