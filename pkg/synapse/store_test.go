package synapse

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestSynapse(id string, p1, p2 string) *Synapse {
	return &Synapse{
		ID:    id,
		Name:  "Test " + id,
		State: StateActive,
		Members: []Member{
			{
				ProjectID: p1,
				Alias:     "producer-" + id,
				Role:      RoleProducer,
				PathHint:  "/path/" + id + "/producer",
			},
			{
				ProjectID: p2,
				Alias:     "consumer-" + id,
				Role:      RoleConsumer,
				PathHint:  "/path/" + id + "/consumer",
			},
		},
		Contracts: []Contract{
			{
				Kind:     "openapi",
				Producer: "producer-" + id,
				Path:     "api/openapi.yaml",
				Consumers: []Consumer{
					{Alias: "consumer-" + id, Path: "src/api/types.ts"},
				},
			},
		},
		Sync: Sync{
			Strategy: "go.work",
		},
	}
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	if st.Dir() != dir {
		t.Fatalf("expected st.Dir() to be %s, got %s", dir, st.Dir())
	}

	// Save nil synapse
	if err := st.Save(nil); err == nil {
		t.Fatalf("expected error saving nil synapse, got nil")
	}

	// Save invalid synapse
	badSynapse := &Synapse{ID: "invalid_id"}
	if err := st.Save(badSynapse); err == nil {
		t.Fatalf("expected error saving invalid synapse, got nil")
	}

	// Save valid synapse (CreatedAt is zero)
	s := createTestSynapse("web-api", "dpid_11111111111111111111111111111111", "dpid_22222222222222222222222222222222")
	if err := st.Save(s); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if s.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be initialized")
	}
	if s.UpdatedAt.IsZero() {
		t.Fatalf("expected UpdatedAt to be initialized")
	}
	createdAt := s.CreatedAt

	// Load synapse
	loaded, err := st.Load("web-api")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.ID != s.ID || loaded.Name != s.Name || loaded.State != s.State {
		t.Fatalf("loaded synapse does not match saved: %+v vs %+v", loaded, s)
	}
	if !loaded.CreatedAt.Equal(createdAt) {
		t.Fatalf("loaded CreatedAt mismatch: %v vs %v", loaded.CreatedAt, createdAt)
	}

	// Update synapse
	time.Sleep(10 * time.Millisecond)
	s.Name = "Updated Web API"
	if err := st.Save(s); err != nil {
		t.Fatalf("Save update failed: %v", err)
	}
	if !s.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt should not change on update: was %v, now %v", createdAt, s.CreatedAt)
	}

	loaded2, err := st.Load("web-api")
	if err != nil {
		t.Fatalf("Load after update failed: %v", err)
	}
	if loaded2.Name != "Updated Web API" {
		t.Fatalf("expected updated name 'Updated Web API', got %q", loaded2.Name)
	}

	// Delete synapse
	if err := st.Delete("web-api"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Load deleted synapse
	_, err = st.Load("web-api")
	if err == nil {
		t.Fatalf("expected ErrNotFound after delete, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Delete nonexistent synapse
	if err := st.Delete("web-api"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting non-existent synapse, got %v", err)
	}
}

func TestStoreTraversalRejection(t *testing.T) {
	st := NewStore(t.TempDir())

	badIDs := []string{
		"../escape",
		"foo/bar",
		"/root",
		"-hyphen",
		"UPPER",
		"",
	}

	for _, badID := range badIDs {
		_, err := st.Load(badID)
		if err == nil {
			t.Fatalf("expected Load error for traversal/bad id %q, got nil", badID)
		}
		if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("expected ErrInvalidID for Load(%q), got %v", badID, err)
		}

		err = st.Delete(badID)
		if err == nil {
			t.Fatalf("expected Delete error for traversal/bad id %q, got nil", badID)
		}
		if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("expected ErrInvalidID for Delete(%q), got %v", badID, err)
		}
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	// List on empty/non-existent dir
	emptyStore := NewStore(filepath.Join(dir, "does-not-exist"))
	list, err := emptyStore.List()
	if err != nil {
		t.Fatalf("List on non-existent dir failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d items", len(list))
	}

	// Save 3 synapses out of alphabetical order
	sZ := createTestSynapse("zeta-link", "dpid_11111111111111111111111111111111", "dpid_22222222222222222222222222222222")
	sA := createTestSynapse("alpha-link", "dpid_33333333333333333333333333333333", "dpid_44444444444444444444444444444444")
	sB := createTestSynapse("beta-link", "dpid_55555555555555555555555555555555", "dpid_66666666666666666666666666666666")

	if err := st.Save(sZ); err != nil {
		t.Fatalf("Save sZ failed: %v", err)
	}
	if err := st.Save(sA); err != nil {
		t.Fatalf("Save sA failed: %v", err)
	}
	if err := st.Save(sB); err != nil {
		t.Fatalf("Save sB failed: %v", err)
	}

	// Add extraneous non-.toml files and a subdirectory
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("notes"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to mkdir subdir: %v", err)
	}

	list, err = st.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 items in list, got %d", len(list))
	}
	if list[0].ID != "alpha-link" || list[1].ID != "beta-link" || list[2].ID != "zeta-link" {
		t.Fatalf("expected list sorted by ID [alpha-link, beta-link, zeta-link], got [%s, %s, %s]",
			list[0].ID, list[1].ID, list[2].ID)
	}

	// Add corrupted .toml file
	badFile := filepath.Join(dir, "corrupt.toml")
	if err := os.WriteFile(badFile, []byte("invalid = = toml"), 0644); err != nil {
		t.Fatalf("failed to write corrupt.toml: %v", err)
	}

	_, err = st.List()
	if err == nil {
		t.Fatalf("expected error from List when corrupt file exists, got nil")
	}
	if !strings.Contains(err.Error(), "corrupt.toml") {
		t.Fatalf("expected error to name 'corrupt.toml', got: %v", err)
	}
}

