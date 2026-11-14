package coverage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PackageCoverage stores measured coverage for a package.
type PackageCoverage struct {
	Package      string  `json:"package"`
	CoveragePct  float64 `json:"coverage_pct"`
	NoTests      bool    `json:"no_tests,omitempty"`
	NoStatements bool    `json:"no_statements,omitempty"`
}

// Regression records a package that dropped below its baseline.
type Regression struct {
	Package      string  `json:"package"`
	CurrentPct   float64 `json:"current_pct"`
	BaselinePct  float64 `json:"baseline_pct"`
	IsNewPackage bool    `json:"is_new_package,omitempty"`
}

// Result summarizes the coverage ratchet run.
type Result struct {
	Passed         bool                       `json:"passed"`
	TotalPackages  int                        `json:"total_packages"`
	PackagesAt100  int                        `json:"packages_at_100"`
	GlobalCoverage float64                    `json:"global_coverage"`
	Offenders      []Regression               `json:"offenders,omitempty"`
	Baseline       map[string]float64         `json:"baseline"`
	Current        map[string]PackageCoverage `json:"current"`
}

// Options configures the coverage ratchet runner.
type Options struct {
	RepoDir      string
	BaselinePath string
	Ratchet      bool
	Update       bool
	// MinPercent, when above 0, replaces the baseline comparison with a floor: a package below it fails.
	MinPercent float64
	// ExemptGlobs are package globs left out of the floor (test helpers always are).
	ExemptGlobs   []string
	Packages      []string
	CommandRunner func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	Stdout        io.Writer
	Stderr        io.Writer
}

// LoadBaseline loads the coverage baseline JSON file.
func LoadBaseline(path string) (map[string]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try direct map format first: { "cmd/dendrite": 1.6 }
	var direct map[string]float64
	if err := json.Unmarshal(data, &direct); err == nil && direct != nil {
		return direct, nil
	}

	// Try structured format: { "packages": { "cmd/dendrite": 1.6 } }
	var structured struct {
		Packages map[string]float64 `json:"packages"`
	}
	if err := json.Unmarshal(data, &structured); err == nil && structured.Packages != nil {
		return structured.Packages, nil
	}

	return nil, fmt.Errorf("invalid coverage baseline JSON in %s", path)
}

// SaveBaseline writes the coverage baseline map to disk in sorted JSON format.
func SaveBaseline(path string, baselines map[string]float64) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(baselines, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// DetectModulePath parses the go.mod file in repoDir to find the module path.
func DetectModulePath(repoDir string) string {
	modPath := filepath.Join(repoDir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
		}
	}
	return ""
}

func normalizePackage(pkg, modulePath string) string {
	pkg = strings.TrimSpace(pkg)
	if modulePath != "" {
		if pkg == modulePath {
			return "."
		}
		prefix := modulePath + "/"
		if strings.HasPrefix(pkg, prefix) {
			return strings.TrimPrefix(pkg, prefix)
		}
	}
	return pkg
}

func roundPct(val float64) float64 {
	return math.Round(val*10) / 10
}

var (
	okCoverageRe     = regexp.MustCompile(`^ok\s+(\S+).*\bcoverage:\s+([0-9.]+)%\s+of\s+statements`)
	okNoStatementsRe = regexp.MustCompile(`^ok\s+(\S+).*\bcoverage:\s+\[no statements\]`)
	noTestsRe        = regexp.MustCompile(`^\?\s+(\S+).*\[no test files\]`)
	// Go >= 1.22 with -cover reports packages without tests as an indented "<pkg> coverage: 0.0% of statements" line.
	bareCoverageRe = regexp.MustCompile(`^(\S+)\s+coverage:\s+([0-9.]+)%\s+of\s+statements`)
	failPkgRe      = regexp.MustCompile(`^FAIL\s+(\S+)`)
)

