package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"bookmark/app/logger"
	"bookmark/app/logic"
	"bookmark/app/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

//go:embed static
var staticFS embed.FS

const (
	nodeTypeFolder   = "folder"
	nodeTypeBookmark = "bookmark"

	// 应用版本
	appVersion = "v1.9.0"

	// 日志模式常量
	logModeDebug   = "debug"
	logModeRelease = "release"
	defaultLogMode = logModeRelease
)

type server struct {
	db          *sql.DB
	httpClient  *http.Client
	faviconChan chan int64 // 图标获取任务队列
	iconPath    string     // 图标存储路径
}

// 全局配置
var (
	logMode string
)

// Debug 调试日志函数，仅在debug模式下打印
func Debug(format string, v ...interface{}) {
	if logMode == logModeDebug {
		log.Printf(format, v...)
	}
}

// Error 错误日志函数，在所有模式下都打印
func Error(format string, v ...interface{}) {
	log.Printf(format, v...)
}

type node struct {
	ID            int64   `json:"id"`
	ParentID      *int64  `json:"parent_id"`
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	URL           *string `json:"url,omitempty"`
	FaviconURL    *string `json:"favicon_url,omitempty"`
	Remark        string  `json:"remark,omitempty"`
	Position      int     `json:"position"`
	Children      []*node `json:"children,omitempty"`
	BookmarkCount int     `json:"bookmark_count,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
	UpdatedAt     string  `json:"updated_at,omitempty"`
}

type user struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Nickname  string  `json:"nickname"`
	Email     string  `json:"email"`
	Avatar    *string `json:"avatar"`
	IsActive  bool    `json:"is_active"`
	IsAdmin   bool    `json:"is_admin"`
	APIKey    *string `json:"api_key,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname,omitempty"`
	Email    string `json:"email,omitempty"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type authResponse struct {
	Token string `json:"token"`
	User  *user  `json:"user"`
}

// 用户上下文键
type contextKey string

const (
	userContextKey contextKey = "user"
)

func main() {
	dataUrl := flag.String("dataUrl", "./data", "数据存储路径")                          // 定义字符串参数
	port := flag.String("port", "8901", "服务器监听端口")                                 // 定义端口参数
	logModeFlag := flag.String("logmode", defaultLogMode, "日志模式: debug 或 release") // 日志模式参数
	flag.Parse()

	// 初始化日志模式：先检查命令行参数，再检查环境变量，最后使用默认值
	logMode = *logModeFlag
	if envLogMode := os.Getenv("LOG_MODE"); envLogMode != "" {
		logMode = envLogMode
	}

	// 计算图标路径：基于dataUrl，处理结尾斜杠
	dataPath := *dataUrl
	if !strings.HasSuffix(dataPath, "/") {
		dataPath += "/"
	}
	iconPath := dataPath + "icons/"
	dbPath := dataPath + "db/"
	logPath := dataPath + "logs/"

	// 验证日志模式
	if logMode != logModeDebug && logMode != logModeRelease {
		log.Fatalf("无效的日志模式: %s, 必须是 debug 或 release", logMode)
	}
	fmt.Println("数据路径:", *dataUrl)
	fmt.Println("监听端口:", *port)
	fmt.Println("图标路径:", iconPath)
	fmt.Println("数据库路径:", dbPath)
	fmt.Println("日志路径:", logPath)
	// 创建数据目录
	if _, err := os.Stat(*dataUrl); os.IsNotExist(err) {
		if err := os.Mkdir(*dataUrl, 0755); err != nil {
			log.Fatalf("failed to create data directory: %v", err)
		}
	}

	// 迁移旧图标：从 static/icons 移动到新路径
	migrateOldIcons("./static/icons", iconPath)

	// 确保图标存储目录存在（如果迁移失败或跳过）
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		if err := os.MkdirAll(iconPath, 0755); err != nil {
			log.Fatalf("failed to create icons directory: %v", err)
		}
	}

	// 创建数据库目录
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dbPath, 0755); err != nil {
			log.Fatalf("failed to create db directory: %v", err)
		}
	}

	// 创建日志目录
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		if err := os.MkdirAll(logPath, 0755); err != nil {
			log.Fatalf("failed to create logs directory: %v", err)
		}
	}

	oldPath := "./"
	if dataPath != "./data/" {
		oldPath = dataPath
	}

	// 迁移旧数据库：从 ./data.db 迁移到新路径并改名为 database.db
	migrateOldDatabase(oldPath, dbPath, "data.db", "database.db")

	db, err := sql.Open("sqlite", dbPath+"database.db?_foreign_keys=on")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	// 初始化数据库
	if err := initializeDB(db); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	// 执行系统升级
	upgrader := logic.NewUpgrade(db, appVersion, logPath)
	if err := upgrader.PerformUpgrade(); err != nil {
		log.Printf("系统升级失败: %v", err)
	}

	// 创建支持自签名证书的HTTP客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// 允许自签名证书和不安全的TLS连接（主要用于内网环境）
			InsecureSkipVerify: true,
		},
	}

	s := &server{
		db: db,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// 允许最多10次重定向
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		faviconChan: make(chan int64, 100), // 缓冲队列，最多100个待处理任务
		iconPath:    iconPath,              // 设置图标路径
	}

	// 启动图标获取协程
	go s.faviconWorker()

	// 创建日志文件
	logFile, err := logger.CreateLogFile(logPath)
	if err != nil {
		log.Fatalf("failed to create log file: %v", err)
	}
	defer logFile.Close()

	r := chi.NewRouter()
	// 使用自定义日志中间件而不是默认的middleware.Logger
	r.Use(logger.LoggingMiddleware(logFile))
	r.Use(middleware.Recoverer)
	r.Use(middleware.AllowContentType("application/json", "text/plain", "application/x-www-form-urlencoded"))
	r.Use(corsMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Post("/change-password", s.tokenAuthMiddleware(s.handleChangePassword))
			r.Post("/regenerate-api-key", s.tokenAuthMiddleware(s.handleRegenerateAPIKey))
			r.Get("/me", s.tokenAuthMiddleware(s.handleGetCurrentUser))
			r.Get("/check", s.handleCheckAuth)
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/", s.tokenAuthMiddleware(s.adminMiddleware(s.handleGetUsers)))
			r.Get("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleGetUser)))
			r.Put("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleUpdateUser)))
			r.Delete("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleDeleteUser)))
			r.Post("/{id}/reset-password", s.tokenAuthMiddleware(s.adminMiddleware(s.handleResetPassword)))
			r.Post("/batch", s.tokenAuthMiddleware(s.adminMiddleware(s.handleBatchUsers)))
		})
		r.Get("/tree", s.tokenAuthMiddleware(s.handleGetTree))
		r.Get("/metadata", s.handleMetadata)
		r.Get("/version", s.handleGetVersion)
		r.Post("/folders", s.tokenAuthMiddleware(s.handleCreateFolder))
		r.Post("/bookmarks", s.tokenAuthMiddleware(s.handleCreateBookmark))
		r.Put("/nodes/{id}", s.tokenAuthMiddleware(s.handleUpdateNode))
		r.Delete("/nodes/{id}", s.tokenAuthMiddleware(s.handleDeleteNode))
		r.Post("/nodes/batch-delete", s.tokenAuthMiddleware(s.handleBatchDeleteNodes))
		r.Post("/nodes/reorder", s.tokenAuthMiddleware(s.handleReorderNodes))
		r.Post("/import", s.tokenAuthMiddleware(s.handleImport))
		r.Post("/import-edge", s.tokenAuthMiddleware(s.handleEdgeImport))
		r.Get("/config/system", s.handleGetSystemConfig)
		r.Get("/config", s.tokenAuthMiddleware(s.handleGetConfig))
		r.Post("/config", s.tokenAuthMiddleware(s.handleUpdateConfig))
	})

	// 浏览器书签同步接口（使用 API Key 认证）
	r.Route("/api/sync", func(r chi.Router) {
		r.Use(s.apiKeyAuthMiddlewareForChi)

		r.Get("/bookmarks", s.handleSyncGetBookmarks)
		r.Post("/bookmarks", s.handleSyncCreateBookmark)
		r.Put("/bookmarks/{id}", s.handleSyncUpdateBookmark)
		r.Delete("/bookmarks/{id}", s.handleSyncDeleteBookmark)

		r.Get("/folders", s.handleSyncGetFolders)
		r.Post("/folders", s.handleSyncCreateFolder)
		r.Put("/folders/{id}", s.handleSyncUpdateFolder)
		r.Delete("/folders/{id}", s.handleSyncDeleteFolder)

		r.Post("/batch", s.handleSyncBatchOperation)

		// 应用→浏览器方向：返回完整树形结构供插件拉取
		r.Get("/tree", s.handleSyncGetTree)
	})

	// 使用嵌入的静态文件系统
	staticFiles, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to create static filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(staticFiles))
	r.Handle("/*", fileServer)
	r.Handle("/static/*", http.StripPrefix("/static", fileServer))

	// 添加图标路径的静态文件服务
	iconFileServer := http.FileServer(http.Dir(iconPath))
	r.Handle("/icons/*", http.StripPrefix("/icons", iconFileServer))

	addr := ":" + *port
	Debug("Bookmark server running on %s", addr)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func initializeDB(db *sql.DB) error {
	var tableExists int
	err := db.QueryRow("SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='nodes'").Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("检查表存在失败: %w", err)
	}

	if tableExists == 0 {
		log.Println("数据库表不存在，开始初始化")

		if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
			log.Println("启用外键约束失败: %w", err)
		}

		if _, err := db.Exec(`
		-- 创建nodes表
		CREATE TABLE IF NOT EXISTS nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL DEFAULT 0,
    parent_id INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('folder', 'bookmark')),
    title TEXT NOT NULL,
    url TEXT,
    favicon_url TEXT,
    remark TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`); err != nil {
			log.Println("创建nodes表失败: %w", err)
		}

		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_parent_position ON nodes(parent_id, position);
CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);
CREATE INDEX IF NOT EXISTS idx_nodes_user_id_parent ON nodes(user_id, parent_id);`); err != nil {
			log.Println("创建nodes表索引失败: %w", err)
		}

		if _, err := db.Exec(string(`-- 创建nodes表的updated_at触发器
