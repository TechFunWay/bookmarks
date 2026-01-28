---
name: "fnapp-packager"
description: "飞牛应用打包工具，用于为 techfunway.bookmarks 应用创建适用于不同平台的 FnApp 安装包"
---

# 飞牛应用打包

## 功能说明

本技能用于为 techfunway.bookmarks 应用创建飞牛应用（FnApp）安装包，支持自动处理版本号、平台适配、文件复制、配置修改和应用打包等操作。

## 支持的平台

- Linux (amd64) - 生成 x86 架构安装包
- Linux (arm64) - 生成 arm 架构安装包

## 工作原理

1. **版本号获取**：从 `main.go` 文件中提取 `appVersion` 常量值
2. **平台打包**：分别处理 linux-amd64 和 linux-arm64 两个平台
3. **文件复制**：
   - 复制 release 目录下对应平台的可执行文件到 `techfunway.bookmarks/app/server/`
   - 复制 release 目录下对应平台的 static 目录到 `techfunway.bookmarks/app/server/`
4. **配置修改**：
   - 修改 `techfunway.bookmarks/manifest` 文件中的版本号
   - 根据打包平台修改 `platform` 字段（amd64 对应 x86，arm64 对应 arm）
   - 更新 `changelog` 字段内容
5. **应用打包**：
   - 在 `techfunway.bookmarks` 目录下执行 `fnpack build` 命令
   - 将生成的 `techfunway.bookmarks.fpk` 文件重命名为 `techfunway.bookmarks-v{版本号}-{架构}.fpk`

## 使用方法

### 基本命令

```bash
# 同时打包两个平台
./.trae/skills/fnapp-packager/scripts/fnapp_packager.sh

# 只打包 amd64 平台
./.trae/skills/fnapp-packager/scripts/fnapp_packager.sh amd64

# 只打包 arm64 平台
./.trae/skills/fnapp-packager/scripts/fnapp_packager.sh arm64
```

### 打包脚本

脚本文件：`.trae/skills/fnapp-packager/scripts/fnapp_packager.sh`

## 技术要点

1. **版本号管理**：从 `main.go` 自动提取版本号，确保打包版本与应用版本一致
2. **平台适配**：正确处理不同平台的文件路径和配置参数
3. **文件操作**：安全处理文件复制和目录创建，确保目标目录存在
4. **配置修改**：使用 `sed` 命令准确修改 `manifest` 文件中的特定字段
5. **错误处理**：添加适当的错误检查和日志输出，确保打包过程可追踪

## 注意事项

1. **依赖项**：
   - 需要安装 `fnpack` 工具（飞牛应用打包工具）
   - 需要已完成 Go 应用的交叉编译（生成 release 目录下的可执行文件）

2. **文件结构**：
   - 确保 `release` 目录下存在对应平台的编译产物
   - 确保 `techfunway.bookmarks/app/server/` 目录存在

3. **权限要求**：
   - 脚本需要有执行权限
   - 需要对 `techfunway.bookmarks` 目录有写权限

## 预期结果

执行打包脚本后，将在项目根目录生成以下文件：
- `techfunway.bookmarks-v{版本号}-amd64.fpk`（适用于 x86 平台）
- `techfunway.bookmarks-v{版本号}-arm64.fpk`（适用于 arm 平台）

这些文件可以直接上传到飞牛应用商店或分发给用户安装使用。