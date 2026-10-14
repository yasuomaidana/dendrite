package advisor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CommandExecutor abstracts remote and local command invocation.
type CommandExecutor interface {
	Execute(ctx context.Context, target NodeTarget, command string) (string, error)
}

// DefaultCommandExecutor routes commands between LocalExecutor and SSHExecutor.
type DefaultCommandExecutor struct {
	Local *LocalExecutor
	SSH   *SSHExecutor
}

func NewDefaultCommandExecutor() *DefaultCommandExecutor {
	return &DefaultCommandExecutor{
		Local: &LocalExecutor{},
		SSH:   &SSHExecutor{},
	}
}

func (e *DefaultCommandExecutor) Execute(ctx context.Context, target NodeTarget, command string) (string, error) {
	if target.IsLocal || target.Host == "localhost" || target.Host == "127.0.0.1" || target.Host == "" {
		return e.Local.Execute(ctx, target, command)
	}
	return e.SSH.Execute(ctx, target, command)
}

// LocalExecutor executes commands directly on the host operating system.
type LocalExecutor struct{}

func (e *LocalExecutor) Execute(ctx context.Context, _ NodeTarget, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %v: %s", ErrCommandExecution, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// SSHExecutor executes commands on remote nodes via standard ssh client.
type SSHExecutor struct{}

func (e *SSHExecutor) Execute(ctx context.Context, target NodeTarget, command string) (string, error) {
	port := target.Port
	if port <= 0 {
		port = 22
	}
	user := target.User
	if user == "" {
		user = "deployer"
	}
	destination := fmt.Sprintf("%s@%s", user, target.Host)

	args := []string{
		"-p", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=5",
	}
	if target.KeyPath != "" {
		args = append(args, "-i", target.KeyPath)
	}
	args = append(args, destination, command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w (%s): %v: %s", ErrUnreachableNode, target.Host, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// HardwareProbe manages host metrics discovery across targets.
type HardwareProbe struct {
	executor CommandExecutor
}

// NewHardwareProbe creates an initialized HardwareProbe.
func NewHardwareProbe(executor CommandExecutor) *HardwareProbe {
	if executor == nil {
		executor = NewDefaultCommandExecutor()
	}
	return &HardwareProbe{executor: executor}
}

// ProbeNode gathers hardware profile metrics for a given NodeTarget.
func (p *HardwareProbe) ProbeNode(ctx context.Context, target NodeTarget) (*NodeHardware, error) {
	if target.ID == "" && target.Host == "" {
		return nil, ErrInvalidTarget
	}

	start := time.Now()
	// Combined discovery script using explicit section delimiters
	script := `echo "===SECTION:SYS===" && uname -s && uname -m && hostname && ` +
		`echo "===SECTION:CPU===" && (nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo "2") && ` +
		`echo "===SECTION:DISK===" && (df -k / | awk 'NR==2 {print $4}') && ` +
		`echo "===SECTION:MEM===" && (cat /proc/meminfo 2>/dev/null || sysctl -n hw.memsize 2>/dev/null || echo "") && ` +
		`echo "===SECTION:GPU===" && (nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null || ` +
		`system_profiler SPDisplaysDataType 2>/dev/null || echo "NONE")`

	out, err := p.executor.Execute(ctx, target, script)
	if err != nil {
		return nil, fmt.Errorf("probe failed for target %s: %w", target.ID, err)
	}
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	nodeID := target.ID
	if nodeID == "" {
		nodeID = target.Host
	}

	hw := &NodeHardware{
		NodeID:    nodeID,
		LatencyMs: latency,
		IsLocal:   target.IsLocal,
		Metadata:  make(map[string]string),
	}

	parseProbeOutput(out, hw)
	return hw, nil
}

// ProbeAll executes concurrent discovery across multiple targets.
func (p *HardwareProbe) ProbeAll(ctx context.Context, targets []NodeTarget) ([]*NodeHardware, error) {
	if len(targets) == 0 {
		return nil, ErrEmptyNodeList
	}

	results := make([]*NodeHardware, len(targets))
	errs := make([]error, len(targets))

	type probeResult struct {
		idx int
		hw  *NodeHardware
		err error
	}
	ch := make(chan probeResult, len(targets))

	for i, t := range targets {
		go func(idx int, tgt NodeTarget) {
			hw, err := p.ProbeNode(ctx, tgt)
			ch <- probeResult{idx: idx, hw: hw, err: err}
		}(i, t)
	}

	var firstErr error
	for i := 0; i < len(targets); i++ {
		res := <-ch
		results[res.idx] = res.hw
		errs[res.idx] = res.err
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}

	var successful []*NodeHardware
	for _, hw := range results {
		if hw != nil {
			successful = append(successful, hw)
		}
	}

	if len(successful) == 0 {
		return nil, fmt.Errorf("all probes failed: %w", firstErr)
	}

	return successful, nil
}

func parseProbeOutput(output string, hw *NodeHardware) {
	lines := strings.Split(output, "\n")
	currentSection := ""
	sectionLines := make(map[string][]string)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "===SECTION:") && strings.HasSuffix(trimmed, "===") {
			currentSection = strings.TrimSuffix(strings.TrimPrefix(trimmed, "===SECTION:"), "===")
			continue
		}
		if currentSection != "" && trimmed != "" {
			sectionLines[currentSection] = append(sectionLines[currentSection], trimmed)
		}
	}

	if body, ok := sectionLines["SYS"]; ok {
		if len(body) > 0 {
			hw.OS = strings.ToLower(strings.TrimSpace(body[0]))
		}
		if len(body) > 1 {
			hw.Arch = normalizeArch(body[1])
		}
		if len(body) > 2 {
			hw.Hostname = strings.TrimSpace(body[2])
		}
	}

	if body, ok := sectionLines["CPU"]; ok && len(body) > 0 {
		if cores, err := strconv.Atoi(strings.TrimSpace(body[0])); err == nil {
			hw.CPUCores = cores
		}
	}

	if body, ok := sectionLines["DISK"]; ok && len(body) > 0 {
		if kb, err := strconv.ParseUint(strings.TrimSpace(body[0]), 10, 64); err == nil {
			hw.DiskFreeGB = kb / (1024 * 1024)
		}
	}

	if body, ok := sectionLines["MEM"]; ok {
		parseMemorySection(body, hw)
	}

	if body, ok := sectionLines["GPU"]; ok {
		parseGPUSection(body, hw)
	}

	if hw.CPUCores == 0 {
		hw.CPUCores = 2
	}
	if hw.DiskFreeGB == 0 {
		hw.DiskFreeGB = 10
	}
}

func parseMemorySection(lines []string, hw *NodeHardware) {
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "MemTotal:") {
		// Linux /proc/meminfo format
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if parts[0] == "MemTotal:" {
					if kb, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
						hw.TotalRAMMB = kb / 1024
					}
				}
				if parts[0] == "MemAvailable:" {
					if kb, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
						hw.FreeRAMMB = kb / 1024
					}
				}
			}
		}
		if hw.FreeRAMMB == 0 && hw.TotalRAMMB > 0 {
			hw.FreeRAMMB = hw.TotalRAMMB / 2
		}
	} else if len(lines) > 0 {
		// Darwin sysctl byte string or fallback
		first := strings.TrimSpace(lines[0])
		if bytesVal, err := strconv.ParseUint(first, 10, 64); err == nil && bytesVal > 0 {
			hw.TotalRAMMB = bytesVal / (1024 * 1024)
			hw.FreeRAMMB = hw.TotalRAMMB * 6 / 10
		}
	}
	if hw.TotalRAMMB == 0 {
		hw.TotalRAMMB = 8192
		hw.FreeRAMMB = 4096
	}
}

