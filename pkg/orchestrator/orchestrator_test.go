package orchestrator_test

import (
	"strings"
	"testing"

	"Dendrite/pkg/orchestrator"
	"Dendrite/pkg/topology"
)

func TestGenerateCloudInit_Valid(t *testing.T) {
	cfg := orchestrator.CloudInitConfig{
		Hostname:           "dendrite-debian",
		TailscaleAuthKey:   "tskey-auth-k123456789",
		DockerUsers:        []string{"admin", "deployer"},
		SSHAuthorizedKeys:  []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI..."},
		ExtraRunCmd:        []string{"echo 'ready' > /etc/dendrite-ready"},
		EnableTailscaleSSH: true,
	}

	script, err := orchestrator.GenerateCloudInit(cfg)
	if err != nil {
		t.Fatalf("unexpected error generating cloud-init: %v", err)
	}

	if !strings.HasPrefix(script, "#cloud-config\n") {
		t.Errorf("expected script to start with #cloud-config header")
	}
	if !strings.Contains(script, "dendrite-debian") {
		t.Errorf("expected script to contain hostname")
	}
	if !strings.Contains(script, "tskey-auth-k123456789") {
		t.Errorf("expected script to contain tailscale auth key")
	}
	if !strings.Contains(script, "--ssh") {
		t.Errorf("expected script to include --ssh flag for tailscale")
	}
	if !strings.Contains(script, "docker.io") {
		t.Errorf("expected default packages to include docker.io")
	}
}

func TestGenerateCloudInit_EmptyHostname(t *testing.T) {
	cfg := orchestrator.CloudInitConfig{
		Hostname: "",
	}
	_, err := orchestrator.GenerateCloudInit(cfg)
	if err == nil {
		t.Errorf("expected error for empty hostname, got nil")
	}
}

func TestComposeCommand_BuildArgs(t *testing.T) {
	var nilCmd *orchestrator.ComposeCommand
	if _, err := nilCmd.BuildArgs(); err == nil {
		t.Errorf("expected error building args on nil ComposeCommand")
	}
	if str := nilCmd.String(); str != "" {
		t.Errorf("expected empty string for nil ComposeCommand String()")
	}

	cmd := orchestrator.NewComposeCommand("up")
	cmd.ComposeFiles = []string{"stacks/compose.control.yaml"}
	cmd.Profiles = []string{"control-plane"}
	cmd.Detached = true
	cmd.ExtraArgs = []string{"--remove-orphans"}

	args, err := cmd.BuildArgs()
	if err != nil {
		t.Fatalf("unexpected error building args: %v", err)
	}

	expectedArgs := []string{
		"compose",
		"-f", "stacks/compose.control.yaml",
		"--profile", "control-plane",
		"up",
		"-d",
		"--remove-orphans",
	}

	if len(args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(args), args)
	}
	for i, arg := range expectedArgs {
		if args[i] != arg {
			t.Errorf("at index %d: expected '%s', got '%s'", i, arg, args[i])
		}
	}

	cmdStr := cmd.String()
	if !strings.Contains(cmdStr, "docker compose") {
		t.Errorf("expected cmd string to start with 'docker compose', got '%s'", cmdStr)
	}

	// Test down action (should not append -d)
	downCmd := orchestrator.NewComposeCommand("down")
	downArgs, err := downCmd.BuildArgs()
	if err != nil {
		t.Fatalf("unexpected error building down args: %v", err)
	}
	for _, a := range downArgs {
		if a == "-d" {
			t.Errorf("did not expect -d flag for down action")
		}
	}

	// Test empty action error
	emptyActionCmd := orchestrator.NewComposeCommand("   ")
	_, err = emptyActionCmd.BuildArgs()
	if err != orchestrator.ErrEmptyAction {
		t.Errorf("expected ErrEmptyAction, got: %v", err)
	}
	if !strings.Contains(emptyActionCmd.String(), "invalid") {
		t.Errorf("expected invalid message in String() output")
	}
}

