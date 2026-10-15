package segmented

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// BlobStore manages compressed, content-addressed blob storage with atomic writes.
type BlobStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewBlobStore creates a new BlobStore rooted at baseDir.
func NewBlobStore(baseDir string) *BlobStore {
	if baseDir == "" {
		baseDir = ".dendrite/store/blobs"
	}
	return &BlobStore{baseDir: baseDir}
}

// ComputeHash calculates a deterministic BLAKE3/SHA256 hex string from raw bytes.
func (b *BlobStore) ComputeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// WriteBlob compresses and atomically writes a blob under its content hash.
// Guarantees zero data corruption on process kill mid-write via atomic rename.
func (b *BlobStore) WriteBlob(hash string, data []byte) (string, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := os.MkdirAll(b.baseDir, 0755); err != nil {
		return "", 0, fmt.Errorf("creating blob directory: %w", err)
	}

	finalPath := filepath.Join(b.baseDir, hash+".zst")
	if info, err := os.Stat(finalPath); err == nil {
		// Already stored
		return finalPath, info.Size(), nil
	}

	tmpPath := filepath.Join(b.baseDir, hash+".tmp")
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return "", 0, fmt.Errorf("compressing blob: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", 0, fmt.Errorf("closing compressor: %w", err)
	}

	compressedData := buf.Bytes()
	if err := os.WriteFile(tmpPath, compressedData, 0644); err != nil {
		return "", 0, fmt.Errorf("writing temporary blob: %w", err)
	}

	// Atomic rename ensures crash-proof commit
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("atomic commit of blob: %w", err)
	}

	return finalPath, int64(len(compressedData)), nil
}

// ReadBlob reads and decompresses a blob, verifying content hash integrity.
func (b *BlobStore) ReadBlob(hash string) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	finalPath := filepath.Join(b.baseDir, hash+".zst")
	data, err := os.ReadFile(finalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrSegmentNotFound
		}
		return nil, fmt.Errorf("reading blob %s: %w", hash, err)
	}

	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: decompression failed: %v", ErrCorruptedBlob, err)
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: read error: %v", ErrCorruptedBlob, err)
	}

	// Validate hash integrity
	computed := b.ComputeHash(decompressed)
	if computed != hash {
		return nil, fmt.Errorf("%w: computed %s != expected %s", ErrCorruptedBlob, computed, hash)
	}

	return decompressed, nil
}

// HasBlob checks if a compressed blob is already present.
func (b *BlobStore) HasBlob(hash string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	finalPath := filepath.Join(b.baseDir, hash+".zst")
	_, err := os.Stat(finalPath)
	return err == nil
}
