package env

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplicator_DryRun(t *testing.T) {
	runner := withTools(t, stackTools(false))
	host := HostInfo{OS: "darwin", HasDocker: true}
	replicator := NewReplicator(runner, host)

	planner := NewPlanner(host)
	plan, err := planner.PlanReplication(ReplicationRequest{
		Mode:       ModeContainer,
		WithLinear: true,
		WithPlane:  true,
		WithPenpot: true,
	})
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	ctx := context.Background()
	res, err := replicator.ExecutePlan(ctx, plan, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected dry run result to be successful")
	}
	for _, act := range res.ActionResults {
		if !act.Skipped {
			t.Errorf("expected action %s to be skipped in dry-run", act.ActionID)
		}
		if !strings.Contains(act.Output, "[DRY-RUN]") {
			t.Errorf("expected dry-run marker in output: %s", act.Output)
		}
	}
}

func TestReplicator_RenderActions(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	runner := withTools(t, stackTools(false))
	host := HostInfo{OS: "darwin", HasDocker: true}
	replicator := NewReplicator(runner, host)

	planner := NewPlanner(host)
	plan, err := planner.PlanReplication(ReplicationRequest{
		Mode:       ModeContainer,
		WithLinear: true,
		WithPlane:  true,
		WithPenpot: true,
	})
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	ctx := context.Background()
	res, err := replicator.ExecutePlan(ctx, plan, false)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected successful execution")
	}

	linearFile := filepath.Join("stacks", "compose.linear.yaml")
	if _, err := os.Stat(linearFile); os.IsNotExist(err) {
		t.Errorf("expected %s to be created", linearFile)
	}
	planeFile := filepath.Join("stacks", "compose.plane.yaml")
	if _, err := os.Stat(planeFile); os.IsNotExist(err) {
		t.Errorf("expected %s to be created", planeFile)
	}
	penpotFile := filepath.Join("stacks", "compose.penpot.yaml")
	if _, err := os.Stat(penpotFile); os.IsNotExist(err) {
		t.Errorf("expected %s to be created", penpotFile)
	}
}

func TestReplicator_MandatoryFailure(t *testing.T) {
	runner := withTools(t, stackTools(true))
	host := HostInfo{OS: "darwin"}
	replicator := NewReplicator(runner, host)

	plan := &ReplicationPlan{
		Mode: ModeContainer,
		Actions: []PlanAction{
			{
				ID:        "check-docker",
				Type:      ActionCheckDependency,
				Command:   "docker info && docker compose version",
				Rollback:  "echo rollback",
				Mandatory: true,
			},
		},
	}

	ctx := context.Background()
	res, err := replicator.ExecutePlan(ctx, plan, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Errorf("expected result to indicate failure")
	}
}
