package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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

	// 只支持 GET、对 HEAD 返回 405 —— 验证 HEAD 失败回退 GET。
	headBlockedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(405)
			return
		}
		w.WriteHeader(200)
	}))
	defer headBlockedSrv.Close()

	if _, cat, _ := srv.checkURL(context.Background(), okSrv.URL); cat != "ok" {
		t.Fatalf("ok server: got %q want ok", cat)
	}
	if _, cat, _ := srv.checkURL(context.Background(), notFoundSrv.URL); cat != "dead" {
		t.Fatalf("404 server: got %q want dead", cat)
	}
	if code, cat, _ := srv.checkURL(context.Background(), headBlockedSrv.URL); cat != "ok" || code != 200 {
		t.Fatalf("head-blocked server: got code=%d cat=%q want 200/ok (GET fallback)", code, cat)
	}
	if _, cat, _ := srv.checkURL(context.Background(), "http://127.0.0.1:1/nope"); cat != "dead" {
		t.Fatalf("connection refused: got %q want dead", cat)
	}
}
