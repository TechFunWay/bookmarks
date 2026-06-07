package main

// Feature tests for F2, F3, F4: audit log search, CSV export, and user
// list filtering. All three features share the same admin surface and
// are exercised through the admin router.
//
// F2 — handleGetAuditLog ?search= substring filter on the `detail`
//      column. The contract is the same as the existing filters: an
//      empty `search` is a no-op, any other value restricts the result
//      set via LIKE %term%.
// F3 — /api/admin/audit-log/export returns a CSV body with the same
//      filters as the JSON list endpoint.
// F4 — handleGetUsers supports is_active / is_admin filter params and
//      a `sort` param (created_at, last_login_at, username) with
//      deterministic ordering.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// seedAuditLogFull inserts an audit_log row with all fields populated.
// Use this when the test cares about target_type, target_id, or detail;
// the simpler seedAuditLog helper only writes user_id and action.
func seedAuditLogFull(t *testing.T, srv *server, userID int64, action, targetType string, targetID int64, detail string) int64 {
	t.Helper()
	res, err := srv.db.ExecContext(context.Background(),
		`INSERT INTO audit_log (user_id, action, target_type, target_id, detail) VALUES (?, ?, ?, ?, ?)`,
		userID, action, targetType, targetID, detail)
	if err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// F2 ----------------------------------------------------------------

func TestAuditLog_SearchByDetail(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)

	seedAuditLogFull(t, srv, adminID, "user_create", "user", aliceID, "admin created alice")
	seedAuditLogFull(t, srv, adminID, "user_update", "user", aliceID, "updated nickname")
	seedAuditLogFull(t, srv, adminID, "user_delete", "user", aliceID, "removed bob from group")

	r := newAdminRouter(srv)

	tests := []struct {
		name      string
		search    string
		wantTotal int
	}{
		{"empty_search_returns_all", "", 3},
		{"match_substring_alice", "alice", 1},
		{"match_substring_user", "user", 0}, // 'user' not in any detail
		{"no_match_returns_zero", "nonexistent_keyword_xyz", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/admin/audit-log"
			if tc.search != "" {
				path += "?search=" + tc.search
			}
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", userTokenFor("root"))
			rec := do(t, r, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
			}
			var resp struct {
				Total int64 `json:"total"`
			}
			decodeJSON(t, rec, &resp)
			if int(resp.Total) != tc.wantTotal {
				t.Fatalf("search=%q: want %d, got %d (body=%q)",
					tc.search, tc.wantTotal, resp.Total, rec.Body.String())
			}
		})
	}
}

// F3 ----------------------------------------------------------------

func TestAuditLog_ExportCSV(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)
	seedAuditLogFull(t, srv, adminID, "user_create", "user", 1, "test detail A")
	seedAuditLogFull(t, srv, adminID, "user_delete", "user", 2, "test detail B")

	r := newAuditExportRouter(srv)

	req := httptest.NewRequest("GET", "/admin/audit-log/export", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("Content-Type: want text/csv*, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id,user_id,username,action,target_type,target_id,detail,ip_address,created_at") {
		t.Fatalf("CSV missing header row, body=%q", body)
	}
	if !strings.Contains(body, "test detail A") || !strings.Contains(body, "test detail B") {
		t.Fatalf("CSV missing expected rows, body=%q", body)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("Content-Disposition: want attachment, got %q", got)
	}
}

func TestAuditLog_ExportCSV_AppliesFilters(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)
	seedAuditLogFull(t, srv, adminID, "user_create", "user", aliceID, "create alice")
	seedAuditLogFull(t, srv, adminID, "user_create", "user", 999, "create someone else")

	r := newAuditExportRouter(srv)
	req := httptest.NewRequest("GET", "/admin/audit-log/export?search=alice", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "create alice") {
		t.Fatalf("CSV should contain matching row, body=%q", body)
	}
	if strings.Contains(body, "create someone else") {
		t.Fatalf("CSV should not contain non-matching row, body=%q", body)
	}
}

// F4 ----------------------------------------------------------------

func TestUsers_FilterByIsActive(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	activeID := insertTestUser(t, db, "alice", false)
	disabledID := insertTestUser(t, db, "bob", false)
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET is_active = 0 WHERE id = ?`, disabledID); err != nil {
		t.Fatalf("disable bob: %v", err)
	}
	_ = activeID

	r := newAdminUsersRouter(srv)

	tests := []struct {
		name      string
		query     string
		wantTotal int
	}{
		{"no_filter_returns_all", "", 3},
		{"is_active_true", "?is_active=true", 2},
		{"is_active_false", "?is_active=false", 1},
		{"is_active_1", "?is_active=1", 2},
		{"is_active_0", "?is_active=0", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/admin/users"+tc.query, nil)
			req.Header.Set("Authorization", userTokenFor("root"))
			rec := do(t, r, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
			}
			var resp struct {
				Total int `json:"total"`
			}
			decodeJSON(t, rec, &resp)
			if resp.Total != tc.wantTotal {
				t.Fatalf("%s: want %d users, got %d (body=%q)",
					tc.name, tc.wantTotal, resp.Total, rec.Body.String())
			}
		})
	}
}

func TestUsers_FilterByIsAdmin(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	insertTestUser(t, db, "alice", false)
	insertTestUser(t, db, "bob", false)

	r := newAdminUsersRouter(srv)
	req := httptest.NewRequest("GET", "/api/admin/users?is_admin=true", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var resp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Fatalf("is_admin=true: want 1, got %d", resp.Total)
	}
}

func TestUsers_SortByLastLogin(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)
	bobID := insertTestUser(t, db, "bob", false)

	// Alice logs in first, then Bob.
	if err := s_UpdateLastLogin(srv, aliceID); err != nil {
		t.Fatalf("alice login: %v", err)
	}
	// Force a different timestamp for bob.
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET last_login_at = datetime('now', '+1 hour') WHERE id = ?`, bobID); err != nil {
		t.Fatalf("bob login: %v", err)
	}

	r := newAdminUsersRouter(srv)
	req := httptest.NewRequest("GET", "/api/admin/users?sort=last_login_at&order=desc", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var resp struct {
		Users []struct {
			Username    string  `json:"username"`
			LastLoginAt *string `json:"last_login_at"`
		} `json:"users"`
	}
	decodeJSON(t, rec, &resp)
	if len(resp.Users) < 2 {
		t.Fatalf("expected at least 2 users, got %d", len(resp.Users))
	}
	// The user with the most recent last_login_at should be bob.
	if resp.Users[0].Username != "bob" {
		t.Fatalf("sort=last_login_at&order=desc: first user should be bob, got %s",
			resp.Users[0].Username)
	}
}

// newAuditExportRouter mounts the CSV export endpoint.
func newAuditExportRouter(srv *server) http.Handler {
	return newEndpointRouter(srv, "/admin/audit-log/export", srv.handleExportAuditLog)
}

// newEndpointRouter mounts a single admin-protected endpoint. Used by
// tests that need to add a new route to the existing admin stack.
func newEndpointRouter(srv *server, path string, h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Use(srv.tokenAuthMiddlewareChi)
	r.Use(srv.adminMiddlewareChi)
	r.Get(path, h)
	return r
}
