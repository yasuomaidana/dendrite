package installer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Typed error hierarchy for Installer Pre-Flight and Probers.
var (
	ErrMissingRequiredTool     = errors.New("required system tool is missing from PATH")
	ErrUnsupportedArchitecture = errors.New("unsupported operating system or CPU architecture")
	ErrPortConflict            = errors.New("required port conflict detected")
)

// AcceleratorType defines the detected hardware acceleration tier.
type AcceleratorType string

const (
	AcceleratorMetal AcceleratorType = "Apple Silicon Metal GPU"
	AcceleratorCUDA  AcceleratorType = "NVIDIA CUDA GPU"
	AcceleratorCPU   AcceleratorType = "Standard CPU"
)

// ToolStatus captures the availability and version of a local system tool.
type ToolStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Required  bool   `json:"required"`
}

// PortStatus reflects the listening state and latency of a target TCP port.
type PortStatus struct {
	Port      int           `json:"port"`
	Service   string        `json:"service"`
	Listening bool          `json:"listening"`
	Latency   time.Duration `json:"latency,omitempty"`
}

// CanonicalPorts audited in Gate 2.
var CanonicalPorts = map[int]string{
	11434: "Ollama Metal GPU Engine",
	8080:  "OxiLLaMa Pure-Rust GGUF Daemon",
	4000:  "LiteLLM Governance Gateway",
	8090:  "Plane v2 Project Management",
	5432:  "PostgreSQL 16 Control Storage",
	6379:  "Redis 7 State Cache",
}

// PreFlightReport summarizes the results of Gates 1 and 2.
type PreFlightReport struct {
	OS                  string                `json:"os"`
	Arch                string                `json:"arch"`
	NumCPU              int                   `json:"num_cpu"`
	Accelerator         AcceleratorType       `json:"accelerator"`
	AcceleratorBrand    string                `json:"accelerator_brand"`
	Tools               map[string]ToolStatus `json:"tools"`
	Ports               map[int]PortStatus    `json:"ports"`
	AllPrerequisitesMet bool                  `json:"all_prerequisites_met"`
	Warnings            []string              `json:"warnings"`
	Errors              []string              `json:"errors"`
	Timestamp           time.Time             `json:"timestamp"`
}

// PreFlightAuditor orchestrates system prerequisite and port probing.
type PreFlightAuditor struct {
	TargetOS         string
	TargetArch       string
	PortProbeTimeout time.Duration
	ToolLookupFn     func(file string) (string, error)
	CommandRunnerFn  func(name string, args ...string) ([]byte, error)
	TCPDialerFn      func(network, address string, timeout time.Duration) (net.Conn, error)
}

// NewPreFlightAuditor creates a new auditor with production defaults.
func NewPreFlightAuditor() *PreFlightAuditor {
	return &PreFlightAuditor{
		TargetOS:         runtime.GOOS,
		TargetArch:       runtime.GOARCH,
		PortProbeTimeout: 300 * time.Millisecond,
		ToolLookupFn:     exec.LookPath,
		CommandRunnerFn: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		},
		TCPDialerFn: net.DialTimeout,
	}
}

// ProbeHardware identifies the OS, architecture, CPU count, and accelerator.
func (a *PreFlightAuditor) ProbeHardware() (AcceleratorType, string) {
	osName := a.TargetOS
	if osName == "" {
		osName = runtime.GOOS
	}
	arch := a.TargetArch
	if arch == "" {
		arch = runtime.GOARCH
	}

	if osName == "darwin" && arch == "arm64" {
		brand := "Apple Silicon M-series"
		if out, err := a.CommandRunnerFn("sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				brand = trimmed
			}
		}
		return AcceleratorMetal, brand
	}

	// Check for NVIDIA CUDA GPU
	if _, err := a.ToolLookupFn("nvidia-smi"); err == nil {
		brand := "NVIDIA CUDA GPU"
		if out, err := a.CommandRunnerFn("nvidia-smi", "--query-gpu=name", "--format=csv,noheader"); err == nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				brand = trimmed
			}
		}
		return AcceleratorCUDA, brand
	}

	return AcceleratorCPU, fmt.Sprintf("Generic CPU (%s/%s)", osName, arch)
}

