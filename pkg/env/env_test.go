package env

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockCommandRunner struct {
	paths    map[string]string
	commands map[string]string
	errors   map[string]error
}

func (m *mockCommandRunner) LookPath(file string) (string, error) {
	if p, ok := m.paths[file]; ok {
		return p, nil
	}
	return "", fmt.Errorf("command not found: %s", file)
}

func (m *mockCommandRunner) Run(ctx context.Context, cmdStr string, env map[string]string) (string, error) {
	if err, ok := m.errors[cmdStr]; ok {
		return m.commands[cmdStr], err
	}
	if out, ok := m.commands[cmdStr]; ok {
		return out, nil
	}
	return "ok", nil
}

func TestInstallMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  InstallMode
		valid bool
	}{
		{ModeContainer, true},
		{ModeHost, true},
		{ModeHybrid, true},
		{InstallMode("invalid"), false},
		{InstallMode(""), false},
	}

	for _, tt := range tests {
		if tt.mode.IsValid() != tt.valid {
			t.Errorf("expected %s.IsValid() == %v", tt.mode, tt.valid)
		}
	}
}

func TestAllTargets(t *testing.T) {
	targets := AllTargets()
	if len(targets) == 0 {
		t.Fatalf("expected non-empty AllTargets")
	}
	var foundPlane, foundPenpot bool
	for _, tgt := range targets {
		if tgt == TargetPlane {
			foundPlane = true
		}
		if tgt == TargetPenpot {
			foundPenpot = true
		}
	}
	if !foundPlane || !foundPenpot {
		t.Errorf("expected Plane and Penpot in AllTargets")
	}
}

