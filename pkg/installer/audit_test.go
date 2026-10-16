package installer

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewPreFlightAuditor(t *testing.T) {
	auditor := NewPreFlightAuditor()
	if auditor == nil {
		t.Fatal("expected non-nil auditor")
	}
	if auditor.PortProbeTimeout <= 0 {
		t.Errorf("expected positive timeout, got %v", auditor.PortProbeTimeout)
	}
}

func TestPreFlightAuditor_ProbeHardware(t *testing.T) {
	auditor := NewPreFlightAuditor()

	t.Run("default runtime hardware probe", func(t *testing.T) {
		accel, brand := auditor.ProbeHardware()
		if accel == "" || brand == "" {
			t.Errorf("expected non-empty accelerator and brand, got %q, %q", accel, brand)
		}
	})

	t.Run("simulated CUDA hardware", func(t *testing.T) {
		custom := &PreFlightAuditor{
			TargetOS:   "linux",
			TargetArch: "amd64",
			ToolLookupFn: func(file string) (string, error) {
				if file == "nvidia-smi" {
					return "/usr/bin/nvidia-smi", nil
				}
				return "", exec.ErrNotFound
			},
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				if name == "nvidia-smi" {
					return []byte("NVIDIA RTX 4090\n"), nil
				}
				return nil, errors.New("command failed")
			},
		}

		accel, brand := custom.ProbeHardware()
		if accel != AcceleratorCUDA {
			t.Errorf("expected AcceleratorCUDA, got %s", accel)
		}
		if !strings.Contains(brand, "NVIDIA RTX 4090") {
			t.Errorf("expected brand to contain 'NVIDIA RTX 4090', got %s", brand)
		}
	})

	t.Run("simulated generic CPU fallback", func(t *testing.T) {
		custom := &PreFlightAuditor{
			TargetOS:   "linux",
			TargetArch: "amd64",
			ToolLookupFn: func(file string) (string, error) {
				return "", exec.ErrNotFound
			},
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("command failed")
			},
		}

		accel, brand := custom.ProbeHardware()
		if accel != AcceleratorCPU {
			t.Errorf("expected AcceleratorCPU, got %s", accel)
		}
		if !strings.Contains(brand, "Generic CPU") {
			t.Errorf("expected brand to contain 'Generic CPU', got %s", brand)
		}
	})

	t.Run("simulated darwin without sysctl brand output", func(t *testing.T) {
		custom := &PreFlightAuditor{
			TargetOS:   "darwin",
			TargetArch: "arm64",
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("sysctl failed")
			},
		}
		accel, brand := custom.ProbeHardware()
		if accel != AcceleratorMetal || brand != "Apple Silicon M-series" {
			t.Errorf("expected default brand on sysctl failure, got %s / %s", accel, brand)
		}
	})

	t.Run("empty TargetOS and TargetArch fallback to runtime", func(t *testing.T) {
		emptyAuditor := &PreFlightAuditor{
			ToolLookupFn: exec.LookPath,
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("command failed")
			},
			TCPDialerFn: func(network, address string, timeout time.Duration) (net.Conn, error) {
				return nil, errors.New("dial failed")
			},
		}
		accel, _ := emptyAuditor.ProbeHardware()
		if accel == "" {
			t.Errorf("expected non-empty accelerator")
		}
		_, _ = emptyAuditor.RunAudit(context.Background(), "127.0.0.1")
	})
}

