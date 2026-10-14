package docgen_test

import (
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/docgen"
)

func TestRenderFeatureSpec(t *testing.T) {
	// 1. Error on empty ID
	if _, err := docgen.RenderFeatureSpec(docgen.FeatureSpec{ID: ""}); !errors.Is(err, docgen.ErrEmptySpecID) {
		t.Errorf("expected ErrEmptySpecID for empty ID")
	}

	// 2. Error on empty title
	if _, err := docgen.RenderFeatureSpec(docgen.FeatureSpec{ID: "FEAT-100", Title: ""}); !errors.Is(err, docgen.ErrEmptyTitle) {
		t.Errorf("expected ErrEmptyTitle for empty title")
	}

	// 3. Valid specification
	spec := docgen.FeatureSpec{
		ID:           "FEAT-042",
		Title:        "Asynchronous Event Bus",
		Status:       "approved",
		PlaneEpic:    "PROJ-E-12",
		Dependencies: []string{"FEAT-001", "FEAT-002"},
		Repository:   "dendrite",
		Tags:         []string{"core", "events", "async"},
		UserStory:    "As an architect, I need an event bus for micro-telemetry.",
		TechnicalBoundaries: []string{
			"Thread-safe channel queues",
			"JSON serialization format",
		},
		AcceptanceCriteria: []string{
			"Zero event drops under 10k/sec load",
			"Sub-millisecond dispatch latency",
		},
	}

	out, err := docgen.RenderFeatureSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error rendering feature spec: %v", err)
	}

	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("expected output to start with YAML frontmatter")
	}
	if !strings.Contains(out, "id: FEAT-042") || !strings.Contains(out, "title: \"Asynchronous Event Bus\"") {
		t.Errorf("expected frontmatter to contain ID and Title")
	}
	if !strings.Contains(out, "## 1. User Story & Business Objective") {
		t.Errorf("expected User Story section")
	}
	if !strings.Contains(out, "## 3. Acceptance Criteria & Verification Matrix") {
		t.Errorf("expected Acceptance Criteria section")
	}
}

func TestRenderADRRecord(t *testing.T) {
	// 1. Error on empty ID
	if _, err := docgen.RenderADRRecord(docgen.ADRRecord{ID: ""}); !errors.Is(err, docgen.ErrEmptySpecID) {
		t.Errorf("expected ErrEmptySpecID for empty ID")
	}

	// 2. Error on empty title
	if _, err := docgen.RenderADRRecord(docgen.ADRRecord{ID: "ADR-001", Title: ""}); !errors.Is(err, docgen.ErrEmptyTitle) {
		t.Errorf("expected ErrEmptyTitle for empty title")
	}

	// 3. Valid ADR
	adr := docgen.ADRRecord{
		ID:           "ADR-005",
		Title:        "Adopt BadgerDB for Local Cache",
		Status:       "accepted",
		Date:         "2026-10-14",
		Repository:   "dendrite",
		Tags:         []string{"storage", "cache"},
		Context:      "SQLite locked on high concurrency writes.",
		Decision:     "Adopt LSM-based BadgerDB for local key-value cache.",
		Consequences: "10x higher concurrent write throughput; requires directory lock handling.",
	}

	out, err := docgen.RenderADRRecord(adr)
	if err != nil {
		t.Fatalf("unexpected error rendering ADR: %v", err)
	}

	if !strings.HasPrefix(out, "---\n") || !strings.Contains(out, "id: ADR-005") {
		t.Errorf("expected valid YAML frontmatter with ID")
	}
	if !strings.Contains(out, "## 1. Context & Problem Statement") ||
		!strings.Contains(out, "## 2. Decision") ||
		!strings.Contains(out, "## 3. Consequences") {
		t.Errorf("expected standard ADR sections")
	}
}

func TestGenerateOpenAPI31(t *testing.T) {
	// 1. Errors on empty title or version
	if _, err := docgen.GenerateOpenAPI31("", "1.0", nil); !errors.Is(err, docgen.ErrEmptyTitle) {
		t.Errorf("expected ErrEmptyTitle")
	}
	if _, err := docgen.GenerateOpenAPI31("API", "", nil); !errors.Is(err, docgen.ErrEmptyVersion) {
		t.Errorf("expected ErrEmptyVersion")
	}

	// 2. Valid OpenAPI generation
	endpoints := []docgen.OpenAPIEndpoint{
		{
			Path:    "/health",
			Method:  "GET",
			Summary: "Service health probe",
			Responses: map[string]string{
				"200": "Healthy",
			},
		},
		{
			Path:    "/tickets",
			Method:  "POST",
			Summary: "Create ticket",
			Responses: map[string]string{
				"201": "Created",
			},
		},
	}

	doc, err := docgen.GenerateOpenAPI31("Dendrite Control Plane", "1.0.0", endpoints)
	if err != nil {
		t.Fatalf("unexpected error generating OpenAPI: %v", err)
	}

	if !strings.Contains(doc, "\"openapi\": \"3.1.0\"") {
		t.Errorf("expected OpenAPI 3.1.0 specification")
	}
	if !strings.Contains(doc, "/health") || !strings.Contains(doc, "/tickets") {
		t.Errorf("expected endpoints in OpenAPI paths")
	}
}

func TestExtractEndpointsFromSource(t *testing.T) {
	sourceCode := `
package server

// @Route GET /api/v1/health [Get health status]
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {}

// @Route POST /api/v1/agent/run [Trigger agent execution]
func (s *Server) agentRunHandler(w http.ResponseWriter, r *http.Request) {}
`

	endpoints := docgen.ExtractEndpointsFromSource(sourceCode)
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	if endpoints[0].Path != "/api/v1/health" || endpoints[0].Method != "GET" {
		t.Errorf("unexpected first endpoint: %+v", endpoints[0])
	}
	if endpoints[1].Path != "/api/v1/agent/run" || endpoints[1].Method != "POST" {
		t.Errorf("unexpected second endpoint: %+v", endpoints[1])
	}
}
