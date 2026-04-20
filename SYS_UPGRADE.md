# 系统升级功能说明

## 功能概述

系统升级功能负责管理应用程序的版本升级，包括数据库表结构变更和业务逻辑更新。所有升级 SQL 和业务逻辑都内嵌在 `app/logic/sys_update.go` 中。

## 核心特性

1. **版本追踪**：自动记录每次升级的状态和结果
2. **SQL 内嵌执行**：SQL 语句内嵌在 Go 代码中，不依赖外部 SQL 文件
3. **业务逻辑升级**：支持特定版本的业务逻辑处理
4. **跨版本升级**：支持一次性跨越多个版本的升级
5. **安全性**：升级失败不影响系统正常启动

## 数据库表结构

```sql
CREATE TABLE sys_update (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version TEXT NOT NULL,
    content TEXT NOT NULL,
    upgrade_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    result TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 0  -- 0: pending, 1: success, -1: failed
);
```

## 升级流程

1. 检查 `sys_update` 表是否存在，不存在则创建
2. 查询最后一次成功的升级记录
3. 如果没有升级记录，从 `v1.6.0` 开始
4. 获取所有大于起始版本且小于等于当前程序版本的待升级版本
5. 按版本号顺序依次执行每个版本的 SQL 和业务逻辑
6. 每个版本升级过程记录到 `sys_update` 表

## 版本升级详情

### v1.7.0

**SQL 变更：**
- 创建 `users` 表（用户系统）
- 创建 `users` 表索引和触发器
- `nodes` 表添加 `user_id` 字段
- 创建 `sys_config` 表（系统配置）
- 删除旧表 `config` 和 `version`

**业务逻辑：** 无特殊处理

### v1.8.0

**SQL 变更：** 无

**业务逻辑：** 初始化默认配置项 `allow_register=true`

### v1.9.0

**SQL 变更：**
- `users` 表添加 `api_key` 字段
- 创建 `api_key` 索引
- `nodes` 表添加 `remark` 字段

**业务逻辑：**
1. 为所有现有用户生成 `api_key`（32位随机十六进制字符串）
2. 重置所有用户密码为 `用户名+2026` 的双重 MD5 哈希

### v2.0.0

**SQL 变更：**
- 创建 `security_questions` 表（安全问题，用于密码重置）

**业务逻辑：** 无特殊处理

## 如何添加新的升级版本

### 1. 在 sys_update.go 中添加版本号

在 `availableVersions` 切片中添加新版本号：

```go
availableVersions := []string{"v1.7.0", "v1.8.0", "v1.9.0", "v2.0.0", "v2.1.0"}
```

### 2. 添加 SQL 语句

在 `getSQLStatements` 方法中添加新版本的 SQL：

```go
case "v2.1.0":
    return []string{
        "ALTER TABLE nodes ADD COLUMN new_field TEXT NOT NULL DEFAULT ''",
    }
```

### 3. （可选）添加业务逻辑

在 `executeBusinessLogic` 方法中添加新版本的业务逻辑：

```go
case "v2.1.0":
    return u.upgradeToV2_1_0()
```

### 4. 实现业务逻辑函数

```go
func (u *Upgrade) upgradeToV2_1_0() error {
    u.logger.Printf("执行 v2.1.0 业务逻辑升级")
    // 添加业务逻辑代码
    return nil
}
```

## 注意事项

1. 版本号必须遵循 `v{major}.{minor}.{patch}` 格式
2. SQL 语句应具有幂等性，多次执行不应产生副作用（使用 `IF NOT EXISTS` 等）
3. 业务逻辑升级函数应处理错误情况
4. 升级失败会记录错误但不会阻止系统启动
5. 所有 SQL 和业务逻辑都内嵌在 `sys_update.go` 中，不使用外部 SQL 文件
6. 升级按版本号顺序执行，确保依赖关系正确
