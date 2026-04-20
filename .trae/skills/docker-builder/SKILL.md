---
name: "docker-builder"
description: "飞牛应用打包工具，用于为 techfunway.bookmarks 应用创建适用于不同平台的 FnApp 安装包"
---

# Docker 多架构构建技能

## 功能描述

该技能用于为 linux 系统的 amd64 和 arm64 平台构建 Docker 多架构镜像，使用 Docker Buildx 实现完全离线的本地构建，无需网络连接。

## 构建方式

使用编译好的可执行文件来构建，利用项目现有的编译结果，避免重复编译，提高构建速度，减小镜像体积。

**镜像内容：**
- `bookmarks` - 主程序二进制文件
- `reset-password` - 密码重置工具

**不包含：** static 目录、LICENSE、README.md

## 支持的架构

- linux/amd64 (x86_64)
- linux/arm64 (ARM64)

## 前置条件

1. 已安装 Docker，且版本支持 Buildx
2. 已运行项目的编译脚本，生成了 release 目录中的可执行文件
3. Docker Buildx 可用（脚本会自动检查）

## 使用方法

### 基本用法

在项目根目录执行以下命令：

```bash
.trae/skills/docker-builder/scripts/docker_builder.sh
```

### 构建流程

1. 从 main.go 中获取版本号
2. 检查 release 目录中的可执行文件是否存在
3. 使用 Docker Buildx 构建多架构镜像（amd64 + arm64）
4. 为镜像添加 latest 标签

## Dockerfile 配置

```dockerfile
FROM scratch
WORKDIR /app
ARG TARGETARCH
ARG VERSION
COPY release/bookmarks-${VERSION}-linux-${TARGETARCH}/bookmarks /app/bookmarks
COPY release/bookmarks-${VERSION}-linux-${TARGETARCH}/reset-password /app/reset-password
EXPOSE 8901
VOLUME /app/data
CMD ["/app/bookmarks"]
```

**关键特性：**
- `ARG VERSION` - 通过构建参数传入版本号
- `ARG TARGETARCH` - Docker Buildx 自动填充目标架构
- 基于 scratch 基础镜像，体积最小化

## 构建结果

构建完成后，会生成以下镜像：

- 多架构镜像：`techfunways/bookmarks:v<版本号>` (包含 amd64 和 arm64)
- 多架构镜像：`techfunways/bookmarks:latest` (包含 amd64 和 arm64)

## 使用示例

### 运行容器

```bash
# 使用 latest 标签（推荐，自动适配架构）
docker run -d -p 8901:8901 techfunways/bookmarks:latest

# 使用版本号
docker run -d -p 8901:8901 techfunways/bookmarks:v2.0.0
```

### 使用 docker-compose

```bash
docker-compose up -d
```

### 数据持久化

```bash
docker run -d -p 8901:8901 -v /path/to/data:/app/data techfunways/bookmarks:latest
```

### 自动架构识别

当用户执行上述命令时，Docker 会自动识别用户的平台架构，并拉取对应架构的镜像版本，无需用户手动指定架构。

## 技术特点

1. **完全离线构建**：使用 Docker Buildx 本地构建，不需要访问 Docker Hub
2. **多架构支持**：同时支持 amd64 和 arm64 架构
3. **自动架构识别**：Docker 自动选择适合当前架构的镜像版本
4. **快速构建**：使用编译好的可执行文件，构建速度快
5. **体积小巧**：基于 scratch 基础镜像，镜像体积小
6. **版本管理**：自动从 main.go 获取版本号，保持版本一致性

## 注意事项

1. 确保 release 目录中存在对应版本的 linux-amd64 和 linux-arm64 可执行文件
2. 构建过程不需要网络连接（完全本地构建）
3. 如需修改镜像名称或仓库地址，请修改脚本中的 REPO_NAME 变量
4. 镜像中包含 bookmarks 和 reset-password 两个可执行文件

## 故障排查

### 常见错误

1. **缺少可执行文件**：确保已运行编译脚本生成可执行文件
2. **Docker Buildx 不可用**：检查 Docker 版本是否支持 Buildx
3. **权限问题**：确保当前用户有执行 Docker 命令的权限

### 解决方法

- 运行编译脚本：`bash .trae/skills/program-compiler/scripts/compile.sh`
- 检查 Docker Buildx：`docker buildx version`
- 确保用户加入 docker 组：`sudo usermod -aG docker $USER`（Linux）
