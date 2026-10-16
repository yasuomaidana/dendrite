package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestModelMigrator_ValidationErrors(t *testing.T) {
	migrator := NewModelMigrator("", "", nil)

	// Empty model name
	_, err := migrator.Migrate(context.Background(), ModelMigrationOptions{ModelName: ""})
	if err != ErrEmptyModelName {
		t.Fatalf("expected ErrEmptyModelName, got %v", err)
	}

	// Non-existent model
	tmpDir := t.TempDir()
	_, err = migrator.Migrate(context.Background(), ModelMigrationOptions{
		ModelName: "nonexistent:latest",
		OllamaDir: tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestModelMigrator_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	ollamaDir := filepath.Join(tmpDir, "ollama")
	oxillamaDir := filepath.Join(tmpDir, "oxillama")

	setupMockOllamaModel(t, ollamaDir, "qwen-test:7b", "dummy-weight-data")

	migrator := NewModelMigrator("", "", nil)
	res, err := migrator.Migrate(context.Background(), ModelMigrationOptions{
		ModelName:   "qwen-test:7b",
		OllamaDir:   ollamaDir,
		OxillamaDir: oxillamaDir,
		DryRun:      true,
	})

	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !res.Success {
		t.Fatal("expected dry run success")
	}
	if _, err := os.Stat(filepath.Join(oxillamaDir, "models")); !os.IsNotExist(err) {
		t.Fatal("dry run should not create target directories")
	}
}

func TestModelMigrator_FullMigrationAndEviction(t *testing.T) {
	tmpDir := t.TempDir()
	ollamaDir := filepath.Join(tmpDir, "ollama")
	oxillamaDir := filepath.Join(tmpDir, "oxillama")

	payload := "tensor-layer-content-xyz"
	setupMockOllamaModel(t, ollamaDir, "qwen3.5:9b", payload)

	// Mock Ollama HTTP daemon
	var deleteReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/delete" {
			deleteReceived = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	migrator := NewModelMigrator(server.URL, "", server.Client())
	res, err := migrator.Migrate(context.Background(), ModelMigrationOptions{
		ModelName:    "qwen3.5:9b",
		OllamaDir:    ollamaDir,
		OxillamaDir:  oxillamaDir,
		DeleteSource: true,
	})

	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("migration failed: %s", res.Error)
	}
	if !res.SourceEvicted {
		t.Fatal("expected source to be evicted")
	}
	if !deleteReceived {
		t.Fatal("expected Ollama delete API to be called")
	}

	// Verify target files exist
	targetManifestPath := filepath.Join(oxillamaDir, "models", "qwen3.5_9b", "oxillama-manifest.json")
	if _, err := os.Stat(targetManifestPath); err != nil {
		t.Fatalf("target manifest missing: %v", err)
	}

	// Verify descriptor content
	data, err := os.ReadFile(targetManifestPath)
	if err != nil {
		t.Fatalf("failed to read target manifest: %v", err)
	}
	var mf OxiLLaMaManifest
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("invalid json manifest: %v", err)
	}
	if mf.Name != "qwen3.5:9b" || mf.Engine != "oxillama" {
		t.Fatalf("unexpected manifest fields: %+v", mf)
	}

	// Verify zero-cost qualification
	reg := GetZeroCostRegistry()
	isZeroCost, _ := reg.Test("oxillama/qwen3.5:9b")
	if !isZeroCost {
		t.Errorf("expected oxillama/qwen3.5:9b to qualify as zero-cost")
	}
}

func TestModelMigrator_VerificationFailureNoEviction(t *testing.T) {
	tmpDir := t.TempDir()
	ollamaDir := filepath.Join(tmpDir, "ollama")
	oxillamaDir := filepath.Join(tmpDir, "oxillama")

	setupMockOllamaModel(t, ollamaDir, "qwen-corrupt:latest", "valid-content")

	// Corrupt blob size in manifest
	manifestFile := filepath.Join(ollamaDir, "models", "manifests", "registry.ollama.ai", "library", "qwen-corrupt", "latest")
	data, _ := os.ReadFile(manifestFile)
	var mf OllamaManifest
	_ = json.Unmarshal(data, &mf)
	mf.Layers[0].Size = 999999 // Mismatch with actual size
	corruptBytes, _ := json.Marshal(mf)
	_ = os.WriteFile(manifestFile, corruptBytes, 0644)

	migrator := NewModelMigrator("", "", nil)
	res, err := migrator.Migrate(context.Background(), ModelMigrationOptions{
		ModelName:    "qwen-corrupt:latest",
		OllamaDir:    ollamaDir,
		OxillamaDir:  oxillamaDir,
		DeleteSource: true,
	})

	if err == nil {
		t.Fatal("expected verification error on size mismatch")
	}
	if res.SourceEvicted {
		t.Fatal("source must NOT be evicted when verification fails")
	}
}

func setupMockOllamaModel(t *testing.T, ollamaDir, modelName, content string) {
	manifestDir := filepath.Join(ollamaDir, "models", "manifests", "registry.ollama.ai", "library", "qwen-test")
	parts := filepath.SplitList(modelName)
	_ = parts
	name := "qwen-test"
	tag := "7b"
	if modelName == "qwen3.5:9b" {
		name = "qwen3.5"
		tag = "9b"
	} else if modelName == "qwen-corrupt:latest" {
		name = "qwen-corrupt"
		tag = "latest"
	}
	manifestDir = filepath.Join(ollamaDir, "models", "manifests", "registry.ollama.ai", "library", name)
	blobsDir := filepath.Join(ollamaDir, "models", "blobs")

	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatal(err)
	}

	configDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	layerDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	configBlob := filepath.Join(blobsDir, "sha256-1111111111111111111111111111111111111111111111111111111111111111")
	layerBlob := filepath.Join(blobsDir, "sha256-2222222222222222222222222222222222222222222222222222222222222222")

	_ = os.WriteFile(configBlob, []byte("{}"), 0644)
	_ = os.WriteFile(layerBlob, []byte(content), 0644)

	mf := OllamaManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Config: OllamaManifestLayer{
			Digest: configDigest,
			Size:   int64(len("{}")),
		},
		Layers: []OllamaManifestLayer{
			{
				MediaType: "application/vnd.ollama.image.tensor",
				Digest:    layerDigest,
				Size:      int64(len(content)),
				Name:      "weights.safetensors",
			},
		},
		Format: "safetensors",
		Runner: "oxillama",
	}

	data, _ := json.Marshal(mf)
	manifestPath := filepath.Join(manifestDir, tag)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("")
}