func TestOrchestrator_Lifecycle(t *testing.T) {
	// Nil config error
	_, err := orchestrator.NewOrchestrator(nil)
	if err != orchestrator.ErrNilTopologyConfig {
		t.Errorf("expected ErrNilTopologyConfig, got: %v", err)
	}

	// Invalid config error
	invalidCfg := &topology.TopologyConfig{Scenario: "invalid"}
	_, err = orchestrator.NewOrchestrator(invalidCfg)
	if err == nil {
		t.Errorf("expected error initializing orchestrator with invalid topology config")
	}

	// Valid LocalHybrid orchestrator
	cfg, err := topology.NewTopologyConfig(topology.ScenarioLocalHybrid, topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	})
	if err != nil {
		t.Fatalf("failed to build topology config: %v", err)
	}

	orch, err := orchestrator.NewOrchestrator(cfg)
	if err != nil {
		t.Fatalf("failed to build orchestrator: %v", err)
	}

	// Test Debian node compose command
	debianCmd, err := orch.BuildNodeComposeCommand(topology.NodeDebian, "up")
	if err != nil {
		t.Fatalf("failed to build compose command for debian: %v", err)
	}
	if len(debianCmd.Profiles) == 0 {
		t.Errorf("expected debian to have profiles")
	}
	if len(debianCmd.ComposeFiles) == 0 {
		t.Errorf("expected debian to have compose files")
	}

	// Test Cloud-Init for Debian node
	cloudInit, err := orch.BuildCloudInitForNode(topology.NodeDebian, "ts-auth-key-123", []string{"ssh-key-abc"})
	if err != nil {
		t.Fatalf("failed to build cloud-init for debian: %v", err)
	}
	if !strings.Contains(cloudInit, "dendrite-debian") {
		t.Errorf("expected cloud-init to target dendrite-debian")
	}

	// Test Cloud-Init for MacBook (should fail)
	_, err = orch.BuildCloudInitForNode(topology.NodeMacBook, "ts-auth", nil)
	if err == nil {
		t.Errorf("expected error building cloud-init for macbook, got nil")
	}

	// Test invalid node
	_, err = orch.BuildNodeComposeCommand("windows", "up")
	if err == nil {
		t.Errorf("expected error for invalid node")
	}
	_, err = orch.BuildCloudInitForNode("windows", "key", nil)
	if err == nil {
		t.Errorf("expected error for invalid node in cloud-init")
	}

	// Test custom profile file override
	orch.SetProfileFile("control-plane", "custom/control.yaml")
	customCmd, _ := orch.BuildNodeComposeCommand(topology.NodeDebian, "up")
	hasCustom := false
	for _, f := range customCmd.ComposeFiles {
		if f == "custom/control.yaml" {
			hasCustom = true
			break
		}
	}
	if !hasCustom {
		t.Errorf("expected custom compose file in debian compose files")
	}

	// Test deployment plan
	plan, err := orch.BuildDeploymentPlan(topology.NodeDebian, "up", "ts-auth-123", []string{"ssh-key-1"})
	if err != nil {
		t.Fatalf("failed to build deployment plan: %v", err)
	}
	if plan.Node != string(topology.NodeDebian) {
		t.Errorf("expected plan node to be debian, got %s", plan.Node)
	}
	if plan.CloudInitScript == "" {
		t.Errorf("expected non-empty cloud-init in deployment plan for debian")
	}
	if plan.ComposeCommand == nil {
		t.Errorf("expected compose command in deployment plan")
	}

	// Test deployment plan for MacBook (cloud-init should be empty string without error)
	macPlan, err := orch.BuildDeploymentPlan(topology.NodeMacBook, "up", "ts-auth-123", nil)
	if err != nil {
		t.Fatalf("unexpected error building deployment plan for macbook: %v", err)
	}
	if macPlan.CloudInitScript != "" {
		t.Errorf("expected empty cloud-init for macbook, got '%s'", macPlan.CloudInitScript)
	}
}

func TestOrchestrationErrorUnwrap(t *testing.T) {
	orchErr := &orchestrator.OrchestrationError{
		Node: "debian",
		Op:   "TestOp",
		Err:  orchestrator.ErrInvalidNode,
	}

	if orchErr.Unwrap() != orchestrator.ErrInvalidNode {
		t.Errorf("expected unwrapped error to be ErrInvalidNode")
	}
	if !strings.Contains(orchErr.Error(), "debian") {
		t.Errorf("expected error string to contain node name")
	}
}
