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
	abs, t, bin, err := s.prepare(path)
	if err != nil {
		return Report{}, err
	}
	data, err := s.run(ctx, bin, abs)
	if err != nil {
		return Report{}, err
	}
	r, err := ParseReport(data, t)
	if err != nil {
		return Report{}, err
	}
	r.Path = abs
	r.ScannerVersion = s.version(ctx, bin)
	return r, nil
}

// prepare checks everything that can be wrong before the scanner starts: the target, the bands and the binary.
func (s Scanner) prepare(path string) (abs string, t Thresholds, bin string, err error) {
	if _, err = os.Stat(path); err != nil {
		return "", t, "", fmt.Errorf("scan target: %w", err)
	}
	if t = s.Thresholds; t == (Thresholds{}) {
		t = DefaultThresholds
	}
	if err = t.Validate(); err != nil {
		return "", t, "", err
	}
	if bin = s.Bin; bin == "" {
		if bin, err = Locate(); err != nil {
			return "", t, "", err
		}
	}
	abs, err = filepath.Abs(path)
	return abs, t, bin, err
}

// run starts the scanner on abs and returns the report it wrote. SkillSpector exits 1 when it finds risk and
// still writes the report, so the report decides; the exit status only explains a missing one.
func (s Scanner) run(ctx context.Context, bin, abs string) ([]byte, error) {
	out, err := os.MkdirTemp("", "immune-")
	if err != nil {
		return nil, err
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
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("skillspector wrote no report (%v): %s", runErr, strings.TrimSpace(stderr.String()))
	}
	return data, nil
}

// version asks the scanner for its version; an unanswered question yields "unknown", not a failure.
func (Scanner) version(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
