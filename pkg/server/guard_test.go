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

func sessionOf(t *testing.T, hdr http.Header) (name, id string) {
	t.Helper()
	cookie := hdr.Get("Set-Cookie")
	for _, want := range []string{sessionCookie + "_", "HttpOnly", "SameSite=Strict", "Max-Age="} {
		if !strings.Contains(cookie, want) {
			t.Fatalf("cookie %q lacks %q", cookie, want)
		}
	}
	kv := strings.SplitN(strings.SplitN(cookie, ";", 2)[0], "=", 2)
	return kv[0], kv[1]
}

func TestGuard_DashboardTokenBecomesRandomSession(t *testing.T) {
	base := "http://" + startGuarded(t)
	status, hdr := guardedStatus(t, http.MethodGet, base+"/?token="+guardToken, "", nil)
	if status != http.StatusOK {
		t.Fatalf("dashboard status %d", status)
	}
	name, id := sessionOf(t, hdr)
	if strings.Contains(hdr.Get("Set-Cookie"), guardToken) || id == guardToken || len(id) != 64 {
		t.Fatalf("the cookie must hold a random id, not the token: %q", hdr.Get("Set-Cookie"))
	}
	if !strings.HasSuffix(name, strings.TrimPrefix(base[strings.LastIndex(base, ":"):], ":")) {
		t.Fatalf("cookie name %q must carry the server's port (%s)", name, base)
	}
	if status, _ = guardedStatus(t, http.MethodGet, base+"/api/v1/health", "", map[string]string{"Cookie": name + "=" + id}); status != http.StatusOK {
		t.Fatalf("the session must authorise, got %d", status)
	}
	_, hdr2 := guardedStatus(t, http.MethodGet, base+"/?token="+guardToken, "", nil)
	if _, id2 := sessionOf(t, hdr2); id2 == id {
		t.Fatal("each visit must get its own session id")
	}
}

func TestGuard_RawTokenAndUnknownIDAsCookieAreRejected(t *testing.T) {
	base := "http://" + startGuarded(t)
	_, hdr := guardedStatus(t, http.MethodGet, base+"/?token="+guardToken, "", nil)
	name, _ := sessionOf(t, hdr)
	for _, v := range []string{guardToken, "0123456789abcdef", ""} {
		if status, _ := guardedStatus(t, http.MethodGet, base+"/api/v1/health", "", map[string]string{"Cookie": name + "=" + v}); status != http.StatusUnauthorized {
			t.Fatalf("cookie value %q must not authorise, got %d", v, status)
		}
	}
}

func TestSessionStore_ExpiresAndBounds(t *testing.T) {
	st := newSessionStore()
	now := time.Now()
	st.now = func() time.Time { return now }
	id, err := st.create()
	if err != nil || !st.valid(id) {
		t.Fatalf("new session invalid: %v", err)
	}
	now = now.Add(sessionTTL + time.Second)
	if st.valid(id) {
		t.Fatal("an expired session must be rejected")
	}
	for i := 0; i < maxSessions+10; i++ {
		if _, err := st.create(); err != nil {
			t.Fatal(err)
		}
	}
	if len(st.ids) > maxSessions {
		t.Fatalf("store grew to %d, cap %d", len(st.ids), maxSessions)
	}
}

func TestGenerateToken_IsRandom(t *testing.T) {
	a, err := GenerateToken()
	b, _ := GenerateToken()
	if err != nil || len(a) != 64 || a == b {
		t.Fatalf("token %q / %q, err %v", a, b, err)
	}
}
