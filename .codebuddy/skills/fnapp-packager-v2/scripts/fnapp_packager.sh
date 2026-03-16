#!/bin/bash

# 飞牛应用打包脚本 v2
# 用于创建 FnApp 安装包

set -e

echo "========================================="
echo "  飞牛应用打包工具 v2"
echo "========================================="
echo ""

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

# 定义平台列表
ALL_PLATFORMS=("arm64" "amd64")

# 解析命令行参数
NON_INTERACTIVE=false
PLATFORMS=()

while [[ $# -gt 0 ]]; do
    case $1 in
        --non-interactive|--auto)
            NON_INTERACTIVE=true
            shift
            ;;
        amd64|arm64)
            if [[ " ${ALL_PLATFORMS[@]} " =~ " $1 " ]]; then
                PLATFORMS+=("$1")
            else
                echo "❌ 错误: 不支持的平台: $1"
                echo ""
                echo "支持的平台: ${ALL_PLATFORMS[@]}"
                exit 1
            fi
            shift
            ;;
        *)
            echo "❌ 错误: 未知参数: $1"
            echo ""
            echo "用法: $0 [--non-interactive|--auto] [amd64|arm64]"
            echo ""
            echo "示例:"
            echo "  $0              # 打包所有平台"
            echo "  $0 amd64        # 只打包 amd64"
            echo "  $0 arm64        # 只打包 arm64"
            echo "  $0 amd64 arm64  # 打包指定平台"
            echo "  $0 --auto        # 非交互模式"
            exit 1
            ;;
    esac
done

# 如果没有指定平台，默认打包所有平台
if [ ${#PLATFORMS[@]} -eq 0 ]; then
    PLATFORMS=("${ALL_PLATFORMS[@]}")
    echo "🎯 将打包所有平台: ${PLATFORMS[@]}"
else
    echo "🎯 将打包以下平台: ${PLATFORMS[@]}"
fi

if [ "$NON_INTERACTIVE" = true ]; then
    echo "🤖 非交互模式: 将使用默认配置"
fi

echo ""

# 统计
SUCCESS_COUNT=0
FAIL_COUNT=0

# 打包函数
package_platform() {
    local ARCH=$1
    local PLATFORM_DIR=""
    local MANIFEST_PLATFORM=""
    local OUTPUT_ARCH=""

    case $ARCH in
        "amd64")
            PLATFORM_DIR="release/bookmarks-v${VERSION}-linux-amd64"
            MANIFEST_PLATFORM="x86"
            OUTPUT_ARCH="amd64"
            ;;
        "arm64")
            PLATFORM_DIR="release/bookmarks-v${VERSION}-linux-arm64"
            MANIFEST_PLATFORM="arm"
            OUTPUT_ARCH="arm64"
            ;;
        *)
            echo "❌ 错误: 未知架构: $ARCH"
            ((FAIL_COUNT++))
            return 1
            ;;
    esac

    echo "----------------------------------------"
    echo "📦 开始打包: $ARCH 平台"
    echo ""

    # 检查平台目录是否存在
    if [ ! -d "$PLATFORM_DIR" ]; then
        echo "❌ 错误: 平台目录不存在: $PLATFORM_DIR"
        echo "   请先执行编译脚本生成对应平台的可执行文件"
        echo "   命令: .trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-${ARCH}"
        ((FAIL_COUNT++))
        return 1
    fi

    # 检查可执行文件是否存在
    if [ ! -f "$PLATFORM_DIR/bookmarks" ]; then
        echo "❌ 错误: 可执行文件不存在: $PLATFORM_DIR/bookmarks"
        ((FAIL_COUNT++))
        return 1
    fi

    # 目标目录
    TARGET_DIR="techfunway.bookmarks/app/server"

    # 创建目标目录
    echo "📁 创建目标目录..."
    mkdir -p "$TARGET_DIR"

    # 清理目标目录中的系统文件
    echo "🧹 清理目标目录中的系统文件..."
    find "$TARGET_DIR" -type f \( -name ".DS_Store" -o -name "._*" -o -name "Thumbs.db" \) -delete 2>/dev/null || true

    # 复制可执行文件
    echo "📝 复制可执行文件..."
    cp "$PLATFORM_DIR/bookmarks" "$TARGET_DIR/"
    chmod +x "$TARGET_DIR/bookmarks"

    # 复制密码重置工具（如果存在）
    if [ -f "$PLATFORM_DIR/reset-password" ]; then
        echo "📝 复制密码重置工具..."
        cp "$PLATFORM_DIR/reset-password" "$TARGET_DIR/"
        chmod +x "$TARGET_DIR/reset-password"
        echo "   ✅ 密码重置工具已包含"
    else
        echo "⚠️  密码重置工具不存在（可选）"
    fi

    # 检查 manifest 文件
    MANIFEST_FILE="techfunway.bookmarks/manifest"

    if [ ! -f "$MANIFEST_FILE" ]; then
        echo "❌ 错误: manifest 文件不存在: $MANIFEST_FILE"
        ((FAIL_COUNT++))
        return 1
    fi

    # 备份 manifest 文件
    cp "$MANIFEST_FILE" "$MANIFEST_FILE.bak"

    # 修改版本号
    echo "⚙️  修改 manifest 文件..."
    sed -i '' "s/^version               = .*/version               = $VERSION/" "$MANIFEST_FILE"

    # 修改 platform 字段
    sed -i '' "s/^platform              = .*/platform              = $MANIFEST_PLATFORM/" "$MANIFEST_FILE"

    echo "   ✅ 版本号: $VERSION"
    echo "   ✅ 平台: $MANIFEST_PLATFORM"

    # 清理 techfunway.bookmarks 目录中的系统文件
    echo "🧹 清理应用目录中的系统文件..."
    find "techfunway.bookmarks" -type f \( -name ".DS_Store" -o -name "._*" -o -name "Thumbs.db" -o -name "desktop.ini" \) -delete 2>/dev/null || true

    # 创建 .fnpackignore 文件（如果不存在）
    if [ ! -f "techfunway.bookmarks/.fnpackignore" ]; then
        echo "📝 创建 .fnpackignore 文件..."
        cat > "techfunway.bookmarks/.fnpackignore" << 'EOF'
