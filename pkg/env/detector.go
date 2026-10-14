package env

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultCommandRunner executes commands using standard os/exec.
type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) Run(ctx context.Context, cmdStr string, env map[string]string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	if len(env) > 0 {
		for k, v := range env {
			cmd.Env = append(cmd.Environ(), k+"="+v)
		}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *DefaultCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Detector probes the host operating system and tools.
type Detector struct {
	runner CommandRunner
}

// NewDetector creates a Detector with the specified or default CommandRunner.
func NewDetector(runner CommandRunner) *Detector {
	if runner == nil {
		runner = &DefaultCommandRunner{}
	}
	return &Detector{runner: runner}
}

// Detect inspects the system environment and returns a populated HostInfo struct.
func (d *Detector) Detect(ctx context.Context) (*HostInfo, error) {
	info := &HostInfo{
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		PackageManager: PkgUnknown,
	}

	// Detect package manager
	if info.OS == "darwin" {
		if _, err := d.runner.LookPath("brew"); err == nil {
			info.PackageManager = PkgBrew
		}
	} else if info.OS == "linux" {
		if _, err := d.runner.LookPath("apt-get"); err == nil {
			info.PackageManager = PkgApt
		} else if _, err := d.runner.LookPath("dnf"); err == nil {
			info.PackageManager = PkgDnf
		} else if _, err := d.runner.LookPath("pacman"); err == nil {
			info.PackageManager = PkgPacman
		}
	}

	// Probe tools
	if _, err := d.runner.LookPath("docker"); err == nil {
		info.HasDocker = true
	}
	// Check docker compose (subcommand or standalone docker-compose)
	if info.HasDocker {
		if out, err := d.runner.Run(ctx, "docker compose version", nil); err == nil && strings.Contains(strings.ToLower(out), "docker compose") {
			info.HasDockerComp = true
		} else if _, err := d.runner.LookPath("docker-compose"); err == nil {
			info.HasDockerComp = true
		}
	}

	if _, err := d.runner.LookPath("ollama"); err == nil {
		info.HasOllama = true
	}
	if _, err := d.runner.LookPath("go"); err == nil {
		info.HasGo = true
	}
	if _, err := d.runner.LookPath("node"); err == nil {
		info.HasNode = true
	}
	if _, err := d.runner.LookPath("npm"); err == nil {
		info.HasNpm = true
	}
	if _, err := d.runner.LookPath("tailscale"); err == nil {
		info.HasTailscale = true
	}
	if _, err := d.runner.LookPath("linear"); err == nil {
		info.HasLinearCLI = true
	}

	return info, nil
}