// ParseCoverageOutput parses stdout from `go test -cover`.
func ParseCoverageOutput(output string, modulePath string) (map[string]PackageCoverage, []string) {
	result := make(map[string]PackageCoverage)
	var failedPkgs []string

	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		if m := okCoverageRe.FindStringSubmatch(line); len(m) == 3 {
			pkg := normalizePackage(m[1], modulePath)
			pct, _ := strconv.ParseFloat(m[2], 64)
			result[pkg] = PackageCoverage{
				Package:     pkg,
				CoveragePct: roundPct(pct),
			}
		} else if m := okNoStatementsRe.FindStringSubmatch(line); len(m) == 2 {
			pkg := normalizePackage(m[1], modulePath)
			result[pkg] = PackageCoverage{
				Package:      pkg,
				CoveragePct:  100.0,
				NoStatements: true,
			}
		} else if m := noTestsRe.FindStringSubmatch(line); len(m) == 2 {
			pkg := normalizePackage(m[1], modulePath)
			result[pkg] = PackageCoverage{
				Package:     pkg,
				CoveragePct: 0.0,
				NoTests:     true,
			}
		} else if m := bareCoverageRe.FindStringSubmatch(line); len(m) == 3 {
			pkg := normalizePackage(m[1], modulePath)
			pct, _ := strconv.ParseFloat(m[2], 64)
			result[pkg] = PackageCoverage{
				Package:     pkg,
				CoveragePct: roundPct(pct),
				NoTests:     true,
			}
		} else if m := failPkgRe.FindStringSubmatch(line); len(m) == 2 {
			rawPkg := m[1]
			if !strings.HasPrefix(rawPkg, ":") && !strings.HasPrefix(rawPkg, "Test") {
				pkg := normalizePackage(rawPkg, modulePath)
				failedPkgs = append(failedPkgs, pkg)
			}
		}
	}

	return result, failedPkgs
}

// CalculateProgress calculates global progress metrics towards 100%.
func CalculateProgress(current map[string]PackageCoverage) (packagesAt100 int, totalPackages int, globalCoverage float64) {
	totalPackages = len(current)
	if totalPackages == 0 {
		return 0, 0, 0.0
	}
	var sum float64
	for _, cov := range current {
		if cov.CoveragePct >= 99.99 {
			packagesAt100++
		}
		sum += cov.CoveragePct
	}
	globalCoverage = roundPct(sum / float64(totalPackages))
	return packagesAt100, totalPackages, globalCoverage
}

// CompareAndRatchet compares measured coverage against baseline.
func CompareAndRatchet(
	current map[string]PackageCoverage,
	baseline map[string]float64,
	baselineExists bool,
	update bool,
) (offenders []Regression, newBaseline map[string]float64) {
	newBaseline = make(map[string]float64)

	// AC-2: If baseline doesn't exist yet, initialize from current
	if !baselineExists {
		for pkg, cov := range current {
			newBaseline[pkg] = roundPct(cov.CoveragePct)
		}
		return nil, newBaseline
	}

	// Copy existing baselines, preserving 100%
	for pkg, baseCov := range baseline {
		if baseCov >= 100.0 {
			newBaseline[pkg] = 100.0
		} else {
			newBaseline[pkg] = roundPct(baseCov)
		}
	}

	pkgNames := make([]string, 0, len(current))
	for pkg := range current {
		pkgNames = append(pkgNames, pkg)
	}
	sort.Strings(pkgNames)

	for _, pkg := range pkgNames {
		cov := roundPct(current[pkg].CoveragePct)
		baseCov, exists := baseline[pkg]

		if !exists {
			// AC-2: "new packages must start at 100%"
			targetBaseline := 100.0
			if update {
				newBaseline[pkg] = targetBaseline
			}
			if cov < targetBaseline-0.001 {
				offenders = append(offenders, Regression{
					Package:      pkg,
					CurrentPct:   cov,
					BaselinePct:  targetBaseline,
					IsNewPackage: true,
				})
			}
		} else {
			// AC-2: "packages at 100% must stay at 100%"
			if baseCov >= 100.0 {
				baseCov = 100.0
			}

			// AC-1: Check if dropped below baseline
			if cov < baseCov-0.001 {
				offenders = append(offenders, Regression{
					Package:      pkg,
					CurrentPct:   cov,
					BaselinePct:  baseCov,
					IsNewPackage: false,
				})
			}

			// AC-2: "Improvements raise the baseline automatically with --update"
			if update {
				if cov > baseCov {
					newBaseline[pkg] = cov
				} else {
					newBaseline[pkg] = baseCov
				}
			}
		}
	}

	return offenders, newBaseline
}