# Git 文件
.git/
.gitignore
.gitattributes

# IDE 文件
.vscode/
.idea/
*.swp
*.swo

# 系统文件
.DS_Store
._*
Thumbs.db
desktop.ini

# 临时文件
*.tmp
*.log
*.bak

# 开发文件
test/
tests/
*.test
EOF
    fi

    # 进入 techfunway.bookmarks 目录执行打包
    echo ""
    echo "📦 执行应用打包..."
    cd "techfunway.bookmarks"

    # 执行 fnpack build 命令
    if command -v fnpack &> /dev/null; then
        fnpack build
    else
        echo "❌ 错误: fnpack 命令未找到"
        echo "   请确保已安装飞牛应用打包工具"
        cd ..
        ((FAIL_COUNT++))
        return 1
    fi

    cd ..

    # 检查打包结果
    if [ ! -f "techfunway.bookmarks/techfunway.bookmarks.fpk" ]; then
        echo "❌ 错误: 打包失败，未生成 fpk 文件"
        ((FAIL_COUNT++))
        return 1
    fi

    # 确保 release 目录存在
    mkdir -p "release"

    # 重命名打包文件并移动到 release 目录
    OUTPUT_FILE="techfunway.bookmarks-v${VERSION}-${OUTPUT_ARCH}.fpk"
    RELEASE_OUTPUT="release/$OUTPUT_FILE"

    echo "📝 重命名打包文件: $OUTPUT_FILE"
    mv "techfunway.bookmarks/techfunway.bookmarks.fpk" "$RELEASE_OUTPUT"

    # 显示文件信息
    FILE_SIZE=$(du -h "$RELEASE_OUTPUT" | cut -f1)
    echo "   ✅ 文件大小: $FILE_SIZE"
    echo "   ✅ 文件位置: $(pwd)/$RELEASE_OUTPUT"

    ((SUCCESS_COUNT++))
    echo ""
}

# 执行打包
for ARCH in "${PLATFORMS[@]}"; do
    package_platform "$ARCH"
done

# 汇总结果
echo "========================================="
echo "  打包完成"
echo "========================================="
echo "✅ 成功: $SUCCESS_COUNT 个平台"
if [ $FAIL_COUNT -gt 0 ]; then
    echo "❌ 失败: $FAIL_COUNT 个平台"
fi
echo ""

# 显示生成的文件
echo "📦 生成的文件:"
ls -lh release/techfunway.bookmarks-v${VERSION}-*.fpk 2>/dev/null || echo "  (无输出)"

echo ""
echo "💡 使用说明:"
echo "  1. 打包文件已生成在 release 目录"
echo "  2. 可以直接上传到飞牛应用商店"
echo "  3. 或分发给用户手动安装"
echo ""

if [ $FAIL_COUNT -gt 0 ]; then
    exit 1
fi
