#!/bin/bash

# 飞牛应用打包脚本
# 用于为 techfunway.bookmarks 应用创建适用于不同平台的 FnApp 安装包

set -e

echo "=== 飞牛应用打包工具 ==="

# 获取版本号
VERSION=$(awk -F'"' '/appVersion = "[^"]+"/ {print $2}' main.go)

if [ -z "$VERSION" ]; then
    echo "错误: 无法从 main.go 中获取版本号"
    exit 1
fi

# 去掉版本号中的 v 前缀（如果有）
VERSION=${VERSION#v}

echo "使用版本号: $VERSION"

# 定义平台列表
PLATFORMS=("arm64" "amd64")

# 非交互模式标志
NON_INTERACTIVE=false

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --non-interactive|--auto)
            NON_INTERACTIVE=true
            shift
            ;;
        amd64|arm64)
            if [[ "${PLATFORMS[@]}" =~ "$1" ]]; then
                PLATFORMS=($1)
                echo "只打包平台: $1"
            else
                echo "错误: 不支持的平台: $1"
                echo "支持的平台: ${PLATFORMS[@]}"
                exit 1
            fi
            shift
            ;;
        *)
            echo "错误: 未知参数: $1"
            echo "用法: $0 [--non-interactive|--auto] [amd64|arm64]"
            exit 1
            ;;
    esac
done

if [ "$NON_INTERACTIVE" = true ]; then
    echo "使用非交互模式，将自动使用默认内容进行打包"
fi

# ---- 打包浏览器插件 ----
echo "\n打包浏览器插件..."
mkdir -p static/downloads
rm -f static/downloads/edge-extension.zip
cd edge-extension
zip -r ../static/downloads/edge-extension.zip \
    manifest.json \
    background.js \
    popup.html \
    popup.js \
    options.html \
    options.js \
    sync-window.html \
    sync-window.js \
    icons/
cd ..
if [ $? -ne 0 ]; then
    echo "错误: 插件打包失败"
    exit 1
fi
echo "插件打包完成: static/downloads/edge-extension.zip"
# ---- end 打包浏览器插件 ----