CREATE TRIGGER IF NOT EXISTS trg_nodes_updated_at
AFTER UPDATE ON nodes
BEGIN
    UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;`)); err != nil {
			log.Println("创建nodes表updated_at触发器失败: %w", err)
		}

		log.Println("数据库初始化成功")
	} else {
		log.Println("数据库表已存在，跳过初始化")
	}

	return nil
}

func (s *server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	nodes, err := s.loadTree(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, nodes)
}

func (s *server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		respondError(w, http.StatusBadRequest, errors.New("missing url parameter"))
		return
	}

	// 处理双重URL编码
	targetURL := rawURL
	if strings.Contains(rawURL, "%253A") || strings.Contains(rawURL, "%2F") {
		// 双重编码检测，尝试解码两次
		if decoded, err := url.QueryUnescape(rawURL); err == nil {
			if strings.Contains(decoded, "%3A") || strings.Contains(decoded, "%2F") {
				// 可能还是编码的，再次解码
				if doubleDecoded, err := url.QueryUnescape(decoded); err == nil {
					targetURL = doubleDecoded
				} else {
					targetURL = decoded
				}
			} else {
				targetURL = decoded
			}
		}
	}

	normalized, err := normalizeURL(targetURL)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid url: %w", err))
		return
	}

	// 特殊处理内网地址
	normalized = handleIntranetURL(normalized)

	title, icon, err := s.fetchMetadata(normalized)
	if err != nil {
		respondError(w, http.StatusBadGateway, fmt.Errorf("metadata fetch failed: %w", err))
		return
	}

	// 下载并保存图标到本地文件
	var savedIcon string
	if icon != "" {
		savedIcon, err = s.downloadAndSaveIcon(icon, s.iconPath)
		if err != nil {
			Debug("下载并保存图标失败: %v, 使用原始URL", err)
			savedIcon = icon // 保存失败时使用原始URL
		} else {
			Debug("图标保存成功: %s", savedIcon)
		}
	}

	resp := map[string]*string{
		"title":       optionalString(title),
		"favicon_url": optionalString(savedIcon),
		"url":         optionalString(normalized),
	}
	respondJSON(w, http.StatusOK, resp)
}

type createFolderRequest struct {
	Title    string `json:"title"`
	ParentID *int64 `json:"parent_id"`
}

func (s *server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req createFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondError(w, http.StatusBadRequest, errors.New("title is required"))
		return
	}
	newNode, err := s.insertNode(r.Context(), userID, nodeTypeFolder, req.Title, req.ParentID, nil, nil, "")
	if err != nil {
		if errors.Is(err, ErrInvalidParent) || errors.Is(err, ErrDuplicateFolderName) {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusCreated, newNode)
}

type createBookmarkRequest struct {
	URL        string  `json:"url"`
	Title      *string `json:"title"`
	ParentID   *int64  `json:"parent_id"`
	FaviconURL *string `json:"favicon_url"`
	Remark     string  `json:"remark"`
}

func (s *server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req createBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		respondError(w, http.StatusBadRequest, errors.New("url is required"))
		return
	}
	normalizedURL, err := normalizeURL(req.URL)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid url: %w", err))
		return
	}

	title := ""
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
	}

	favicon := ""
	if req.FaviconURL != nil {
		favicon = strings.TrimSpace(*req.FaviconURL)
	}

	if title == "" || favicon == "" {
		metaTitle, metaIcon, metaErr := s.fetchMetadata(normalizedURL)
		if metaErr == nil {
			if title == "" {
				title = metaTitle
			}
			if favicon == "" {
				favicon = metaIcon
			}
		}
	}

	if title == "" {
		title = normalizedURL
	}

	urlCopy := normalizedURL
	var faviconPtr *string
	if favicon != "" {
		tmp := favicon
		faviconPtr = &tmp
	}
	newNode, err := s.insertNode(r.Context(), userID, nodeTypeBookmark, title, req.ParentID, &urlCopy, faviconPtr, req.Remark)
	if err != nil {
		if errors.Is(err, ErrInvalidParent) || errors.Is(err, ErrDuplicateFolderName) || errors.Is(err, ErrDuplicateBookmark) {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusCreated, newNode)
}

type updateNodeRequest struct {
	Title      *string `json:"title"`
	URL        *string `json:"url"`
	ParentID   *int64  `json:"parent_id"`
	FaviconURL *string `json:"favicon_url"`
	Remark     *string `json:"remark"`
}

func (s *server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var req updateNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if err := s.updateNode(r.Context(), userID, id, req); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			respondError(w, http.StatusNotFound, errors.New("node not found"))
		case errors.Is(err, ErrInvalidParent), errors.Is(err, ErrCycleDetected), errors.Is(err, ErrInvalidUpdate),
			errors.Is(err, ErrDuplicateFolderName), errors.Is(err, ErrDuplicateBookmark):
			respondError(w, http.StatusBadRequest, err)
		default:
			respondError(w, http.StatusInternalServerError, err)
		}
		return
	}

	updatedNode, err := s.getNode(r.Context(), userID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, updatedNode)
}

func (s *server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	// 先确认节点存在且属于当前用户
	var nodeType string
	err = s.db.QueryRowContext(r.Context(), "SELECT type FROM nodes WHERE id = ? AND user_id = ?", id, userID).Scan(&nodeType)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, errors.New("node not found"))
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	// 用递归 CTE 收集该节点及所有子孙节点的 id 与 type
	rows, err := s.db.QueryContext(r.Context(), `
		WITH RECURSIVE subtree(id, type) AS (
			SELECT id, type FROM nodes WHERE id = ? AND user_id = ?
			UNION ALL
			SELECT n.id, n.type FROM nodes n
			INNER JOIN subtree s ON n.parent_id = s.id
			WHERE n.user_id = ?
		)
		SELECT id, type FROM subtree
	`, id, userID, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	var allIDs []int64
	var folders, bookmarks int64
	for rows.Next() {
		var nid int64
		var ntype string
		if err := rows.Scan(&nid, &ntype); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		allIDs = append(allIDs, nid)
		if ntype == nodeTypeFolder {
			folders++
		} else {
			bookmarks++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if len(allIDs) == 0 {
		respondError(w, http.StatusNotFound, errors.New("node not found"))
		return
	}

	// 构建 IN 子句批量删除
	placeholders := make([]string, len(allIDs))
	args := make([]interface{}, len(allIDs)+1)
	args[0] = userID
	for i, nid := range allIDs {
		placeholders[i] = "?"
		args[i+1] = nid
	}
	query := "DELETE FROM nodes WHERE user_id = ? AND id IN (" + strings.Join(placeholders, ",") + ")"
	if _, err = s.db.ExecContext(r.Context(), query, args...); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "deleted",
		"folders":   folders,
		"bookmarks": bookmarks,
	})
}

type batchDeleteRequest struct {
	IDs []int64 `json:"ids"`
}

func (s *server) handleBatchDeleteNodes(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req batchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if len(req.IDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("ids cannot be empty"))
		return
	}

	Debug("批量删除请求开始，共 %d 个ID: %v", len(req.IDs), req.IDs)

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		Debug("批量删除失败，开启事务失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %w", err))
		return
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(r.Context(), "DELETE FROM nodes WHERE id = ? AND user_id = ?")
	if err != nil {
		Debug("批量删除失败，准备语句失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to prepare statement: %w", err))
		return
	}
	defer stmt.Close()

	var deletedCount int64
	for _, id := range req.IDs {
		res, err := stmt.ExecContext(r.Context(), id, userID)
		if err != nil {
			Debug("批量删除失败，删除ID %d 时出错: %v", id, err)
			respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to delete node %d: %w", id, err))
			return
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			Debug("成功删除ID: %d", id)
			deletedCount += affected
		} else {
			Debug("未找到ID: %d，删除失败", id)
		}
	}

	if err := tx.Commit(); err != nil {
		Debug("批量删除失败，提交事务失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to commit transaction: %w", err))
		return
	}

	Debug("批量删除请求完成，请求 %d 个ID，成功删除 %d 个", len(req.IDs), deletedCount)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "deleted",
		"deleted_count":   deletedCount,
		"requested_count": len(req.IDs),
	})
}

type reorderRequest struct {
	ParentID   *int64  `json:"parent_id"`
	OrderedIDs []int64 `json:"ordered_ids"`
}

func (s *server) handleReorderNodes(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	if len(req.OrderedIDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("ordered_ids cannot be empty"))
		return
	}
	if err := s.reorderNodes(r.Context(), userID, req.ParentID, req.OrderedIDs); err != nil {
		switch {
		case errors.Is(err, ErrInvalidParent), errors.Is(err, ErrInvalidUpdate):
			respondError(w, http.StatusBadRequest, err)
		default:
			respondError(w, http.StatusInternalServerError, err)
		}
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type importRequest struct {
	Bookmarks []*node `json:"bookmarks"`
	Mode      string  `json:"mode"`      // merge 或 replace
	ParentID  *int64  `json:"parent_id"` // 导入到指定的父文件夹ID
}

type importStats struct {
	Folders   int `json:"folders"`
	Bookmarks int `json:"bookmarks"`
	Skipped   int `json:"skipped"`
}

func (s *server) handleImport(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if len(req.Bookmarks) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("no bookmarks to import"))
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// remarkMap 仅在 replace 模式下有值，用于删除前保存 url→remark
	var remarkMap map[string]string

	if req.Mode == "replace" {
		if req.ParentID != nil {
			var folderTitle string
			var folderPosition int
			var folderParentID *int64
			err = tx.QueryRowContext(r.Context(), "SELECT title, position, parent_id FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID).Scan(&folderTitle, &folderPosition, &folderParentID)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：删除文件夹 ID=%d, 标题=%s", *req.ParentID, folderTitle)

			// 删除前保存该子树下所有书签的 url→remark 映射
			remarkMap = loadRemarkMapForSubtree(tx, r.Context(), *req.ParentID, userID)

			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID); err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			var res sql.Result
			res, err = tx.ExecContext(r.Context(), `
				INSERT INTO nodes (parent_id, type, title, position, user_id)
				VALUES (?, ?, ?, ?, ?)
			`, folderParentID, nodeTypeFolder, folderTitle, folderPosition, userID)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			newFolderID, err2 := res.LastInsertId()
			if err2 != nil {
				err = err2
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：重新创建文件夹，新ID=%d", newFolderID)
			req.ParentID = &newFolderID
		} else {
			// 全量替换：保存所有用户书签的 remark
			remarkMap = loadRemarkMapForUser(tx, r.Context(), userID)

			Debug("执行replace模式，删除所有数据")
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE user_id = ?", userID); err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}
		}
	}

	Debug("开始导入节点，parentID=%v, mode=%s", req.ParentID, req.Mode)
	stats := &importStats{}
	faviconQueue := []int64{}
	if err = s.importNodes(tx, r.Context(), userID, req.Bookmarks, req.ParentID, req.Mode, stats, true, &faviconQueue, remarkMap); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	for _, nodeID := range faviconQueue {
		s.queueFaviconFetch(nodeID)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "imported",
		"stats":  stats,
	})
}

// Edge导入请求结构
type edgeImportRequest struct {
	HTML     string `json:"html"`      // Edge导出的HTML内容
	Mode     string `json:"mode"`      // merge 或 replace
	ParentID *int64 `json:"parent_id"` // 导入到指定的父文件夹ID
}

// 解析Edge HTML书签并导入
func (s *server) handleEdgeImport(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req edgeImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error("JSON解码失败: %v", err)
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if req.HTML == "" {
		Error("HTML内容为空")
		respondError(w, http.StatusBadRequest, errors.New("no html content to import"))
		return
	}

	nodes, err := parseEdgeHTML(req.HTML, s.iconPath)
	if err != nil {
		Error("HTML解析失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to parse HTML: %w", err))
		return
	}

	if len(nodes) == 0 {
		Error("未找到书签")
		respondError(w, http.StatusBadRequest, errors.New("no bookmarks found in HTML"))
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error("开启事务失败: %v", err)
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// remarkMap 仅在 replace 模式下有值
	var remarkMap map[string]string

	if req.Mode == "replace" {
		if req.ParentID != nil {
			var folderTitle string
			var folderPosition int
			var folderParentID *int64
			err := tx.QueryRowContext(r.Context(), "SELECT title, position, parent_id FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID).Scan(&folderTitle, &folderPosition, &folderParentID)
			if err != nil {
				Error("获取文件夹信息失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：删除文件夹 ID=%d, 标题=%s", *req.ParentID, folderTitle)

			// 删除前保存该子树下所有书签的 url→remark 映射
			remarkMap = loadRemarkMapForSubtree(tx, r.Context(), *req.ParentID, userID)

			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID); err != nil {
				Error("删除数据失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			res, err := tx.ExecContext(r.Context(), `
				INSERT INTO nodes (parent_id, type, title, position, user_id)
				VALUES (?, ?, ?, ?, ?)
			`, folderParentID, nodeTypeFolder, folderTitle, folderPosition, userID)
			if err != nil {
				Error("创建文件夹失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			newFolderID, err := res.LastInsertId()
			if err != nil {
				Error("获取新文件夹ID失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：重新创建文件夹，新ID=%d", newFolderID)

			req.ParentID = &newFolderID
		} else {
			// 全量替换：保存所有用户书签的 remark
			remarkMap = loadRemarkMapForUser(tx, r.Context(), userID)

			Debug("执行replace模式，删除所有数据")
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE user_id = ?", userID); err != nil {
				Error("删除数据失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}
		}
	}

	stats := &importStats{}
	Debug("开始导入节点，共%d个根节点，父文件夹ID=%v", len(nodes), req.ParentID)
	faviconQueue := []int64{}
	if err = s.importNodes(tx, r.Context(), userID, nodes, req.ParentID, req.Mode, stats, true, &faviconQueue, remarkMap); err != nil {
		Error("导入节点失败: %v", err)
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	Debug("导入节点成功，统计: 文件夹=%d, 书签=%d, 跳过=%d", stats.Folders, stats.Bookmarks, stats.Skipped)
	if err = tx.Commit(); err != nil {
		Error("提交事务失败: %v", err)
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	for _, nodeID := range faviconQueue {
		s.queueFaviconFetch(nodeID)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "imported",
		"stats":  stats,
	})
}

// 解析Edge导出的HTML书签
func parseEdgeHTML(htmlContent string, iconPath string) ([]*node, error) {
	// 解析HTML文档
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		Error("HTML解析失败: %v", err)
		return nil, err
	}

	// 查找body标签，从body开始解析
	var body *html.Node
	var findBody func(*html.Node)
	findBody = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBody(c)
			if body != nil {
				return
			}
		}
	}
	findBody(doc)

	if body == nil {
		Error("未找到body标签")
		return nil, errors.New("no body tag found")
	}

	// 解析书签树
	nodes := []*node{}
	var parentStack []*node
	var currentParent *node

	// 计数器，用于调试
	depth := 0

	var parseNodes func(*html.Node)
	parseNodes = func(n *html.Node) {
		depth++
		defer func() {
			depth--
		}()

		Debug("解析节点: 深度=%d, 类型=%s, 数据=%s", depth, n.Type, n.Data)

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				Debug("处理元素: 深度=%d, 标签=%s", depth, c.Data)

				switch c.Data {
				case "h3":
					// 创建文件夹
					folderName := extractText(c)
					Debug("创建文件夹: 深度=%d, 名称=%s", depth, folderName)
					if folderName == "" {
						Debug("文件夹名称为空，跳过")
						continue
					}

					newFolder := &node{
						Type:  nodeTypeFolder,
						Title: folderName,
					}

					// 添加到当前父文件夹
					if currentParent != nil {
						if currentParent.Children == nil {
							currentParent.Children = []*node{}
						}
						currentParent.Children = append(currentParent.Children, newFolder)
						Debug("将文件夹添加到父文件夹: 父文件夹=%s", currentParent.Title)
					} else {
						// 根文件夹
						nodes = append(nodes, newFolder)
						Debug("添加根文件夹: %s", folderName)
					}

					// 将新文件夹压入栈，并设置为当前父文件夹
					parentStack = append(parentStack, newFolder)
					currentParent = newFolder
					Debug("更新当前父文件夹: %s, 栈深度=%d", currentParent.Title, len(parentStack))
				case "a":
					// 创建书签
					bookmark := &node{
						Type: nodeTypeBookmark,
					}

					// 提取URL和图标
					var url string
					var iconData string
					for _, attr := range c.Attr {
						attrKey := strings.ToLower(attr.Key)
						switch attrKey {
						case "href":
							url = attr.Val
							bookmark.URL = optionalString(url)
						case "icon":
							iconData = attr.Val
							Debug("找到图标属性: %s, 值长度=%d", attr.Key, len(iconData))
						}
					}

					if bookmark.URL == nil || *bookmark.URL == "" {
						Debug("书签URL为空，跳过")
						continue
					}

					// 提取标题
					bookmark.Title = extractText(c)

					// 处理base64图标，保存到本地文件
					if iconData != "" {
						Debug("处理图标数据: 长度=%d, 前30字符=%s", len(iconData), iconData[:min(30, len(iconData))])
						// 保存base64图标到本地文件
						localPath, err := saveBase64Icon(iconData, iconPath)
						if err != nil {
							Error("保存base64图标失败: %v", err)
							// 保存失败时，仍然使用原始base64数据
							bookmark.FaviconURL = optionalString(iconData)
						} else {
							// 保存成功，使用本地路径
							bookmark.FaviconURL = optionalString(localPath)
							Debug("图标保存成功: %s", localPath)
						}
					}
					Debug("创建书签: 深度=%d, 标题=%s, URL=%s, 有图标=%t", depth, bookmark.Title, *bookmark.URL, bookmark.FaviconURL != nil)

					// 添加到当前父文件夹
					if currentParent != nil {
						if currentParent.Children == nil {
							currentParent.Children = []*node{}
						}
						currentParent.Children = append(currentParent.Children, bookmark)
						Debug("将书签添加到父文件夹: 父文件夹=%s", currentParent.Title)
					} else {
						// 根书签
						nodes = append(nodes, bookmark)
						Debug("添加根书签: %s", bookmark.Title)
					}
				case "dl":
					// 进入文件夹层级，递归解析子节点
					Debug("进入文件夹层级: 深度=%d", depth)
					parseNodes(c)
					// 解析完DL标签后，退出当前文件夹层级
					if len(parentStack) > 0 {
						// 弹出当前文件夹
						currentFolder := parentStack[len(parentStack)-1]
						parentStack = parentStack[:len(parentStack)-1]
						// 设置新的当前父文件夹
						if len(parentStack) > 0 {
							currentParent = parentStack[len(parentStack)-1]
						} else {
							currentParent = nil
						}
						Debug("退出文件夹层级: 文件夹=%s, 新的当前父文件夹=%s, 栈深度=%d", currentFolder.Title, func() string {
							if currentParent != nil {
								return currentParent.Title
							} else {
								return "nil"
							}
						}(), len(parentStack))
					}
				case "dt":
					// 解析DT标签内的内容
					Debug("处理DT标签: 深度=%d", depth)
					parseNodes(c)
				case "p":
					// 忽略P标签
					Debug("忽略P标签: 深度=%d", depth)
					continue
				default:
					Debug("未知标签: 深度=%d, 标签=%s", depth, c.Data)
					continue
				}
			}
		}
	}

	// 开始解析
	Debug("开始解析body标签")
	parseNodes(body)

	Debug("解析完成，共找到%d个根节点", len(nodes))
	return nodes, nil
}

// 提取HTML节点的文本内容
func extractText(n *html.Node) string {
	var text strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(text.String())
}

// loadRemarkMapForUser 查询指定用户所有书签的 url→remark 映射（全量替换前调用）
func loadRemarkMapForUser(tx *sql.Tx, ctx context.Context, userID int64) map[string]string {
	m := make(map[string]string)
	rows, err := tx.QueryContext(ctx,
		"SELECT url, remark FROM nodes WHERE user_id = ? AND type = 'bookmark' AND remark != ''",
		userID)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var url, remark string
		if rows.Scan(&url, &remark) == nil && url != "" {
			m[url] = remark
		}
	}
	return m
}

// loadRemarkMapForSubtree 递归查询指定子树下所有书签的 url→remark 映射（指定文件夹替换前调用）
// SQLite 支持 WITH RECURSIVE，使用 CTE 递归遍历整棵子树
func loadRemarkMapForSubtree(tx *sql.Tx, ctx context.Context, rootID int64, userID int64) map[string]string {
	m := make(map[string]string)
	rows, err := tx.QueryContext(ctx, `
		WITH RECURSIVE subtree(id) AS (
			SELECT id FROM nodes WHERE id = ? AND user_id = ?
			UNION ALL
			SELECT n.id FROM nodes n INNER JOIN subtree s ON n.parent_id = s.id WHERE n.user_id = ?
		)
		SELECT n.url, n.remark
		FROM nodes n
		INNER JOIN subtree s ON n.id = s.id
		WHERE n.type = 'bookmark' AND n.remark != '' AND n.url IS NOT NULL
	`, rootID, userID, userID)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var url, remark string
		if rows.Scan(&url, &remark) == nil && url != "" {
			m[url] = remark
		}
	}
	return m
}

func (s *server) importNodes(tx *sql.Tx, ctx context.Context, userID int64, nodes []*node, parentID *int64, mode string, stats *importStats, fetchMetadata bool, faviconQueue *[]int64, remarkMap map[string]string) error {
	if parentID != nil {
		var count int
		var err error
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE id = ? AND user_id = ?", *parentID, userID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			Debug("警告：parent_id %d 不存在，创建临时文件夹", *parentID)
			res, err := tx.ExecContext(ctx, `
				INSERT INTO nodes (parent_id, type, title, position, user_id)
				VALUES (NULL, ?, ?, 0, ?)
			`, nodeTypeFolder, "临时文件夹", userID)
			if err != nil {
				return err
			}
			newParentID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			parentID = &newParentID
		}
	}

	for pos, node := range nodes {
		var newID int64
		var err error

		switch node.Type {
		case nodeTypeFolder:
			var exists bool
			if mode == "merge" {
				var count int
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND user_id = ?", nodeTypeFolder, node.Title, userID).Scan(&count); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND user_id = ?", nodeTypeFolder, *parentID, node.Title, userID).Scan(&count); err != nil {
						return err
					}
				}
				exists = count > 0
			}

			if exists {
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT id FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND user_id = ?", nodeTypeFolder, node.Title, userID).Scan(&newID); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT id FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND user_id = ?", nodeTypeFolder, *parentID, node.Title, userID).Scan(&newID); err != nil {
						return err
					}
				}
				stats.Skipped++
			} else {
				res, err := tx.ExecContext(ctx, `
					INSERT INTO nodes (parent_id, type, title, position, user_id)
					VALUES (?, ?, ?, ?, ?)
				`, parentID, nodeTypeFolder, node.Title, pos, userID)
				if err != nil {
					return err
				}
				newID, err = res.LastInsertId()
				if err != nil {
					return err
				}
				stats.Folders++
			}

			if len(node.Children) > 0 {
				if err = s.importNodes(tx, ctx, userID, node.Children, &newID, mode, stats, fetchMetadata, faviconQueue, remarkMap); err != nil {
					return err
				}
			}

		case nodeTypeBookmark:
			if node.URL == nil {
				stats.Skipped++
				continue
			}

			var exists bool
			if mode == "merge" {
				var count int
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND url = ? AND user_id = ?", nodeTypeBookmark, node.Title, *node.URL, userID).Scan(&count); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND url = ? AND user_id = ?", nodeTypeBookmark, *parentID, node.Title, *node.URL, userID).Scan(&count); err != nil {
						return err
					}
				}
				exists = count > 0
			}

			if exists {
				stats.Skipped++
			} else {
				var favicon *string
				if node.FaviconURL != nil {
					tmp := *node.FaviconURL
					favicon = &tmp
				}

				// 从 remarkMap 里按 url 取回已有备注（replace 模式删除前保存）
				remark := ""
				if remarkMap != nil && node.URL != nil {
					if r, ok := remarkMap[*node.URL]; ok {
						remark = r
					}
				}

				res, err := tx.ExecContext(ctx, `
					INSERT INTO nodes (parent_id, type, title, url, favicon_url, position, user_id, remark)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, parentID, nodeTypeBookmark, node.Title, node.URL, favicon, pos, userID, remark)
				if err != nil {
					return err
				}
				newID, err = res.LastInsertId()
				if err != nil {
					return err
				}
				stats.Bookmarks++

				if fetchMetadata && favicon == nil {
					*faviconQueue = append(*faviconQueue, newID)
				}
			}
		}
	}
	return nil
}

