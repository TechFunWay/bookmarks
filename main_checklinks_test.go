package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestClassifyLinkResult(t *testing.T) {
	cases := []struct {
		name        string
		code        int
		err         error
		wantCat     string
		wantErrType string
	}{
		{"timeout", 0, context.DeadlineExceeded, "suspicious", "timeout"},
		{"dns", 0, &net.DNSError{Err: "no such host", Name: "x.test"}, "suspicious", "dns"},
		{"connection refused", 0, &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}, "suspicious", "connection"},
		{"tls", 0, errors.New("tls: handshake failure"), "suspicious", "tls"},
		{"200 ok", 200, nil, "ok", ""},
		{"301 ok", 301, nil, "ok", ""},
		{"404 dead", 404, nil, "dead", ""},
		{"410 dead", 410, nil, "dead", ""},
		{"403 suspicious", 403, nil, "suspicious", ""},
		{"429 suspicious", 429, nil, "suspicious", ""},
		{"500 suspicious", 500, nil, "suspicious", ""},
		{"504 suspicious", 504, nil, "suspicious", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cat, _, et := classifyLinkResult(c.code, c.err)
			if cat != c.wantCat || et != c.wantErrType {
				t.Fatalf("classifyLinkResult(%d,%v) = cat=%q et=%q, want cat=%q et=%q",
					c.code, c.err, cat, et, c.wantCat, c.wantErrType)
			}
		})
	}
}

func TestTransportErrorsAreSuspicious(t *testing.T) {
	errs := []error{
		context.DeadlineExceeded,
		&net.DNSError{Err: "no such host", Name: "x"},
		errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
		errors.New("tls: failed to verify certificate"),
	}
	for _, e := range errs {
		cat, _, _ := classifyLinkResult(0, e)
		if cat == "dead" {
			t.Fatalf("transport error %v classified as dead", e)
		}
		if cat != "suspicious" {
			t.Fatalf("transport error %v: got %q want suspicious", e, cat)
		}
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

	if _, cat, _, _ := srv.checkURL(context.Background(), okSrv.URL); cat != "ok" {
		t.Fatalf("ok server: got %q want ok", cat)
	}
	if _, cat, _, _ := srv.checkURL(context.Background(), notFoundSrv.URL); cat != "dead" {
		t.Fatalf("404 server: got %q want dead", cat)
	}
	if code, cat, _, _ := srv.checkURL(context.Background(), headBlockedSrv.URL); cat != "ok" || code != 200 {
		t.Fatalf("head-blocked server: got code=%d cat=%q want 200/ok (GET only)", code, cat)
	}
	if code, cat, _, _ := srv.checkURL(context.Background(), headNotFoundSrv.URL); cat != "ok" || code != 200 {
		t.Fatalf("head-404/get-200 server: got code=%d cat=%q want 200/ok (GET only)", code, cat)
	}
	if _, cat, _, et := srv.checkURL(context.Background(), "http://127.0.0.1:1/nope"); cat != "suspicious" {
		t.Fatalf("connection refused: got %q want suspicious", cat)
	} else if et != "connection" {
		t.Fatalf("connection refused error_type: got %q want connection", et)
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
				ID        int64  `json:"id"`
				Category  string `json:"category"`
				ErrorType string `json:"error_type"`
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

func TestNormalizeURLKey(t *testing.T) {
	tests := []struct {
		name string
		opts dupeKeyOptions
		raw  string
		want string
	}{
		{"no opts", dupeKeyOptions{}, "https://www.x.com/path?a=1", "https://www.x.com/path?a=1"},
		{"ignore scheme", dupeKeyOptions{ignoreScheme: true}, "https://x.com/", "x.com/"},
		{"ignore scheme http", dupeKeyOptions{ignoreScheme: true}, "http://x.com/", "x.com/"},
		{"ignore www", dupeKeyOptions{ignoreWWW: true}, "https://www.x.com/", "https://x.com/"},
		{"ignore www+scheme", dupeKeyOptions{ignoreScheme: true, ignoreWWW: true}, "https://www.x.com/", "x.com/"},
		{"ignore trailing slash", dupeKeyOptions{ignoreTrailingSlash: true}, "https://x.com/a/", "https://x.com/a"},
		{"ignore query", dupeKeyOptions{ignoreQuery: true}, "https://x.com/a?b=c", "https://x.com/a"},
		{"all combined", dupeKeyOptions{ignoreScheme: true, ignoreWWW: true, ignoreTrailingSlash: true, ignoreQuery: true},
			"https://www.x.com/path/?a=1", "x.com/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURLKey(tt.raw, tt.opts)
			if got != tt.want {
				t.Fatalf("normalizeURLKey(%q, %+v) = %q, want %q", tt.raw, tt.opts, got, tt.want)
			}
		})
	}
}
