---
name: "project-architect"
description: "Provides comprehensive understanding of the bookmarks project architecture, tech stack, database models, API routes, and business logic. Invoke when user asks to modify code, add features, or understand the project structure."
---

# Project Architect - Bookmarks Application

## Project Overview

This is a **multi-user bookmark management system** built with Go, featuring:
- User authentication (register/login/logout)
- Hierarchical bookmark organization (folders and bookmarks)
- Web metadata fetching (title and favicon)
- Edge browser import support
- API Key authentication for browser extension sync
- Bookmark remark (notes) support
- Security questions for password reset
- User management (admin)
- System upgrade management
- RESTful API with JSON responses

## Technology Stack

### Core Dependencies
- **Go 1.24.0** - Programming language
- **chi/v5** - HTTP router and middleware
- **modernc.org/sqlite** - SQLite database driver (pure Go, no CGO)
- **golang.org/x/crypto** - Password hashing (bcrypt)
- **google/uuid** - UUID generation for tokens
- **golang.org/x/net/html** - HTML parsing for metadata extraction

### Database
- **SQLite** with foreign key support
- Embedded in application data directory

## Project Structure

```
bookmarks/
├── main.go                          # Main application entry point
├── cmd/
│   └── reset-password.go            # Password reset CLI tool
├── go.mod / go.sum                  # Go module dependencies
├── static/                          # Embedded static files (HTML, CSS, JS)
│   ├── index.html, login.html, register.html
│   ├── app.js, style.css
│   └── img/
├── app/
│   ├── logger/logger.go             # Logging middleware and file rotation
│   ├── logic/sys_update.go          # System upgrade manager
│   └── utils/                       # Utility functions
│       ├── common_utils.go          # Random, retry, slice operations
│       └── string_utils.go          # String manipulation utilities
├── edge-extension/                  # Edge browser extension
├── techfunway.bookmarks/            # FnApp packaging files
├── Dockerfile                       # Multi-arch Docker build (ARG VERSION, ARG TARGETARCH)
├── docker-compose.yaml              # Docker Compose configuration
├── SYS_UPGRADE.md                   # System upgrade documentation
├── CMD_RESET_PASSWORD.md            # Password reset documentation
└── RELEASE.md                       # Release notes
```

## Database Schema

### Tables

#### `nodes` - Bookmark and folder storage
```sql
CREATE TABLE nodes (
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
);
```
- **Indexes**: `idx_nodes_parent`, `idx_nodes_parent_position`, `idx_nodes_user_id`, `idx_nodes_user_id_parent`
- **Trigger**: `trg_nodes_updated_at` - Auto-update `updated_at` on modifications

#### `users` - User accounts
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    token TEXT,
    nickname TEXT,
    avatar TEXT,
    email TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_admin INTEGER NOT NULL DEFAULT 0,
    api_key TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```
- **Indexes**: `idx_users_username`, `idx_users_email`, `idx_users_token`, `idx_users_api_key`
- **Trigger**: `trg_users_updated_at` - Auto-update `updated_at` on modifications
- **Password**: Dual MD5 hash (first `MD5(password)`, then `MD5(firstHash + "bookmarks")`)
- **API Key**: 32-character random hex string for browser extension sync

#### `security_questions` - Security questions for password reset (v2.0.0)
```sql
CREATE TABLE security_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    question1 TEXT NOT NULL,
    answer1 TEXT NOT NULL,
    question2 TEXT NOT NULL,
    answer2 TEXT NOT NULL,
    question3 TEXT NOT NULL,
    answer3 TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

#### `sys_config` - System and user configuration
```sql
CREATE TABLE sys_config (
    user_id INTEGER NOT NULL DEFAULT 0,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);
```
- **user_id = 0**: System-wide configuration (e.g., `allow_register`)
- **user_id > 0**: User-specific settings
- **Index**: `idx_sys_config_user_key`
- **Trigger**: `trg_sys_config_updated_at`

#### `sys_update` - Upgrade history
```sql
CREATE TABLE sys_update (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version TEXT NOT NULL,
    content TEXT NOT NULL,
    upgrade_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    result TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 0  -- 0: pending, 1: success, -1: failed
);
```

## API Routes

### Authentication (`/api/auth`)
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - User login
- `POST /api/auth/logout` - User logout
- `POST /api/auth/change-password` - Change password (authenticated)
- `POST /api/auth/regenerate-api-key` - Regenerate API key (authenticated)
- `GET /api/auth/me` - Get current user (authenticated)
- `GET /api/auth/check` - Check authentication status
- `POST /api/auth/security-questions` - Set security questions (authenticated)
- `GET /api/auth/security-questions` - Get security questions (authenticated)
- `GET /api/auth/security-questions/reset` - Get security questions for reset (no auth)
- `POST /api/auth/verify-and-reset` - Verify answers and reset password (no auth)

