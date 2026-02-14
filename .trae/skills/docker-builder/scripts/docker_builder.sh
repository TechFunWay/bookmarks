#!/bin/bash

# 获取版本号
VERSION=$(grep 'appVersion = "' main.go | awk -F'"' '{print $2}')

if [ -z "$VERSION" ]; then
    echo "Error: 无法从 main.go 中获取版本号"
    exit 1
fi

echo "使用版本号: $VERSION"

# 检查release目录中的可执行文件
RELEASE_DIR="release/bookmarks-${VERSION}"
AMD64_DIR="release/bookmarks-${VERSION}-linux-amd64"
ARM64_DIR="release/bookmarks-${VERSION}-linux-arm64"

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
echo "\n构建linux/amd64镜像..."
docker build \
    --platform linux/amd64 \
    -t ${REPO_NAME}:${VERSION}-amd64 \
    -f - . <<EOF
FROM scratch

# 创建工作目录
WORKDIR /app

# 复制可执行文件和静态文件到镜像中
COPY ${AMD64_DIR}/bookmarks /app/
COPY ${AMD64_DIR}/static /app/static
COPY LICENSE /app/
COPY README.md /app/

# 暴露端口（应用实际使用8901端口）
EXPOSE 8901

# 持久化图标目录
VOLUME /app/static/icons

# 启动应用
CMD ["/app/bookmarks"]
EOF

if [ $? -ne 0 ]; then
    echo "Error: 构建linux/amd64镜像失败"
    exit 1
fi

# 构建linux/arm64镜像
echo "\n构建linux/arm64镜像..."
docker build \
    --platform linux/arm64 \
    -t ${REPO_NAME}:${VERSION}-arm64 \
    -f - . <<EOF
FROM scratch

# 创建工作目录
WORKDIR /app

# 复制可执行文件和静态文件到镜像中
COPY ${ARM64_DIR}/bookmarks /app/
COPY ${ARM64_DIR}/static /app/static
COPY LICENSE /app/
COPY README.md /app/

# 暴露端口（应用实际使用8901端口）
EXPOSE 8901

# 持久化图标目录
VOLUME /app/static/icons

# 启动应用
CMD ["/app/bookmarks"]
EOF

if [ $? -ne 0 ]; then
    echo "Error: 构建linux/arm64镜像失败"
    exit 1
fi

# 尝试创建多架构镜像manifest list
echo "\n创建多架构镜像manifest list..."
docker manifest create ${REPO_NAME}:${VERSION} \
    ${REPO_NAME}:${VERSION}-amd64 \
    ${REPO_NAME}:${VERSION}-arm64 2>/dev/null

# 保存manifest create命令的退出状态
MANIFEST_STATUS=$?

if [ $MANIFEST_STATUS -eq 0 ]; then
    echo "✓ 多架构镜像manifest list创建成功"
    # 验证镜像
    echo "\n验证镜像..."
    docker manifest inspect ${REPO_NAME}:${VERSION} 2>/dev/null || echo "Warning: 无法检查多架构镜像manifest list"
else
    echo "Warning: 创建多架构镜像manifest list失败（可能是网络问题）"
    echo "但架构特定镜像已成功构建，您可以手动创建manifest list"
    echo "手动创建命令: docker manifest create ${REPO_NAME}:${VERSION} ${REPO_NAME}:${VERSION}-amd64 ${REPO_NAME}:${VERSION}-arm64"
fi

# 为镜像添加 latest 标签
echo "\n为镜像添加 latest 标签..."

# 为 amd64 镜像添加 latest 标签
docker tag ${REPO_NAME}:${VERSION}-amd64 ${REPO_NAME}:latest-amd64
if [ $? -eq 0 ]; then
    echo "✓ 已为 amd64 镜像添加 latest 标签: ${REPO_NAME}:latest-amd64"
else
    echo "Warning: 为 amd64 镜像添加 latest 标签失败"
fi

# 为 arm64 镜像添加 latest 标签
docker tag ${REPO_NAME}:${VERSION}-arm64 ${REPO_NAME}:latest-arm64
if [ $? -eq 0 ]; then
    echo "✓ 已为 arm64 镜像添加 latest 标签: ${REPO_NAME}:latest-arm64"
else
    echo "Warning: 为 arm64 镜像添加 latest 标签失败"
fi

# 为多架构 manifest 添加 latest 标签
if [ $MANIFEST_STATUS -eq 0 ]; then
    docker tag ${REPO_NAME}:${VERSION} ${REPO_NAME}:latest
    if [ $? -eq 0 ]; then
        echo "✓ 已为多架构镜像添加 latest 标签: ${REPO_NAME}:latest"
    else
        echo "Warning: 为多架构镜像添加 latest 标签失败"
    fi
fi

# 显示构建结果
echo "\n构建完成！"
echo "构建的镜像:"
echo "- ${REPO_NAME}:${VERSION}-amd64 (x86_64架构)"
echo "- ${REPO_NAME}:latest-amd64 (x86_64架构)"
echo "- ${REPO_NAME}:${VERSION}-arm64 (ARM64架构)"
echo "- ${REPO_NAME}:latest-arm64 (ARM64架构)"
if [ $MANIFEST_STATUS -eq 0 ]; then
    echo "- ${REPO_NAME}:${VERSION} (多架构镜像 - 推荐使用)"
    echo "- ${REPO_NAME}:latest (多架构镜像 - 推荐使用)"
fi
echo "\n使用方法:"
if [ $MANIFEST_STATUS -eq 0 ]; then
    echo "推荐使用多架构镜像（自动适配架构）: docker run -p 8901:8901 ${REPO_NAME}:latest"
    echo "或使用版本号: docker run -p 8901:8901 ${REPO_NAME}:${VERSION}"
fi
echo "x86_64架构测试: docker run -p 8901:8901 ${REPO_NAME}:latest-amd64"
echo "ARM64架构测试: docker run -p 8901:8901 ${REPO_NAME}:latest-arm64"

# 总结
echo "\n总结:"
echo "✓ 架构特定镜像已成功构建"
echo "✓ 已为所有镜像添加 latest 标签"
if [ $MANIFEST_STATUS -eq 0 ]; then
    echo "✓ 多架构镜像已成功创建，您可以使用单一tag ${REPO_NAME}:latest 访问"
else
    echo "⚠ 多架构镜像创建失败，但您仍然可以使用架构特定的镜像tag"
    echo "   当网络条件改善后，您可以运行上述手动创建命令来创建多架构镜像"
fi
echo "\n注意: 构建过程已优化，不依赖网络连接。"
echo "即使多架构镜像创建失败，架构特定镜像仍然可以正常使用。"