func TestStoreReferencingProject(t *testing.T) {
	st := NewStore(t.TempDir())

	pCommon := "dpid_00000000000000000000000000000000"
	p1 := "dpid_11111111111111111111111111111111"
	p2 := "dpid_22222222222222222222222222222222"
	p3 := "dpid_33333333333333333333333333333333"

	s1 := createTestSynapse("link-one", pCommon, p1)
	s2 := createTestSynapse("link-two", pCommon, p2)
	s3 := createTestSynapse("link-three", p2, p3)

	if err := st.Save(s1); err != nil {
		t.Fatalf("Save s1 failed: %v", err)
	}
	if err := st.Save(s2); err != nil {
		t.Fatalf("Save s2 failed: %v", err)
	}
	if err := st.Save(s3); err != nil {
		t.Fatalf("Save s3 failed: %v", err)
	}

	// pCommon is in link-one and link-two
	refs, err := st.ReferencingProject(pCommon)
	if err != nil {
		t.Fatalf("ReferencingProject failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 synapses referencing pCommon, got %d", len(refs))
	}
	if refs[0].ID != "link-one" || refs[1].ID != "link-two" {
		t.Fatalf("unexpected references: [%s, %s]", refs[0].ID, refs[1].ID)
	}

	// p1 is only in link-one
	refs1, err := st.ReferencingProject(p1)
	if err != nil {
		t.Fatalf("ReferencingProject failed: %v", err)
	}
	if len(refs1) != 1 || refs1[0].ID != "link-one" {
		t.Fatalf("expected [link-one], got %v", refs1)
	}

	// p3 is only in link-three
	refs3, err := st.ReferencingProject(p3)
	if err != nil {
		t.Fatalf("ReferencingProject failed: %v", err)
	}
	if len(refs3) != 1 || refs3[0].ID != "link-three" {
		t.Fatalf("expected [link-three], got %v", refs3)
	}

	// Non-existent project ID
	refsNone, err := st.ReferencingProject("dpid_99999999999999999999999999999999")
	if err != nil {
		t.Fatalf("ReferencingProject failed: %v", err)
	}
	if len(refsNone) != 0 {
		t.Fatalf("expected 0 references, got %d", len(refsNone))
	}
}

func TestDefaultStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	st, err := DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore failed: %v", err)
	}

	expectedDir := filepath.Join(home, DirName)
	if st.Dir() != expectedDir {
		t.Fatalf("expected default store dir %s, got %s", expectedDir, st.Dir())
	}
}
