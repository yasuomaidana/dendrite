package env

import (
	"Dendrite/pkg/stacks"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateControlComposeMatchesCommittedStack(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", "stacks", "compose.control.yaml"))
	if err != nil {
		t.Fatalf("read committed control stack: %v", err)
	}
	if GenerateControlCompose() != string(committed) {
		t.Fatal("GenerateControlCompose drifted from stacks/compose.control.yaml; update the template")
	}
	if !strings.Contains(GenerateControlCompose(), "LITELLM_MASTER_KEY:?") {
		t.Fatal("control stack must require LITELLM_MASTER_KEY")
	}
}

func TestReplicator_RenderNeverOverwritesExistingStack(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(stacks.EnvDir, "stacks") // a stack directory inside the temp working directory
	if err := os.MkdirAll("stacks", 0o755); err != nil {
		t.Fatal(err)
	}
	userStack := "# user-maintained stack\n"
	if err := os.WriteFile(filepath.Join("stacks", "compose.control.yaml"), []byte(userStack), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewReplicator(nil, HostInfo{OS: "darwin"})
	for _, target := range []ReplicationTarget{TargetLiteLLM, TargetPenpot} {
		if _, err := r.handleRenderAction(PlanAction{Target: target}); err != nil {
			t.Fatalf("render %s: %v", target, err)
		}
	}
	got, _ := os.ReadFile(filepath.Join("stacks", "compose.control.yaml"))
	if string(got) != userStack {
		t.Fatalf("existing control stack was overwritten:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join("stacks", "compose.penpot.yaml")); err != nil {
		t.Fatalf("missing stack files must still be generated: %v", err)
	}
}

func TestWriteStackFileIfAbsent_StatError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeStackFileIfAbsent(filepath.Join(blocker, "x", "compose.yaml"), "x"); err == nil {
		t.Fatal("expected an error when the parent path is a regular file")
	}
}

func TestWriteStackFileIfAbsent_UnwritableParent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := writeStackFileIfAbsent(filepath.Join(dir, "stacks", "compose.yaml"), "x"); err == nil {
		t.Fatal("expected an error when the stacks directory cannot be created")
	}
}