# 主打包函数
package_platform() {
    local ARCH=$1
    local PLATFORM_DIR=""
    local MANIFEST_PLATFORM=""
    local OUTPUT_ARCH=""

    case $ARCH in
        "amd64")
            PLATFORM_DIR="release/v${VERSION}/bookmarks-v${VERSION}-linux-amd64"
            MANIFEST_PLATFORM="x86"
            OUTPUT_ARCH="amd64"
            ;;
        "arm64")
            PLATFORM_DIR="release/v${VERSION}/bookmarks-v${VERSION}-linux-arm64"
            MANIFEST_PLATFORM="arm"
            OUTPUT_ARCH="arm64"
            ;;
        *)
            echo "错误: 未知架构: $ARCH"
            return 1
            ;;
    esac

    echo "\n=== 开始打包 $ARCH 平台 ==="

    # 检查平台目录是否存在
    if [ ! -d "$PLATFORM_DIR" ]; then
        echo "错误: 平台目录不存在: $PLATFORM_DIR"
        echo "请先执行编译脚本生成对应平台的可执行文件"
        return 1
    fi

    # 检查可执行文件是否存在
    if [ ! -f "$PLATFORM_DIR/bookmarks" ]; then
        echo "错误: 可执行文件不存在: $PLATFORM_DIR/bookmarks"
        return 1
    fi

    # 目标目录
    TARGET_DIR="techfunway.bookmarks/app/server"

    # 创建目标目录
    echo "创建目标目录..."
    mkdir -p "$TARGET_DIR"

    # 清理目标目录中的系统文件
    echo "清理目标目录中的系统文件..."
    find "$TARGET_DIR" -type f \( -name ".DS_Store" -o -name "._*" -o -name "Thumbs.db" \) -delete 2>/dev/null || true

    # 复制可执行文件
    echo "复制可执行文件..."
    cp "$PLATFORM_DIR/bookmarks" "$TARGET_DIR/"
    chmod +x "$TARGET_DIR/bookmarks"

    # 复制 reset-password
    if [ -f "$PLATFORM_DIR/reset-password" ]; then
        echo "复制 reset-password..."
        cp "$PLATFORM_DIR/reset-password" "$TARGET_DIR/"
        chmod +x "$TARGET_DIR/reset-password"
    fi

    # 复制 edge-extension.zip
    echo "复制 edge-extension.zip..."
    cp "static/downloads/edge-extension.zip" "$TARGET_DIR/"

    # 修改 manifest 文件
    echo "修改 manifest 文件..."
    MANIFEST_FILE="techfunway.bookmarks/manifest"

    if [ ! -f "$MANIFEST_FILE" ]; then
        echo "错误: manifest 文件不存在: $MANIFEST_FILE"
        return 1
    fi

    # 备份 manifest 文件
    cp "$MANIFEST_FILE" "$MANIFEST_FILE.bak"

    # 修改版本号
    sed -i '' "s/^version               = .*/version               = $VERSION/" "$MANIFEST_FILE"

    # 修改 platform 字段
    sed -i '' "s/^platform              = .*/platform              = $MANIFEST_PLATFORM/" "$MANIFEST_FILE"

    echo "manifest 文件已更新：版本=$VERSION，平台=$MANIFEST_PLATFORM"

    # 清理 techfunway.bookmarks 目录中的系统文件
    echo "清理 techfunway.bookmarks 目录中的系统文件..."
    find "techfunway.bookmarks" -type f \( -name ".DS_Store" -o -name "._*" -o -name "Thumbs.db" \) -delete 2>/dev/null || true

    # 清理旧的 fpk 文件
    find "techfunway.bookmarks" -name "*.fpk" -delete 2>/dev/null || true

    # 进入 techfunway.bookmarks 目录执行打包
    echo "\n=== 执行应用打包 ==="
    cd "techfunway.bookmarks"

    # 执行 fnpack build 命令
    if command -v fnpack &> /dev/null; then
        fnpack build
    else
        echo "错误: fnpack 命令未找到，请确保已安装飞牛应用打包工具"
        # 恢复 manifest 备份
        cd ..
        mv "$MANIFEST_FILE.bak" "$MANIFEST_FILE"
        return 1
    fi

    cd ..

    # 检查打包结果
    if [ ! -f "techfunway.bookmarks/techfunway.bookmarks.fpk" ]; then
        echo "错误: 打包失败，未生成 fpk 文件"
        # 恢复 manifest 备份
        mv "$MANIFEST_FILE.bak" "$MANIFEST_FILE"
        return 1
    fi

    # 确保 release 目录存在
    mkdir -p "release/v${VERSION}"

    # 重命名打包文件并移动到 release 目录
    OUTPUT_FILE="techfunway.bookmarks-v${VERSION}-${OUTPUT_ARCH}.fpk"
    RELEASE_OUTPUT="release/v${VERSION}/$OUTPUT_FILE"
    echo "重命名打包文件为: $OUTPUT_FILE"
    echo "移动到 release 目录"
    mv "techfunway.bookmarks/techfunway.bookmarks.fpk" "$RELEASE_OUTPUT"

    # 恢复 manifest 备份
    echo "恢复 manifest 文件..."
    mv "$MANIFEST_FILE.bak" "$MANIFEST_FILE"

    echo "平台 $ARCH 打包完成！"
    echo "打包文件位置: $(pwd)/$RELEASE_OUTPUT"
}

# 执行打包
for ARCH in "${PLATFORMS[@]}"; do
    package_platform "$ARCH"
    echo ""
done

echo "=== 打包完成 ==="
echo "生成的文件:"
ls -la release/v${VERSION}/techfunway.bookmarks-v${VERSION}-*.fpk || true

echo "\n使用说明:"
echo "1. 打包文件已生成在 release/v${VERSION} 目录"
echo "2. 可以直接将这些文件上传到飞牛应用商店"
echo "3. 或者分发给用户手动安装"
