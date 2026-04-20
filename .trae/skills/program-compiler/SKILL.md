---
name: "program-compiler"
description: "交叉编译Go项目到不同平台的可执行程序。当需要为多个操作系统和架构构建可执行文件时调用，支持自动版本号获取、目录创建和资源复制。"
---

# 程序编译

## 功能说明

本技能用于交叉编译 Go 项目到不同平台的可执行程序，自动处理版本号获取、目录创建、浏览器插件打包和压缩打包等操作。

## 支持的平台

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## 工作原理

1. **版本号获取**：从 `main.go` 文件中提取 `appVersion` 常量值
2. **浏览器插件打包**：将 edge-extension 目录打包为 zip
3. **目录创建**：在 `release` 目录下创建对应版本和平台的子目录
4. **交叉编译**：为每个平台执行 Go 交叉编译
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

### 编译流程

1. 从 main.go 获取版本号
2. 打包浏览器插件到 `static/downloads/edge-extension.zip`
3. 创建 release 目录
4. 为每个平台编译可执行文件
5. 创建压缩包

### 编译参数

| 平台 | GOOS | GOARCH | CGO_ENABLED | 说明 |
|------|------|--------|-------------|------|
| linux-amd64 | linux | amd64 | 0 | 静态编译，用于 Docker |
| linux-arm64 | linux | arm64 | 0 | 静态编译，用于 Docker |
| macos-amd64 | darwin | amd64 | 1 | |
| macos-arm64 | darwin | arm64 | 1 | |
| windows-amd64 | windows | amd64 | 1 | |
| windows-arm64 | windows | arm64 | 1 | |

**重要：** Linux 平台使用 `CGO_ENABLED=0` 进行静态编译，因为 Docker 镜像基于 scratch 基础镜像，不包含 C 库。

### 编译产物

编译完成后，在 `release/` 目录下生成：

```
release/
├── bookmarks-v2.0.0-linux-amd64/
│   └── bookmarks
├── bookmarks-v2.0.0-linux-arm64/
│   └── bookmarks
├── bookmarks-v2.0.0-macos-amd64/
│   └── bookmarks
├── bookmarks-v2.0.0-macos-arm64/
│   └── bookmarks
├── bookmarks-v2.0.0-windows-amd64/
│   └── bookmarks.exe
├── bookmarks-v2.0.0-windows-arm64/
│   └── bookmarks.exe
├── bookmarks-v2.0.0-linux-amd64.tar.gz
├── bookmarks-v2.0.0-linux-arm64.tar.gz
├── bookmarks-v2.0.0-macos-amd64.tar.gz
├── bookmarks-v2.0.0-macos-arm64.tar.gz
├── bookmarks-v2.0.0-windows-amd64.tar.gz
└── bookmarks-v2.0.0-windows-arm64.tar.gz
```

## 注意事项

1. Linux 平台使用 `CGO_ENABLED=0` 静态编译，不依赖 C 库
2. macOS 和 Windows 平台使用 `CGO_ENABLED=1`
3. 编译产物只包含可执行文件，不包含 static 目录、LICENSE 和 README.md
4. 浏览器插件会在编译前自动打包到 `static/downloads/edge-extension.zip`
5. 使用 `-ldflags="-s -w"` 去除调试信息，减小可执行文件体积

## 故障排查

### 常见错误

1. **CGO 编译失败**：确保安装了对应的 C 编译器（macOS 需要 Xcode Command Line Tools）
2. **版本号获取失败**：确保 main.go 中存在 `appVersion = "vX.X.X"` 常量
3. **权限问题**：确保脚本有执行权限

### 解决方法

- 安装 Xcode Command Line Tools：`xcode-select --install`
- 检查版本号：`grep 'appVersion = "' main.go`
- 添加执行权限：`chmod +x .trae/skills/program-compiler/scripts/compile.sh`
