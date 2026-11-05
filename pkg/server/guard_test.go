package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

const guardToken = "guard-test-token"

func startGuarded(t *testing.T) string {
	t.Helper()
	srv := NewServer(ServerConfig{Host: "127.0.0.1", AuthToken: guardToken}, nil, nil)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv.Addr()
}

func guardedStatus(t *testing.T, method, url, body string, headers map[string]string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header
}

func TestGuard_RejectsCrossOriginRebindingAndBadContentType(t *testing.T) {
	addr := startGuarded(t)
	base := "http://" + addr
	auth := map[string]string{"Authorization": "Bearer " + guardToken}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range auth {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name, method, path, body string
		headers                  map[string]string
		want                     int
	}{
		{"cross-origin page with the token", http.MethodPost, "/api/v1/rag/retrieve", `{}`, with(map[string]string{"Origin": "https://evil.example", "Content-Type": "application/json"}), http.StatusForbidden},
		{"cross-origin preflight", http.MethodOptions, "/api/v1/rag/retrieve", "", map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "POST"}, http.StatusForbidden},
		{"dns rebinding host", http.MethodGet, "/api/v1/health", "", with(map[string]string{"Host": "attacker.example:3456"}), http.StatusForbidden},
		{"same origin without a token", http.MethodGet, "/api/v1/health", "", map[string]string{"Origin": base}, http.StatusUnauthorized},
		{"text/plain body", http.MethodPost, "/api/v1/workspaces/switch", `{}`, with(map[string]string{"Content-Type": "text/plain"}), http.StatusUnsupportedMediaType},
		{"json body reaches the handler (no registry: 500)", http.MethodPost, "/api/v1/workspaces/switch", `{}`, with(map[string]string{"Content-Type": "application/json"}), http.StatusInternalServerError},
		{"same origin with the token", http.MethodGet, "/api/v1/health", "", with(map[string]string{"Origin": base}), http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, hdr := guardedStatus(t, c.method, base+c.path, c.body, c.headers)
			if got != c.want {
				t.Fatalf("%s %s: status %d, want %d", c.method, c.path, got, c.want)
			}
			if hdr.Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("no CORS header may be sent")
			}
		})
	}
}

func TestGuard_DashboardTokenBecomesSessionCookie(t *testing.T) {
	base := "http://" + startGuarded(t)
	status, hdr := guardedStatus(t, http.MethodGet, base+"/?token="+guardToken, "", nil)
	cookie := hdr.Get("Set-Cookie")
	for _, want := range []string{sessionCookie + "=" + guardToken, "HttpOnly", "SameSite=Strict"} {
		if !strings.Contains(cookie, want) {
			t.Fatalf("cookie %q lacks %q", cookie, want)
		}
	}
	if status != http.StatusOK {
		t.Fatalf("dashboard status %d", status)
	}
	status, _ = guardedStatus(t, http.MethodGet, base+"/api/v1/health", "", map[string]string{"Cookie": sessionCookie + "=" + guardToken})
	if status != http.StatusOK {
		t.Fatalf("cookie must authorise, got %d", status)
	}
}

func TestGenerateToken_IsRandom(t *testing.T) {
	a, err := GenerateToken()
	b, _ := GenerateToken()
	if err != nil || len(a) != 64 || a == b {
		t.Fatalf("token %q / %q, err %v", a, b, err)
	}
}
