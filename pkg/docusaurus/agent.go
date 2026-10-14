package docusaurus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentAuditSummary details the assessment performed by DocusaurusAgent.
type AgentAuditSummary struct {
	TotalFeatures       int      `json:"total_features"`
	DocumentedFeatures  int      `json:"documented_features"`
	UndocumentedModules []string `json:"undocumented_modules"`
	MissingAcceptance   []string `json:"missing_acceptance_criteria"`
	SharingPrepared     bool     `json:"sharing_prepared"`
	Recommendations     []string `json:"recommendations"`
}

// DocusaurusAgent orchestrates documentation synthesis, auditing, and sharing readiness.
type DocusaurusAgent struct {
	projectRoot string
	docsDir     string
}

// NewDocusaurusAgent constructs an agent bounded to a specific project.
func NewDocusaurusAgent(projectRoot, docsDir string) *DocusaurusAgent {
	if docsDir == "" {
		docsDir = filepath.Join(projectRoot, "docs")
	}
	return &DocusaurusAgent{
		projectRoot: projectRoot,
		docsDir:     docsDir,
	}
}

// Audit evaluates documentation completeness against packages and features.
func (a *DocusaurusAgent) Audit(ctx context.Context) (*AgentAuditSummary, error) {
	summary := &AgentAuditSummary{
		UndocumentedModules: make([]string, 0),
		MissingAcceptance:   make([]string, 0),
		Recommendations:     make([]string, 0),
	}

	packages, err := a.discoverPackages()
	if err != nil {
		return nil, fmt.Errorf("package discovery failed: %w", err)
	}

	docFiles, err := a.discoverDocFiles()
	if err != nil {
		return nil, fmt.Errorf("doc discovery failed: %w", err)
	}

	for _, pkg := range packages {
		matched := false
		pkgBase := filepath.Base(pkg)
		for _, doc := range docFiles {
			if strings.Contains(strings.ToLower(doc), strings.ToLower(pkgBase)) {
				matched = true
				break
			}
		}
		if !matched {
			summary.UndocumentedModules = append(summary.UndocumentedModules, pkgBase)
		}
	}

	a.auditFeatureSpecs(docFiles, summary)

	if len(summary.UndocumentedModules) > 0 {
		summary.Recommendations = append(summary.Recommendations,
			fmt.Sprintf("Add architectural reference pages for %d modules: %s",
				len(summary.UndocumentedModules), strings.Join(summary.UndocumentedModules, ", ")))
	}

	summary.SharingPrepared = len(docFiles) >= 5
	if !summary.SharingPrepared {
		summary.Recommendations = append(summary.Recommendations,
			"Add project sharing hub manifest and export archive before publication.")
	}

	return summary, nil
}

func (a *DocusaurusAgent) discoverPackages() ([]string, error) {
	pkgRoot := filepath.Join(a.projectRoot, "pkg")
	entries, err := os.ReadDir(pkgRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var pkgs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			pkgs = append(pkgs, e.Name())
		}
	}
	return pkgs, nil
}

func (a *DocusaurusAgent) discoverDocFiles() ([]string, error) {
	var files []string
	err := filepath.Walk(a.docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".md") || strings.HasSuffix(info.Name(), ".markdown")) {
			rel, _ := filepath.Rel(a.docsDir, path)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return files, nil
}

func (a *DocusaurusAgent) auditFeatureSpecs(docFiles []string, summary *AgentAuditSummary) {
	for _, file := range docFiles {
		if strings.Contains(file, "FEAT-") {
			summary.TotalFeatures++
			fullPath := filepath.Join(a.docsDir, file)
			content, err := os.ReadFile(fullPath)
			if err == nil {
				text := string(content)
				if strings.Contains(text, "Acceptance Criteria") || strings.Contains(text, "AC-") {
					summary.DocumentedFeatures++
				} else {
					summary.MissingAcceptance = append(summary.MissingAcceptance, file)
				}
			}
		}
	}
}
