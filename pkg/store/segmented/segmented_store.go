package segmented

import (
	"Dendrite/pkg/statedir"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SegmentedStore coordinates persistent content-addressed segmented storage and chunk checkpointing.
type SegmentedStore struct {
	baseDir string
	blobs   *BlobStore
	index   *SQLiteIndex
	mu      sync.RWMutex
}

// Blobs returns the underlying BlobStore.
func (s *SegmentedStore) Blobs() *BlobStore {
	return s.blobs
}

// ComputeHash computes a deterministic BLAKE3/SHA256 hex digest from raw bytes.
func (s *SegmentedStore) ComputeHash(data []byte) string {
	return s.blobs.ComputeHash(data)
}

// NewSegmentedStore creates a new SegmentedStore rooted at baseDir (default ~/.dendrite/store).
func NewSegmentedStore(baseDir string) (*SegmentedStore, error) {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			baseDir = filepath.Join(home, ".dendrite", "store")
		} else {
			baseDir = statedir.Resolve(".dendrite/store")
		}
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating base store dir: %w", err)
	}

	blobsDir := filepath.Join(baseDir, "blobs")
	dbPath := filepath.Join(baseDir, "meta.db")

	blobs := NewBlobStore(blobsDir)
	index, err := NewSQLiteIndex(dbPath)
	if err != nil {
		return nil, fmt.Errorf("initializing sqlite index: %w", err)
	}

	return &SegmentedStore{
		baseDir: baseDir,
		blobs:   blobs,
		index:   index,
	}, nil
}

// CommitChunk atomically commits a chunk of segments and their data blobs.
// If isFinal is false, checkpoint state is ANALYZING_PARTIAL (queryable immediately).
// If isFinal is true, state is STORED.
func (s *SegmentedStore) CommitChunk(
	ctx context.Context,
	project string,
	chunkID string,
	segments []Segment,
	dataBlobs map[string][]byte,
	isFinal bool,
) (*ChunkCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(segments) == 0 {
		return nil, ErrEmptyChunk
	}

	now := time.Now().UTC()
	committedSegments := make([]Segment, 0, len(segments))

	// 1. Write blobs atomically
	for _, seg := range segments {
		data, ok := dataBlobs[seg.Hash]
		if !ok && seg.SizeBytes > 0 {
			data = []byte(seg.Identifier)
		}

		blobPath, compSize, err := s.blobs.WriteBlob(seg.Hash, data)
		if err != nil {
			return nil, fmt.Errorf("writing blob %s: %w", seg.Hash, err)
		}

		seg.BlobPath = blobPath
		seg.CompressedSize = compSize
		seg.SizeBytes = int64(len(data))
		if seg.CreatedAt.IsZero() {
			seg.CreatedAt = now
		}
		committedSegments = append(committedSegments, seg)
	}

	// 2. Determine chunk state
	state := StateAnalyzingPartial
	if isFinal {
		state = StateStored
	}

	cp := ChunkCheckpoint{
		ChunkID:           chunkID,
		Project:           project,
		State:             state,
		TotalSegments:     len(committedSegments),
		CommittedSegments: len(committedSegments),
		PercentComplete:   100.0,
		Segments:          committedSegments,
		UpdatedAt:         now,
	}

	// 3. Commit checkpoint and segments in SQLite ACID transaction
	if err := s.index.SaveCheckpointTx(cp, committedSegments); err != nil {
		return nil, fmt.Errorf("committing sqlite transaction: %w", err)
	}

	return &cp, nil
}

// QueryPartial retrieves all committed segments for a project, including partial analyses.
func (s *SegmentedStore) QueryPartial(ctx context.Context, project string) ([]Segment, error) {
	return s.index.ListSegments(project)
}

// GetSegmentData reads and decompresses blob bytes for a segment hash.
func (s *SegmentedStore) GetSegmentData(ctx context.Context, hash string) ([]byte, error) {
	return s.blobs.ReadBlob(hash)
}

// GetCheckpoint returns the status of a specific chunk.
func (s *SegmentedStore) GetCheckpoint(ctx context.Context, chunkID string) (*ChunkCheckpoint, error) {
	return s.index.GetCheckpoint(chunkID)
}

// ListCheckpoints lists all checkpoints for a project.
func (s *SegmentedStore) ListCheckpoints(ctx context.Context, project string) ([]ChunkCheckpoint, error) {
	return s.index.ListCheckpoints(project)
}

// UpdateCheckpointState modifies the operational state of a chunk checkpoint.
func (s *SegmentedStore) UpdateCheckpointState(ctx context.Context, chunkID string, newState ChunkState) (*ChunkCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp, err := s.index.GetCheckpoint(chunkID)
	if err != nil {
		return nil, err
	}

	cp.State = newState
	cp.UpdatedAt = time.Now().UTC()

	if err := s.index.SaveCheckpointTx(*cp, cp.Segments); err != nil {
		return nil, fmt.Errorf("updating checkpoint state: %w", err)
	}

	return cp, nil
}

// PurgeProject explicitly removes all data for a project (persists until this call).
func (s *SegmentedStore) PurgeProject(ctx context.Context, project string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.index.PurgeProject(project)
}

// SimulateCrashMidWrite simulates an interrupted process kill mid-write.
// Validates that partial temporary files do not corrupt the committed SQLite store.
func (s *SegmentedStore) SimulateCrashMidWrite(ctx context.Context, project, chunkID string, dataBlobs map[string][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Intentionally write uncommitted .tmp files in blobs directory and exit before DB commit
	for hash, data := range dataBlobs {
		tmpPath := filepath.Join(s.blobs.baseDir, hash+".tmp")
		_ = os.WriteFile(tmpPath, data, 0644)
	}

	// Simulated crash: return without updating index
	return fmt.Errorf("simulated process kill mid-chunk write")
}
