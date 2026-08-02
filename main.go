package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bookmark/app/logger"
	"bookmark/app/logic"
	"bookmark/app/utils"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
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
	appVersion = "v3.3.0"

	// 日志模式常量
	logModeDebug   = "debug"
	logModeRelease = "release"
	defaultLogMode = logModeRelease
)

type versionCheckCache struct {
	latestVersion string
	downloadURL   string
	checkedAt     time.Time
}

type server struct {
	db                *sql.DB
	httpClient        *http.Client
	faviconChan       chan int64 // 图标获取任务队列
	iconPath          string     // 图标存储路径
	securityQuestions *logic.SecurityQuestions
	verCache          *versionCheckCache
	verCacheMu        sync.Mutex
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
	UserID        int64   `json:"user_id,omitempty"`
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	URL           *string `json:"url,omitempty"`
	FaviconURL    *string `json:"favicon_url,omitempty"`
	Remark        string  `json:"remark,omitempty"`
	Visibility    string  `json:"visibility,omitempty"`
	Position      int     `json:"position"`
	Children      []*node `json:"children,omitempty"`
	BookmarkCount int     `json:"bookmark_count,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
	UpdatedAt     string  `json:"updated_at,omitempty"`
}

type user struct {
	ID                   int64    `json:"id"`
	Username             string   `json:"username"`
	Nickname             string   `json:"nickname"`
	Email                string   `json:"email"`
	Avatar               *string  `json:"avatar"`
	IsActive             bool     `json:"is_active"`
	IsAdmin              bool     `json:"is_admin"`
	APIKey               *string  `json:"api_key,omitempty"`
	LastLoginAt          *string  `json:"last_login_at,omitempty"`
	CreatedAt            string   `json:"created_at"`
	HasSecurityQuestions bool     `json:"has_security_questions"`
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

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname,omitempty"`
	Email    string `json:"email,omitempty"`
	IsAdmin  bool   `json:"is_admin"`
}

type auditLogEntry struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	Detail     string `json:"detail"`
	IPAddress  string `json:"ip_address"`
	CreatedAt  string `json:"created_at"`
}

type auditLogResponse struct {
	Logs  []auditLogEntry `json:"logs"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// 用户上下文键
type contextKey string

const (
	userContextKey contextKey = "user"
)

type StatsRequest struct {
	AppName    string `json:"app_name"`
	Version    string `json:"version"`
	DeviceType string `json:"device_type"`
	DeviceID   string `json:"device_id"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Hostname   string `json:"hostname"`
}

func getDeviceID(dataUrl string) string {
	hostname, _ := os.Hostname()
	raw := hostname + runtime.GOOS + runtime.GOARCH + dataUrl
	return fmt.Sprintf("%x", md5.Sum([]byte(raw)))
}

func startStatsReporter(appName, version, deviceType, dataUrl string) {
	hostname, _ := os.Hostname()
	deviceID := getDeviceID(dataUrl)

	req := StatsRequest{
		AppName:    appName,
		Version:    version,
		DeviceType: deviceType,
		DeviceID:   deviceID,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Hostname:   hostname,
	}

	report := func() {
		body, _ := json.Marshal(req)
		http.Post("http://techfunway.wycto.cn/api/apps.online/refresh", "application/json", bytes.NewReader(body))
	}

	ticker := time.NewTicker(60 * time.Minute)
	go func() {
		report()
		for range ticker.C {
			report()
		}
	}()
}

func main() {
	dataUrl := flag.String("dataUrl", "./data", "数据存储路径")
	port := flag.String("port", "8901", "服务器监听端口")
	logModeFlag := flag.String("logmode", defaultLogMode, "日志模式: debug 或 release")
	deviceType := flag.String("deviceType", "", "设备类型")
	insecureTLS := flag.Bool("insecureTLS", false, "抓取网页元数据时跳过TLS证书验证（用于内网自签名证书环境）")
	disableStats := flag.Bool("disableStats", false, "禁用匿名使用统计上报（也可设置环境变量 DISABLE_STATS=1）")
	flag.Parse()

	if !*disableStats && os.Getenv("DISABLE_STATS") == "" {
		startStatsReporter("bookmarks", appVersion, *deviceType, *dataUrl)
	} else {
		log.Println("使用统计上报已禁用")
	}

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

	db, err := sql.Open("sqlite", dbPath+"database.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	// SQLite 单连接避免多连接写锁竞争；WAL+busy_timeout 已在 DSN 中配置
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// 初始化数据库
	if err := initializeDB(db); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	// 执行系统升级
	upgrader := logic.NewUpgrade(db, appVersion, logPath)
	if err := upgrader.PerformUpgrade(); err != nil {
		log.Printf("系统升级失败: %v", err)
	}

	// 元数据抓取的HTTP客户端；默认验证TLS证书，-insecureTLS 可关闭（内网自签名证书场景）
	transport := &http.Transport{}
	if *insecureTLS {
		log.Println("警告: 已禁用TLS证书验证（-insecureTLS）")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	s := &server{
		db: db,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// 允许最多10次重定向
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		faviconChan:       make(chan int64, 100), // 缓冲队列，最多100个待处理任务
		iconPath:          iconPath,              // 设置图标路径
		securityQuestions: logic.NewSecurityQuestions(db),
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
			// 安全问题相关接口
			r.Post("/security-questions", s.tokenAuthMiddleware(s.handleSetSecurityQuestions))
			r.Get("/security-questions", s.tokenAuthMiddleware(s.handleGetSecurityQuestions))
			r.Get("/security-questions/reset", s.handleGetSecurityQuestionsForReset)
			r.Post("/verify-and-reset", s.handleVerifyAndResetPassword)
		})
		r.Route("/users", func(r chi.Router) {
			r.Post("/", s.tokenAuthMiddleware(s.adminMiddleware(s.handleCreateUser)))
			r.Get("/", s.tokenAuthMiddleware(s.adminMiddleware(s.handleGetUsers)))
			r.Get("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleGetUser)))
			r.Put("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleUpdateUser)))
			r.Delete("/{id}", s.tokenAuthMiddleware(s.adminMiddleware(s.handleDeleteUser)))
			r.Post("/{id}/reset-password", s.tokenAuthMiddleware(s.adminMiddleware(s.handleResetPassword)))
			r.Post("/batch", s.tokenAuthMiddleware(s.adminMiddleware(s.handleBatchUsers)))
		})
		r.Route("/admin", func(r chi.Router) {
			r.Use(s.tokenAuthMiddlewareChi)
			r.Use(s.adminMiddlewareChi)
			r.Get("/stats", s.handleAdminStats)
			r.Get("/users/{userId}/tree", s.handleAdminGetUserTree)
			r.Get("/users/{userId}/export", s.handleAdminExportUser)
			r.Put("/nodes/{id}", s.handleAdminUpdateNode)
			r.Delete("/nodes/{id}", s.handleAdminDeleteNode)
			r.Get("/audit-log", s.handleGetAuditLog)
			r.Get("/audit-log/export", s.handleExportAuditLog)
				r.Post("/folders", s.handleAdminCreateFolder)
				r.Post("/bookmarks", s.handleAdminCreateBookmark)
				r.Put("/nodes/reorder", s.handleAdminReorderNodes)
		})
		r.Get("/tree", s.optionalAuthMiddleware(s.handleGetTree))
		r.Get("/public-tree", s.handleGetPublicTree)
		r.Get("/stats", s.tokenAuthMiddleware(s.handleUserStats))
		r.Get("/metadata", s.handleMetadata)
		r.Get("/version", s.handleGetVersion)
		r.Get("/version/check", s.tokenAuthMiddleware(s.handleCheckVersion))
		r.Post("/folders", s.optionalAuthMiddleware(s.handleCreateFolder))
		r.Post("/bookmarks", s.optionalAuthMiddleware(s.handleCreateBookmark))
		r.Put("/nodes/{id}", s.optionalAuthMiddleware(s.handleUpdateNode))
		r.Delete("/nodes/{id}", s.optionalAuthMiddleware(s.handleDeleteNode))
		r.Delete("/bookmarks/{id}", s.optionalAuthMiddleware(s.handleDeleteNode))
		r.Post("/nodes/batch-delete", s.optionalAuthMiddleware(s.handleBatchDeleteNodes))
		r.Post("/nodes/reorder", s.optionalAuthMiddleware(s.handleReorderNodes))
		r.Post("/import", s.optionalAuthMiddleware(s.handleImport))
		r.Post("/import-edge", s.optionalAuthMiddleware(s.handleEdgeImport))
		r.Get("/config/system", s.handleGetSystemConfig)
		r.Get("/config", s.optionalAuthMiddleware(s.handleGetConfig))
		r.Post("/config", s.optionalAuthMiddleware(s.handleUpdateConfig))
		r.Get("/check-duplicates", s.optionalAuthMiddleware(s.handleCheckDuplicates))
		r.Post("/check-links", s.optionalAuthMiddleware(s.handleCheckLinks))
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
	r.Get("/manifest.webmanifest", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		fileServer.ServeHTTP(w, req)
	})
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
			log.Printf("启用外键约束失败: %v", err)
		}

		if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL DEFAULT 0,
    parent_id INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('folder', 'bookmark')),
    title TEXT NOT NULL,
    url TEXT,
    favicon_url TEXT,
    remark TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
			log.Printf("创建nodes表失败: %v", err)
		}

		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_parent_position ON nodes(parent_id, position);
CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);
CREATE INDEX IF NOT EXISTS idx_nodes_user_id_parent ON nodes(user_id, parent_id);`); err != nil {
			log.Printf("创建nodes表索引失败: %v", err)
		}

		if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS trg_nodes_updated_at
AFTER UPDATE ON nodes
BEGIN
    UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;`); err != nil {
			log.Printf("创建nodes表updated_at触发器失败: %v", err)
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

func (s *server) handleGetPublicTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.loadPublicTree(r.Context())
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
	Icon     string `json:"icon"`
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
	var icon *string
	if req.Icon != "" {
		icon = &req.Icon
	}
	newNode, err := s.insertNode(r.Context(), userID, nodeTypeFolder, req.Title, req.ParentID, nil, icon, "", "private")
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
	Visibility string  `json:"visibility"`
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
	visibility := req.Visibility
	if visibility == "" {
		visibility = "private"
	}
	newNode, err := s.insertNode(r.Context(), userID, nodeTypeBookmark, title, req.ParentID, &urlCopy, faviconPtr, req.Remark, visibility)
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
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	ParentID    *int64  `json:"parent_id"`
	ParentIDSet bool    `json:"-"`
	FaviconURL  *string `json:"favicon_url"`
	Remark      *string `json:"remark"`
	Visibility  *string `json:"visibility"`
}

func (r *updateNodeRequest) UnmarshalJSON(data []byte) error {
	type Alias updateNodeRequest
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = updateNodeRequest(a)
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) == nil {
		if _, ok := raw["parent_id"]; ok {
			r.ParentIDSet = true
		}
	}
	return nil
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
		"success":   true,
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
	defer tx.Rollback()

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
	defer tx.Rollback()

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
// parseBoolFilter 解析布尔过滤参数
func parseBoolFilter(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return 1, true
	case "false", "0":
		return 0, true
	}
	return 0, false
}

