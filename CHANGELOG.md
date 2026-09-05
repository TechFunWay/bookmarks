# 📋 更新日志 (Changelog)

所有版本的详细更新记录。每个版本包含下载链接和部署配置。

> 📦 **下载地址**：[Gitee Releases](https://gitee.com/TechFunWay/bookmarks/releases) | [GitHub Releases](https://github.com/TechFunWay/bookmarks/releases)
> 🐳 **Docker 镜像**：`techfunways/bookmarks:latest`

---

## 📌 最新版本：v3.3.0

> 本版本汇总自 v3.2.0 之后的全部功能与修复。

### 🚀 快速开始

**方式一：下载安装包**
- 📥 [前往 Gitee Releases 下载 v3.3.0](https://gitee.com/TechFunWay/bookmarks/releases/tag/v3.3.0)

**方式二：Docker 一键部署**
```bash
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks
docker-compose up -d
```

### 📥 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v3.3.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v3.3.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v3.3.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v3.3.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v3.3.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v3.3.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.3.0/bookmarks-v3.3.0-windows-arm64.tar.gz) |

### ✨ 更新内容

**链接检测更准确**
- 能区分「确定失效」和「可能无法访问」的链接，网络超时、DNS、证书等错误细分归类，不再直接当作失效。
- 修复跳转链接和部分限制普通请求的网站被误判为失效的问题。
- 遇到 403 拦截页（如 Cloudflare 防护）自动用无头浏览器复检，大幅降低误判。
- 「清理失效」新增按 HTTP 状态码分组视图，批量处理失效书签更直观。

**去重复更灵活**
- 去重复页支持自定义判定规则，并支持勾选批量删除，清理相似书签更省心。

**飞牛 NAS 一键登录**
- 支持从飞牛桌面一键登录并绑定账号，无需手动输入密码。
- 修复应用端口入口识别，桌面图标直接打开正确访问地址。
- 飞牛登录入口界面焕新。

**☕ 应用内赞赏支持**
- 管理后台新增「请作者喝杯咖啡」：完全免费、无广告，赞赏自愿、金额随意（1 元也是心意），不支付不影响任何功能。
- 应用每次启动后进入控制台弹出赞赏窗口：点【暂不支持】本次运行内不再弹出（刷新页面不重复弹），重启应用后再次提示；点【已支持】发送成功后永不再提示，发送失败则继续提示；顶部横幅同步保留。
- 匿名计数与设备统计改进：设备唯一标识升级为系统机器标识（容器重建、卸载重装后身份稳定），不再上报主机名，支持按设备类型/系统/架构统计装机分布。

**🎨 全新 3D 玻璃质感图标**
- 全新立体玻璃风格应用图标：飞牛应用中心、桌面入口、浏览器扩展、网页 favicon、PWA 图标全端统一换新，小尺寸下依然清晰。

**低配设备使用更流畅**
- 优化大量书签打开时的加载、图标请求和列表渲染，低配置 NAS、旧电脑和手机上滚动、管理书签更顺畅。
- 添加书签不再等待网页信息抓取，先立即保存，再自动补全标题和图标。
- 后台页面改用本地字体，不再等待远程字体加载。

**手机端体验**
- 补齐添加到手机桌面的应用图标，作为快捷方式打开时显示更完整。

---

## v3.0.0

### 🚀 快速开始

**方式一：下载安装包**
- 📥 [前往 Gitee Releases 下载 v3.0.0](https://gitee.com/TechFunWay/bookmarks/releases/tag/v3.0.0)
- 📥 [前往 GitHub Releases 下载 v3.0.0](https://github.com/TechFunWay/bookmarks/releases/tag/v3.0.0)

**方式二：Docker 一键部署**
```bash
# 克隆项目 (Gitee)
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 或者 (GitHub)
# git clone https://github.com/TechFunWay/bookmarks.git
# cd bookmarks

# 启动服务
docker-compose up -d

# 访问应用
open http://localhost:8901
```

### 📥 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v3.0.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v3.0.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v3.0.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v3.0.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v3.0.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v3.0.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-windows-arm64.tar.gz) |
| Docker | multiarch | [bookmarks-v3.0.0-docker-multiarch.tar](https://gitee.com/TechFunWay/bookmarks/releases/download/v3.0.0/bookmarks-v3.0.0-docker-multiarch.tar) |

### 🐳 Docker 部署配置

```bash
# 克隆项目
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 使用项目根目录的 docker-compose.yaml 启动
docker-compose up -d
```

### ✨ 更新内容

- 全新版本，包含最新功能和优化

---

## v2.2.0


### 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v2.2.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v2.2.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v2.2.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v2.2.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v2.2.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v2.2.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.2.0/bookmarks-v2.2.0-windows-arm64.tar.gz) |

### 🐳 Docker 部署配置

```bash
# 克隆项目
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 使用项目根目录的 docker-compose.yaml 启动
docker-compose up -d
```

### 更新内容

- 新增功能和性能优化

---

## v2.1.0


### 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v2.1.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v2.1.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v2.1.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v2.1.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v2.1.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v2.1.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.1.0/bookmarks-v2.1.0-windows-arm64.tar.gz) |

### 🐳 Docker 部署配置

```bash
# 克隆项目
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 使用项目根目录的 docker-compose.yaml 启动
docker-compose up -d
```

### 更新内容

- 更新：浏览器同步插件支持同时同步多个设备，每个设备独立配置设置
- 新增：免登录功能（开启免登录后实际使用的是管理员数据）
- 新增：新增分类显示页面，支持滑动缩放图标大小

---

## v2.0.0


### 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v2.0.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v2.0.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v2.0.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v2.0.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v2.0.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v2.0.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v2.0.0/bookmarks-v2.0.0-windows-arm64.tar.gz) |

### 🐳 Docker 部署配置

```bash
# 克隆项目
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 使用项目根目录的 docker-compose.yaml 启动
docker-compose up -d
```

### 更新内容

- 全新架构重构
- 新增多用户支持
- 新增用户认证系统（注册/登录/登出）
- 新增管理员用户管理功能
- 新增安全问题密码重置
- 新增 API Key 认证（支持浏览器扩展同步）
- 新增系统自动升级功能
- 新增书签备注功能
- 优化数据库结构和性能

---

## v1.9.0


### 下载链接

| 平台 | 架构 | 下载地址 |
|------|------|----------|
| Linux | amd64 | [bookmarks-v1.9.0-linux-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-linux-amd64.tar.gz) |
| Linux | arm64 | [bookmarks-v1.9.0-linux-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-linux-arm64.tar.gz) |
| macOS | amd64 | [bookmarks-v1.9.0-macos-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-macos-amd64.tar.gz) |
| macOS | arm64 | [bookmarks-v1.9.0-macos-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-macos-arm64.tar.gz) |
| Windows | amd64 | [bookmarks-v1.9.0-windows-amd64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-windows-amd64.tar.gz) |
| Windows | arm64 | [bookmarks-v1.9.0-windows-arm64.tar.gz](https://gitee.com/TechFunWay/bookmarks/releases/download/v1.9.0/bookmarks-v1.9.0-windows-arm64.tar.gz) |

### 🐳 Docker 部署配置

```bash
# 克隆项目
git clone https://gitee.com/TechFunWay/bookmarks.git
cd bookmarks

# 使用项目根目录的 docker-compose.yaml 启动
docker-compose up -d
```

### 更新内容

- 性能优化和bug修复

---

## v1.4.0


### 下载链接

请从 [Gitee Releases](https://gitee.com/TechFunWay/bookmarks/releases) 或 [GitHub Releases](https://github.com/TechFunWay/bookmarks/releases) 页面下载对应版本。

### 更新内容

- 新增背景设置功能，支持默认背景、纯色背景、自定义图片背景
- 新增面板透明度调节功能，可调节列表和结构面板的透明度
- 新增每页显示数量保存功能，设置自动保存到数据库，跨浏览器同步
- 优化浅色主题下选中数字的显示效果
- 调整PC端列表布局高度，解决内容超出问题
- 性能优化和bug修复

---

## v1.2.0


### 下载链接

请从 [Gitee Releases](https://gitee.com/TechFunWay/bookmarks/releases) 或 [GitHub Releases](https://github.com/TechFunWay/bookmarks/releases) 页面下载对应版本。

### 更新内容

- 新增导入导出功能，支持JSON格式书签的导入和导出
- 新增Edge浏览器支持，支持导入和导出Edge浏览器的HTML格式书签
- 新增主题切换功能，支持浅色和深色主题切换
- 优化了文件夹选择功能，修复了导入时无法选择文件夹的问题
- 修复了交叉编译问题，支持生成不同架构的Linux二进制文件
- 性能优化和bug修复

---

## v1.1.0


### 下载链接

请从 [Gitee Releases](https://gitee.com/TechFunWay/bookmarks/releases) 或 [GitHub Releases](https://github.com/TechFunWay/bookmarks/releases) 页面下载对应版本。

### 更新内容

- 新增搜索功能，支持按网址名称搜索
- 手机端适配优化
- 性能优化和bug修复

---

## v1.0.0


### 下载链接

请从 [Gitee Releases](https://gitee.com/TechFunWay/bookmarks/releases) 或 [GitHub Releases](https://github.com/TechFunWay/bookmarks/releases) 页面下载对应版本。

### 更新内容

- 初始版本发布
- 基础文件夹和书签管理功能
- 拖拽排序支持
- 批量操作功能
- 网页元数据获取
- 响应式设计，适配不同屏幕尺寸
- 现代化美观界面
- 支持内网HTTPS站点访问