var (
	ErrInvalidParent       = errors.New("parent folder 不存在或不是文件夹")
	ErrCycleDetected       = errors.New("不能将文件夹移动到自己的子层级中")
	ErrInvalidUpdate       = errors.New("无效的更新数据")
	ErrDuplicateFolderName = errors.New("同一层级已存在同名文件夹")
	ErrDuplicateBookmark   = errors.New("同一文件夹中已存在相同名称和网址的收藏")
)

func (s *server) loadTree(ctx context.Context, userID int64) ([]*node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, parent_id, type, title, url, favicon_url, remark, position, created_at, updated_at
		FROM nodes
		WHERE user_id = ?
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawNode struct {
		id         int64
		parentID   sql.NullInt64
		nodeType   string
		title      string
		url        sql.NullString
		faviconURL sql.NullString
		remark     string
		position   int
		createdAt  string
		updatedAt  string
	}

	var rawNodes []rawNode
	for rows.Next() {
		var rn rawNode
		if err := rows.Scan(&rn.id, &rn.parentID, &rn.nodeType, &rn.title, &rn.url, &rn.faviconURL, &rn.remark, &rn.position, &rn.createdAt, &rn.updatedAt); err != nil {
			return nil, err
		}
		rawNodes = append(rawNodes, rn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	nodeMap := make(map[int64]*node, len(rawNodes))
	var roots []*node
	var nodesWithoutValidParent []*node

	for _, rn := range rawNodes {
		n := &node{
			ID:        rn.id,
			Type:      rn.nodeType,
			Title:     rn.title,
			Remark:    rn.remark,
			Position:  rn.position,
			CreatedAt: rn.createdAt,
			UpdatedAt: rn.updatedAt,
		}
		if rn.parentID.Valid {
			parentID := rn.parentID.Int64
			n.ParentID = &parentID
		}
		if rn.url.Valid {
			urlStr := rn.url.String
			n.URL = &urlStr
		}
		if rn.faviconURL.Valid {
			favicon := rn.faviconURL.String
			n.FaviconURL = &favicon
		}
		nodeMap[rn.id] = n
	}

	for _, n := range nodeMap {
		if n.ParentID == nil {
			roots = append(roots, n)
			continue
		}
		parent := nodeMap[*n.ParentID]
		if parent == nil {
			// 父节点不存在，将该节点作为根节点处理
			Debug("节点 %d 的父节点 %d 不存在，作为根节点处理", n.ID, *n.ParentID)
			n.ParentID = nil // 将父节点设置为nil，作为根节点
			nodesWithoutValidParent = append(nodesWithoutValidParent, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}

	// 如果没有根节点，将所有没有有效父节点的节点作为根节点
	if len(roots) == 0 {
		Debug("没有找到根节点，将 %d 个没有有效父节点的节点作为根节点", len(nodesWithoutValidParent))
		roots = nodesWithoutValidParent
	} else {
		// 如果有根节点，但也有没有有效父节点的节点，将它们也作为根节点
		if len(nodesWithoutValidParent) > 0 {
			Debug("找到 %d 个根节点，另外添加 %d 个没有有效父节点的节点作为根节点", len(roots), len(nodesWithoutValidParent))
			roots = append(roots, nodesWithoutValidParent...)
		}
	}

	sortNodes(roots)

	// 计算每个文件夹的书签数量
	calculateBookmarkCounts(roots)

	// 确保 roots 不是 nil，避免返回 null
	if roots == nil {
		roots = []*node{}
	}
	return roots, nil
}

func calculateBookmarkCounts(nodes []*node) {
	for _, n := range nodes {
		if n.Type == nodeTypeFolder {
			// 递归计算子节点的书签数量
			if len(n.Children) > 0 {
				calculateBookmarkCounts(n.Children)
				// 累加所有子节点的书签数量
				for _, child := range n.Children {
					if child.Type == nodeTypeBookmark {
						n.BookmarkCount++
					} else if child.Type == nodeTypeFolder {
						n.BookmarkCount += child.BookmarkCount
					}
				}
			}
		}
	}
}

// sortNodes 对节点进行排序
// 注意：SQL查询已经按parent_id, position, id排序，理论上此函数是冗余的
// 但为了确保数据一致性，保留此函数作为额外的保障
func sortNodes(nodes []*node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Position == nodes[j].Position {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].Position < nodes[j].Position
	})
	for _, child := range nodes {
		if len(child.Children) > 0 {
			sortNodes(child.Children)
		}
	}
}

func (s *server) insertNode(ctx context.Context, userID int64, nType, title string, parentID *int64, url, favicon *string, remark string) (*node, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if parentID != nil {
		var parentType string
		if err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *parentID, userID).Scan(&parentType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = ErrInvalidParent
			}
			return nil, err
		}
		if parentType != nodeTypeFolder {
			err = ErrInvalidParent
			return nil, err
		}
	}

	switch nType {
	case nodeTypeFolder:
		if err := ensureUniqueFolderTx(tx, userID, parentID, title, nil); err != nil {
			return nil, err
		}
	case nodeTypeBookmark:
		if url == nil || strings.TrimSpace(*url) == "" {
			return nil, ErrInvalidUpdate
		}
		if err := ensureUniqueBookmarkTx(tx, userID, parentID, title, *url, nil); err != nil {
			return nil, err
		}
	}

	var nextPos int
	if parentID == nil {
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL AND user_id = ?", userID).Scan(&nextPos); err != nil {
			return nil, err
		}
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ? AND user_id = ?", *parentID, userID).Scan(&nextPos); err != nil {
			return nil, err
		}
	}

	res, execErr := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, url, favicon_url, remark, position, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, parentID, nType, title, url, favicon, remark, nextPos, userID)
	if execErr != nil {
		err = execErr
		return nil, err
	}

	newID, execErr := res.LastInsertId()
	if execErr != nil {
		err = execErr
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	insertedNode := &node{
		ID:       newID,
		Type:     nType,
		Title:    title,
		Remark:   remark,
		Position: nextPos,
	}
	if parentID != nil {
		copyID := *parentID
		insertedNode.ParentID = &copyID
	}
	if url != nil {
		copyURL := *url
		insertedNode.URL = &copyURL
	}
	if favicon != nil {
		copyFav := *favicon
		insertedNode.FaviconURL = &copyFav
	}
	return insertedNode, nil
}

