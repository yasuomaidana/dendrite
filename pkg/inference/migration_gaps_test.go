package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	cfgDigest   = "sha256:aaaa"
	layerDigest = "sha256:bbbb"
)

// ollamaStore writes a one-layer model into an Ollama directory and returns the manifest's path.
func ollamaStore(t *testing.T, base, name, tag string, layerName string, withBlobs bool) string {
	t.Helper()
	dir := filepath.Join(base, "models", "manifests", "registry.ollama.ai", "library", name)
	blobs := filepath.Join(base, "models", "blobs")
	for _, d := range []string{dir, blobs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withBlobs {
		_ = os.WriteFile(filepath.Join(blobs, "sha256-aaaa"), []byte("{}"), 0o644)
		_ = os.WriteFile(filepath.Join(blobs, "sha256-bbbb"), []byte("weights"), 0o644)
	}
	mf := OllamaManifest{
		SchemaVersion: 2,
		Format:        "gguf",
		Config:        OllamaManifestLayer{Digest: cfgDigest, Size: 2},
		Layers:        []OllamaManifestLayer{{Digest: layerDigest, Size: 7, Name: layerName}},
	}
	data, _ := json.Marshal(mf)
	path := filepath.Join(dir, tag)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_HOME", t.TempDir())
}

func TestFindOllamaManifestFallsBackToTheDaemon(t *testing.T) {
	base := t.TempDir()
	broken := filepath.Join(base, "models", "manifests", "registry.ollama.ai", "library", "m")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "latest"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		handler http.HandlerFunc
		found   bool
	}{
		{"the daemon knows the model", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"modelfile":"x","details":{}}`)) }, true},
		{"the daemon answers garbage", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("nope")) }, false},
		{"the daemon refuses", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := httptest.NewServer(c.handler)
			defer s.Close()
			path, mf, err := NewModelMigrator(s.URL, nil).findOllamaManifest(base, "m")
			if c.found != (err == nil) {
				t.Fatalf("path = %q, mf = %v, err = %v", path, mf, err)
			}
			if c.found && !strings.HasPrefix(path, "ollama-api:") {
				t.Errorf("path = %q", path)
			}
		})
	}
	if _, _, err := NewModelMigrator(closedEndpoint(), nil).findOllamaManifest(base, "m"); err == nil {
		t.Error("an unreachable daemon finds nothing")
	}
	if _, _, err := NewModelMigrator("http://%zz", nil).findOllamaManifest(base, "m"); err == nil {
		t.Error("an unusable daemon URL finds nothing")
	}
}

func TestEvictionReportsWhatItManaged(t *testing.T) {
	isolate(t)
	// A declined delete leaves the source in place.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer s.Close()
	base := t.TempDir()
	manifest := ollamaStore(t, base, "m", "latest", "", true)
	m := NewModelMigrator(s.URL, nil)
	evicted, freed := m.evictFromOllama(context.Background(), "ollama-api:m", "m", 42)
	if evicted || freed != 42 {
		t.Fatalf("evicted = %v, freed = %d", evicted, freed)
	}
	if evicted, _ := m.evictFromOllama(context.Background(), manifest, "m", 1); !evicted {
		t.Error("removing the manifest from disk counts as an eviction")
	}
	if evicted, _ := NewModelMigrator("http://%zz", nil).evictFromOllama(context.Background(), "", "m", 1); evicted {
		t.Error("an unusable daemon URL evicts nothing")
	}
}
