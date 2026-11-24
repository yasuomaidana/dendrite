package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelMigratorValidationErrors(t *testing.T) {
	m := NewModelMigrator("", nil)
	if _, err := m.Migrate(context.Background(), ModelMigrationOptions{ModelName: ""}); err != ErrEmptyModelName {
		t.Fatalf("expected ErrEmptyModelName, got %v", err)
	}
	if _, err := m.Migrate(context.Background(), ModelMigrationOptions{ModelName: "nonexistent:latest", OllamaDir: t.TempDir()}); err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestModelMigratorDryRunCreatesNothing(t *testing.T) {
	base, target := t.TempDir(), filepath.Join(t.TempDir(), "models")
	ollamaStore(t, base, "m", "latest", "", true)
	res, err := NewModelMigrator(closedEndpoint(), nil).Migrate(context.Background(), ModelMigrationOptions{ModelName: "m", OllamaDir: base, ModelsDir: target, DryRun: true})
	if err != nil || !res.Success {
		t.Fatalf("dry run: res = %+v, err = %v", res, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("dry run should not create target directories")
	}
}

// migrationFixture is an Ollama store holding model "m" and an empty models directory.
type migrationFixture struct {
	ollama, models string
	manifest       string
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	isolate(t)
	f := migrationFixture{ollama: t.TempDir(), models: t.TempDir()}
	f.manifest = ollamaStore(t, f.ollama, "m", "latest", "", true)
	return f
}

func (f migrationFixture) options() ModelMigrationOptions {
	return ModelMigrationOptions{ModelName: "m", OllamaDir: f.ollama, ModelsDir: f.models}
}

func (f migrationFixture) blob() string {
	return filepath.Join(f.ollama, "models", "blobs", "sha256-bbbb")
}

func TestModelMigratorLinksTheWeightsAndWritesTheID(t *testing.T) {
	f := newMigrationFixture(t)
	res, err := NewModelMigrator(closedEndpoint(), nil).Migrate(context.Background(), f.options())
	if err != nil || !res.Success || res.SourceEvicted {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	gguf := filepath.Join(f.models, "m", "model.gguf")
	if res.TargetModel != gguf || res.TargetEngine != "llamacpp" {
		t.Errorf("res = %+v, want target %s", res, gguf)
	}
	if got, _ := os.ReadFile(gguf); string(got) != "weights" {
		t.Errorf("weights = %q", got)
	}
	if id, _ := os.ReadFile(filepath.Join(f.models, "m", "id")); string(id) != "m\n" {
		t.Errorf("id = %q", id)
	}
	if ok, _ := GetZeroCostRegistry().Test("llamacpp/m"); !ok {
		t.Error("expected llamacpp/m to qualify as zero-cost")
	}
}

func TestModelMigratorEvictsTheSourceOnRequest(t *testing.T) {
	f := newMigrationFixture(t)
	deleted := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleted = deleted || (r.Method == http.MethodDelete && r.URL.Path == "/api/delete")
	}))
	defer s.Close()
	opts := f.options()
	opts.DeleteSource = true
	res, err := NewModelMigrator(s.URL, s.Client()).Migrate(context.Background(), opts)
	if err != nil || !res.SourceEvicted || !deleted {
		t.Fatalf("res = %+v, err = %v, deleted = %v", res, err, deleted)
	}
	if _, err := os.Stat(f.manifest); !os.IsNotExist(err) {
		t.Errorf("the source manifest should be removed, stat = %v", err)
	}
}