### User Management (`/api/users`, admin only)
- `GET /api/users/` - List all users
- `GET /api/users/{id}` - Get user by ID
- `PUT /api/users/{id}` - Update user
- `DELETE /api/users/{id}` - Delete user
- `POST /api/users/{id}/reset-password` - Reset user password
- `POST /api/users/batch` - Batch user operations

### Bookmark Management
- `GET /api/tree` - Get bookmark tree (authenticated)
- `POST /api/folders` - Create folder (authenticated)
- `POST /api/bookmarks` - Create bookmark (authenticated)
- `PUT /api/nodes/{id}` - Update node (authenticated)
- `DELETE /api/nodes/{id}` - Delete node (authenticated)
- `DELETE /api/bookmarks/{id}` - Delete bookmark (authenticated)
- `POST /api/nodes/batch-delete` - Batch delete nodes (authenticated)
- `POST /api/nodes/reorder` - Reorder nodes (authenticated)
- `GET /api/check-duplicates` - Check for duplicate bookmarks (authenticated)

### Browser Sync API (`/api/sync`, API Key authentication)
- `GET /api/sync/bookmarks` - Get bookmarks
- `POST /api/sync/bookmarks` - Create bookmark
- `PUT /api/sync/bookmarks/{id}` - Update bookmark
- `DELETE /api/sync/bookmarks/{id}` - Delete bookmark
- `GET /api/sync/folders` - Get folders
- `POST /api/sync/folders` - Create folder
- `PUT /api/sync/folders/{id}` - Update folder
- `DELETE /api/sync/folders/{id}` - Delete folder
- `POST /api/sync/batch` - Batch sync operations
- `GET /api/sync/tree` - Get full tree

### Import/Export
- `POST /api/import` - Import bookmarks (JSON format, authenticated)
- `POST /api/import-edge` - Import from Edge browser (HTML format, authenticated)
- `GET /api/metadata` - Fetch URL metadata (title, favicon)

### Configuration
- `GET /api/config/system` - Get system configuration (no auth)
- `GET /api/config` - Get user configuration (authenticated)
- `POST /api/config` - Update configuration (authenticated)

### System
- `GET /api/version` - Get application version

### Static Files
- `/*` - Serve embedded static files
- `/static/*` - Serve embedded static files (with prefix stripping)
- `/icons/*` - Serve favicon images from data directory

## Core Business Logic

### 1. Application Initialization

**Startup Sequence:**
1. Parse command-line flags: `-dataUrl`, `-port`, `-logmode`
2. Create data directories: `data/icons/`, `data/db/`, `data/logs/`
3. Migrate old data (icons and database from legacy paths)
4. Open SQLite database with foreign keys enabled
5. Initialize database schema if needed
6. Execute system upgrades ([sys_update.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/app/logic/sys_update.go))
7. Create HTTP client with TLS skip verify (for intranet)
8. Start favicon worker goroutine (async icon fetching)
9. Setup HTTP router with chi
10. Start HTTP server

**Configuration:**
- Default port: `8901`
- Default data path: `./data`
- Log modes: `debug` or `release`
- App version: `v2.0.0`

### 2. Authentication System

**Dual authentication:**
- **Token-based**: Token stored in `users.token` column, passed via `Authorization` header or `?token=` query parameter
- **API Key-based**: API key stored in `users.api_key` column, used for browser extension sync (`/api/sync/*` routes)

**Password handling:**
- Dual MD5 hash: `MD5(MD5(password) + "bookmarks")`
- Minimum 6 characters
- Old password verification required for changes
- Security questions for self-service password reset

**User roles:**
- `is_admin`: Can manage users, modify system config
- First registered user automatically becomes admin

### 3. Bookmark Tree Management

**Data structures:**
```go
type node struct {
    ID         int64
    ParentID   *int64  // nil for root nodes
    Type       string  // "folder" or "bookmark"
    Title      string
    URL        *string // nil for folders
    FaviconURL *string
    Remark     string  // bookmark notes
    Position   int
    Children   []*node // populated for tree response
    CreatedAt  string
    UpdatedAt  string
}
```

**Validation rules:**
- Same folder cannot have duplicate folder names
- Same folder cannot have duplicate bookmark (title + URL)
- Cannot move folder into its own descendants (cycle detection)
- Parent must be a folder

