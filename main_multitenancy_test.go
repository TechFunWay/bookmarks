package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func insertTestNode(t *testing.T, db *sql.DB, userID int64, parentID *int64, nodeType, title, url, visibility string, position int) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(), `
		INSERT INTO nodes (user_id, parent_id, type, title, url, visibility, position)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, parentID, nodeType, title, url, visibility, position)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("node LastInsertId: %v", err)
	}
	return id
}

func TestMultiUserTreeAndMutationIsolation(t *testing.T) {
	srv, db := newTestServer(t)
	aliceID := insertTestUser(t, db, "alice", false)
	bobID := insertTestUser(t, db, "bob", false)

	aliceFolderID := insertTestNode(t, db, aliceID, nil, nodeTypeFolder, "Alice private", "", "private", 0)
	aliceBookmarkID := insertTestNode(t, db, aliceID, &aliceFolderID, nodeTypeBookmark, "Alice bookmark", "https://alice.example", "private", 0)
	insertTestNode(t, db, bobID, nil, nodeTypeFolder, "Bob private", "", "private", 0)

	aliceTree, err := srv.loadTree(context.Background(), aliceID)
	if err != nil {
		t.Fatalf("load Alice tree: %v", err)
	}
	if len(aliceTree) != 1 || aliceTree[0].ID != aliceFolderID {
		t.Fatalf("Alice tree leaked or lost nodes: %+v", aliceTree)
	}
	if len(aliceTree[0].Children) != 1 || aliceTree[0].Children[0].ID != aliceBookmarkID {
		t.Fatalf("Alice tree missing own bookmark: %+v", aliceTree[0].Children)
	}

	if _, err := srv.loadTree(context.Background(), bobID); err != nil {
		t.Fatalf("load Bob tree: %v", err)
	}
	if err := srv.updateNode(context.Background(), bobID, aliceBookmarkID, updateNodeRequest{Title: stringPtr("must not update")}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user update: want sql.ErrNoRows, got %v", err)
	}
}

func TestSyncAPIKeyIsScopedToOwningUser(t *testing.T) {
	srv, db := newTestServer(t)
	aliceID := insertTestUser(t, db, "alice", false)
	bobID := insertTestUser(t, db, "bob", false)
	mustExec(t, db, "UPDATE users SET api_key = ? WHERE id = ?", "key-alice", aliceID)
	mustExec(t, db, "UPDATE users SET api_key = ? WHERE id = ?", "key-bob", bobID)
	insertTestNode(t, db, aliceID, nil, nodeTypeBookmark, "Alice bookmark", "https://alice.example", "private", 0)
	insertTestNode(t, db, bobID, nil, nodeTypeBookmark, "Bob bookmark", "https://bob.example", "private", 0)

	r := chi.NewRouter()
	r.Use(srv.apiKeyAuthMiddlewareForChi)
	r.Get("/api/sync/tree", srv.handleSyncGetTree)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/tree", nil)
	req.Header.Set("X-API-Key", "key-alice")
	rec := do(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync tree: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	var response struct {
		Nodes []struct {
			Title string `json:"title"`
		} `json:"nodes"`
	}
	decodeJSON(t, rec, &response)
	if len(response.Nodes) != 1 || response.Nodes[0].Title != "Alice bookmark" {
		t.Fatalf("API key returned wrong user's nodes: %+v", response.Nodes)
	}
}

func TestNoLoginModeUsesActiveAdministratorData(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)
	insertTestUser(t, db, "alice", false)
	mustExec(t, db, `CREATE TABLE sys_config (
		user_id INTEGER NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (user_id, key)
	)`)
	mustExec(t, db, "INSERT INTO sys_config (user_id, key, value) VALUES (0, 'require_login', 'false')")
	insertTestNode(t, db, adminID, nil, nodeTypeBookmark, "Administrator bookmark", "https://admin.example", "private", 0)

	h := srv.optionalAuthMiddleware(srv.handleGetTree)
	req := httptest.NewRequest(http.MethodGet, "/api/tree", nil)
	rec := runHandler(srv, h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("no-login tree: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	var tree []node
	if err := json.Unmarshal(rec.Body.Bytes(), &tree); err != nil {
		t.Fatalf("decode no-login tree: %v", err)
	}
	if len(tree) != 1 || tree[0].Title != "Administrator bookmark" {
		t.Fatalf("no-login mode did not use administrator tree: %+v", tree)
	}
}

func stringPtr(value string) *string {
	return &value
}
