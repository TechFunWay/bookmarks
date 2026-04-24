#!/bin/bash

# 获取版本号
VERSION=$(grep 'appVersion = "' main.go | awk -F'"' '{print $2}')

if [ -z "$VERSION" ]; then
    echo "Error: 无法从 main.go 中获取版本号"
    exit 1
fi

echo "使用版本号: $VERSION"

AMD64_DIR="release/$VERSION/bookmarks-${VERSION}-linux-amd64"
ARM64_DIR="release/$VERSION/bookmarks-${VERSION}-linux-arm64"

if [ ! -d "$AMD64_DIR" ] || [ ! -f "$AMD64_DIR/bookmarks" ]; then
    echo "Error: 缺少linux-amd64可执行文件"
    exit 1
fi

if [ ! -d "$ARM64_DIR" ] || [ ! -f "$ARM64_DIR/bookmarks" ]; then
    echo "Error: 缺少linux-arm64可执行文件"
    exit 1
fi

echo "可执行文件检查通过"

# 构建镜像名称
REPO_NAME="techfunways/bookmarks"

# 构建模式: 仅本地构建（不推送）
echo "\n构建模式: 仅本地构建（不推送）"

# 构建linux/amd64镜像
echo "\n使用 Docker Buildx 构建多架构镜像..."
docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --build-arg VERSION=${VERSION} \
    --output type=docker \
    -t ${REPO_NAME}:${VERSION} \
    .

if [ $? -ne 0 ]; then
    echo "Error: 构建多架构镜像失败"
    exit 1
fi

echo "✓ 多架构镜像 ${REPO_NAME}:${VERSION} 构建成功"

echo "\n为镜像添加 latest 标签..."
docker tag ${REPO_NAME}:${VERSION} ${REPO_NAME}:latest

if [ $? -eq 0 ]; then
    echo "✓ 已为镜像添加 latest 标签: ${REPO_NAME}:latest"
else
    echo "Warning: 为镜像添加 latest 标签失败"
fi

echo "\n构建完成！"
echo "构建的镜像:"
echo "- ${REPO_NAME}:${VERSION} (多架构镜像，包含 amd64 和 arm64)"
echo "- ${REPO_NAME}:latest (多架构镜像，包含 amd64 和 arm64)"
echo "\n使用方法:"
echo "推荐使用多架构镜像（自动适配架构）: docker run -p 8901:8901 ${REPO_NAME}:latest"
echo "或使用版本号: docker run -p 8901:8901 ${REPO_NAME}:${VERSION}"
echo "\n总结:"
echo "✓ 多架构镜像已成功构建，支持 amd64 和 arm64 架构"
echo "✓ Docker 会自动选择适合当前架构的镜像版本"
echo "\n注意: 镜像只包含二进制可执行文件，不包含 static 目录。"
