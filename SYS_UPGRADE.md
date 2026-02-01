# 系统升级功能说明

## 功能概述

系统升级功能负责管理应用程序的版本升级，包括数据库表结构变更和业务逻辑更新。

## 核心特性

1. **版本追踪**：自动记录每次升级的状态和结果
2. **SQL脚本执行**：自动执行SQL目录下的版本脚本
3. **业务逻辑升级**：支持特定版本的业务逻辑处理
4. **跨版本升级**：支持一次性跨越多个版本的升级
5. **安全性**：升级失败不影响系统正常启动

## 目录结构

```
project/
├── sql/                    # SQL升级脚本目录
│   ├── v1.5.0.sql        # 版本1.5.0的升级脚本
│   ├── v1.7.0.sql        # 版本1.7.0的升级脚本
│   ├── v1.8.0.sql        # 版本1.8.0的升级脚本
│   └── v2.0.0.sql        # 版本2.0.0的升级脚本
└── app/
    └── logic/
        └── sys_update.go # 升级功能主文件
```

## 数据库表结构

```sql
CREATE TABLE sys_update (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version TEXT NOT NULL,              -- 版本号
    content TEXT NOT NULL,              -- 升级内容描述
    upgrade_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,  -- 升级时间
    result TEXT NOT NULL,               -- 升级结果
    status TEXT NOT NULL DEFAULT 'pending'  -- 升级状态 (pending, success, failed)
);
```

## 升级流程

1. 检查`sys_update`表是否存在，不存在则创建
2. 查询最后一次成功的升级记录
3. 从该版本开始，执行到当前版本之间的所有升级
4. 按版本号顺序依次执行SQL脚本和业务逻辑
5. 记录每次升级的状态和结果

## 如何添加新的升级版本

### 1. 创建SQL脚本

在`sql/`目录下创建以版本号命名的SQL文件，例如：

```sql
-- sql/v2.1.0.sql
-- 在这里添加你的SQL语句
CREATE TABLE new_table (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

ALTER TABLE existing_table ADD COLUMN new_column TEXT;
```

### 2. （可选）添加业务逻辑

在`sys_update.go`文件中，可以在`executeBusinessLogic`方法中为特定版本添加业务逻辑：

```go
func (u *Upgrade) executeBusinessLogic(version string) error {
    switch version {
    case "v2.1.0":
        return u.upgradeToV2_1_0() // 添加新的处理函数
    // ... 其他版本
    default:
        u.logger.Printf("执行通用升级逻辑，版本: %s", version)
        return nil
    }
}
```

## API接口

升级状态可以通过以下方式获取：

```go
upgrader := NewUpgrade(db, appVersion)
status := upgrader.GetStatus() // 获取升级状态
```

## 注意事项

1. SQL文件名必须遵循`v{major}.{minor}.{patch}.sql`格式
2. 版本号必须为三段式（如v1.7.0）
3. SQL语句应具有幂等性，多次执行不应产生副作用
4. 业务逻辑升级函数应处理错误情况
5. 升级失败会记录错误但不会阻止系统启动