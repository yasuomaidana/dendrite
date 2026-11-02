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
			path, mf, err := NewModelMigrator(s.URL, "", nil).findOllamaManifest(base, "m")
			if c.found != (err == nil) {
				t.Fatalf("path = %q, mf = %v, err = %v", path, mf, err)
			}
			if c.found && !strings.HasPrefix(path, "ollama-api:") {
				t.Errorf("path = %q", path)
			}
		})
	}
	if _, _, err := NewModelMigrator(closedEndpoint(), "", nil).findOllamaManifest(base, "m"); err == nil {
		t.Error("an unreachable daemon finds nothing")
	}
	if _, _, err := NewModelMigrator("http://%zz", "", nil).findOllamaManifest(base, "m"); err == nil {
		t.Error("an unusable daemon URL finds nothing")
	}
}

func TestMigrateReportsEachStepThatFails(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	m := NewModelMigrator(closedEndpoint(), "", nil)

	t.Run("the model directory cannot be created", func(t *testing.T) {
		base, target := t.TempDir(), filepath.Join(t.TempDir(), "ox")
		ollamaStore(t, base, "m", "latest", "", true)
		if err := os.WriteFile(target, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Migrate(ctx, ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: target}); err == nil {
			t.Fatal("expected the directory error")
		}
	})
	t.Run("the blobs directory cannot be created", func(t *testing.T) {
		base, target := t.TempDir(), t.TempDir()
		ollamaStore(t, base, "m", "latest", "", true)
		if err := os.WriteFile(filepath.Join(target, "blobs"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Migrate(ctx, ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: target}); err == nil {
			t.Fatal("expected the blobs directory error")
		}
	})
	t.Run("the config blob is missing", func(t *testing.T) {
		base := t.TempDir()
		ollamaStore(t, base, "m", "latest", "", false)
		res, err := m.Migrate(ctx, ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: t.TempDir()})
		if err == nil || !strings.Contains(res.Error, "config blob") {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
	t.Run("a layer blob is missing", func(t *testing.T) {
		base := t.TempDir()
		ollamaStore(t, base, "m", "latest", "", true)
		if err := os.Remove(filepath.Join(base, "models", "blobs", "sha256-bbbb")); err != nil {
			t.Fatal(err)
		}
		res, err := m.Migrate(ctx, ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: t.TempDir()})
		if err == nil || !strings.Contains(res.Error, "layer blob") {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
	t.Run("the descriptor cannot be written", func(t *testing.T) {
		base, target := t.TempDir(), t.TempDir()
		ollamaStore(t, base, "m", "latest", "", true)
		if err := os.MkdirAll(filepath.Join(target, "models", "m", "oxillama-manifest.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		res, err := m.Migrate(ctx, ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: target})
		if err == nil || !strings.Contains(res.Error, "write oxillama manifest") {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
}

func TestMigrateLinksNamedLayersAndEvictsTheSource(t *testing.T) {
	isolate(t)
	deleted := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleted = deleted || r.Method == http.MethodDelete
	}))
	defer s.Close()
	base, target := t.TempDir(), t.TempDir()
	manifest := ollamaStore(t, base, "m", "latest", "weights/model.gguf", true)

	res, err := NewModelMigrator(s.URL, "", nil).Migrate(context.Background(), ModelMigrationOptions{ModelName: "m", OllamaDir: base, OxillamaDir: target, DeleteSource: true})
	if err != nil || !res.Success || !res.SourceEvicted || !deleted {
		t.Fatalf("res = %+v, err = %v, deleted = %v", res, err, deleted)
	}
	if _, err := os.Stat(filepath.Join(target, "models", "m", "weights", "model.gguf")); err != nil {
		t.Errorf("the named layer must be linked into the model directory: %v", err)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Errorf("the source manifest should be removed, stat = %v", err)
	}
}

func TestEvictionReportsWhatItManaged(t *testing.T) {
	isolate(t)
	// A declined delete leaves the source in place.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer s.Close()
	base := t.TempDir()
	manifest := ollamaStore(t, base, "m", "latest", "", true)
	m := NewModelMigrator(s.URL, "", nil)
	evicted, freed := m.evictFromOllama(context.Background(), "ollama-api:m", "m", 42)
	if evicted || freed != 42 {
		t.Fatalf("evicted = %v, freed = %d", evicted, freed)
	}
	if evicted, _ := m.evictFromOllama(context.Background(), manifest, "m", 1); !evicted {
		t.Error("removing the manifest from disk counts as an eviction")
	}
	if evicted, _ := NewModelMigrator("http://%zz", "", nil).evictFromOllama(context.Background(), "", "m", 1); evicted {
		t.Error("an unusable daemon URL evicts nothing")
	}
}

func TestBlobHelpers(t *testing.T) {
	dir := t.TempDir()
	m := NewModelMigrator("", "", nil)
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	_ = os.MkdirAll(src, 0o755)
	_ = os.MkdirAll(dst, 0o755)

	if err := m.linkOrCopyBlob(src, dst, "sha256:zzz"); err == nil {
		t.Error("a blob missing at the source is an error")
	}
	_ = os.WriteFile(filepath.Join(src, "sha256-zzz"), []byte("data"), 0o644)
	if err := m.linkOrCopyBlob(src, dst, "sha256:zzz"); err != nil {
		t.Fatal(err)
	}
	if err := m.linkOrCopyBlob(src, dst, "sha256:zzz"); err != nil {
		t.Errorf("a blob already at the destination is left alone: %v", err)
	}

	// A link cannot replace an existing file, so the copy fallback overwrites it.
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.WriteFile(a, []byte("new"), 0o644)
	_ = os.WriteFile(b, []byte("old"), 0o644)
	if err := linkOrCopyFile(a, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(b); string(got) != "new" {
		t.Errorf("copy fallback wrote %q", got)
	}
	if err := linkOrCopyFile(filepath.Join(dir, "missing"), filepath.Join(dir, "c")); err == nil {
		t.Error("a missing source cannot be copied")
	}
	if err := linkOrCopyFile(a, filepath.Join(dir, "no-such-dir", "c")); err == nil {
		t.Error("a destination in a missing directory cannot be created")
	}
}

func TestVerifyMigratedModelNamesTheProblem(t *testing.T) {
	dir := t.TempDir()
	blobs := filepath.Join(dir, "blobs")
	_ = os.MkdirAll(blobs, 0o755)
	m := NewModelMigrator("", "", nil)

	if err := m.verifyMigratedModel(dir, OxiLLaMaManifest{ConfigDigest: cfgDigest}); err == nil || !strings.Contains(err.Error(), "config blob") {
		t.Errorf("err = %v", err)
	}
	_ = os.WriteFile(filepath.Join(blobs, "sha256-aaaa"), []byte("{}"), 0o644)
	if err := m.verifyMigratedModel(dir, OxiLLaMaManifest{ConfigDigest: cfgDigest, Layers: []OllamaManifestLayer{{Digest: layerDigest}}}); err == nil || !strings.Contains(err.Error(), "layer blob") {
		t.Errorf("err = %v", err)
	}
	_ = os.WriteFile(filepath.Join(blobs, "sha256-bbbb"), []byte("12345"), 0o644)
	if err := m.verifyMigratedModel(dir, OxiLLaMaManifest{Layers: []OllamaManifestLayer{{Digest: layerDigest, Size: 9}}}); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("err = %v", err)
	}
}

func TestMigrateReportsTheEnginesItWasGiven(t *testing.T) {
	base := t.TempDir()
	ollamaStore(t, base, "m", "latest", "", true)
	res, err := NewModelMigrator(closedEndpoint(), "", nil).Migrate(context.Background(), ModelMigrationOptions{
		ModelName: "m", OllamaDir: base, DryRun: true, SourceEngine: "ollama-metal", TargetEngine: "oxillama-rust",
	})
	if err != nil || res.SourceEngine != "ollama-metal" || res.TargetEngine != "oxillama-rust" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}
