package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

func TestEngramCmd_HelpAndUsageErrors(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer

	// Parent help with no args
	if err := runWith(ctx, &buf, newEngramCmd(), "engram"); err != nil {
		t.Fatalf("engram help failed: %v", err)
	}
	for _, want := range []string{"status", "inspect", "rm", "chunks", "query", "purge", "commit"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("engram help missing %q:\n%s", want, buf.String())
		}
	}

	// Alias help
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "store", "--help"); err != nil {
		t.Fatalf("store --help failed: %v", err)
	}

	// Usage errors: unknown subcommand, missing arg, missing required flag, bad flag, rm without --force
	usageCases := [][]string{
		{"engram", "bogus"},
		{"engram", "inspect"},
		{"engram", "rm"},
		{"engram", "rm", "some-id"},
		{"engram", "query"},
		{"engram", "purge"},
		{"engram", "commit", "--project", "p"},
		{"engram", "status", "--unknown-flag"},
	}
	for _, args := range usageCases {
		buf.Reset()
		err := runWith(ctx, &buf, newEngramCmd(), args...)
		if err == nil || !isUsageError(err) {
			t.Errorf("args %v: got err=%v, want usage error", args, err)
		}
	}
}

func TestEngramCmd_LifecycleAndShortHash(t *testing.T) {
	if got := shortHash("abc"); got != "abc" {
		t.Fatalf("shortHash('abc') = %q, want 'abc'", got)
	}
	if got := shortHash("0123456789abcdef"); got != "0123456789ab" {
		t.Fatalf("shortHash(16 chars) = %q, want '0123456789ab'", got)
	}

	// The segmented store lives under $HOME/.dendrite/store; keep it out of the real home.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())

	sampleFile := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(sampleFile, []byte("package sample\nfunc Hello() int { return 42 }\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	var buf bytes.Buffer

	// 1. commit
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "commit", "--project", "demo-proj", "--chunk", "chk-1", "--file", sampleFile, "--final"); err != nil {
		t.Fatalf("engram commit failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Committed chunk 'chk-1' for project 'demo-proj'") {
		t.Errorf("unexpected commit output: %s", buf.String())
	}

	// Insert a segment with a short hash (<12 chars) directly to verify inspect and query do not panic
	st, _, err := openSegmentedStore()
	if err != nil {
		t.Fatalf("openSegmentedStore: %v", err)
	}
	shortSeg := segmented.Segment{
		Hash:       "short",
		Type:       segmented.SegmentFunction,
		Identifier: "ShortFn",
		Project:    "demo-proj",
		SizeBytes:  7,
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := st.CommitChunk(ctx, "demo-proj", "chk-short", []segmented.Segment{shortSeg}, map[string][]byte{"short": []byte("content")}, true); err != nil {
		t.Fatalf("CommitChunk short hash: %v", err)
	}

	// 2. status (human + JSON)
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "status", "--project", "demo-proj"); err != nil {
		t.Fatalf("engram status failed: %v", err)
	}
	if !strings.Contains(buf.String(), "demo-proj") {
		t.Errorf("status output missing demo-proj: %s", buf.String())
	}

	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "status", "--project", "demo-proj", "--json"); err != nil {
		t.Fatalf("engram status --json failed: %v", err)
	}
	var statusRep status.StoreStatusReport
	if err := json.Unmarshal(buf.Bytes(), &statusRep); err != nil {
		t.Fatalf("unmarshal status JSON: %v\n%s", err, buf.String())
	}
	if statusRep.TotalStores != 1 {
		t.Errorf("TotalStores = %d, want 1", statusRep.TotalStores)
	}

	// 3. inspect (human + JSON)
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "inspect", "demo-proj"); err != nil {
		t.Fatalf("engram inspect failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Store Analysis Details: demo-proj") || !strings.Contains(buf.String(), "short") {
		t.Errorf("inspect output missing expected details:\n%s", buf.String())
	}

	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "inspect", "demo-proj", "--json"); err != nil {
		t.Fatalf("engram inspect --json failed: %v", err)
	}
	var detailRep status.StoreDetailReport
	if err := json.Unmarshal(buf.Bytes(), &detailRep); err != nil {
		t.Fatalf("unmarshal inspect JSON: %v", err)
	}
	if detailRep.Summary.StoreID != "demo-proj" {
		t.Errorf("Summary.StoreID = %q, want demo-proj", detailRep.Summary.StoreID)
	}

	// 4. chunks (human + JSON via alias checkpoints)
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "chunks", "--project", "demo-proj"); err != nil {
		t.Fatalf("engram chunks failed: %v", err)
	}
	if !strings.Contains(buf.String(), "chk-1") {
		t.Errorf("chunks output missing chk-1: %s", buf.String())
	}

	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "checkpoints", "--project", "demo-proj", "--json"); err != nil {
		t.Fatalf("engram checkpoints --json failed: %v", err)
	}
	var cps []segmented.ChunkCheckpoint
	if err := json.Unmarshal(buf.Bytes(), &cps); err != nil || len(cps) != 2 {
		t.Fatalf("checkpoints JSON: len=%d, err=%v", len(cps), err)
	}

	// 5. query (human + JSON)
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "query", "--project", "demo-proj"); err != nil {
		t.Fatalf("engram query failed: %v", err)
	}
	if !strings.Contains(buf.String(), "ShortFn") {
		t.Errorf("query output missing ShortFn: %s", buf.String())
	}

	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "query", "--project", "demo-proj", "--json"); err != nil {
		t.Fatalf("engram query --json failed: %v", err)
	}
	var segs []segmented.Segment
	if err := json.Unmarshal(buf.Bytes(), &segs); err != nil || len(segs) != 2 {
		t.Fatalf("query JSON: len=%d, err=%v", len(segs), err)
	}

	// 6. rm with --force
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "rm", "demo-proj", "--force"); err != nil {
		t.Fatalf("engram rm --force failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Safely removed store 'demo-proj'") {
		t.Errorf("rm output missing confirmation: %s", buf.String())
	}

	// 7. Re-commit and remove with global --yes, then re-commit and purge
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "commit", "--project", "demo-proj", "--chunk", "chk-2", "--file", sampleFile); err != nil {
		t.Fatalf("re-commit failed: %v", err)
	}
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "--yes", "engram", "remove", "demo-proj"); err != nil {
		t.Fatalf("engram remove --yes failed: %v", err)
	}

	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "commit", "--project", "demo-proj", "--chunk", "chk-3", "--file", sampleFile); err != nil {
		t.Fatalf("re-commit 2 failed: %v", err)
	}
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "purge", "--project", "demo-proj"); err != nil {
		t.Fatalf("engram purge failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Explicitly purged all segmented analysis data for project: demo-proj") {
		t.Errorf("unexpected purge output: %s", buf.String())
	}

	// 8. Lazy open error when $HOME/.dendrite is a file instead of a directory
	badHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(badHome, ".dendrite"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile bad .dendrite: %v", err)
	}
	t.Setenv("HOME", badHome)
	buf.Reset()
	if err := runWith(ctx, &buf, newEngramCmd(), "engram", "status"); err == nil {
		t.Errorf("expected error when segmented store cannot be opened, got nil")
	}
}
