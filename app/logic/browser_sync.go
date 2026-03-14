package logic

import (
	"context"
	"database/sql"
	"fmt"
)

// SyncBookmark 同步书签模型
type SyncBookmark struct {
	ID         int64   `json:"id"`
	ParentID   *int64  `json:"parent_id"`
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	FaviconURL *string `json:"favicon_url,omitempty"`
	Position   int     `json:"position"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// SyncFolder 同步文件夹模型
type SyncFolder struct {
	ID        int64   `json:"id"`
	ParentID  *int64  `json:"parent_id"`
	Title     string  `json:"title"`
	Position  int     `json:"position"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// BatchOperationRequest 批量操作请求
type BatchOperationRequest struct {
	Create *CreateOperation `json:"create,omitempty"`
	Update *UpdateOperation `json:"update,omitempty"`
	Delete *DeleteOperation `json:"delete,omitempty"`
}

// CreateOperation 创建操作
type CreateOperation struct {
	Bookmarks []*SyncBookmark `json:"bookmarks,omitempty"`
	Folders   []*SyncFolder   `json:"folders,omitempty"`
}

// UpdateOperation 更新操作
type UpdateOperation struct {
	Bookmarks []*SyncBookmark `json:"bookmarks,omitempty"`
	Folders   []*SyncFolder   `json:"folders,omitempty"`
}

// DeleteOperation 删除操作
type DeleteOperation struct {
	BookmarkIDs []int64 `json:"bookmark_ids,omitempty"`
	FolderIDs   []int64 `json:"folder_ids,omitempty"`
}

// BatchOperationResult 批量操作结果
type BatchOperationResult struct {
	Created struct {
		Bookmarks []*SyncBookmark `json:"bookmarks"`
		Folders   []*SyncFolder   `json:"folders"`
	} `json:"created"`
	Updated struct {
		Bookmarks []*SyncBookmark `json:"bookmarks"`
		Folders   []*SyncFolder   `json:"folders"`
	} `json:"updated"`
	Deleted struct {
		BookmarkIDs []int64 `json:"bookmark_ids"`
		FolderIDs   []int64 `json:"folder_ids"`
	} `json:"deleted"`
	Errors []string `json:"errors,omitempty"`
}

// BrowserSync 浏览器同步管理器
type BrowserSync struct {
	db *sql.DB
}

// NewBrowserSync 创建浏览器同步管理器
func NewBrowserSync(db *sql.DB) *BrowserSync {
	return &BrowserSync{db: db}
}

// GetBookmarks 获取用户的所有书签（扁平化列表）
func (bs *BrowserSync) GetBookmarks(ctx context.Context, userID int64) ([]*SyncBookmark, error) {
	rows, err := bs.db.QueryContext(ctx, `
		SELECT id, parent_id, title, url, favicon_url, position, created_at, updated_at
		FROM nodes
		WHERE user_id = ? AND type = 'bookmark'
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询书签失败: %w", err)
	}
	defer rows.Close()

	var bookmarks []*SyncBookmark
	for rows.Next() {
		var b SyncBookmark
		var parentID sql.NullInt64
		var faviconURL sql.NullString

		err := rows.Scan(&b.ID, &parentID, &b.Title, &b.URL, &faviconURL, &b.Position, &b.CreatedAt, &b.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("扫描书签数据失败: %w", err)
		}

		if parentID.Valid {
			b.ParentID = &parentID.Int64
		}
		if faviconURL.Valid {
			b.FaviconURL = &faviconURL.String
		}

		bookmarks = append(bookmarks, &b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历书签数据失败: %w", err)
	}

	return bookmarks, nil
}

// GetFolders 获取用户的所有文件夹（扁平化列表）
func (bs *BrowserSync) GetFolders(ctx context.Context, userID int64) ([]*SyncFolder, error) {
	rows, err := bs.db.QueryContext(ctx, `
		SELECT id, parent_id, title, position, created_at, updated_at
		FROM nodes
		WHERE user_id = ? AND type = 'folder'
		ORDER BY parent_id IS NOT NULL, parent_id, position, id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询文件夹失败: %w", err)
	}
	defer rows.Close()

	var folders []*SyncFolder
	for rows.Next() {
		var f SyncFolder
		var parentID sql.NullInt64

		err := rows.Scan(&f.ID, &parentID, &f.Title, &f.Position, &f.CreatedAt, &f.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("扫描文件夹数据失败: %w", err)
		}

		if parentID.Valid {
			f.ParentID = &parentID.Int64
		}

		folders = append(folders, &f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历文件夹数据失败: %w", err)
	}

	return folders, nil
}

// CreateBookmark 创建书签
func (bs *BrowserSync) CreateBookmark(ctx context.Context, userID int64, bookmark *SyncBookmark) (*SyncBookmark, error) {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 检查父文件夹是否存在
	if bookmark.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("父文件夹不存在")
			}
			return nil, fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return nil, fmt.Errorf("父节点不是文件夹")
		}
	}

	// 获取下一个位置
	var nextPos int
	if bookmark.ParentID == nil {
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL AND user_id = ?", userID).Scan(&nextPos)
	} else {
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&nextPos)
	}
	if err != nil {
		return nil, fmt.Errorf("获取位置失败: %w", err)
	}

	// 如果请求中指定了位置，使用请求的位置
	if bookmark.Position > 0 {
		nextPos = bookmark.Position
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, url, favicon_url, position, user_id)
		VALUES (?, 'bookmark', ?, ?, ?, ?, ?)
	`, bookmark.ParentID, bookmark.Title, bookmark.URL, bookmark.FaviconURL, nextPos, userID)
	if err != nil {
		return nil, fmt.Errorf("插入书签失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取插入ID失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	bookmark.ID = id
	bookmark.Position = nextPos
	return bookmark, nil
}

// UpdateBookmark 更新书签
func (bs *BrowserSync) UpdateBookmark(ctx context.Context, userID int64, bookmark *SyncBookmark) error {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 检查书签是否存在
	var existingType string
	err = tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", bookmark.ID, userID).Scan(&existingType)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("书签不存在")
		}
		return fmt.Errorf("查询书签失败: %w", err)
	}
	if existingType != "bookmark" {
		return fmt.Errorf("指定ID不是书签")
	}

	// 检查父文件夹是否存在
	if bookmark.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("父文件夹不存在")
			}
			return fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return fmt.Errorf("父节点不是文件夹")
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE nodes SET parent_id = ?, title = ?, url = ?, favicon_url = ?, position = ?
		WHERE id = ? AND user_id = ?
	`, bookmark.ParentID, bookmark.Title, bookmark.URL, bookmark.FaviconURL, bookmark.Position, bookmark.ID, userID)
	if err != nil {
		return fmt.Errorf("更新书签失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// DeleteBookmark 删除书签
func (bs *BrowserSync) DeleteBookmark(ctx context.Context, userID int64, bookmarkID int64) error {
	result, err := bs.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ? AND user_id = ? AND type = 'bookmark'", bookmarkID, userID)
	if err != nil {
		return fmt.Errorf("删除书签失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("书签不存在或无权删除")
	}

	return nil
}

// CreateFolder 创建文件夹
func (bs *BrowserSync) CreateFolder(ctx context.Context, userID int64, folder *SyncFolder) (*SyncFolder, error) {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 检查父文件夹是否存在
	if folder.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("父文件夹不存在")
			}
			return nil, fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return nil, fmt.Errorf("父节点不是文件夹")
		}
	}

	// 获取下一个位置
	var nextPos int
	if folder.ParentID == nil {
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL AND user_id = ?", userID).Scan(&nextPos)
	} else {
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&nextPos)
	}
	if err != nil {
		return nil, fmt.Errorf("获取位置失败: %w", err)
	}

	// 如果请求中指定了位置，使用请求的位置
	if folder.Position > 0 {
		nextPos = folder.Position
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, position, user_id)
		VALUES (?, 'folder', ?, ?, ?)
	`, folder.ParentID, folder.Title, nextPos, userID)
	if err != nil {
		return nil, fmt.Errorf("插入文件夹失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取插入ID失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	folder.ID = id
	folder.Position = nextPos
	return folder, nil
}

// UpdateFolder 更新文件夹
func (bs *BrowserSync) UpdateFolder(ctx context.Context, userID int64, folder *SyncFolder) error {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 检查文件夹是否存在
	var existingType string
	err = tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", folder.ID, userID).Scan(&existingType)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("文件夹不存在")
		}
		return fmt.Errorf("查询文件夹失败: %w", err)
	}
	if existingType != "folder" {
		return fmt.Errorf("指定ID不是文件夹")
	}

	// 检查父文件夹是否存在（且不是自己）
	if folder.ParentID != nil {
		if *folder.ParentID == folder.ID {
			return fmt.Errorf("不能将文件夹设置为自己的子文件夹")
		}

		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("父文件夹不存在")
			}
			return fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return fmt.Errorf("父节点不是文件夹")
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE nodes SET parent_id = ?, title = ?, position = ?
		WHERE id = ? AND user_id = ?
	`, folder.ParentID, folder.Title, folder.Position, folder.ID, userID)
	if err != nil {
		return fmt.Errorf("更新文件夹失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// DeleteFolder 删除文件夹
func (bs *BrowserSync) DeleteFolder(ctx context.Context, userID int64, folderID int64) error {
	result, err := bs.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ? AND user_id = ? AND type = 'folder'", folderID, userID)
	if err != nil {
		return fmt.Errorf("删除文件夹失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("文件夹不存在或无权删除")
	}

	return nil
}

// BatchOperation 批量操作
func (bs *BrowserSync) BatchOperation(ctx context.Context, userID int64, req *BatchOperationRequest) (*BatchOperationResult, error) {
	result := &BatchOperationResult{}

	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 处理创建操作
	if req.Create != nil {
		// 创建文件夹
		for _, folder := range req.Create.Folders {
			createdFolder, err := bs.createFolderTx(ctx, tx, userID, folder)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("创建文件夹 '%s' 失败: %v", folder.Title, err))
				continue
			}
			result.Created.Folders = append(result.Created.Folders, createdFolder)
		}

		// 创建书签
		for _, bookmark := range req.Create.Bookmarks {
			createdBookmark, err := bs.createBookmarkTx(ctx, tx, userID, bookmark)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("创建书签 '%s' 失败: %v", bookmark.Title, err))
				continue
			}
			result.Created.Bookmarks = append(result.Created.Bookmarks, createdBookmark)
		}
	}

	// 处理更新操作
	if req.Update != nil {
		// 更新文件夹
		for _, folder := range req.Update.Folders {
			err := bs.updateFolderTx(ctx, tx, userID, folder)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("更新文件夹 '%s' 失败: %v", folder.Title, err))
				continue
			}
			result.Updated.Folders = append(result.Updated.Folders, folder)
		}

		// 更新书签
		for _, bookmark := range req.Update.Bookmarks {
			err := bs.updateBookmarkTx(ctx, tx, userID, bookmark)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("更新书签 '%s' 失败: %v", bookmark.Title, err))
				continue
			}
			result.Updated.Bookmarks = append(result.Updated.Bookmarks, bookmark)
		}
	}

	// 处理删除操作
	if req.Delete != nil {
		// 删除书签
		for _, id := range req.Delete.BookmarkIDs {
			_, err := tx.ExecContext(ctx, "DELETE FROM nodes WHERE id = ? AND user_id = ? AND type = 'bookmark'", id, userID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("删除书签 ID=%d 失败: %v", id, err))
				continue
			}
			result.Deleted.BookmarkIDs = append(result.Deleted.BookmarkIDs, id)
		}

		// 删除文件夹
		for _, id := range req.Delete.FolderIDs {
			_, err := tx.ExecContext(ctx, "DELETE FROM nodes WHERE id = ? AND user_id = ? AND type = 'folder'", id, userID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("删除文件夹 ID=%d 失败: %v", id, err))
				continue
			}
			result.Deleted.FolderIDs = append(result.Deleted.FolderIDs, id)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	return result, nil
}

