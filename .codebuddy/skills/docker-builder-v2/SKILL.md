---
name: "docker-builder-v2"
description: "Docker 多架构构建工具 v2，支持构建和推送多架构 Docker 镜像，自动版本管理、Buildx 优化和安全配置。"
---

# Docker 多架构构建工具 v2

## 功能说明

本技能用于构建和推送 Docker 多架构镜像，支持自动版本管理、Buildx 优化、安全配置和完整性验证。

## 支持的架构

| 架构 | 平台 | 输出镜像 | 说明 |
|-----|------|----------|------|
| amd64 | linux/amd64 | `techfunways/bookmarks:v{版本}-amd64` | x86_64 服务器 |
| arm64 | linux/arm64 | `techfunways/bookmarks:v{版本}-arm64` | ARM 服务器（飞牛、NAS） |
| 多架构 | linux/amd64,linux/arm64 | `techfunways/bookmarks:v{版本}` | 自动识别用户架构 |

## 构建方式

### 1. 基于编译产物（推荐）

使用已经编译好的可执行文件构建，避免重复编译，构建速度快。

**优点：**
- ✅ 构建速度快（无需交叉编译）
- ✅ 镜像体积小（只有二进制文件）
- ✅ 可控性强（使用本地编译产物）

**注意：**
- 镜像只包含可执行文件，不包含 static 目录
- 适合生产环境部署

### 2. 多阶段构建（备选）

在 Dockerfile 中编译和构建。

**优点：**
- ✅ 完全自动化
- ✅ 可在 CI/CD 中使用

**缺点：**
- ⏱ 构建时间长（需要交叉编译）
- 📦 镜像体积较大

## 工作原理

### 构建流程

1. **版本号获取**：从 `main.go` 提取 `appVersion` 常量
2. **编译产物验证**：检查 `release/` 目录下的可执行文件
3. **Buildx 配置**：
   - 检查并启用 Docker Buildx
   - 创建 multi-platform 构建器
4. **镜像构建**：
   - 构建 `linux/amd64` 镜像
   - 构建 `linux/arm64` 镜像
5. **Manifest 创建**：
   - 创建多架构 manifest list
   - 推送 manifest 到仓库
6. **标签管理**：
   - 创建版本标签：`v{版本号}`
   - 创建 latest 标签
7. **完整性验证**：验证镜像构建和推送结果

## 使用方法

### 基本用法

```bash
# 构建所有架构并推送
.trae/skills/docker-builder-v2/scripts/docker_builder.sh

# 只构建不推送
.trae/skills/docker-builder-v2/scripts/docker_builder.sh --no-push

# 指定镜像名称
.trae/skills/docker-builder-v2/scripts/docker_builder.sh --image myregistry/bookmarks

# 使用 Dockerfile 编译模式
.trae/skills/docker-builder-v2/scripts/docker_builder.sh --dockerfile-build
```

### 参数说明

| 参数 | 说明 | 默认值 |
|-----|------|-------|
| `--no-push` | 只构建不推送 | false |
| `--image <name>` | 指定镜像名称 | techfunways/bookmarks |
| `--dockerfile-build` | 使用 Dockerfile 编译模式 | false |
| `--platform <arch>` | 只构建指定架构 | amd64,arm64 |
| `--tag <tag>` | 指定版本标签 | 从 main.go 获取 |

## 前置条件

### 1. Docker 环境

```bash
# 检查 Docker 版本
docker --version

# 检查 Buildx 支持
docker buildx version

# 如果 Buildx 未启用，手动启用
docker buildx create --use
```

### 2. 编译产物

确保已运行编译脚本：

```bash
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 linux-arm64
```

### 3. Docker 镜像仓库

- **Docker Hub**: 需要先 `docker login`
- **私有仓库**: 需要配置认证
- **阿里云/腾讯云**: 需要配置仓库地址

## Dockerfile 配置

### 基于编译产物（推荐）

```dockerfile
# 阶段1: 准备编译产物
FROM scratch AS build

# 复制编译好的可执行文件
COPY release/bookmarks-v1.9.0-linux-amd64/bookmarks /bookmarks

# 阶段2: 最终镜像
FROM scratch
COPY --from=build /bookmarks /bookmarks
WORKDIR /
EXPOSE 8901
CMD ["/bookmarks", "-dataUrl", "./data"]
```

### 多阶段构建

```dockerfile
# 阶段1: 编译
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o bookmarks main.go

# 阶段2: 最终镜像
FROM scratch
COPY --from=builder /app/bookmarks /bookmarks
WORKDIR /
EXPOSE 8901
CMD ["/bookmarks", "-dataUrl", "./data"]
```

## 输出结果

构建完成后，生成以下镜像：

```bash
# 单架构镜像
techfunways/bookmarks:v1.9.0-amd64
techfunways/bookmarks:v1.9.0-arm64

# 多架构镜像
techfunways/bookmarks:v1.9.0
techfunways/bookmarks:latest
```

