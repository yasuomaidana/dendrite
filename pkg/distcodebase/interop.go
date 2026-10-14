package distcodebase

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// InteropRequest defines a cross-project feature dependency coordination.
type InteropRequest struct {
	RequesterProject  string `json:"requester_project"`  // e.g. "Docula"
	DependencyProject string `json:"dependency_project"` // e.g. "Oxipix"
	FeatureRequested  string `json:"feature_requested"`  // e.g. "blend_modes_simd"
	AllowFallback     bool   `json:"allow_fallback"`     // Whether requester can continue with stub/interim impl
	InterimFallback   string `json:"interim_fallback,omitempty"`
}

// ReconciliationResult details changes applied to the requester once the dependency is finished.
type ReconciliationResult struct {
	CoordinationID string `json:"coordination_id"`
	Requester      string `json:"requester"`
	Dependency     string `json:"dependency"`
	Status         string `json:"status"`
	PatchGenerated string `json:"patch_generated"`
	TODOsResolved  int    `json:"todos_resolved"`
}

// InteropCoordinator orchestrates dependencies and non-blocking progress across distributed projects.
type InteropCoordinator struct {
	mu            sync.RWMutex
	registry      *Registry
	coordinations map[string]*InteropCoordination
}

// NewInteropCoordinator creates a new coordinator instance.
func NewInteropCoordinator(registry *Registry) *InteropCoordinator {
	if registry == nil {
		registry = NewDefaultRegistry()
	}
	return &InteropCoordinator{
		registry:      registry,
		coordinations: make(map[string]*InteropCoordination),
	}
}

// CoordinateFeature handles a feature gap in an owned dependency, generating unblocking fallbacks.
func (c *InteropCoordinator) CoordinateFeature(req InteropRequest) (*InteropCoordination, error) {
	if strings.TrimSpace(req.RequesterProject) == "" || strings.TrimSpace(req.DependencyProject) == "" {
		return nil, ErrEmptyProjectName
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	id := fmt.Sprintf("interop-%s-%s-%d",
		strings.ToLower(req.RequesterProject),
		strings.ToLower(req.DependencyProject),
		time.Now().UnixNano()%1000000,
	)

	isOwned := c.registry.IsOwnedLibrary(req.DependencyProject)
	todoMarker := fmt.Sprintf("// TODO(interop:%s): Replace interim fallback with %s::%s once implemented",
		req.DependencyProject, req.DependencyProject, req.FeatureRequested)

	fallbackCode := req.InterimFallback
	if fallbackCode == "" {
		fallbackCode = fmt.Sprintf("%s\npub fn %s_interim() -> Result<(), &'static str> {\n    // Interim software fallback while %s implements hardware-accelerated feature\n    Ok(())\n}",
			todoMarker, req.FeatureRequested, req.DependencyProject)
	}

	state := InteropPending
	if req.AllowFallback {
		state = InteropUnblockedWithFallback
	}

	coord := &InteropCoordination{
		ID:                  id,
		RequesterProject:    req.RequesterProject,
		DependencyProject:   req.DependencyProject,
		FeatureRequested:    req.FeatureRequested,
		DependencyIsOwned:   isOwned,
		State:               state,
		InterimFallbackCode: fallbackCode,
		FallbackTODO:        todoMarker,
		UnblocksWorkflow:    req.AllowFallback,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	c.coordinations[id] = coord
	return coord, nil
}

// CompleteDependencyFeature signals that the feature has been implemented and tested in the dependency project.
func (c *InteropCoordinator) CompleteDependencyFeature(coordinationID string) (*InteropCoordination, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	coord, exists := c.coordinations[coordinationID]
	if !exists {
		return nil, ErrCoordinationNotFound
	}

	coord.State = InteropDependencyCompleted
	coord.UpdatedAt = time.Now()
	return coord, nil
}

// ReconcileProject removes interim fallback code and applies the finalized dependency integration.
func (c *InteropCoordinator) ReconcileProject(coordinationID string) (*ReconciliationResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	coord, exists := c.coordinations[coordinationID]
	if !exists {
		return nil, ErrCoordinationNotFound
	}

	patch := fmt.Sprintf("--- a/src/lib.rs\n+++ b/src/lib.rs\n@@ -1,5 +1,4 @@\n-%s\n-// Removed interim fallback\n+use %s::%s;\n",
		coord.FallbackTODO, strings.ToLower(coord.DependencyProject), coord.FeatureRequested)

	coord.State = InteropIntegrated
	coord.UpdatedAt = time.Now()

	return &ReconciliationResult{
		CoordinationID: coordinationID,
		Requester:      coord.RequesterProject,
		Dependency:     coord.DependencyProject,
		Status:         "successfully_reconciled",
		PatchGenerated: patch,
		TODOsResolved:  1,
	}, nil
}

// GetCoordination retrieves a coordination record by ID.
func (c *InteropCoordinator) GetCoordination(id string) (*InteropCoordination, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	coord, ok := c.coordinations[id]
	if !ok {
		return nil, ErrCoordinationNotFound
	}
	return coord, nil
}

// ListCoordinations returns all active and historical project coordinations.
func (c *InteropCoordinator) ListCoordinations() []*InteropCoordination {
	c.mu.RLock()
	defer c.mu.RUnlock()

	list := make([]*InteropCoordination, 0, len(c.coordinations))
	for _, coord := range c.coordinations {
		list = append(list, coord)
	}
	return list
}

// FormatLocalLLMPrompt synthesizes an efficient coordination prompt for local Ollama models.
func (c *InteropCoordinator) FormatLocalLLMPrompt(coord *InteropCoordination) string {
	if coord == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Distributed Codebase Interoperability Decision Matrix]\n\n")
	sb.WriteString(fmt.Sprintf("Requester Project: %s\n", coord.RequesterProject))
	sb.WriteString(fmt.Sprintf("Dependency Project: %s (User Owned: %t, Priority 1)\n", coord.DependencyProject, coord.DependencyIsOwned))
	sb.WriteString(fmt.Sprintf("Missing Capability: %s\n", coord.FeatureRequested))
	sb.WriteString(fmt.Sprintf("Current State: %s\n", coord.State))
	sb.WriteString(fmt.Sprintf("Interim Unblocking Strategy: %s\n\n", coord.FallbackTODO))
	sb.WriteString("Instructions for Local LLM Coordinator:\n")
	sb.WriteString("1. Confirm whether the requester can proceed with software fallback or interim adapter.\n")
	sb.WriteString("2. Schedule atomic feature development ticket in the dependency repository.\n")
	sb.WriteString("3. Upon dependency unit test pass, trigger reconciliation to swap interim fallback with native call.\n")

	return sb.String()
}
