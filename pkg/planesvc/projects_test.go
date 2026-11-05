package planesvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// endingPlane answers any project-ending request and remembers the paths it was asked for.
type endingPlane struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newEndingPlane(t *testing.T) *endingPlane {
	t.Helper()
	e := &endingPlane{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.paths = append(e.paths, r.Method+" "+r.URL.Path)
		e.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(e.Close)
	return e
}

func (e *endingPlane) saw(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.paths {
		if strings.Contains(p, "/projects/"+id) {
			return true
		}
	}
	return false
}

// The sidecar refuses to end a project without an explicit project_id and confirm: true, so both
// calls must state them and reach the project.
func TestArchiveAndDeleteProjectStateTheProjectAndConfirm(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	cases := map[string]func(*Service, context.Context, *endingPlane) error{
		"archive": func(s *Service, ctx context.Context, e *endingPlane) error {
			return s.ArchiveProject(ctx, planeClientAt(t, e.URL, e.Client()), "proj-9")
		},
		"delete": func(s *Service, ctx context.Context, e *endingPlane) error {
			return s.DeleteProject(ctx, planeClientAt(t, e.URL, e.Client()), "proj-9")
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEndingPlane(t)
			svc := realService()
			defer svc.Close()
			if err := call(svc, context.Background(), e); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !e.saw("proj-9") {
				t.Fatalf("%s did not reach the project; saw %v", name, e.paths)
			}
		})
	}
}
