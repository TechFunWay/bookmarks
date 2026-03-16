#!/bin/bash

# Go 多平台编译脚本
# 支持交叉编译到不同平台和架构

set -e

echo "========================================="
echo "  Go 多平台编译工具"
echo "========================================="

# 获取版本号
VERSION=$(awk -F'"' '/appVersion = "[^"]+"/ {print $2}' main.go)

if [ -z "$VERSION" ]; then
    echo "❌ 错误: 无法从 main.go 中获取版本号"
    exit 1
fi

# 去掉版本号中的 v 前缀（如果有）
VERSION=${VERSION#v}

echo "📦 版本号: $VERSION"
echo ""

# 定义平台列表（默认编译所有平台）
ALL_PLATFORMS=(
    "linux-amd64"
    "linux-arm64"
    "macos-amd64"
    "macos-arm64"
    "windows-amd64"
    "windows-arm64"
)

# 解析命令行参数
if [ $# -gt 0 ]; then
    # 验证参数
    for arg in "$@"; do
        if [[ ! " ${ALL_PLATFORMS[@]} " =~ " ${arg} " ]]; then
            echo "❌ 错误: 不支持的平台: $arg"
            echo ""
            echo "支持的平台:"
            for platform in "${ALL_PLATFORMS[@]}"; do
                echo "  - $platform"
            done
            exit 1
        fi
    done
    PLATFORMS=("$@")
else
    PLATFORMS=("${ALL_PLATFORMS[@]}")
fi

echo "🎯 将编译以下平台:"
for platform in "${PLATFORMS[@]}"; do
    echo "  ✓ $platform"
done
echo ""

# 成功计数
SUCCESS_COUNT=0
FAIL_COUNT=0

# 编译函数
compile_platform() {
    local PLATFORM=$1
    local GOOS=""
    local GOARCH=""
    local EXT=""
    local OUTPUT_NAME=""

    case $PLATFORM in
        "linux-amd64")
            GOOS=linux
            GOARCH=amd64
            EXT=""
            OUTPUT_NAME="bookmarks"
            ;;
        "linux-arm64")
            GOOS=linux
            GOARCH=arm64
            EXT=""
            OUTPUT_NAME="bookmarks"
            ;;
        "macos-amd64")
            GOOS=darwin
            GOARCH=amd64
            EXT=""
            OUTPUT_NAME="bookmarks"
            ;;
        "macos-arm64")
            GOOS=darwin
            GOARCH=arm64
            EXT=""
            OUTPUT_NAME="bookmarks"
            ;;
        "windows-amd64")
            GOOS=windows
            GOARCH=amd64
            EXT=".exe"
            OUTPUT_NAME="bookmarks.exe"
            ;;
        "windows-arm64")
            GOOS=windows
            GOARCH=arm64
            EXT=".exe"
            OUTPUT_NAME="bookmarks.exe"
            ;;
        *)
            echo "❌ 错误: 未知架构: $PLATFORM"
            return 1
            ;;
    esac

    echo "----------------------------------------"
    echo "🔨 编译: $PLATFORM"
    echo "   GOOS=$GOOS, GOARCH=$GOARCH"

    # 创建目标目录
    TARGET_DIR="release/bookmarks-v${VERSION}-${PLATFORM}"
    mkdir -p "$TARGET_DIR/static"

    # 编译可执行文件
    echo "   📝 执行编译..."
    if CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$TARGET_DIR/$OUTPUT_NAME" main.go; then
        echo "   ✅ 编译成功"
    else
        echo "   ❌ 编译失败"
        ((FAIL_COUNT++))
        return 1
    fi

    # 复制静态资源（不复制 icons 目录）
    if [ -d "static" ]; then
        echo "   📁 复制静态资源..."

        # 创建临时文件列表
        TMP_FILE=$(mktemp)

        # 找到所有需要复制的文件（排除 icons 目录）
        find static -type f -not -path "*/icons/*" -not -name ".*" > "$TMP_FILE"

        # 复制文件
        while IFS= read -r file; do
            if [ -f "$file" ]; then
                REL_PATH="${file#static/}"
                DEST_FILE="$TARGET_DIR/static/$REL_PATH"
                DEST_DIR=$(dirname "$DEST_FILE")
                mkdir -p "$DEST_DIR"
                cp "$file" "$DEST_FILE"
            fi
        done < "$TMP_FILE"

        # 清理临时文件
        rm -f "$TMP_FILE"
    fi

    # 复制 LICENSE 文件
    if [ -f "LICENSE" ]; then
        cp LICENSE "$TARGET_DIR/"
    fi

    # 复制 README 文件
    if [ -f "README.md" ]; then
        cp README.md "$TARGET_DIR/"
    fi

    # 复制 docker-compose.yaml
    if [ -f "docker-compose.yaml" ]; then
        cp docker-compose.yaml "$TARGET_DIR/"
    fi

    # 复制 Dockerfile
    if [ -f "Dockerfile" ]; then
        cp Dockerfile "$TARGET_DIR/"
    fi

    echo "   ✅ 资源复制完成"
    echo "   📂 输出目录: $TARGET_DIR"
    ((SUCCESS_COUNT++))
}

# 编译每个平台
for PLATFORM in "${PLATFORMS[@]}"; do
    compile_platform "$PLATFORM"
    echo ""
done

# 汇总结果
echo "========================================="
echo "  编译完成"
echo "========================================="
echo "✅ 成功: $SUCCESS_COUNT 个平台"
if [ $FAIL_COUNT -gt 0 ]; then
    echo "❌ 失败: $FAIL_COUNT 个平台"
fi
echo ""

# 显示生成的目录
echo "📦 生成的目录:"
ls -lh release/bookmarks-v${VERSION}-*/ 2>/dev/null || echo "  (无输出)"

echo ""
echo "💡 使用提示:"
echo "  1. 可执行文件位于: release/bookmarks-v${VERSION}-<PLATFORM>/"
echo "  2. 直接运行: ./release/bookmarks-v${VERSION}-<PLATFORM>/bookmarks"
echo "  3. 使用 docker-builder skill 构建 Docker 镜像"
echo "  4. 使用 fnapp-packager skill 打包飞牛应用"
echo ""

if [ $FAIL_COUNT -gt 0 ]; then
    exit 1
fi