// DefaultCommandRunner executes a shell command.
func DefaultCommandRunner(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// Run executes the coverage ratchet workflow.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.RepoDir == "" {
		opts.RepoDir = "."
	}
	if opts.BaselinePath == "" {
		opts.BaselinePath = ".github/coverage_baseline.json"
	}
	if opts.CommandRunner == nil {
		opts.CommandRunner = DefaultCommandRunner
	}
	if len(opts.Packages) == 0 {
		opts.Packages = []string{"./..."}
	}

	baselineFullPath := opts.BaselinePath
	if !filepath.IsAbs(baselineFullPath) {
		baselineFullPath = filepath.Join(opts.RepoDir, opts.BaselinePath)
	}

	baseline, err := LoadBaseline(baselineFullPath)
	baselineExists := err == nil
	if err != nil && !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load coverage baseline: %w", err)
	}
	if !baselineExists {
		baseline = make(map[string]float64)
	}

	if !baselineExists && opts.Ratchet && !opts.Update {
		fmt.Fprintf(opts.Stderr, "[-] Coverage baseline not found at %s. Run with --update to initialize baseline.\n", opts.BaselinePath)
		return &Result{
			Passed:   false,
			Baseline: baseline,
		}, nil
	}

	cmdArgs := append([]string{"test", "-cover"}, opts.Packages...)
	fmt.Fprintf(opts.Stdout, "[+] Running go %s in %s...\n", strings.Join(cmdArgs, " "), opts.RepoDir)
	output, cmdErr := opts.CommandRunner(ctx, opts.RepoDir, "go", cmdArgs...)

	modulePath := DetectModulePath(opts.RepoDir)
	current, failedPkgs := ParseCoverageOutput(string(output), modulePath)

	at100, total, globalCov := CalculateProgress(current)
	fmt.Fprintf(opts.Stdout, "[+] Coverage Ratchet Progress: %d/%d packages at 100.0%% (Global Average: %.1f%%)\n", at100, total, globalCov)

	offenders, newBaseline := CompareAndRatchet(current, baseline, baselineExists, opts.Update)
	offenders = withoutExempt(offenders, opts.ExemptGlobs)
	if opts.MinPercent > 0 {
		offenders = Floor(current, opts.MinPercent, func(pkg string) bool { return IsExempt(pkg, opts.ExemptGlobs) })
	}

	if opts.Update {
		if saveErr := SaveBaseline(baselineFullPath, newBaseline); saveErr != nil {
			return nil, fmt.Errorf("failed to save baseline: %w", saveErr)
		}
		if !baselineExists {
			fmt.Fprintf(opts.Stdout, "[✓] Initialized coverage baseline at %s (%d packages recorded)\n", opts.BaselinePath, len(newBaseline))
		} else {
			fmt.Fprintf(opts.Stdout, "[✓] Updated coverage baseline saved to %s (%d packages recorded)\n", opts.BaselinePath, len(newBaseline))
		}
	}

	passed := true
	if cmdErr != nil || len(failedPkgs) > 0 {
		passed = false
		fmt.Fprintf(opts.Stderr, "[-] Test execution failed:\n%s\n", string(output))
		if len(failedPkgs) > 0 {
			fmt.Fprintf(opts.Stderr, "[-] Failed package(s):\n")
			for _, fp := range failedPkgs {
				fmt.Fprintf(opts.Stderr, "    * %s\n", fp)
			}
		}
	}

	if opts.MinPercent > 0 && len(offenders) > 0 {
		passed = false
		fmt.Fprintf(opts.Stderr, "[-] Coverage floor violation: %d package(s) below %.1f%%:\n", len(offenders), opts.MinPercent)
		for _, off := range offenders {
			fmt.Fprintf(opts.Stderr, "    * %s: %.1f%%\n", off.Package, off.CurrentPct)
		}
	} else if opts.Ratchet && len(offenders) > 0 {
		passed = false
		fmt.Fprintf(opts.Stderr, "[-] Coverage ratchet violation: %d package(s) dropped below baseline:\n", len(offenders))
		for _, off := range offenders {
			if off.IsNewPackage {
				fmt.Fprintf(opts.Stderr, "    * %s: %.1f%% < 100.0%% (new packages must start at 100.0%%)\n", off.Package, off.CurrentPct)
			} else {
				fmt.Fprintf(opts.Stderr, "    * %s: %.1f%% < %.1f%% (dropped by %.1f%%)\n", off.Package, off.CurrentPct, off.BaselinePct, roundPct(off.BaselinePct-off.CurrentPct))
			}
		}
	}

	if passed {
		fmt.Fprintln(opts.Stdout, "[✓] Coverage ratchet check passed.")
	}

	resultBaseline := baseline
	if opts.Update {
		resultBaseline = newBaseline
	}

	return &Result{
		Passed:         passed,
		TotalPackages:  total,
		PackagesAt100:  at100,
		GlobalCoverage: globalCov,
		Offenders:      offenders,
		Baseline:       resultBaseline,
		Current:        current,
	}, nil
}
