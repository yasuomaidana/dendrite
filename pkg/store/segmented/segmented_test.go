package segmented

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBlobStore_LifecycleAndIntegrity(t *testing.T) {
	tempDir := t.TempDir()
	bs := NewBlobStore(filepath.Join(tempDir, "blobs"))

	content := []byte(strings.Repeat("Dendrite Content Addressed Segmented AST Blob Data\n", 50))
	hash := bs.ComputeHash(content)

	if bs.HasBlob(hash) {
		t.Fatal("expected HasBlob to be false before write")
	}

	// Write blob
	blobPath, compSize, err := bs.WriteBlob(hash, content)
	if err != nil {
		t.Fatalf("WriteBlob failed: %v", err)
	}
	if compSize >= int64(len(content)) {
		t.Fatalf("expected compression to reduce size: compSize=%d, rawSize=%d", compSize, len(content))
	}
	if !bs.HasBlob(hash) {
		t.Fatal("expected HasBlob to be true after write")
	}

	// Read blob
	readData, err := bs.ReadBlob(hash)
	if err != nil {
		t.Fatalf("ReadBlob failed: %v", err)
	}
	if string(readData) != string(content) {
		t.Fatal("decompressed content does not match original")
	}

	// Re-writing same blob should succeed idempotently
	_, _, err = bs.WriteBlob(hash, content)
	if err != nil {
		t.Fatalf("idempotent WriteBlob failed: %v", err)
	}

	// Corrupt blob on disk
	_ = os.WriteFile(blobPath, []byte("corrupted invalid blob data"), 0644)
	_, err = bs.ReadBlob(hash)
	if err == nil || !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected ErrCorruptedBlob on damaged data, got %v", err)
	}

	// Read non-existent blob
	_, err = bs.ReadBlob("0000000000000000000000000000000000000000000000000000000000000000")
	if err != ErrSegmentNotFound {
		t.Fatalf("expected ErrSegmentNotFound, got %v", err)
	}
}

func TestSegmentedStore_IncrementalChunkCheckpointing(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewSegmentedStore(tempDir)
	if err != nil {
		t.Fatalf("NewSegmentedStore failed: %v", err)
	}

	ctx := context.Background()
	project := "carla-engine"

	// 1. Commit chunk 1 (Partial)
	seg1Data := []byte("func ParseSensorPacket() { ... }")
	seg1Hash := store.blobs.ComputeHash(seg1Data)
	seg1 := Segment{
		Hash:       seg1Hash,
		Type:       SegmentFunction,
		Identifier: "ParseSensorPacket",
		Project:    project,
		CreatedAt:  time.Now().UTC(),
		Metadata:   map[string]interface{}{"entrypoint": true},
	}

	cp1, err := store.CommitChunk(ctx, project, "chunk-001", []Segment{seg1}, map[string][]byte{seg1Hash: seg1Data}, false)
	if err != nil {
		t.Fatalf("CommitChunk 1 failed: %v", err)
	}

	if cp1.State != StateAnalyzingPartial {
		t.Fatalf("expected StateAnalyzingPartial, got %s", cp1.State)
	}

	// 2. Verify query on partial analysis returns committed segments immediately
	partialSegs, err := store.QueryPartial(ctx, project)
	if err != nil {
		t.Fatalf("QueryPartial failed: %v", err)
	}
	if len(partialSegs) != 1 {
		t.Fatalf("expected 1 partial segment, got %d", len(partialSegs))
	}
	if partialSegs[0].Identifier != "ParseSensorPacket" {
		t.Fatalf("expected ParseSensorPacket, got %s", partialSegs[0].Identifier)
	}

	// Verify segment blob data retrieval
	data, err := store.GetSegmentData(ctx, seg1Hash)
	if err != nil {
		t.Fatalf("GetSegmentData failed: %v", err)
	}
	if string(data) != string(seg1Data) {
		t.Fatalf("expected %s, got %s", string(seg1Data), string(data))
	}

	// 3. Commit chunk 2 (Final)
	seg2Data := []byte("func RenderFrame() { ... }")
	seg2Hash := store.blobs.ComputeHash(seg2Data)
	seg2 := Segment{
		Hash:       seg2Hash,
		Type:       SegmentFunction,
		Identifier: "RenderFrame",
		Project:    project,
		CreatedAt:  time.Now().UTC(),
	}

	cp2, err := store.CommitChunk(ctx, project, "chunk-002", []Segment{seg2}, map[string][]byte{seg2Hash: seg2Data}, true)
	if err != nil {
		t.Fatalf("CommitChunk 2 failed: %v", err)
	}

	if cp2.State != StateStored {
		t.Fatalf("expected StateStored, got %s", cp2.State)
	}

	// Verify GetCheckpoint
	fetchedCP, err := store.GetCheckpoint(ctx, "chunk-001")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if fetchedCP.State != StateAnalyzingPartial {
		t.Fatalf("expected StateAnalyzingPartial, got %s", fetchedCP.State)
	}

	// 4. Verify all checkpoints
	checkpoints, err := store.ListCheckpoints(ctx, project)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(checkpoints))
	}

	// 5. Test empty chunk error
	_, err = store.CommitChunk(ctx, project, "chunk-empty", nil, nil, false)
	if err != ErrEmptyChunk {
		t.Fatalf("expected ErrEmptyChunk, got %v", err)
	}

	// 6. Test PurgeProject explicitly deletes records
	if err := store.PurgeProject(ctx, project); err != nil {
		t.Fatalf("PurgeProject failed: %v", err)
	}

	postPurgeSegs, err := store.QueryPartial(ctx, project)
	if err != nil {
		t.Fatalf("QueryPartial post purge failed: %v", err)
	}
	if len(postPurgeSegs) != 0 {
		t.Fatalf("expected 0 segments post purge, got %d", len(postPurgeSegs))
	}
}

