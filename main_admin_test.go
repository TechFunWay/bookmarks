package main

// Regression tests for admin create endpoints
// (handleAdminCreateFolder, handleAdminCreateBookmark, handleAdminReorderNodes).
//
// Bug B7/B8/B9: these handlers accept any int64 in `user_id` and create
// nodes / reorder children for a non-existent user. The result is orphan
// rows in the nodes table (or a silent no-op for reorder) and an audit
// log entry that points to a user nobody can identify. Validating the
// target user up front makes the failure visible and the audit trail
// accurate.

import (
	"context"
	"net/http"
	"testing"
)

// TestHandleAdminCreateFolder_UnknownUser_Returns404 verifies that
// creating a folder for a non-existent user fails with 404.
//
// RED (before fix): the handler inserts a row with the supplied user_id
// even when the user does not exist. The test asserts the row count
// does not change and the response is 404.
func TestHandleAdminCreateFolder_UnknownUser_Returns404(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)

	r := newAdminRouter(srv)
	body := map[string]any{
		"user_id": int64(999999),
		"title":   "ghost folder",
	}
	req := jsonRequest(t, "POST", "/admin/folders", body)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown user_id, got %d (body=%q)", rec.Code, rec.Body.String())
	}

	var nodeCount int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM nodes WHERE user_id = 999999`).Scan(&nodeCount); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount != 0 {
		t.Fatalf("handler should not insert rows for unknown user; nodes created: %d", nodeCount)
	}
	_ = adminID
}

func TestHandleAdminCreateFolder_ValidUser_Succeeds(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)

	r := newAdminRouter(srv)
	body := map[string]any{"user_id": aliceID, "title": "alice folder"}
	req := jsonRequest(t, "POST", "/admin/folders", body)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("valid user_id: want 201, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestHandleAdminCreateBookmark_UnknownUser_Returns404 mirrors the
// folder case for the bookmark handler.
func TestHandleAdminCreateBookmark_UnknownUser_Returns404(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)

	r := newAdminRouter(srv)
	body := map[string]any{
		"user_id": int64(999999),
		"url":     "https://example.com",
		"title":   "ghost bookmark",
	}
	req := jsonRequest(t, "POST", "/admin/bookmarks", body)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown user_id, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestHandleAdminReorderNodes_UnknownUser_Returns404 verifies reorder
// also rejects unknown user ids.
func TestHandleAdminReorderNodes_UnknownUser_Returns404(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)

	r := newAdminRouter(srv)
	body := map[string]any{
		"user_id":     int64(999999),
		"parent_id":   nil,
		"ordered_ids": []int64{1, 2, 3},
	}
	req := jsonRequest(t, "PUT", "/admin/nodes/reorder", body)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown user_id, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}
