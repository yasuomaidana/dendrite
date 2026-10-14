package advisor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type mockExecutor struct {
	responses map[string]string
	err       error
}

func (m *mockExecutor) Execute(_ context.Context, target NodeTarget, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if resp, ok := m.responses[target.ID]; ok {
		return resp, nil
	}
	if resp, ok := m.responses[target.Host]; ok {
		return resp, nil
	}
	if resp, ok := m.responses["default"]; ok {
		return resp, nil
	}
	return "", errors.New("target not found in mock")
}

func TestProbeNode_LinuxServer(t *testing.T) {
	mockOut := "===SECTION:SYS===\nLinux\nx86_64\ndebian-srv\n" +
		"===SECTION:CPU===\n8\n" +
		"===SECTION:DISK===\n104857600\n" +
		"===SECTION:MEM===\nMemTotal:       32768000 kB\nMemAvailable:   24576000 kB\n" +
		"===SECTION:GPU===\nNVIDIA GeForce RTX 4090, 24576 MiB\n"

	exec := &mockExecutor{
		responses: map[string]string{
			"debian": mockOut,
		},
	}

	probe := NewHardwareProbe(exec)
	target := NodeTarget{
		ID:   "debian",
		Host: "100.84.12.10",
		User: "deployer",
	}

	hw, err := probe.ProbeNode(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error probing linux node: %v", err)
	}

	if hw.NodeID != "debian" {
		t.Errorf("expected NodeID 'debian', got '%s'", hw.NodeID)
	}
	if hw.OS != "linux" {
		t.Errorf("expected OS 'linux', got '%s'", hw.OS)
	}
	if hw.Arch != "amd64" {
		t.Errorf("expected Arch 'amd64', got '%s'", hw.Arch)
	}
	if hw.CPUCores != 8 {
		t.Errorf("expected CPUCores 8, got %d", hw.CPUCores)
	}
	if hw.TotalRAMMB < 30000 {
		t.Errorf("expected TotalRAMMB >= 30000, got %d", hw.TotalRAMMB)
	}
	if hw.FreeRAMMB < 20000 {
		t.Errorf("expected FreeRAMMB >= 20000, got %d", hw.FreeRAMMB)
	}
	if hw.DiskFreeGB != 100 {
		t.Errorf("expected DiskFreeGB 100, got %d", hw.DiskFreeGB)
	}
	if !hw.HasGPU || hw.GPUType != GPUTypeCUDA {
		t.Errorf("expected CUDA GPU, got hasGPU=%v, type=%s", hw.HasGPU, hw.GPUType)
	}
	if hw.VRAMMB != 24576 {
		t.Errorf("expected VRAMMB 24576, got %d", hw.VRAMMB)
	}
}

func TestProbeNode_DarwinAppleSilicon(t *testing.T) {
	mockOut := "===SECTION:SYS===\nDarwin\narm64\nmacbook-pro\n" +
		"===SECTION:CPU===\n12\n" +
		"===SECTION:DISK===\n209715200\n" +
		"===SECTION:MEM===\n25769803776\n" +
		"===SECTION:GPU===\nApple M3 Max\n"

	exec := &mockExecutor{
		responses: map[string]string{
			"macbook": mockOut,
		},
	}

	probe := NewHardwareProbe(exec)
	target := NodeTarget{
		ID:      "macbook",
		Host:    "localhost",
		IsLocal: true,
	}

	hw, err := probe.ProbeNode(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error probing darwin node: %v", err)
	}

	if hw.OS != "darwin" {
		t.Errorf("expected OS 'darwin', got '%s'", hw.OS)
	}
	if hw.Arch != "arm64" {
		t.Errorf("expected Arch 'arm64', got '%s'", hw.Arch)
	}
	if hw.CPUCores != 12 {
		t.Errorf("expected CPUCores 12, got %d", hw.CPUCores)
	}
	if hw.TotalRAMMB != 24576 {
		t.Errorf("expected TotalRAMMB 24576, got %d", hw.TotalRAMMB)
	}
	if !hw.HasGPU || hw.GPUType != GPUTypeMetal {
		t.Errorf("expected Metal GPU, got hasGPU=%v, type=%s", hw.HasGPU, hw.GPUType)
	}
}

func TestProbeNode_InvalidTarget(t *testing.T) {
	probe := NewHardwareProbe(&mockExecutor{})
	_, err := probe.ProbeNode(context.Background(), NodeTarget{})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestProbeNode_CommandFailure(t *testing.T) {
	exec := &mockExecutor{err: errors.New("connection timed out")}
	probe := NewHardwareProbe(exec)
	_, err := probe.ProbeNode(context.Background(), NodeTarget{ID: "node1", Host: "node1"})
	if err == nil {
		t.Fatalf("expected error on command failure, got nil")
	}
	if !strings.Contains(err.Error(), "connection timed out") {
		t.Errorf("expected connection timed out in error, got %v", err)
	}
}

func TestProbeAll(t *testing.T) {
	exec := &mockExecutor{
		responses: map[string]string{
			"node1": "===SECTION:SYS===\nLinux\nx86_64\nhost1\n===SECTION:CPU===\n4\n===SECTION:DISK===\n52428800\n===SECTION:MEM===\nMemTotal: 8192000 kB\n===SECTION:GPU===\nNONE\n",
			"node2": "===SECTION:SYS===\nDarwin\narm64\nhost2\n===SECTION:CPU===\n8\n===SECTION:DISK===\n52428800\n===SECTION:MEM===\n17179869184\n===SECTION:GPU===\nApple M2\n",
		},
	}

	probe := NewHardwareProbe(exec)

	// Test empty list
	_, err := probe.ProbeAll(context.Background(), nil)
	if !errors.Is(err, ErrEmptyNodeList) {
		t.Errorf("expected ErrEmptyNodeList, got %v", err)
	}

	// Test concurrent probe
	targets := []NodeTarget{
		{ID: "node1", Host: "10.0.0.1"},
		{ID: "node2", Host: "10.0.0.2"},
	}

	results, err := probe.ProbeAll(context.Background(), targets)
	if err != nil {
		t.Fatalf("unexpected error in ProbeAll: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestDefaultCommandExecutor_Routing(t *testing.T) {
	executor := NewDefaultCommandExecutor()
	if executor.Local == nil || executor.SSH == nil {
		t.Fatalf("expected Local and SSH executors to be non-nil")
	}

	// Local execution test
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := executor.Execute(ctx, NodeTarget{IsLocal: true}, "echo 'dendrite-advisor-test'")
	if err != nil {
		t.Fatalf("local execution failed: %v", err)
	}
	if !strings.Contains(out, "dendrite-advisor-test") {
		t.Errorf("unexpected output: %s", out)
	}
}
