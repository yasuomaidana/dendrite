package distcodebase

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
)

// ProjectSpawner handles disk scaffolding, Git initialization, workspace registration, and Plane project creation.
type ProjectSpawner struct {
	planeClient       *plane.Client
	workspaceRegistry *workspace.Registry
}

// NewProjectSpawner creates a spawner configured with optional Plane and workspace registry clients.
func NewProjectSpawner(planeClient *plane.Client, wsRegistry *workspace.Registry) *ProjectSpawner {
	return &ProjectSpawner{
		planeClient:       planeClient,
		workspaceRegistry: wsRegistry,
	}
}

// Spawn creates a new sub-project or satellite project on disk, initializes git, registers workspace, and creates Plane project.
func (s *ProjectSpawner) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResult, error) {
	if strings.TrimSpace(req.ProjectName) == "" {
		return nil, ErrEmptyProjectName
	}

	targetDir := req.TargetDir
	if targetDir == "" {
		targetDir = filepath.Join(".", "projects", strings.ToLower(req.ProjectName))
	}
	cleanDir := filepath.Clean(targetDir)

	if info, err := os.Stat(cleanDir); err == nil && info.IsDir() {
		// Check if empty
		entries, _ := os.ReadDir(cleanDir)
		if len(entries) > 0 {
			return nil, ErrProjectDirExists
		}
	}

	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	// 1. Scaffolding
	lang := strings.ToLower(req.Language)
	if lang == "" {
		lang = "rust"
	}
	if err := s.generateScaffolding(cleanDir, req.ProjectName, lang, req.Description); err != nil {
		return nil, fmt.Errorf("failed to generate scaffolding: %w", err)
	}

	// 2. Git initialization
	gitInitSuccess := false
	if req.InitGit {
		cmd := exec.CommandContext(ctx, "git", "init")
		cmd.Dir = cleanDir
		if err := cmd.Run(); err == nil {
			gitInitSuccess = true
		}
	}

	// 3. Register in Dendrite Workspace
	if s.workspaceRegistry != nil {
		_, _ = s.workspaceRegistry.AddProject(req.ProjectName, cleanDir)
	}

	// 4. Create in Plane v2 if requested
	planeID := ""
	planeErr := ""
	if req.CreatePlaneProject && s.planeClient != nil {
		ident := req.Identifier
		if ident == "" {
			ident = strings.ToUpper(strings.ReplaceAll(req.ProjectName, "-", ""))
			if len(ident) > 6 {
				ident = ident[:6]
			}
		}
		desc := req.Description
		if desc == "" {
			desc = fmt.Sprintf("Autonomous project %s managed via Dendrite", req.ProjectName)
		}

		proj, err := s.planeClient.CreateProject(ctx, req.ProjectName, ident, desc)
		if err != nil {
			planeErr = fmt.Sprintf("identifier %s: %v", ident, err)
		} else if proj != nil {
			planeID = proj.ID
		}
	}

	return &SpawnResult{
		ProjectName:    req.ProjectName,
		ProjectPath:    cleanDir,
		PlaneProjectID: planeID,
		PlaneError:     planeErr,
		GitInitialized: gitInitSuccess,
		Status:         "created_and_initialized",
	}, nil
}

// SpawnParallel spawns multiple projects in parallel.
func (s *ProjectSpawner) SpawnParallel(ctx context.Context, requests []SpawnRequest) ([]*SpawnResult, error) {
	results := make([]*SpawnResult, len(requests))
	errorsList := make([]error, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, r SpawnRequest) {
			defer wg.Done()
			res, err := s.Spawn(ctx, r)
			results[idx] = res
			errorsList[idx] = err
		}(i, req)
	}

	wg.Wait()

	for _, err := range errorsList {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

func (s *ProjectSpawner) generateScaffolding(dir, name, lang, desc string) error {
	readmeContent := fmt.Sprintf("# %s\n\n%s\n\nGenerated autonomously by Dendrite Distributed Codebase Engine.\n", name, desc)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte(readmeContent), 0644)
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/target\n/bin\n*.log\n.DS_Store\n"), 0644)

	switch lang {
	case "rust":
		cargoContent := fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
`, strings.ToLower(name))
		_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoContent), 0644)
		srcDir := filepath.Join(dir, "src")
		_ = os.MkdirAll(srcDir, 0755)
		libContent := fmt.Sprintf("//! %s library\n\npub fn init() -> bool {\n    true\n}\n", name)
		_ = os.WriteFile(filepath.Join(srcDir, "lib.rs"), []byte(libContent), 0644)

	case "go":
		goModContent := fmt.Sprintf("module %s\n\ngo 1.22\n", strings.ToLower(name))
		_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644)
		mainContent := fmt.Sprintf("package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"%s initialized\")\n}\n", name)
		_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644)

	case "typescript", "javascript":
		pkgContent := fmt.Sprintf(`{
  "name": "%s",
  "version": "1.0.0",
  "main": "dist/index.js",
  "scripts": {
    "build": "tsc"
  }
}
`, strings.ToLower(name))
		_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgContent), 0644)

	case "python":
		pyContent := fmt.Sprintf(`[project]
name = "%s"
version = "0.1.0"
dependencies = []
`, strings.ToLower(name))
		_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyContent), 0644)
	}

	return nil
}