func (s *server) updateNode(ctx context.Context, userID int64, id int64, req updateNodeRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var current struct {
		Type     string
		ParentID sql.NullInt64
		Title    string
		URL      sql.NullString
		Favicon  sql.NullString
		Remark   string
	}
	err = tx.QueryRowContext(ctx, "SELECT type, parent_id, title, url, favicon_url, remark FROM nodes WHERE id = ? AND user_id = ?", id, userID).Scan(
		&current.Type, &current.ParentID, &current.Title, &current.URL, &current.Favicon, &current.Remark,
	)
	if err != nil {
		return err
	}

	if req.ParentID != nil {
		if *req.ParentID == id {
			err = ErrCycleDetected
			return err
		}
		var parentType string
		if err = tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID).Scan(&parentType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = ErrInvalidParent
			}
			return err
		}
		if parentType != nodeTypeFolder {
			err = ErrInvalidParent
			return err
		}
		isCycle, cycErr := s.parentCreatesCycle(tx, userID, id, *req.ParentID)
		if cycErr != nil {
			return cycErr
		}
		if isCycle {
			err = ErrCycleDetected
			return err
		}
	}

	var parentIDValue int64
	var targetParentID *int64
	if current.ParentID.Valid {
		parentIDValue = current.ParentID.Int64
		targetParentID = &parentIDValue
	}

	targetTitle := current.Title
	titleSet := false

	var targetURL string
	urlValid := current.URL.Valid
	if urlValid {
		targetURL = current.URL.String
	}
	urlSet := false

	var targetFavicon string
	faviconValid := current.Favicon.Valid
	if faviconValid {
		targetFavicon = current.Favicon.String
	}
	faviconSet := false

	targetRemark := current.Remark
	remarkSet := false

	parentSet := false

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			err = ErrInvalidUpdate
			return err
		}
		targetTitle = title
		titleSet = true
	}

	if req.URL != nil {
		if current.Type != nodeTypeBookmark {
			err = ErrInvalidUpdate
			return err
		}
		normalized, normErr := normalizeURL(strings.TrimSpace(*req.URL))
		if normErr != nil {
			return normErr
		}
		targetURL = normalized
		urlValid = true
		urlSet = true

		metaTitle, metaIcon, metaErr := s.fetchMetadata(normalized)
		if metaErr == nil {
			if req.Title == nil && metaTitle != "" {
				targetTitle = metaTitle
				titleSet = true
			}
			if req.FaviconURL == nil && metaIcon != "" {
				targetFavicon = metaIcon
				faviconValid = true
				faviconSet = true
			}
		}
	}

	if req.FaviconURL != nil {
		if current.Type != nodeTypeBookmark {
			err = ErrInvalidUpdate
			return err
		}
		favicon := strings.TrimSpace(*req.FaviconURL)
		if favicon == "" {
			faviconValid = false
			targetFavicon = ""
		} else {
			targetFavicon = favicon
			faviconValid = true
		}
		faviconSet = true
	}

	if req.Remark != nil {
		targetRemark = *req.Remark
		remarkSet = true
	}

	if req.ParentID != nil {
		newParent := *req.ParentID
		parentIDValue = newParent
		targetParentID = &parentIDValue
		parentSet = true
	}

	if !titleSet && !urlSet && !faviconSet && !parentSet && !remarkSet {
		return ErrInvalidUpdate
	}

	switch current.Type {
	case nodeTypeFolder:
		if titleSet || parentSet {
			if err := ensureUniqueFolderTx(tx, userID, targetParentID, targetTitle, &id); err != nil {
				return err
			}
		}
	case nodeTypeBookmark:
		if !urlValid {
			err = ErrInvalidUpdate
			return err
		}
		if titleSet || urlSet || parentSet {
			if err := ensureUniqueBookmarkTx(tx, userID, targetParentID, targetTitle, targetURL, &id); err != nil {
				return err
			}
		}
	}

	fields := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if titleSet {
		fields = append(fields, "title = ?")
		args = append(args, targetTitle)
	}

	if current.Type == nodeTypeBookmark {
		if urlSet {
			fields = append(fields, "url = ?")
			args = append(args, targetURL)
		}
		if faviconSet {
			fields = append(fields, "favicon_url = ?")
			if faviconValid {
				args = append(args, targetFavicon)
			} else {
				args = append(args, nil)
			}
		}
	}

	if parentSet {
		fields = append(fields, "parent_id = ?")
		if targetParentID != nil {
			args = append(args, *targetParentID)
		} else {
			args = append(args, nil)
		}
	}

	if remarkSet {
		fields = append(fields, "remark = ?")
		args = append(args, targetRemark)
	}

	args = append(args, id)

	query := fmt.Sprintf("UPDATE nodes SET %s WHERE id = ?", strings.Join(fields, ", "))
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *server) reorderNodes(ctx context.Context, userID int64, parentID *int64, orderedIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	placeholders := strings.Repeat("?,", len(orderedIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(orderedIDs)+1)
	args = append(args, orderedIDsToAny(orderedIDs)...)
	var count int
	var query string
	if parentID == nil {
		query = fmt.Sprintf("SELECT COUNT(*) FROM nodes WHERE parent_id IS NULL AND id IN (%s) AND user_id = ?", placeholders)
		args = append(args, userID)
	} else {
		query = fmt.Sprintf("SELECT COUNT(*) FROM nodes WHERE parent_id = ? AND id IN (%s) AND user_id = ?", placeholders)
		args = append([]any{*parentID}, args...)
		args = append(args, userID)
	}

	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return err
	}
	if count != len(orderedIDs) {
		return ErrInvalidParent
	}

	for pos, id := range orderedIDs {
		if _, err := tx.ExecContext(ctx, "UPDATE nodes SET position = ? WHERE id = ? AND user_id = ?", pos, id, userID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ensureUniqueFolderTx(tx *sql.Tx, userID int64, parentID *int64, title string, excludeID *int64) error {
	var query string
	var args []any
	if parentID == nil {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND user_id = ?"
		args = []any{nodeTypeFolder, title, userID}
	} else {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND user_id = ?"
		args = []any{nodeTypeFolder, *parentID, title, userID}
	}
	if excludeID != nil {
		query += " AND id != ?"
		args = append(args, *excludeID)
	}
	var count int
	if err := tx.QueryRow(query, args...).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicateFolderName
	}
	return nil
}

func ensureUniqueBookmarkTx(tx *sql.Tx, userID int64, parentID *int64, title, url string, excludeID *int64) error {
	var query string
	var args []any
	if parentID == nil {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND url = ? AND user_id = ?"
		args = []any{nodeTypeBookmark, title, url, userID}
	} else {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND url = ? AND user_id = ?"
		args = []any{nodeTypeBookmark, *parentID, title, url, userID}
	}
	if excludeID != nil {
		query += " AND id != ?"
		args = append(args, *excludeID)
	}
	var count int
	if err := tx.QueryRow(query, args...).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicateBookmark
	}
	return nil
}

func orderedIDsToAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func (s *server) parentCreatesCycle(tx *sql.Tx, userID int64, nodeID, newParentID int64) (bool, error) {
	current := newParentID
	for {
		if current == nodeID {
			return true, nil
		}
		var parent sql.NullInt64
		if err := tx.QueryRow("SELECT parent_id FROM nodes WHERE id = ? AND user_id = ?", current, userID).Scan(&parent); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, ErrInvalidParent
			}
			return false, err
		}
		if !parent.Valid {
			return false, nil
		}
		current = parent.Int64
	}
}