## 使用方式

### 拉取和运行

```bash
# 自动选择架构（推荐）
docker run -d -p 8901:8901 --name bookmarks techfunways/bookmarks:latest

# 指定版本
docker run -d -p 8901:8901 --name bookmarks techfunways/bookmarks:v1.9.0

# 指定架构
docker run -d -p 8901:8901 --name bookmarks techfunways/bookmarks:v1.9.0-amd64
```

### 数据持久化

```bash
# 挂载数据目录
docker run -d \
  -p 8901:8901 \
  -v /path/to/data:/app/data \
  --name bookmarks \
  techfunways/bookmarks:latest

# 挂载配置目录
docker run -d \
  -p 8901:8901 \
  -v /path/to/data:/app/data \
  -v /path/to/config:/app/config \
  --name bookmarks \
  techfunways/bookmarks:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  bookmarks:
    image: techfunways/bookmarks:latest
    container_name: bookmarks
    ports:
      - "8901:8901"
    volumes:
      - ./data:/app/data
    restart: unless-stopped
```

## 技术要点

### 1. Buildx 多架构构建

```bash
# 创建多平台构建器
docker buildx create --name multiarch --use

# 构建并推送
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t techfunways/bookmarks:v1.9.0 \
  -t techfunways/bookmarks:latest \
  --push \
  .
```

### 2. Manifest 管理

```bash
# 创建 manifest list
docker manifest create \
  techfunways/bookmarks:v1.9.0 \
  techfunways/bookmarks:v1.9.0-amd64 \
  techfunways/bookmarks:v1.9.0-arm64

# 推送 manifest
docker manifest push techfunways/bookmarks:v1.9.0
```

### 3. Scratch 镜像优化

- 最小化镜像体积（只有可执行文件）
- 无操作系统依赖
- 安全性高（攻击面小）

### 4. 自动架构识别

```bash
# 用户执行此命令时，Docker 自动选择对应架构
docker run techfunways/bookmarks:latest

# Docker 会根据主机架构自动拉取:
# - amd64 主机: techfunways/bookmarks:v1.9.0-amd64
# - arm64 主机: techfunways/bookmarks:v1.9.0-arm64
```

## 常见问题

### Q1: 提示"Buildx 未启用"

**A**: 启用 Buildx：

```bash
docker buildx create --use

# 或使用默认构建器
docker buildx use default
```

### Q2: 推送镜像失败

**A**: 检查登录状态：

```bash
# 登录 Docker Hub
docker login

# 或登录私有仓库
docker login myregistry.com
```

### Q3: 构建速度慢

**A**: 使用缓存和并行构建：

```bash
# 启用 BuildKit 缓存
DOCKER_BUILDKIT=1 docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --cache-from type=local,src=/tmp/.buildx-cache \
  --cache-to type=local,dest=/tmp/.buildx-cache-new \
  --push \
  .
```

### Q4: 镜像体积过大

**A**: 使用 scratch 基础镜像和优化编译：

```bash
# 使用 -ldflags="-s -w" 优化
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -ldflags="-s -w" -o bookmarks main.go

# 使用 UPX 进一步压缩
upx --best --lzma bookmarks
```

## CI/CD 集成

### GitHub Actions 示例

```yaml
name: Docker Build

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2

      - name: Login to Docker Hub
        uses: docker/login-action@v2
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Download artifacts
        uses: actions/download-artifact@v3
        with:
          path: release/

      - name: Build and push
        run: |
          .trae/skills/docker-builder-v2/scripts/docker_builder.sh --push
```

### GitLab CI 示例

```yaml
docker-build:
  image: docker:latest
  services:
    - docker:dind
  script:
    - .trae/skills/docker-builder-v2/scripts/docker_builder.sh --push
  only:
    - tags
    - /^v\d+\.\d+\.\d+$/
```

## 安全建议

1. **使用官方镜像**：尽量使用官方基础镜像
2. **最小权限原则**：容器内使用非 root 用户运行
3. **定期更新**：及时更新基础镜像和依赖
4. **扫描漏洞**：使用 `docker scan` 检测安全漏洞
5. **签名验证**：对镜像进行签名验证

## 相关技能

- **go-multi-platform-compiler**: 多平台 Go 编译
- **fnapp-packager-v2**: 飞牛应用打包

使用这三个技能可以完成从编译、打包到发布的完整自动化流程。

## 更新日志

### v2.0 (2026-03-16)
- ✨ 全新设计，改进架构和文档
- ✨ 增强错误处理和日志输出
- ✨ 添加参数化配置支持
- ✨ 支持多种构建模式
- ✨ 完整性验证和健康检查
- 🐛 修复 Buildx 兼容性问题
- 📝 完善文档和故障排查