func TestModelMigratorDefaultsTheModelsDirectory(t *testing.T) {
	f := newMigrationFixture(t)
	t.Setenv("DENDRITE_GGUF_MODELS_DIR", f.models)
	opts := f.options()
	opts.ModelsDir = ""
	res, err := NewModelMigrator(closedEndpoint(), nil).Migrate(context.Background(), opts)
	if err != nil || !strings.HasPrefix(res.TargetModel, f.models) {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

// Each broken store or target stops the migration at its own step and names it.
func TestModelMigratorReportsEachStepThatFails(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(t *testing.T, f migrationFixture) ModelMigrationOptions
		want   string
	}{
		{"the models directory is a file", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			o := f.options()
			o.ModelsDir = filepath.Join(f.models, "file")
			writeFile(t, o.ModelsDir, "")
			return o
		}, "target model dir"},
		{"the weights blob is missing", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			if err := os.Remove(f.blob()); err != nil {
				t.Fatal(err)
			}
			return f.options()
		}, "weights blob"},
		{"the weights blob cannot be copied", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			if err := os.Remove(f.blob()); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(f.blob(), 0o755); err != nil {
				t.Fatal(err)
			}
			return f.options()
		}, "weights blob"},
		{"the id file is a directory", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			if err := os.MkdirAll(filepath.Join(f.models, "m", "id"), 0o755); err != nil {
				t.Fatal(err)
			}
			return f.options()
		}, "model id"},
		{"the model has no layers", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			writeFile(t, f.manifest, `{"schemaVersion":2}`)
			return f.options()
		}, "no weights layer"},
		{"the blob size differs from the manifest", func(t *testing.T, f migrationFixture) ModelMigrationOptions {
			writeFile(t, f.blob(), "longer than seven")
			o := f.options()
			o.DeleteSource = true
			return o
		}, "size mismatch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newMigrationFixture(t)
			opts := c.break_(t, f)
			res, err := NewModelMigrator(closedEndpoint(), nil).Migrate(context.Background(), opts)
			if err == nil || res.SourceEvicted || !strings.Contains(res.Error, c.want) {
				t.Fatalf("res = %+v, err = %v, want an error naming %q and the source kept", res, err, c.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWeightsLayerPrefersTheTypedLayerThenTheLargest(t *testing.T) {
	typed := OllamaManifestLayer{MediaType: "application/vnd.ollama.image.model", Digest: "t", Size: 1}
	big := OllamaManifestLayer{MediaType: "application/vnd.ollama.image.params", Digest: "b", Size: 9}
	small := OllamaManifestLayer{Digest: "s", Size: 2}
	if got, ok := weightsLayer(&OllamaManifest{Layers: []OllamaManifestLayer{big, typed}}); !ok || got.Digest != "t" {
		t.Errorf("typed layer: %+v, %v", got, ok)
	}
	if got, ok := weightsLayer(&OllamaManifest{Layers: []OllamaManifestLayer{small, big}}); !ok || got.Digest != "b" {
		t.Errorf("largest layer: %+v, %v", got, ok)
	}
	if _, ok := weightsLayer(&OllamaManifest{}); ok {
		t.Error("no layers, no weights")
	}
}

func TestModelMigratorReportsTheEnginesItWasGiven(t *testing.T) {
	base := t.TempDir()
	ollamaStore(t, base, "m", "latest", "", true)
	res, err := NewModelMigrator(closedEndpoint(), nil).Migrate(context.Background(), ModelMigrationOptions{
		ModelName: "m", OllamaDir: base, DryRun: true, SourceEngine: "ollama-metal", TargetEngine: "llamacpp-local",
	})
	if err != nil || res.SourceEngine != "ollama-metal" || res.TargetEngine != "llamacpp-local" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestLinkOrCopyFileFallsBackToACopy(t *testing.T) {
	dir := t.TempDir()
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
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := verifyMigratedModel(path, 0); err == nil || !strings.Contains(err.Error(), "missing weights") {
		t.Errorf("err = %v", err)
	}
	_ = os.WriteFile(path, []byte("12345"), 0o644)
	if err := verifyMigratedModel(path, 9); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("err = %v", err)
	}
	if err := verifyMigratedModel(path, 5); err != nil {
		t.Errorf("matching size: %v", err)
	}
	if err := verifyMigratedModel(path, 0); err != nil {
		t.Errorf("unstated size: %v", err)
	}
}
