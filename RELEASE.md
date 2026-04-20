# Bookmarks - 多用户书签管理系统

## 快速开始

### Docker 部署（推荐）

**前置要求：**
- Docker 已安装
- Docker Compose 已安装（通常随 Docker Desktop 一起安装）

**一键启动：**
```bash
docker-compose up -d
```

启动成功后，访问：http://localhost:8901

### 直接运行

```bash
# 编译
go build -o bookmarks main.go

# 运行
./bookmarks -dataUrl ./data -port 8901 -logmode release
```

## 功能特性

- **用户认证**：注册/登录/登出
- **书签管理**：文件夹和书签的层级管理
- **元数据获取**：自动获取网页标题和图标
- **Edge 浏览器导入**：支持从 Edge 浏览器导入书签
- **API Key 认证**：支持浏览器扩展同步
- **书签备注**：为书签添加备注
- **安全问题**：通过安全问题重置密码
- **用户管理**：管理员可管理用户
- **系统升级**：自动管理数据库版本升级

## 架构支持

- ✅ **linux/amd64** - Intel 和 AMD 处理器（PC、服务器）
- ✅ **linux/arm64** - ARM 处理器（树莓派、Apple M 系列芯片）
- ✅ **macOS/amd64** - Intel Mac
- ✅ **macOS/arm64** - Apple Silicon Mac
- ✅ **windows/amd64** - Windows 64位
- ✅ **windows/arm64** - Windows ARM64

## Docker 使用

### 使用 docker-compose（推荐）

```bash
# 启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止
docker-compose down

# 重启
docker-compose restart
```

### 直接使用 Docker

```bash
# 运行
docker run -d -p 8901:8901 -v ./data:/app/data techfunways/bookmarks:latest

# 自定义端口
docker run -d -p 8080:8901 -v ./data:/app/data techfunways/bookmarks:latest
```

## 数据持久化

应用数据存储在 `./data` 目录：

```
data/
├── db/           # SQLite 数据库文件
├── icons/        # 网站图标缓存
└── logs/         # 访问日志
```

### 备份数据

```bash
tar -czf bookmarks-backup-$(date +%Y%m%d).tar.gz data/
```

### 恢复数据

```bash
tar -xzf bookmarks-backup-20250101.tar.gz
```

## 密码重置

如果忘记管理员密码，可以使用命令行工具重置：

```bash
# 自动查找管理员账号并重置
./reset-password -password newpassword

# 指定用户名重置
./reset-password -username admin -password newpassword
```

## 系统要求

- **操作系统**：Windows、macOS、Linux
- **Docker 版本**：20.10 或更高版本（Docker 部署）
- **内存要求**：至少 512MB 可用内存
- **磁盘空间**：至少 100MB 可用空间

## 版本信息

- 当前版本：v2.0.0
- 数据库：SQLite（内嵌）
- 端口：8901

## 许可证

MIT License