func (s *server) getNode(ctx context.Context, userID int64, id int64) (*node, error) {
	var n node
	var parent sql.NullInt64
	var urlVal sql.NullString
	var icon sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, parent_id, type, title, url, favicon_url, remark, position, created_at, updated_at
		FROM nodes WHERE id = ? AND user_id = ?
	`, id, userID).Scan(&n.ID, &parent, &n.Type, &n.Title, &urlVal, &icon, &n.Remark, &n.Position, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if parent.Valid {
		p := parent.Int64
		n.ParentID = &p
	}
	if urlVal.Valid {
		u := urlVal.String
		n.URL = &u
	}
	if icon.Valid {
		i := icon.String
		n.FaviconURL = &i
	}
	return &n, nil
}

func (s *server) fetchMetadata(rawURL string) (string, string, error) {
	// 解析URL以获取主机名作为备用标题
	parsedURL, parseErr := url.Parse(rawURL)
	var hostname string
	var baseIconURL string

	if parseErr == nil && parsedURL != nil {
		// 确保parsedURL正确初始化
		if parsedURL.Scheme == "" {
			parsedURL.Scheme = "https"
		}
		if parsedURL.Host != "" {
			hostname = parsedURL.Hostname()
			// 预构建基础图标URL
			baseIconURL = parsedURL.Scheme + "://" + parsedURL.Host + "/favicon.ico"
		}
	}

	// 重试机制配置
	maxRetries := 2
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 每次重试添加一些延迟
		if attempt > 0 {
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(time.Second + jitter)
		}

		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		// 创建跟踪重定向的响应
		var finalURL string = rawURL

		// 使用更完整的用户代理和HTTP头，更像真实浏览器
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")

		// 设置超时上下文
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req = req.WithContext(ctx)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			// 网络错误时继续重试
			lastErr = err
			continue
		}

		// 获取最终的URL（跟随重定向后）
		finalURL = resp.Request.URL.String()
		if finalURL != rawURL {
			Debug("URL重定向: %s -> %s", rawURL, finalURL)
		}

		defer resp.Body.Close()

		// 处理gzip压缩内容
		var bodyReader io.Reader = resp.Body
		if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
			gz, err := gzip.NewReader(resp.Body)
			if err != nil {
				Debug("无法创建gzip读取器: %v", err)
				// 尝试作为普通内容继续处理，不立即放弃
				bodyReader = resp.Body
			} else {
				defer gz.Close()
				bodyReader = gz
			}
		}
		// 也处理deflate压缩
		if strings.Contains(resp.Header.Get("Content-Encoding"), "deflate") {
			zl := flate.NewReader(resp.Body)
			defer zl.Close()
			bodyReader = zl
		}

		// 对于403错误，尝试不同的策略
		if resp.StatusCode == 403 {
			// 记录错误但继续到下一个重试
			Debug("Received 403 Forbidden for URL: %s, attempt: %d", rawURL, attempt+1)
			lastErr = fmt.Errorf("remote status 403 Forbidden")
			continue
		}

		// 对于其他错误状态码，直接使用URL信息作为备选
		if resp.StatusCode >= 400 {
			Debug("Received status %d for URL: %s", resp.StatusCode, rawURL)
			// 即使状态码错误，也尝试从URL获取基本信息
			if hostname != "" {
				return hostname, baseIconURL, nil
			}
			return rawURL, baseIconURL, nil
		}

		// 确保我们只读取HTML内容
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			// 对于非HTML内容，使用已解析的主机名
			if hostname != "" {
				return hostname, baseIconURL, nil
			}
			return rawURL, baseIconURL, nil
		}

		body, err := io.ReadAll(io.LimitReader(bodyReader, 1<<20))
		if err != nil {
			lastErr = err
			continue
		}

		// 初始化标题变量
		var title string

		// 1. 首先尝试增强的正则表达式提取标题
		// 使用更宽松的正则表达式，添加DOTALL模式支持换行符
		titleRegex := regexp.MustCompile(`(?si)<title[^>]*>(.*?)</title>`)
		matches := titleRegex.FindSubmatch(body)
		if len(matches) > 1 {
			// 提取标题并清理
			titleContent := strings.TrimSpace(html.UnescapeString(string(matches[1])))
			// 移除多余的空白字符和HTML标签
			titleContent = strings.Join(strings.Fields(titleContent), " ")
			// 确保标题不为空
			if titleContent != "" {
				title = titleContent
			}
		}

		// 2. 如果正则没找到，尝试从meta标签获取og:title
		if title == "" {
			metaTitleRegex := regexp.MustCompile(`(?si)<meta[^>]*property=["']og:title["'][^>]*content=["'](.*?)["']`)
			metaMatches := metaTitleRegex.FindSubmatch(body)
			if len(metaMatches) > 1 {
				metaTitle := strings.TrimSpace(html.UnescapeString(string(metaMatches[1])))
				metaTitle = strings.Join(strings.Fields(metaTitle), " ")
				if metaTitle != "" {
					title = metaTitle
				}
			}
		}

		// 3. 尝试解析HTML文档（即使正则已经找到标题，也进行解析以获取图标）
		var doc *html.Node
		doc, err = html.Parse(bytes.NewReader(body))

		// 如果正则表达式没有找到标题，但HTML解析成功，尝试使用html包解析
		if title == "" && err == nil {
			htmlTitle := extractTitle(doc)
			if htmlTitle != "" {
				title = htmlTitle
			}
		}

		// 4. 尝试从页面文本中提取第一个有意义的文本作为标题
		if title == "" {
			// 用原生go从网址parsedURL获取网页内容
			title, err = getPageTitle(rawURL)
		}

		// 5. 如果所有方法都失败，使用已解析的主机名或URL
		if title == "" {
			if hostname != "" {
				title = hostname
			} else {
				title = finalURL
			}
		}

		// 优先使用预构建的默认图标URL
		iconURL := baseIconURL

		// 然后尝试从页面提取更具体的图标
		if err == nil && doc != nil {
			iconHref := extractIconHref(doc)
			if iconHref != "" {
				// 使用resolveURL函数解析相对URL
				resolved, resolveErr := resolveURL(rawURL, iconHref)
				if resolveErr == nil {
					iconURL = resolved
				}
			}
		}

		return title, iconURL, nil
	}

	// 如果所有重试都失败，返回URL信息作为备选
	Error("All %d attempts failed for URL: %s, last error: %v", maxRetries+1, rawURL, lastErr)
	if hostname != "" {
		return hostname, baseIconURL, nil
	}
	return rawURL, baseIconURL, nil
}

func extractTitle(n *html.Node) string {
	// 递归查找title标签
	if n.Type == html.ElementNode && n.Data == "title" {
		// 获取title标签内的所有文本内容
		var titleText strings.Builder
		var getText func(*html.Node)
		getText = func(node *html.Node) {
			if node.Type == html.TextNode {
				titleText.WriteString(node.Data)
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				getText(child)
			}
		}
		getText(n)
		title := strings.TrimSpace(titleText.String())
		if title != "" {
			// 处理HTML实体
			title = html.UnescapeString(title)
			// 清理多余的空白字符
			title = strings.Join(strings.Fields(title), " ")
			return title
		}
	}
	// 递归搜索子节点
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractTitle(c); title != "" {
			return title
		}
	}
	return ""
}