// parseUserSort 校验排序参数
func parseUserSort(sort, order string) (string, string) {
	col := "created_at"
	switch sort {
	case "created_at", "last_login_at", "username":
		col = sort
	}
	dir := "DESC"
	if strings.EqualFold(order, "asc") {
		dir = "ASC"
	}
	return col, dir
}

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

		Debug("解析节点: 深度=%d, 类型=%d, 数据=%s", depth, n.Type, n.Data)

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
					INSERT INTO nodes (parent_id, type, title, url, favicon_url, remark, position, user_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, parentID, nodeTypeBookmark, node.Title, node.URL, favicon, remark, pos, userID)
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
		SELECT id, parent_id, user_id, type, title, url, favicon_url, remark, visibility, position, created_at, updated_at
		FROM nodes
		WHERE user_id = ?
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawNode struct {
		userID     int64
		id         int64
		parentID   sql.NullInt64
		nodeType   string
		title      string
		url        sql.NullString
		faviconURL sql.NullString
		remark     string
		visibility string
		position   int
		createdAt  string
		updatedAt  string
	}

	var rawNodes []rawNode
	for rows.Next() {
		var rn rawNode
		if err := rows.Scan(&rn.id, &rn.parentID, &rn.userID, &rn.nodeType, &rn.title, &rn.url, &rn.faviconURL, &rn.remark, &rn.visibility, &rn.position, &rn.createdAt, &rn.updatedAt); err != nil {
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
			ID:         rn.id,
			UserID:     rn.userID,
			Type:       rn.nodeType,
			Title:      rn.title,
			Remark:     rn.remark,
			Visibility: rn.visibility,
			Position:   rn.position,
			CreatedAt:  rn.createdAt,
			UpdatedAt:  rn.updatedAt,
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

func (s *server) loadPublicTree(ctx context.Context) ([]*node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, parent_id, user_id, type, title, url, favicon_url, remark, visibility, position, created_at, updated_at
		FROM nodes
		WHERE visibility = 'public'
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawNode struct {
		id         int64
		parentID   sql.NullInt64
		userID     int64
		nodeType   string
		title      string
		url        sql.NullString
		faviconURL sql.NullString
		remark     string
		visibility string
		position   int
		createdAt  string
		updatedAt  string
	}

	var rawNodes []rawNode
	for rows.Next() {
		var rn rawNode
		if err := rows.Scan(&rn.id, &rn.parentID, &rn.userID, &rn.nodeType, &rn.title, &rn.url, &rn.faviconURL, &rn.remark, &rn.visibility, &rn.position, &rn.createdAt, &rn.updatedAt); err != nil {
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
			ID:         rn.id,
			UserID:     rn.userID,
			Type:       rn.nodeType,
			Title:      rn.title,
			Remark:     rn.remark,
			Visibility: rn.visibility,
			Position:   rn.position,
			CreatedAt:  rn.createdAt,
			UpdatedAt:  rn.updatedAt,
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
			n.ParentID = nil
			nodesWithoutValidParent = append(nodesWithoutValidParent, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}

	if len(roots) == 0 {
		roots = nodesWithoutValidParent
	} else {
		roots = append(roots, nodesWithoutValidParent...)
	}

	sortNodes(roots)
	calculateBookmarkCounts(roots)

	if roots == nil {
		roots = []*node{}
	}
	return roots, nil
}

// filterNodesByUser 递归过滤掉指定用户的节点，返回其他用户的公开节点
func filterNodesByUser(nodes []*node, excludeUserID int64) []*node {
	var result []*node
	for _, n := range nodes {
		if n.UserID == excludeUserID {
			continue
		}
		filteredChildren := filterNodesByUser(n.Children, excludeUserID)
		if n.Type == "folder" && len(filteredChildren) == 0 {
			continue
		}
		n.Children = filteredChildren
		result = append(result, n)
	}
	return result
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

func (s *server) insertNode(ctx context.Context, userID int64, nType, title string, parentID *int64, url, favicon *string, remark, visibility string) (*node, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

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
		INSERT INTO nodes (parent_id, type, title, url, favicon_url, remark, position, user_id, visibility)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, parentID, nType, title, url, favicon, remark, nextPos, userID, visibility)
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
			Visibility: visibility,
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
	var current struct {
		Type     string
		ParentID sql.NullInt64
		Title    string
		URL      sql.NullString
		Favicon  sql.NullString
		Remark   string
		Visibility string
	}
	err := s.db.QueryRowContext(ctx, "SELECT type, parent_id, title, url, favicon_url, remark, visibility FROM nodes WHERE id = ? AND user_id = ?", id, userID).Scan(
		&current.Type, &current.ParentID, &current.Title, &current.URL, &current.Favicon, &current.Remark, &current.Visibility,
	)
	if err != nil {
		return err
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
			return ErrInvalidUpdate
		}
		targetTitle = title
		titleSet = true
	}

	if req.URL != nil {
		if current.Type != nodeTypeBookmark {
			return ErrInvalidUpdate
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

	targetVisibility := current.Visibility
	visibilitySet := false

	if req.Visibility != nil {
		if current.Type != nodeTypeBookmark {
			return ErrInvalidUpdate
		}
		v := strings.TrimSpace(*req.Visibility)
		if v != "public" && v != "private" {
			return ErrInvalidUpdate
		}
		targetVisibility = v
		visibilitySet = true
	}

	if req.ParentIDSet {
		if req.ParentID != nil {
			parentIDValue = *req.ParentID
			targetParentID = &parentIDValue
		} else {
			targetParentID = nil
		}
		parentSet = true
	}

	if !titleSet && !urlSet && !faviconSet && !parentSet && !remarkSet && !visibilitySet {
		return ErrInvalidUpdate
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if req.ParentIDSet && req.ParentID != nil {
		if *req.ParentID == id {
			return ErrCycleDetected
		}
		var parentType string
		if err = tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *req.ParentID, userID).Scan(&parentType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = ErrInvalidParent
			}
			return err
		}
		if parentType != nodeTypeFolder {
			return ErrInvalidParent
		}
		isCycle, cycErr := s.parentCreatesCycle(tx, userID, id, *req.ParentID)
		if cycErr != nil {
			return cycErr
		}
		if isCycle {
			return ErrCycleDetected
		}
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
			return ErrInvalidUpdate
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
		if visibilitySet {
			fields = append(fields, "visibility = ?")
			args = append(args, targetVisibility)
		}
	}
	if faviconSet {
		fields = append(fields, "favicon_url = ?")
		if faviconValid {
			args = append(args, targetFavicon)
		} else {
			args = append(args, nil)
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
	defer tx.Rollback()

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

// fetchMetadataOnce performs a single HTTP attempt. Returns (title, iconURL, retryable, err).
// retryable=true means the caller should retry; retryable=false with err=nil means a usable
// result was obtained even if it is a fallback value.
func (s *server) fetchMetadataOnce(rawURL, hostname, baseIconURL string) (string, string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", true, err
	}

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
	req = req.WithContext(ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", true, err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	if finalURL != rawURL {
		Debug("URL重定向: %s -> %s", rawURL, finalURL)
	}

	var bodyReader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			Debug("无法创建gzip读取器: %v", err)
		} else {
			defer gz.Close()
			bodyReader = gz
		}
	} else if strings.Contains(resp.Header.Get("Content-Encoding"), "deflate") {
		zl := flate.NewReader(resp.Body)
		defer zl.Close()
		bodyReader = zl
	}

	if resp.StatusCode == 403 {
		Debug("Received 403 Forbidden for URL: %s", rawURL)
		return "", "", true, fmt.Errorf("remote status 403 Forbidden")
	}

	if resp.StatusCode >= 400 {
		Debug("Received status %d for URL: %s", resp.StatusCode, rawURL)
		if hostname != "" {
			return hostname, baseIconURL, false, nil
		}
		return rawURL, baseIconURL, false, nil
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		if hostname != "" {
			return hostname, baseIconURL, false, nil
		}
		return rawURL, baseIconURL, false, nil
	}

	body, err := io.ReadAll(io.LimitReader(bodyReader, 1<<20))
	if err != nil {
		return "", "", true, err
	}

	var title string

	// 1. 正则提取 <title>
	titleRegex := regexp.MustCompile(`(?si)<title[^>]*>(.*?)</title>`)
	if matches := titleRegex.FindSubmatch(body); len(matches) > 1 {
		t := strings.Join(strings.Fields(html.UnescapeString(string(matches[1]))), " ")
		if t != "" {
			title = t
		}
	}

	// 2. og:title meta 标签
	if title == "" {
		metaTitleRegex := regexp.MustCompile(`(?si)<meta[^>]*property=["']og:title["'][^>]*content=["'](.*?)["']`)
		if metaMatches := metaTitleRegex.FindSubmatch(body); len(metaMatches) > 1 {
			t := strings.Join(strings.Fields(html.UnescapeString(string(metaMatches[1]))), " ")
			if t != "" {
				title = t
			}
		}
	}

	// 3. HTML 解析（同时用于提取图标）
	doc, parseErr := html.Parse(bytes.NewReader(body))
	if title == "" && parseErr == nil {
		if t := extractTitle(doc); t != "" {
			title = t
		}
	}

	// 4. getPageTitle 兜底
	if title == "" {
		title, _ = getPageTitle(rawURL)
	}

	// 5. 最终兜底：主机名或 URL
	if title == "" {
		if hostname != "" {
			title = hostname
		} else {
			title = finalURL
		}
	}

	iconURL := baseIconURL
	if parseErr == nil && doc != nil {
		if iconHref := extractIconHref(doc); iconHref != "" {
			if resolved, resolveErr := resolveURL(rawURL, iconHref); resolveErr == nil {
				iconURL = resolved
			}
		}
	}

	return title, iconURL, false, nil
}

func (s *server) fetchMetadata(rawURL string) (string, string, error) {
	parsedURL, parseErr := url.Parse(rawURL)
	var hostname string
	var baseIconURL string

	if parseErr == nil && parsedURL != nil {
		if parsedURL.Scheme == "" {
			parsedURL.Scheme = "https"
		}
		if parsedURL.Host != "" {
			hostname = parsedURL.Hostname()
			baseIconURL = parsedURL.Scheme + "://" + parsedURL.Host + "/favicon.ico"
		}
	}

	maxRetries := 2
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(time.Second + jitter)
		}

		title, iconURL, retry, err := s.fetchMetadataOnce(rawURL, hostname, baseIconURL)
		if err == nil {
			return title, iconURL, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}

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

// dupeKeyOptions 去重规范化选项
type dupeKeyOptions struct {
	crossFolder       bool // 不同文件夹计重复
	ignoreScheme       bool // HTTP/HTTPS 视为相同
	ignoreWWW          bool // www. 与无 www 视为相同
	ignoreTrailingSlash bool // 末尾 / 差异忽略
	ignoreQuery        bool // ? 参数差异忽略
}

// normalizeURLKey 将 URL 按用户选择的规则规范化后返回，用于去重比较。
func normalizeURLKey(rawURL string, opts dupeKeyOptions) string {
	s := rawURL
	if opts.ignoreScheme {
		s = strings.TrimPrefix(s, "https://")
		s = strings.TrimPrefix(s, "http://")
	}
	if opts.ignoreWWW {
		s = strings.Replace(s, "://www.", "://", 1)
		// 如果已经去掉了 scheme（ignoreScheme=true），也要处理没有 scheme 的情况
		if opts.ignoreScheme {
			s = strings.TrimPrefix(s, "www.")
		}
	}
	if opts.ignoreQuery {
		if idx := strings.Index(s, "?"); idx >= 0 {
			s = s[:idx]
		}
	}
	if opts.ignoreTrailingSlash {
		s = strings.TrimRight(s, "/")
	}
	return s
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

	var userCount int
	if err := s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		config["no_users"] = "true"
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
	if req.Key == "allow_register" || req.Key == "require_login" || req.Key == "default_template" {
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

// handleCheckVersion 检查是否有新版本（结果缓存 24 小时）
func (s *server) handleCheckVersion(w http.ResponseWriter, r *http.Request) {
	s.verCacheMu.Lock()
	defer s.verCacheMu.Unlock()

	if s.verCache != nil && time.Since(s.verCache.checkedAt) < 24*time.Hour {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"current":      appVersion,
			"latest":       s.verCache.latestVersion,
			"download_url": s.verCache.downloadURL,
			"has_update":   isNewerVersion(s.verCache.latestVersion, appVersion),
		})
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET",
		"https://api.github.com/repos/TechFunWay/bookmarks/releases/latest", nil)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"current": appVersion, "has_update": false})
		return
	}
	req.Header.Set("User-Agent", "bookmarks-app/"+appVersion)
	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		respondJSON(w, http.StatusOK, map[string]interface{}{"current": appVersion, "has_update": false})
		return
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil || release.TagName == "" {
		respondJSON(w, http.StatusOK, map[string]interface{}{"current": appVersion, "has_update": false})
		return
	}

	s.verCache = &versionCheckCache{
		latestVersion: release.TagName,
		downloadURL:   release.HTMLURL,
		checkedAt:     time.Now(),
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"current":      appVersion,
		"latest":       release.TagName,
		"download_url": release.HTMLURL,
		"has_update":   isNewerVersion(release.TagName, appVersion),
	})
}

