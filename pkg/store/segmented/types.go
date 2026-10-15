package segmented

import (
	"errors"
	"time"
)

var (
	// ErrSegmentNotFound is returned when a requested segment does not exist.
	ErrSegmentNotFound = errors.New("segment not found")
	// ErrCheckpointNotFound is returned when a chunk checkpoint does not exist.
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	// ErrEmptyChunk is returned when attempting to commit an empty chunk.
	ErrEmptyChunk = errors.New("chunk cannot be empty")
	// ErrCorruptedBlob is returned when a stored blob fails integrity verification.
	ErrCorruptedBlob = errors.New("stored blob corrupted or checksum mismatch")
)

// ChunkState represents the operational status of an analysis chunk.
type ChunkState string

const (
	StateStored            ChunkState = "STORED"            // 100% complete analysis
	StateAnalyzingPartial  ChunkState = "ANALYZING_PARTIAL" // Partially committed, queryable
	StateQueuedNightLocal  ChunkState = "QUEUED_NIGHT_LOCAL"
	StatePausedQuotaFloor  ChunkState = "PAUSED_QUOTA_FLOOR"
)

// SegmentType identifies the granularity of an indexed code artifact.
type SegmentType string

const (
	SegmentFile     SegmentType = "file"
	SegmentFunction SegmentType = "function"
	SegmentCFGBlock SegmentType = "cfg_block"
	SegmentASTGraph SegmentType = "ast_graph"
)

// Segment represents a content-addressed code or binary slice.
type Segment struct {
	Hash           string                 `json:"hash"`
	Type           SegmentType            `json:"type"`
	Identifier     string                 `json:"identifier"`
	Project        string                 `json:"project"`
	SizeBytes      int64                  `json:"size_bytes"`
	CompressedSize int64                  `json:"compressed_size"`
	BlobPath       string                 `json:"blob_path,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ChunkCheckpoint records the atomic checkpoint of a project's segmented analysis.
type ChunkCheckpoint struct {
	ChunkID           string     `json:"chunk_id"`
	Project           string     `json:"project"`
	State             ChunkState `json:"state"`
	TotalSegments     int        `json:"total_segments"`
	CommittedSegments int        `json:"committed_segments"`
	PercentComplete   float64    `json:"percent_complete"`
	Segments          []Segment  `json:"segments,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
