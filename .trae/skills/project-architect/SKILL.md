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
- System upgrade management
- RESTful API with JSON responses

## Technology Stack

### Core Dependencies
- **Go 1.24.0** - Programming language
- **chi/v5** - HTTP router and middleware
- **modernc.org/sqlite** - SQLite database driver
- **golang.org/x/crypto** - Password hashing (bcrypt)
- **google/uuid** - UUID generation for tokens
- **golang.org/x/net/html** - HTML parsing for metadata extraction

### Database
- **SQLite** with foreign key support
- Embedded in application data directory

## Project Structure

```
bookmarks/
├── main.go                          # Main application entry point (~3000 lines)
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
└── techfunway.bookmarks/            # FnApp packaging files
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
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```
- **Indexes**: `idx_nodes_parent`, `idx_nodes_parent_position`, `idx_nodes_user_id`
- **Trigger**: Auto-update `updated_at` on modifications

#### `users` - User accounts
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,  -- bcrypt hashed
    token TEXT,              -- Session token
    nickname TEXT,
    avatar TEXT,
    email TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```
- **Indexes**: `idx_users_username`, `idx_users_email`, `idx_users_token`

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
- `GET /api/auth/me` - Get current user (authenticated)
- `GET /api/auth/check` - Check authentication status

### Bookmark Management
- `GET /api/tree` - Get bookmark tree (authenticated)
- `POST /api/folders` - Create folder (authenticated)
- `POST /api/bookmarks` - Create bookmark (authenticated)
- `PUT /api/nodes/{id}` - Update node (authenticated)
- `DELETE /api/nodes/{id}` - Delete node (authenticated)
- `POST /api/nodes/batch-delete` - Batch delete nodes (authenticated)
- `POST /api/nodes/reorder` - Reorder nodes (authenticated)

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
- `/icons/*` - Serve favicon images from data directory

## Core Business Logic

### 1. Application Initialization ([main.go:133-305](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L133-L305))

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
- App version: `v1.8.0`

### 2. Authentication System

**Token-based authentication:**
- Token stored in `users.token` column
- Passed via `Authorization` header or `?token=` query parameter
- Auth middleware validates token and sets `userContextKey` in request context

**Password handling:**
- Hashed with bcrypt before storage
- Minimum 6 characters
- Old password verification required for changes

**User roles:**
- `is_admin`: Can modify system config (e.g., `allow_register`)
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
    Position   int
    Children   []*node // populated for tree response
    CreatedAt  string
    UpdatedAt  string
}
```

**Key operations:**
- **Load tree** ([main.go:1257-1355](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L1257-L1355)): Fetch all nodes, build hierarchical structure
- **Insert node** ([main.go:1371-1459](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L1371-L1459)): Validate parent, check uniqueness, assign position
- **Update node** ([main.go:1461-1661](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L1461-L1661)): Validate changes, prevent cycles, update metadata
- **Reorder nodes** ([main.go:1663-1703](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L1663-L1703)): Update position values

**Validation rules:**
- Same folder cannot have duplicate folder names
- Same folder cannot have duplicate bookmark (title + URL)
- Cannot move folder into its own descendants (cycle detection)
- Parent must be a folder

### 4. Metadata Fetching

**Process** ([main.go:1808-2014](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L1808-L2014)):
1. Normalize URL (add https:// if missing)
2. Send HTTP GET with browser-like headers
3. Handle redirects (up to 10)
4. Handle gzip/deflate compression
5. Extract title from `<title>` tag or meta tags
6. Extract favicon from `<link rel="icon">` or default to `/favicon.ico`
7. Download and save favicon to local file
8. Return title and local icon path

**Retry logic:**
- Up to 2 retries with exponential backoff
- Fallback to hostname as title if all methods fail

**Icon storage:**
- Saved to `data/icons/YYYYMMDD/UUID.ext`
- Served via `/icons/` route
- Supports PNG, JPG, GIF, WEBP, SVG, ICO formats

### 5. Import System

**JSON import** ([main.go:699-789](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L699-L789)):
- Accepts hierarchical bookmark tree
- Modes: `merge` (skip duplicates) or `replace` (clear first)
- Queues favicon fetching for imported bookmarks

**Edge HTML import** ([main.go:799-913](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go#L799-L913)):
- Parses Edge's bookmark HTML format
- Handles `<h3>` (folder), `<a>` (bookmark), `<dl>` (container)
- Extracts base64 icons from `icon` attribute
- Saves icons to local files

### 6. System Upgrade

**Upgrade manager** ([sys_update.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/app/logic/sys_update.go)):
- Tracks upgrade history in `sys_update` table
- Compares current version with database version
- Executes SQL migrations for each version
- Runs data processing logic per version
- Logs all upgrade operations to dedicated log file

**Available versions:**
- `v1.7.0`: Added user system, config table
- `v1.8.0`: Current version (no schema changes)

### 7. Logging

**Access logging** ([logger.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/app/logger/logger.go)):
- Custom middleware wraps ResponseWriter
- Logs: timestamp, method, path, client IP, status, response size, duration
- Daily log rotation: `access_YYYYMMDD.log`
- Auto-cleanup: deletes logs older than 3 days

**Debug logging:**
- Only active when `logmode=debug`
- Uses `Debug()` function throughout codebase
- Error logging always active via `Error()` function

## Key Constants and Configuration

```go
const (
    appVersion = "v1.8.0"
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

## File Organization Guidelines

When modifying code:

1. **Main application logic**: Add to [main.go](file:///Users/weiyi/develop/gitee/TechFunWay/bookmarks/main.go) for now (monolithic structure)
2. **Reusable utilities**: Add to `app/utils/` directory
3. **Business logic modules**: Create new files in `app/logic/` if complex
4. **Logging**: Use existing `Debug()` and `Error()` functions
5. **Database changes**: Add upgrade script to `sys_update.go`

## Testing

Run tests with:
```bash
go test ./...
```

## Build

```bash
go build -o bookmarks main.go
```

Run with custom data directory:
```bash
./bookmarks -dataUrl /path/to/data -port 8901 -logmode debug
```
