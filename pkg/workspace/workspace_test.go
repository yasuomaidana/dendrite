package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		from State
		to   State
		err  bool
	}{
		{StateUnregistered, StateRegistered, false},
		{StateUnregistered, StateActive, true},
		{StateRegistered, StateActive, false},
		{StateRegistered, StateSuspended, true},
		{StateRegistered, StateUnregistered, false},
		{StateActive, StateSuspended, false},
		{StateActive, StateUnregistered, false},
		{StateSuspended, StateActive, false},
		{StateSuspended, StateUnregistered, false},
	}

	for _, tt := range tests {
		err := ValidateTransition(tt.from, tt.to)
		if tt.err && err == nil {
			t.Errorf("expected transition %s -> %s to fail", tt.from, tt.to)
		} else if !tt.err && err != nil {
			t.Errorf("expected transition %s -> %s to succeed, got: %v", tt.from, tt.to, err)
		}
	}
}

func TestRegistry_AddProject_Validation(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")

	reg, err := NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Invalid name
	_, err = reg.AddProject("invalid name with spaces", tempDir)
	if err != ErrInvalidProjectName {
		t.Errorf("expected ErrInvalidProjectName, got: %v", err)
	}

	// Non-existent path
	_, err = reg.AddProject("proj1", "/non/existent/path/xyz")
	if err != ErrInvalidProjectPath {
		t.Errorf("expected ErrInvalidProjectPath, got: %v", err)
	}

	// Valid add
	p1Dir := filepath.Join(tempDir, "p1")
	_ = os.MkdirAll(p1Dir, 0755)

	entry, err := reg.AddProject("proj-alpha", p1Dir)
	if err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	if entry.State != StateRegistered {
		t.Errorf("expected state StateRegistered, got: %s", entry.State)
	}
	if entry.VirtualKeyAlias != "project-proj-alpha" {
		t.Errorf("expected key alias 'project-proj-alpha', got: %s", entry.VirtualKeyAlias)
	}

	// Verify spend cache file was generated
	if _, err := os.Stat(entry.SpendCachePath); err != nil {
		t.Errorf("expected spend cache file at %s", entry.SpendCachePath)
	}

	// Duplicate add
	_, err = reg.AddProject("proj-alpha", p1Dir)
	if err != ErrProjectAlreadyExists {
		t.Errorf("expected ErrProjectAlreadyExists, got: %v", err)
	}
}

func TestRegistry_SwitchProject_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")

	reg, err := NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	p1Dir := filepath.Join(tempDir, "p1")
	p2Dir := filepath.Join(tempDir, "p2")
	_ = os.MkdirAll(p1Dir, 0755)
	_ = os.MkdirAll(p2Dir, 0755)

	_, err = reg.AddProject("alpha", p1Dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reg.AddProject("beta", p2Dir)
	if err != nil {
		t.Fatal(err)
	}

	// Initially no active project
	_, err = reg.GetActiveProject()
	if err != ErrNoActiveProject {
		t.Fatalf("expected ErrNoActiveProject, got: %v", err)
	}

	// Switch to non-existent
	_, err = reg.SwitchProject("gamma")
	if err != ErrProjectNotFound {
		t.Fatalf("expected ErrProjectNotFound, got: %v", err)
	}

	// Switch to alpha with latency check (< 50ms)
	start := time.Now()
	switched, err := reg.SwitchProject("alpha")
	duration := time.Since(start)
	if err != nil {
		t.Fatalf("failed to switch to alpha: %v", err)
	}
	limit := 50 * time.Millisecond
	if os.Getenv("CI") != "" {
		limit = 500 * time.Millisecond // shared runners stall; the 50 ms budget is held on a developer machine
	}
	if duration > limit {
		t.Errorf("switch latency exceeded %v: %v", limit, duration)
	}
	if switched.State != StateActive {
		t.Errorf("expected alpha to be StateActive, got: %s", switched.State)
	}

	active, err := reg.GetActiveProject()
	if err != nil || active.Name != "alpha" {
		t.Fatalf("expected active project alpha, got: %v", active)
	}

	// Switch to alpha again (idempotent)
	switchedAgain, err := reg.SwitchProject("alpha")
	if err != nil || switchedAgain.State != StateActive {
		t.Fatalf("expected idempotent switch to succeed, got: %v", err)
	}

	// Switch to beta: alpha must become Suspended, beta becomes Active
	switchedBeta, err := reg.SwitchProject("beta")
	if err != nil {
		t.Fatalf("failed to switch to beta: %v", err)
	}
	if switchedBeta.State != StateActive {
		t.Errorf("expected beta to be StateActive, got: %s", switchedBeta.State)
	}

	alphaEntry, _ := reg.GetProject("alpha")
	if alphaEntry.State != StateSuspended {
		t.Errorf("expected alpha to be StateSuspended, got: %s", alphaEntry.State)
	}

	// List projects
	list := reg.ListProjects()
	if len(list) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Errorf("expected sorted list [alpha, beta], got: %v", list)
	}

	// Re-load registry from disk and verify persistence
	reg2, err := NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	active2, err := reg2.GetActiveProject()
	if err != nil || active2.Name != "beta" {
		t.Fatalf("expected active project beta after reload, got: %v", active2)
	}
}

func TestRegistry_RemoveProject(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")

	reg, err := NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	pDir := filepath.Join(tempDir, "proj")
	_ = os.MkdirAll(pDir, 0755)

	entry, err := reg.AddProject("proj-rem", pDir)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = reg.SwitchProject("proj-rem")

	// Remove non-existent
	if err := reg.RemoveProject("unknown", false); err != ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got: %v", err)
	}

	// Remove project and purge data
	if err := reg.RemoveProject("proj-rem", true); err != nil {
		t.Fatalf("failed to remove project: %v", err)
	}

	_, err = reg.GetActiveProject()
	if err != ErrNoActiveProject {
		t.Errorf("expected ErrNoActiveProject after removing active project, got: %v", err)
	}

	if _, err := os.Stat(entry.DataDir); !os.IsNotExist(err) {
		t.Errorf("expected dataDir to be purged, but it exists")
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")

	reg, err := NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pDir := filepath.Join(tempDir, "p", string(rune('a'+idx)))
			_ = os.MkdirAll(pDir, 0755)
			name := "proj-" + string(rune('a'+idx))
			_, _ = reg.AddProject(name, pDir)
			_ = reg.ListProjects()
		}(i)
	}
	wg.Wait()

	projects := reg.ListProjects()
	if len(projects) != 10 {
		t.Fatalf("expected 10 projects, got %d", len(projects))
	}
}

func TestRegistry_DefaultsAndErrors(t *testing.T) {
	// Test default paths
	rDef, err := NewRegistry("", "")
	if err != nil {
		t.Fatalf("failed to create registry with default paths: %v", err)
	}
	if rDef.configPath == "" || rDef.baseDataDir == "" {
		t.Errorf("expected default paths to be populated")
	}

	// Test GetProject non-existent
	_, err = rDef.GetProject("does-not-exist")
	if err != ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got: %v", err)
	}

	// Test corrupted config load
	tempDir := t.TempDir()
	corruptFile := filepath.Join(tempDir, "corrupt.json")
	if err := os.WriteFile(corruptFile, []byte("{invalid json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = NewRegistry(corruptFile, tempDir)
	if err == nil {
		t.Errorf("expected error on corrupt config json")
	}
}
