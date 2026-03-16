---
name: "go-multi-platform-compiler"
description: "Go多平台编译工具，支持交叉编译到不同平台架构，自动版本号管理、资源复制和构建优化。"
---

# Go 多平台编译工具

## 功能说明

本技能用于将 Go 项目交叉编译到不同平台和架构的可执行程序，支持自动版本号获取、资源文件复制、构建优化和错误处理。

## 支持的平台

| 操作系统 | 架构 | 编译目标 | 说明 |
|---------|-------|---------|------|
| Linux | amd64 | linux-amd64 | 适用于 x86_64 服务器和 PC |
| Linux | arm64 | linux-arm64 | 适用于 ARM 服务器（飞牛、NAS） |
| macOS | amd64 | macos-amd64 | 适用于 Intel Mac |
| macOS | arm64 | macos-arm64 | 适用于 Apple Silicon (M1/M2/M3) |
| Windows | amd64 | windows-amd64 | 适用于 x86_64 Windows |
| Windows | arm64 | windows-arm64 | 适用于 ARM Windows |

## 工作原理

1. **版本号获取**：从 `main.go` 文件中提取 `appVersion` 常量值
2. **平台筛选**：根据参数选择编译平台
3. **环境配置**：设置 CGO_ENABLED、GOOS、GOARCH 等环境变量
4. **交叉编译**：执行 Go 编译，使用 `-ldflags="-s -w"` 优化二进制大小
5. **资源复制**：
   - 复制 `static` 目录（排除 icons 目录）
   - 复制 `LICENSE` 和 `README.md` 文件
   - 复制必要的配置文件
6. **目录结构**：在 `release` 目录下创建规范的目录结构

## 使用方法

### 编译所有平台

```bash
.trae/skills/go-multi-platform-compiler/scripts/compile.sh
```

### 编译指定平台

```bash
# 单个平台
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64

# 多个平台
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 linux-arm64 macos-amd64
```

### 平台快捷方式

```bash
# 只编译飞牛需要的 Linux 平台
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 linux-arm64

# 只编译 Windows 平台
.trae/skills/go-multi-platform-compiler/scripts/compile.sh windows-amd64

# 只编译 macOS 平台
.trae/skills/go-multi-platform-compiler/scripts/compile.sh macos-amd64 macos-arm64
```

## 技术要点

### 1. 编译优化

```bash
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bookmarks main.go
```

- `-s`: 去除符号表
- `-w`: 去除 DWARF 调试信息
- 结果：减小二进制文件大小约 30-40%

### 2. CGO 支持

- `CGO_ENABLED=1`: 启用 CGO，支持 SQLite 等需要 CGO 的库
- 静态编译：确保可执行文件在目标平台可直接运行

### 3. 资源处理

- 自动排除 `static/icons/` 目录（图标是运行时下载的）
- 保留 `static/js/`、`static/css/`、`static/html/` 等前端资源
- 复制配置文件：`docker-compose.yaml`、`Dockerfile`

## 输出结果

编译完成后，在 `release/` 目录下生成：

```
release/
├── bookmarks-v1.9.0-linux-amd64/
│   ├── bookmarks
│   ├── static/
│   │   ├── index.html
│   │   ├── login.html
│   │   └── ...
│   ├── LICENSE
│   ├── README.md
│   └── docker-compose.yaml
├── bookmarks-v1.9.0-linux-arm64/
│   └── ...
├── bookmarks-v1.9.0-macos-amd64/
│   └── ...
└── ...
```

## 前置条件

1. **Go 环境**：已安装 Go 1.16+
2. **交叉编译**：已配置交叉编译环境（可选）
3. **CGO 支持**：如果使用 CGO，需要对应的交叉编译工具链

### Windows 交叉编译

```bash
# macOS/Linux 编译 Windows
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -o bookmarks.exe main.go
```

### macOS 交叉编译

```bash
# Linux 编译 macOS
CGO_ENABLED=1 CC=o64-clang GOOS=darwin GOARCH=amd64 go build -o bookmarks main.go
```

## 常见问题

### Q1: 编译失败，提示 CGO 错误

**A**: 检查是否已安装交叉编译工具链：

```bash
# macOS/Linux
sudo apt-get install gcc-x86-64-linux-gnu gcc-aarch64-linux-gnu

# macOS (Homebrew)
brew install x86_64-linux-gnu-gcc aarch64-linux-gnu-gcc
```

### Q2: 生成的可执行文件很大

**A**: 已使用 `-ldflags="-s -w"` 优化。如需进一步减小：

```bash
# 使用 upx 压缩
upx --best --lzma bookmarks
```

### Q3: 编译后无法运行

**A**: 检查文件权限：

```bash
chmod +x release/bookmarks-v*/bookmarks
```

### Q4: 如何只编译特定的几个平台？

**A**: 使用参数指定平台：

```bash
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 macos-arm64
```

## 集成到 CI/CD

### GitHub Actions 示例

```yaml
name: Build

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    strategy:
      matrix:
        include:
          - platform: linux-amd64
            goos: linux
            goarch: amd64
          - platform: linux-arm64
            goos: linux
            goarch: arm64
          - platform: macos-amd64
            goos: darwin
            goarch: amd64
          - platform: macos-arm64
            goos: darwin
            goarch: arm64

    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Build
        run: |
          CGO_ENABLED=1 GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} \
          go build -ldflags="-s -w" -o bookmarks-${{ matrix.platform }} main.go

      - name: Upload
        uses: actions/upload-artifact@v3
        with:
          name: bookmarks-${{ matrix.platform }}
          path: bookmarks-${{ matrix.platform }}
```

## 性能优化建议

1. **并行编译**：使用 `make` 或并行脚本同时编译多个平台
2. **缓存依赖**：使用 Go modules 缓存加速编译
3. **增量编译**：只重新编译修改的文件
4. **二进制压缩**：使用 UPX 进一步减小体积

## 注意事项

1. **静态链接**：某些平台可能需要静态链接 `musl` 或 `glibc`
2. **系统依赖**：检查目标平台的系统库依赖
3. **测试验证**：编译后务必在目标平台测试运行
4. **版本管理**：更新版本号后记得提交到代码仓库

## 相关技能

- **fnapp-packager**: 打包飞牛应用安装包
- **docker-builder**: 构建 Docker 多架构镜像

使用这些技能可以完成从编译到发布的完整流程。
