package immune

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BinaryName is the SkillSpector executable; BinaryEnv overrides where it is found.
const (
	BinaryName = "skillspector"
	BinaryEnv  = "DENDRITE_SKILLSPECTOR_BIN"
)

// ErrScannerMissing means no SkillSpector executable was found.
var ErrScannerMissing = errors.New("skillspector not found: install it with `uv tool install skillspector` or set " + BinaryEnv)

// Scanner runs SkillSpector. The zero value locates the binary on use.
type Scanner struct {
	// Bin is the executable; empty means locate it.
	Bin string
	// Thresholds map a score to a verdict; the zero value means DefaultThresholds.
	Thresholds Thresholds
	// LLM turns on the scanner's model pass (it needs a provider configured in the environment).
	LLM bool
}

// Locate returns the SkillSpector executable: $DENDRITE_SKILLSPECTOR_BIN (which must work if set), then
// PATH, then ~/.local/bin where `uv tool install` puts it.
func Locate() (string, error) {
	if p := os.Getenv(BinaryEnv); p != "" {
		if err := executable(p); err != nil {
			return "", fmt.Errorf("%s=%s: %w", BinaryEnv, p, err)
		}
		return p, nil
	}
	if p, err := exec.LookPath(BinaryName); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		if p := filepath.Join(home, ".local", "bin", BinaryName); executable(p) == nil {
			return p, nil
		}
	}
	return "", ErrScannerMissing
}

func executable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return errors.New("not an executable file")
	}
	return nil
}

// Scan scans one skill directory (or a SKILL.md file) and returns the gate's report.
func (s Scanner) Scan(ctx context.Context, path string) (Report, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Report{}, err
	}
	if _, err := os.Stat(abs); err != nil {
		return Report{}, fmt.Errorf("scan target: %w", err)
	}
	t := s.Thresholds
	if t == (Thresholds{}) {
		t = DefaultThresholds
	}
	if err := t.Validate(); err != nil {
		return Report{}, err
	}
	bin := s.Bin
	if bin == "" {
		if bin, err = Locate(); err != nil {
			return Report{}, err
		}
	}
	out, err := os.MkdirTemp("", "immune-")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(out)
	reportPath := filepath.Join(out, "report.json")
	args := []string{"scan", abs, "--format", "json", "--output", reportPath}
	if !s.LLM {
		args = append(args, "--no-llm")
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	// SkillSpector exits 1 when it finds risk and still writes the report, so the report decides; the exit
	// status only explains a missing one.
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return Report{}, fmt.Errorf("skillspector wrote no report (%v): %s", runErr, strings.TrimSpace(stderr.String()))
	}
	r, err := ParseReport(data, t)
	if err != nil {
		return Report{}, err
	}
	r.Path = abs
	r.ScannerVersion = s.version(ctx, bin)
	return r, nil
}

// version asks the scanner for its version; an unanswered question yields "unknown", not a failure.
func (Scanner) version(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
