package main

import (
	"bytes"
	"context"
	"database/sql"
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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/net/html"
	_ "modernc.org/sqlite"
)

const (
	nodeTypeFolder   = "folder"
	nodeTypeBookmark = "bookmark"
)

type server struct {
	db         *sql.DB
	httpClient *http.Client
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
	dataUrl := flag.String("dataUrl", "./", "数据存储路径") // 定义字符串参数
	flag.Parse()                                      // 缺少此行将导致获取默认值
	fmt.Println("url:", *dataUrl)
	// 创建目录
	if _, err := os.Stat(*dataUrl); os.IsNotExist(err) {
		if err := os.Mkdir(*dataUrl, 0755); err != nil {
			log.Fatalf("failed to create directory: %v", err)
		}
	}

	db, err := sql.Open("sqlite", *dataUrl+"data.db?_foreign_keys=on")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(1)

	if err := initializeDB(db); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	s := &server{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.AllowContentType("application/json", "text/plain", "application/x-www-form-urlencoded"))

	r.Route("/api", func(r chi.Router) {
		r.Get("/tree", s.handleGetTree)
		r.Get("/metadata", s.handleMetadata)
		r.Post("/folders", s.handleCreateFolder)
		r.Post("/bookmarks", s.handleCreateBookmark)
		r.Put("/nodes/{id}", s.handleUpdateNode)
		r.Delete("/nodes/{id}", s.handleDeleteNode)
		r.Post("/nodes/reorder", s.handleReorderNodes)
	})

	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/*", fileServer)

	addr := ":8080"
	log.Printf("Bookmark server running on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func initializeDB(db *sql.DB) error {
	schema := `
	PRAGMA foreign_keys = ON;
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
	CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
	CREATE INDEX IF NOT EXISTS idx_nodes_parent_position ON nodes(parent_id, position);

	CREATE TRIGGER IF NOT EXISTS trg_nodes_updated_at
	AFTER UPDATE ON nodes
	BEGIN
		UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
	END;
	`

	_, err := db.Exec(schema)
	return err
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
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid url: %w", err))
		return
	}
	title, icon, err := s.fetchMetadata(normalized)
	if err != nil {
		respondError(w, http.StatusBadGateway, fmt.Errorf("metadata fetch failed: %w", err))
		return
	}
	resp := map[string]*string{
		"title":       optionalString(title),
		"favicon_url": optionalString(icon),
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
			roots = append(roots, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}

	sortNodes(roots)
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

		defer resp.Body.Close()

		// 对于403错误，尝试不同的策略
		if resp.StatusCode == 403 {
			// 记录错误但继续到下一个重试
			log.Printf("Received 403 Forbidden for URL: %s, attempt: %d", rawURL, attempt+1)
			lastErr = fmt.Errorf("remote status 403 Forbidden")
			continue
		}

		// 对于其他错误状态码，直接使用URL信息作为备选
		if resp.StatusCode >= 400 {
			log.Printf("Received status %d for URL: %s", resp.StatusCode, rawURL)
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

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			lastErr = err
			continue
		}

		// 首先尝试简单的正则表达式提取标题，作为备份方法
		titleRegex := regexp.MustCompile(`<title[^>]*>(.*?)</title>`)
		matches := titleRegex.FindSubmatch(body)
		var title string
		if len(matches) > 1 {
			title = strings.TrimSpace(string(matches[1]))
			// 移除HTML实体
			title = strings.ReplaceAll(title, "&nbsp;", " ")
			title = strings.ReplaceAll(title, "&lt;", "<")
			title = strings.ReplaceAll(title, "&gt;", ">\n")
			title = strings.ReplaceAll(title, "&amp;", "&")
			title = strings.ReplaceAll(title, "&quot;", "\"")
			title = strings.ReplaceAll(title, "&#39;", "'")
		}

		// 解析HTML文档用于标题和图标提取
		var doc *html.Node
		doc, err = html.Parse(bytes.NewReader(body))

		// 如果正则表达式没有找到标题，尝试使用html包解析
		if title == "" && err == nil {
			title = extractTitle(doc)
		}

		// 如果仍然没有标题，使用已解析的主机名
		if title == "" {
			if hostname != "" {
				title = hostname
			} else {
				title = rawURL
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
	log.Printf("All %d attempts failed for URL: %s, last error: %v", maxRetries+1, rawURL, lastErr)
	if hostname != "" {
		return hostname, baseIconURL, nil
	}
	return rawURL, baseIconURL, nil
}

func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
		return strings.TrimSpace(n.FirstChild.Data)
	}
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
