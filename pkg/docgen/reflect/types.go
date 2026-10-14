package reflect

import (
	"errors"
)

// Common errors in documentation reflection.
var (
	ErrEmptyReferenceDir = errors.New("reference docs directory cannot be empty")
	ErrEmptyTargetDir    = errors.New("target directory cannot be empty")
	ErrInvalidBlueprint  = errors.New("invalid or empty documentation blueprint")
	ErrRefDirNotFound    = errors.New("reference docs directory does not exist")
)

// HeadingTemplate captures heading structure and indentation level.
type HeadingTemplate struct {
	Level int    `json:"level"`
	Title string `json:"title"`
}

// DiagramStyle captures detected Mermaid diagram patterns.
type DiagramStyle struct {
	DiagramType string   `json:"diagram_type"` // e.g. "flowchart TD", "sequenceDiagram"
	SampleNodes []string `json:"sample_nodes,omitempty"`
}

// DocFileTemplate represents a documentation file structure harvested from a reference project.
type DocFileTemplate struct {
	RelativePath string            `json:"relative_path"`
	Category     string            `json:"category"`
	Frontmatter  map[string]string `json:"frontmatter"`
	Headings     []HeadingTemplate `json:"headings"`
	MermaidStyle *DiagramStyle     `json:"mermaid_style,omitempty"`
	RawContent   string            `json:"raw_content,omitempty"`
}

// DocBlueprint captures the full taxonomy, frontmatter schema, and styles of a reference documentation tree.
type DocBlueprint struct {
	ReferenceDir string                     `json:"reference_dir"`
	Categories   []string                   `json:"categories"`
	Templates    map[string]DocFileTemplate `json:"templates"`
	SidebarJSON  string                     `json:"sidebar_json,omitempty"`
}

// TargetSymbol represents an extracted code entity from the target codebase.
type TargetSymbol struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // "func", "type", "interface", "struct"
	Doc     string `json:"doc,omitempty"`
}

// PlannedDocFile represents a concrete document file planned for generation.
type PlannedDocFile struct {
	RelativePath string            `json:"relative_path"`
	Frontmatter  map[string]string `json:"frontmatter"`
	Content      string            `json:"content"`
}

// ReflectionPlan defines the scheduled files and modifications to apply to the target docs suite.
type ReflectionPlan struct {
	TargetDocsDir string           `json:"target_docs_dir"`
	Files         []PlannedDocFile `json:"files"`
	SidebarConfig string           `json:"sidebar_config,omitempty"`
	Warnings      []string         `json:"warnings,omitempty"`
}

// ReflectionResult details the outcomes of an executed reflection.
type ReflectionResult struct {
	FilesWritten  []string `json:"files_written"`
	SidebarSynced bool     `json:"sidebar_synced"`
	DurationMs    int64    `json:"duration_ms"`
}
