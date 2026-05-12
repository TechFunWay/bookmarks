# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Commands

```bash
# Run the app (default port 8901)
go run main.go

# Run with custom port and data path
go run main.go -port 3000 -dataUrl /path/to/data

# Debug logging
go run main.go -logmode debug

# Build a single binary
go build -o bookmarks main.go

# Build the reset-password CLI tool
cd cmd/reset-password && go build -o reset-password .

# Run tests
go test ./...
```

## Architecture

This is a **single-binary Go web app** for bookmark management. The entire backend lives in `main.go` (~3700 lines), with static frontend files embedded via `//go:embed static`. There is no separate frontend build step — the `static/` directory contains raw HTML/JS/CSS served directly.

### Go module name is `bookmark` (not a full path).

### Request flow
1. `main()` parses flags, initializes SQLite DB, runs the upgrade system (`app/logic/sys_update.go`), then starts chi router on the configured port.
2. Chi router with middleware stack: logging → recoverer → content type → CORS.
3. All API routes are under `/api`. Auth uses a token-based scheme fetched from the `users.token` column. Three auth middleware variants: `tokenAuthMiddleware` (required), `optionalAuthMiddleware` (attempts auth but continues for anonymous users — used for the "no-login mode" where admin data is shared), and `apiKeyAuthMiddleware` (for browser extension sync, uses `X-API-Key` header).
4. Static files are served as catch-all `/*` via the embedded `staticFS`.

### Database
- **SQLite** via `modernc.org/sqlite` (pure Go, no CGO, cross-compiles trivially).
- The main table is `nodes` with a `type` column (`folder` or `bookmark`), `parent_id` for tree structure, and `user_id` for multi-tenancy.
- `users` table stores credentials. Password hashing is **double MD5**: frontend does `MD5(password)`, backend does `MD5(frontendHash + "bookmarks")`. The reset-password CLI tool uses its own single MD5 (note the inconsistency — `cmd/reset-password/main.go` uses `crypto/md5` directly).
- `sys_config` table stores per-user key-value configuration.
- `security_questions` table for password recovery.
- `sys_update` table tracks version upgrade history.

### Upgrade system (`app/logic/sys_update.go`)
On startup, the app compares the last recorded successful upgrade version against the hardcoded `appVersion` constant. It runs version-specific SQL migrations and data processing logic in order. The hardcoded version list is `["v1.7.0", "v1.8.0", "v1.9.0", "v2.0.0"]` — when bumping the app version, you must add a new case to the switch statements in `sys_update.go`.

### Browser sync (`app/logic/browser_sync.go`)
The Edge browser extension (`edge-extension/`) syncs bookmarks via the `/api/sync/*` endpoints, which use API key authentication. The sync logic supports bidirectional sync with batch operations (create/update/delete in a single transaction).

### Key directories
- `app/logger/` — HTTP request logging middleware
- `app/logic/` — Business logic extracted from main.go: browser sync, security questions, system upgrade
- `app/utils/` — Shared utilities (MD5 hashing, string utils, retry helpers)
- `cmd/reset-password/` — Standalone CLI for resetting a user's password directly in the SQLite DB
- `edge-extension/` — Edge/Chrome browser extension (Manifest V3) for bookmark sync
- `static/` — Vue.js 3 frontend (no build step, uses `vue.global.prod.js` from CDN-like embedded file)
- `data/` — Runtime data directory (DB file, favicon cache, logs)
- `release/` — Pre-compiled binaries organized by version and platform
- `techfunway.bookmarks/` — Config files for the fnapp packaging/distribution tool