func extractIconHref(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "link" {
		var rel, href string
		for _, attr := range n.Attr {
			if attr.Key == "rel" {
				rel = strings.ToLower(attr.Val)
			}
			if attr.Key == "href" {
				href = attr.Val
			}
		}
		if href != "" && strings.Contains(rel, "icon") {
			return href
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := extractIconHref(c); href != "" {
			return href
		}
	}
	return ""
}

func resolveURL(baseURL, href string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func buildFaviconFallback(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Path = "/favicon.ico"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func normalizeURL(input string) (string, error) {
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	parsed, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("仅支持 http/https")
	}
	if parsed.Host == "" {
		return "", errors.New("缺少主机名")
	}
	return parsed.String(), nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

// handleGetSystemConfig 获取系统级配置（无需认证）
func (s *server) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT key, value FROM sys_config WHERE user_id = 0")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		config[key] = value
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, config)
}

// handleGetConfig 获取配置
func (s *server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	// 获取当前用户的所有配置项
	rows, err := s.db.QueryContext(r.Context(), "SELECT key, value FROM sys_config WHERE user_id = ?", userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	// 构建配置响应
	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		config[key] = value
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, config)
}

// updateConfigRequest 更新配置请求

type updateConfigRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// handleUpdateConfig 更新配置
func (s *server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req updateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	// 验证请求数据
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, errors.New("key is required"))
		return
	}

	userID := getUserID(r)

	var err error
	if req.Key == "allow_register" {
		var isAdmin int
		err = s.db.QueryRow("SELECT is_admin FROM users WHERE id = ?", userID).Scan(&isAdmin)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		if isAdmin != 1 {
			respondError(w, http.StatusForbidden, errors.New("需要管理员权限"))
			return
		}

		_, err = s.db.ExecContext(r.Context(), `
			INSERT INTO sys_config (user_id, key, value) VALUES (0, ?, ?)
			ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, req.Key, req.Value)
	} else {
		_, err = s.db.ExecContext(r.Context(), `
			INSERT INTO sys_config (user_id, key, value) VALUES (?, ?, ?)
			ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, userID, req.Key, req.Value)
	}

	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

// handleGetVersion 返回应用版本信息
func (s *server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"version": appVersion})
}

