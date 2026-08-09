package main

// Tests for admin endpoints in main.go.
// These tests use an in-memory SQLite database and exercise the admin
// handlers directly via httptest.
//
// Conventions:
//   - Each bug fix is driven by a failing test written first (RED),
//     then the smallest possible change to flip it to GREEN.
//   - Tests are table-driven where it clarifies multiple scenarios.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	_ "modernc.org/sqlite"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func testDBName(t *testing.T) string {
	t.Helper()
	// Temp file (not :memory:) avoids the "out of memory" error some
	// modernc.org/sqlite builds emit for shared in-memory caches.
	path := filepath.Join(os.TempDir(),
		fmt.Sprintf("bookmarks_test_%s_%d.db", t.Name(), time.Now().UnixNano()))
	return fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", path)
}

// newTestServer wires up a *server backed by a fresh in-memory SQLite.
// It creates the users, nodes, and audit_log tables the admin endpoints
// need without going through the full upgrade system.
func newTestServer(t *testing.T) (*server, *sql.DB) {
	t.Helper()
	dsn := testDBName(t)
	dbPath := strings.TrimPrefix(strings.TrimSuffix(dsn, "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"), "file:")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	mustExec(t, db, `
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
			last_login_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	mustExec(t, db, `
		CREATE TABLE nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 0,
			parent_id INTEGER,
			type TEXT NOT NULL CHECK (type IN ('folder', 'bookmark')),
			title TEXT NOT NULL,
			url TEXT,
			favicon_url TEXT,
			remark TEXT NOT NULL DEFAULT '',
			visibility TEXT NOT NULL DEFAULT 'private',
			position INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	mustExec(t, db, `
		CREATE TABLE audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id INTEGER NOT NULL DEFAULT 0,
			detail TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)

	srv := &server{db: db, httpClient: http.DefaultClient}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})
	return srv, db
}

func mustExec(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

func TestLoadTree_PreservesSQLSiblingOrder(t *testing.T) {
	srv, db := newTestServer(t)
	userID := insertTestUser(t, db, "tree-order", false)

	insert := func(parentID *int64, nodeType, title string, position int) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO nodes (user_id, parent_id, type, title, position) VALUES (?, ?, ?, ?, ?)`, userID, parentID, nodeType, title, position)
		if err != nil {
			t.Fatalf("insert %s: %v", title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id for %s: %v", title, err)
		}
		return id
	}

	second := insert(nil, nodeTypeFolder, "second", 2)
	first := insert(nil, nodeTypeFolder, "first", 1)
	insert(&first, nodeTypeBookmark, "later child", 2)
	insert(&first, nodeTypeBookmark, "first child", 1)
	_ = second

	tree, err := srv.loadTree(context.Background(), userID)
	if err != nil {
		t.Fatalf("load tree: %v", err)
	}
	if len(tree) != 2 || tree[0].Title != "first" || tree[1].Title != "second" {
		t.Fatalf("root order = %#v, want [first second]", tree)
	}
	if len(tree[0].Children) != 2 || tree[0].Children[0].Title != "first child" || tree[0].Children[1].Title != "later child" {
		t.Fatalf("child order = %#v, want [first child later child]", tree[0].Children)
	}
}

func TestCreateBookmark_DoesNotWaitForRemoteMetadata(t *testing.T) {
	srv, db := newTestServer(t)
	userID := insertTestUser(t, db, "create-fast", false)

	// A nil favicon queue takes the non-blocking default branch. If the
	// handler ever returns to synchronous metadata fetching, this client
	// makes the regression deterministic instead of relying on the network.
	srv.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("create bookmark must not issue a remote metadata request")
		return nil, nil
	})}
	req := jsonRequest(t, http.MethodPost, "/bookmarks", map[string]string{"url": "https://example.com"})
	req = req.WithContext(withUserID(req.Context(), userID))
	rec := runHandler(srv, srv.handleCreateBookmark, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bookmark: want 201, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	var created node
	decodeJSON(t, rec, &created)
	if created.Title != "https://example.com" {
		t.Fatalf("temporary title = %q, want normalized URL", created.Title)
	}
}

// insertTestUser inserts a users row and stores a stable token of the
// form "tok-<username>" so callers can authenticate by passing it as
// the Authorization header. nickname defaults to the username so
// handleGetUsers can scan it as a non-null string.
func insertTestUser(t *testing.T, db *sql.DB, username string, isAdmin bool) int64 {
	t.Helper()
	admin := 0
	if isAdmin {
		admin = 1
	}
	token := "tok-" + username
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO users (username, password, nickname, email, is_admin, is_active, token) VALUES (?, ?, ?, ?, ?, 1, ?)`,
		username, "x", username, username+"@example.com", admin, token)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// userTokenFor returns the canonical token stored for a user created via
// insertTestUser. Tests pass it in the Authorization header so the real
// tokenAuthMiddleware accepts the request.
func userTokenFor(username string) string { return "tok-" + username }

// withUserID returns a context carrying a user id, matching the
// shape main.go uses after the token-auth middleware sets it.
func withUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

// jsonRequest builds an *http.Request with a JSON body (or nil for GET/DELETE).
func jsonRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, reader)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

// newRecorder executes the handler under test and returns the recorder.
func runHandler(srv *server, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec
}

// newAdminRouter returns a chi router that mounts only the admin endpoints
// with the same middleware stack as main.go. Tests use this to assert
// end-to-end status codes without spinning up a real HTTP listener.
func newAdminRouter(srv *server) http.Handler {
	r := chi.NewRouter()
	r.Use(srv.tokenAuthMiddlewareChi)
	r.Use(srv.adminMiddlewareChi)
	r.Get("/admin/stats", srv.handleAdminStats)
	r.Get("/admin/users/{userId}/tree", srv.handleAdminGetUserTree)
	r.Put("/admin/nodes/{id}", srv.handleAdminUpdateNode)
	r.Delete("/admin/nodes/{id}", srv.handleAdminDeleteNode)
	r.Get("/admin/audit-log", srv.handleGetAuditLog)
	r.Post("/admin/folders", srv.handleAdminCreateFolder)
	r.Post("/admin/bookmarks", srv.handleAdminCreateBookmark)
	r.Put("/admin/nodes/reorder", srv.handleAdminReorderNodes)
	return r
}

// do runs an HTTP request through the router and returns the response.
func do(t *testing.T, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// decodeJSON unmarshals a response body into target and fails the test
// on any decode error.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("decode json (status=%d, body=%q): %v", rec.Code, rec.Body.String(), err)
	}
}

// contains reports whether sub is contained in s.
func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
