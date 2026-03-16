---
name: "fnapp-packager-v2"
description: "飞牛应用打包工具 v2，用于创建 FnApp 安装包，支持多平台、自动版本管理、配置优化和构建验证。"
---

# 飞牛应用打包工具 v2

## 功能说明

本技能用于将编译好的 Go 应用打包成飞牛应用（FnApp）安装包，支持自动版本管理、平台适配、配置优化和完整性验证。

## 支持的平台

| 平台 | 架构 | 输出文件 | 适用场景 |
|-----|-------|----------|---------|
| Linux | amd64 | `techfunway.bookmarks-v{版本}-amd64.fpk` | x86 架构 NAS 和服务器 |
| Linux | arm64 | `techfunway.bookmarks-v{版本}-arm64.fpk` | ARM 架构 NAS 和服务器（飞牛） |

## 工作原理

### 完整流程

1. **版本号获取**：从 `main.go` 自动提取 `appVersion` 常量
2. **编译产物验证**：检查 release 目录下对应平台的可执行文件是否存在
3. **目录准备**：
   - 创建/清理 `techfunway.bookmarks/app/server/` 目录
   - 清理系统文件（.DS_Store、._*、Thumbs.db）
4. **文件复制**：
   - 复制可执行文件到 `app/server/`
   - 复制 static 目录（排除 icons）
5. **配置修改**：
   - 更新 `manifest` 文件的版本号
   - 设置正确的 platform 字段（x86/arm）
   - 更新 changelog
6. **应用打包**：
   - 使用 `fnpack build` 命令打包
   - 验证生成的 fpk 文件
7. **文件整理**：
   - 重命名为规范的文件名
   - 移动到 release 目录
   - 清理临时文件

## 使用方法

### 基本用法

```bash
# 打包所有支持的平台
.trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh

# 只打包 amd64 平台
.trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh amd64

# 只打包 arm64 平台
.trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh arm64

# 非交互模式（自动使用默认配置）
.trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh --auto
```

### 快捷方式

```bash
# 打包飞牛需要的两个平台
.trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh amd64 arm64
```

## 前置条件

### 1. 编译完成

确保已运行 Go 编译脚本：

```bash
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 linux-arm64
```

### 2. 安装 fnpack

```bash
# 检查 fnpack 是否安装
which fnpack

# 如果未安装，需要从飞牛官方获取
```

### 3. 文件结构

确保项目目录结构正确：

```
bookmarks/
├── main.go                          # 主程序（包含版本号）
├── techfunway.bookmarks/             # 飞牛应用目录
│   ├── manifest                     # 应用清单文件
│   ├── app/
│   │   └── server/                 # 服务端文件目录
│   └── ...
├── release/                        # 编译输出目录
│   ├── bookmarks-v1.9.0-linux-amd64/
│   │   ├── bookmarks
│   │   └── static/
│   └── bookmarks-v1.9.0-linux-arm64/
│       └── ...
└── .trae/skills/fnapp-packager-v2/
    └── scripts/fnapp_packager.sh
```

## 输出结果

打包完成后，在 `release/` 目录生成：

```
release/
├── techfunway.bookmarks-v1.9.0-amd64.fpk   # x86 架构安装包
└── techfunway.bookmarks-v1.9.0-arm64.fpk   # ARM 架构安装包
```

## 技术要点

### 1. 版本号自动提取

```bash
VERSION=$(awk -F'"' '/appVersion = "[^"]+"/ {print $2}' main.go)
```

- 精确提取，支持带 `v` 前缀的版本号
- 自动去除 `v` 前缀

### 2. Manifest 配置优化

```ini
[app]
name         = "网址收藏夹"
version       = 1.9.0
platform      = x86  ; 或 arm
author        = "TechFunWay"
icon          = "icon.png"
description   = "书签管理应用"
changelog     = "更新内容..."

[service]
name          = "bookmarks"
exec          = "./app/server/bookmarks"
type          = "binary"
auto_start    = true
auto_restart  = true
```

### 3. 系统文件清理

自动清理以下系统文件：

| 系统 | 文件类型 | 说明 |
|-----|---------|------|
| macOS | `.DS_Store` | Finder 元数据 |
| macOS | `._*` | 资源分支文件 |
| Windows | `Thumbs.db` | 缩略图缓存 |
| Windows | `desktop.ini` | 桌面配置 |

清理时机：
1. 复制文件前：清理目标目录
2. 打包前：清理整个应用目录

### 4. .fnpackignore 配置

创建 `.fnpackignore` 文件排除不必要的文件：

```
# Git 文件
.git/
.gitignore
.gitattributes

# IDE 文件
.vscode/
.idea/
*.swp
*.swo

# 系统文件
.DS_Store
._*
Thumbs.db
desktop.ini

# 临时文件
*.tmp
*.log
*.bak

# 开发文件
test/
tests/
*.test
```

## 验证清单

打包完成后，检查以下项目：

- [ ] fpk 文件已生成在 release 目录
- [ ] 文件名包含正确的版本号
- [ ] 文件名包含正确的架构标识（amd64/arm64）
- [ ] manifest 文件版本号正确
- [ ] manifest 文件 platform 字段正确
- [ ] 可执行文件有执行权限
- [ ] static 资源文件已复制
- [ ] 没有系统文件混入

## 常见问题

### Q1: 提示"可执行文件不存在"

**A**: 先运行编译脚本：

```bash
.trae/skills/go-multi-platform-compiler/scripts/compile.sh linux-amd64 linux-arm64
```

### Q2: 提示"fnpack 命令未找到"

**A**: 需要安装飞牛应用打包工具：

```bash
# 从飞牛官方获取安装包
# 或使用包管理器安装（如果有）
```

### Q3: 打包后安装失败

**A**: 检查 manifest 文件配置：

```bash
# 查看生成的 manifest
cat techfunway.bookmarks/manifest

# 检查关键字段
grep -E "^version|^platform|^name" techfunway.bookmarks/manifest
```

### Q4: fpk 文件很大

**A**: 检查是否包含了不必要的文件：

```bash
# 查看文件大小
ls -lh release/*.fpk

# 检查 .fnpackignore 是否生效
cat techfunway.bookmarks/.fnpackignore
```

## 集成到 CI/CD

### GitHub Actions 示例

```yaml
name: Package FnApp

on:
  workflow_run:
    workflows: ["Build"]
    types:
      - completed

jobs:
  package:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Download artifacts
        uses: actions/download-artifact@v3
        with:
          path: release/

      - name: Package FnApp
        run: |
          .trae/skills/fnapp-packager-v2/scripts/fnapp_packager.sh amd64 arm64

      - name: Upload FPK files
        uses: actions/upload-artifact@v3
        with:
          name: fnapp-packages
          path: release/*.fpk
```

## 发布流程建议

1. **编译**：使用 `go-multi-platform-compiler` 编译所有平台
2. **测试**：在每个平台测试可执行文件
3. **打包**：使用本脚本打包 FPK 文件
4. **验证**：在飞牛测试安装包
5. **发布**：上传到应用商店或分发渠道

## 相关技能

- **go-multi-platform-compiler**: 多平台 Go 编译
- **docker-builder**: Docker 多架构镜像构建

使用这三个技能可以完成从编译到发布的完整自动化流程。

## 更新日志

### v2.0 (2026-03-16)
- ✨ 重新设计，改进文档和脚本结构
- ✨ 增强错误处理和日志输出
- ✨ 添加完整性验证清单
- ✨ 优化系统文件清理机制
- ✨ 改进版本号提取逻辑
- 🐛 修复 manifest 配置问题
- 📝 完善使用文档和故障排查
