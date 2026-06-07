package main

// Regression tests for the admin middleware (main.go:4222).
//
// Bug B1: adminMiddleware queried `SELECT is_admin FROM users WHERE id = ?`
// and returned 500 Internal Server Error when the user id referred to a
// row that did not exist (sql.ErrNoRows). For a missing user the correct
// response is 401 Unauthorized or 403 Forbidden — never 500. This test
// pins the expected behavior so future refactors do not regress it.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAdminMiddleware_MissingUser_Returns401Or403 verifies that a context
// carrying a user id with no matching users row does NOT produce 500.
//
// RED (before fix): the handler returns 500 because QueryRow returns
// sql.ErrNoRows and the code responds with StatusInternalServerError.
// GREEN (after fix): the handler returns 401 or 403.
func TestAdminMiddleware_MissingUser_Returns401Or403(t *testing.T) {
	srv, db := newTestServer(t)
	_ = db

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := srv.adminMiddleware(next)

	req := httptest.NewRequest("GET", "/admin/anything", nil)
	ctx := withUserID(req.Context(), 999999)
	// 999999 is a user id that definitely does not exist in the fresh DB.
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	mw(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("adminMiddleware leaked 500 for missing user; want 401/403, got %d (body=%q)",
			rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("adminMiddleware for missing user: want 401/403, got %d (body=%q)",
			rec.Code, rec.Body.String())
	}
	if called {
		t.Fatalf("adminMiddleware should not invoke next handler for missing user")
	}
}

// TestAdminMiddleware_NonAdminUser_Returns403 verifies that a valid but
// non-admin user is rejected with 403, not 500.
func TestAdminMiddleware_NonAdminUser_Returns403(t *testing.T) {
	srv, db := newTestServer(t)
	nonAdminID := insertTestUser(t, db, "alice", false)

	mw := srv.adminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/admin/anything", nil)
	req = req.WithContext(withUserID(req.Context(), nonAdminID))

	rec := httptest.NewRecorder()
	mw(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin user: want 403, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestAdminMiddleware_AdminUser_PassesThrough verifies that an admin user
// is allowed through to the wrapped handler. This protects against
// accidentally over-tightening the fix.
func TestAdminMiddleware_AdminUser_PassesThrough(t *testing.T) {
	srv, db := newTestServer(t)
	adminID := insertTestUser(t, db, "root", true)

	called := false
	mw := srv.adminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/admin/anything", nil)
	req = req.WithContext(withUserID(req.Context(), adminID))

	rec := httptest.NewRecorder()
	mw(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("admin user: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatalf("admin user should reach the wrapped handler")
	}
}
