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

	"Dendrite/pkg/llamalease"
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
	TargetEngine string `json:"target_engine"` // default "llamacpp"
	DeleteSource bool   `json:"delete_source"` // evict from source engine on success to reclaim memory
	OllamaDir    string `json:"ollama_dir"`    // defaults to ~/.ollama
	ModelsDir    string `json:"models_dir"`    // defaults to the llama-server models directory
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
	TargetModel        string        `json:"target_model"`
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

// ModelMigrator moves model weights from Ollama to the llama-server models directory.
type ModelMigrator struct {
	httpClient     *http.Client
	ollamaEndpoint string
	ollamaDir      string
}

// NewModelMigrator creates an instance of ModelMigrator with default or custom endpoints.
func NewModelMigrator(ollamaEndpoint string, client *http.Client) *ModelMigrator {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if ollamaEndpoint == "" {
		ollamaEndpoint = "http://127.0.0.1:11434"
	}

	home, _ := os.UserHomeDir()
	return &ModelMigrator{
		httpClient:     client,
		ollamaEndpoint: strings.TrimRight(ollamaEndpoint, "/"),
		ollamaDir:      filepath.Join(home, ".ollama"),
	}
}

// Migrate places a model's GGUF weights, which Ollama stores as a plain GGUF blob, where
// llama-server finds them (<models dir>/<model>/model.gguf plus an `id` file), verifies the
// result and evicts the source on request.
func (m *ModelMigrator) Migrate(ctx context.Context, opts ModelMigrationOptions) (*ModelMigrationResult, error) {
	start := time.Now()
	res := newMigrationResult(opts)

	if strings.TrimSpace(opts.ModelName) == "" {
		res.Error = ErrEmptyModelName.Error()
		return res, ErrEmptyModelName
	}

	ollamaBase := m.ollamaDir
	if opts.OllamaDir != "" {
		ollamaBase = opts.OllamaDir
	}
	modelsBase := opts.ModelsDir
	if modelsBase == "" {
		modelsBase = llamalease.ModelsDir()
	}

	manifestPath, manifest, err := m.findOllamaManifest(ollamaBase, opts.ModelName)
	if err != nil {
		res.Error = fmt.Sprintf("failed to find Ollama manifest: %v", err)
		return res, fmt.Errorf("%w: %v", ErrModelNotFound, err)
	}
	totalBytes := res.countBlobs(manifest)

	if !opts.DryRun {
		if err := m.install(res, opts, manifest, ollamaBase, modelsBase); err != nil {
			return res, err
		}
		if opts.DeleteSource {
			res.SourceEvicted, res.BytesFreed = m.evictFromOllama(ctx, manifestPath, opts.ModelName, totalBytes)
		}
	}

	res.Success = true
	res.Duration = time.Since(start)
	return res, nil
}

// newMigrationResult starts a result with the engines the caller named, or the defaults.
func newMigrationResult(opts ModelMigrationOptions) *ModelMigrationResult {
	res := &ModelMigrationResult{ModelName: opts.ModelName, SourceEngine: "ollama", TargetEngine: string(EngineLlamaCpp)}
	if opts.SourceEngine != "" {
		res.SourceEngine = opts.SourceEngine
	}
	if opts.TargetEngine != "" {
		res.TargetEngine = opts.TargetEngine
	}
	return res
}

// countBlobs records how many blobs and bytes the model holds and returns the bytes.
func (r *ModelMigrationResult) countBlobs(mf *OllamaManifest) int64 {
	total := mf.Config.Size
	for _, l := range mf.Layers {
		total += l.Size
	}
	r.TotalBytesMigrated = total
	r.BlobsMigrated = len(mf.Layers)
	if mf.Config.Digest != "" {
		r.BlobsMigrated++
	}
	return total
}

// install places the weights, registers the model as zero-cost and verifies the result; res.Error
// says which step failed.
func (m *ModelMigrator) install(res *ModelMigrationResult, opts ModelMigrationOptions, mf *OllamaManifest, ollamaBase, modelsBase string) error {
	weights, ok := weightsLayer(mf)
	if !ok {
		res.Error = "the model has no weights layer"
		return errors.New(res.Error)
	}
	targetDir := filepath.Join(modelsBase, strings.ReplaceAll(opts.ModelName, ":", "_"))
	target, err := placeWeights(filepath.Join(ollamaBase, "models", "blobs"), targetDir, weights, opts.ModelName)
	if err != nil {
		res.Error = err.Error()
		return err
	}
	res.TargetModel = target

	reg := GetZeroCostRegistry()
	_, _ = reg.Add("llamacpp/" + opts.ModelName)
	_, _ = reg.Add(opts.ModelName)
	_ = reg.SaveToFile(statedir.Resolve(DefaultZeroCostConfigFile))

	if err := verifyMigratedModel(target, weights.Size); err != nil {
		res.Error = fmt.Sprintf("target verification failed: %v", err)
		return err
	}
	return nil
}

// placeWeights links the weights blob to <targetDir>/model.gguf and writes the id file.
func placeWeights(blobsDir, targetDir string, weights OllamaManifestLayer, id string) (string, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target model dir: %w", err)
	}
	srcBlob := filepath.Join(blobsDir, strings.Replace(weights.Digest, ":", "-", 1))
	if _, err := os.Stat(srcBlob); err != nil {
		return "", fmt.Errorf("failed to migrate weights blob %s: %w", weights.Digest, err)
	}
	target := filepath.Join(targetDir, "model.gguf")
	if _, err := os.Stat(target); err != nil {
		if err := linkOrCopyFile(srcBlob, target); err != nil {
			return "", fmt.Errorf("failed to migrate weights blob %s: %w", weights.Digest, err)
		}
	}
	if err := os.WriteFile(filepath.Join(targetDir, "id"), []byte(id+"\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to write model id: %w", err)
	}
	return target, nil
}

// weightsLayer is the manifest layer holding the GGUF weights: the one Ollama types as the model
// (application/vnd.ollama.image.model), otherwise the largest.
func weightsLayer(mf *OllamaManifest) (OllamaManifestLayer, bool) {
	var best OllamaManifestLayer
	found := false
	for _, l := range mf.Layers {
		if strings.HasSuffix(l.MediaType, ".image.model") {
			return l, true
		}
		if !found || l.Size > best.Size {
			best, found = l, true
		}
	}
	return best, found
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
					}
					return "ollama-api:" + modelName, &mf, nil
				}
			}
		}
	}

	return "", nil, fmt.Errorf("manifest for %s not found in %s", modelName, ollamaDir)
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

// verifyMigratedModel confirms the weights exist at the target and, when the manifest states a
// size, that it matches.
func verifyMigratedModel(path string, wantSize int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("missing weights %s: %w", path, err)
	}
	if wantSize > 0 && info.Size() != wantSize {
		return fmt.Errorf("weights size mismatch for %s: expected %d, got %d", path, wantSize, info.Size())
	}
	return nil
}

// evictFromOllama sends delete request to Ollama daemon and unlinks local manifest.
func (m *ModelMigrator) evictFromOllama(ctx context.Context, manifestPath, modelName string, freedBytes int64) (bool, int64) {
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

	return evicted, freedBytes
}
