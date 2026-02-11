-- 数据库初始化脚本
-- 启用外键约束
PRAGMA foreign_keys = ON;

-- 创建nodes表
CREATE TABLE IF NOT EXISTS nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL DEFAULT 0,
    parent_id INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('folder', 'bookmark')),
    title TEXT NOT NULL,
    url TEXT,
    favicon_url TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 创建nodes表索引
CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_parent_position ON nodes(parent_id, position);

-- 创建nodes表的updated_at触发器
CREATE TRIGGER IF NOT EXISTS trg_nodes_updated_at
AFTER UPDATE ON nodes
BEGIN
    UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
