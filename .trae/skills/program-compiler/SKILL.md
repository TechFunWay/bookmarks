---
name: "program-compiler"
description: "交叉编译Go项目到不同平台的可执行程序。当需要为多个操作系统和架构构建可执行文件时调用，支持自动版本号获取、目录创建和资源复制。"
---

# 程序编译

## 功能说明

本技能用于交叉编译Go项目到不同平台的可执行程序，自动处理版本号获取、目录创建、资源复制和压缩打包等操作。

## 支持的平台

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## 工作原理

1. **版本号获取**：从 `main.go` 文件中提取 `appVersion` 常量值
2. **目录创建**：在 `release` 目录下创建对应版本和平台的子目录
3. **交叉编译**：为每个平台执行 Go 交叉编译
4. **资源复制**：复制静态资源文件到对应目录（不复制 icons 目录）
5. **压缩打包**：创建压缩包方便分发

## 使用方法

### 基本命令

```bash
# 编译所有支持的平台
.trae/skills/program-compiler/scripts/compile.sh

# 编译指定平台
.trae/skills/program-compiler/scripts/compile.sh linux-amd64
.trae/skills/program-compiler/scripts/compile.sh linux-arm64
.trae/skills/program-compiler/scripts/compile.sh macos-amd64
.trae/skills/program-compiler/scripts/compile.sh macos-arm64
.trae/skills/program-compiler/scripts/compile.sh windows-amd64
.trae/skills/program-compiler/scripts/compile.sh windows-arm64
```

### 编译脚本

脚本文件位于 `./trae/skills/program-compiler/scripts/compile.sh`：

```bash
#!/bin/bash

# 获取版本号
VERSION=$(grep -oP 'appVersion = "\K[^"]+' main.go)

if [ -z "$VERSION" ]; then
    echo "Error: 无法从 main.go 中获取版本号"
    exit 1
fi

echo "使用版本号: $VERSION"

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
    TARGET_DIR="release/bookmarks-v${VERSION}-${PLATFORM}"
    mkdir -p "$TARGET_DIR/static"
    
    # 编译可执行文件
    echo "执行编译..."
    CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w" -o "$TARGET_DIR/bookmarks$EXT" main.go
    
    if [ $? -ne 0 ]; then
        echo "Error: 编译失败"
        continue
    fi
    
    # 复制静态资源（不复制 icons 目录）
    echo "复制静态资源..."
    if [ -d "static" ]; then
        # 复制除 icons 外的所有文件和目录
        find static -type f -not -path "*/icons/*" | while read FILE; do
            REL_PATH=$(echo "$FILE" | sed 's/^static\///')
            DEST_FILE="$TARGET_DIR/static/$REL_PATH"
            mkdir -p "$(dirname "$DEST_FILE")"
            cp "$FILE" "$DEST_FILE"
        done
    fi
    
    # 复制 LICENSE 和 README
    if [ -f "LICENSE" ]; then
        cp LICENSE "$TARGET_DIR/"
    fi
    if [ -f "README.md" ]; then
        cp README.md "$TARGET_DIR/"
    fi
    
    # 创建压缩包
    echo "创建压缩包..."
    cd "release"
    tar -czf "bookmarks-v${VERSION}-${PLATFORM}.tar.gz" "bookmarks-v${VERSION}-${PLATFORM}"
    cd ..
    
    echo "平台 $PLATFORM 编译完成"
done

echo "\n所有编译任务完成！"
echo "编译结果位于: release/ 目录"