// isNewerVersion 比较两个 semver 字符串，latest > current 返回 true
func isNewerVersion(latest, current string) bool {
	parse := func(v string) [3]int {
		v = strings.TrimPrefix(v, "v")
		parts := strings.SplitN(v, ".", 3)
		var nums [3]int
		for i, p := range parts {
			if i >= 3 {
				break
			}
			fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums
	}
	l, c := parse(latest), parse(current)
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

// handleCheckDuplicates 检查重复书签
//
// 支持查询参数让用户自定义重复判定规则：
//   ignore_scheme=true      忽略 http/https 协议前缀
//   ignore_www=true          忽略 www. 主机名前缀
//   ignore_trailing_slash=true  忽略路径末尾的 /
//   ignore_query=true        忽略 ? 查询参数
func (s *server) handleCheckDuplicates(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	q := r.URL.Query()

	opts := dupeKeyOptions{
		crossFolder:         q.Get("cross_folder") == "true",
		ignoreScheme:        q.Get("ignore_scheme") == "true",
		ignoreWWW:           q.Get("ignore_www") == "true",
		ignoreTrailingSlash: q.Get("ignore_trailing_slash") == "true",
		ignoreQuery:         q.Get("ignore_query") == "true",
	}

	// 查询所有书签
	rows, err := s.db.Query(`
		SELECT b.id, b.title, b.url, b.parent_id, b.position
		FROM nodes b
		WHERE b.user_id = ? AND b.type = 'bookmark'
		ORDER BY b.url, b.id
	`, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	type bookmarkInfo struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		ParentID int64  `json:"parent_id"`
		Position int    `json:"position"`
		Path     string `json:"path"`
	}

	// 按规范化 URL 分组
	urlBookmarks := make(map[string][]bookmarkInfo)
	hasHTTP := make(map[string]bool)  // 记录该规范化 key 下是否有 http 版本
	hasHTTPS := make(map[string]bool) // 记录是否有 https 版本
	for rows.Next() {
		var b bookmarkInfo
		if err := rows.Scan(&b.ID, &b.Title, &b.URL, &b.ParentID, &b.Position); err != nil {
			continue
		}
		key := normalizeURLKey(b.URL, opts)
		if !opts.crossFolder {
			key = fmt.Sprintf("%d|%s", b.ParentID, key)
		}
		urlBookmarks[key] = append(urlBookmarks[key], b)
		if opts.ignoreScheme {
			if strings.HasPrefix(b.URL, "https://") {
				hasHTTPS[key] = true
			} else {
				hasHTTP[key] = true
			}
		}
	}

	// 查找重复的URL（数量大于1）
	var duplicates []struct {
		URL              string         `json:"url"`
		Bookmarks        []bookmarkInfo `json:"bookmarks"`
		HasSchemeMismatch bool          `json:"hasSchemeMismatch"`
	}

	totalBookmarks := 0
	duplicateBookmarksCount := 0

	for key, bookmarks := range urlBookmarks {
		totalBookmarks += len(bookmarks)

		if len(bookmarks) > 1 {
			// 为每个书签构建路径
			for i := range bookmarks {
				path, err := s.buildBookmarkPath(bookmarks[i].ParentID, userID)
				if err != nil {
					path = ""
				}
				bookmarks[i].Path = path
			}

			// 展示 URL 用去协议后的规范化 key（http 和 https 版本都能看懂）
			displayURL := key
			duplicates = append(duplicates, struct {
				URL              string         `json:"url"`
				Bookmarks        []bookmarkInfo `json:"bookmarks"`
				HasSchemeMismatch bool          `json:"hasSchemeMismatch"`
			}{
				URL:              displayURL,
				Bookmarks:        bookmarks,
				HasSchemeMismatch: hasHTTP[key] && hasHTTPS[key],
			})
			duplicateBookmarksCount += len(bookmarks)
		}
	}

	// 按重复个数从大到小排序
	sort.Slice(duplicates, func(i, j int) bool {
		return len(duplicates[i].Bookmarks) > len(duplicates[j].Bookmarks)
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"duplicates":              duplicates,
			"totalBookmarks":          totalBookmarks,
			"duplicateCount":          len(duplicates),
			"duplicateBookmarksCount": duplicateBookmarksCount,
		},
	})
}

// classifyTransportError 将传输层错误细分为 timeout/dns/connection/tls。
func classifyTransportError(err error) (errorType, reason string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "请求超时"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout", "请求超时"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns", "DNS 解析失败"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such host") || strings.Contains(msg, "server misbehaving") {
		return "dns", "DNS 解析失败"
	}
	if strings.Contains(msg, "tls:") || strings.Contains(msg, "certificate") || strings.Contains(msg, "x509:") {
		return "tls", "TLS/证书错误"
	}
	return "connection", "连接失败"
}

// classifyLinkResult 根据 HTTP 状态码和传输层错误把链接分类为 ok / dead / suspicious。
// dead = 确定失效（仅 404、410）；suspicious = 疑似失效（其他 4xx、5xx、全部网络错误）。
func classifyLinkResult(code int, err error) (category, reason, errorType string) {
	if err != nil {
		et, r := classifyTransportError(err)
		return "suspicious", r, et
	}
	switch {
	case code >= 200 && code < 400:
		return "ok", "", ""
	case code == 404 || code == 410:
		return "dead", http.StatusText(code), ""
	case code >= 400:
		return "suspicious", http.StatusText(code), ""
	default:
		return "suspicious", http.StatusText(code), ""
	}
}

const linkCheckUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// noRedirectClient 不自动跟随重定向的 HTTP 客户端，用于捕获首次请求状态码。
var noRedirectClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// checkURLWithBrowser 用 headless Chrome 检测 URL，可绕过 Cloudflare 等 JS 挑战。
// 返回最终 HTTP 状态码（0 表示超时或错误）。
func checkURLWithBrowser(ctx context.Context, rawURL string) int {
	ctx, cancel := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var statusCode int
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 监听网络响应，获取 HTTP 状态码
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				if e, ok := ev.(*network.EventResponseReceived); ok {
					if string(e.Type) == "Document" {
						statusCode = int(e.Response.Status)
					}
				}
			})
			return nil
		}),
		chromedp.Navigate(rawURL),
		chromedp.Sleep(3*time.Second), // 等待 JS 执行（Cloudflare 挑战）
	)
	if err != nil {
		return 0
	}
	return statusCode
}

