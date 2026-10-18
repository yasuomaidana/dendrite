package assert

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PackageCoverage stores statement coverage metrics for an individual package.
type PackageCoverage struct {
	Package      string  `json:"package"`
	CoveredStmts int     `json:"covered_stmts"`
	TotalStmts   int     `json:"total_stmts"`
	Percentage   float64 `json:"percentage"`
}

// CoverageReport aggregates statement coverage results across target packages.
type CoverageReport struct {
	CoveredStmts int                        `json:"covered_stmts"`
	TotalStmts   int                        `json:"total_stmts"`
	Percentage   float64                    `json:"percentage"`
	Packages     map[string]PackageCoverage `json:"packages"`
	Uncovered    []string                   `json:"uncovered,omitempty"`
}

// ChangedPackages inspects git diff against baseRef to determine modified package directories.
func ChangedPackages(repoDir, baseRef string) ([]string, error) {
	if repoDir == "" {
		repoDir = "."
	}

	rawOutput, err := runGitDiff(repoDir, baseRef)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	// New files are part of a change too; git diff does not list untracked files.
	untracked := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	untracked.Dir = repoDir
	if out, uErr := untracked.Output(); uErr == nil {
		rawOutput += "\n" + string(out)
	}

	return extractPackagesFromDiff(rawOutput), nil
}

func runGitDiff(repoDir, baseRef string) (string, error) {
	var args []string
	if baseRef != "" {
		args = []string{"diff", "--name-only", baseRef}
	} else {
		args = []string{"diff", "--name-only", "HEAD"}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}

	// Fallback to git diff without HEAD if HEAD is unavailable
	if baseRef == "" {
		cmdFallback := exec.Command("git", "diff", "--name-only")
		cmdFallback.Dir = repoDir
		fallbackOut, fallbackErr := cmdFallback.Output()
		if fallbackErr == nil {
			return string(fallbackOut), nil
		}
	}

	return "", err
}

func extractPackagesFromDiff(diffOutput string) []string {
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(diffOutput))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ".git/") || strings.HasPrefix(line, ".dendrite/") {
			continue
		}

		dir := filepath.ToSlash(filepath.Dir(line))
		if dir == "." {
			if isSourceFile(line) {
				seen["."] = true
			}
			continue
		}

		seen[dir] = true
	}

	var pkgs []string
	for p := range seen {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs
}

func isSourceFile(filename string) bool {
	ext := filepath.Ext(filename)
	switch ext {
	case ".go", ".rs", ".ts", ".js", ".py", ".c", ".cpp", ".h":
		return true
	default:
		return false
	}
}

// CoverageForPackages parses go coverprofile data and calculates coverage restricted to target packages.
func CoverageForPackages(profileOrPath string, targetPackages []string) (float64, error) {
	report, err := ParseCoverageReport(profileOrPath, targetPackages)
	if err != nil {
		return 0, err
	}
	return report.Percentage, nil
}

// ParseCoverageReport parses coverage profile content and filters metrics for target packages.
func ParseCoverageReport(profileOrPath string, targetPackages []string) (*CoverageReport, error) {
	reader, closer, err := openProfileReader(profileOrPath)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}

	report := &CoverageReport{
		Packages: make(map[string]PackageCoverage),
	}
	targets := normalizeTargetPackages(targetPackages)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || strings.TrimSpace(line) == "" {
			continue
		}

		processCoverageLine(line, targets, report)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading coverage profile: %w", err)
	}

	finalizeReport(report)
	return report, nil
}

func openProfileReader(profileOrPath string) (io.Reader, io.Closer, error) {
	if strings.Contains(profileOrPath, "\n") || strings.HasPrefix(profileOrPath, "mode:") {
		return strings.NewReader(profileOrPath), nil, nil
	}

	info, err := os.Stat(profileOrPath)
	if err == nil && !info.IsDir() {
		f, err := os.Open(profileOrPath)
		if err != nil {
			return nil, nil, err
		}
		return f, f, nil
	}

	return strings.NewReader(profileOrPath), nil, nil
}

func processCoverageLine(line string, targets []string, report *CoverageReport) {
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return
	}

	blockSpec := parts[0]
	filePath := extractFilePath(blockSpec)
	fileDir := filepath.ToSlash(filepath.Dir(filePath))

	if len(targets) > 0 && !matchesAnyTarget(fileDir, targets) {
		return
	}

	numStmts, err1 := strconv.Atoi(parts[1])
	count, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return
	}

	report.TotalStmts += numStmts
	if count > 0 {
		report.CoveredStmts += numStmts
	} else {
		report.Uncovered = append(report.Uncovered, blockSpec)
	}

	pkgKey := fileDir
	pkgCov := report.Packages[pkgKey]
	pkgCov.Package = pkgKey
	pkgCov.TotalStmts += numStmts
	if count > 0 {
		pkgCov.CoveredStmts += numStmts
	}
	report.Packages[pkgKey] = pkgCov
}

func extractFilePath(blockSpec string) string {
	lastColon := strings.LastIndex(blockSpec, ":")
	if lastColon != -1 {
		return blockSpec[:lastColon]
	}
	return blockSpec
}

func finalizeReport(report *CoverageReport) {
	for k, pkg := range report.Packages {
		if pkg.TotalStmts > 0 {
			pkg.Percentage = (float64(pkg.CoveredStmts) / float64(pkg.TotalStmts)) * 100.0
		} else {
			pkg.Percentage = 100.0
		}
		report.Packages[k] = pkg
	}

	if report.TotalStmts == 0 {
		report.Percentage = 100.0
	} else {
		report.Percentage = (float64(report.CoveredStmts) / float64(report.TotalStmts)) * 100.0
	}
}

func normalizeTargetPackages(targets []string) []string {
	var cleaned []string
	for _, t := range targets {
		c := filepath.ToSlash(strings.TrimSpace(t))
		c = strings.TrimPrefix(c, "./")
		c = strings.TrimSuffix(c, "/...")
		c = strings.TrimSuffix(c, "/")
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	return cleaned
}

func matchesAnyTarget(dir string, targets []string) bool {
	cleanDir := strings.TrimPrefix(dir, "./")
	for _, target := range targets {
		if matchesSingleTarget(cleanDir, target) {
			return true
		}
	}
	return false
}

func matchesSingleTarget(dir, target string) bool {
	if dir == target {
		return true
	}
	if strings.HasSuffix(dir, "/"+target) {
		return true
	}
	if strings.HasPrefix(dir, target+"/") {
		return true
	}
	if strings.Contains(dir, "/"+target+"/") {
		return true
	}
	return false
}

// EvaluateCoverage checks whether coverage meets or exceeds the required gate percentage.
func EvaluateCoverage(coverage float64, gate float64) (Verdict, []string) {
	if gate <= 0 {
		gate = DefaultCoverageGate
	}

	if coverage < gate {
		reason := fmt.Sprintf("coverage %.1f%% is below required gate of %.1f%%", coverage, gate)
		return CoverageBelowGate, []string{reason}
	}

	return VerifiedComplete, nil
}