func parseGPUSection(lines []string, hw *NodeHardware) {
	joined := strings.ToLower(strings.Join(lines, " "))

	// Check CUDA / NVIDIA
	if strings.Contains(joined, "nvidia") || strings.Contains(joined, "tesla") || strings.Contains(joined, "geforce") || strings.Contains(joined, "rtx") || strings.Contains(joined, "a100") {
		hw.HasGPU = true
		hw.GPUType = GPUTypeCUDA
		hw.VRAMMB = 16384
		for _, line := range lines {
			if strings.Contains(line, "MiB") || strings.Contains(line, "MB") {
				fields := strings.Split(line, ",")
				if len(fields) >= 2 {
					memStr := strings.TrimSpace(strings.ReplaceAll(fields[1], "MiB", ""))
					memStr = strings.TrimSpace(strings.ReplaceAll(memStr, "MB", ""))
					if vram, err := strconv.ParseUint(memStr, 10, 64); err == nil {
						hw.VRAMMB = vram
					}
				}
			}
		}
		return
	}

	// Check Apple Silicon Metal
	if hw.OS == "darwin" && (hw.Arch == "arm64" || strings.Contains(joined, "apple") || strings.Contains(joined, "metal")) {
		hw.HasGPU = true
		hw.GPUType = GPUTypeMetal
		hw.VRAMMB = hw.TotalRAMMB * 3 / 4
		return
	}

	hw.HasGPU = false
	hw.GPUType = GPUTypeNone
	hw.VRAMMB = 0
}

func normalizeArch(arch string) string {
	a := strings.ToLower(strings.TrimSpace(arch))
	switch a {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return a
	}
}
