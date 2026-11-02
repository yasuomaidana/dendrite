package plane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
)

// routePlane serves fixed answers keyed "METHOD path-suffix"; the longest matching suffix wins and
// anything unmatched is a 404.
func routePlane(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	keys := make([]string, 0, len(routes))
	for k := range routes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, k := range keys {
			method, suffix, _ := strings.Cut(k, " ")
			if r.Method == method && strings.HasSuffix(r.URL.Path, suffix) {
				routes[k](w, r)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func answerWith(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func TestFindRepoRootReportsAnUnresolvablePath(t *testing.T) {
	old := absPath
	absPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { absPath = old })
	if _, err := FindRepoRoot("anywhere"); err == nil {
		t.Fatal("expected the path error")
	}
}

func TestLinkForDirOutsideARepository(t *testing.T) {
	if _, _, err := LinkForDir(t.TempDir()); err == nil {
		t.Fatal("a directory outside any repository has no link")
	}
}

func TestWriteLinkReportsAnUnwritableRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".dendrite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteLink(root, ProjectLink{ProjectID: "p"}); err == nil {
		t.Fatal("expected the write error")
	}
}

func TestIndexedLinkNeedsAKnownProjectWithABinding(t *testing.T) {
	root := t.TempDir()
	if indexedLink(root) != nil {
		t.Fatal("a directory without a project id has no indexed link")
	}

	id, err := projectid.Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	t.Run("corrupt index", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(home, "projects.json"), []byte("{broken"), 0o644); err != nil {
			t.Fatal(err)
		}
		if indexedLink(root) != nil {
			t.Fatal("a corrupt index yields no link")
		}
	})
	t.Run("no home directory", func(t *testing.T) {
		t.Setenv("DENDRITE_HOME", "")
		t.Setenv("HOME", "")
		if indexedLink(root) != nil {
			t.Fatal("no home yields no link")
		}
	})
	t.Run("an entry without a Plane binding", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(home, "projects.json"), []byte(`{"version":1,"projects":{"`+id+`":{"id":"`+id+`","path":"`+root+`"}}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if indexedLink(root) != nil {
			t.Fatal("an entry without a binding yields no link")
		}
	})
}

func TestProjectLookupAndEnsureReportEachFailure(t *testing.T) {
	ctx := context.Background()
	projects := "/api/v1/workspaces/ws/projects/"
	states := "/api/v1/workspaces/ws/projects/new/states/"
	listing := `[{"id":"new","identifier":"NEW"}]`

	t.Run("FindProject on a failing list", func(t *testing.T) {
		s := routePlane(t, map[string]http.HandlerFunc{"GET " + projects: answerWith(500, "")})
		if _, err := planeAt(t, s.URL).FindProject(ctx, "NEW"); err == nil {
			t.Fatal("expected the list error")
		}
	})
	t.Run("EnsureProject on a failing list", func(t *testing.T) {
		s := routePlane(t, map[string]http.HandlerFunc{"GET " + projects: answerWith(500, "")})
		if _, _, err := planeAt(t, s.URL).EnsureProject(ctx, "n", "NEW", "d"); err == nil {
			t.Fatal("expected the list error")
		}
	})
	t.Run("EnsureProject when the project cannot be created", func(t *testing.T) {
		s := routePlane(t, map[string]http.HandlerFunc{
			"GET " + projects:  answerWith(200, `[]`),
			"POST " + projects: answerWith(500, ""),
		})
		if _, _, err := planeAt(t, s.URL).EnsureProject(ctx, "n", "NEW", "d"); err == nil {
			t.Fatal("expected the create error")
		}
	})
	t.Run("EnsureProject when its states cannot be converged", func(t *testing.T) {
		s := routePlane(t, map[string]http.HandlerFunc{
			"GET " + projects: answerWith(200, listing),
			"GET " + states:   answerWith(500, ""),
		})
		proj, created, err := planeAt(t, s.URL).EnsureProject(ctx, "n", "NEW", "d")
		if err == nil || !strings.Contains(err.Error(), "ensure lifecycle states") || proj == nil || created {
			t.Fatalf("proj=%v created=%v err=%v", proj, created, err)
		}
	})
}
