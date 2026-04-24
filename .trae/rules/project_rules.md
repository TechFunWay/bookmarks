# Bookmarks 项目开发规范

## 版本号管理

- 版本号定义在 `main.go` 的 `appVersion` 常量中
- 格式：`v{major}.{minor}.{patch}`，例如 `v2.0.0`
- 所有 skill 脚本从 `main.go` 自动获取版本号

## 代码风格

- Go 代码遵循标准 Go 风格
- 不添加注释（除非用户要求）
- 错误处理使用 `fmt.Errorf` 包装
- 数据库操作使用事务（`defer tx.Rollback()` 模式）

## 数据库变更

- 所有数据库 schema 变更必须通过 `sys_update.go` 中的升级逻辑执行
- SQL 语句使用 `IF NOT EXISTS` 确保幂等性
- 新版本升级步骤：
  1. 在 `availableVersions` 添加版本号
  2. 在 `getSQLStatements` 添加 SQL
  3. 在 `executeBusinessLogic` 添加业务逻辑（如需要）

## 编译

- 使用 program-compiler skill：`bash .trae/skills/program-compiler/scripts/compile.sh`
- Linux 平台使用 `CGO_ENABLED=0` 静态编译（Docker scratch 基础镜像需要）
- macOS/Windows 使用 `CGO_ENABLED=1`

## Docker 构建

- 使用 docker-builder skill：`bash .trae/skills/docker-builder/scripts/docker_builder.sh`
- Dockerfile 使用 `ARG VERSION` 和 `ARG TARGETARCH` 支持多架构
- 基于 scratch 基础镜像，只包含二进制文件
- 使用 Docker Buildx 离线构建多架构镜像

## FnApp 打包

- 使用 fnapp-packager skill：`bash .trae/skills/fnapp-packager/scripts/fnapp_packager.sh`

## 测试

```bash
go test ./...
```

## 运行

```bash
./bookmarks -dataUrl ./data -port 8901 -logmode release
```

## 密码重置

```bash
./reset-password -password newpassword
./reset-password -username admin -password newpassword
```
