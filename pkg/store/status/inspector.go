package status

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/store/segmented"
)

// StoreInspector provides high-level telemetry, chunk inspection, and safe removal routines.
type StoreInspector struct {
	store *segmented.SegmentedStore
}

// NewStoreInspector creates a new StoreInspector.
func NewStoreInspector(store *segmented.SegmentedStore) *StoreInspector {
	return &StoreInspector{store: store}
}

// InspectStore returns an aggregated status report across all stored projects or a specific filtered project.
func (i *StoreInspector) InspectStore(ctx context.Context, projectFilter string) (*StoreStatusReport, error) {
	checkpoints, err := i.store.ListCheckpoints(ctx, projectFilter)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}

	segments, err := i.store.QueryPartial(ctx, projectFilter)
	if err != nil {
		return nil, fmt.Errorf("querying segments: %w", err)
	}

	groupedCheckpoints := make(map[string][]segmented.ChunkCheckpoint)
	for _, cp := range checkpoints {
		groupedCheckpoints[cp.Project] = append(groupedCheckpoints[cp.Project], cp)
	}

	groupedSegments := make(map[string][]segmented.Segment)
	for _, s := range segments {
		groupedSegments[s.Project] = append(groupedSegments[s.Project], s)
	}

	projectSet := make(map[string]struct{})
	for p := range groupedCheckpoints {
		projectSet[p] = struct{}{}
	}
	for p := range groupedSegments {
		projectSet[p] = struct{}{}
	}

	report := &StoreStatusReport{
		Stores: make([]StoreItemSummary, 0, len(projectSet)),
	}

	for p := range projectSet {
		cps := groupedCheckpoints[p]
		segs := groupedSegments[p]

		summary := summarizeProject(p, cps, segs)
		report.Stores = append(report.Stores, summary)
		report.TotalStores++
		report.TotalSegments += summary.SegmentsCount
		report.TotalRawBytes += summary.RawBytes
		report.TotalCompBytes += summary.CompressedBytes

		if summary.State == segmented.StateStored {
			report.StoredStores++
		} else {
			report.PartialStores++
		}
	}

	return report, nil
}

// InspectStoreDetails retrieves chunk-by-chunk analysis state and segment details for a specific store/project.
func (i *StoreInspector) InspectStoreDetails(ctx context.Context, storeID string) (*StoreDetailReport, error) {
	checkpoints, err := i.store.ListCheckpoints(ctx, storeID)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}

	segments, err := i.store.QueryPartial(ctx, storeID)
	if err != nil {
		return nil, fmt.Errorf("querying segments: %w", err)
	}

	if len(checkpoints) == 0 && len(segments) == 0 {
		return nil, ErrStoreNotFound
	}

	summary := summarizeProject(storeID, checkpoints, segments)
	return &StoreDetailReport{
		Summary:     summary,
		Checkpoints: checkpoints,
		Segments:    segments,
	}, nil
}

// RemoveStore safely purges SQLite records and unlinks compressed blobs with confirmation gate.
func (i *StoreInspector) RemoveStore(ctx context.Context, storeID string, force bool) (*RemovalResult, error) {
	if !force {
		return nil, ErrConfirmationRequired
	}

	segments, err := i.store.QueryPartial(ctx, storeID)
	if err != nil {
		return nil, fmt.Errorf("querying segments for removal: %w", err)
	}

	if len(segments) == 0 {
		// Check if any checkpoints exist
		cps, _ := i.store.ListCheckpoints(ctx, storeID)
		if len(cps) == 0 {
			return nil, ErrStoreNotFound
		}
	}

	var bytesFreed int64
	var blobsUnlinked int

	for _, s := range segments {
		bytesFreed += s.CompressedSize
		if s.BlobPath != "" {
			if err := os.Remove(s.BlobPath); err == nil {
				blobsUnlinked++
			}
		}
	}

	if err := i.store.PurgeProject(ctx, storeID); err != nil {
		return nil, fmt.Errorf("purging project records: %w", err)
	}

	return &RemovalResult{
		StoreID:        storeID,
		SegmentsPurged: len(segments),
		BlobsUnlinked:  blobsUnlinked,
		BytesFreed:     bytesFreed,
		Success:        true,
	}, nil
}

func summarizeProject(project string, cps []segmented.ChunkCheckpoint, segs []segmented.Segment) StoreItemSummary {
	var totalRawBytes, totalCompBytes int64
	var latestUpdate time.Time
	overallState := segmented.StateStored

	for _, cp := range cps {
		if cp.State == segmented.StateAnalyzingPartial {
			overallState = segmented.StateAnalyzingPartial
		}
		if cp.UpdatedAt.After(latestUpdate) {
			latestUpdate = cp.UpdatedAt
		}
	}

	kind := "repo"
	for _, s := range segs {
		totalRawBytes += s.SizeBytes
		totalCompBytes += s.CompressedSize
		if s.Type == segmented.SegmentCFGBlock {
			kind = "binary"
		}
		if s.CreatedAt.After(latestUpdate) {
			latestUpdate = s.CreatedAt
		}
	}

	progress := 100.0
	if overallState == segmented.StateAnalyzingPartial {
		progress = 50.0
	}
	if len(cps) == 0 && len(segs) > 0 {
		progress = 100.0
	}

	tokensEstimated := totalRawBytes / 4
	if tokensEstimated < 1 && totalRawBytes > 0 {
		tokensEstimated = 1
	}

	return StoreItemSummary{
		StoreID:         project,
		Name:            project,
		Kind:            kind,
		State:           overallState,
		Progress:        progress,
		SegmentsCount:   len(segs),
		RawBytes:        totalRawBytes,
		CompressedBytes: totalCompBytes,
		TokensEstimated: tokensEstimated,
		UpdatedAt:       latestUpdate,
	}
}

// FormatTable formats StoreStatusReport into tabular CLI rows.
func FormatTable(report *StoreStatusReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-18s %-20s %-8s %-18s %-10s %-12s %s\n",
		"STORE ID", "NAME", "KIND", "STATE", "PROGRESS", "TOKENS", "UPDATED"))
	sb.WriteString(strings.Repeat("-", 100) + "\n")

	for _, s := range report.Stores {
		updatedStr := "never"
		if !s.UpdatedAt.IsZero() {
			updatedStr = s.UpdatedAt.Format("2006-01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("%-18s %-20s %-8s %-18s %-9.1f%% %-12d %s\n",
			truncate(s.StoreID, 18), truncate(s.Name, 20), s.Kind, s.State, s.Progress, s.TokensEstimated, updatedStr))
	}
	return sb.String()
}

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen-3] + "..."
}