func TestDetector_Detect(t *testing.T) {
	runner := &mockCommandRunner{
		paths: map[string]string{
			"docker":    "/usr/local/bin/docker",
			"ollama":    "/usr/local/bin/ollama",
			"go":        "/usr/local/bin/go",
			"brew":      "/opt/homebrew/bin/brew",
			"npm":       "/usr/local/bin/npm",
			"node":      "/usr/local/bin/node",
			"tailscale": "/usr/local/bin/tailscale",
			"linear":    "/usr/local/bin/linear",
		},
		commands: map[string]string{
			"docker compose version": "Docker Compose version v2.24.5",
		},
	}

	detector := NewDetector(runner)
	ctx := context.Background()
	info, err := detector.Detect(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.HasDocker {
		t.Errorf("expected HasDocker to be true")
	}
	if !info.HasDockerComp {
		t.Errorf("expected HasDockerComp to be true")
	}
	if !info.HasOllama {
		t.Errorf("expected HasOllama to be true")
	}
	if !info.HasLinearCLI {
		t.Errorf("expected HasLinearCLI to be true")
	}
	if !info.HasTailscale {
		t.Errorf("expected HasTailscale to be true")
	}
}

func TestDefaultConstructors(t *testing.T) {
	d := NewDetector(nil)
	if d == nil {
		t.Errorf("expected non-nil detector")
	}
	r := NewReplicator(nil, HostInfo{})
	if r == nil {
		t.Errorf("expected non-nil replicator")
	}
	cmdRunner := &DefaultCommandRunner{}
	_, _ = cmdRunner.LookPath("go")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, _ = cmdRunner.Run(ctx, "echo test", nil)
}

func TestPlanner_ContainerMode_WithLinearPlanePenpotGrafana(t *testing.T) {
	host := HostInfo{
		OS:            "darwin",
		Arch:          "arm64",
		HasDocker:     true,
		HasDockerComp: true,
	}

	planner := NewPlanner(host)
	req := ReplicationRequest{
		Mode:        ModeContainer,
		WithLinear:  true,
		WithPlane:   true,
		WithPenpot:  true,
		WithGrafana: true,
	}

	plan, err := planner.PlanReplication(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Mode != ModeContainer {
		t.Errorf("expected mode container, got %s", plan.Mode)
	}

	var hasLinearAction, hasPlaneAction, hasPenpotAction, hasGrafanaAction bool
	for _, a := range plan.Actions {
		if a.Target == TargetLinear {
			hasLinearAction = true
		}
		if a.Target == TargetPlane {
			hasPlaneAction = true
		}
		if a.Target == TargetPenpot {
			hasPenpotAction = true
		}
		if a.Target == TargetGrafana {
			hasGrafanaAction = true
		}
	}
	if !hasLinearAction {
		t.Errorf("expected container plan to contain TargetLinear action")
	}
	if !hasPlaneAction {
		t.Errorf("expected container plan to contain TargetPlane action")
	}
	if !hasPenpotAction {
		t.Errorf("expected container plan to contain TargetPenpot action")
	}
	if !hasGrafanaAction {
		t.Errorf("expected container plan to contain TargetGrafana action")
	}

	if len(plan.ComposeFiles) < 4 {
		t.Errorf("expected at least 4 compose files, got %d", len(plan.ComposeFiles))
	}
}

func TestPlanner_HybridMode(t *testing.T) {
	host := HostInfo{
		OS:             "darwin",
		Arch:           "arm64",
		PackageManager: PkgBrew,
		HasDocker:      true,
		HasDockerComp:  true,
		HasOllama:      false,
	}

	planner := NewPlanner(host)
	req := ReplicationRequest{
		Mode:       ModeHybrid,
		WithPlane:  true,
		WithLinear: true,
	}

	plan, err := planner.PlanReplication(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Mode != ModeHybrid {
		t.Errorf("expected mode hybrid, got %s", plan.Mode)
	}

	var hasOllamaHost, hasPlaneCLIHost bool
	for _, a := range plan.Actions {
		if a.ID == "install-host-ollama-metal" {
			hasOllamaHost = true
		}
		if a.ID == "install-host-plane-cli" {
			hasPlaneCLIHost = true
		}
	}
	if !hasOllamaHost {
		t.Errorf("expected hybrid plan to install Ollama Metal on host")
	}
	if !hasPlaneCLIHost {
		t.Errorf("expected hybrid plan to install Plane CLI on host")
	}
}

func TestPlanner_HostMode(t *testing.T) {
	host := HostInfo{
		OS:             "darwin",
		Arch:           "arm64",
		PackageManager: PkgBrew,
		HasNpm:         true,
	}

	planner := NewPlanner(host)
	req := ReplicationRequest{
		Mode:    ModeHost,
		Targets: []ReplicationTarget{TargetLinear, TargetPlane, TargetOllama, TargetTailscale, TargetCloudflare},
	}

	plan, err := planner.PlanReplication(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Mode != ModeHost {
		t.Errorf("expected mode host, got %s", plan.Mode)
	}

	var hasCloudflare bool
	for _, a := range plan.Actions {
		if a.Target == TargetCloudflare {
			hasCloudflare = true
		}
	}
	if !hasCloudflare {
		t.Errorf("expected host plan to include Cloudflare action")
	}
}

func TestPlanner_HostMode_Linux(t *testing.T) {
	host := HostInfo{
		OS:             "linux",
		Arch:           "amd64",
		PackageManager: PkgApt,
		HasNpm:         false,
	}

	planner := NewPlanner(host)
	req := ReplicationRequest{
		Mode:    ModeHost,
		Targets: []ReplicationTarget{TargetLinear, TargetPlane, TargetOllama, TargetTailscale, TargetCloudflare},
	}

	plan, err := planner.PlanReplication(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Actions) == 0 {
		t.Errorf("expected actions for linux host mode")
	}
}

func TestPlanner_InvalidMode(t *testing.T) {
	host := HostInfo{}
	planner := NewPlanner(host)
	_, err := planner.PlanReplication(ReplicationRequest{Mode: "unsupported"})
	if err == nil {
		t.Errorf("expected error for unsupported mode")
	}
}

func TestLinear_GenerateLinearComposeDefaults(t *testing.T) {
	cfg := LinearConfig{}
	yamlStr := GenerateLinearCompose(cfg)
	if !strings.Contains(yamlStr, "8085:8085") {
		t.Errorf("expected default 8085 port")
	}

	info := HostInfo{PackageManager: PkgApt}
	cmd := GenerateHostLinearInstallCommand(info)
	if !strings.Contains(cmd, "install.sh") {
		t.Errorf("expected install.sh command")
	}
	infoBrew := HostInfo{PackageManager: PkgBrew}
	cmdBrew := GenerateHostLinearInstallCommand(infoBrew)
	if !strings.Contains(cmdBrew, "brew install") {
		t.Errorf("expected brew install command")
	}
}

func TestPlane_GeneratePlaneComposeDefaults(t *testing.T) {
	cfg := PlaneConfig{}
	yamlStr := GeneratePlaneCompose(cfg)
	if !strings.Contains(yamlStr, "PLANE_API_PORT:-8090") {
		t.Errorf("expected default port 8090")
	}
	if !strings.Contains(yamlStr, "plane-ui:") {
		t.Errorf("expected plane-ui service to be enabled by default")
	}
	if !strings.Contains(yamlStr, "plane-proxy:") {
		t.Errorf("expected plane-proxy service to be enabled by default")
	}
	if !strings.Contains(yamlStr, "PLANE_UI_PORT:-8091") {
		t.Errorf("expected default UI port 8091")
	}

	info := HostInfo{PackageManager: PkgApt}
	cmd := GenerateHostPlaneInstallCommand(info)
	if !strings.Contains(cmd, "install.sh") {
		t.Errorf("expected install.sh fallback")
	}
}

func TestPlane_GeneratePlaneCompose_OptionalUI(t *testing.T) {
	cfg := PlaneConfig{}
	cfg.SetWithUI(false)
	yamlStr := GeneratePlaneCompose(cfg)
	if !strings.Contains(yamlStr, "PLANE_API_PORT:-8090") {
		t.Errorf("expected default port 8090")
	}
	if strings.Contains(yamlStr, "plane-ui:") {
		t.Errorf("expected plane-ui service to be excluded when disabled")
	}
	if strings.Contains(yamlStr, "plane-proxy:") {
		t.Errorf("expected plane-proxy service to be excluded when disabled")
	}
	if strings.Contains(yamlStr, "plane_proxy.conf") {
		t.Errorf("expected plane_proxy.conf volume to be excluded when disabled")
	}
}

func TestPlanner_PlaneUI_OptionalToggle(t *testing.T) {
	host := HostInfo{OS: "darwin", Arch: "arm64", HasDocker: true}
	planner := NewPlanner(host)

	// Default on: WithPlane: true, WithPlaneUI: nil
	planOn, err := planner.PlanReplication(ReplicationRequest{
		Mode:      ModeContainer,
		WithPlane: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var hasUIAction bool
	for _, a := range planOn.Actions {
		if a.ID == "verify-plane-ui-health" {
			hasUIAction = true
		}
	}
	if !hasUIAction {
		t.Errorf("expected verify-plane-ui-health action when Plane UI is default on")
	}

	// Explicitly off
	disabled := false
	planOff, err := planner.PlanReplication(ReplicationRequest{
		Mode:        ModeContainer,
		WithPlane:   true,
		WithPlaneUI: &disabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range planOff.Actions {
		if a.ID == "verify-plane-ui-health" {
			t.Errorf("expected verify-plane-ui-health action to be omitted when Plane UI is disabled")
		}
	}
}

func TestPenpot_GeneratePenpotComposeDefaults(t *testing.T) {
	cfg := PenpotConfig{}
	yamlStr := GeneratePenpotCompose(cfg)
	if !strings.Contains(yamlStr, "9000:80") {
		t.Errorf("expected default port 9000")
	}
}

func TestHealthChecks(t *testing.T) {
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	if !CheckLinearHealth(ctx, ts.URL, 500*time.Millisecond) {
		t.Errorf("expected linear health to be true")
	}
	if !CheckPlaneHealth(ctx, ts.URL, 500*time.Millisecond) {
		t.Errorf("expected plane health to be true")
	}
	if !CheckPenpotHealth(ctx, ts.URL, 500*time.Millisecond) {
		t.Errorf("expected penpot health to be true")
	}
	if !CheckPlaneUIHealth(ctx, ts.URL, 500*time.Millisecond) {
		t.Errorf("expected plane UI health to be true")
	}

	// Test negative cases
	_ = CheckLinearHealth(ctx, "http://invalid-endpoint-1234:9999", 50*time.Millisecond)
	_ = CheckPlaneHealth(ctx, "http://invalid-endpoint-1234:9999", 50*time.Millisecond)
	_ = CheckPlaneUIHealth(ctx, "http://invalid-endpoint-1234:9999", 50*time.Millisecond)
	_ = CheckPenpotHealth(ctx, "http://invalid-endpoint-1234:9999", 50*time.Millisecond)
}

func TestReplicator_DryRun(t *testing.T) {
	runner := &mockCommandRunner{}
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

	runner := &mockCommandRunner{}
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
	runner := &mockCommandRunner{
		errors: map[string]error{
			"docker info && docker compose version": fmt.Errorf("daemon stopped"),
		},
	}
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
