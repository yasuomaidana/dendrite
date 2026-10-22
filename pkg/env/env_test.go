package env

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTools runs the real DefaultCommandRunner against shell scripts installed as the named
// tools: PATH holds only their directory and the system directories sh needs, so each test
// decides which tools exist and what they print or fail with, and no real docker or
// network is touched.
func withTools(t *testing.T, tools map[string]string) CommandRunner {
	t.Helper()
	dir := t.TempDir()
	for name, body := range tools {
		script := "#!/bin/sh\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/bin"+string(os.PathListSeparator)+"/usr/bin")
	return &DefaultCommandRunner{}
}

// stackTools answer the commands a replication plan and its audit run: docker succeeds
// (failInfo makes `docker info` fail like a stopped daemon) and curl reports healthy.
func stackTools(failInfo bool) map[string]string {
	docker := "echo ok"
	if failInfo {
		docker = "if [ \"$1\" = info ]; then echo 'daemon stopped' >&2; exit 1; fi\necho ok"
	}
	return map[string]string{"docker": docker, "curl": "echo ok"}
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
	installed := map[string]string{
		"docker":    "if [ \"$1\" = compose ] && [ \"$2\" = version ]; then echo 'Docker Compose version v2.24.5'; fi",
		"ollama":    "exit 0",
		"go":        "exit 0",
		"brew":      "exit 0",
		"npm":       "exit 0",
		"node":      "exit 0",
		"tailscale": "exit 0",
		"linear":    "exit 0",
	}
	runner := withTools(t, installed)

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
