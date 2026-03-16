#!/bin/bash

# Docker 多架构构建脚本 v2
# 支持构建和推送多架构 Docker 镜像

set -e

echo "========================================="
echo "  Docker 多架构构建工具 v2"
echo "========================================="
echo ""

# 默认配置
IMAGE_NAME="techfunways/bookmarks"
PUSH_IMAGES=true
USE_DOCKERFILE_BUILD=false
BUILD_PLATFORMS=""

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-push)
            PUSH_IMAGES=false
            echo "📦 只构建，不推送"
            shift
            ;;
        --image)
            if [ -n "$2" ]; then
                IMAGE_NAME="$2"
                echo "📦 镜像名称: $IMAGE_NAME"
                shift 2
            else
                echo "❌ 错误: --image 需要参数"
                exit 1
            fi
            ;;
        --dockerfile-build)
            USE_DOCKERFILE_BUILD=true
            echo "📝 使用 Dockerfile 编译模式"
            shift
            ;;
        --platform)
            if [ -n "$2" ]; then
                BUILD_PLATFORMS="$2"
                echo "🎯 指定平台: $BUILD_PLATFORMS"
                shift 2
            else
                echo "❌ 错误: --platform 需要参数"
                exit 1
            fi
            ;;
        --tag)
            if [ -n "$2" ]; then
                VERSION="$2"
                echo "🏷 指定版本: $VERSION"
                shift 2
            fi
            ;;
        --help|-h)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --no-push              只构建，不推送镜像"
            echo "  --image <name>        指定镜像名称 (默认: techfunways/bookmarks)"
            echo "  --dockerfile-build    使用 Dockerfile 编译模式"
            echo "  --platform <arch>      只构建指定架构 (amd64|arm64)"
            echo "  --tag <version>       指定版本标签"
            echo "  --help, -h            显示帮助信息"
            echo ""
            echo "示例:"
            echo "  $0                           # 构建所有架构并推送"
            echo "  $0 --no-push                # 只构建不推送"
            echo "  $0 --image myrepo/app       # 使用自定义镜像名"
            echo "  $0 --platform amd64          # 只构建 amd64"
            exit 0
            ;;
        *)
            echo "❌ 错误: 未知参数: $1"
            echo "使用 --help 查看帮助"
            exit 1
            ;;
    esac
done

