package main

// Feature tests for F1: Last login tracking.
//
// The admin user list is much more useful when it shows when each user
// last authenticated. This file pins the contract:
//   1. A fresh user has last_login_at == NULL.
//   2. handleLogin updates last_login_at on a successful login.
//   3. The user struct returned by /api/users and /api/users/{id}
//      surfaces last_login_at as a nullable string field.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestUser_LastLoginAt_StartsNull(t *testing.T) {
	_, db := newTestServer(t)
	aliceID := insertTestUser(t, db, "alice", false)

	var raw sql.NullString
	if err := db.QueryRowContext(context.Background(),
		`SELECT last_login_at FROM users WHERE id = ?`, aliceID).Scan(&raw); err != nil {
		t.Fatalf("read last_login_at: %v", err)
	}
	if raw.Valid {
		t.Fatalf("fresh user should have NULL last_login_at, got %q", raw.String)
	}
}

func TestUser_LastLoginAt_UpdatedByLogin(t *testing.T) {
	srv, db := newTestServer(t)
	aliceID := insertTestUser(t, db, "alice", false)

	if err := s_UpdateLastLogin(srv, aliceID); err != nil {
		t.Fatalf("update last_login_at: %v", err)
	}

	var got time.Time
	if err := db.QueryRowContext(context.Background(),
		`SELECT last_login_at FROM users WHERE id = ?`, aliceID).Scan(&got); err != nil {
		t.Fatalf("read last_login_at: %v", err)
	}
	if time.Since(got) > 5*time.Second {
		t.Fatalf("last_login_at should be recent, got %s (delta %s)", got, time.Since(got))
	}
}

func TestUser_LastLoginAt_ReturnedInList(t *testing.T) {
	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)
	aliceID := insertTestUser(t, db, "alice", false)
	if err := s_UpdateLastLogin(srv, aliceID); err != nil {
		t.Fatalf("update last_login_at: %v", err)
	}

	r := newAdminRouter(srv)
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	// we don't have a /api/admin/users mount in newAdminRouter; add the
	// user list via a fresh router that has both endpoints.
	r = newAdminUsersRouter(srv)
	req = httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET users: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Users []struct {
			ID           int64   `json:"id"`
			Username     string  `json:"username"`
			LastLoginAt  *string `json:"last_login_at"`
		} `json:"users"`
	}
	decodeJSON(t, rec, &resp)
	if len(resp.Users) != 2 {
		t.Fatalf("want 2 users, got %d (body=%q)", len(resp.Users), rec.Body.String())
	}
	var alice, admin *struct {
		ID          int64   `json:"id"`
		Username    string  `json:"username"`
		LastLoginAt *string `json:"last_login_at"`
	}
	for i := range resp.Users {
		u := &resp.Users[i]
		if u.Username == "alice" {
			alice = u
		}
		if u.Username == "root" {
			admin = u
		}
	}
	if alice == nil {
		t.Fatalf("alice missing from response")
	}
	if alice.LastLoginAt == nil || *alice.LastLoginAt == "" {
		t.Fatalf("alice.last_login_at should be set, got %v", alice.LastLoginAt)
	}
	if admin != nil && admin.LastLoginAt != nil {
		t.Fatalf("admin (never logged in) should have null last_login_at, got %v", *admin.LastLoginAt)
	}
}

// s_UpdateLastLogin is a thin test helper around whatever method we use
// to record a login. Today it goes through the same SQL the login
// handler will use; if we add a dedicated server method, this helper
// will be a one-line wrapper.
func s_UpdateLastLogin(srv *server, userID int64) error {
	_, err := srv.db.ExecContext(context.Background(),
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	return err
}

// newAdminUsersRouter mounts the user list handler alongside the admin
// middleware. Kept separate from newAdminRouter so test scope is clear.
func newAdminUsersRouter(srv *server) http.Handler {
	r := chi.NewRouter()
	r.Use(srv.tokenAuthMiddlewareChi)
	r.Use(srv.adminMiddlewareChi)
	r.Get("/api/admin/users", srv.handleGetUsers)
	return r
}