### 4. Metadata Fetching

**Process:**
1. Normalize URL (add https:// if missing)
2. Send HTTP GET with browser-like headers
3. Handle redirects (up to 10)
4. Handle gzip/deflate compression
5. Extract title from `<title>` tag or meta tags
6. Extract favicon from `<link rel="icon">` or default to `/favicon.ico`
7. Download and save favicon to local file
8. Return title and local icon path

**Icon storage:**
- Saved to `data/icons/YYYYMMDD/UUID.ext`
- Served via `/icons/` route
- Supports PNG, JPG, GIF, WEBP, SVG, ICO formats

### 5. Import System

**JSON import**: Accepts hierarchical bookmark tree, modes: `merge` (skip duplicates) or `replace` (clear first)

**Edge HTML import**: Parses Edge's bookmark HTML format, handles `<h3>` (folder), `<a>` (bookmark), `<dl>` (container)

### 6. System Upgrade

**Upgrade manager** ([sys_update.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/app/logic/sys_update.go)):
- Tracks upgrade history in `sys_update` table
- Compares current version with database version
- Executes SQL migrations for each version (SQL embedded in Go code)
- Runs data processing logic per version
- Logs all upgrade operations to dedicated log file

**Available versions:**
- `v1.7.0`: Added user system, config table, user_id on nodes
- `v1.8.0`: Initialize default config `allow_register=true`
- `v1.9.0`: Added api_key on users, remark on nodes, password reset
- `v2.0.0`: Added security_questions table

### 7. Password Reset CLI Tool

**Location**: [cmd/reset-password.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/cmd/reset-password.go)

**Usage:**
```bash
./reset-password -username admin -password 123456
./reset-password -password 123456  # auto-find admin
```

**Features:**
- Auto-find admin account if username not specified
- Dual MD5 hash password
- Update database directly

### 8. Logging

**Access logging** ([logger.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/app/logger/logger.go)):
- Custom middleware wraps ResponseWriter
- Logs: timestamp, method, path, client IP, status, response size, duration
- Daily log rotation: `access_YYYYMMDD.log`
- Auto-cleanup: deletes logs older than 3 days

## Key Constants and Configuration

```go
const (
    appVersion = "v2.0.0"
    nodeTypeFolder   = "folder"
    nodeTypeBookmark = "bookmark"
    logModeDebug   = "debug"
    logModeRelease = "release"
)

// Database connection pool
db.SetMaxOpenConns(10)
db.SetMaxIdleConns(2)

// HTTP client
Timeout: 10 * time.Second
MaxRedirects: 10
TLS InsecureSkipVerify: true (for intranet)

// Favicon queue
Buffer size: 100
```

## Important Patterns

### Error Handling
- Custom errors: `ErrInvalidParent`, `ErrCycleDetected`, `ErrDuplicateFolderName`, `ErrDuplicateBookmark`
- Consistent error responses: `{"error": "message"}`
- HTTP status codes: 400 (bad request), 401 (unauthorized), 403 (forbidden), 404 (not found), 500 (server error)

### Database Transactions
- Always use transactions for multi-step operations
- `defer tx.Rollback()` pattern
- Commit only after all operations succeed

### Context Usage
- Pass `r.Context()` to all database operations
- Use `getUserID(r)` to extract authenticated user ID from context

### Async Operations
- Favicon fetching uses buffered channel
- Worker goroutine processes queue independently
- Non-blocking queue: drops if full

### Authentication Middleware
- `authMiddleware`: Token-based auth for web UI
- `apiKeyAuthMiddleware`: API Key-based auth for browser sync

## File Organization Guidelines

When modifying code:

1. **Main application logic**: Add to [main.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go) for now (monolithic structure)
2. **CLI tools**: Add to `cmd/` directory
3. **Reusable utilities**: Add to `app/utils/` directory
4. **Business logic modules**: Create new files in `app/logic/` if complex
5. **Logging**: Use existing `Debug()` and `Error()` functions
6. **Database changes**: Add upgrade SQL and logic to `sys_update.go`

## Build & Deploy

### Build
```bash
go build -o bookmarks main.go
go build -o reset-password ./cmd/reset-password.go
```

### Docker
```bash
# Build multi-arch image using Buildx
bash .trae/skills/docker-builder/scripts/docker_builder.sh

# Or use docker-compose
docker-compose up -d
```

### Run
```bash
./bookmarks -dataUrl /path/to/data -port 8901 -logmode debug
```
