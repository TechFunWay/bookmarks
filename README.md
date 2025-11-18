# 网址收藏夹

一个简单易用的网址收藏夹管理工具，基于Go语言后端和Vue.js前端构建。

![Bookmark Manager](bookmarks.png)

## 功能特性

- 📁 **文件夹管理** - 创建、编辑、删除文件夹，支持嵌套结构
- 🔖 **书签管理** - 添加、编辑、删除网址书签，支持批量操作
- 🔄 **拖拽排序** - 支持文件夹和书签的拖拽重排序
- 📦 **批量操作** - 批量选择、删除和移动书签
- 🌐 **元数据获取** - 自动获取网页标题和favicon图标
- 🎨 **现代化界面** - 简洁美观的用户界面
- 📱 **响应式设计** - 适配不同屏幕尺寸
- ⚡ **快速响应** - 基于SQLite本地数据库，查询速度快
- 🔒 **安全可靠** - 支持内网HTTPS站点访问

## 技术架构

### 后端技术栈
- **Go 1.21+** - 主要编程语言
- **Chi** - 轻量级HTTP路由器
- **SQLite** - 本地数据库存储
- **Go Modules** - 依赖管理

### 前端技术栈
- **Vue.js 3** - 现代JavaScript框架
- **HTML5/CSS3** - 页面布局和样式
- **Fetch API** - HTTP请求处理

### 数据存储
- SQLite数据库文件：`data.db`
- 支持外键约束和数据一致性
- 自动维护排序位置

## 快速开始

### 系统要求
- Go 1.21 或更高版本
- 支持SQLite的操作系统（Windows/macOS/Linux）

### 安装步骤

1. **克隆或下载项目**
   ```bash
   git clone <项目地址>
   cd bookmarks
   ```

2. **运行应用**
   ```bash
   go run main.go
   ```

3. **访问应用**
   - 打开浏览器访问：http://localhost:8901
   - 应用将在8901端口启动

### 自定义数据路径
```bash
go run main.go -dataUrl=/path/to/your/data/
```

## 使用说明

### 基础操作

1. **创建文件夹**
   - 点击左侧"新建文件夹"按钮
   - 或右键点击现有文件夹选择"新建子文件夹"

2. **添加书签**
   - 点击左侧"添加网址"按钮
   - 或右键点击文件夹选择"添加网址"
   - 输入网址后点击"获取信息"可自动填充标题和图标

3. **编辑项目**
   - 双击文件夹或书签名称进行编辑
   - 或右键选择"编辑"选项

4. **删除项目**
   - 右键选择"删除"选项
   - 或在编辑模式下批量选择删除

### 高级功能

1. **重排序**
   - 拖拽文件夹或书签到新位置
   - 或使用右键菜单的"上移"/"下移"选项

2. **移动到文件夹**
   - 右键选择"移动到文件夹"
   - 选择目标文件夹并确认

3. **批量操作**
   - 点击"编辑"按钮进入编辑模式
   - 使用复选框选择多个项目
   - 执行批量删除或移动操作

4. **搜索和组织**
   - 通过文件夹结构组织书签
   - 查看书签路径信息

### 快捷操作

- **右键菜单**：在任何项目上右键查看可用操作
- **双击编辑**：双击项目名称快速编辑
- **拖拽移动**：直接拖拽到目标位置
- **批量选择**：使用编辑模式进行批量操作

## API接口

### RESTful API

- `GET /api/tree` - 获取完整树结构
- `GET /api/metadata?url=<网址>` - 获取网页元数据
- `POST /api/folders` - 创建文件夹
- `POST /api/bookmarks` - 创建书签
- `PUT /api/nodes/{id}` - 更新节点
- `DELETE /api/nodes/{id}` - 删除节点
- `POST /api/nodes/reorder` - 重新排序节点

### 请求示例

```bash
# 获取树结构
curl http://localhost:8901/api/tree

# 创建文件夹
curl -X POST http://localhost:8901/api/folders \
  -H "Content-Type: application/json" \
  -d '{"title":"工作相关","parent_id":null}'

# 创建书签
curl -X POST http://localhost:8901/api/bookmarks \
  -H "Content-Type: application/json" \
  -d '{"title":"GitHub","url":"https://github.com","parent_id":1}'
```

## 配置说明

### 数据库配置
- 数据库文件位置：可通过`-dataUrl`参数指定
- 默认位置：当前目录下的`data.db`
- 自动创建必要的表和索引

### 服务器配置
- 默认端口：8901
- 支持静态文件服务
- 内置CORS支持

## 目录结构

```
bookmarks/
├── main.go              # 主程序入口
├── go.mod               # Go模块定义
├── go.sum               # 依赖校验
├── data.db              # SQLite数据库文件（运行时生成）
├── static/              # 静态文件目录
│   ├── index.html       # 主页面
│   ├── app.js          # Vue.js应用
│   └── style.css       # 样式文件
├── README.md           # 中文说明
├── README.en.md        # English README
└── techfunway.bookmarks/ # 打包相关文件
```

## 开发说明

### 环境要求
- Go 1.21+
- 现代浏览器支持（Chrome、Firefox、Safari、Edge）

### 开发运行
```bash
# 启动开发服务器
go run main.go

# 修改前端代码后刷新浏览器即可看到效果
```

### 部署建议
- 可以打包成单文件可执行程序
- 支持在任何支持Go的平台上运行
- 数据文件可与程序分离，便于备份

## 故障排除

### 常见问题

1. **端口被占用**
   - 检查8901端口是否被其他程序占用
   - 或修改代码中的端口配置

2. **数据库错误**
   - 确保有写入权限
   - 检查磁盘空间是否充足

3. **网页元数据获取失败**
   - 检查网络连接
   - 某些网站可能限制爬虫访问

4. **HTTPS站点访问失败**
   - 应用支持自签名证书
   - 内网HTTPS站点可正常访问

### 日志查看
- 启动时会显示服务器运行端口
- 浏览器开发者工具查看前端错误
- 服务器日志显示API请求和错误信息

## 许可证

本项目采用MIT许可证，详见LICENSE文件。

## 参与贡献

欢迎提交Issue和Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 更新日志

### v1.0.0
- 初始版本发布
- 基础文件夹和书签管理功能
- 拖拽排序支持
- 批量操作功能
- 网页元数据获取