func TestSegmentedStore_CrashMidWriteZeroCorruption(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewSegmentedStore(tempDir)
	if err != nil {
		t.Fatalf("NewSegmentedStore failed: %v", err)
	}

	ctx := context.Background()
	project := "crash-resilient-proj"

	// Initial valid commit
	initData := []byte("initial valid segment")
	initHash := store.blobs.ComputeHash(initData)
	initSeg := Segment{
		Hash:       initHash,
		Type:       SegmentFile,
		Identifier: "init.go",
		Project:    project,
	}

	_, err = store.CommitChunk(ctx, project, "chunk-init", []Segment{initSeg}, map[string][]byte{initHash: initData}, false)
	if err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}

	// Simulate crash mid-write
	crashBlobs := map[string][]byte{
		"deadbeef12345678": []byte("incomplete uncommitted data"),
	}
	err = store.SimulateCrashMidWrite(ctx, project, "chunk-crashed", crashBlobs)
	if err == nil {
		t.Fatal("expected simulated crash error")
	}

	// Verify database is completely uncorrupted and only contains initial commit
	segs, err := store.QueryPartial(ctx, project)
	if err != nil {
		t.Fatalf("QueryPartial after crash failed: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected exactly 1 valid segment after crash, got %d", len(segs))
	}
	if segs[0].Hash != initHash {
		t.Fatalf("expected valid initial hash %s, got %s", initHash, segs[0].Hash)
	}

	// Verify uncommitted chunk does not exist in checkpoints
	_, err = store.GetCheckpoint(ctx, "chunk-crashed")
	if err != ErrCheckpointNotFound {
		t.Fatalf("expected ErrCheckpointNotFound for crashed chunk, got %v", err)
	}
}

func TestSegmentedStore_DefaultsAndListing(t *testing.T) {
	// Test default constructors
	bs := NewBlobStore("")
	if bs.baseDir != ".dendrite/store/blobs" {
		t.Fatalf("unexpected blob default: %s", bs.baseDir)
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "meta.db")
	idx, err := NewSQLiteIndex(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteIndex failed: %v", err)
	}

	// Query empty database
	allSegs, err := idx.ListSegments("")
	if err != nil {
		t.Fatalf("ListSegments empty failed: %v", err)
	}
	if len(allSegs) != 0 {
		t.Fatalf("expected 0 segments in empty db, got %d", len(allSegs))
	}

	allCheckpoints, err := idx.ListCheckpoints("")
	if err != nil {
		t.Fatalf("ListCheckpoints empty failed: %v", err)
	}
	if len(allCheckpoints) != 0 {
		t.Fatalf("expected 0 checkpoints in empty db, got %d", len(allCheckpoints))
	}

	// GetCheckpoint not found
	_, err = idx.GetCheckpoint("nonexistent")
	if err != ErrCheckpointNotFound {
		t.Fatalf("expected ErrCheckpointNotFound, got %v", err)
	}

	// GetSegment not found
	_, err = idx.GetSegment("nonexistent_hash")
	if err != ErrSegmentNotFound {
		t.Fatalf("expected ErrSegmentNotFound, got %v", err)
	}

	// Save one segment and list all
	seg := Segment{
		Hash:       "def456hash",
		Type:       SegmentFile,
		Identifier: "lib.rs",
		Project:    "crate-a",
		SizeBytes:  256,
		CreatedAt:  time.Now().UTC(),
	}
	cp := ChunkCheckpoint{
		ChunkID:           "chk-all",
		Project:           "crate-a",
		State:             StateStored,
		TotalSegments:     1,
		CommittedSegments: 1,
		UpdatedAt:         time.Now().UTC(),
	}
	if err := idx.SaveCheckpointTx(cp, []Segment{seg}); err != nil {
		t.Fatalf("SaveCheckpointTx failed: %v", err)
	}

	// GetSegment found
	fetched, err := idx.GetSegment("def456hash")
	if err != nil || fetched.Identifier != "lib.rs" {
		t.Fatalf("expected lib.rs, got %v (err: %v)", fetched, err)
	}

	// List all across projects
	allSegs, err = idx.ListSegments("")
	if err != nil || len(allSegs) != 1 {
		t.Fatalf("expected 1 segment from ListSegments(''), got %d (err: %v)", len(allSegs), err)
	}

	allCheckpoints, err = idx.ListCheckpoints("")
	if err != nil || len(allCheckpoints) != 1 {
		t.Fatalf("expected 1 checkpoint from ListCheckpoints(''), got %d (err: %v)", len(allCheckpoints), err)
	}
}
