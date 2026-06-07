package main

// Regression tests for handleAdminStats (main.go:3235).
//
// Bug B10: the handler ran five Count(*) queries with no error handling.
// If any query failed (e.g. a missing audit_log table after a partial
// upgrade, a corrupt db, or a permission issue), the offending counter
// silently stayed at 0 and the client received a deceptively normal
// 200 response. This hides production incidents and makes triage slow.
//
// After the fix, a query failure must surface as 500 and no counter
// should be reported as 0 by accident.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAdminStats_HappyPath(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	insertTestUser(t, db, "alice", false)

	r := newAdminRouter(srv)
	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	var stats struct {
		TotalUsers      int `json:"total_users"`
		TotalBookmarks  int `json:"total_bookmarks"`
		TotalFolders    int `json:"total_folders"`
		PublicBookmarks int `json:"public_bookmarks"`
		TotalNodes      int `json:"total_nodes"`
	}
	decodeJSON(t, rec, &stats)
	if stats.TotalUsers != 2 {
		t.Fatalf("total_users: want 2, got %d", stats.TotalUsers)
	}
	if stats.TotalBookmarks != 0 || stats.TotalFolders != 0 {
		t.Fatalf("expected 0 bookmarks/folders, got %d/%d", stats.TotalBookmarks, stats.TotalFolders)
	}
	if stats.TotalNodes != 0 {
		t.Fatalf("expected 0 total nodes, got %d", stats.TotalNodes)
	}
}

// TestHandleAdminStats_DBError_Surfaces500 drops the nodes table to
// force the four node-related count queries to fail. The handler must
// surface the error as 500 (currently it returns 0 silently).
func TestHandleAdminStats_DBError_Surfaces500(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)

	if _, err := db.Exec(`DROP TABLE nodes`); err != nil {
		t.Fatalf("drop nodes: %v", err)
	}

	r := newAdminRouter(srv)
	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("want non-200 when nodes table is missing, got 200 (body=%q)", rec.Body.String())
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}