// createFolderTx 在事务中创建文件夹
func (bs *BrowserSync) createFolderTx(ctx context.Context, tx *sql.Tx, userID int64, folder *SyncFolder) (*SyncFolder, error) {
	// 检查父文件夹是否存在
	if folder.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("父文件夹不存在")
			}
			return nil, fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return nil, fmt.Errorf("父节点不是文件夹")
		}
	}

	// 获取下一个位置
	var nextPos int
	if folder.ParentID == nil {
		err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL AND user_id = ?", userID).Scan(&nextPos)
		if err != nil {
			return nil, fmt.Errorf("获取位置失败: %w", err)
		}
	} else {
		err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&nextPos)
		if err != nil {
			return nil, fmt.Errorf("获取位置失败: %w", err)
		}
	}

	// 如果请求中指定了位置，使用请求的位置
	if folder.Position > 0 {
		nextPos = folder.Position
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, position, user_id)
		VALUES (?, 'folder', ?, ?, ?)
	`, folder.ParentID, folder.Title, nextPos, userID)
	if err != nil {
		return nil, fmt.Errorf("插入文件夹失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取插入ID失败: %w", err)
	}

	folder.ID = id
	folder.Position = nextPos
	return folder, nil
}

// createBookmarkTx 在事务中创建书签
func (bs *BrowserSync) createBookmarkTx(ctx context.Context, tx *sql.Tx, userID int64, bookmark *SyncBookmark) (*SyncBookmark, error) {
	// 检查父文件夹是否存在
	if bookmark.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("父文件夹不存在")
			}
			return nil, fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return nil, fmt.Errorf("父节点不是文件夹")
		}
	}

	// 获取下一个位置
	var nextPos int
	if bookmark.ParentID == nil {
		err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id IS NULL AND user_id = ?", userID).Scan(&nextPos)
		if err != nil {
			return nil, fmt.Errorf("获取位置失败: %w", err)
		}
	} else {
		err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM nodes WHERE parent_id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&nextPos)
		if err != nil {
			return nil, fmt.Errorf("获取位置失败: %w", err)
		}
	}

	// 如果请求中指定了位置，使用请求的位置
	if bookmark.Position > 0 {
		nextPos = bookmark.Position
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO nodes (parent_id, type, title, url, favicon_url, position, user_id)
		VALUES (?, 'bookmark', ?, ?, ?, ?, ?)
	`, bookmark.ParentID, bookmark.Title, bookmark.URL, bookmark.FaviconURL, nextPos, userID)
	if err != nil {
		return nil, fmt.Errorf("插入书签失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取插入ID失败: %w", err)
	}

	bookmark.ID = id
	bookmark.Position = nextPos
	return bookmark, nil
}