// checkURL 检测单个 URL 是否可访问。
//
// 直接用 GET（与浏览器实际打开网页一致），不使用 HEAD：很多服务器/CDN 对
// HEAD 处理不可靠——会返回 404/403，或干脆超时不响应（即便 GET 完全正常，
// 如 example.com、example.com），用 HEAD 反而更慢、更易误判。
// 传输错误（多为瞬时超时/网络抖动）会重试一次，进一步降低误判。
// 重定向策略：记录首次请求状态码，若首次为 2xx/3xx 则算「能访问」，避免
// 中间页（如 /act/redirect → 404）误判为失效。
func (s *server) checkURL(ctx context.Context, rawURL string) (code int, category, reason, errorType string) {
	doCheck := func() (int, string, string, string, bool) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			cat, rsn, et := classifyLinkResult(0, err)
			return 0, cat, rsn, et, true
		}
		req.Header.Set("User-Agent", linkCheckUserAgent)

		// 第一次请求：不跟随重定向，捕获真实状态码
		firstResp, err := noRedirectClient.Do(req)
		if err != nil {
			cat, rsn, et := classifyLinkResult(0, err)
			return 0, cat, rsn, et, true
		}
		firstResp.Body.Close()
		firstCode := firstResp.StatusCode

		// 若首次请求成功（2xx/3xx），直接返回——不管后续重定向变成什么
		if firstCode >= 200 && firstCode < 400 {
			cat, rsn, et := classifyLinkResult(firstCode, nil)
			return firstCode, cat, rsn, et, false
		}

		// 首次非 2xx/3xx，跟随重定向拿最终状态
		finalReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			cat, rsn, et := classifyLinkResult(firstCode, nil)
			return firstCode, cat, rsn, et, false
		}
		finalReq.Header.Set("User-Agent", linkCheckUserAgent)
		finalResp, err := s.httpClient.Do(finalReq)
		if err != nil {
			cat, rsn, et := classifyLinkResult(firstCode, nil)
			return firstCode, cat, rsn, et, false
		}
		finalResp.Body.Close()
		cat, rsn, et := classifyLinkResult(finalResp.StatusCode, nil)
		return finalResp.StatusCode, cat, rsn, et, false
	}

	code, category, reason, errorType, retryable := doCheck()
	if retryable && ctx.Err() == nil {
		// 瞬时网络错误重试一次（仅当整体上下文尚未取消）
		code, category, reason, errorType, _ = doCheck()
	}

	// HTTP 返回 403 时，用 headless Chrome 重试——可能是 Cloudflare JS 挑战
	if code == 403 && ctx.Err() == nil {
		browserCode := checkURLWithBrowser(ctx, rawURL)
		if browserCode >= 200 && browserCode < 400 {
			return browserCode, "ok", "", ""
		}
		if browserCode > 0 {
			cat, rsn, et := classifyLinkResult(browserCode, nil)
			return browserCode, cat, rsn, et
		}
	}

	return code, category, reason, errorType
}

type checkLinksRequest struct {
	Bookmarks []struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	} `json:"bookmarks"`
}

type linkResult struct {
	ID        int64  `json:"id"`
	Code      int    `json:"code"`
	Category  string `json:"category"`
	Reason    string `json:"reason"`
	ErrorType string `json:"error_type"`
}

// handleCheckLinks 并发检测一批书签链接是否失效。前端分批调用，实时展示进度。
func (s *server) handleCheckLinks(w http.ResponseWriter, r *http.Request) {
	var req checkLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if len(req.Bookmarks) == 0 {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    map[string]interface{}{"results": []linkResult{}},
		})
		return
	}
	if len(req.Bookmarks) > 100 {
		respondError(w, http.StatusBadRequest, fmt.Errorf("单批最多检测 100 个，收到 %d 个", len(req.Bookmarks)))
		return
	}

	const concurrency = 10
	sem := make(chan struct{}, concurrency)
	results := make([]linkResult, len(req.Bookmarks))
	var wg sync.WaitGroup

	for i, b := range req.Bookmarks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, id int64, rawURL string) {
			defer wg.Done()
			defer func() { <-sem }()
			code, category, reason, errorType := s.checkURL(r.Context(), rawURL)
			results[idx] = linkResult{ID: id, Code: code, Category: category, Reason: reason, ErrorType: errorType}
		}(i, b.ID, b.URL)
	}
	wg.Wait()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    map[string]interface{}{"results": results},
	})
}

