#!/bin/bash

# 获取版本号
VERSION=$(grep 'appVersion = "' main.go | awk -F'"' '{print $2}')

if [ -z "$VERSION" ]; then
    echo "Error: 无法从 main.go 中获取版本号"
    exit 1
fi

echo "使用版本号: $VERSION"

# ---- 打包浏览器插件 ----
echo "\n打包浏览器插件..."
mkdir -p static/downloads
# 删除旧的压缩包（如果存在）
rm -f static/downloads/edge-extension.zip
# 进入edge-extension目录进行打包，避免路径层级问题
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
    echo "Error: 插件打包失败"
    exit 1
fi
echo "插件打包完成: static/downloads/edge-extension.zip"
# ---- end 打包浏览器插件 ----

# 创建 release 目录
mkdir -p release

# 定义平台列表
PLATFORMS=("linux-amd64" "linux-arm64" "macos-amd64" "macos-arm64" "windows-amd64" "windows-arm64")

# 如果指定了平台参数，只编译指定平台
if [ $# -gt 0 ]; then
    PLATFORMS=($@)
fi

# 编译每个平台
for PLATFORM in "${PLATFORMS[@]}"; do
    echo "\n编译平台: $PLATFORM"
    
    # 解析平台和架构
    case $PLATFORM in
        "linux-amd64")
            GOOS=linux
            GOARCH=amd64
            EXT=""
            ;;
        "linux-arm64")
            GOOS=linux
            GOARCH=arm64
            EXT=""
            ;;
        "macos-amd64")
            GOOS=darwin
            GOARCH=amd64
            EXT=""
            ;;
        "macos-arm64")
            GOOS=darwin
            GOARCH=arm64
            EXT=""
            ;;
        "windows-amd64")
            GOOS=windows
            GOARCH=amd64
            EXT=".exe"
            ;;
        "windows-arm64")
            GOOS=windows
            GOARCH=arm64
            EXT=".exe"
            ;;
        *)
            echo "Error: 不支持的平台: $PLATFORM"
            continue
            ;;
    esac
    
    # 创建目标目录
    TARGET_DIR="release/bookmarks-${VERSION}-${PLATFORM}"
    mkdir -p "$TARGET_DIR"
    
    # 编译可执行文件
    echo "执行编译..."
    # 对于Linux平台，使用CGO_ENABLED=0进行静态编译
    if [[ "$GOOS" == "linux" ]]; then
        CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$TARGET_DIR/bookmarks$EXT" main.go
    else
        CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$TARGET_DIR/bookmarks$EXT" main.go
    fi
    
    if [ $? -ne 0 ]; then
        echo "Error: 编译失败"
        continue
    fi
    
    # 创建压缩包
    echo "创建压缩包..."
    cd "release"
    tar -czf "bookmarks-${VERSION}-${PLATFORM}.tar.gz" "bookmarks-${VERSION}-${PLATFORM}"
    cd ..
    
    echo "平台 $PLATFORM 编译完成"
done

echo "\n所有编译任务完成！"
echo "编译结果位于: release/ 目录"
