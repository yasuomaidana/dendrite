package distcodebase

import (
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/rea"
)

// DecomposeRequest configures modular decomposition for a target system.
type DecomposeRequest struct {
	ProjectName         string            `json:"project_name"`
	TargetBinaryOrSpec  string            `json:"target_binary_or_spec"`
	Language            string            `json:"language"`
	ForcedModular       *bool             `json:"forced_modular"` // Defaults to true
	RequestedModules    []string          `json:"requested_modules,omitempty"`
	UserCustomOverrides []string          `json:"user_custom_overrides,omitempty"` // Names of modules user explicitly wants custom
	TechnicalBenefits   map[string]string `json:"technical_benefits,omitempty"`    // Module -> reason (e.g. SIMD acceleration)
}

// Decomposer splits monolithic targets into reusable modules and queries ecosystem libraries.
type Decomposer struct {
	registry *Registry
}

// NewDecomposer constructs a new Decomposer using the provided distributed codebase registry.
func NewDecomposer(registry *Registry) *Decomposer {
	if registry == nil {
		registry = NewDefaultRegistry()
	}
	return &Decomposer{
		registry: registry,
	}
}

// DecomposeProject divides a project into modular components, applying priority-driven library matching.
func (d *Decomposer) DecomposeProject(req DecomposeRequest) (*DecompositionPlan, error) {
	if strings.TrimSpace(req.ProjectName) == "" {
		return nil, ErrEmptyProjectName
	}

	forced := true
	if req.ForcedModular != nil {
		forced = *req.ForcedModular
	}

	targetLang := req.Language
	if targetLang == "" {
		targetLang = "rust" // Default language
	}

	moduleNames := req.RequestedModules
	if len(moduleNames) == 0 {
		// By default divide into standard cohesive decoupled modules
		moduleNames = []string{
			"storage-engine",
			"hierarchical-query",
			"render-pipeline",
			"pixel-processor",
			"semantic-graph",
			"api-gateway",
		}
	}

	userOverrides := make(map[string]bool)
	for _, m := range req.UserCustomOverrides {
		userOverrides[strings.ToLower(strings.TrimSpace(m))] = true
	}

	modules := make([]ModuleDefinition, 0, len(moduleNames))
	reusedCount := 0
	customCount := 0
	satelliteCount := 0

	for i, modName := range moduleNames {
		modID := fmt.Sprintf("mod-%02d-%s", i+1, strings.ToLower(strings.ReplaceAll(modName, " ", "-")))
		caps := inferCapabilities(modName)

		mod := ModuleDefinition{
			ID:                  modID,
			Name:                modName,
			Language:            targetLang,
			Purpose:             fmt.Sprintf("Modular decoupled component for %s", modName),
			Capabilities:        caps,
			Dependencies:        []string{},
			EstimatedComplexity: 5,
		}

		userWantsCustom := userOverrides[strings.ToLower(modName)]
		mod.UserRequestedCustom = userWantsCustom

		// Resolve across distributed codebase libraries in priority order
		var matched *LibraryEntry
		for _, capTag := range caps {
			if lib, found := d.registry.ResolveCapability(capTag, targetLang); found {
				matched = lib
				break
			}
		}

		benefitReason := req.TechnicalBenefits[modName]

		if matched != nil && !userWantsCustom && benefitReason == "" {
			// Scenario 1: Reuse existing library from distributed codebase
			mod.Strategy = StrategyReuseLibrary
			mod.MatchedLibrary = matched
			reusedCount++
		} else if userWantsCustom {
			mod.Strategy = StrategyDevelopCustom
			mod.DevelopCustomReason = "Explicit user request to develop custom in-house implementation"
			customCount++
		} else if benefitReason != "" {
			mod.Strategy = StrategyDevelopCustom
			mod.DevelopCustomReason = fmt.Sprintf("Verified architectural benefit: %s", benefitReason)
			customCount++
		} else {
			// No suitable library found; check if it qualifies as satellite side-project
			if strings.Contains(strings.ToLower(modName), "subsystem") || strings.Contains(strings.ToLower(modName), "pipeline") {
				mod.Strategy = StrategySatelliteProject
				satelliteCount++
			} else {
				mod.Strategy = StrategyDevelopCustom
				mod.DevelopCustomReason = "No existing library matches required capabilities in distributed ecosystem"
				customCount++
			}
		}

		modules = append(modules, mod)
	}

	plan := &DecompositionPlan{
		TargetProject:          req.ProjectName,
		TargetBinaryOrSpec:     req.TargetBinaryOrSpec,
		ForcedModular:          forced,
		Modules:                modules,
		TotalModules:           len(modules),
		ReusedLibrariesCount:   reusedCount,
		CustomModulesCount:     customCount,
		SatelliteProjectsCount: satelliteCount,
		CreatedAt:              time.Now(),
	}

	return plan, nil
}