type userListResponse struct {
	Users []user `json:"users"`
	Total int64  `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

type updateUserRequest struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type batchUsersRequest struct {
	Action  string  `json:"action"`
	UserIDs []int64 `json:"user_ids"`
}

func (s *server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))

	offset := (page - 1) * limit

	var whereClause string
	var args []interface{}

	if search != "" {
		whereClause = "WHERE username LIKE ? OR nickname LIKE ? OR email LIKE ?"
		searchPattern := "%" + search + "%"
		args = []interface{}{searchPattern, searchPattern, searchPattern}
	}

	countQuery := "SELECT COUNT(*) FROM users " + whereClause
	var total int64
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	query := "SELECT id, username, nickname, email, avatar, is_active, is_admin, created_at FROM users " + whereClause + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	var users []user
	for rows.Next() {
		var u user
		var isActive, isAdmin int
		var avatar sql.NullString
		err := rows.Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &avatar, &isActive, &isAdmin, &u.CreatedAt)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		if avatar.Valid {
			u.Avatar = &avatar.String
		}
		u.IsActive = isActive == 1
		u.IsAdmin = isAdmin == 1
		users = append(users, u)
	}

	respondJSON(w, http.StatusOK, userListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

func (s *server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	var u user
	var isActive, isAdmin int
	var avatar sql.NullString
	err = s.db.QueryRow("SELECT id, username, nickname, email, avatar, is_active, is_admin, created_at FROM users WHERE id = ?", userID).
		Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &avatar, &isActive, &isAdmin, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, errors.New("user not found"))
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if avatar.Valid {
		u.Avatar = &avatar.String
	}
	u.IsActive = isActive == 1
	u.IsAdmin = isAdmin == 1

	respondJSON(w, http.StatusOK, u)
}

func (s *server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	targetUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	currentUserID := getUserID(r)

	if targetUserID == currentUserID {
		respondError(w, http.StatusBadRequest, errors.New("不能修改自己的信息"))
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.Nickname != "" {
		updates = append(updates, fmt.Sprintf("nickname = $%d", argIndex))
		args = append(args, req.Nickname)
		argIndex++
	}

	if req.Email != "" {
		updates = append(updates, fmt.Sprintf("email = $%d", argIndex))
		args = append(args, req.Email)
		argIndex++
	}

	if req.Avatar != "" {
		updates = append(updates, fmt.Sprintf("avatar = $%d", argIndex))
		args = append(args, req.Avatar)
		argIndex++
	}

	if len(updates) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("no fields to update"))
		return
	}

	args = append(args, targetUserID)

	query := fmt.Sprintf("UPDATE users SET %s, updated_at = CURRENT_TIMESTAMP WHERE id = $%d", strings.Join(updates, ", "), argIndex)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

func (s *server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	targetUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	currentUserID := getUserID(r)

	if targetUserID == currentUserID {
		respondError(w, http.StatusBadRequest, errors.New("不能删除自己"))
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE user_id = ?", targetUserID); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if _, err = tx.ExecContext(r.Context(), "DELETE FROM sys_config WHERE user_id = ?", targetUserID); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	result, err := tx.ExecContext(r.Context(), "DELETE FROM users WHERE id = ?", targetUserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

func (s *server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	targetUserID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if len(req.NewPassword) < 6 {
		respondError(w, http.StatusBadRequest, errors.New("密码长度至少6位"))
		return
	}

	// 前端已经 MD5 过一次，后端再进行一次 MD5（双重 MD5）
	doubleHashedPassword := utils.MD5Hash(req.NewPassword, "bookmarks")

	_, err = s.db.Exec("UPDATE users SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", doubleHashedPassword, targetUserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

func (s *server) handleBatchUsers(w http.ResponseWriter, r *http.Request) {
	var req batchUsersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if len(req.UserIDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("user_ids is required"))
		return
	}

	currentUserID := getUserID(r)

	for _, userID := range req.UserIDs {
		if userID == currentUserID {
			respondError(w, http.StatusBadRequest, errors.New("不能对自己执行批量操作"))
			return
		}
	}

	userIDStrs := make([]string, len(req.UserIDs))
	for i, id := range req.UserIDs {
		userIDStrs[i] = strconv.FormatInt(id, 10)
	}

	switch req.Action {
	case "delete":
		rows, err := s.db.QueryContext(r.Context(), "SELECT id FROM users WHERE id IN ("+strings.Join(userIDStrs, ",")+") AND is_admin = 1")
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		defer rows.Close()

		var adminIDs []int64
		for rows.Next() {
			var adminID int64
			if err := rows.Scan(&adminID); err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}
			adminIDs = append(adminIDs, adminID)
		}

		if len(adminIDs) > 0 {
			respondError(w, http.StatusBadRequest, errors.New("不能删除管理员用户"))
			return
		}

		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		defer tx.Rollback()

		if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE user_id IN ("+strings.Join(userIDStrs, ",")+")"); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		if _, err = tx.ExecContext(r.Context(), "DELETE FROM sys_config WHERE user_id IN ("+strings.Join(userIDStrs, ",")+")"); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		if _, err = tx.ExecContext(r.Context(), "DELETE FROM users WHERE id IN ("+strings.Join(userIDStrs, ",")+")"); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		if err = tx.Commit(); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

	case "activate":
		_, err := s.db.Exec("UPDATE users SET is_active = 1, updated_at = CURRENT_TIMESTAMP WHERE id IN (" + strings.Join(userIDStrs, ",") + ")")
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

	case "deactivate":
		_, err := s.db.Exec("UPDATE users SET is_active = 0, updated_at = CURRENT_TIMESTAMP WHERE id IN (" + strings.Join(userIDStrs, ",") + ")")
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

	default:
		respondError(w, http.StatusBadRequest, errors.New("invalid action"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

// 辅助函数：获取最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// saveBase64Icon 保存base64图标到本地文件
func saveBase64Icon(iconData string, iconPath string) (string, error) {
	// 检查是否是base64数据
	if !strings.HasPrefix(iconData, "data:image/") {
		// 不是base64数据，直接返回原值
		return iconData, nil
	}

	// 解析base64数据
	parts := strings.SplitN(iconData, ";base64,", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid base64 data format")
	}

	// 获取文件扩展名
	mimeType := parts[0]
	var ext string
	switch {
	case strings.Contains(mimeType, "image/png"):
		ext = ".png"
	case strings.Contains(mimeType, "image/jpeg"):
		ext = ".jpg"
	case strings.Contains(mimeType, "image/gif"):
		ext = ".gif"
	case strings.Contains(mimeType, "image/webp"):
		ext = ".webp"
	case strings.Contains(mimeType, "image/svg"):
		ext = ".svg"
	default:
		ext = ".png" // 默认使用png
	}

	// 解码base64数据
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// 按日期格式创建目录 (YYYYMMDD)
	dateDir := time.Now().Format("20060102")
	iconDir := fmt.Sprintf("%s/%s", iconPath, dateDir)

	// 创建目录
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create icon directory: %w", err)
	}

	// 生成文件名（使用UUID避免冲突）
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath := fmt.Sprintf("%s/%s", iconDir, filename)

	// 保存文件
	if err := os.WriteFile(filePath, decoded, 0644); err != nil {
		return "", fmt.Errorf("failed to save icon file: %w", err)
	}

	// 返回相对路径（注意：静态文件服务器从 ./static 目录提供服务）
	return fmt.Sprintf("/icons/%s/%s", dateDir, filename), nil
}

// downloadAndSaveIcon 下载图标URL并保存到本地文件
func (s *server) downloadAndSaveIcon(iconURL string, iconPath string) (string, error) {
	// 检查是否是HTTP/HTTPS URL
	if !strings.HasPrefix(iconURL, "http://") && !strings.HasPrefix(iconURL, "https://") {
		// 不是HTTP URL，直接返回原值
		return iconURL, nil
	}

	// 创建HTTP请求
	req, err := http.NewRequest("GET", iconURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置用户代理，避免被拒绝
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download icon: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("icon download failed with status: %d", resp.StatusCode)
	}

	// 读取响应体
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read icon data: %w", err)
	}

	// 检测内容类型并确定扩展名
	contentType := resp.Header.Get("Content-Type")
	var ext string
	switch {
	case strings.Contains(contentType, "image/png"):
		ext = ".png"
	case strings.Contains(contentType, "image/jpeg"):
		ext = ".jpg"
	case strings.Contains(contentType, "image/gif"):
		ext = ".gif"
	case strings.Contains(contentType, "image/webp"):
		ext = ".webp"
	case strings.Contains(contentType, "image/svg"):
		ext = ".svg"
	case strings.Contains(contentType, "image/x-icon"):
		ext = ".ico"
	default:
		ext = ".ico" // 默认使用ico
	}

	// 按日期格式创建目录 (YYYYMMDD)
	dateDir := time.Now().Format("20060102")
	iconDir := fmt.Sprintf("%s/%s", iconPath, dateDir)

	// 创建目录
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create icon directory: %w", err)
	}

	// 生成文件名（使用UUID避免冲突）
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath := fmt.Sprintf("%s/%s", iconDir, filename)

	// 保存文件
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to save icon file: %w", err)
	}

	// 返回相对路径（注意：静态文件服务器从 ./static 目录提供服务）
	return fmt.Sprintf("/icons/%s/%s", dateDir, filename), nil
}

// handleIntranetURL 处理内网地址的特殊逻辑
func handleIntranetURL(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	host := strings.ToLower(parsed.Host)

	// 检测是否为内网地址
	if strings.Contains(host, "127.0.0.1") ||
		strings.Contains(host, "localhost") ||
		strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") {

		// 如果是API端点，重定向到正确的内部API调用
		if strings.Contains(urlStr, "/api/metadata") {

			// 提取URL参数中的URL
			apiURL := parsed.Query().Get("url")
			if apiURL != "" {
				return apiURL
			}
		}
	}

	return urlStr
}

func getPageTitle(url string) (string, error) {
	// 1. 发送HTTP请求
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 2. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("非200状态码: %d", resp.StatusCode)
	}

	// 3. 解析HTML并提取<title>
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("HTML解析失败: %w", err)
	}

	title, found := findTitle(doc)
	if !found {
		return "", fmt.Errorf("未找到<title>标签")
	}
	return strings.TrimSpace(title), nil
}

// 递归遍历DOM树查找<title>
func findTitle(n *html.Node) (string, bool) {
	// 在<head>内搜索<title>
	if n.Type == html.ElementNode && n.Data == "head" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "title" && c.FirstChild != nil {
				return c.FirstChild.Data, true
			}
		}
	}

	// 深度优先遍历子节点
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title, found := findTitle(c); found {
			return title, true
		}
	}
	return "", false
}

// faviconWorker 异步图标获取工作协程
func (s *server) faviconWorker() {
	for nodeID := range s.faviconChan {
		// 获取书签信息
		var url string
		err := s.db.QueryRow("SELECT url FROM nodes WHERE id = ? AND type = ?", nodeID, nodeTypeBookmark).Scan(&url)
		if err != nil {
			Error("获取书签 %d 信息失败: %v", nodeID, err)
			continue
		}

		// 只处理 http/https 协议，跳过 chrome-extension://、file:// 等不可访问的 URL
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			Debug("书签 %d URL 协议不支持，跳过 favicon 抓取: %s", nodeID, url)
			continue
		}

		// 如果已经有图标，跳过
		var existingFavicon sql.NullString
		err = s.db.QueryRow("SELECT favicon_url FROM nodes WHERE id = ?", nodeID).Scan(&existingFavicon)
		if err == nil && existingFavicon.Valid && existingFavicon.String != "" {
			Debug("书签 %d 已有图标，跳过", nodeID)
			continue
		}

		// 获取图标
		_, icon, err := s.fetchMetadata(url)
		if err != nil {
			Debug("获取书签 %d 图标失败: %v", nodeID, err)
			continue
		}

		if icon == "" {
			Debug("书签 %d 没有找到图标", nodeID)
			continue
		}

		// 更新数据库
		_, err = s.db.Exec("UPDATE nodes SET favicon_url = ? WHERE id = ?", icon, nodeID)
		if err != nil {
			Error("更新书签 %d 图标失败: %v", nodeID, err)
			continue
		}

		Debug("成功更新书签 %d 的图标", nodeID)
	}
}

// queueFaviconFetch 将书签ID加入图标获取队列
func (s *server) queueFaviconFetch(nodeID int64) {
	select {
	case s.faviconChan <- nodeID:
		Debug("书签 %d 已加入图标获取队列", nodeID)
	default:
		Debug("图标获取队列已满，跳过书签 %d", nodeID)
	}
}

// tokenAuthMiddleware 仅支持 Token 的认证中间件
func (s *server) tokenAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			respondError(w, http.StatusUnauthorized, errors.New("未提供认证token"))
			return
		}

		// 先查用户是否存在（不管 is_active 状态）
		var userID int64
		var isActive int
		err := s.db.QueryRow("SELECT id, is_active FROM users WHERE token = ?", token).Scan(&userID, &isActive)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// token 对应的用户根本不存在（已被删除）
				respondError(w, http.StatusUnauthorized, errors.New("账号不存在，请重新登录"))
				return
			}
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		if isActive == 0 {
			// 用户存在但被禁用
			respondError(w, http.StatusForbidden, errors.New("账号已被禁用，请联系管理员"))
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

// apiKeyAuthMiddleware 仅支持 API Key 的认证中间件
func (s *server) apiKeyAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey == "" {
			respondError(w, http.StatusUnauthorized, errors.New("未提供api_key"))
			return
		}

		var userID int64
		err := s.db.QueryRow("SELECT id FROM users WHERE api_key = ? AND is_active = 1", apiKey).Scan(&userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusUnauthorized, errors.New("无效的api_key"))
				return
			}
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

// apiKeyAuthMiddlewareForChi 适配 Chi 路由器的 API Key 中间件
func (s *server) apiKeyAuthMiddlewareForChi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.apiKeyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})(w, r)
	})
}

// corsMiddleware CORS 跨域中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 设置 CORS 头
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getUserID 从上下文中获取用户ID
func getUserID(r *http.Request) int64 {
	if userID, ok := r.Context().Value(userContextKey).(int64); ok {
		return userID
	}
	return 0
}

// adminMiddleware 管理员权限检查中间件
func (s *server) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)

		var isAdmin int
		err := s.db.QueryRow("SELECT is_admin FROM users WHERE id = ?", userID).Scan(&isAdmin)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}

		if isAdmin != 1 {
			respondError(w, http.StatusForbidden, errors.New("需要管理员权限"))
			return
		}

		next(w, r)
	}
}

// handleRegister 用户注册
func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Email = strings.TrimSpace(req.Email)

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, errors.New("用户名和密码不能为空"))
		return
	}

	if len(req.Password) < 6 {
		respondError(w, http.StatusBadRequest, errors.New("密码长度不能少于6位"))
		return
	}

	var allowRegister string
	err := s.db.QueryRow("SELECT value FROM sys_config WHERE user_id = 0 AND key = ?", "allow_register").Scan(&allowRegister)
	if err == nil && allowRegister == "false" {
		respondError(w, http.StatusForbidden, errors.New("系统已关闭注册功能"))
		return
	}

	// 前端已经 MD5 过一次，后端再进行一次 MD5（双重 MD5）
	doubleHashedPassword := utils.MD5Hash(req.Password, "bookmarks")

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	var userCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	isAdmin := userCount == 0

	var userID int64
	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}

	result, err := tx.Exec(`
		INSERT INTO users (username, password, nickname, email, is_admin)
		VALUES (?, ?, ?, ?, ?)
	`, req.Username, doubleHashedPassword, nickname, req.Email, isAdmin)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondError(w, http.StatusBadRequest, errors.New("用户名已存在"))
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	userID, err = result.LastInsertId()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	token := uuid.New().String()
	apiKey := strings.ReplaceAll(uuid.New().String(), "-", "")
	_, err = tx.Exec("UPDATE users SET token = ?, api_key = ? WHERE id = ?", token, apiKey, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if isAdmin {
		_, err = tx.Exec("UPDATE nodes SET user_id = ? WHERE user_id = 0", userID)
		if err != nil {
			Debug("更新nodes表user_id失败: %v", err)
		}
		_, err = tx.Exec("UPDATE sys_config SET user_id = ? WHERE user_id = 0", userID)
		if err != nil {
			Debug("更新config表user_id失败: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	user := &user{
		ID:       userID,
		Username: req.Username,
		Nickname: nickname,
		Email:    req.Email,
		IsAdmin:  isAdmin,
		IsActive: true,
	}

	respondJSON(w, http.StatusCreated, authResponse{
		Token: token,
		User:  user,
	})
}

// handleLogin 用户登录
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, errors.New("用户名和密码不能为空"))
		return
	}

	var dbUser struct {
		ID        int64
		Username  string
		Password  string
		Nickname  string
		Email     sql.NullString
		Avatar    sql.NullString
		IsActive  int
		IsAdmin   int
		Token     sql.NullString
		APIKey    sql.NullString
		CreatedAt string
	}

	err := s.db.QueryRow(`
		SELECT id, username, password, nickname, email, avatar, is_active, is_admin, token, api_key, created_at
		FROM users WHERE username = ?
	`, req.Username).Scan(
		&dbUser.ID, &dbUser.Username, &dbUser.Password, &dbUser.Nickname,
		&dbUser.Email, &dbUser.Avatar, &dbUser.IsActive, &dbUser.IsAdmin,
		&dbUser.Token, &dbUser.APIKey, &dbUser.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusUnauthorized, errors.New("用户名或密码错误"))
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if dbUser.IsActive != 1 {
		respondError(w, http.StatusForbidden, errors.New("用户已被禁用"))
		return
	}

	// 前端已经 MD5 过一次，后端再进行一次 MD5（双重 MD5）
	doubleHashedPassword := utils.MD5Hash(req.Password, "bookmarks")

	// 兼容处理：先检查是否是新格式（双重MD5），再检查旧格式（bcrypt）
	if dbUser.Password != doubleHashedPassword {
		// 尝试旧格式（bcrypt）验证
		err = bcrypt.CompareHashAndPassword([]byte(dbUser.Password), []byte(doubleHashedPassword))
		if err != nil {
			respondError(w, http.StatusUnauthorized, errors.New("用户名或密码错误"))
			return
		}
		// 旧方式验证成功，升级密码存储方式为新格式
		_, _ = s.db.Exec("UPDATE users SET password = ? WHERE id = ?", doubleHashedPassword, dbUser.ID)
	}

	token := dbUser.Token.String
	if !dbUser.Token.Valid || dbUser.Token.String == "" {
		token = uuid.New().String()
		_, err = s.db.Exec("UPDATE users SET token = ? WHERE id = ?", token, dbUser.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
	}

	if !dbUser.APIKey.Valid || dbUser.APIKey.String == "" {
		apiKey := strings.ReplaceAll(uuid.New().String(), "-", "")
		_, err = s.db.Exec("UPDATE users SET api_key = ? WHERE id = ?", apiKey, dbUser.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
	}

	user := &user{
		ID:        dbUser.ID,
		Username:  dbUser.Username,
		Nickname:  dbUser.Nickname,
		IsActive:  dbUser.IsActive == 1,
		IsAdmin:   dbUser.IsAdmin == 1,
		CreatedAt: dbUser.CreatedAt,
	}
	if dbUser.Email.Valid {
		user.Email = dbUser.Email.String
	}
	if dbUser.Avatar.Valid {
		user.Avatar = &dbUser.Avatar.String
	}
	if dbUser.APIKey.Valid {
		user.APIKey = &dbUser.APIKey.String
	}

	respondJSON(w, http.StatusOK, authResponse{
		Token: token,
		User:  user,
	})
}

// handleLogout 用户登出
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	if token != "" {
		_, err := s.db.Exec("UPDATE users SET token = NULL WHERE token = ?", token)
		if err != nil {
			Debug("清除token失败: %v", err)
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "登出成功"})
}

// handleGetCurrentUser 获取当前登录用户信息
func (s *server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var dbUser struct {
		ID        int64
		Username  string
		Nickname  string
		Email     string
		Avatar    sql.NullString
		IsActive  int
		IsAdmin   int
		APIKey    sql.NullString
		CreatedAt string
	}

	err := s.db.QueryRow(`
		SELECT id, username, nickname, email, avatar, is_active, is_admin, api_key, created_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&dbUser.ID, &dbUser.Username, &dbUser.Nickname,
		&dbUser.Email, &dbUser.Avatar, &dbUser.IsActive, &dbUser.IsAdmin, &dbUser.APIKey, &dbUser.CreatedAt,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	user := &user{
		ID:        dbUser.ID,
		Username:  dbUser.Username,
		Nickname:  dbUser.Nickname,
		Email:     dbUser.Email,
		IsActive:  dbUser.IsActive == 1,
		IsAdmin:   dbUser.IsAdmin == 1,
		CreatedAt: dbUser.CreatedAt,
	}
	if dbUser.Avatar.Valid {
		user.Avatar = &dbUser.Avatar.String
	}
	if dbUser.APIKey.Valid {
		user.APIKey = &dbUser.APIKey.String
	}

	respondJSON(w, http.StatusOK, user)
}

