CREATE TABLE IF NOT EXISTS config (
    user_id INTEGER NOT NULL DEFAULT 0,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);

CREATE TRIGGER IF NOT EXISTS trg_config_updated_at
AFTER UPDATE ON config
BEGIN
    UPDATE config SET updated_at = CURRENT_TIMESTAMP WHERE user_id = NEW.user_id AND key = NEW.key;
END;

-- 创建用户表
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    token TEXT,
    nickname TEXT,
    avatar TEXT,
    email TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 创建用户表索引
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_token ON users(token);

-- 创建用户表的updated_at触发器
CREATE TRIGGER IF NOT EXISTS trg_users_updated_at
AFTER UPDATE ON users
BEGIN
    UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- 为nodes表添加user_id字段，默认值为0
ALTER TABLE nodes ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;

-- 创建nodes表的user_id索引
CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);
CREATE INDEX IF NOT EXISTS idx_nodes_user_parent ON nodes(user_id, parent_id);

-- 为config表添加复合索引，优化用户配置查询
CREATE INDEX IF NOT EXISTS idx_config_user_key ON config(user_id, key);