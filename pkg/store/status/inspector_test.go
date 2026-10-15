package status

import (
	"context"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/store/segmented"
)

func TestStoreInspector_LifecycleAndRemoval(t *testing.T) {
	tempDir := t.TempDir()
	store, err := segmented.NewSegmentedStore(tempDir)
	if err != nil {
		t.Fatalf("NewSegmentedStore failed: %v", err)
	}

	inspector := NewStoreInspector(store)
	ctx := context.Background()

	// Initial inspect empty
	report, err := inspector.InspectStore(ctx, "")
	if err != nil {
		t.Fatalf("InspectStore empty failed: %v", err)
	}
	if report.TotalStores != 0 {
		t.Fatalf("expected 0 stores, got %d", report.TotalStores)
	}

	// Commit project 1 (Partial)
	proj1 := "proj-alpha"
	data1 := []byte("function AlphaCompute() { return 42; }")
	hash1 := store.ComputeHash(data1)
	seg1 := segmented.Segment{
		Hash:       hash1,
		Type:       segmented.SegmentFunction,
		Identifier: "AlphaCompute",
		Project:    proj1,
		CreatedAt:  time.Now().UTC(),
	}
	_, err = store.CommitChunk(ctx, proj1, "chk-a1", []segmented.Segment{seg1}, map[string][]byte{hash1: data1}, false)
	if err != nil {
		t.Fatalf("CommitChunk proj1 failed: %v", err)
	}

	// Commit project 2 (Stored / Complete with binary CFG block)
	proj2 := "proj-beta"
	data2 := []byte("block_0x401000: mov eax, 1; ret")
	hash2 := store.ComputeHash(data2)
	seg2 := segmented.Segment{
		Hash:       hash2,
		Type:       segmented.SegmentCFGBlock,
		Identifier: "block_0x401000",
		Project:    proj2,
		CreatedAt:  time.Now().UTC(),
	}
	_, err = store.CommitChunk(ctx, proj2, "chk-b1", []segmented.Segment{seg2}, map[string][]byte{hash2: data2}, true)
	if err != nil {
		t.Fatalf("CommitChunk proj2 failed: %v", err)
	}

	// Inspect all
	report, err = inspector.InspectStore(ctx, "")
	if err != nil {
		t.Fatalf("InspectStore failed: %v", err)
	}
	if report.TotalStores != 2 {
		t.Fatalf("expected 2 stores, got %d", report.TotalStores)
	}
	if report.StoredStores != 1 || report.PartialStores != 1 {
		t.Fatalf("expected 1 stored and 1 partial, got stored=%d partial=%d", report.StoredStores, report.PartialStores)
	}

	// Verify tabular formatting
	table := FormatTable(report)
	if !strings.Contains(table, "STORE ID") || !strings.Contains(table, "proj-alpha") || !strings.Contains(table, "proj-beta") {
		t.Fatalf("table missing expected columns or project rows:\n%s", table)
	}

	// Inspect details for proj1
	details, err := inspector.InspectStoreDetails(ctx, proj1)
	if err != nil {
		t.Fatalf("InspectStoreDetails failed: %v", err)
	}
	if details.Summary.State != segmented.StateAnalyzingPartial {
		t.Fatalf("expected StateAnalyzingPartial, got %s", details.Summary.State)
	}
	if len(details.Checkpoints) != 1 || len(details.Segments) != 1 {
		t.Fatalf("expected 1 checkpoint and 1 segment, got %d and %d", len(details.Checkpoints), len(details.Segments))
	}

	// Inspect details for non-existent
	_, err = inspector.InspectStoreDetails(ctx, "nonexistent-proj")
	if err != ErrStoreNotFound {
		t.Fatalf("expected ErrStoreNotFound, got %v", err)
	}

	// Remove without force -> error confirmation required
	_, err = inspector.RemoveStore(ctx, proj1, false)
	if err != ErrConfirmationRequired {
		t.Fatalf("expected ErrConfirmationRequired, got %v", err)
	}

	// Remove with force -> success
	res, err := inspector.RemoveStore(ctx, proj1, true)
	if err != nil {
		t.Fatalf("RemoveStore with force failed: %v", err)
	}
	if !res.Success || res.SegmentsPurged != 1 {
		t.Fatalf("unexpected removal result: %+v", res)
	}

	// Remove non-existent -> error
	_, err = inspector.RemoveStore(ctx, "nonexistent-proj", true)
	if err != ErrStoreNotFound {
		t.Fatalf("expected ErrStoreNotFound, got %v", err)
	}
}
