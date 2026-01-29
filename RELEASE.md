# 书签管理 - Docker 版本

## 快速开始

### 前置要求
- Docker 已安装（[安装指南](https://docs.docker.com/get-docker/)）
- Docker Compose 已安装（通常随Docker Desktop一起安装）

### 一键启动

**Linux/macOS用户：**
```bash
cd docker
./start.sh
```

**Windows用户：**
```cmd
cd docker
start.bat
```

启动成功后，访问：http://localhost:8901

## 自定义端口

如果8901端口被占用，可以使用其他端口：

**Linux/macOS：**
```bash
cd docker
./start.sh start 8080
```

**Windows：**
```cmd
cd docker
start.bat start 8080
```

## 详细使用指南

请查看 `docker/README.md` 获取完整的使用文档，包括：
- 快速启动
- 自定义端口
- 手动构建和运行
- 常用命令
- 数据持久化
- 故障排除

## 系统要求

- **操作系统**：Windows、macOS、Linux
- **Docker版本**：20.10 或更高版本
- **内存要求**：至少 512MB 可用内存
- **磁盘空间**：至少 100MB 可用空间

## 架构支持

- ✅ **AMD64/x86_64** - Intel和AMD处理器（PC、服务器）
- ✅ **ARM64/aarch64** - ARM处理器（树莓派、Apple M系列芯片）

## 常用命令

### 查看日志
```bash
cd docker
./start.sh logs        # Linux/macOS
start.bat logs         # Windows
```

### 停止应用
```bash
cd docker
./start.sh stop        # Linux/macOS
start.bat stop         # Windows
```

### 重启应用
```bash
cd docker
./start.sh restart     # Linux/macOS
start.bat restart      # Windows
```

### 查看状态
```bash
cd docker
./start.sh status     # Linux/macOS
start.bat status      # Windows
```

### 更新应用
```bash
cd docker
./start.sh update     # Linux/macOS
start.bat update      # Windows
```

## 数据持久化

应用数据存储在 `./data` 目录（相对于项目根目录）。即使删除容器，数据也不会丢失。

### 备份数据
```bash
# 备份整个data目录
tar -czf bookmarks-backup-$(date +%Y%m%d).tar.gz data/
```

### 恢复数据
```bash
# 解压备份
tar -xzf bookmarks-backup-20240120.tar.gz

# 重启容器
cd docker
./start.sh restart
```

## 故障排除

### 端口被占用
```bash
# 查看端口占用情况
lsof -i :8901  # macOS/Linux
netstat -ano | findstr :8901  # Windows

# 解决方法：使用其他端口
cd docker
./start.sh start 8080
```

### 容器无法启动
```bash
# 查看容器日志
cd docker
./start.sh logs

# 检查Docker是否正常运行
docker ps
```

### 无法访问应用
```bash
# 检查容器状态
docker ps | grep bookmarks

# 检查容器健康状态
docker inspect bookmarks | grep -A 10 Health
```

## 安全建议

1. **不要暴露到公网**：应用设计为本地使用，不建议直接暴露到互联网
2. **定期备份数据**：定期备份 `data` 目录
3. **使用防火墙**：限制外部访问
4. **更新Docker**：保持Docker版本最新以获得安全补丁

## 获取帮助

如果遇到问题：

1. 查看日志：`cd docker && ./start.sh logs`
2. 检查Docker状态：`docker ps`
3. 查看容器详情：`docker inspect bookmarks`
4. 查看详细文档：`docker/README.md`

## 版本信息

- 版本：v1.5.0
- 发布日期：2024-01-20
- Docker支持：是

## 许可证

MIT License

---

**享受使用书签管理工具！** 🎉
