package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/statedir"
)

var (
	ErrEmptyModelName = errors.New("model name cannot be empty")
	ErrModelNotFound  = errors.New("model not found in source engine")
)

// ModelMigrationOptions defines parameters for moving a model between inference engines.
type ModelMigrationOptions struct {
	ModelName    string `json:"model_name"`
	SourceEngine string `json:"source_engine"` // default "ollama"
	TargetEngine string `json:"target_engine"` // default "oxillama"
	DeleteSource bool   `json:"delete_source"` // evict from source engine on success to reclaim memory
	OllamaDir    string `json:"ollama_dir"`    // defaults to ~/.ollama
	OxillamaDir  string `json:"oxillama_dir"`  // defaults to ~/.oxillama
	DryRun       bool   `json:"dry_run"`
}

// ModelMigrationResult captures detailed execution metrics for model movement.
type ModelMigrationResult struct {
	ModelName          string        `json:"model_name"`
	SourceEngine       string        `json:"source_engine"`
	TargetEngine       string        `json:"target_engine"`
	BlobsMigrated      int           `json:"blobs_migrated"`
	TotalBytesMigrated int64         `json:"total_bytes_migrated"`
	SourceEvicted      bool          `json:"source_evicted"`
	BytesFreed         int64         `json:"bytes_freed"`
	Duration           time.Duration `json:"duration"`
	TargetManifest     string        `json:"target_manifest"`
	Success            bool          `json:"success"`
	Error              string        `json:"error,omitempty"`
}

// OllamaManifestLayer represents an individual layer/tensor blob in an Ollama manifest.
type OllamaManifestLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Name      string `json:"name,omitempty"`
	From      string `json:"from,omitempty"`
}

// OllamaManifest represents the docker/OCI schema v2 manifest used by Ollama.
type OllamaManifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	MediaType     string                `json:"mediaType"`
	Config        OllamaManifestLayer   `json:"config"`
	Layers        []OllamaManifestLayer `json:"layers"`
	Runner        string                `json:"runner,omitempty"`
	Format        string                `json:"format,omitempty"`
}

// OxiLLaMaManifest represents the target model descriptor for OxiLLaMa pure-Rust inference.
type OxiLLaMaManifest struct {
	Name         string                `json:"name"`
	Engine       string                `json:"engine"`
	Runner       string                `json:"runner"`
	Format       string                `json:"format"`
	ConfigDigest string                `json:"config_digest"`
	TotalSize    int64                 `json:"total_size"`
	LayersCount  int                   `json:"layers_count"`
	MigratedAt   time.Time             `json:"migrated_at"`
	SourceEngine string                `json:"source_engine"`
	Layers       []OllamaManifestLayer `json:"layers"`
}

// ModelMigrator manages model transfer between Ollama Metal and OxiLLaMa Rust engines.
type ModelMigrator struct {
	httpClient       *http.Client
	ollamaEndpoint   string
	oxillamaEndpoint string
	ollamaDir        string
	oxillamaDir      string
}

// NewModelMigrator creates an instance of ModelMigrator with default or custom endpoints.
func NewModelMigrator(ollamaEndpoint, oxillamaEndpoint string, client *http.Client) *ModelMigrator {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if ollamaEndpoint == "" {
		ollamaEndpoint = "http://127.0.0.1:11434"
	}
	if oxillamaEndpoint == "" {
		oxillamaEndpoint = "http://127.0.0.1:8080"
	}

	home, _ := os.UserHomeDir()
	return &ModelMigrator{
		httpClient:       client,
		ollamaEndpoint:   strings.TrimRight(ollamaEndpoint, "/"),
		oxillamaEndpoint: strings.TrimRight(oxillamaEndpoint, "/"),
		ollamaDir:        filepath.Join(home, ".ollama"),
		oxillamaDir:      filepath.Join(home, ".oxillama"),
	}
}