// updateFolderTx 在事务中更新文件夹
func (bs *BrowserSync) updateFolderTx(ctx context.Context, tx *sql.Tx, userID int64, folder *SyncFolder) error {
	// 检查文件夹是否存在
	var existingType string
	err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", folder.ID, userID).Scan(&existingType)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("文件夹不存在")
		}
		return fmt.Errorf("查询文件夹失败: %w", err)
	}
	if existingType != "folder" {
		return fmt.Errorf("指定ID不是文件夹")
	}

	// 检查父文件夹是否存在（且不是自己）
	if folder.ParentID != nil {
		if *folder.ParentID == folder.ID {
			return fmt.Errorf("不能将文件夹设置为自己的子文件夹")
		}

		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *folder.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("父文件夹不存在")
			}
			return fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return fmt.Errorf("父节点不是文件夹")
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE nodes SET parent_id = ?, title = ?, position = ?
		WHERE id = ? AND user_id = ?
	`, folder.ParentID, folder.Title, folder.Position, folder.ID, userID)
	if err != nil {
		return fmt.Errorf("更新文件夹失败: %w", err)
	}

	return nil
}

// updateBookmarkTx 在事务中更新书签
func (bs *BrowserSync) updateBookmarkTx(ctx context.Context, tx *sql.Tx, userID int64, bookmark *SyncBookmark) error {
	// 检查书签是否存在
	var existingType string
	err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", bookmark.ID, userID).Scan(&existingType)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("书签不存在")
		}
		return fmt.Errorf("查询书签失败: %w", err)
	}
	if existingType != "bookmark" {
		return fmt.Errorf("指定ID不是书签")
	}

	// 检查父文件夹是否存在
	if bookmark.ParentID != nil {
		var parentType string
		err := tx.QueryRowContext(ctx, "SELECT type FROM nodes WHERE id = ? AND user_id = ?", *bookmark.ParentID, userID).Scan(&parentType)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("父文件夹不存在")
			}
			return fmt.Errorf("查询父文件夹失败: %w", err)
		}
		if parentType != "folder" {
			return fmt.Errorf("父节点不是文件夹")
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE nodes SET parent_id = ?, title = ?, url = ?, favicon_url = ?, position = ?
		WHERE id = ? AND user_id = ?
	`, bookmark.ParentID, bookmark.Title, bookmark.URL, bookmark.FaviconURL, bookmark.Position, bookmark.ID, userID)
	if err != nil {
		return fmt.Errorf("更新书签失败: %w", err)
	}

	return nil
}
