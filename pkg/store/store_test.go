package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_KeyValueAndHashing(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "store")
	adrDir := filepath.Join(tempDir, "adr")

	s := NewStore(baseDir, adrDir)

	data := []byte("hello dendrite store")
	hash := s.ComputeHash(data)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify Has before Put
	if s.Has("test-key") {
		t.Fatal("expected Has to be false before Put")
	}

	// Verify Get before Put
	_, err := s.Get("test-key")
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}

	// Put data
	if err := s.Put("test-key", data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify Has after Put
	if !s.Has("test-key") {
		t.Fatal("expected Has to be true after Put")
	}

	// Get data
	retrieved, err := s.Get("test-key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retrieved) != string(data) {
		t.Fatalf("expected %s, got %s", string(data), string(retrieved))
	}
}

func TestStore_ADRLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "store")
	adrDir := filepath.Join(tempDir, "docs", "adr")

	s := NewStore(baseDir, adrDir)

	// Invalid ADR validation
	_, err := s.SaveADR(&ADR{Title: ""})
	if err != ErrInvalidADR {
		t.Fatalf("expected ErrInvalidADR, got %v", err)
	}

	// Save first ADR
	adr1 := &ADR{
		TicketID:     "DEN-058",
		Title:        "Use nalgebra for Linear Algebra Computations",
		TierSelected: "Tier 3: Ecosystem Registry",
		PackageName:  "nalgebra",
		SourceURI:    "https://crates.io/crates/nalgebra",
		Rationale:    "Standard crates.io linear algebra crate",
		Context:      "Auditing matrix operations",
		Evaluation:   "Tier 1: none; Tier 2: none; Tier 3: nalgebra",
		Consequences: "High performance math",
	}

	path1, err := s.SaveADR(adr1)
	if err != nil {
		t.Fatalf("SaveADR failed: %v", err)
	}
	if adr1.ID != "ADR-0001" {
		t.Fatalf("expected ADR-0001, got %s", adr1.ID)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Fatalf("ADR file does not exist at %s: %v", path1, err)
	}

	// Save second ADR
	adr2 := &ADR{
		TicketID:     "DEN-058",
		Title:        "Adopt Ent for Database Schema",
		TierSelected: "Tier 3: Ecosystem Registry",
		PackageName:  "ent",
		SourceURI:    "https://pkg.go.dev/entgo.io/ent",
		Rationale:    "Graph-based ORM for Go",
	}

	_, err = s.SaveADR(adr2)
	if err != nil {
		t.Fatalf("SaveADR 2 failed: %v", err)
	}
	if adr2.ID != "ADR-0002" {
		t.Fatalf("expected ADR-0002, got %s", adr2.ID)
	}

	// List ADRs
	list, err := s.ListADRs()
	if err != nil {
		t.Fatalf("ListADRs failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 ADRs, got %d", len(list))
	}

	// Get ADR by ID
	fetched, err := s.GetADR("ADR-0001")
	if err != nil {
		t.Fatalf("GetADR failed: %v", err)
	}
	if fetched.Title != adr1.Title {
		t.Fatalf("expected title %s, got %s", adr1.Title, fetched.Title)
	}

	// Get non-existent ADR
	_, err = s.GetADR("ADR-9999")
	if err != ErrADRNotFound {
		t.Fatalf("expected ErrADRNotFound, got %v", err)
	}
}

func TestStore_DefaultPaths(t *testing.T) {
	s := NewStore("", "")
	if s.baseDir != ".dendrite/store" || s.adrDir != "docs/adr" {
		t.Fatalf("unexpected defaults: baseDir=%s, adrDir=%s", s.baseDir, s.adrDir)
	}
}

func TestStore_ParseADRMalformed(t *testing.T) {
	tempDir := t.TempDir()
	badFile := filepath.Join(tempDir, "bad.md")
	_ = os.WriteFile(badFile, []byte("no frontmatter here"), 0644)

	_, err := parseADRFile(badFile)
	if err == nil {
		t.Fatal("expected error for malformed ADR file")
	}

	badFile2 := filepath.Join(tempDir, "bad2.md")
	_ = os.WriteFile(badFile2, []byte("---\nincomplete frontmatter"), 0644)
	_, err = parseADRFile(badFile2)
	if err == nil {
		t.Fatal("expected error for incomplete frontmatter")
	}
}
