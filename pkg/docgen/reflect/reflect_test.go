package reflect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractBlueprint_Validation(t *testing.T) {
	_, err := ExtractBlueprint("")
	if err != ErrEmptyReferenceDir {
		t.Fatalf("expected ErrEmptyReferenceDir, got: %v", err)
	}

	_, err = ExtractBlueprint("/path/that/does/not/exist/ever")
	if err != ErrRefDirNotFound {
		t.Fatalf("expected ErrRefDirNotFound, got: %v", err)
	}
}

func TestExtractBlueprint_Success(t *testing.T) {
	tempDir := t.TempDir()

	featDir := filepath.Join(tempDir, "features")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}

	sampleDoc := `---
id: FEAT-999
title: Sample Feature
status: approved
tags: [test, doc]
---

# FEAT-999: Sample Feature

## 1. User Story
As a developer I want docs.

## 2. Technical Boundaries & Specifications
1. Must work fast.

` + "```mermaid\nflowchart TD\n    A[RefNode] --> B[TargetNode]\n```\n"

	if err := os.WriteFile(filepath.Join(featDir, "FEAT-999.md"), []byte(sampleDoc), 0644); err != nil {
		t.Fatal(err)
	}

	sidebarData := `{"docsSidebar": ["features/FEAT-999.md"]}`
	if err := os.WriteFile(filepath.Join(tempDir, "sidebars.json"), []byte(sidebarData), 0644); err != nil {
		t.Fatal(err)
	}

	bp, err := ExtractBlueprint(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bp.Categories) == 0 || bp.Categories[0] != "features" {
		t.Errorf("expected category 'features', got: %v", bp.Categories)
	}

	tmpl, ok := bp.Templates[filepath.Join("features", "FEAT-999.md")]
	if !ok {
		t.Fatalf("template for features/FEAT-999.md not found in blueprint")
	}

	if tmpl.Frontmatter["id"] != "FEAT-999" {
		t.Errorf("expected frontmatter id FEAT-999, got: %s", tmpl.Frontmatter["id"])
	}

	if len(tmpl.Headings) < 3 {
		t.Errorf("expected at least 3 headings, got: %d", len(tmpl.Headings))
	}

	if tmpl.MermaidStyle == nil || tmpl.MermaidStyle.DiagramType != "flowchart TD" {
		t.Errorf("expected flowchart TD diagram style, got: %v", tmpl.MermaidStyle)
	}

	if bp.SidebarJSON == "" {
		t.Errorf("expected sidebar JSON to be captured")
	}
}

func TestHarvestTargetSymbols(t *testing.T) {
	_, err := HarvestTargetSymbols("")
	if err != ErrEmptyTargetDir {
		t.Fatalf("expected ErrEmptyTargetDir, got: %v", err)
	}

	tempDir := t.TempDir()
	goCode := `package sample

// EngineInterface manages execution.
type EngineInterface interface {
	Start() error
}

// ControllerConfig provides runtime configuration.
type ControllerConfig struct {
	Port int
}

// RunEngine runs the engine.
func RunEngine(cfg ControllerConfig) error {
	return nil
}

func privateHelper() {}
`
	if err := os.WriteFile(filepath.Join(tempDir, "sample.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a test file that should be ignored
	if err := os.WriteFile(filepath.Join(tempDir, "sample_test.go"), []byte("package sample\nfunc TestX() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	symbols, err := HarvestTargetSymbols(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(symbols) != 3 {
		t.Fatalf("expected 3 exported symbols, got %d", len(symbols))
	}

	symbolNames := make(map[string]string)
	for _, s := range symbols {
		symbolNames[s.Name] = s.Kind
	}

	if symbolNames["EngineInterface"] != "interface" {
		t.Errorf("expected EngineInterface as interface, got %s", symbolNames["EngineInterface"])
	}
	if symbolNames["ControllerConfig"] != "struct" {
		t.Errorf("expected ControllerConfig as struct, got %s", symbolNames["ControllerConfig"])
	}
	if symbolNames["RunEngine"] != "func" {
		t.Errorf("expected RunEngine as func, got %s", symbolNames["RunEngine"])
	}
}

func TestPlanReflection(t *testing.T) {
	_, err := PlanReflection(nil, "src", "docs")
	if err != ErrInvalidBlueprint {
		t.Fatalf("expected ErrInvalidBlueprint, got: %v", err)
	}

	bp := &DocBlueprint{
		Templates: map[string]DocFileTemplate{
			"features/FEAT-001.md": {
				RelativePath: "features/FEAT-001.md",
				Frontmatter: map[string]string{
					"id":    "FEAT-001",
					"title": "Auth",
				},
				Headings: []HeadingTemplate{
					{Level: 1, Title: "FEAT-001: Auth"},
					{Level: 2, Title: "Technical Boundaries & Specifications"},
				},
				MermaidStyle: &DiagramStyle{
					DiagramType: "flowchart TD",
				},
			},
		},
	}

	_, err = PlanReflection(bp, "src", "")
	if err != ErrEmptyTargetDir {
		t.Fatalf("expected ErrEmptyTargetDir, got: %v", err)
	}

	srcDir := t.TempDir()
	goCode := `package auth
type SessionToken struct {
	ID string
}
func ValidateToken(t SessionToken) bool { return true }
`
	if err := os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanReflection(bp, srcDir, "/target/docs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Files) != 1 {
		t.Fatalf("expected 1 planned file, got %d", len(plan.Files))
	}

	file := plan.Files[0]
	if !strings.Contains(file.Content, "auth.SessionToken") {
		t.Errorf("expected planned doc to contain verified symbol auth.SessionToken, content: %s", file.Content)
	}
	if !strings.Contains(file.Content, "```mermaid") {
		t.Errorf("expected planned doc to contain mermaid diagram")
	}
}

func TestApplyReflection_And_ReflectTaxonomy(t *testing.T) {
	_, err := ApplyReflection(nil)
	if err != ErrInvalidBlueprint {
		t.Fatalf("expected ErrInvalidBlueprint, got: %v", err)
	}

	refDir := t.TempDir()
	archDir := filepath.Join(refDir, "architecture")
	if err := os.MkdirAll(archDir, 0755); err != nil {
		t.Fatal(err)
	}
	refDoc := `---
id: ARCH-001
title: Microservice Architecture
---

# ARCH-001: Microservice Architecture

## Technical Specifications
`
	if err := os.WriteFile(filepath.Join(archDir, "ARCH-001.md"), []byte(refDoc), 0644); err != nil {
		t.Fatal(err)
	}

	targetSrcDir := t.TempDir()
	goCode := `package service
type Dispatcher struct {}
`
	if err := os.WriteFile(filepath.Join(targetSrcDir, "service.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	targetDocsDir := filepath.Join(t.TempDir(), "project_docs")

	result, err := ReflectTaxonomy(refDir, targetSrcDir, targetDocsDir)
	if err != nil {
		t.Fatalf("ReflectTaxonomy failed: %v", err)
	}

	if len(result.FilesWritten) != 1 {
		t.Fatalf("expected 1 file written, got %d", len(result.FilesWritten))
	}

	if !result.SidebarSynced {
		t.Errorf("expected sidebar to be synced")
	}

	content, err := os.ReadFile(result.FilesWritten[0])
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "service.Dispatcher") {
		t.Errorf("expected content to include target symbol, got: %s", string(content))
	}

	// Verify error on invalid ref
	_, err = ReflectTaxonomy("/non/existent/ref", targetSrcDir, targetDocsDir)
	if err == nil {
		t.Errorf("expected error on non-existent refDir")
	}
}
