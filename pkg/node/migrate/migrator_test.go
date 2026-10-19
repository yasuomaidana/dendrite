package migrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type scriptedExecutor struct {
	ExecuteFunc     func(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error)
	StreamFileFunc  func(ctx context.Context, host string, port int, user string, keyPath string, localPath string, remotePath string) error
	ProbeHealthFunc func(ctx context.Context, url string, timeout time.Duration) (bool, error)
	ExecutedCmds    []string
	StreamedFiles   []string
}

func newScriptedExecutor() *scriptedExecutor {
	return &scriptedExecutor{
		ExecutedCmds:  make([]string, 0),
		StreamedFiles: make([]string, 0),
	}
}

func (s *scriptedExecutor) Execute(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error) {
	s.ExecutedCmds = append(s.ExecutedCmds, cmd)
	if s.ExecuteFunc != nil {
		return s.ExecuteFunc(ctx, host, port, user, keyPath, cmd)
	}
	if strings.Contains(cmd, "uname") {
		return "Linux\nx86_64\ndebian-server\n", nil
	}
	if strings.Contains(cmd, "cat /etc/os-release") {
		return "ID=debian\nVERSION_ID=12\n", nil
	}
	if strings.Contains(cmd, "docker --version") {
		return "Docker version 27.0.3, build 7d4bed1\n", nil
	}
	return "ok\n", nil
}

func (s *scriptedExecutor) StreamFile(ctx context.Context, host string, port int, user string, keyPath string, localPath string, remotePath string) error {
	s.StreamedFiles = append(s.StreamedFiles, fmt.Sprintf("%s -> %s", localPath, remotePath))
	if s.StreamFileFunc != nil {
		return s.StreamFileFunc(ctx, host, port, user, keyPath, localPath, remotePath)
	}
	return nil
}

func (s *scriptedExecutor) ProbeHealth(ctx context.Context, url string, timeout time.Duration) (bool, error) {
	if s.ProbeHealthFunc != nil {
		return s.ProbeHealthFunc(ctx, url, timeout)
	}
	return true, nil
}

func TestEvaluateDebianServer_4GB_1TB(t *testing.T) {
	plan := EvaluateDebianServer(4096, 1000)

	if plan.TotalRAMMB != 4096 {
		t.Fatalf("expected 4096 MB RAM, got %d", plan.TotalRAMMB)
	}
	if plan.TotalDiskGB != 1000 {
		t.Fatalf("expected 1000 GB Disk, got %d", plan.TotalDiskGB)
	}

	eligibleMap := make(map[string]bool)
	for _, e := range plan.EligibleServices {
		eligibleMap[e.Service] = true
	}

	// Plane, LiteLLM, and Database MUST be eligible
	if !eligibleMap["plane"] {
		t.Errorf("expected 'plane' to be eligible for Debian 4GB server")
	}
	if !eligibleMap["litellm"] {
		t.Errorf("expected 'litellm' to be eligible for Debian 4GB server")
	}
	if !eligibleMap["database"] {
		t.Errorf("expected 'database' to be eligible for Debian 4GB server")
	}

	// Ollama MUST be excluded (GPU required)
	excludedMap := make(map[string]string)
	for _, e := range plan.ExcludedServices {
		excludedMap[e.Service] = e.Reason
	}
	if _, ok := excludedMap["ollama"]; !ok {
		t.Errorf("expected 'ollama' to be excluded from Debian CPU server")
	}

	// Verify cumulative allocated RAM is <= 3072 MB (4096 - 1024 OS reserve)
	if plan.AllocatedRAMMB > 3072 {
		t.Errorf("allocated RAM %d MB exceeds available 3072 MB", plan.AllocatedRAMMB)
	}

	if !strings.Contains(plan.RecommendedAction, "dendrite translocate run") {
		t.Errorf("expected recommended action to contain 'dendrite translocate run', got '%s'", plan.RecommendedAction)
	}
}

func TestMigrate_HappyPath(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	req := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
		PruneLocal:  false,
		Timeout:     5 * time.Second,
	}

	res, err := migrator.Migrate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected migration error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected migration to succeed")
	}
	if len(res.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(res.Steps))
	}

	for _, step := range res.Steps {
		if step.Status != "SUCCESS" {
			t.Errorf("step %s did not succeed: %s", step.Phase, step.Details)
		}
	}

	if ep, ok := res.UpdatedEndpoints["plane"]; !ok || ep != "http://debian-server:8090" {
		t.Errorf("expected updated endpoint http://debian-server:8090, got '%s'", ep)
	}
}

