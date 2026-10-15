package status

import (
	"errors"
	"time"

	"Dendrite/pkg/store/segmented"
)

var (
	// ErrConfirmationRequired is returned when removal is requested without confirmation.
	ErrConfirmationRequired = errors.New("removal confirmation required (pass force=true or --force)")
	// ErrStoreNotFound is returned when the target store/project is not found.
	ErrStoreNotFound = errors.New("store record not found")
)

// StoreItemSummary represents a high-level summary of a stored project analysis.
type StoreItemSummary struct {
	StoreID         string               `json:"store_id"`
	Name            string               `json:"name"`
	Kind            string               `json:"kind"` // repo, binary, slice
	State           segmented.ChunkState `json:"state"`
	Progress        float64              `json:"progress"`
	SegmentsCount   int                  `json:"segments_count"`
	RawBytes        int64                `json:"raw_bytes"`
	CompressedBytes int64                `json:"compressed_bytes"`
	TokensEstimated int64                `json:"tokens_estimated"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// StoreStatusReport aggregates the full status of all stored analysis entities.
type StoreStatusReport struct {
	TotalStores     int                `json:"total_stores"`
	TotalSegments   int                `json:"total_segments"`
	TotalRawBytes   int64              `json:"total_raw_bytes"`
	TotalCompBytes  int64              `json:"total_comp_bytes"`
	StoredStores    int                `json:"stored_stores"`
	PartialStores   int                `json:"partial_stores"`
	Stores          []StoreItemSummary `json:"stores"`
}

// StoreDetailReport contains granular chunk-by-chunk and segment breakdown.
type StoreDetailReport struct {
	Summary     StoreItemSummary            `json:"summary"`
	Checkpoints []segmented.ChunkCheckpoint `json:"checkpoints"`
	Segments    []segmented.Segment         `json:"segments"`
}

// RemovalResult confirms unlinked records and blob files.
type RemovalResult struct {
	StoreID       string `json:"store_id"`
	SegmentsPurged int    `json:"segments_purged"`
	BlobsUnlinked int    `json:"blobs_unlinked"`
	BytesFreed    int64  `json:"bytes_freed"`
	Success       bool   `json:"success"`
}
