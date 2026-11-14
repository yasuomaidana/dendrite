package reuse

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/store"
)

func TestAuditor_EmptyCapability(t *testing.T) {
	tempDir := t.TempDir()
	s := store.NewStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "adr"))
	auditor := NewAuditor(s, nil, "")

	_, err := auditor.AuditReuse(context.Background(), AuditRequest{Capability: ""})
	if err != ErrEmptyCapability {
		t.Fatalf("expected ErrEmptyCapability, got %v", err)
	}
}

func TestAuditor_Tier1SourceReference(t *testing.T) {
	tempDir := t.TempDir()
	s := store.NewStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "adr"))
	refMgr := NewRefManager(filepath.Join(tempDir, "refs.json"))
	auditor := NewAuditor(s, refMgr, "")

	// "kinematics" matches carla-sensor-rs in default references
	req := AuditRequest{
		Capability: "kinematics",
		Language:   "rust",
		TicketID:   "DEN-058",
		Intent:     "Vehicle kinematic matrix transforms",
	}

	res, err := auditor.AuditReuse(context.Background(), req)
	if err != nil {
		t.Fatalf("AuditReuse failed: %v", err)
	}

	if res.TierSelected != Tier1SourceRef {
		t.Fatalf("expected %s, got %s", Tier1SourceRef, res.TierSelected)
	}
	if res.CandidateName != "carla-sensor-rs" {
		t.Fatalf("expected carla-sensor-rs, got %s", res.CandidateName)
	}
	if res.Action != "slice_and_import" {
		t.Fatalf("expected slice_and_import, got %s", res.Action)
	}
	if res.ADRPath == "" || res.ADRID == "" {
		t.Fatal("expected non-empty ADRPath and ADRID")
	}

	// Verify ADR was written
	if _, err := os.Stat(res.ADRPath); err != nil {
		t.Fatalf("ADR file does not exist: %v", err)
	}
}

func TestAuditor_Tier2PoolMember(t *testing.T) {
	tempDir := t.TempDir()
	s := store.NewStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "adr"))

	// Create a mock pool.json
	poolPath := filepath.Join(tempDir, "pool.json")
	mockPool := map[string]interface{}{
		"projects": []map[string]string{
			{"name": "spatial-math", "path": "./spatial-math", "ecosystem": "rust"},
		},
	}
	data, _ := json.Marshal(mockPool)
	_ = os.WriteFile(poolPath, data, 0644)

	// Create empty refs manager so Tier 1 doesn't match
	refsPath := filepath.Join(tempDir, "refs.json")
	_ = os.WriteFile(refsPath, []byte("[]"), 0644)
	refMgr := NewRefManager(refsPath)

	auditor := NewAuditor(s, refMgr, poolPath)

	req := AuditRequest{
		Capability: "spatial-math",
		Language:   "rust",
		TicketID:   "DEN-058",
		Intent:     "Need spatial math operations",
	}

	res, err := auditor.AuditReuse(context.Background(), req)
	if err != nil {
		t.Fatalf("AuditReuse failed: %v", err)
	}

	if res.TierSelected != Tier2PoolMember {
		t.Fatalf("expected %s, got %s", Tier2PoolMember, res.TierSelected)
	}
	if res.CandidateName != "spatial-math" {
		t.Fatalf("expected spatial-math, got %s", res.CandidateName)
	}
	if res.Action != "wire_pool_edge" {
		t.Fatalf("expected wire_pool_edge, got %s", res.Action)
	}
}

func TestAuditor_Tier3EcosystemRegistry(t *testing.T) {
	tempDir := t.TempDir()
	s := store.NewStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "adr"))
	refsPath := filepath.Join(tempDir, "refs.json")
	_ = os.WriteFile(refsPath, []byte("[]"), 0644)
	refMgr := NewRefManager(refsPath)
	auditor := NewAuditor(s, refMgr, filepath.Join(tempDir, "nonexistent-pool.json"))

	tests := []struct {
		name        string
		capability  string
		language    string
		expectedPkg string
	}{
		{"Rust nalgebra", "matrix", "rust", "nalgebra"},
		{"Rust serde", "serialization", "rust", "serde"},
		{"Rust diesel", "database", "rust", "diesel"},
		{"Go ent", "database", "go", "ent"},
		{"Go gin", "routing", "go", "gin"},
		{"TypeScript three", "graphics", "typescript", "three"},
		{"TypeScript zod", "schema", "typescript", "zod"},
		{"Python numpy", "matrix", "python", "numpy"},
		{"Python pydantic", "validation", "python", "pydantic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := auditor.AuditReuse(context.Background(), AuditRequest{
				Capability: tt.capability,
				Language:   tt.language,
				TicketID:   "DEN-058",
			})
			if err != nil {
				t.Fatalf("AuditReuse failed: %v", err)
			}
			if res.TierSelected != Tier3Registry {
				t.Fatalf("expected %s, got %s", Tier3Registry, res.TierSelected)
			}
			if res.CandidateName != tt.expectedPkg {
				t.Fatalf("expected package %s, got %s", tt.expectedPkg, res.CandidateName)
			}
			if res.Action != "add_registry_dep" {
				t.Fatalf("expected add_registry_dep, got %s", res.Action)
			}
		})
	}
}

func TestAuditor_UnresolvedFallback(t *testing.T) {
	tempDir := t.TempDir()
	s := store.NewStore(filepath.Join(tempDir, "store"), filepath.Join(tempDir, "adr"))
	refsPath := filepath.Join(tempDir, "refs.json")
	_ = os.WriteFile(refsPath, []byte("[]"), 0644)
	refMgr := NewRefManager(refsPath)
	auditor := NewAuditor(s, refMgr, filepath.Join(tempDir, "nonexistent-pool.json"))

	req := AuditRequest{
		Capability: "quantum_flux_transistor",
		Language:   "rust",
		TicketID:   "DEN-058",
	}

	res, err := auditor.AuditReuse(context.Background(), req)
	if err != nil {
		t.Fatalf("AuditReuse failed: %v", err)
	}

	if res.TierSelected != TierUnresolved {
		t.Fatalf("expected %s, got %s", TierUnresolved, res.TierSelected)
	}
	if res.Action != "synthesize_new" {
		t.Fatalf("expected synthesize_new, got %s", res.Action)
	}
}

func TestRefManager_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	refsPath := filepath.Join(tempDir, "refs.json")
	refMgr := NewRefManager(refsPath)

	// List initial default refs
	refs, err := refMgr.ListRefs()
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) < 3 {
		t.Fatalf("expected at least 3 default refs, got %d", len(refs))
	}

	// Add new ref
	newRef := SourceReference{
		ID:           "ref-custom-math",
		Name:         "custom-math",
		Repository:   "https://github.com/test/custom-math",
		Language:     "rust",
		Capabilities: []string{"custom_blas"},
		License:      "MIT",
	}
	if err := refMgr.AddRef(newRef); err != nil {
		t.Fatalf("AddRef failed: %v", err)
	}

	// Duplicate add error
	if err := refMgr.AddRef(newRef); err != ErrDuplicateReference {
		t.Fatalf("expected ErrDuplicateReference, got %v", err)
	}

	// Remove ref
	if err := refMgr.RemoveRef("ref-custom-math"); err != nil {
		t.Fatalf("RemoveRef failed: %v", err)
	}

	// Remove non-existent
	if err := refMgr.RemoveRef("ref-nonexistent"); err != ErrReferenceNotFound {
		t.Fatalf("expected ErrReferenceNotFound, got %v", err)
	}
}
