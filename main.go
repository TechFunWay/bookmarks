package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

const (
	nodeTypeFolder   = "folder"
	nodeTypeBookmark = "bookmark"

	// 应用版本
	appVersion = "v1.7.0"

	// 日志模式常量
	logModeDebug   = "debug"
	logModeRelease = "release"
	defaultLogMode = logModeRelease
)

type server struct {
	db          *sql.DB
	httpClient  *http.Client
	faviconChan chan int64 // 图标获取任务队列
}

// 全局日志配置
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
	ID         int64   `json:"id"`
	ParentID   *int64  `json:"parent_id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	URL        *string `json:"url,omitempty"`
	FaviconURL *string `json:"favicon_url,omitempty"`
	Position   int     `json:"position"`
	Children   []*node `json:"children,omitempty"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
}

func main() {
	dataUrl := flag.String("dataUrl", "./", "数据存储路径")                              // 定义字符串参数
	port := flag.String("port", "8901", "服务器监听端口")                                 // 定义端口参数
	logModeFlag := flag.String("logmode", defaultLogMode, "日志模式: debug 或 release") // 日志模式参数
	flag.Parse()                                                                   // 缺少此行将导致获取默认值

	// 初始化日志模式：先检查命令行参数，再检查环境变量，最后使用默认值
	logMode = *logModeFlag
	if envLogMode := os.Getenv("LOG_MODE"); envLogMode != "" {
		logMode = envLogMode
	}

	// 验证日志模式
	if logMode != logModeDebug && logMode != logModeRelease {
		log.Fatalf("无效的日志模式: %s, 必须是 debug 或 release", logMode)
	}
	fmt.Println("数据路径:", *dataUrl)
	fmt.Println("监听端口:", *port)
	// 创建数据目录
	if _, err := os.Stat(*dataUrl); os.IsNotExist(err) {
		if err := os.Mkdir(*dataUrl, 0755); err != nil {
			log.Fatalf("failed to create data directory: %v", err)
		}
	}

	// 创建图标存储目录
	if err := os.MkdirAll("static/icons", 0755); err != nil {
		log.Fatalf("failed to create icons directory: %v", err)
	}

	db, err := sql.Open("sqlite", *dataUrl+"data.db?_foreign_keys=on")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	// 初始化数据库
	if err := initializeDB(db); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
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
	}

	// 启动图标获取协程
	go s.faviconWorker()

	// 创建日志文件
	logFile, err := logger.CreateLogFile()
	if err != nil {
		log.Fatalf("failed to create log file: %v", err)
	}
	defer logFile.Close()

	r := chi.NewRouter()
	// 使用自定义日志中间件而不是默认的middleware.Logger
	r.Use(logger.LoggingMiddleware(logFile))
	r.Use(middleware.Recoverer)
	r.Use(middleware.AllowContentType("application/json", "text/plain", "application/x-www-form-urlencoded"))

	r.Route("/api", func(r chi.Router) {
		r.Get("/tree", s.handleGetTree)
		r.Get("/metadata", s.handleMetadata)
		r.Get("/version", s.handleGetVersion)
		r.Post("/folders", s.handleCreateFolder)
		r.Post("/bookmarks", s.handleCreateBookmark)
		r.Put("/nodes/{id}", s.handleUpdateNode)
		r.Delete("/nodes/{id}", s.handleDeleteNode)
		r.Post("/nodes/batch-delete", s.handleBatchDeleteNodes)
		r.Post("/nodes/reorder", s.handleReorderNodes)
		r.Post("/import", s.handleImport)
		r.Post("/import-edge", s.handleEdgeImport)
		r.Get("/config", s.handleGetConfig)
		r.Post("/config", s.handleUpdateConfig)
	})

	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/*", fileServer)
	r.Handle("/static/*", http.StripPrefix("/static", fileServer))

	addr := ":" + *port
	Debug("Bookmark server running on %s", addr)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func initializeDB(db *sql.DB) error {
	// 启用外键约束
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return err
	}

	// 创建nodes表（如果不存在）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			parent_id INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
			type TEXT NOT NULL CHECK (type IN ('folder', 'bookmark')),
			title TEXT NOT NULL,
			url TEXT,
			favicon_url TEXT,
			position INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return err
	}

	// 创建nodes表索引（如果不存在）
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
		CREATE INDEX IF NOT EXISTS idx_nodes_parent_position ON nodes(parent_id, position);
	`); err != nil {
		return err
	}

	// 创建nodes表的updated_at触发器（如果不存在）
	if _, err := db.Exec(`
		CREATE TRIGGER IF NOT EXISTS trg_nodes_updated_at
		AFTER UPDATE ON nodes
		BEGIN
			UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;
	`); err != nil {
		return err
	}

	// 数据库版本管理：创建version表（如果不存在）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS version (
			id INTEGER PRIMARY KEY,
			version INTEGER NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return err
	}

	// 检查当前数据库版本
	var version int
	err := db.QueryRow("SELECT version FROM version WHERE id = 1").Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// 如果版本表为空，插入初始版本1
	if err == sql.ErrNoRows {
		if _, err := db.Exec("INSERT INTO version (id, version) VALUES (1, 1)"); err != nil {
			return err
		}
		version = 1
	}

	// 执行数据库升级
	if err := upgradeDatabase(db, version); err != nil {
		return err
	}

	// 执行系统升级
	upgrader := logic.NewUpgrade(db, appVersion)
	if err := upgrader.PerformUpgrade(); err != nil {
		log.Printf("系统升级失败: %v", err)
		// 注意：这里我们记录错误但不返回错误，以避免阻止系统启动
	}

	return nil
}

// upgradeDatabase 执行数据库升级
func upgradeDatabase(db *sql.DB, currentVersion int) error {
	// 升级到版本2：添加配置表
	if currentVersion < 2 {
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS config (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
		`); err != nil {
			return err
		}

		// 创建config表的updated_at触发器
		if _, err := db.Exec(`
			CREATE TRIGGER IF NOT EXISTS trg_config_updated_at
			AFTER UPDATE ON config
			BEGIN
				UPDATE config SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
			END;
		`); err != nil {
			return err
		}

		// 更新版本号
		if _, err := db.Exec("UPDATE version SET version = 2 WHERE id = 1"); err != nil {
			return err
		}
		currentVersion = 2
	}

	return nil
}

func (s *server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.loadTree(r.Context())
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
		savedIcon, err = s.downloadAndSaveIcon(icon)
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
	newNode, err := s.insertNode(r.Context(), nodeTypeFolder, req.Title, req.ParentID, nil, nil)
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
}

func (s *server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
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
	newNode, err := s.insertNode(r.Context(), nodeTypeBookmark, title, req.ParentID, &urlCopy, faviconPtr)
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
}

func (s *server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
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

	if err := s.updateNode(r.Context(), id, req); err != nil {
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

	updatedNode, err := s.getNode(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, updatedNode)
}

func (s *server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}
	res, err := s.db.ExecContext(r.Context(), "DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		respondError(w, http.StatusNotFound, errors.New("node not found"))
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type batchDeleteRequest struct {
	IDs []int64 `json:"ids"`
}

func (s *server) handleBatchDeleteNodes(w http.ResponseWriter, r *http.Request) {
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

	// 使用事务批量删除
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		Debug("批量删除失败，开启事务失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %w", err))
		return
	}
	defer tx.Rollback()

	// 准备删除语句
	stmt, err := tx.PrepareContext(r.Context(), "DELETE FROM nodes WHERE id = ?")
	if err != nil {
		Debug("批量删除失败，准备语句失败: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Errorf("failed to prepare statement: %w", err))
		return
	}
	defer stmt.Close()

	// 批量执行删除
	var deletedCount int64
	for _, id := range req.IDs {
		res, err := stmt.ExecContext(r.Context(), id)
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

	// 提交事务
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
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid body: %w", err))
		return
	}
	if len(req.OrderedIDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("ordered_ids cannot be empty"))
		return
	}
	if err := s.reorderNodes(r.Context(), req.ParentID, req.OrderedIDs); err != nil {
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

	// 如果是replace模式，删除指定文件夹及其子节点，然后重新创建
	if req.Mode == "replace" {
		if req.ParentID != nil {
			// 先获取要删除的文件夹信息
			var folderTitle string
			var folderPosition int
			var folderParentID *int64
			err := tx.QueryRowContext(r.Context(), "SELECT title, position, parent_id FROM nodes WHERE id = ?", *req.ParentID).Scan(&folderTitle, &folderPosition, &folderParentID)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：删除文件夹 ID=%d, 标题=%s", *req.ParentID, folderTitle)

			// 删除指定文件夹及其所有子节点
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE id = ?", *req.ParentID); err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			// 重新创建文件夹
			res, err := tx.ExecContext(r.Context(), `
				INSERT INTO nodes (parent_id, type, title, position)
				VALUES (?, ?, ?, ?)
			`, folderParentID, nodeTypeFolder, folderTitle, folderPosition)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			// 获取新创建的文件夹ID
			newFolderID, err := res.LastInsertId()
			if err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：重新创建文件夹，新ID=%d", newFolderID)

			// 更新parent_id为新创建的文件夹ID
			req.ParentID = &newFolderID
		} else {
			// 如果没有指定parent_id，删除所有数据
			Debug("Replace模式：删除所有数据")
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes"); err != nil {
				respondError(w, http.StatusInternalServerError, err)
				return
			}
		}
	}

	// 递归导入节点
	Debug("开始导入节点，parentID=%v, mode=%s", req.ParentID, req.Mode)
	stats := &importStats{}
	faviconQueue := []int64{}
	if err = s.importNodes(tx, r.Context(), req.Bookmarks, req.ParentID, req.Mode, stats, true, &faviconQueue); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	// 异步获取图标
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

	// 解析HTML内容为节点结构
	nodes, err := parseEdgeHTML(req.HTML)
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

	// 如果是replace模式
	if req.Mode == "replace" {
		if req.ParentID != nil {
			// 1. 先获取要删除的文件夹信息
			var folderTitle string
			var folderPosition int
			var folderParentID *int64
			err := tx.QueryRowContext(r.Context(), "SELECT title, position, parent_id FROM nodes WHERE id = ?", *req.ParentID).Scan(&folderTitle, &folderPosition, &folderParentID)
			if err != nil {
				Error("获取文件夹信息失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：删除文件夹 ID=%d, 标题=%s", *req.ParentID, folderTitle)

			// 2. 删除指定文件夹及其所有子节点
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes WHERE id = ?", *req.ParentID); err != nil {
				Error("删除数据失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			// 3. 重新创建文件夹
			res, err := tx.ExecContext(r.Context(), `
				INSERT INTO nodes (parent_id, type, title, position)
				VALUES (?, ?, ?, ?)
			`, folderParentID, nodeTypeFolder, folderTitle, folderPosition)
			if err != nil {
				Error("创建文件夹失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			// 4. 获取新创建的文件夹ID
			newFolderID, err := res.LastInsertId()
			if err != nil {
				Error("获取新文件夹ID失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}

			Debug("Replace模式：重新创建文件夹，新ID=%d", newFolderID)

			// 5. 更新parent_id为新创建的文件夹ID
			req.ParentID = &newFolderID
		} else {
			// 如果没有指定parent_id，删除所有数据
			Debug("执行replace模式，删除所有数据")
			if _, err = tx.ExecContext(r.Context(), "DELETE FROM nodes"); err != nil {
				Error("删除数据失败: %v", err)
				respondError(w, http.StatusInternalServerError, err)
				return
			}
		}
	}

	// 递归导入节点
	stats := &importStats{}
	Debug("开始导入节点，共%d个根节点，父文件夹ID=%v", len(nodes), req.ParentID)
	faviconQueue := []int64{}
	if err = s.importNodes(tx, r.Context(), nodes, req.ParentID, req.Mode, stats, true, &faviconQueue); err != nil {
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

	// 异步获取图标
	for _, nodeID := range faviconQueue {
		s.queueFaviconFetch(nodeID)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "imported",
		"stats":  stats,
	})
}

// 解析Edge导出的HTML书签
func parseEdgeHTML(htmlContent string) ([]*node, error) {
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
						localPath, err := saveBase64Icon(iconData)
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

func (s *server) importNodes(tx *sql.Tx, ctx context.Context, nodes []*node, parentID *int64, mode string, stats *importStats, fetchMetadata bool, faviconQueue *[]int64) error {
	// 在merge模式下，验证parent_id是否存在，如果不存在则创建
	// 在replace模式下，如果parent_id不存在，也创建一个临时文件夹
	if parentID != nil {
		var count int
		var err error
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE id = ?", *parentID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			// parent_id不存在，创建一个临时文件夹
			Debug("警告：parent_id %d 不存在，创建临时文件夹", *parentID)
			res, err := tx.ExecContext(ctx, `
				INSERT INTO nodes (parent_id, type, title, position)
				VALUES (NULL, ?, ?, 0)
			`, nodeTypeFolder, "临时文件夹")
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
		// 插入当前节点
		var newID int64
		var err error

		switch node.Type {
		case nodeTypeFolder:
			// 检查文件夹是否已存在（仅在merge模式下检查）
			var exists bool
			if mode == "merge" {
				var count int
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ?", nodeTypeFolder, node.Title).Scan(&count); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ?", nodeTypeFolder, *parentID, node.Title).Scan(&count); err != nil {
						return err
					}
				}
				exists = count > 0
			}

			if exists {
				// 文件夹已存在，获取其ID并继续导入子节点
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT id FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ?", nodeTypeFolder, node.Title).Scan(&newID); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT id FROM nodes WHERE type = ? AND parent_id = ? AND title = ?", nodeTypeFolder, *parentID, node.Title).Scan(&newID); err != nil {
						return err
					}
				}
				stats.Skipped++
			} else {
				// 插入文件夹
				res, err := tx.ExecContext(ctx, `
					INSERT INTO nodes (parent_id, type, title, position)
					VALUES (?, ?, ?, ?)
				`, parentID, nodeTypeFolder, node.Title, pos)
				if err != nil {
					return err
				}
				newID, err = res.LastInsertId()
				if err != nil {
					return err
				}
				stats.Folders++
			}

			// 递归导入子节点
			if len(node.Children) > 0 {
				if err = s.importNodes(tx, ctx, node.Children, &newID, mode, stats, fetchMetadata, faviconQueue); err != nil {
					return err
				}
			}

		case nodeTypeBookmark:
			if node.URL == nil {
				stats.Skipped++
				continue // 跳过无效的书签
			}

			// 检查书签是否已存在（仅在merge模式下检查）
			var exists bool
			if mode == "merge" {
				var count int
				if parentID == nil {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND url = ?", nodeTypeBookmark, node.Title, *node.URL).Scan(&count); err != nil {
						return err
					}
				} else {
					if err = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND url = ?", nodeTypeBookmark, *parentID, node.Title, *node.URL).Scan(&count); err != nil {
						return err
					}
				}
				exists = count > 0
			}

			if exists {
				stats.Skipped++
			} else {
				// 如果没有图标，根据参数决定是否自动获取
				var favicon *string
				if node.FaviconURL != nil {
					tmp := *node.FaviconURL
					favicon = &tmp
				}

				// 插入书签
				res, err := tx.ExecContext(ctx, `
					INSERT INTO nodes (parent_id, type, title, url, favicon_url, position)
					VALUES (?, ?, ?, ?, ?, ?)
				`, parentID, nodeTypeBookmark, node.Title, node.URL, favicon, pos)
				if err != nil {
					return err
				}
				newID, err = res.LastInsertId()
				if err != nil {
					return err
				}
				stats.Bookmarks++

				// 如果需要获取图标且没有图标，加入异步队列
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

func (s *server) loadTree(ctx context.Context) ([]*node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, parent_id, type, title, url, favicon_url, position, created_at, updated_at
		FROM nodes
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`)
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
		position   int
		createdAt  string
		updatedAt  string
	}

	var rawNodes []rawNode
	for rows.Next() {
		var rn rawNode
		if err := rows.Scan(&rn.id, &rn.parentID, &rn.nodeType, &rn.title, &rn.url, &rn.faviconURL, &rn.position, &rn.createdAt, &rn.updatedAt); err != nil {
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
	// 确保 roots 不是 nil，避免返回 null
	if roots == nil {
		roots = []*node{}
	}
	return roots, nil
}

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

func (s *server) insertNode(ctx context.Context, nType, title string, parentID *int64, url, favicon *string) (*node, error) {
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
		if err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ?", *parentID).Scan(&parentType); err != nil {
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
		if err := ensureUniqueFolderTx(tx, parentID, title, nil); err != nil {
			return nil, err
		}
	case nodeTypeBookmark:
		if url == nil || strings.TrimSpace(*url) == "" {
			return nil, ErrInvalidUpdate
		}
		if err := ensureUniqueBookmarkTx(tx, parentID, title, *url, nil); err != nil {
			return nil, err
		}
	}

	var nextPos int
	if parentID == nil {
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL").Scan(&nextPos); err != nil {
			return nil, err
		}
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ?", *parentID).Scan(&nextPos); err != nil {
			return nil, err
		}
	}

	res, execErr := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, url, favicon_url, position)
		VALUES (?, ?, ?, ?, ?, ?)
	`, parentID, nType, title, url, favicon, nextPos)
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

func (s *server) updateNode(ctx context.Context, id int64, req updateNodeRequest) error {
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
	}
	err = tx.QueryRowContext(ctx, "SELECT type, parent_id, title, url, favicon_url FROM nodes WHERE id = ?", id).Scan(
		&current.Type, &current.ParentID, &current.Title, &current.URL, &current.Favicon,
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
		if err = tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ?", *req.ParentID).Scan(&parentType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = ErrInvalidParent
			}
			return err
		}
		if parentType != nodeTypeFolder {
			err = ErrInvalidParent
			return err
		}
		isCycle, cycErr := s.parentCreatesCycle(tx, id, *req.ParentID)
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

	if req.ParentID != nil {
		newParent := *req.ParentID
		parentIDValue = newParent
		targetParentID = &parentIDValue
		parentSet = true
	}

	if !titleSet && !urlSet && !faviconSet && !parentSet {
		return ErrInvalidUpdate
	}

	switch current.Type {
	case nodeTypeFolder:
		if titleSet || parentSet {
			if err := ensureUniqueFolderTx(tx, targetParentID, targetTitle, &id); err != nil {
				return err
			}
		}
	case nodeTypeBookmark:
		if !urlValid {
			err = ErrInvalidUpdate
			return err
		}
		if titleSet || urlSet || parentSet {
			if err := ensureUniqueBookmarkTx(tx, targetParentID, targetTitle, targetURL, &id); err != nil {
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

	args = append(args, id)

	query := fmt.Sprintf("UPDATE nodes SET %s WHERE id = ?", strings.Join(fields, ", "))
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *server) reorderNodes(ctx context.Context, parentID *int64, orderedIDs []int64) error {
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
		query = fmt.Sprintf("SELECT COUNT(*) FROM nodes WHERE parent_id IS NULL AND id IN (%s)", placeholders)
	} else {
		query = fmt.Sprintf("SELECT COUNT(*) FROM nodes WHERE parent_id = ? AND id IN (%s)", placeholders)
		args = append([]any{*parentID}, args...)
	}

	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return err
	}
	if count != len(orderedIDs) {
		return ErrInvalidParent
	}

	for pos, id := range orderedIDs {
		if _, err := tx.ExecContext(ctx, "UPDATE nodes SET position = ? WHERE id = ?", pos, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ensureUniqueFolderTx(tx *sql.Tx, parentID *int64, title string, excludeID *int64) error {
	var query string
	var args []any
	if parentID == nil {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ?"
		args = []any{nodeTypeFolder, title}
	} else {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ?"
		args = []any{nodeTypeFolder, *parentID, title}
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

func ensureUniqueBookmarkTx(tx *sql.Tx, parentID *int64, title, url string, excludeID *int64) error {
	var query string
	var args []any
	if parentID == nil {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id IS NULL AND title = ? AND url = ?"
		args = []any{nodeTypeBookmark, title, url}
	} else {
		query = "SELECT COUNT(1) FROM nodes WHERE type = ? AND parent_id = ? AND title = ? AND url = ?"
		args = []any{nodeTypeBookmark, *parentID, title, url}
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

func (s *server) parentCreatesCycle(tx *sql.Tx, nodeID, newParentID int64) (bool, error) {
	current := newParentID
	for {
		if current == nodeID {
			return true, nil
		}
		var parent sql.NullInt64
		if err := tx.QueryRow("SELECT parent_id FROM nodes WHERE id = ?", current).Scan(&parent); err != nil {
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

func (s *server) getNode(ctx context.Context, id int64) (*node, error) {
	var n node
	var parent sql.NullInt64
	var urlVal sql.NullString
	var icon sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, parent_id, type, title, url, favicon_url, position, created_at, updated_at
		FROM nodes WHERE id = ?
	`, id).Scan(&n.ID, &parent, &n.Type, &n.Title, &urlVal, &icon, &n.Position, &n.CreatedAt, &n.UpdatedAt)
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

// handleGetConfig 获取配置
func (s *server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// 获取所有配置项
	rows, err := s.db.QueryContext(r.Context(), "SELECT key, value FROM config")
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

	// 执行更新或插入操作
	_, err := s.db.ExecContext(r.Context(), `
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, req.Key, req.Value)
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

// 辅助函数：获取最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// saveBase64Icon 保存base64图标到本地文件
func saveBase64Icon(iconData string) (string, error) {
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
	iconDir := fmt.Sprintf("static/icons/%s", dateDir)

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
func (s *server) downloadAndSaveIcon(iconURL string) (string, error) {
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
	iconDir := fmt.Sprintf("static/icons/%s", dateDir)

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