// buildBookmarkPath 构建书签路径
func (s *server) buildBookmarkPath(parentID int64, userID int64) (string, error) {
	if parentID == 0 {
		return "/", nil
	}

	var pathParts []string
	currentID := parentID

	for currentID != 0 {
		var name string
		var pid int64
		err := s.db.QueryRow("SELECT title, parent_id FROM nodes WHERE id = ? AND user_id = ?", currentID, userID).Scan(&name, &pid)
		if err != nil {
			break
		}

		pathParts = append([]string{name}, pathParts...)
		currentID = pid
	}

	if len(pathParts) == 0 {
		return "/", nil
	}

	return "/" + strings.Join(pathParts, "/"), nil
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
	var whereParts []string
	var args []interface{}

	if search != "" {
		whereParts = append(whereParts, "(username LIKE ? OR nickname LIKE ? OR email LIKE ?)")
		searchPattern := "%" + search + "%"
		args = []interface{}{searchPattern, searchPattern, searchPattern}
	}

	if isActiveStr := r.URL.Query().Get("is_active"); isActiveStr != "" {
		if v, ok := parseBoolFilter(isActiveStr); ok {
			whereParts = append(whereParts, "is_active = ?")
			args = append(args, v)
		}
	}
	if isAdminStr := r.URL.Query().Get("is_admin"); isAdminStr != "" {
		if v, ok := parseBoolFilter(isAdminStr); ok {
			whereParts = append(whereParts, "is_admin = ?")
			args = append(args, v)
		}
	}

	if len(whereParts) > 0 {
		whereClause = "WHERE " + strings.Join(whereParts, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM users " + whereClause
	var total int64
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	sortCol, sortOrder := parseUserSort(r.URL.Query().Get("sort"), r.URL.Query().Get("order"))
	query := "SELECT id, username, nickname, email, avatar, is_active, is_admin, last_login_at, created_at FROM users " +
		whereClause + fmt.Sprintf(" ORDER BY %s %s LIMIT ? OFFSET ?", sortCol, sortOrder)
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
		var avatar, lastLogin sql.NullString
		err := rows.Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &avatar, &isActive, &isAdmin, &lastLogin, &u.CreatedAt)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		if avatar.Valid {
			u.Avatar = &avatar.String
		}
		if lastLogin.Valid {
			s := lastLogin.String
			u.LastLoginAt = &s
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
	var avatar, lastLogin sql.NullString
	err = s.db.QueryRow("SELECT id, username, nickname, email, avatar, is_active, is_admin, last_login_at, created_at FROM users WHERE id = ?", userID).
		Scan(&u.ID, &u.Username, &u.Nickname, &u.Email, &avatar, &isActive, &isAdmin, &lastLogin, &u.CreatedAt)
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
	if lastLogin.Valid {
		s := lastLogin.String
		u.LastLoginAt = &s
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

	s.logAudit(r.Context(), currentUserID, "", "user_update", "user", targetUserID, "修改用户信息", r.RemoteAddr)

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

	s.logAudit(r.Context(), currentUserID, "", "user_delete", "user", targetUserID, "删除用户", r.RemoteAddr)

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

	currentUserID := getUserID(r)
	s.logAudit(r.Context(), currentUserID, "", "password_reset", "user", targetUserID, "管理员重置密码", r.RemoteAddr)

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

	s.logAudit(r.Context(), currentUserID, "", "batch_"+req.Action, "user", 0,
		fmt.Sprintf("批量操作: %s, 用户数: %d", req.Action, len(req.UserIDs)), r.RemoteAddr)

	respondJSON(w, http.StatusOK, map[string]string{"message": "success"})
}

// handleUserStats 返回当前登录用户自己的统计信息
func (s *server) handleUserStats(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var stats struct {
		TotalBookmarks  int `json:"total_bookmarks"`
		TotalFolders    int `json:"total_folders"`
		PublicBookmarks int `json:"public_bookmarks"`
	}
	queries := []struct {
		dest *int
		sql  string
	}{
		{&stats.TotalBookmarks, "SELECT COUNT(*) FROM nodes WHERE type = 'bookmark' AND user_id = ?"},
		{&stats.TotalFolders, "SELECT COUNT(*) FROM nodes WHERE type = 'folder' AND user_id = ?"},
		{&stats.PublicBookmarks, "SELECT COUNT(*) FROM nodes WHERE type = 'bookmark' AND visibility = 'public' AND user_id = ?"},
	}
	for _, q := range queries {
		if err := s.db.QueryRowContext(r.Context(), q.sql, userID).Scan(q.dest); err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Errorf("stats query failed: %w", err))
			return
		}
	}
	respondJSON(w, http.StatusOK, stats)
}

// handleAdminStats 返回后台管理统计信息
func (s *server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	var stats struct {
		TotalUsers      int `json:"total_users"`
		TotalBookmarks  int `json:"total_bookmarks"`
		TotalFolders    int `json:"total_folders"`
		PublicBookmarks int `json:"public_bookmarks"`
		TotalNodes      int `json:"total_nodes"`
	}

	queries := []struct {
		dest *int
		sql  string
	}{
		{&stats.TotalUsers, "SELECT COUNT(*) FROM users"},
		{&stats.TotalBookmarks, "SELECT COUNT(*) FROM nodes WHERE type = 'bookmark'"},
		{&stats.TotalFolders, "SELECT COUNT(*) FROM nodes WHERE type = 'folder'"},
		{&stats.PublicBookmarks, "SELECT COUNT(*) FROM nodes WHERE type = 'bookmark' AND visibility = 'public'"},
		{&stats.TotalNodes, "SELECT COUNT(*) FROM nodes"},
	}
	for _, q := range queries {
		if err := s.db.QueryRowContext(r.Context(), q.sql).Scan(q.dest); err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Errorf("stats query failed: %w", err))
			return
		}
	}

	respondJSON(w, http.StatusOK, stats)
}

// handleCreateUser 管理员创建用户
func (s *server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
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

	doubleHashedPassword := utils.MD5Hash(req.Password, "bookmarks")

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}

	isAdmin := 0
	if req.IsAdmin {
		isAdmin = 1
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

	userID, err := result.LastInsertId()
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

	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	adminUserID := getUserID(r)
	s.logAudit(r.Context(), adminUserID, "", "user_create", "user", userID,
		fmt.Sprintf("管理员创建用户: %s", req.Username), r.RemoteAddr)

	user := &user{
		ID:       userID,
		Username: req.Username,
		Nickname: nickname,
		Email:    req.Email,
		IsAdmin:  req.IsAdmin,
		IsActive: true,
	}

	respondJSON(w, http.StatusCreated, user)
}

// handleAdminGetUserTree 查看任意用户书签树
func (s *server) handleAdminGetUserTree(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	tree, err := s.loadTree(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, tree)
}

// handleAdminDeleteNode 删除任意用户书签
func (s *server) handleAdminDeleteNode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var ownerUserID int64
	err = s.db.QueryRowContext(r.Context(), "SELECT user_id FROM nodes WHERE id = ?", id).Scan(&ownerUserID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, errors.New("node not found"))
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	var nodeType string
	err = s.db.QueryRowContext(r.Context(), "SELECT type FROM nodes WHERE id = ?", id).Scan(&nodeType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
		WITH RECURSIVE subtree(id, type) AS (
			SELECT id, type FROM nodes WHERE id = ? AND user_id = ?
			UNION ALL
			SELECT n.id, n.type FROM nodes n
			INNER JOIN subtree s ON n.parent_id = s.id
			WHERE n.user_id = ?
		)
		SELECT id, type FROM subtree
	`, id, ownerUserID, ownerUserID)
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
		if ntype == "folder" {
			folders++
		} else {
			bookmarks++
		}
	}
	rows.Close()

	idStrs := make([]string, len(allIDs))
	for i, nid := range allIDs {
		idStrs[i] = strconv.FormatInt(nid, 10)
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE id IN ("+strings.Join(idStrs, ",")+")"); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	adminUserID := getUserID(r)
	s.logAudit(r.Context(), adminUserID, "", "admin_delete_node", "node", id,
		fmt.Sprintf("管理员删除节点, 文件夹: %d, 书签: %d", folders, bookmarks), r.RemoteAddr)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "删除成功",
		"folders":   folders,
		"bookmarks": bookmarks,
	})
}

// handleAdminUpdateNode 管理员更新任意用户节点
func (s *server) handleAdminUpdateNode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var ownerUserID int64
	err = s.db.QueryRowContext(r.Context(), "SELECT user_id FROM nodes WHERE id = ?", id).Scan(&ownerUserID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, errors.New("node not found"))
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	var req struct {
		FaviconURL *string `json:"favicon_url"`
		Title      *string `json:"title"`
		URL        *string `json:"url"`
		Remark     *string `json:"remark"`
		Visibility *string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	if req.FaviconURL == nil && req.Title == nil && req.URL == nil && req.Remark == nil && req.Visibility == nil {
		respondError(w, http.StatusBadRequest, errors.New("no fields to update"))
		return
	}

	fields := make([]string, 0, 5)
	args := make([]any, 0, 5)

	if req.FaviconURL != nil {
		favicon := strings.TrimSpace(*req.FaviconURL)
		fields = append(fields, "favicon_url = ?")
		if favicon == "" {
			args = append(args, nil)
		} else {
			args = append(args, favicon)
		}
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			respondError(w, http.StatusBadRequest, errors.New("title cannot be empty"))
			return
		}
		fields = append(fields, "title = ?")
		args = append(args, title)
	}

	if req.URL != nil {
		targetURL := strings.TrimSpace(*req.URL)
		if targetURL == "" {
			respondError(w, http.StatusBadRequest, errors.New("url cannot be empty"))
			return
		}
		normalizedURL, err := normalizeURL(targetURL)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Errorf("invalid url: %w", err))
			return
		}
		fields = append(fields, "url = ?")
		args = append(args, normalizedURL)
	}

	if req.Remark != nil {
		fields = append(fields, "remark = ?")
		args = append(args, *req.Remark)
	}

	if req.Visibility != nil {
		vis := *req.Visibility
		if vis != "public" && vis != "private" {
			respondError(w, http.StatusBadRequest, errors.New("visibility must be 'public' or 'private'"))
			return
		}
		fields = append(fields, "visibility = ?")
		args = append(args, vis)
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE nodes SET %s WHERE id = ?", strings.Join(fields, ", "))
	if _, err = s.db.ExecContext(r.Context(), query, args...); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	adminUserID := getUserID(r)
	s.logAudit(r.Context(), adminUserID, "", "admin_update_node", "node", id, "管理员更新节点", r.RemoteAddr)

	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "更新成功"})
}

// handleGetAuditLog 查询操作日志
func (s *server) handleGetAuditLog(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	whereClause := "WHERE 1=1"
	var args []interface{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		uid, parseErr := strconv.ParseInt(userIDStr, 10, 64)
		if parseErr != nil || uid < 0 {
			respondError(w, http.StatusBadRequest, errors.New("user_id must be a non-negative integer"))
			return
		}
		whereClause += " AND user_id = ?"
		args = append(args, uid)
	}
	if action := r.URL.Query().Get("action"); action != "" {
		whereClause += " AND action = ?"
		args = append(args, action)
	}
	if search := r.URL.Query().Get("search"); search != "" {
		whereClause += " AND detail LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		whereClause += " AND created_at >= ?"
		args = append(args, dateFrom)
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		whereClause += " AND created_at <= ?"
		args = append(args, dateTo+" 23:59:59")
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM audit_log " + whereClause
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	query := "SELECT id, user_id, username, action, target_type, target_id, detail, ip_address, created_at FROM audit_log " +
		whereClause + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	var logs []auditLogEntry
	for rows.Next() {
		var log auditLogEntry
		if err := rows.Scan(&log.ID, &log.UserID, &log.Username, &log.Action, &log.TargetType, &log.TargetID, &log.Detail, &log.IPAddress, &log.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		logs = append(logs, log)
	}

	respondJSON(w, http.StatusOK, auditLogResponse{
		Logs:  logs,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// handleExportAuditLog 导出操作日志为CSV
func (s *server) handleExportAuditLog(w http.ResponseWriter, r *http.Request) {
	whereClause := "WHERE 1=1"
	var args []interface{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		uid, parseErr := strconv.ParseInt(userIDStr, 10, 64)
		if parseErr != nil || uid < 0 {
			respondError(w, http.StatusBadRequest, errors.New("user_id must be a non-negative integer"))
			return
		}
		whereClause += " AND user_id = ?"
		args = append(args, uid)
	}
	if action := r.URL.Query().Get("action"); action != "" {
		whereClause += " AND action = ?"
		args = append(args, action)
	}
	if search := r.URL.Query().Get("search"); search != "" {
		whereClause += " AND detail LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		whereClause += " AND created_at >= ?"
		args = append(args, dateFrom)
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		whereClause += " AND created_at <= ?"
		args = append(args, dateTo+" 23:59:59")
	}

	query := "SELECT id, user_id, username, action, target_type, target_id, detail, ip_address, created_at FROM audit_log " +
		whereClause + " ORDER BY created_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-log.csv"`)

	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"id", "user_id", "username", "action", "target_type", "target_id", "detail", "ip_address", "created_at",
	}); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	for rows.Next() {
		var (
			id         int64
			userID     int64
			username   sql.NullString
			action     string
			targetType sql.NullString
			targetID   sql.NullInt64
			detail     sql.NullString
			ipAddress  sql.NullString
			createdAt  string
		)
		if err := rows.Scan(&id, &userID, &username, &action, &targetType, &targetID, &detail, &ipAddress, &createdAt); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		row := []string{
			strconv.FormatInt(id, 10),
			strconv.FormatInt(userID, 10),
			username.String,
			action,
			targetType.String,
			func() string {
				if targetID.Valid {
					return strconv.FormatInt(targetID.Int64, 10)
				}
				return ""
			}(),
			detail.String,
			ipAddress.String,
			createdAt,
		}
		if err := writer.Write(row); err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
}