func TestMigrate_DryRun(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	req := MigrationRequest{
		Services:    []string{"plane", "litellm"},
		TargetHost:  "deployer@debian-server",
		DryRun:      true,
		AutoApprove: false, // dry-run should not require approval
	}

	res, err := migrator.Migrate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected dry-run to succeed")
	}

	if len(exec.ExecutedCmds) > 0 {
		t.Errorf("expected zero remote commands in dry run, got %d", len(exec.ExecutedCmds))
	}

	if ep, ok := res.UpdatedEndpoints["litellm"]; !ok || ep != "http://debian-server:4000" {
		t.Errorf("expected litellm endpoint, got '%s'", ep)
	}
}

func TestMigrate_InteractiveAuthorization(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	// Case 1: Non-interactive without force -> Error
	reqNoIn := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: false,
		In:          nil,
	}
	_, err := migrator.Migrate(ctx, reqNoIn)
	if !errors.Is(err, ErrNonInteractiveNoForce) {
		t.Errorf("expected ErrNonInteractiveNoForce, got %v", err)
	}

	// Case 2: Interactive with 'n' (denial) -> Error
	inDenied := bytes.NewBufferString("n\n")
	outBuf := &bytes.Buffer{}
	reqDenied := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: false,
		In:          inDenied,
		Out:         outBuf,
	}
	_, err = migrator.Migrate(ctx, reqDenied)
	if !errors.Is(err, ErrInteractiveAuthDenied) {
		t.Errorf("expected ErrInteractiveAuthDenied, got %v", err)
	}

	// Case 3: Interactive with 'y' (approved) -> Success
	inApproved := bytes.NewBufferString("y\n")
	reqApproved := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: false,
		In:          inApproved,
		Out:         outBuf,
	}
	res, err := migrator.Migrate(ctx, reqApproved)
	if err != nil {
		t.Fatalf("expected approval to succeed, got %v", err)
	}
	if !res.Success {
		t.Errorf("expected successful result")
	}
}

func TestMigrate_HealthCheckFailureAndRollback(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	// Simulate probe failure
	exec.ProbeHealthFunc = func(ctx context.Context, url string, timeout time.Duration) (bool, error) {
		return false, errors.New("connection refused on port 8090")
	}

	migrator := NewMigrator(exec, nil)
	req := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
		Timeout:     1 * time.Second,
	}

	res, err := migrator.Migrate(ctx, req)
	if err == nil {
		t.Fatalf("expected migration to fail due to health probe failure")
	}

	if res.Success {
		t.Errorf("expected res.Success to be false")
	}

	// Verify rollback docker compose down was executed on remote
	var foundRollback bool
	for _, cmd := range exec.ExecutedCmds {
		if strings.Contains(cmd, "docker compose") && strings.Contains(cmd, "down") {
			foundRollback = true
			break
		}
	}
	if !foundRollback {
		t.Errorf("expected rollback down command to be invoked on remote")
	}
}

func TestMigrate_DebianAlias(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	req := MigrationRequest{
		Services:    []string{"debian"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
		DryRun:      true,
	}

	res, err := migrator.Migrate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error for debian alias: %v", err)
	}

	// Should have expanded to plane, litellm, database
	if len(res.ServicesMigrated) < 3 {
		t.Errorf("expected at least 3 services migrated for debian alias, got %d", len(res.ServicesMigrated))
	}
}

func TestMigrate_CapacityAndOSValidation(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()

	// Case 1: Insufficient RAM
	migrator := NewMigrator(exec, nil)
	reqRAM := MigrationRequest{
		Services:    []string{"plane"}, // requires 1536 MB
		TargetHost:  "deployer@debian-server",
		TargetRAMMB: 512, // target offers only 512 MB
		AutoApprove: true,
	}
	_, err := migrator.Migrate(ctx, reqRAM)
	if !errors.Is(err, ErrInsufficientCapacity) {
		t.Errorf("expected ErrInsufficientCapacity, got %v", err)
	}

	// Case 2: Non-Linux OS response
	execDarwin := newScriptedExecutor()
	execDarwin.ExecuteFunc = func(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error) {
		if strings.Contains(cmd, "uname") {
			return "Darwin\narm64\n", nil
		}
		return "ok", nil
	}
	migratorDarwin := NewMigrator(execDarwin, nil)
	reqOS := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "macbook-remote",
		AutoApprove: true,
	}
	_, err = migratorDarwin.Migrate(ctx, reqOS)
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("expected ErrUnsupportedOS, got %v", err)
	}
}

