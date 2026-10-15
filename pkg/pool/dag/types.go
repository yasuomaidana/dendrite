package dag

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Ecosystem identifies the technology stack or language of a project.
type Ecosystem string

const (
	EcosystemGo         Ecosystem = "go"
	EcosystemRust       Ecosystem = "rust"
	EcosystemTypeScript Ecosystem = "typescript"
	EcosystemPython     Ecosystem = "python"
	EcosystemUnknown    Ecosystem = "unknown"
)

// Standard typed errors.
var (
	ErrCyclicDependency = errors.New("cyclic dependency hazard detected")
	ErrManifestMalformed = errors.New("manifest file malformed")
	ErrProjectNotFound  = errors.New("project not found in pool")
	ErrDuplicateProject = errors.New("project already registered in pool")
	ErrEmptyPool        = errors.New("project pool is empty")
)

// CyclicDependencyError represents a detected circular dependency with path trace.
type CyclicDependencyError struct {
	Cycle []string
}

func (e *CyclicDependencyError) Error() string {
	return fmt.Sprintf("%v: %s", ErrCyclicDependency, strings.Join(e.Cycle, " -> "))
}

func (e *CyclicDependencyError) Unwrap() error {
	return ErrCyclicDependency
}

// ProjectInfo describes a registered project member inside the pool.
type ProjectInfo struct {
	Name         string      `json:"name"`
	Path         string      `json:"path"`
	Ecosystems   []Ecosystem `json:"ecosystems"`
	Manifests    []string    `json:"manifests"`
	Dependencies []string    `json:"dependencies"`
}

// PoolConfig represents the persistent state serialized to .dendrite/pool.json.
type PoolConfig struct {
	Version   string        `json:"version"`
	Projects  []ProjectInfo `json:"projects"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// DependencyEdge represents a directed dependency link between two projects.
type DependencyEdge struct {
	From      string    `json:"from"`      // Dependent project (requires Target)
	To        string    `json:"to"`        // Dependency project (required by Source)
	Ecosystem Ecosystem `json:"ecosystem"` // Ecosystem context
}

// DAGNode encapsulates a project node in the dependency graph.
type DAGNode struct {
	ID          string      `json:"id"`
	Path        string      `json:"path"`
	Ecosystems  []Ecosystem `json:"ecosystems"`
	DependsOn   []string    `json:"depends_on"`   // Projects this node requires
	DependedBy  []string    `json:"depended_by"`  // Projects that require this node
}

// DAGResolution encapsulates the computed topological graph and analysis.
type DAGResolution struct {
	Nodes            map[string]*DAGNode `json:"nodes"`
	TopologicalOrder []string            `json:"topological_order"` // Dependencies first
	Edges            []DependencyEdge    `json:"edges"`
	LeafNodes        []string            `json:"leaf_nodes"`        // Foundational (DependsOn is empty)
	RootNodes        []string            `json:"root_nodes"`        // Top-level (DependedBy is empty)
}