// handleAdminExportUser 导出任意用户书签
func (s *server) handleAdminExportUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	tree, err := s.loadTree(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bookmarks_export_user_%d.json"`, userID))
	json.NewEncoder(w).Encode(tree)
}

// handleAdminCreateFolder 管理员为任意用户创建文件夹
func (s *server) handleAdminCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   int64  `json:"user_id"`
		Title    string `json:"title"`
		ParentID *int64 `json:"parent_id"`
		Icon     string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	if !s.userExists(r.Context(), req.UserID) {
		respondError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondError(w, http.StatusBadRequest, errors.New("title is required"))
		return
	}
	var icon *string
	if req.Icon != "" {
		icon = &req.Icon
	}
	newNode, err := s.insertNode(r.Context(), req.UserID, nodeTypeFolder, req.Title, req.ParentID, nil, icon, "", "private")
	if err != nil {
		if errors.Is(err, ErrInvalidParent) || errors.Is(err, ErrDuplicateFolderName) {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	adminUserID := getUserID(r)
	s.logAudit(r.Context(), adminUserID, "", "admin_create_folder", "folder", newNode.ID,
		fmt.Sprintf("管理员为用户 #%d 创建文件夹: %s", req.UserID, req.Title), r.RemoteAddr)
	respondJSON(w, http.StatusCreated, newNode)
}

// handleAdminCreateBookmark 管理员为任意用户创建书签
func (s *server) handleAdminCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     int64  `json:"user_id"`
		URL        string `json:"url"`
		Title      string `json:"title"`
		ParentID   *int64 `json:"parent_id"`
		FaviconURL string `json:"favicon_url"`
		Visibility string `json:"visibility"`
		Remark     string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	if !s.userExists(r.Context(), req.UserID) {
		respondError(w, http.StatusNotFound, errors.New("user not found"))
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
	title := req.Title
	favicon := req.FaviconURL
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
	visibility := "private"
	if req.Visibility != "" {
		visibility = req.Visibility
	}
	var faviconPtr *string
	if favicon != "" {
		faviconPtr = &favicon
	}
	remark := req.Remark
	newNode, err := s.insertNode(r.Context(), req.UserID, nodeTypeBookmark, title, req.ParentID, &normalizedURL, faviconPtr, remark, visibility)
	if err != nil {
		if errors.Is(err, ErrInvalidParent) || errors.Is(err, ErrDuplicateFolderName) {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	adminUserID := getUserID(r)
	s.logAudit(r.Context(), adminUserID, "", "admin_create_bookmark", "bookmark", newNode.ID,
		fmt.Sprintf("管理员为用户 #%d 创建书签: %s", req.UserID, req.Title), r.RemoteAddr)
	respondJSON(w, http.StatusCreated, newNode)
}

// handleAdminReorderNodes 管理员为任意用户排序节点
func (s *server) handleAdminReorderNodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     int64   `json:"user_id"`
		ParentID   *int64  `json:"parent_id"`
		OrderedIDs []int64 `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	if !s.userExists(r.Context(), req.UserID) {
		respondError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	if len(req.OrderedIDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("ordered_ids cannot be empty"))
		return
	}
	if err := s.reorderNodes(r.Context(), req.UserID, req.ParentID, req.OrderedIDs); err != nil {
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

// optionalAuthMiddleware 可选认证中间件（支持免登录模式）
func (s *server) optionalAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != "" {
			var userID int64
			var isActive int
			err := s.db.QueryRow("SELECT id, is_active FROM users WHERE token = ?", token).Scan(&userID, &isActive)
			if err == nil && isActive == 1 {
				ctx := context.WithValue(r.Context(), userContextKey, userID)
				next(w, r.WithContext(ctx))
				return
			}
		}

		var requireLogin string
		err := s.db.QueryRow("SELECT value FROM sys_config WHERE user_id = 0 AND key = 'require_login'").Scan(&requireLogin)
		if err != nil || requireLogin != "false" {
			respondError(w, http.StatusUnauthorized, errors.New("未提供认证token"))
			return
		}

		var adminID int64
		err = s.db.QueryRow("SELECT id FROM users WHERE is_admin = 1 AND is_active = 1 LIMIT 1").Scan(&adminID)
		if err != nil {
			respondError(w, http.StatusUnauthorized, errors.New("未找到管理员账户"))
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, adminID)
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

func (s *server) apiKeyAuthMiddlewareForChi(next http.Handler) http.Handler {
	return s.apiKeyAuthMiddleware(next.ServeHTTP)
}

func (s *server) tokenAuthMiddlewareChi(next http.Handler) http.Handler {
	return s.tokenAuthMiddleware(next.ServeHTTP)
}

func (s *server) adminMiddlewareChi(next http.Handler) http.Handler {
	return s.adminMiddleware(next.ServeHTTP)
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

// userExists reports whether the users table contains a row with the
// given id. Used by admin endpoints to fail fast when a target user is
// missing instead of creating orphan rows or no-op updates.
func (s *server) userExists(ctx context.Context, userID int64) bool {
	if userID <= 0 {
		return false
	}
	var found int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM users WHERE id = ?", userID).Scan(&found)
	return err == nil
}

// logAudit 记录审计日志
func (s *server) logAudit(ctx context.Context, userID int64, username, action, targetType string, targetID int64, detail, ip string) {
	s.db.ExecContext(ctx, `INSERT INTO audit_log (user_id, username, action, target_type, target_id, detail, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, username, action, targetType, targetID, detail, ip)
	// 保留最近 10000 条
	s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE id <= (SELECT MAX(id) - 10000 FROM audit_log)`)
}

// adminMiddleware 管理员权限检查中间件
func (s *server) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)

		var isAdmin int
		err := s.db.QueryRow("SELECT is_admin FROM users WHERE id = ?", userID).Scan(&isAdmin)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusForbidden, errors.New("需要管理员权限"))
				return
			}
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

	s.logAudit(r.Context(), userID, req.Username, "user_register", "user", userID, "用户注册", r.RemoteAddr)

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

	if _, err = s.db.Exec("UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?", dbUser.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
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

	s.logAudit(r.Context(), dbUser.ID, dbUser.Username, "login", "user", dbUser.ID, "用户登录", r.RemoteAddr)

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
		var uid int64
		var uname string
		s.db.QueryRow("SELECT id, username FROM users WHERE token = ?", token).Scan(&uid, &uname)
		_, err := s.db.Exec("UPDATE users SET token = NULL WHERE token = ?", token)
		if err != nil {
			Debug("清除token失败: %v", err)
		}
		if uid > 0 {
			s.logAudit(r.Context(), uid, uname, "logout", "user", uid, "用户登出", r.RemoteAddr)
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "登出成功"})
}

// handleGetCurrentUser 获取当前登录用户信息
func (s *server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var dbUser struct {
		ID          int64
		Username    string
		Nickname    string
		Email       string
		Avatar      sql.NullString
		IsActive    int
		IsAdmin     int
		APIKey      sql.NullString
		LastLoginAt sql.NullString
		CreatedAt   string
	}

	err := s.db.QueryRow(`
		SELECT id, username, nickname, email, avatar, is_active, is_admin, api_key, last_login_at, created_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&dbUser.ID, &dbUser.Username, &dbUser.Nickname,
		&dbUser.Email, &dbUser.Avatar, &dbUser.IsActive, &dbUser.IsAdmin, &dbUser.APIKey, &dbUser.LastLoginAt, &dbUser.CreatedAt,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	// 检查是否设置了安全问题
	var hasSecurityQuestions bool
	err = s.db.QueryRow("SELECT COUNT(*) > 0 FROM security_questions WHERE user_id = ?", userID).Scan(&hasSecurityQuestions)
	if err != nil {
		// 查询失败时不影响用户信息返回
		hasSecurityQuestions = false
	}

	user := &user{
		ID:                   dbUser.ID,
		Username:             dbUser.Username,
		Nickname:             dbUser.Nickname,
		Email:                dbUser.Email,
		IsActive:             dbUser.IsActive == 1,
		IsAdmin:              dbUser.IsAdmin == 1,
		CreatedAt:            dbUser.CreatedAt,
		HasSecurityQuestions: hasSecurityQuestions,
	}
	if dbUser.Avatar.Valid {
		user.Avatar = &dbUser.Avatar.String
	}
	if dbUser.APIKey.Valid {
		user.APIKey = &dbUser.APIKey.String
	}
	if dbUser.LastLoginAt.Valid {
		s := dbUser.LastLoginAt.String
		user.LastLoginAt = &s
	}

	respondJSON(w, http.StatusOK, user)
}

// handleCheckAuth 检查登录状态
func (s *server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	var requireLogin string
	s.db.QueryRow("SELECT value FROM sys_config WHERE user_id = 0 AND key = 'require_login'").Scan(&requireLogin)

	if token == "" {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
			"require_login": requireLogin != "false",
		})
		return
	}

	var userID int64
	err := s.db.QueryRow("SELECT id FROM users WHERE token = ? AND is_active = 1", token).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"authenticated": false,
				"require_login": requireLogin != "false",
			})
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user_id":       userID,
		"require_login": requireLogin != "false",
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

	s.logAudit(r.Context(), userID, "", "password_change", "user", userID, "修改密码", r.RemoteAddr)

	respondJSON(w, http.StatusOK, map[string]string{"message": "密码修改成功"})
}