func TestPreFlightAuditor_AuditTools(t *testing.T) {
	t.Run("real system tools audit", func(t *testing.T) {
		auditor := NewPreFlightAuditor()
		tools := auditor.AuditTools()

		for _, name := range []string{"curl", "git", "python3"} {
			st, ok := tools[name]
			if !ok {
				t.Errorf("missing tool status for %s", name)
			}
			if !st.Installed {
				t.Logf("note: %s not installed in test environment", name)
			}
		}
	})

	t.Run("custom tool lookup and version parsing", func(t *testing.T) {
		custom := &PreFlightAuditor{
			ToolLookupFn: func(file string) (string, error) {
				if file == "curl" {
					return "/usr/bin/curl", nil
				}
				return "", exec.ErrNotFound
			},
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				if name == "curl" {
					return []byte("curl 8.4.0 (x86_64-apple-darwin23.0)\nRelease-Date: 2023-10-11\n"), nil
				}
				return nil, errors.New("not found")
			},
		}

		tools := custom.AuditTools()
		curlSt := tools["curl"]
		if !curlSt.Installed || curlSt.Version != "curl 8.4.0 (x86_64-apple-darwin23.0)" {
			t.Errorf("unexpected curl status: %+v", curlSt)
		}
		gitSt := tools["git"]
		if gitSt.Installed {
			t.Errorf("expected git to be not installed")
		}
	})
}

func TestPreFlightAuditor_ProbePort_And_ProbeAllPorts(t *testing.T) {
	// Start an ephemeral TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on ephemeral port: %v", err)
	}
	defer ln.Close()

	tcpAddr := ln.Addr().(*net.TCPAddr)
	port := tcpAddr.Port

	auditor := NewPreFlightAuditor()

	t.Run("probe listening port", func(t *testing.T) {
		st := auditor.ProbePort("127.0.0.1", port, "Test Service")
		if !st.Listening {
			t.Errorf("expected port %d to be listening", port)
		}
		if st.Service != "Test Service" {
			t.Errorf("expected service 'Test Service', got %s", st.Service)
		}
		if st.Latency < 0 {
			t.Errorf("expected non-negative latency, got %v", st.Latency)
		}
	})

	t.Run("probe closed port", func(t *testing.T) {
		// Pick a port that is closed
		closedPort := port + 1000
		st := auditor.ProbePort("127.0.0.1", closedPort, "Closed Service")
		if st.Listening {
			t.Errorf("expected closed port %d to not be listening", closedPort)
		}
	})

	t.Run("probe all ports default host", func(t *testing.T) {
		ports := auditor.ProbeAllPorts("")
		if len(ports) != len(CanonicalPorts) {
			t.Errorf("expected %d canonical ports, got %d", len(CanonicalPorts), len(ports))
		}
	})
}

func TestPreFlightAuditor_RunAudit(t *testing.T) {
	ctx := context.Background()

	t.Run("successful audit with real tools", func(t *testing.T) {
		auditor := NewPreFlightAuditor()
		rep, err := auditor.RunAudit(ctx, "127.0.0.1")
		if err != nil {
			t.Fatalf("unexpected audit error: %v", err)
		}
		if !rep.AllPrerequisitesMet {
			t.Errorf("expected all prerequisites met, errors: %v", rep.Errors)
		}
		if len(rep.Tools) == 0 {
			t.Errorf("expected audited tools")
		}
		if len(rep.Ports) == 0 {
			t.Errorf("expected audited ports")
		}
	})

	t.Run("missing required tool triggers error", func(t *testing.T) {
		custom := &PreFlightAuditor{
			TargetOS:         "linux",
			TargetArch:       "amd64",
			PortProbeTimeout: 10 * time.Millisecond,
			ToolLookupFn: func(file string) (string, error) {
				return "", exec.ErrNotFound // all tools missing
			},
			CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("command not found")
			},
			TCPDialerFn: func(network, address string, timeout time.Duration) (net.Conn, error) {
				return nil, errors.New("connection refused")
			},
		}

		rep, err := custom.RunAudit(ctx, "127.0.0.1")
		if err == nil {
			t.Fatalf("expected error for missing required tools, got nil")
		}
		if !errors.Is(err, ErrMissingRequiredTool) {
			t.Errorf("expected ErrMissingRequiredTool, got %v", err)
		}
		if rep.AllPrerequisitesMet {
			t.Errorf("expected AllPrerequisitesMet = false")
		}
		if len(rep.Errors) == 0 {
			t.Errorf("expected error messages in report")
		}
		if len(rep.Warnings) == 0 {
			t.Errorf("expected cargo and CPU warnings")
		}
	})
}
