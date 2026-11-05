package store

import (
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

func TestStoreUI_RenderStorageBarAndDashboard(t *testing.T) {
	report := &status.StoreStatusReport{
		TotalStores:    2,
		TotalSegments:  15,
		TotalRawBytes:  1024 * 1024 * 50, // 50MB
		TotalCompBytes: 1024 * 1024 * 10, // 10MB
		StoredStores:   1,
		PartialStores:  1,
		Stores: []status.StoreItemSummary{
			{
				StoreID:         "proj-a",
				Kind:            "repo",
				State:           segmented.StateStored,
				Progress:        100.0,
				SegmentsCount:   10,
				CompressedBytes: 1024 * 1024 * 8,
				TokensEstimated: 20000,
				UpdatedAt:       time.Now().UTC(),
			},
			{
				StoreID:         "proj-b",
				Kind:            "binary",
				State:           segmented.StateAnalyzingPartial,
				Progress:        50.0,
				SegmentsCount:   5,
				CompressedBytes: 1024 * 1024 * 2,
				TokensEstimated: 5000,
				UpdatedAt:       time.Now().UTC(),
			},
		},
	}

	bar := RenderStorageBar(report, 0) // test default capacity
	if !strings.Contains(bar, "dendrite-storage-bar-widget") || !strings.Contains(bar, "Total Projects: 2") {
		t.Fatalf("storage bar output malformed:\n%s", bar)
	}

	dashboard := RenderDashboard(report)
	if !strings.Contains(dashboard, "proj-a") || !strings.Contains(dashboard, "proj-b") || !strings.Contains(dashboard, "table") {
		t.Fatalf("dashboard output malformed:\n%s", dashboard)
	}
}

func TestStoreUI_CapacityCapAndEmptyReport(t *testing.T) {
	// Test capacity cap (>100% used)
	overflowReport := &status.StoreStatusReport{
		TotalCompBytes: 2000,
	}
	cappedBar := RenderStorageBar(overflowReport, 1000)
	if !strings.Contains(cappedBar, "width: 100.0%") {
		t.Fatalf("expected capped 100.0%% width, got:\n%s", cappedBar)
	}

	// Test empty report
	emptyReport := &status.StoreStatusReport{}
	emptyBar := RenderStorageBar(emptyReport, 5000)
	if !strings.Contains(emptyBar, "width: 0.0%") {
		t.Fatalf("expected 0.0%% width, got:\n%s", emptyBar)
	}

	emptyDashboard := RenderDashboard(emptyReport)
	if !strings.Contains(emptyDashboard, "table") || strings.Contains(emptyDashboard, "<tr style=\"border-bottom") {
		t.Fatalf("unexpected content in empty dashboard:\n%s", emptyDashboard)
	}
}
