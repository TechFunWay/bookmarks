package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestClassifyLinkResult(t *testing.T) {
	cases := []struct {
		name string
		code int
		err  error
		want string
	}{
		{"transport error", 0, errors.New("dial tcp: no such host"), "dead"},
		{"200 ok", 200, nil, "ok"},
		{"301 redirect", 301, nil, "ok"},
		{"404 dead", 404, nil, "dead"},
		{"410 dead", 410, nil, "dead"},
		{"403 suspicious", 403, nil, "suspicious"},
		{"429 suspicious", 429, nil, "suspicious"},
		{"500 suspicious", 500, nil, "suspicious"},
		{"503 suspicious", 503, nil, "suspicious"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := classifyLinkResult(c.code, c.err)
			if got != c.want {
				t.Fatalf("classifyLinkResult(%d,%v) = %q, want %q", c.code, c.err, got, c.want)
			}
		})
	}
}

func TestCheckURL(t *testing.T) {
	srv := &server{httpClient: http.DefaultClient}

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer okSrv.Close()

	notFoundSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer notFoundSrv.Close()

	// 对 HEAD 返回 405、对 GET 返回 200 —— 检测应只用 GET，故判为 ok。
	headBlockedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(405)
			return
		}
		w.WriteHeader(200)
	}))
	defer headBlockedSrv.Close()

	// 对 HEAD 返回 404、对 GET 返回 200 —— 真实站点(如 example.com)的行为。
	// 检测只用 GET，绝不能因 HEAD 的 404 就误判为失效。
	headNotFoundSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(200)
	}))
	defer headNotFoundSrv.Close()

	if _, cat, _ := srv.checkURL(context.Background(), okSrv.URL); cat != "ok" {
		t.Fatalf("ok server: got %q want ok", cat)
	}
	if _, cat, _ := srv.checkURL(context.Background(), notFoundSrv.URL); cat != "dead" {
		t.Fatalf("404 server: got %q want dead", cat)
	}
	if code, cat, _ := srv.checkURL(context.Background(), headBlockedSrv.URL); cat != "ok" || code != 200 {
		t.Fatalf("head-blocked server: got code=%d cat=%q want 200/ok (GET only)", code, cat)
	}
	if code, cat, _ := srv.checkURL(context.Background(), headNotFoundSrv.URL); cat != "ok" || code != 200 {
		t.Fatalf("head-404/get-200 server: got code=%d cat=%q want 200/ok (GET only)", code, cat)
	}
	if _, cat, _ := srv.checkURL(context.Background(), "http://127.0.0.1:1/nope"); cat != "dead" {
		t.Fatalf("connection refused: got %q want dead", cat)
	}
}

func TestHandleCheckLinks(t *testing.T) {
	srv := &server{httpClient: http.DefaultClient}

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer okSrv.Close()
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer deadSrv.Close()
	suspSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer suspSrv.Close()

	r := chi.NewRouter()
	r.Post("/api/check-links", srv.handleCheckLinks)

	body := map[string]any{"bookmarks": []map[string]any{
		{"id": 1, "url": okSrv.URL},
		{"id": 2, "url": deadSrv.URL},
		{"id": 3, "url": suspSrv.URL},
	}}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/check-links", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := do(t, r, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Results []struct {
				ID       int64  `json:"id"`
				Category string `json:"category"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	got := map[int64]string{}
	for _, x := range resp.Data.Results {
		got[x.ID] = x.Category
	}
	if got[1] != "ok" || got[2] != "dead" || got[3] != "suspicious" {
		t.Fatalf("categories wrong: %+v", got)
	}
}
