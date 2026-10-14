package docgen

import (
	"errors"
)

// Standard error definitions for docgen operations.
var (
	ErrEmptySpecID  = errors.New("specification ID cannot be empty")
	ErrEmptyTitle   = errors.New("title cannot be empty")
	ErrEmptyVersion = errors.New("version cannot be empty")
)

// FeatureSpec represents a Docusaurus Docs-as-Code feature specification with mandatory frontmatter.
type FeatureSpec struct {
	ID                  string   `yaml:"id" json:"id"`
	Title               string   `yaml:"title" json:"title"`
	Status              string   `yaml:"status" json:"status"`
	PlaneEpic           string   `yaml:"plane_epic,omitempty" json:"plane_epic,omitempty"`
	Dependencies        []string `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Repository          string   `yaml:"repository" json:"repository"`
	Tags                []string `yaml:"tags" json:"tags"`
	UserStory           string   `yaml:"-" json:"user_story"`
	TechnicalBoundaries []string `yaml:"-" json:"technical_boundaries"`
	AcceptanceCriteria  []string `yaml:"-" json:"acceptance_criteria"`
}

// ADRRecord represents an Architecture Decision Record with mandatory YAML frontmatter.
type ADRRecord struct {
	ID           string   `yaml:"id" json:"id"`
	Title        string   `yaml:"title" json:"title"`
	Status       string   `yaml:"status" json:"status"`
	Date         string   `yaml:"date" json:"date"`
	Repository   string   `yaml:"repository" json:"repository"`
	Tags         []string `yaml:"tags" json:"tags"`
	Context      string   `yaml:"-" json:"context"`
	Decision     string   `yaml:"-" json:"decision"`
	Consequences string   `yaml:"-" json:"consequences"`
}

// OpenAPIEndpoint describes a REST API endpoint for OpenAPI 3.1 generation.
type OpenAPIEndpoint struct {
	Path        string            `json:"path"`
	Method      string            `json:"method"`
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Responses   map[string]string `json:"responses"`
}

// OpenAPISpec encapsulates an OpenAPI 3.1.0 document model.
type OpenAPISpec struct {
	OpenAPI string              `json:"openapi"`
	Info    OpenAPIInfo         `json:"info"`
	Paths   map[string]PathItem `json:"paths"`
}

// OpenAPIInfo encapsulates metadata for the API.
type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// PathItem maps HTTP methods to operations in OpenAPI.
type PathItem map[string]Operation

// Operation defines a single API endpoint operation.
type Operation struct {
	Summary     string                       `json:"summary"`
	Description string                       `json:"description,omitempty"`
	Responses   map[string]OperationResponse `json:"responses"`
}

// OperationResponse represents an HTTP status response description.
type OperationResponse struct {
	Description string `json:"description"`
}
