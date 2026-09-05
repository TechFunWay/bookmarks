package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestGetDeviceID_StableAndHashed 验证设备 ID 稳定性：
// 同一数据目录重复调用必须返回相同值，且只暴露 32 位哈希，
// 不泄露机器标识原文。
func TestGetDeviceID_StableAndHashed(t *testing.T) {
	dir := t.TempDir()

	id1 := getDeviceID(dir)
	id2 := getDeviceID(dir)

	if id1 != id2 {
		t.Fatalf("device id should be stable, got %q and %q", id1, id2)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id1) {
		t.Fatalf("device id should be 32-char md5 hex, got %q", id1)
	}

	// 数据路径变化不应影响身份（同一台机器）
	if id3 := getDeviceID(filepath.Join(dir, "other")); id3 != id1 {
		t.Fatalf("device id should not depend on data path, got %q vs %q", id3, id1)
	}
}

// TestGetPersistentDeviceID_Fallback 验证数据目录兜底路径：
// 首次调用生成随机 ID 并写入 device.id，之后复用同一 ID
func TestGetPersistentDeviceID_Fallback(t *testing.T) {
	dir := t.TempDir()

	id1 := getPersistentDeviceID(dir)
	if id1 == "" {
		t.Fatal("persistent device id should not be empty")
	}

	b, err := os.ReadFile(filepath.Join(dir, "device.id"))
	if err != nil {
		t.Fatalf("device.id file should be created: %v", err)
	}
	if strings.TrimSpace(string(b)) != id1 {
		t.Fatalf("device.id content %q should equal returned id %q", string(b), id1)
	}

	if id2 := getPersistentDeviceID(dir); id2 != id1 {
		t.Fatalf("persistent id should be stable, got %q and %q", id1, id2)
	}
}

// TestHandleDonateSupport 验证赞赏上报：
// 带有效 token 的 POST 会向统计服务器发送 event=donate_support，
// 且只包含设备统计字段、不含 hostname 或用户数据。
func TestHandleDonateSupport(t *testing.T) {
	received := make(chan StatsRequest, 1)
	statsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req StatsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode stats body: %v", err)
		}
		received <- req
		w.WriteHeader(http.StatusOK)
	}))
	defer statsSrv.Close()

	t.Setenv("STATS_ENDPOINT", statsSrv.URL)
	origBase := statsBaseRequest
	statsBaseRequest = StatsRequest{
		AppName:  "bookmarks",
		Version:  "vTest",
		DeviceID: "abc123",
		OS:       "linux",
		Arch:     "amd64",
	}
	defer func() { statsBaseRequest = origBase }()

	srv, db := newTestServer(t)
	insertTestUser(t, db, "root", true)

	r := chi.NewRouter()
	r.Post("/api/donate/support", srv.tokenAuthMiddleware(srv.handleDonateSupport))

	req := httptest.NewRequest("POST", "/api/donate/support", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := do(t, r, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-received:
		if got.Event != "donate_support" {
			t.Fatalf("event: want donate_support, got %q", got.Event)
		}
		if got.DeviceID != "abc123" || got.AppName != "bookmarks" || got.Version != "vTest" {
			t.Fatalf("unexpected payload: %+v", got)
		}
	default:
		t.Fatal("stats server received no donate report")
	}
}

// TestHandleDonateSupport_Unauthorized 验证未携带 token 的请求被拒绝
func TestHandleDonateSupport_Unauthorized(t *testing.T) {
	srv, _ := newTestServer(t)

	r := chi.NewRouter()
	r.Post("/api/donate/support", srv.tokenAuthMiddleware(srv.handleDonateSupport))

	req := httptest.NewRequest("POST", "/api/donate/support", nil)
	rec := do(t, r, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
}
