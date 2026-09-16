#!/usr/bin/env bash
# Apple 快捷指令构建脚本
#
# 为什么需要它：cherri 是裸二进制、来源不可追溯；patcher 必须紧跟编译；签名前要重启
# Shortcuts。这三步的顺序和参数此前只存在于人的记忆里，产物因此"存在但不可重建"。
#
# 用法：
#   ./build.sh                      # 构建全部 .cherri 源码并签名
#   ./build.sh Send                 # 只构建名字含 Send 的源码
#   ./build.sh --no-sign            # 只编译+注入问答，不签名（CI / 无界面环境）
#   ./build.sh --verify-only        # 只跑校验，不重新构建
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$HERE/source"
OUT="$HERE"

# cherri 的"版本身份"。它当前不是 brew 安装、也不是 release 二进制，而是从源码构建的裸二进制，
# 所以必须钉 commit，否则产物不可复现。`cherri --version` 打印的 v2.3.0 只是版本常量，
# 实际代码来自下面的 commit（见 go version -m 的伪版本号）。
CHERRI_VERSION="v2.3.0"
CHERRI_COMMIT="dc82114f346f"
CHERRI_REPO="github.com/electrikmilk/cherri"

DO_SIGN=1
VERIFY_ONLY=0
FILTER=""

for arg in "$@"; do
    case "$arg" in
        --no-sign)     DO_SIGN=0 ;;
        --verify-only) VERIFY_ONLY=1 ;;
        -h|--help)     sed -n '2,12p' "${BASH_SOURCE[0]}"; exit 0 ;;
        *)             FILTER="$arg" ;;
    esac
done

# 本机 /usr/local/bin 不在沙箱 shell 的默认 PATH 里，显式补上，否则会误报"未安装"。
export PATH="/usr/local/bin:$PATH"

PY="$(command -v python3)"
GO="$(command -v go || true)"

VERIFY_FAILED=0

check_cherri() {
    if ! command -v cherri >/dev/null 2>&1; then
        cat >&2 <<EOF
错误：未找到 cherri。

请二选一：
  1) 按 commit 从源码构建（推荐，与已验证的产物一致）：
       $GO install ${CHERRI_REPO}@${CHERRI_COMMIT}
     或
       git clone https://github.com/electrikmilk/cherri /tmp/cherri-src \\
         && cd /tmp/cherri-src && git checkout ${CHERRI_COMMIT} && go build -o /usr/local/bin/cherri .
  2) 下载 release 预编译二进制（${CHERRI_VERSION}，可能略旧于上述 commit）：
       https://github.com/electrikmilk/cherri/releases/download/${CHERRI_VERSION}/cherri_darwin-\$(uname -m).zip

注意：cherri 是 Go 编写的，不是 Swift。网上流传的"swift build"说法是错的。
EOF
        exit 1
    fi
    echo "cherri: $(cherri --version 2>&1 | head -1)"
    if [ -n "$GO" ]; then
        local mod
        mod="$(go version -m "$(command -v cherri)" 2>/dev/null | awk '$1=="mod"{print $3}' | head -1)"
        if [ -n "$mod" ]; then
            echo "       构建版本: $mod"
            if ! printf '%s' "$mod" | grep -q "$CHERRI_COMMIT"; then
                echo "       警告：与记录在案的 commit $CHERRI_COMMIT 不一致，产物可能与历史版本有差异。" >&2
            fi
        fi
    fi
}

build_one() {
    local src="$1"
    local name
    # cherri 的输出文件名由源码里的 #define name 决定，-o 参数不生效，且会静默覆盖同名文件。
    name="$(awk '/^#define name /{print $3; exit}' "$src")"
    if [ -z "$name" ]; then
        echo "跳过 $src：没有 #define name" >&2
        return 0
    fi

    local unsigned="$SRC/${name}_unsigned.shortcut"
    local signed="$SRC/${name}.shortcut"

    echo "── 构建 $name"
    ( cd "$SRC" && cherri "$(basename "$src")" --skip-sign )

    if [ ! -f "$unsigned" ]; then
        echo "错误：cherri 未产出 $unsigned" >&2
        return 1
    fi

    # patcher 是长期必需的，不是临时补丁：cherri 的 shortcutgen.go:1194 读取 q.actionIndex，
    # 但全代码没有任何赋值点，恒为 0 并被 omitempty 省略 → 导入面板接不上。
    "$PY" "$SRC/inject_import_questions.py" "$src" "$unsigned"

    if [ "$DO_SIGN" = "1" ]; then
        # 签名前必须重启 Shortcuts，否则 sign 会拿到过期的动作定义。
        # 注意：这一步会扰动用户已安装的捷径（观察到被重命名、动作数 +1），属已知副作用。
        pkill -x Shortcuts 2>/dev/null || true
        sleep 2
        open -a Shortcuts
        sleep 6
        shortcuts sign -i "$unsigned" -o "$signed" --mode anyone
        # sign 会打印若干 ObjC "Unrecognized attribute string flag" 噪音，属正常现象。
        cp "$signed" "$OUT/"
        echo "   已签名并放置: $OUT/$(basename "$signed")"
    fi

    # 校验失败不立即中止——否则一个不合规的源码会挡住后面所有源码的构建。
    # 改为累积失败，全部跑完后再以非零码退出。
    if ! "$PY" "$HERE/verify.py" "$src" "$unsigned"; then
        VERIFY_FAILED=1
    fi
}

if [ "$VERIFY_ONLY" = "1" ]; then
    for src in "$SRC"/*.cherri; do
        [ -n "$FILTER" ] && case "$src" in *"$FILTER"*) ;; *) continue ;; esac
        name="$(awk '/^#define name /{print $3; exit}' "$src")"
        unsigned="$SRC/${name}_unsigned.shortcut"
        if [ -f "$unsigned" ]; then
            "$PY" "$HERE/verify.py" "$src" "$unsigned" || VERIFY_FAILED=1
        fi
    done
    [ "${VERIFY_FAILED:-0}" = "0" ] || { echo "有产物未通过校验" >&2; exit 1; }
    exit 0
fi

check_cherri

found=0
for src in "$SRC"/*.cherri; do
    [ -e "$src" ] || continue
    if [ -n "$FILTER" ]; then
        case "$src" in *"$FILTER"*) ;; *) continue ;; esac
    fi
    found=1
    build_one "$src"
done

[ "$found" = "1" ] || { echo "没有匹配 '$FILTER' 的源码" >&2; exit 1; }

if [ "${VERIFY_FAILED:-0}" != "0" ]; then
    echo "构建完成，但有源码未通过校验（见上方 ✗ 项）。" >&2
    exit 1
fi
echo "完成。"