// handleCheckAuth 检查登录状态
func (s *server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	if token == "" {
		respondJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	var userID int64
	err := s.db.QueryRow("SELECT id FROM users WHERE token = ? AND is_active = 1", token).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user_id":       userID,
	})
}

// handleChangePassword 修改密码
func (s *server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	req.OldPassword = strings.TrimSpace(req.OldPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	if req.OldPassword == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, errors.New("旧密码和新密码不能为空"))
		return
	}

	if len(req.NewPassword) < 6 {
		respondError(w, http.StatusBadRequest, errors.New("新密码长度不能少于6位"))
		return
	}

	if req.OldPassword == req.NewPassword {
		respondError(w, http.StatusBadRequest, errors.New("新密码不能与旧密码相同"))
		return
	}

	userID := getUserID(r)

	var dbPassword string
	err := s.db.QueryRow("SELECT password FROM users WHERE id = ?", userID).Scan(&dbPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	// 前端已经 MD5 过一次，后端再进行一次 MD5（双重 MD5）
	doubleHashedOldPassword := utils.MD5Hash(req.OldPassword, "bookmarks")

	// 兼容处理：先检查是否是新格式（双重MD5），再检查旧格式（bcrypt）
	if dbPassword != doubleHashedOldPassword {
		// 尝试旧格式（bcrypt）验证
		err = bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(doubleHashedOldPassword))
		if err != nil {
			respondError(w, http.StatusUnauthorized, errors.New("旧密码错误"))
			return
		}
	}

	// 新密码同样进行双重 MD5
	doubleHashedNewPassword := utils.MD5Hash(req.NewPassword, "bookmarks")

	_, err = s.db.Exec("UPDATE users SET password = ? WHERE id = ?", doubleHashedNewPassword, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "密码修改成功"})
}

// handleRegenerateAPIKey 重新生成 api_key
func (s *server) handleRegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	newAPIKey := strings.ReplaceAll(uuid.New().String(), "-", "")

	_, err := s.db.Exec("UPDATE users SET api_key = ? WHERE id = ?", newAPIKey, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"api_key": newAPIKey,
		"message": "api_key 重新生成成功",
	})
}

// migrateOldIcons 迁移旧图标从 static/icons 到新路径
func migrateOldIcons(oldPath, newPath string) {
	oldStat, err := os.Stat(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			Debug("旧图标路径不存在，跳过迁移: %s", oldPath)
			fmt.Printf("旧图标路径不存在，跳过迁移: %s\n", oldPath)
			return
		}
		Error("检查旧图标路径失败: %v", err)
		fmt.Printf("检查旧图标路径失败: %v\n", err)
		return
	}

	if !oldStat.IsDir() {
		Debug("旧图标路径不是目录，跳过迁移: %s", oldPath)
		fmt.Printf("旧图标路径不是目录，跳过迁移: %s\n", oldPath)
		return
	}

	newStat, err := os.Stat(newPath)
	if err == nil {
		if !newStat.IsDir() {
			Debug("新图标路径不是目录，跳过迁移: %s", newPath)
			fmt.Printf("新图标路径不是目录，跳过迁移: %s\n", newPath)
			return
		}
		entries, err := os.ReadDir(newPath)
		if err != nil {
			Error("读取新图标目录失败: %v", err)
			fmt.Printf("读取新图标目录失败: %v\n", err)
			return
		}
		if len(entries) > 0 {
			Debug("新图标路径已存在且不为空，跳过迁移: %s", newPath)
			fmt.Printf("新图标路径已存在且不为空，跳过迁移: %s\n", newPath)
			return
		}
		Debug("新图标路径已存在但为空，将删除后迁移: %s", newPath)
		fmt.Printf("新图标路径已存在但为空，将删除后迁移: %s\n", newPath)
		os.RemoveAll(newPath)
	}

	Debug("开始迁移图标: %s -> %s", oldPath, newPath)
	fmt.Printf("开始迁移图标: %s -> %s\n", oldPath, newPath)

	err = os.Rename(oldPath, newPath)
	if err != nil {
		Error("直接移动图标目录失败: %v，尝试逐个迁移", err)
		fmt.Printf("直接移动图标目录失败: %v，尝试逐个迁移\n", err)
		entries, err := os.ReadDir(oldPath)
		if err != nil {
			Error("读取旧图标目录失败: %v", err)
			fmt.Printf("读取旧图标目录失败: %v\n", err)
			return
		}

		if len(entries) == 0 {
			Debug("旧图标目录为空，跳过迁移")
			fmt.Printf("旧图标目录为空，跳过迁移\n")
			return
		}

		migratedCount := 0
		for _, entry := range entries {
			srcPath := oldPath + "/" + entry.Name()
			dstPath := newPath + "/" + entry.Name()

			err := os.Rename(srcPath, dstPath)
			if err != nil {
				Error("迁移图标目录失败 %s: %v", entry.Name(), err)
				continue
			}
			migratedCount++
			fmt.Printf("迁移图标目录: %s\n", entry.Name())
		}

		if migratedCount > 0 {
			fmt.Printf("成功迁移 %d 个图标目录\n", migratedCount)
		} else {
			fmt.Printf("没有需要迁移的图标\n")
		}
		return
	}

	fmt.Printf("成功迁移图标目录: %s -> %s\n", oldPath, newPath)
}

// migrateOldDatabase 迁移旧数据库从旧路径到新路径并改名
func migrateOldDatabase(oldPath, newPath, oldName, newName string) {
	oldFilePath := oldPath + oldName
	newFilePath := newPath + newName

	oldStat, err := os.Stat(oldFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			Debug("旧数据库文件不存在，跳过迁移: %s", oldFilePath)
			return
		}
		Error("检查旧数据库文件失败: %v", err)
		return
	}

	if oldStat.IsDir() {
		Debug("旧数据库路径是目录而非文件，跳过迁移: %s", oldFilePath)
		return
	}

	_, err = os.Stat(newFilePath)
	if err == nil {
		Debug("新数据库文件已存在，跳过迁移: %s", newFilePath)
		return
	}
	if !os.IsNotExist(err) {
		Error("检查新数据库文件失败: %v", err)
		return
	}

	Debug("开始迁移数据库: %s -> %s", oldFilePath, newFilePath)

	err = os.Rename(oldFilePath, newFilePath)
	if err != nil {
		Error("迁移数据库文件失败: %v", err)
		return
	}

	fmt.Printf("成功迁移数据库文件: %s -> %s\n", oldFilePath, newFilePath)
}

// ========== 浏览器书签同步接口处理器 ==========

// handleSyncGetBookmarks 获取所有书签（扁平化列表）
func (s *server) handleSyncGetBookmarks(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	bs := logic.NewBrowserSync(s.db)
	bookmarks, err := bs.GetBookmarks(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"bookmarks": bookmarks,
	})
}

// handleSyncCreateBookmark 创建书签
func (s *server) handleSyncCreateBookmark(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var bookmark logic.SyncBookmark
	if err := json.NewDecoder(r.Body).Decode(&bookmark); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if bookmark.Title == "" || bookmark.URL == "" {
		respondError(w, http.StatusBadRequest, errors.New("title and url are required"))
		return
	}

	bs := logic.NewBrowserSync(s.db)
	created, err := bs.CreateBookmark(r.Context(), userID, &bookmark)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

// handleSyncUpdateBookmark 更新书签
func (s *server) handleSyncUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var bookmark logic.SyncBookmark
	if err := json.NewDecoder(r.Body).Decode(&bookmark); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	bookmark.ID = id

	bs := logic.NewBrowserSync(s.db)
	if err := bs.UpdateBookmark(r.Context(), userID, &bookmark); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// handleSyncDeleteBookmark 删除书签
func (s *server) handleSyncDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	bs := logic.NewBrowserSync(s.db)
	if err := bs.DeleteBookmark(r.Context(), userID, id); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// handleSyncGetFolders 获取所有文件夹（扁平化列表）
func (s *server) handleSyncGetFolders(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	bs := logic.NewBrowserSync(s.db)
	folders, err := bs.GetFolders(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"folders": folders,
	})
}

// handleSyncCreateFolder 创建文件夹
func (s *server) handleSyncCreateFolder(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var folder logic.SyncFolder
	if err := json.NewDecoder(r.Body).Decode(&folder); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if folder.Title == "" {
		respondError(w, http.StatusBadRequest, errors.New("title is required"))
		return
	}

	bs := logic.NewBrowserSync(s.db)
	created, err := bs.CreateFolder(r.Context(), userID, &folder)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

// handleSyncUpdateFolder 更新文件夹
func (s *server) handleSyncUpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var folder logic.SyncFolder
	if err := json.NewDecoder(r.Body).Decode(&folder); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	folder.ID = id

	bs := logic.NewBrowserSync(s.db)
	if err := bs.UpdateFolder(r.Context(), userID, &folder); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// handleSyncDeleteFolder 删除文件夹
func (s *server) handleSyncDeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	bs := logic.NewBrowserSync(s.db)
	if err := bs.DeleteFolder(r.Context(), userID, id); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// handleSyncBatchOperation 批量操作
func (s *server) handleSyncBatchOperation(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var req logic.BatchOperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	bs := logic.NewBrowserSync(s.db)
	result, err := bs.BatchOperation(r.Context(), userID, &req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	// 对新创建的、favicon 为空的书签异步补抓图标
	for _, b := range result.Created.Bookmarks {
		if b.FaviconURL == nil || *b.FaviconURL == "" {
			s.queueFaviconFetch(b.ID)
		}
	}

	respondJSON(w, http.StatusOK, result)
}

// handleSyncGetTree 返回应用书签的完整树形结构，供插件「应用→浏览器」方向同步使用
// 查询参数：folder_id（可选，指定根文件夹 ID，不传则返回全量根节点）
func (s *server) handleSyncGetTree(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var rootFolderID *int64
	if idStr := r.URL.Query().Get("folder_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Errorf("invalid folder_id"))
			return
		}
		// 校验该文件夹属于当前用户
		var nodeType string
		err = s.db.QueryRowContext(r.Context(), "SELECT type FROM nodes WHERE id = ? AND user_id = ?", id, userID).Scan(&nodeType)
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, fmt.Errorf("folder not found"))
			return
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		if nodeType != nodeTypeFolder {
			respondError(w, http.StatusBadRequest, fmt.Errorf("specified id is not a folder"))
			return
		}
		rootFolderID = &id
	}

	bs := logic.NewBrowserSync(s.db)
	tree, err := bs.GetTree(r.Context(), userID, rootFolderID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": tree,
	})
}
