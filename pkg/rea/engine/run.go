package engine

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/sandbox"
)

const (
	// maxFunctions bounds the functions kept from an engine's answer.
	maxFunctions = 500
	// detailLimit is how many of the largest functions get their disassembly read.
	detailLimit = 20
	// maxOutput bounds what is read back from one engine run.
	maxOutput = 8 << 20
	// runTimeout bounds one engine run.
	runTimeout = 5 * time.Minute
)

// Result is what an engine run found. Engine is empty when no engine ran, and Note says why.
type Result struct {
	Engine    string     `json:"engine,omitempty"`
	Mode      string     `json:"mode,omitempty"`
	Note      string     `json:"note,omitempty"`
	Functions []Function `json:"functions,omitempty"`
}

// Service runs engines. Exec is the command runner the container goes through (the real one runs `docker`).
type Service struct {
	Env  Env
	Exec sandbox.CommandRunner
}

// exec is the command runner of the service: the one given, or the host's.
func (s *Service) exec() sandbox.CommandRunner {
	if s.Exec != nil {
		return s.Exec
	}
	return &sandbox.OSCommandRunner{}
}

// RunError is an engine that was chosen and started but failed. A refused policy is not a RunError.
type RunError struct {
	Engine, Mode string
	Err          error
}

func (e *RunError) Error() string { return fmt.Sprintf("%s (%s): %v", e.Engine, e.Mode, e.Err) }

// Unwrap exposes the cause.
func (e *RunError) Unwrap() error { return e.Err }

// NewService is the real service: Docker and host binaries as they are on this machine.
func NewService() *Service {
	return &Service{Env: Env{
		DockerUp: func(ctx context.Context) bool {
			return exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").Run() == nil
		},
		HasImage: func(ctx context.Context, image string) bool {
			return exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil
		},
		LookPath: exec.LookPath,
	}}
}

// Run picks an engine by policy for a target of the given format and reads its functions. With no usable engine it
// answers a Result with a note, not an error; an error means the policy was refused or the engine failed.
func (s *Service) Run(ctx context.Context, target, format string, p Policy) (*Result, error) {
	d, err := Select(ctx, p, format, s.Env)
	if err != nil {
		return nil, err
	}
	if d.Choice == nil {
		return &Result{Note: d.Note}, nil
	}
	abs, _ := filepath.Abs(target) // a path that cannot be made absolute fails when the engine opens it
	var fns []Function
	switch d.Choice.Spec.Family {
	case FamilyR2:
		fns, err = s.runR2(ctx, d.Choice, abs)
	case FamilyGhidra:
		fns, err = s.runGhidra(ctx, d.Choice, abs)
	default:
		fns, err = s.runObjdump(ctx, d.Choice, abs)
	}
	if err != nil {
		return nil, &RunError{Engine: d.Choice.Spec.Name, Mode: d.Choice.Mode, Err: err}
	}
	sort.SliceStable(fns, func(i, j int) bool { return fns[i].Size > fns[j].Size })
	if len(fns) > maxFunctions {
		fns = fns[:maxFunctions]
	}
	return &Result{Engine: d.Choice.Spec.Name, Mode: d.Choice.Mode, Functions: fns}, nil
}

// run executes argv (which refers to the target as TargetPath) in the chosen mode and returns its standard output.
func (s *Service) run(ctx context.Context, c *Choice, target string, argv []string) ([]byte, error) {
	timeout := runTimeout
	if c.Spec.Timeout > 0 {
		timeout = c.Spec.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var out []byte
	var err error
	if c.Mode == ModeContainer {
		out, err = s.runContainer(ctx, c.Spec, target, argv)
	} else {
		out, err = runHost(ctx, c.HostBin, target, argv)
	}
	return capOutput(out), err
}

// capOutput keeps at most maxOutput bytes of an engine's answer.
func capOutput(out []byte) []byte {
	if len(out) > maxOutput {
		return out[:maxOutput]
	}
	return out
}

// runContainer runs argv in the hardened sandbox: no network, no capabilities, a read-only root, and the target
// mounted read-only at TargetPath. Standard error is discarded inside the container so it cannot mix into the
// answer.
func (s *Service) runContainer(ctx context.Context, spec Spec, target string, argv []string) ([]byte, error) {
	cfg := sandbox.NewDefaultSandboxConfig(spec.Image, []string{"sh", "-c", shellLine(argv) + " 2>/dev/null"})
	cfg.ReadOnlyMounts = []sandbox.Mount{{Host: target, Container: TargetPath}}
	if spec.MemoryMB > 0 {
		cfg.MemoryLimitMB = spec.MemoryMB
	}
	if spec.Timeout > 0 {
		cfg.Timeout = spec.Timeout
	}
	for k, v := range spec.Env {
		cfg.Environment[k] = v
	}
	sess := sandbox.NewSession(cfg, s.Exec)
	defer func() { _ = sess.Close() }()
	res, err := sess.Execute(ctx)
	return stdoutOf(res), err
}

// stdoutOf is what a container run printed, nothing for a run that never started.
func stdoutOf(res *sandbox.ExecutionResult) []byte {
	if res == nil {
		return nil
	}
	return []byte(res.Stdout)
}

// runHost runs the host binary on the target. Only a trusted or text target gets here (see Select).
func runHost(ctx context.Context, bin, target string, argv []string) ([]byte, error) {
	args := make([]string, 0, len(argv))
	for _, a := range argv[1:] {
		args = append(args, strings.ReplaceAll(a, TargetPath, target))
	}
	return exec.CommandContext(ctx, bin, args...).Output()
}

// shellLine joins argv into a shell command, quoting every word. Words come from this package, never from users;
// the target is always the fixed TargetPath.
func shellLine(argv []string) string {
	words := make([]string, len(argv))
	for i, a := range argv {
		words[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(words, " ")
}