// handleSetSecurityQuestions 设置安全问题
func (s *server) handleSetSecurityQuestions(w http.ResponseWriter, r *http.Request) {
	var req logic.SecurityQuestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	userID := getUserID(r)

	if err := s.securityQuestions.SetSecurityQuestions(userID, &req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "安全问题设置成功"})
}

// handleGetSecurityQuestions 获取安全问题（仅返回问题，不返回答案）
func (s *server) handleGetSecurityQuestions(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	response, err := s.securityQuestions.GetSecurityQuestions(userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, response)
}

// handleGetSecurityQuestionsForReset 获取用户的安全问题（用于重置密码，需要用户名）
func (s *server) handleGetSecurityQuestionsForReset(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		respondError(w, http.StatusBadRequest, errors.New("用户名不能为空"))
		return
	}

	questions, err := s.securityQuestions.GetSecurityQuestionsForReset(username)
	if err != nil {
		if err.Error() == "用户不存在" {
			respondError(w, http.StatusNotFound, err)
		} else if err.Error() == "该用户未设置安全问题，无法通过此方式重置密码" {
			respondError(w, http.StatusForbidden, err)
		} else {
			respondError(w, http.StatusInternalServerError, err)
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"username":  username,
		"questions": *questions,
	})
}

// handleVerifyAndResetPassword 验证安全问题并重置密码
func (s *server) handleVerifyAndResetPassword(w http.ResponseWriter, r *http.Request) {
	var req logic.VerifySecurityQuestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}

	// 验证安全问题
	if err := s.securityQuestions.VerifyAndResetPassword(&req); err != nil {
		if err.Error() == "用户不存在" {
			respondError(w, http.StatusNotFound, err)
		} else if err.Error() == "安全问题答案错误" || err.Error() == "用户名和新密码不能为空" || err.Error() == "新密码长度不能少于6位" {
			respondError(w, http.StatusBadRequest, err)
		} else if err.Error() == "安全问题答案错误" {
			respondError(w, http.StatusUnauthorized, err)
		} else {
			respondError(w, http.StatusInternalServerError, err)
		}
		return
	}

	// 验证通过，重置密码
	doubleHashedNewPassword := utils.MD5Hash(req.NewPassword, "bookmarks")
	if err := s.securityQuestions.UpdatePassword(req.Username, doubleHashedNewPassword); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "密码重置成功，请使用新密码登录"})
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
