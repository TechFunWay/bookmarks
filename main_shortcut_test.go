package main

// 桌面快捷方式（添加到桌面）相关测试：
//   1. 页面图标优选：apple-touch-icon 优先、按 sizes 面积取最大、跳过 mask-icon。
//   2. 自定义图标上传接口：合法 PNG 落盘返回 /icons/ 路径，非图片拒绝。
//   3. /go/{id} 跳转页：带 apple-touch-icon 与 manifest，标题转义，非书签/非
//      http(s) 地址 404。
//   4. /go/{id}/manifest.webmanifest：名称、start_url 与图标指向正确。
//   5. 修改书签网址时，本地保存的自定义图标不被自动抓取覆盖。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPickBestIconHref_PrefersAppleTouch(t *testing.T) {
	got := pickBestIconHref([]linkIconCandidate{
		{rel: "icon", href: "/big.png", sizes: "512x512"},
		{rel: "apple-touch-icon", href: "/touch.png", sizes: "180x180"},
	})
	if got != "/touch.png" {
		t.Fatalf("apple-touch-icon should win, got %q", got)
	}
}

func TestPickBestIconHref_LargestSizes(t *testing.T) {
	got := pickBestIconHref([]linkIconCandidate{
		{rel: "shortcut icon", href: "/small.ico", sizes: "16x16"},
		{rel: "icon", href: "/large.png", sizes: "32x32 196x196"},
	})
	if got != "/large.png" {
		t.Fatalf("largest sizes should win, got %q", got)
	}
}

func TestPickBestIconHref_SkipsMaskIcon(t *testing.T) {
	got := pickBestIconHref([]linkIconCandidate{
		{rel: "mask-icon", href: "/mask.svg", sizes: "any"},
		{rel: "icon", href: "/favicon.ico"},
	})
	if got != "/favicon.ico" {
		t.Fatalf("mask-icon must be skipped, got %q", got)
	}
}

func TestPickBestIconHref_NoSizes(t *testing.T) {
	got := pickBestIconHref([]linkIconCandidate{{rel: "icon", href: "/favicon.ico"}})
	if got != "/favicon.ico" {
		t.Fatalf("plain favicon should be returned, got %q", got)
	}
}

func TestIconDeclaredArea_ScalableAny(t *testing.T) {
	if area := iconDeclaredArea("any", false); area != 512*512 {
		t.Fatalf("sizes=any should score high, got %d", area)
	}
	if area := iconDeclaredArea("", true); area != 180*180 {
		t.Fatalf("apple-touch default should be 180x180, got %d", area)
	}
}

func TestHandleIconUpload_SavesPNG(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.iconPath = t.TempDir()

	const png = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	rec := httptest.NewRecorder()
	srv.handleIconUpload(rec, jsonRequest(t, "POST", "/api/icons/upload", map[string]string{"data": png}))

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload should return 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(out.URL, "/icons/") {
		t.Fatalf("icon path should start with /icons/, got %q", out.URL)
	}
	if !strings.HasSuffix(out.URL, ".png") {
		t.Fatalf("icon path should end with .png, got %q", out.URL)
	}
}

func TestHandleIconUpload_RejectsNonImage(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.iconPath = t.TempDir()

	rec := httptest.NewRecorder()
	srv.handleIconUpload(rec, jsonRequest(t, "POST", "/api/icons/upload", map[string]string{"data": "data:text/html;base64,PGI+"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-image data should be rejected, got %d", rec.Code)
	}
}

func newGoTestRouter(srv *server) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/go/{id}", srv.handleGoJump)
	r.Get("/go/{id}/manifest.webmanifest", srv.handleGoManifest)
	return r
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestHandleGoJump_RendersShortcutPage(t *testing.T) {
	srv, db := newTestServer(t)
	res, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url, favicon_url) VALUES (1, 'bookmark', '控制台 <管理>', 'https://demo.example.com/panel?a=1', '/icons/20261002/abc.png')`)
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	id, _ := res.LastInsertId()

	r := newGoTestRouter(srv)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/go/"+itoa(id), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("jump page should return 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<link rel="apple-touch-icon" sizes="180x180" href="/icons/20261002/abc.png">`,
		`<link rel="icon" href="/icons/20261002/abc.png">`,
		`<link rel="manifest" href="/go/` + itoa(id) + `/manifest.webmanifest">`,
		`控制台 &lt;管理&gt;`,
		`var target = "https://demo.example.com/panel?a=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("jump page missing %q\npage:\n%s", want, body)
		}
	}
}

func TestHandleGoJump_NotFoundForFolderOrNonHTTP(t *testing.T) {
	srv, db := newTestServer(t)
	if _, err := db.Exec(`INSERT INTO nodes (user_id, type, title) VALUES (1, 'folder', '文件夹')`); err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url) VALUES (1, 'bookmark', '内网', 'ftp://files.example.com')`); err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}

	r := newGoTestRouter(srv)
	for _, path := range []string{"/go/1", "/go/2", "/go/999"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s should 404, got %d", path, rec.Code)
		}
	}
}