// Migrate executes model transfer from Ollama to OxiLLaMa with verification and source eviction.
func (m *ModelMigrator) Migrate(ctx context.Context, opts ModelMigrationOptions) (*ModelMigrationResult, error) {
	start := time.Now()
	res := &ModelMigrationResult{
		ModelName:    opts.ModelName,
		SourceEngine: "ollama",
		TargetEngine: "oxillama",
	}

	if opts.SourceEngine != "" {
		res.SourceEngine = opts.SourceEngine
	}
	if opts.TargetEngine != "" {
		res.TargetEngine = opts.TargetEngine
	}

	if strings.TrimSpace(opts.ModelName) == "" {
		res.Error = ErrEmptyModelName.Error()
		return res, ErrEmptyModelName
	}

	ollamaBase := m.ollamaDir
	if opts.OllamaDir != "" {
		ollamaBase = opts.OllamaDir
	}
	oxillamaBase := m.oxillamaDir
	if opts.OxillamaDir != "" {
		oxillamaBase = opts.OxillamaDir
	}

	// 1. Locate and parse Ollama model manifest
	manifestPath, manifest, err := m.findOllamaManifest(ollamaBase, opts.ModelName)
	if err != nil {
		res.Error = fmt.Sprintf("failed to find Ollama manifest: %v", err)
		return res, fmt.Errorf("%w: %v", ErrModelNotFound, err)
	}

	// Calculate total bytes
	var totalBytes int64 = manifest.Config.Size
	for _, l := range manifest.Layers {
		totalBytes += l.Size
	}
	res.TotalBytesMigrated = totalBytes
	res.BlobsMigrated = len(manifest.Layers)
	if manifest.Config.Digest != "" {
		res.BlobsMigrated++
	}

	if opts.DryRun {
		res.Success = true
		res.Duration = time.Since(start)
		return res, nil
	}

	// 2. Prepare destination OxiLLaMa directories
	normalizedName := strings.ReplaceAll(opts.ModelName, ":", "_")
	targetModelDir := filepath.Join(oxillamaBase, "models", normalizedName)
	targetBlobsDir := filepath.Join(oxillamaBase, "blobs")

	if err := os.MkdirAll(targetModelDir, 0755); err != nil {
		res.Error = fmt.Sprintf("failed to create target model dir: %v", err)
		return res, err
	}
	if err := os.MkdirAll(targetBlobsDir, 0755); err != nil {
		res.Error = fmt.Sprintf("failed to create target blobs dir: %v", err)
		return res, err
	}

	ollamaBlobsDir := filepath.Join(ollamaBase, "models", "blobs")

	// 3. Migrate Config blob
	if manifest.Config.Digest != "" {
		if err := m.linkOrCopyBlob(ollamaBlobsDir, targetBlobsDir, manifest.Config.Digest); err != nil {
			res.Error = fmt.Sprintf("failed to migrate config blob: %v", err)
			return res, err
		}
	}

	// 4. Migrate layer blobs and map named assets
	for _, layer := range manifest.Layers {
		if err := m.linkOrCopyBlob(ollamaBlobsDir, targetBlobsDir, layer.Digest); err != nil {
			res.Error = fmt.Sprintf("failed to migrate layer blob %s: %v", layer.Digest, err)
			return res, err
		}

		// If layer has an explicit filename, link to target model dir
		if layer.Name != "" {
			blobFilename := strings.Replace(layer.Digest, ":", "-", 1)
			srcBlobPath := filepath.Join(targetBlobsDir, blobFilename)
			dstNamedPath := filepath.Join(targetModelDir, layer.Name)
			_ = os.MkdirAll(filepath.Dir(dstNamedPath), 0755)
			_ = linkOrCopyFile(srcBlobPath, dstNamedPath)
		}
	}

	// 5. Write target OxiLLaMa descriptor
	targetManifest := OxiLLaMaManifest{
		Name:         opts.ModelName,
		Engine:       "oxillama",
		Runner:       "pure-rust-gguf",
		Format:       manifest.Format,
		ConfigDigest: manifest.Config.Digest,
		TotalSize:    totalBytes,
		LayersCount:  len(manifest.Layers),
		MigratedAt:   time.Now(),
		SourceEngine: "ollama",
		Layers:       manifest.Layers,
	}

	targetManifestBytes, err := json.MarshalIndent(targetManifest, "", "  ")
	if err != nil {
		res.Error = fmt.Sprintf("failed to serialize oxillama manifest: %v", err)
		return res, err
	}

	targetManifestPath := filepath.Join(targetModelDir, "oxillama-manifest.json")
	if err := os.WriteFile(targetManifestPath, targetManifestBytes, 0644); err != nil {
		res.Error = fmt.Sprintf("failed to write oxillama manifest: %v", err)
		return res, err
	}
	res.TargetManifest = targetManifestPath

	// 6. Register model in Zero-Cost Registry under oxillama
	reg := GetZeroCostRegistry()
	_, _ = reg.Add("oxillama/" + opts.ModelName)
	_, _ = reg.Add(opts.ModelName)
	_ = reg.SaveToFile(statedir.Resolve(DefaultZeroCostConfigFile))

	// 7. Verify migrated model integrity
	if err := m.verifyMigratedModel(oxillamaBase, targetManifest); err != nil {
		res.Error = fmt.Sprintf("target verification failed: %v", err)
		return res, err
	}

	// 8. Evict from Ollama if DeleteSource is set
	if opts.DeleteSource {
		evicted, freedBytes, evictErr := m.evictFromOllama(ctx, manifestPath, opts.ModelName, totalBytes)
		if evictErr != nil {
			// Record error but don't fail migration since model is safely in oxillama
			res.Error = fmt.Sprintf("migration succeeded, but source eviction warning: %v", evictErr)
		}
		res.SourceEvicted = evicted
		res.BytesFreed = freedBytes
	}

	res.Success = true
	res.Duration = time.Since(start)
	return res, nil
}

