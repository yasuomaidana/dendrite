package dag

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestScanner_MultiEcosystem(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Go Project
	goDir := filepath.Join(tempDir, "project-go")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	goModContent := `module github.com/test/project-go

go 1.24

require (
	github.com/test/project-rust v1.0.0
	github.com/gin-gonic/gin v1.9.1
)
`
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("write go.mod failed: %v", err)
	}

	// 2. Rust Project
	rustDir := filepath.Join(tempDir, "project-rust")
	if err := os.MkdirAll(rustDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	cargoContent := `[package]
name = "project-rust"
version = "0.1.0"
edition = "2024"

[dependencies]
serde = "1.0"
project-ts = { path = "../project-ts" }

[dependencies.tokio]
version = "1.0"
`
	if err := os.WriteFile(filepath.Join(rustDir, "Cargo.toml"), []byte(cargoContent), 0644); err != nil {
		t.Fatalf("write Cargo.toml failed: %v", err)
	}

	// 3. TS Project
	tsDir := filepath.Join(tempDir, "project-ts")
	if err := os.MkdirAll(tsDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	pkgContent := `{
  "name": "project-ts",
  "version": "1.0.0",
  "dependencies": {
    "project-py": "^0.1.0",
    "svelte": "^5.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(tsDir, "package.json"), []byte(pkgContent), 0644); err != nil {
		t.Fatalf("write package.json failed: %v", err)
	}

	// 4. Python Project
	pyDir := filepath.Join(tempDir, "project-py")
	if err := os.MkdirAll(pyDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	pyprojectContent := `[project]
name = "project-py"
version = "0.1.0"
dependencies = [
    "pydantic>=2.0",
]

[tool.poetry.dependencies]
python = "^3.12"
uvicorn = "^0.23.0"
`
	if err := os.WriteFile(filepath.Join(pyDir, "pyproject.toml"), []byte(pyprojectContent), 0644); err != nil {
		t.Fatalf("write pyproject.toml failed: %v", err)
	}

	scanner := NewManifestScanner()

	// Test Go
	infoGo, err := scanner.ScanProject(goDir)
	if err != nil {
		t.Fatalf("Scan Go failed: %v", err)
	}
	if infoGo.Name != "github.com/test/project-go" {
		t.Errorf("expected module name github.com/test/project-go, got %s", infoGo.Name)
	}
	if len(infoGo.Ecosystems) == 0 || infoGo.Ecosystems[0] != EcosystemGo {
		t.Errorf("expected Go ecosystem, got %v", infoGo.Ecosystems)
	}

	// Test Rust
	infoRust, err := scanner.ScanProject(rustDir)
	if err != nil {
		t.Fatalf("Scan Rust failed: %v", err)
	}
	if infoRust.Name != "project-rust" {
		t.Errorf("expected crate name project-rust, got %s", infoRust.Name)
	}
	if len(infoRust.Ecosystems) == 0 || infoRust.Ecosystems[0] != EcosystemRust {
		t.Errorf("expected Rust ecosystem, got %v", infoRust.Ecosystems)
	}

	// Test TS
	infoTS, err := scanner.ScanProject(tsDir)
	if err != nil {
		t.Fatalf("Scan TS failed: %v", err)
	}
	if infoTS.Name != "project-ts" {
		t.Errorf("expected package name project-ts, got %s", infoTS.Name)
	}

	// Test Python
	infoPy, err := scanner.ScanProject(pyDir)
	if err != nil {
		t.Fatalf("Scan Python failed: %v", err)
	}
	if infoPy.Name != "project-py" {
		t.Errorf("expected python name project-py, got %s", infoPy.Name)
	}
}

func TestDAGSolver_DiamondDependencyResolution(t *testing.T) {
	// A -> B -> D
	// A -> C -> D
	projects := []ProjectInfo{
		{
			Name:         "node-d",
			Ecosystems:   []Ecosystem{EcosystemRust},
			Dependencies: []string{},
		},
		{
			Name:         "node-b",
			Ecosystems:   []Ecosystem{EcosystemGo},
			Dependencies: []string{"node-d"},
		},
		{
			Name:         "node-c",
			Ecosystems:   []Ecosystem{EcosystemTypeScript},
			Dependencies: []string{"node-d"},
		},
		{
			Name:         "node-a",
			Ecosystems:   []Ecosystem{EcosystemPython},
			Dependencies: []string{"node-b", "node-c"},
		},
	}

	solver := NewDAGSolver()
	res, err := solver.Resolve(projects)
	if err != nil {
		t.Fatalf("expected resolution to succeed, got %v", err)
	}

	// D must come before B and C; B and C must come before A
	idxMap := make(map[string]int)
	for i, id := range res.TopologicalOrder {
		idxMap[id] = i
	}

	if idxMap["node-d"] >= idxMap["node-b"] {
		t.Errorf("node-d must precede node-b: order=%v", res.TopologicalOrder)
	}
	if idxMap["node-d"] >= idxMap["node-c"] {
		t.Errorf("node-d must precede node-c: order=%v", res.TopologicalOrder)
	}
	if idxMap["node-b"] >= idxMap["node-a"] {
		t.Errorf("node-b must precede node-a: order=%v", res.TopologicalOrder)
	}
	if idxMap["node-c"] >= idxMap["node-a"] {
		t.Errorf("node-c must precede node-a: order=%v", res.TopologicalOrder)
	}

	if len(res.LeafNodes) != 1 || res.LeafNodes[0] != "node-d" {
		t.Errorf("expected leaf node [node-d], got %v", res.LeafNodes)
	}
	if len(res.RootNodes) != 1 || res.RootNodes[0] != "node-a" {
		t.Errorf("expected root node [node-a], got %v", res.RootNodes)
	}
}

func TestDAGSolver_CyclicDependencyDetection(t *testing.T) {
	// X -> Y -> Z -> X
	projects := []ProjectInfo{
		{Name: "svc-x", Dependencies: []string{"svc-y"}},
		{Name: "svc-y", Dependencies: []string{"svc-z"}},
		{Name: "svc-z", Dependencies: []string{"svc-x"}},
	}

	solver := NewDAGSolver()
	_, err := solver.Resolve(projects)
	if err == nil {
		t.Fatal("expected cyclic dependency error, got nil")
	}

	if !errors.Is(err, ErrCyclicDependency) {
		t.Errorf("expected ErrCyclicDependency, got %v", err)
	}

	var cycErr *CyclicDependencyError
	if !errors.As(err, &cycErr) {
		t.Errorf("expected *CyclicDependencyError type, got %T", err)
	} else if len(cycErr.Cycle) == 0 {
		t.Errorf("expected non-empty cycle path, got empty")
	}
}

func TestDAGSolver_SelfDependency(t *testing.T) {
	projects := []ProjectInfo{
		{Name: "svc-self", Dependencies: []string{"svc-self"}},
	}

	solver := NewDAGSolver()
	_, err := solver.Resolve(projects)
	if err == nil {
		t.Fatal("expected error on self dependency, got nil")
	}
	if !errors.Is(err, ErrCyclicDependency) {
		t.Errorf("expected ErrCyclicDependency on self dependency, got %v", err)
	}
}

func TestDAGSolver_EmptyPool(t *testing.T) {
	solver := NewDAGSolver()
	_, err := solver.Resolve([]ProjectInfo{})
	if !errors.Is(err, ErrEmptyPool) {
		t.Errorf("expected ErrEmptyPool, got %v", err)
	}
}

func TestPoolManager_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, ".dendrite", "pool.json")

	// Create 2 test sub-projects
	p1 := filepath.Join(tempDir, "proj1")
	p2 := filepath.Join(tempDir, "proj2")
	_ = os.MkdirAll(p1, 0755)
	_ = os.MkdirAll(p2, 0755)
	_ = os.WriteFile(filepath.Join(p1, "go.mod"), []byte("module proj1\n"), 0644)
	_ = os.WriteFile(filepath.Join(p2, "Cargo.toml"), []byte("[package]\nname = \"proj2\"\nversion = \"0.1.0\"\n"), 0644)

	pm := NewPoolManager(cfgFile)

	// Add projects
	added, err := pm.Add(p1, p2)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("expected 2 added projects, got %d", len(added))
	}

	// Duplicate add rejection
	_, err = pm.Add(p1)
	if !errors.Is(err, ErrDuplicateProject) {
		t.Errorf("expected ErrDuplicateProject on duplicate path, got %v", err)
	}

	// Save
	if err := pm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload from new manager
	pm2 := NewPoolManager(cfgFile)
	list := pm2.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 projects after reload, got %d", len(list))
	}

	// Remove project
	if err := pm2.Remove("proj1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(pm2.List()) != 1 {
		t.Errorf("expected 1 project after remove, got %d", len(pm2.List()))
	}

	// Remove non-existent
	if err := pm2.Remove("proj_non_existent"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}
