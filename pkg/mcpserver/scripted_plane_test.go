package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

// apiRule answers requests whose method and path contain the given parts (either may be empty).
type apiRule struct {
	method, contains string
	status           int
	body             string
}

// scriptedPlane serves a Plane API from an ordered list of rules (first match wins) and records
// every request as "METHOD path". Unmatched requests get 200 and an empty object.
type scriptedPlane struct {
	*httptest.Server
	mu    sync.Mutex
	rules []apiRule
	Calls []string
}

func newScriptedPlane(t *testing.T, rules ...apiRule) *scriptedPlane {
	t.Helper()
	sp := &scriptedPlane{rules: rules}
	sp.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sp.mu.Lock()
		sp.Calls = append(sp.Calls, r.Method+" "+r.URL.Path)
		sp.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		for _, ru := range sp.rules {
			if (ru.method == "" || ru.method == r.Method) && strings.Contains(r.URL.Path, ru.contains) {
				w.WriteHeader(ru.status)
				_, _ = w.Write([]byte(ru.body))
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(sp.Close)
	return sp
}

func (sp *scriptedPlane) client(t *testing.T) *plane.Client {
	t.Helper()
	c, err := plane.NewClient(plane.Config{BaseURL: sp.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "token"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (sp *scriptedPlane) called(prefix string) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	n := 0
	for _, c := range sp.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// toolsWith registers every default tool against a Plane client built from sp (nil for none).
func toolsWith(t *testing.T, sp *scriptedPlane, devStatus string) *Server {
	t.Helper()
	srv := NewServer(ServerConfig{})
	deps := Dependencies{DevStatus: devStatus}
	if sp != nil {
		deps.PlaneClient = sp.client(t)
	}
	RegisterDefaultTools(srv, deps)
	return srv
}

// noEnvPlane makes plane.NewClientFromEnv fail, as it does where no API key is configured.
func noEnvPlane(t *testing.T) {
	t.Helper()
	t.Setenv("PLANE_API_KEY", "")
}
