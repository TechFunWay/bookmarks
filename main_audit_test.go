package main

// Regression tests for handleGetAuditLog (main.go:3560).
//
// Bug B4: when a caller passed `?user_id=<digits>`, the value was bound
// to the SQL query as a string. The audit_log.user_id column is INTEGER
// (declared in v2.3.0 upgrade, main.go sys_update.go). SQLite will coerce
// the value, so the query still returns the right rows — but the wrong
// type leaks through to anything that inspects the bind value (and is
// fragile against future stricter type checks). The handler also silently
// ignored a non-numeric `user_id` such as "?user_id=abc" or empty input.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
)

// seedAuditLog inserts a single audit row and returns its id.
func seedAuditLog(t *testing.T, srv *server, userID int64, action string) int64 {
	t.Helper()
	res, err := srv.db.ExecContext(context.Background(),
		`INSERT INTO audit_log (user_id, action) VALUES (?, ?)`, userID, action)
	if err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// TestHandleGetAuditLog_FilterByUserID verifies that the user_id query
// parameter is parsed as an integer and filters the result set.
//
// RED (before fix): the user_id is bound as a string, so requests with
// invalid user_id ("abc", "0") produce incorrect results.
func TestHandleGetAuditLog_FilterByUserID(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)
	bobID := insertTestUser(t, db, "bob", false)

	seedAuditLog(t, srv, aliceID, "user_update")
	seedAuditLog(t, srv, aliceID, "user_delete")
	seedAuditLog(t, srv, bobID, "login")
	seedAuditLog(t, srv, adminID, "user_create")

	r := newAdminRouter(srv)
	adminToken := userTokenFor("root")

	tests := []struct {
		name       string
		userID     string
		wantCount  int
		wantAction string
	}{
		{name: "filters_by_alice", userID: strconv.FormatInt(aliceID, 10), wantCount: 2, wantAction: "user_delete"},
		{name: "filters_by_bob", userID: strconv.FormatInt(bobID, 10), wantCount: 1, wantAction: "login"},
		{name: "no_filter_returns_all", userID: "", wantCount: 4},
		{name: "unknown_user_returns_empty", userID: "999999", wantCount: 0},
		{name: "invalid_user_id_returns_400", userID: "abc", wantCount: -1},
		{name: "negative_user_id_returns_400", userID: "-1", wantCount: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/admin/audit-log"
			if tc.userID != "" {
				path += "?user_id=" + tc.userID
			}
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", adminToken)
			rec := do(t, r, req)

			if tc.wantCount == -1 {
				if rec.Code == http.StatusOK {
					t.Fatalf("want 400 for invalid user_id %q, got 200 (body=%q)",
						tc.userID, rec.Body.String())
				}
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("want 400 for invalid user_id %q, got %d", tc.userID, rec.Code)
				}
				return
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
			}
			var resp struct {
				Logs  []map[string]any `json:"logs"`
				Total int64             `json:"total"`
			}
			decodeJSON(t, rec, &resp)
			if int(resp.Total) != tc.wantCount {
				t.Fatalf("user_id=%q: want %d logs, got %d (body=%q)",
					tc.userID, tc.wantCount, resp.Total, rec.Body.String())
			}
			if len(resp.Logs) != tc.wantCount {
				t.Fatalf("user_id=%q: want %d log rows, got %d", tc.userID, tc.wantCount, len(resp.Logs))
			}
		})
	}
}

// _ = chi.NewRouter keeps the chi import referenced in case future
// sub-tests need a custom router; remove if unused.
var _ = chi.NewRouter
