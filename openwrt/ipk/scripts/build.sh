#!/bin/bash
# 构建cloud-clipboard二进制文件

set -e

# 确定版本号
if [ -n "$1" ]; then
    VERSION="$1"
else
    VERSION=$(grep "server_version" cloud-clip/main.go | head -1 | cut -d '"' -f 2)
fi

# 确定输出目录
OUTPUT_DIR="openwrt/ipk/build"
mkdir -p "$OUTPUT_DIR"

echo "=== 构建 Cloud Clipboard 版本: $VERSION ==="

# 构建前端文件
echo "检查前端代码..."
if [ -d "client" ]; then
    echo "找到前端代码目录，准备构建..."
    
    # 检查Node.js是否可用
    if command -v node &> /dev/null && command -v npm &> /dev/null; then
        echo "Node.js已安装，开始构建前端代码..."
        
        # 构建前端
        cd client
        npm install
        npm run build
        
        # 创建static目录并复制前端文件
        mkdir -p "../cloud-clip/static"
        cp -r dist/* "../cloud-clip/static/"
        
        echo "✓ 前端文件构建完成并复制到 cloud-clip/static"
        
        # 返回项目根目录
        cd ..
    else
        echo "! 警告: 未找到Node.js，跳过前端构建"
    fi
else
    echo "! 未找到前端代码目录，跳过前端构建"
fi

# 进入Go项目目录
cd cloud-clip

# 检查go.mod文件是否存在，如果不存在则初始化
if [ ! -f "go.mod" ]; then
    echo "未找到go.mod文件，正在初始化Go模块..."
    go mod init github.com/jonnyan404/cloud-clipboard-go/cloud-clip
    go mod tidy
    echo "Go模块初始化完成"
fi

# 编译函数
build() {
    local GOOS=$1
    local GOARCH=$2
    local ARM=$3
    local MIPS=$4
    local ARCH_NAME=$5
    local OUTPUT="../$OUTPUT_DIR/cloud-clipboard-$VERSION-$ARCH_NAME"  # 注意添加了../
    
    echo "构建 $ARCH_NAME 架构..."
    
    BUILD_CMD="GOOS=$GOOS GOARCH=$GOARCH"
    [ -n "$ARM" ] && BUILD_CMD="$BUILD_CMD GOARM=$ARM"
    [ -n "$MIPS" ] && BUILD_CMD="$BUILD_CMD GOMIPS=$MIPS"
    
    eval "$BUILD_CMD go build -ldflags=\"-s -w -X main.server_version=$VERSION\" -o \"$OUTPUT\" ."
    
    echo "✓ 完成: $OUTPUT"
}

# 构建各种架构
build linux arm64 "" "" "aarch64"
# build linux amd64 "" "" "amd64"
# build linux 386 "" "" "i386"
# build linux arm "7" "" "arm-7"
# build linux mips "" "softfloat" "mips"
# build linux mipsle "" "softfloat" "mipsel"

# 返回原目录
cd ..

echo "=== 所有架构构建完成，二进制文件位于: $OUTPUT_DIR ==="