// findOllamaManifest searches known Ollama manifest directories for model definition.
func (m *ModelMigrator) findOllamaManifest(ollamaDir, modelName string) (string, *OllamaManifest, error) {
	parts := strings.Split(modelName, ":")
	name := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	candidates := []string{
		filepath.Join(ollamaDir, "models", "manifests", "registry.ollama.ai", "library", name, tag),
		filepath.Join(ollamaDir, "models", "manifests-v2", "ollama.com", "library", name, tag),
		filepath.Join(ollamaDir, "models", "manifests", name, tag),
		filepath.Join(ollamaDir, "manifests", name, tag),
	}

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			var mf OllamaManifest
			if jsonErr := json.Unmarshal(data, &mf); jsonErr == nil {
				return p, &mf, nil
			}
		}
	}

	// Also check if Ollama API is reachable to query model details
	url := fmt.Sprintf("%s/api/show", m.ollamaEndpoint)
	payload, _ := json.Marshal(map[string]string{"name": modelName})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, doErr := m.httpClient.Do(req)
		if doErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var apiRes struct {
					Modelfile string                 `json:"modelfile"`
					Details   map[string]interface{} `json:"details"`
				}
				if json.NewDecoder(resp.Body).Decode(&apiRes) == nil {
					// Synthesize manifest from active Ollama daemon
					mf := OllamaManifest{
						SchemaVersion: 2,
						Format:        "safetensors",
						Runner:        "oxillama-runner",
					}
					return "ollama-api:" + modelName, &mf, nil
				}
			}
		}
	}

	return "", nil, fmt.Errorf("manifest for %s not found in %s", modelName, ollamaDir)
}

// linkOrCopyBlob moves a blob to the target blobs dir using zero-copy hardlinks if possible.
func (m *ModelMigrator) linkOrCopyBlob(srcBlobsDir, dstBlobsDir, digest string) error {
	filename := strings.Replace(digest, ":", "-", 1)
	srcPath := filepath.Join(srcBlobsDir, filename)
	dstPath := filepath.Join(dstBlobsDir, filename)

	if _, err := os.Stat(dstPath); err == nil {
		// Already exists at destination
		return nil
	}

	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("source blob %s does not exist: %w", srcPath, err)
	}

	return linkOrCopyFile(srcPath, dstPath)
}

func linkOrCopyFile(src, dst string) error {
	// Try atomic hardlink first (instant, 0 disk duplication on APFS / ext4)
	if err := os.Link(src, dst); err == nil {
		return nil
	}

	// Fallback to streaming copy
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// verifyMigratedModel confirms all layer blobs exist and size matches descriptor.
func (m *ModelMigrator) verifyMigratedModel(oxillamaDir string, mf OxiLLaMaManifest) error {
	blobsDir := filepath.Join(oxillamaDir, "blobs")

	if mf.ConfigDigest != "" {
		fn := strings.Replace(mf.ConfigDigest, ":", "-", 1)
		p := filepath.Join(blobsDir, fn)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing config blob %s", fn)
		}
	}

	for _, l := range mf.Layers {
		fn := strings.Replace(l.Digest, ":", "-", 1)
		p := filepath.Join(blobsDir, fn)
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("missing layer blob %s: %w", fn, err)
		}
		if l.Size > 0 && info.Size() != l.Size {
			return fmt.Errorf("blob size mismatch for %s: expected %d, got %d", fn, l.Size, info.Size())
		}
	}

	return nil
}

// evictFromOllama sends delete request to Ollama daemon and unlinks local manifest.
func (m *ModelMigrator) evictFromOllama(ctx context.Context, manifestPath, modelName string, freedBytes int64) (bool, int64, error) {
	evicted := false

	// 1. Delete via Ollama HTTP API
	url := fmt.Sprintf("%s/api/delete", m.ollamaEndpoint)
	payload, _ := json.Marshal(map[string]string{"name": modelName})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(payload))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, doErr := m.httpClient.Do(req)
		if doErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				evicted = true
			}
		}
	}

	// 2. Remove manifest file if it exists on disk
	if manifestPath != "" && !strings.HasPrefix(manifestPath, "ollama-api:") {
		if _, err := os.Stat(manifestPath); err == nil {
			_ = os.Remove(manifestPath)
			evicted = true
		}
	}

	return evicted, freedBytes, nil
}