func TestHandleGoManifest_PointsToJumpPage(t *testing.T) {
	srv, db := newTestServer(t)
	res, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url, favicon_url) VALUES (1, 'bookmark', '示例站点', 'https://demo.example.com', '/icons/20261002/abc.png')`)
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	id, _ := res.LastInsertId()

	r := newGoTestRouter(srv)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/go/"+itoa(id)+"/manifest.webmanifest", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("manifest should return 200, got %d", rec.Code)
	}
	var m struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		StartURL  string `json:"start_url"`
		Scope     string `json:"scope"`
		Display   string `json:"display"`
		Icons     []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if m.Name != "示例站点" || m.ShortName != "示例站点" {
		t.Fatalf("unexpected names: %q / %q", m.Name, m.ShortName)
	}
	if m.StartURL != "/go/"+itoa(id) {
		t.Fatalf("start_url should be the jump page, got %q", m.StartURL)
	}
	if m.Scope != "/" || m.Display != "standalone" {
		t.Fatalf("unexpected scope/display: %q / %q", m.Scope, m.Display)
	}
	if len(m.Icons) != 1 || m.Icons[0].Src != "/icons/20261002/abc.png" || m.Icons[0].Sizes != "512x512" {
		t.Fatalf("unexpected icons: %+v", m.Icons)
	}
}

func TestHandleGoManifest_FallsBackToAppIcon(t *testing.T) {
	srv, db := newTestServer(t)
	res, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url, favicon_url) VALUES (1, 'bookmark', '无图站点', 'https://plain.example.com', '⭐')`)
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	id, _ := res.LastInsertId()

	r := newGoTestRouter(srv)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/go/"+itoa(id)+"/manifest.webmanifest", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("manifest should return 200, got %d", rec.Code)
	}
	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Icons) != 1 || m.Icons[0].Src != "/app-icons/icon-512.png" {
		t.Fatalf("emoji favicon should fall back to app icon, got %+v", m.Icons)
	}
}

func TestGoBasePrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "/go/1", nil)
	if got := goBasePrefix(req); got != "" {
		t.Fatalf("plain request should have empty base, got %q", got)
	}

	req = httptest.NewRequest("GET", "/app/techfunway-bookmarks/go/1", nil)
	if got := goBasePrefix(req); got != "/app/techfunway-bookmarks" {
		t.Fatalf("prefixed path should reveal base, got %q", got)
	}

	ctxReq := httptest.NewRequest("GET", "/go/1", nil).WithContext(
		context.WithValue(context.Background(), fnOSGatewayPrefixContextKey{}, "/app/techfunway-bookmarks/"),
	)
	if got := goBasePrefix(ctxReq); got != "/app/techfunway-bookmarks" {
		t.Fatalf("gateway context prefix should be used, got %q", got)
	}
}

func TestUpdateNode_KeepsLocalCustomIconOnURLChange(t *testing.T) {
	srv, db := newTestServer(t)

	// 本地元数据服务器：提供 apple-touch-icon，模拟改网址后的自动抓取
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><link rel="apple-touch-icon" href="/touch.png"><title>新站</title></head><body></body></html>`))
	}))
	defer meta.Close()

	res, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url, favicon_url) VALUES (1, 'bookmark', '旧站', 'https://old.example.com', '/icons/20261002/custom.png')`)
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	id, _ := res.LastInsertId()

	newURL := meta.URL + "/page"
	req := updateNodeRequest{URL: &newURL}
	if err := srv.updateNode(context.Background(), 1, id, req); err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	var favicon string
	if err := db.QueryRow(`SELECT favicon_url FROM nodes WHERE id = ?`, id).Scan(&favicon); err != nil {
		t.Fatalf("read favicon: %v", err)
	}
	if favicon != "/icons/20261002/custom.png" {
		t.Fatalf("local custom icon must be kept, got %q", favicon)
	}
}