# 获取版本号
if [ -z "$VERSION" ]; then
    VERSION=$(awk -F'"' '/appVersion = "[^"]+"/ {print $2}' main.go)

    if [ -z "$VERSION" ]; then
        echo "❌ 错误: 无法从 main.go 中获取版本号"
        exit 1
    fi

    # 去掉版本号中的 v 前缀（如果有）
    VERSION=${VERSION#v}
fi

echo "📦 版本号: $VERSION"
echo "📦 镜像名称: $IMAGE_NAME"
echo ""

# 检查 Docker 环境
echo "🔍 检查 Docker 环境..."

if ! command -v docker &> /dev/null; then
    echo "❌ 错误: Docker 未安装"
    exit 1
fi

echo "✅ Docker 已安装"

# 检查 Buildx 支持
if ! docker buildx version &> /dev/null; then
    echo "⚠️  警告: Docker Buildx 未安装或未启用"
    echo "   尝试启用 Buildx..."

    if docker buildx create --use &> /dev/null; then
        echo "✅ Buildx 已启用"
    else
        echo "❌ 错误: 无法启用 Buildx"
        echo "   请手动启用: docker buildx create --use"
        exit 1
    fi
else
    echo "✅ Buildx 已安装"
fi

echo ""

# 确定要构建的平台
if [ -n "$BUILD_PLATFORMS" ]; then
    PLATFORMS=("$BUILD_PLATFORMS")
else
    PLATFORMS=("amd64" "arm64")
fi

echo "🎯 将构建以下架构: ${PLATFORMS[@]}"
echo ""

# 检查编译产物（非 Dockerfile 构建模式）
if [ "$USE_DOCKERFILE_BUILD" = false ]; then
    echo "🔍 检查编译产物..."

    for ARCH in "${PLATFORMS[@]}"; do
        PLATFORM_DIR="release/bookmarks-v${VERSION}-linux-${ARCH}"

        if [ ! -d "$PLATFORM_DIR" ]; then
            echo "❌ 错误: 平台目录不存在: $PLATFORM_DIR"
            echo "   请先运行编译脚本: .trae/skills/go-multi-platform-compiler/scripts/compile.sh"
            exit 1
        fi

        if [ ! -f "$PLATFORM_DIR/bookmarks" ]; then
            echo "❌ 错误: 可执行文件不存在: $PLATFORM_DIR/bookmarks"
            exit 1
        fi

        echo "   ✅ $ARCH: $PLATFORM_DIR/bookmarks"
    done

    echo ""
fi

# 创建 Dockerfile（如果使用编译产物）
if [ "$USE_DOCKERFILE_BUILD" = false ]; then
    echo "📝 创建 Dockerfile..."

    cat > Dockerfile.build << 'EOF'
# 阶段1: 准备编译产物
FROM scratch AS build

# 复制编译好的可执行文件
ARG ARCH
COPY release/bookmarks-vVERSION-linux-${ARCH}/bookmarks /bookmarks

# 阶段2: 最终镜像
FROM scratch
COPY --from=build /bookmarks /bookmarks
WORKDIR /
EXPOSE 8901
CMD ["/bookmarks"]
EOF

    # 替换版本号占位符
    sed -i.bak "s/VERSION/$VERSION/g" Dockerfile.build
    rm -f Dockerfile.build.bak

    echo "✅ Dockerfile 已创建"
    echo ""
fi

# 统计
SUCCESS_COUNT=0
FAIL_COUNT=0

# 构建函数
build_architecture() {
    local ARCH=$1
    local FULL_ARCH="linux/${ARCH}"

    echo "----------------------------------------"
    echo "🔨 构建: $FULL_ARCH"
    echo ""

    # 确定镜像标签
    local IMAGE_TAG="${IMAGE_NAME}:v${VERSION}-${ARCH}"

    if [ "$USE_DOCKERFILE_BUILD" = true ]; then
        # 使用 Dockerfile 编译模式
        echo "📝 使用 Dockerfile 编译模式..."
        echo "   执行: docker buildx build --platform ${FULL_ARCH} -t ${IMAGE_TAG}"

        if [ "$PUSH_IMAGES" = true ]; then
            docker buildx build \
                --platform ${FULL_ARCH} \
                -t ${IMAGE_TAG} \
                --push \
                .
        else
            docker buildx build \
                --platform ${FULL_ARCH} \
                -t ${IMAGE_TAG} \
                --load \
                .
        fi
    else
        # 使用编译产物模式
        echo "📦 使用编译产物模式..."

        if [ "$PUSH_IMAGES" = true ]; then
            docker buildx build \
                --platform ${FULL_ARCH} \
                --build-arg ARCH=${ARCH} \
                -f Dockerfile.build \
                -t ${IMAGE_TAG} \
                --push \
                .
        else
            docker buildx build \
                --platform ${FULL_ARCH} \
                --build-arg ARCH=${ARCH} \
                -f Dockerfile.build \
                -t ${IMAGE_TAG} \
                --load \
                .
        fi
    fi

    # 检查构建结果
    if [ $? -eq 0 ]; then
        echo "✅ 构建成功: $IMAGE_TAG"
        ((SUCCESS_COUNT++))
    else
        echo "❌ 构建失败: $FULL_ARCH"
        ((FAIL_COUNT++))
        return 1
    fi

    echo ""
}

# 构建每个架构
for ARCH in "${PLATFORMS[@]}"; do
    build_architecture "$ARCH"
done

# 如果构建了多个架构且都成功，创建 manifest
if [ ${#PLATFORMS[@]} -gt 1 ] && [ $FAIL_COUNT -eq 0 ] && [ "$PUSH_IMAGES" = true ]; then
    echo "========================================="
    echo "  创建多架构 Manifest"
    echo "========================================="
    echo ""

    echo "📦 创建 manifest list..."
    docker manifest create \
        ${IMAGE_NAME}:v${VERSION} \
        ${IMAGE_NAME}:v${VERSION}-amd64 \
        ${IMAGE_NAME}:v${VERSION}-arm64

    if [ $? -eq 0 ]; then
        echo "✅ Manifest 创建成功"
    else
        echo "❌ Manifest 创建失败"
        ((FAIL_COUNT++))
    fi

    echo ""

    echo "📤 推送 manifest..."
    docker manifest push ${IMAGE_NAME}:v${VERSION}

    if [ $? -eq 0 ]; then
        echo "✅ Manifest 推送成功"
    else
        echo "❌ Manifest 推送失败"
        ((FAIL_COUNT++))
    fi

    echo ""

    echo "🏷 创建 latest 标签..."
    docker manifest create \
        ${IMAGE_NAME}:latest \
        ${IMAGE_NAME}:v${VERSION}-amd64 \
        ${IMAGE_NAME}:v${VERSION}-arm64

    echo "📤 推送 latest 标签..."
    docker manifest push ${IMAGE_NAME}:latest

    if [ $? -eq 0 ]; then
        echo "✅ Latest 标签推送成功"
    else
        echo "❌ Latest 标签推送失败"
        ((FAIL_COUNT++))
    fi

    echo ""
fi

# 汇总结果
echo "========================================="
echo "  构建完成"
echo "========================================="
echo "✅ 成功: $SUCCESS_COUNT 个架构"
if [ $FAIL_COUNT -gt 0 ]; then
    echo "❌ 失败: $FAIL_COUNT 个架构"
fi
echo ""

# 显示可用的镜像
echo "📦 可用的镜像:"
docker images | grep "$IMAGE_NAME" | grep -v "<none>" || echo "  (无本地镜像)"

echo ""
echo "💡 使用提示:"
echo "  1. 运行容器: docker run -d -p 8901:8901 ${IMAGE_NAME}:latest"
echo "  2. 数据持久化: docker run -d -p 8901:8901 -v \$(pwd)/data:/app/data ${IMAGE_NAME}:latest"
echo "  3. 指定版本: docker run -d -p 8901:8901 ${IMAGE_NAME}:v${VERSION}"
echo "  4. 指定架构: docker run -d -p 8901:8901 ${IMAGE_NAME}:v${VERSION}-amd64"
echo ""

# 清理临时文件
if [ -f "Dockerfile.build" ]; then
    echo "🧹 清理临时文件..."
    rm -f Dockerfile.build
    echo "✅ Dockerfile.build 已删除"
    echo ""
fi

if [ $FAIL_COUNT -gt 0 ]; then
    exit 1
fi
