package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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
