# Docker多架构构建技能

## 功能描述

该技能用于为linux系统的amd平台和arm平台构建docker镜像，支持自动识别用户的平台架构，实现无缝的跨架构部署。

## 构建方式

使用编译好的可执行文件来构建，利用项目现有的编译结果，避免重复编译，提高构建速度，减小镜像体积。

**重要：镜像只包含二进制可执行文件，不包含 static 目录、LICENSE 和 README.md 文件。**

## 支持的架构

- linux/amd64 (x86_64)
- linux/arm64 (ARM64)

## 前置条件

1. 已安装Docker，且版本支持Buildx
2. 已运行项目的编译脚本，生成了release目录中的可执行文件
3. 已启用Docker Buildx（技能会自动检查和启用）

## 使用方法

### 基本用法

在项目根目录执行以下命令：

```bash
.trae/skills/docker-builder/scripts/docker_builder.sh
```

### 构建流程

1. 从main.go中获取版本号
2. 检查release目录中的可执行文件是否存在
3. 启用Docker Buildx
4. 构建linux/amd64镜像
5. 构建linux/arm64镜像
6. 创建并推送manifest list
7. 为仓库名称创建tag和推送
8. 验证镜像构建结果

## 构建结果

构建完成后，会生成以下镜像：

- 版本镜像：`techfunways/bookmarks:v<版本号>-amd64` (amd64架构)
- 版本镜像：`techfunways/bookmarks:v<版本号>-arm64` (arm64架构)
- 多架构镜像：`techfunways/bookmarks:v<版本号>` (多架构，包含amd64和arm64)
- 多架构镜像：`techfunways/bookmarks:latest` (多架构，包含amd64和arm64)

## 使用示例

### 本地测试

```bash
# 使用多架构镜像（推荐）
docker run -p 8901:8901 techfunways/bookmarks:latest

# 使用版本号
docker run -p 8901:8901 techfunways/bookmarks:v1.8.0

# 使用特定架构
docker run -p 8901:8901 techfunways/bookmarks:v1.8.0-amd64
docker run -p 8901:8901 techfunways/bookmarks:v1.8.0-arm64
```

### 数据持久化

```bash
# 挂载数据目录
docker run -d -p 8901:8901 -v /path/to/data:/app/data techfunways/bookmarks:latest
```

### 自动架构识别

当用户执行上述命令时，Docker会自动识别用户的平台架构，并拉取对应架构的镜像版本，无需用户手动指定架构。

## 技术特点

1. **多架构支持**：同时支持amd64和arm64架构
2. **自动识别**：利用Docker manifest实现自动架构识别
3. **快速构建**：使用编译好的可执行文件，构建速度快
4. **体积小巧**：基于scratch基础镜像，镜像体积小
5. **版本管理**：自动从main.go获取版本号，保持版本一致性

## 注意事项

1. 确保release目录中存在对应版本的linux-amd64和linux-arm64可执行文件
2. 构建过程需要网络连接，用于推送镜像
3. 首次构建可能需要较长时间，因为需要拉取基础镜像
4. 如需修改镜像名称或仓库地址，请修改脚本中的相关变量

## 故障排查

### 常见错误

1. **缺少可执行文件**：确保已运行编译脚本生成可执行文件
2. **Docker Buildx未启用**：脚本会自动尝试启用Buildx
3. **网络连接问题**：确保网络连接正常，能够推送镜像
4. **权限问题**：确保当前用户有执行Docker命令的权限

### 解决方法

- 运行编译脚本：`./scripts/compile.sh linux-amd64 linux-arm64`
- 检查Docker服务状态：`systemctl status docker`（Linux）或重启Docker Desktop（Windows/Mac）
- 检查网络连接：`ping docker.io`
- 确保用户加入docker组：`sudo usermod -aG docker $USER`（Linux）