func TestUpdateNode_RefreshesRemoteIconOnURLChange(t *testing.T) {
	srv, db := newTestServer(t)

	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><link rel="apple-touch-icon" href="/touch.png"><title>新站</title></head><body></body></html>`))
	}))
	defer meta.Close()

	res, err := db.Exec(`INSERT INTO nodes (user_id, type, title, url, favicon_url) VALUES (1, 'bookmark', '旧站', 'https://old.example.com', 'https://old.example.com/favicon.ico')`)
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	id, _ := res.LastInsertId()

	newURL := meta.URL + "/page"
	req := updateNodeRequest{URL: &newURL}
	if err := srv.updateNode(context.Background(), 1, id, req); err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	var favicon string
	if err := db.QueryRow(`SELECT favicon_url FROM nodes WHERE id = ?`, id).Scan(&favicon); err != nil {
		t.Fatalf("read favicon: %v", err)
	}
	if favicon != meta.URL+"/touch.png" {
		t.Fatalf("remote icon should refresh to %q, got %q", meta.URL+"/touch.png", favicon)
	}
}

// ============ 应用自定义图标（PWA / 桌面快捷方式） ============

func newAppIconRouter(srv *server) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		srv.serveAppIcon(w, req, "favicon.ico")
	})
	r.Get("/app-icons/{name}", func(w http.ResponseWriter, req *http.Request) {
		srv.serveAppIcon(w, req, chi.URLParam(req, "name"))
	})
	r.Get("/manifest.webmanifest", srv.handleAppManifest)
	return r
}

func TestServeAppIcon_FallsBackToStatic(t *testing.T) {
	srv, _ := newTestServer(t)
	r := newAppIconRouter(srv)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/app-icons/icon-192.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("default icon should be served, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type should be image/png, got %q", ct)
	}
	want, _ := staticFS.ReadFile("static/app-icons/icon-192.png")
	if string(rec.Body.Bytes()) != string(want) {
		t.Fatalf("fallback icon should match embedded default")
	}
}

func TestServeAppIcon_CustomOverridesAll(t *testing.T) {
	srv, _ := newTestServer(t)
	custom := []byte("custom-app-icon-png-bytes")
	srv.appIconPath = filepath.Join(t.TempDir(), "appicon.png")
	if err := os.WriteFile(srv.appIconPath, custom, 0644); err != nil {
		t.Fatalf("write custom icon: %v", err)
	}
	r := newAppIconRouter(srv)

	for _, path := range []string{"/app-icons/icon-180.png", "/app-icons/icon-192.png", "/app-icons/icon-512.png", "/favicon.ico"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s should serve custom icon, got %d", path, rec.Code)
		}
		if string(rec.Body.Bytes()) != string(custom) {
			t.Fatalf("%s should return custom icon bytes", path)
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s should be no-cache", path)
		}
	}
}

func TestServeAppIcon_RejectsUnknownName(t *testing.T) {
	srv, _ := newTestServer(t)
	r := newAppIconRouter(srv)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/app-icons/evil.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown icon name should 404, got %d", rec.Code)
	}
}

func TestHandleAppManifest_IconsFollowGatewayPrefix(t *testing.T) {
	srv, _ := newTestServer(t)
	r := newAppIconRouter(srv)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/manifest.webmanifest", nil))
	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Icons) != 2 || m.Icons[0].Src != "/app-icons/icon-192.png" {
		t.Fatalf("plain request should have unprefixed icons, got %+v", m.Icons)
	}

	ctxReq := httptest.NewRequest("GET", "/manifest.webmanifest", nil).WithContext(
		context.WithValue(context.Background(), fnOSGatewayPrefixContextKey{}, "/app/techfunway-bookmarks"),
	)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, ctxReq)
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Icons) != 2 || m.Icons[0].Src != "/app/techfunway-bookmarks/app-icons/icon-192.png" {
		t.Fatalf("gateway request should prefix icon paths, got %+v", m.Icons)
	}
}

func TestHandleAppIconUpload_AdminOnly(t *testing.T) {
	srv, db := newTestServer(t)
	srv.appIconPath = filepath.Join(t.TempDir(), "appicon.png")
	insertTestUser(t, db, "alice", false)
	insertTestUser(t, db, "root", true)

	r := chi.NewRouter()
	r.Post("/api/appicon", srv.optionalAuthMiddleware(srv.handleAppIconUpload))

	body := func(token string) *http.Request {
		req := httptest.NewRequest("POST", "/api/appicon", strings.NewReader(
			`{"data":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="}`))
		req.Header.Set("Authorization", token)
		return req
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, body(userTokenFor("alice")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin should be rejected, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, body(userTokenFor("root")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin upload should succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(srv.appIconPath); err != nil {
		t.Fatalf("custom icon file should exist: %v", err)
	}

	// 系统配置应带出 app_icon_custom 标记（测试夹具无 sys_config 表，补一个最小表）
	mustExec(t, db, `CREATE TABLE sys_config (
		user_id INTEGER NOT NULL DEFAULT 0,
		key TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, key)
	)`)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/config/system", nil)
	srv.handleGetSystemConfig(rec, req)
	if !strings.Contains(rec.Body.String(), `"app_icon_custom":"true"`) {
		t.Fatalf("system config should flag custom icon, got %s", rec.Body.String())
	}
}

func TestHandleAppIconReset_RemovesCustomIcon(t *testing.T) {
	srv, db := newTestServer(t)
	srv.appIconPath = filepath.Join(t.TempDir(), "appicon.png")
	if err := os.WriteFile(srv.appIconPath, []byte("x"), 0644); err != nil {
		t.Fatalf("seed icon: %v", err)
	}
	insertTestUser(t, db, "root", true)

	r := chi.NewRouter()
	r.Delete("/api/appicon", srv.optionalAuthMiddleware(srv.handleAppIconReset))
	req := httptest.NewRequest("DELETE", "/api/appicon", nil)
	req.Header.Set("Authorization", userTokenFor("root"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset should succeed, got %d", rec.Code)
	}
	if _, err := os.Stat(srv.appIconPath); !os.IsNotExist(err) {
		t.Fatalf("custom icon should be removed, err=%v", err)
	}
}