func TestMigrate_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	// Empty target host
	_, err := migrator.Migrate(ctx, MigrationRequest{
		Services: []string{"plane"},
	})
	if err == nil || !strings.Contains(err.Error(), "target host is required") {
		t.Errorf("expected error for empty target host, got %v", err)
	}

	// Empty services
	_, err = migrator.Migrate(ctx, MigrationRequest{
		TargetHost: "debian-server",
		Services:   []string{},
	})
	if err == nil || !strings.Contains(err.Error(), "no services specified") {
		t.Errorf("expected error for empty services, got %v", err)
	}

	// Unknown service
	_, err = migrator.Migrate(ctx, MigrationRequest{
		TargetHost: "debian-server",
		Services:   []string{"unknown-random-svc"},
	})
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestMigrator_Plan(t *testing.T) {
	migrator := NewMigrator(nil, nil)
	plan, err := migrator.Plan(MigrationRequest{
		TargetRAMMB:  4096,
		TargetDiskGB: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	if plan.TotalRAMMB != 4096 {
		t.Errorf("expected 4096 MB, got %d", plan.TotalRAMMB)
	}
	if len(plan.EligibleServices) == 0 {
		t.Errorf("expected eligible services")
	}
}

func TestDefaultRemoteExecutor_ProbeHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	executor := NewDefaultRemoteExecutor()
	executor.HTTPClient = server.Client()

	ctx := context.Background()
	ok, err := executor.ProbeHealth(ctx, server.URL+"/health", 2*time.Second)
	if !ok || err != nil {
		t.Errorf("expected healthy probe, got %v, err: %v", ok, err)
	}

	// Test failing endpoint
	okFail, errFail := executor.ProbeHealth(ctx, server.URL+"/invalid", 500*time.Millisecond)
	if okFail || errFail == nil {
		t.Errorf("expected probe to fail on /invalid")
	}
}

func TestDefaultRemoteExecutor_ExecuteAndStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	exec := NewDefaultRemoteExecutor()
	// Test command execution to invalid port - should fail fast
	_, err := exec.Execute(ctx, "127.0.0.1", 65530, "user", "", "echo test")
	if err == nil {
		t.Errorf("expected execution error on closed port")
	}

	// Test stream file to invalid destination
	err = exec.StreamFile(ctx, "127.0.0.1", 65530, "user", "", "nonexistent", "/tmp/nonexistent")
	if err == nil {
		t.Errorf("expected stream error on closed port")
	}
}

func TestExecCommand(t *testing.T) {
	ctx := context.Background()
	err := execCommand(ctx, "true")
	if err != nil {
		t.Errorf("expected 'true' command to succeed: %v", err)
	}
}

func TestMigrate_PruneLocal(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()
	migrator := NewMigrator(exec, nil)

	req := MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
		PruneLocal:  true,
		Timeout:     5 * time.Second,
	}

	res, err := migrator.Migrate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected migration error with PruneLocal: %v", err)
	}
	if !res.Success {
		t.Errorf("expected migration to succeed")
	}
}

func TestScriptedExecutor_StreamAndExecute(t *testing.T) {
	ctx := context.Background()
	exec := newScriptedExecutor()

	err := exec.StreamFile(ctx, "host", 22, "user", "", "local.yaml", "remote.yaml")
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if len(exec.StreamedFiles) != 1 {
		t.Errorf("expected 1 streamed file, got %d", len(exec.StreamedFiles))
	}

	out, err := exec.Execute(ctx, "host", 22, "user", "", "docker --version")
	if err != nil || !strings.Contains(out, "Docker") {
		t.Errorf("expected docker version output, got: %s", out)
	}
}

func TestExtractHostOnly(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"deployer@debian-server", "debian-server"},
		{"debian-server", "debian-server"},
		{"root@192.168.1.100", "192.168.1.100"},
	}
	for _, tc := range tests {
		got := extractHostOnly(tc.input)
		if got != tc.expected {
			t.Errorf("extractHostOnly(%s) = %s, expected %s", tc.input, got, tc.expected)
		}
	}
}

func TestMigrate_FailureBranches(t *testing.T) {
	ctx := context.Background()

	// Data sync error
	execSyncErr := newScriptedExecutor()
	execSyncErr.ExecuteFunc = func(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error) {
		if strings.Contains(cmd, "mkdir") {
			return "", errors.New("read-only filesystem")
		}
		if strings.Contains(cmd, "uname") {
			return "Linux\nx86_64\n", nil
		}
		return "ok", nil
	}
	mSync := NewMigrator(execSyncErr, nil)
	_, err := mSync.Migrate(ctx, MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
	})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize remote stacks") {
		t.Errorf("expected data sync error, got: %v", err)
	}

	// Remote deploy error
	execDeployErr := newScriptedExecutor()
	execDeployErr.ExecuteFunc = func(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error) {
		if strings.Contains(cmd, "docker compose") && strings.Contains(cmd, "up") {
			return "", errors.New("daemon not running")
		}
		if strings.Contains(cmd, "uname") {
			return "Linux\nx86_64\n", nil
		}
		return "ok", nil
	}
	mDeploy := NewMigrator(execDeployErr, nil)
	_, err = mDeploy.Migrate(ctx, MigrationRequest{
		Services:    []string{"plane"},
		TargetHost:  "deployer@debian-server",
		AutoApprove: true,
	})
	if err == nil || !strings.Contains(err.Error(), "failed to start remote service") {
		t.Errorf("expected deploy error, got: %v", err)
	}
}
