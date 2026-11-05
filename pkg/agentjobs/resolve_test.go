package agentjobs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

func TestTicketPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DEN-1048", "DEN"},
		{"DRAG-30", "DRAG"},
		{"drag-30", "DRAG"},
		{"FOO_BAR-99", "FOO"},
		{"PROJ", "PROJ"},
		{"[DEN-123]", "DEN"},
		{"[drag-42]", "DRAG"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := TicketPrefix(tt.input)
			if got != tt.want {
				t.Errorf("TicketPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTicketFromJobID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DRAG-30-20261021T180000Z-a1b2c3", "DRAG-30"},
		{"DEN-988-20261017T180000Z-abcd12", "DEN-988"},
		{"FEATURE-FOO-12-20261017T180000Z-123456", "FEATURE-FOO-12"},
		{"JOB-123", "JOB-123"},
		{"plain-id", "plain-id"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := TicketFromJobID(tt.input)
			if got != tt.want {
				t.Errorf("TicketFromJobID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLedger_FindByTicketAndLatest(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	now := time.Now().UTC()

	j1, err := l.Create(&Job{Ticket: "DEN-1", Agent: "agy", CreatedAt: now.Add(-10 * time.Minute)})
	if err != nil {
		t.Fatalf("Create j1: %v", err)
	}
	j2, err := l.Create(&Job{Ticket: "DEN-1", Agent: "agy", CreatedAt: now.Add(-5 * time.Minute)})
	if err != nil {
		t.Fatalf("Create j2: %v", err)
	}
	j3, err := l.Create(&Job{Ticket: "DEN-2", Agent: "agy", CreatedAt: now})
	if err != nil {
		t.Fatalf("Create j3: %v", err)
	}
	_ = j3

	// FindByTicket exact case
	matches, err := l.FindByTicket("DEN-1")
	if err != nil {
		t.Fatalf("FindByTicket: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	if matches[0].ID != j1.ID || matches[1].ID != j2.ID {
		t.Errorf("matches = [%s, %s], want [%s, %s]", matches[0].ID, matches[1].ID, j1.ID, j2.ID)
	}

	// FindByTicket case-insensitive
	matchesLower, err := l.FindByTicket("den-1")
	if err != nil {
		t.Fatalf("FindByTicket lower: %v", err)
	}
	if len(matchesLower) != 2 {
		t.Fatalf("len(matchesLower) = %d, want 2", len(matchesLower))
	}

	// LatestByTicket
	latest, err := l.LatestByTicket("DEN-1")
	if err != nil {
		t.Fatalf("LatestByTicket: %v", err)
	}
	if latest == nil || latest.ID != j2.ID {
		t.Fatalf("LatestByTicket = %v, want %s", latest, j2.ID)
	}

	// Non-existent ticket
	nonExistent, err := l.FindByTicket("NONEXISTENT-99")
	if err != nil {
		t.Fatalf("FindByTicket non-existent err: %v", err)
	}
	if len(nonExistent) != 0 {
		t.Errorf("len(nonExistent) = %d, want 0", len(nonExistent))
	}

	latestNone, err := l.LatestByTicket("NONEXISTENT-99")
	if err != nil {
		t.Fatalf("LatestByTicket non-existent err: %v", err)
	}
	if latestNone != nil {
		t.Errorf("LatestByTicket non-existent = %v, want nil", latestNone)
	}

	// Nil ledger
	var nilLedger *Ledger
	if _, err := nilLedger.FindByTicket("DEN-1"); err == nil {
		t.Error("nilLedger.FindByTicket expected error, got nil")
	}
}

func TestResolveTicketLedger_CurrentProjectMatches(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	job, err := l.Create(&Job{Ticket: "DEN-100", Agent: "agy"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resLedger, matches, err := ResolveTicketLedger(l, "DEN-100")
	if err != nil {
		t.Fatalf("ResolveTicketLedger: %v", err)
	}
	if resLedger.Root() != l.Root() {
		t.Errorf("resLedger.Root() = %q, want %q", resLedger.Root(), l.Root())
	}
	if len(matches) != 1 || matches[0].ID != job.ID {
		t.Errorf("matches = %v, want [%s]", matches, job.ID)
	}
}

func TestResolveTicketLedger_OtherProjectViaGlobalIndex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	// Create registered project directory for Dragon
	dragonDir := t.TempDir()
	planeJSON := `{"identifier":"DRAG","project_id":"drag-proj-uuid","name":"Dragon"}`
	if err := os.MkdirAll(filepath.Join(dragonDir, ".dendrite"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dragonDir, ".dendrite", "plane.json"), []byte(planeJSON), 0644); err != nil {
		t.Fatalf("Write plane.json: %v", err)
	}

	// Register in DENDRITE_HOME/projects.json
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatalf("DefaultIndexPath: %v", err)
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	idx.Upsert(projectid.Entry{
		ID:   "dpid_dragon",
		Path: dragonDir,
		Name: "Dragon",
	})
	if err := idx.Save(); err != nil {
		t.Fatalf("Save index: %v", err)
	}

	// Create job for DRAG-30 in Dragon's ledger
	dragonLedger := NewLedger(filepath.Join(dragonDir, ".dendrite", "jobs"))
	job, err := dragonLedger.Create(&Job{Ticket: "DRAG-30", Agent: "agy", Status: StatusRunning})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}

	// Current ledger is an empty directory (representing Dendrite checkout)
	currentDir := t.TempDir()
	currentLedger := NewLedger(currentDir)

	resLedger, matches, err := ResolveTicketLedger(currentLedger, "DRAG-30")
	if err != nil {
		t.Fatalf("ResolveTicketLedger: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != job.ID {
		t.Fatalf("matches = %v, want [%s]", matches, job.ID)
	}

	expectedRoot := filepath.Join(dragonDir, ".dendrite", "jobs")
	resRoot, _ := filepath.EvalSymlinks(resLedger.Root())
	expRoot, _ := filepath.EvalSymlinks(expectedRoot)
	if resRoot != expRoot {
		t.Errorf("resLedger.Root() = %q, want %q", resRoot, expRoot)
	}
}

func TestResolveTicketLedger_OtherProjectViaSynapse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	projDir := t.TempDir()
	planeJSON := `{"identifier":"PEER","project_id":"peer-proj-uuid"}`
	if err := os.MkdirAll(filepath.Join(projDir, ".dendrite"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".dendrite", "plane.json"), []byte(planeJSON), 0644); err != nil {
		t.Fatalf("Write plane.json: %v", err)
	}

	// Save synapse manifest pointing to projDir
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore: %v", err)
	}
	syn := &synapse.Synapse{
		ID:    "syn-test",
		Name:  "Test Synapse",
		State: synapse.StateActive,
		Members: []synapse.Member{
			{ProjectID: "dpid_local", Alias: "local", Role: synapse.RolePeer, PathHint: t.TempDir()},
			{ProjectID: "dpid_peer", Alias: "peer", Role: synapse.RolePeer, PathHint: projDir, PlaneProject: "PEER"},
		},
	}
	if err := st.Save(syn); err != nil {
		t.Fatalf("Save synapse: %v", err)
	}

	// Create job in peer ledger
	peerLedger := NewLedger(filepath.Join(projDir, ".dendrite", "jobs"))
	job, err := peerLedger.Create(&Job{Ticket: "PEER-42", Agent: "agy"})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}

	currentLedger := NewLedger(t.TempDir())
	resLedger, matches, err := ResolveTicketLedger(currentLedger, "PEER-42")
	if err != nil {
		t.Fatalf("ResolveTicketLedger via synapse: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != job.ID {
		t.Fatalf("matches = %v, want [%s]", matches, job.ID)
	}
	_ = resLedger
}

func TestResolveTicketLedger_UnknownTicket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	currentLedger := NewLedger(t.TempDir())
	_, _, err := ResolveTicketLedger(currentLedger, "UNKNOWN-999")
	if err == nil {
		t.Fatal("expected error for unknown ticket, got nil")
	}
	if !errors.Is(err, ErrNoMatchingJobs) {
		t.Errorf("expected ErrNoMatchingJobs, got %v", err)
	}
}

func TestResolveTicketLedger_KnownProjectNoJobs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)

	projDir := t.TempDir()
	planeJSON := `{"identifier":"DRAG","project_id":"drag-proj-uuid"}`
	_ = os.MkdirAll(filepath.Join(projDir, ".dendrite"), 0755)
	_ = os.WriteFile(filepath.Join(projDir, ".dendrite", "plane.json"), []byte(planeJSON), 0644)

	indexPath, _ := projectid.DefaultIndexPath()
	idx, _ := projectid.LoadIndex(indexPath)
	idx.Upsert(projectid.Entry{ID: "dpid_drag", Path: projDir})
	_ = idx.Save()

	currentLedger := NewLedger(t.TempDir())
	_, _, err := ResolveTicketLedger(currentLedger, "DRAG-999")
	if err == nil {
		t.Fatal("expected error for ticket with no jobs, got nil")
	}
	if !errors.Is(err, ErrNoMatchingJobs) {
		t.Errorf("expected ErrNoMatchingJobs, got %v", err)
	}
}

func TestResolveTicketLedger_EmptyTicket(t *testing.T) {
	currentLedger := NewLedger(t.TempDir())
	_, _, err := ResolveTicketLedger(currentLedger, "")
	if err == nil {
		t.Fatal("expected error for empty ticket, got nil")
	}
}
