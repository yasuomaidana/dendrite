package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// GenerateContainerName creates a unique ephemeral sandbox container tag.
func GenerateContainerName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("dendrite-sandbox-%s", hex.EncodeToString(b))
}

// NewDefaultSandboxConfig creates a hardened configuration adhering strictly to FEAT-006.
func NewDefaultSandboxConfig(image string, command []string) *SandboxConfig {
	return &SandboxConfig{
		ContainerName:       GenerateContainerName(),
		Image:               image,
		Command:             command,
		NetworkMode:         "none",
		ReadOnlyRoot:        true,
		DropAllCapabilities: true,
		NoNewPrivileges:     true,
		MemoryLimitMB:       1024,
		CPULimit:            2.0,
		TmpfsSize:           "1G",
		Timeout:             5 * time.Minute,
		Environment:         make(map[string]string),
	}
}

// BuildDockerRunArgs generates the CLI arguments for hardened container execution.
func (c *SandboxConfig) BuildDockerRunArgs() ([]string, error) {
	if strings.TrimSpace(c.Image) == "" {
		return nil, ErrEmptyImage
	}
	if len(c.Command) == 0 {
		return nil, ErrEmptyCommand
	}

	name := c.ContainerName
	if name == "" {
		name = GenerateContainerName()
	}

	args := []string{
		"run",
		"--name", name,
		"--rm",
	}

	if c.DropAllCapabilities {
		args = append(args, "--cap-drop", "ALL")
	}

	if c.NoNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges:true")
	}

	if c.ReadOnlyRoot {
		args = append(args, "--read-only")
	}

	netMode := c.NetworkMode
	if netMode == "" {
		netMode = "none"
	}
	args = append(args, "--network", netMode)

	tmpfs := c.TmpfsSize
	if tmpfs == "" {
		tmpfs = "1G"
	}
	args = append(args, "--tmpfs", fmt.Sprintf("/tmp:rw,noexec,nosuid,size=%s", tmpfs))

	if c.MemoryLimitMB > 0 {
		args = append(args, "-m", fmt.Sprintf("%dm", c.MemoryLimitMB))
	}

	if c.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", c.CPULimit))
	}

	if c.HostScratchDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace:rw", c.HostScratchDir))
		args = append(args, "-w", "/workspace")
	}

	// Environment variables sorted for determinism
	keys := make([]string, 0, len(c.Environment))
	for k := range c.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, c.Environment[k]))
	}

	args = append(args, c.Image)
	args = append(args, c.Command...)

	return args, nil
}