// AuditTools verifies required tools (curl, git, python3) and optional tools (cargo, docker).
func (a *PreFlightAuditor) AuditTools() map[string]ToolStatus {
	toolSpecs := []struct {
		name     string
		required bool
	}{
		{"curl", true},
		{"git", true},
		{"python3", true},
		{"cargo", false}, // optional toolchain for OxiLLaMa native compilation
		{"docker", false},
	}

	res := make(map[string]ToolStatus)
	for _, ts := range toolSpecs {
		path, err := a.ToolLookupFn(ts.name)
		installed := err == nil

		var versionStr string
		if installed {
			if out, vErr := a.CommandRunnerFn(ts.name, "--version"); vErr == nil {
				versionStr = strings.TrimSpace(string(out))
				// Keep first line
				if idx := strings.Index(versionStr, "\n"); idx != -1 {
					versionStr = versionStr[:idx]
				}
			}
		}

		res[ts.name] = ToolStatus{
			Name:      ts.name,
			Installed: installed,
			Path:      path,
			Version:   versionStr,
			Required:  ts.required,
		}
	}
	return res
}

// ProbePort checks whether a specific TCP port is listening on the host.
func (a *PreFlightAuditor) ProbePort(host string, port int, service string) PortStatus {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	start := time.Now()
	conn, err := a.TCPDialerFn("tcp", addr, a.PortProbeTimeout)
	if err == nil {
		_ = conn.Close()
		return PortStatus{
			Port:      port,
			Service:   service,
			Listening: true,
			Latency:   time.Since(start),
		}
	}
	return PortStatus{
		Port:      port,
		Service:   service,
		Listening: false,
	}
}

// ProbeAllPorts inspects all canonical FEAT-101 ports on localhost.
func (a *PreFlightAuditor) ProbeAllPorts(host string) map[int]PortStatus {
	if host == "" {
		host = "127.0.0.1"
	}
	res := make(map[int]PortStatus)
	for port, service := range CanonicalPorts {
		res[port] = a.ProbePort(host, port, service)
	}
	return res
}

// RunAudit performs the complete pre-flight audit for Gates 1 and 2.
func (a *PreFlightAuditor) RunAudit(ctx context.Context, host string) (*PreFlightReport, error) {
	osName := a.TargetOS
	if osName == "" {
		osName = runtime.GOOS
	}
	arch := a.TargetArch
	if arch == "" {
		arch = runtime.GOARCH
	}

	accelerator, brand := a.ProbeHardware()
	tools := a.AuditTools()
	ports := a.ProbeAllPorts(host)

	var warnings []string
	var errs []string

	// Validate required tools
	for name, st := range tools {
		if st.Required && !st.Installed {
			errs = append(errs, fmt.Sprintf("Missing required system tool: %s", name))
		}
	}

	// Warn if cargo is missing
	if cargoSt, ok := tools["cargo"]; ok && !cargoSt.Installed {
		warnings = append(warnings, "Cargo (Rust toolchain) not found. Pre-built OxiLLaMa binary will be required.")
	}

	// Warn if running on CPU
	if accelerator == AcceleratorCPU {
		warnings = append(warnings, "No dedicated Metal/CUDA GPU accelerator detected. Local inference will execute on CPU.")
	}

	allPrereqsMet := len(errs) == 0

	rep := &PreFlightReport{
		OS:                  osName,
		Arch:                arch,
		NumCPU:              runtime.NumCPU(),
		Accelerator:         accelerator,
		AcceleratorBrand:    brand,
		Tools:               tools,
		Ports:               ports,
		AllPrerequisitesMet: allPrereqsMet,
		Warnings:            warnings,
		Errors:              errs,
		Timestamp:           time.Now(),
	}

	if !allPrereqsMet {
		return rep, fmt.Errorf("%w: %s", ErrMissingRequiredTool, strings.Join(errs, "; "))
	}

	return rep, nil
}