// DecomposeREAEvidence decomposes REA evidence bundles into modular segments and maps ecosystem libraries.
func (d *Decomposer) DecomposeREAEvidence(bundle *rea.EvidenceBundle, userRequests []string) (*DecompositionPlan, error) {
	if bundle == nil {
		return nil, rea.ErrEmptyBinaryPath
	}

	// Extract domain capabilities from pseudocode, strings, and cross-references
	detectedModules := make([]string, 0)

	combined := strings.ToLower(bundle.Pseudocode + " " + strings.Join(bundle.StringConstants, " ") + " " + strings.Join(bundle.CrossReferences, " "))

	if strings.Contains(combined, "wgpu") || strings.Contains(combined, "shader") || strings.Contains(combined, "render") || strings.Contains(combined, "vulkan") || strings.Contains(combined, "metal") {
		detectedModules = append(detectedModules, "render-pipeline")
	}
	if strings.Contains(combined, "sql") || strings.Contains(combined, "database") || strings.Contains(combined, "table") || strings.Contains(combined, "turso") || strings.Contains(combined, "sqlite") {
		detectedModules = append(detectedModules, "storage-engine")
	}
	if strings.Contains(combined, "cte") || strings.Contains(combined, "recursive") || strings.Contains(combined, "with recursive") || strings.Contains(combined, "hierarchy") {
		detectedModules = append(detectedModules, "hierarchical-query")
	}
	if strings.Contains(combined, "pixel") || strings.Contains(combined, "rgba") || strings.Contains(combined, "simd") || strings.Contains(combined, "image") {
		detectedModules = append(detectedModules, "pixel-processor")
	}
	if strings.Contains(combined, "dag") || strings.Contains(combined, "graph") || strings.Contains(combined, "ast") || strings.Contains(combined, "tree") {
		detectedModules = append(detectedModules, "semantic-graph")
	}

	if len(detectedModules) == 0 {
		detectedModules = append(detectedModules, "core-kernel", "io-adapter")
	}

	req := DecomposeRequest{
		ProjectName:         fmt.Sprintf("rea-%s", strings.TrimSuffix(bundle.TargetBinary, ".bin")),
		TargetBinaryOrSpec:  bundle.TargetBinary,
		Language:            "rust",
		RequestedModules:    detectedModules,
		UserCustomOverrides: userRequests,
	}

	return d.DecomposeProject(req)
}

func inferCapabilities(moduleName string) []string {
	n := strings.ToLower(moduleName)
	switch {
	case strings.Contains(n, "render") || strings.Contains(n, "gpu") || strings.Contains(n, "shader"):
		return []string{"gpu-rendering", "shaders", "graphics"}
	case strings.Contains(n, "storage") || strings.Contains(n, "database") || strings.Contains(n, "db"):
		return []string{"database", "edge-db", "sqlite-compatible"}
	case strings.Contains(n, "cte") || strings.Contains(n, "hierarchical") || strings.Contains(n, "query"):
		return []string{"cte", "recursive-queries", "hierarchical-sql"}
	case strings.Contains(n, "pixel") || strings.Contains(n, "image") || strings.Contains(n, "imaging"):
		return []string{"pixels", "imaging", "rendering", "simd"}
	case strings.Contains(n, "graph") || strings.Contains(n, "dag") || strings.Contains(n, "semantic"):
		return []string{"dag", "graph", "semantics"}
	case strings.Contains(n, "orm"):
		return []string{"orm", "query-builder", "sql"}
	default:
		return []string{n}
	}
}
