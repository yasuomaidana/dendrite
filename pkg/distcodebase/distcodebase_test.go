package distcodebase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/rea"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
)

func TestRegistry_DefaultsAndPriorityResolution(t *testing.T) {
	reg := NewDefaultRegistry()

	sources := reg.ListSources()
	if len(sources) < 7 {
		t.Fatalf("expected at least 7 default sources, got %d", len(sources))
	}

	// Verify priority sorting (Priority 1 first)
	if sources[0].Priority != 1 || sources[0].ID != "src-own-yasuomaidana" {
		t.Errorf("expected Priority 1 own source first, got ID=%s Priority=%d", sources[0].ID, sources[0].Priority)
	}

	// Verify libraries catalog
	libs := reg.ListLibraries()
	if len(libs) < 7 {
		t.Fatalf("expected at least 7 default libraries, got %d", len(libs))
	}

	// Verify specific required libraries exist
	requiredLibs := []string{"wgpu", "turso", "diesel", "diesel-cte-ext", "Docula", "semdag", "Oxipix"}
	for _, reqName := range requiredLibs {
		found := false
		for _, l := range libs {
			if strings.EqualFold(l.Name, reqName) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required library %s missing from catalog", reqName)
		}
	}

	// Test Capability Resolution in Priority Order
	// 1. Resolve GPU rendering -> wgpu
	gpuLib, found := reg.ResolveCapability("gpu-rendering", "rust")
	if !found || gpuLib.Name != "wgpu" {
		t.Errorf("expected wgpu for gpu-rendering, got %+v (found=%t)", gpuLib, found)
	}

	// 2. Resolve CTE -> diesel-cte-ext
	cteLib, found := reg.ResolveCapability("cte", "rust")
	if !found || cteLib.Name != "diesel-cte-ext" {
		t.Errorf("expected diesel-cte-ext for cte, got %+v (found=%t)", cteLib, found)
	}

	// 3. Resolve edge DB -> turso
	dbLib, found := reg.ResolveCapability("edge-db", "rust")
	if !found || dbLib.Name != "turso" {
		t.Errorf("expected turso for edge-db, got %+v (found=%t)", dbLib, found)
	}

	// 4. Resolve pixels / imaging -> Oxipix (Priority 1)
	pixLib, found := reg.ResolveCapability("pixels", "rust")
	if !found || pixLib.Name != "Oxipix" || !pixLib.IsOwned || pixLib.Priority != 1 {
		t.Errorf("expected owned Oxipix for pixels, got %+v (found=%t)", pixLib, found)
	}

	// Verify IsOwnedLibrary
	if !reg.IsOwnedLibrary("Oxipix") {
		t.Error("expected Oxipix to be owned")
	}
	if !reg.IsOwnedLibrary("Docula") {
		t.Error("expected Docula to be owned")
	}
	if reg.IsOwnedLibrary("wgpu") {
		t.Error("expected wgpu NOT to be owned")
	}
	if reg.IsOwnedLibrary("unknown-lib") {
		t.Error("expected unknown-lib NOT to be owned")
	}
}

func TestRegistry_AddSourceAndErrorCases(t *testing.T) {
	reg := NewRegistry()

	// Error on invalid priority <= 0
	err := reg.AddSource(CodebaseSource{
		ID:       "invalid",
		Priority: 0,
	})
	if err != ErrInvalidPriority {
		t.Errorf("expected ErrInvalidPriority, got %v", err)
	}

	// Success
	err = reg.AddSource(CodebaseSource{
		ID:       "custom-git",
		Priority: 1,
		URL:      "git://internal.server/repo",
	})
	if err != nil {
		t.Fatalf("unexpected error adding source: %v", err)
	}

	// Error on duplicate
	err = reg.AddSource(CodebaseSource{
		ID:       "custom-git",
		Priority: 1,
	})
	if err != ErrDuplicateSource {
		t.Errorf("expected ErrDuplicateSource, got %v", err)
	}

	// GetSource success and error
	src, err := reg.GetSource("custom-git")
	if err != nil || src.ID != "custom-git" {
		t.Errorf("failed to get source: %v", err)
	}
	_, err = reg.GetSource("non-existent")
	if err != ErrSourceNotFound {
		t.Errorf("expected ErrSourceNotFound, got %v", err)
	}

	// Add Library with inherited priority
	err = reg.AddLibrary(LibraryEntry{
		Name:         "custom-util",
		SourceID:     "custom-git",
		Language:     "rust",
		Capabilities: []string{"custom-computation"},
	})
	if err != nil {
		t.Fatalf("failed to add library: %v", err)
	}

	foundLib, found := reg.ResolveCapability("custom-computation", "rust")
	if !found || foundLib.Priority != 1 {
		t.Errorf("expected custom-util with Priority 1, got %+v", foundLib)
	}

	// Non-matching capability returns false
	_, found = reg.ResolveCapability("non-existent-capability-12345", "rust")
	if found {
		t.Error("expected false for nonexistent capability")
	}
}

func TestDecomposer_ForcedModularAndScenarios(t *testing.T) {
	reg := NewDefaultRegistry()
	decomposer := NewDecomposer(reg)

	// Scenario 1: Monolithic project is forcibly divided by default
	req := DecomposeRequest{
		ProjectName: "QuantumEngine",
		Language:    "rust",
		RequestedModules: []string{
			"render-pipeline",
			"storage-engine",
			"hierarchical-query",
			"pixel-processor",
			"custom-kernel",
		},
		UserCustomOverrides: []string{"custom-kernel"},
		TechnicalBenefits: map[string]string{
			"storage-engine": "Proprietary zero-allocation ring buffer required for < 500ns writes",
		},
	}

	plan, err := decomposer.DecomposeProject(req)
	if err != nil {
		t.Fatalf("failed to decompose project: %v", err)
	}

	if !plan.ForcedModular {
		t.Error("expected ForcedModular to be true by default")
	}
	if plan.TotalModules != 5 {
		t.Errorf("expected 5 modules, got %d", plan.TotalModules)
	}

	// render-pipeline should reuse wgpu
	renderMod := plan.Modules[0]
	if renderMod.Strategy != StrategyReuseLibrary || renderMod.MatchedLibrary == nil || renderMod.MatchedLibrary.Name != "wgpu" {
		t.Errorf("render-pipeline expected to reuse wgpu, got: %+v", renderMod)
	}

	// storage-engine had technical benefit specified -> develop custom
	storageMod := plan.Modules[1]
	if storageMod.Strategy != StrategyDevelopCustom || !strings.Contains(storageMod.DevelopCustomReason, "zero-allocation") {
		t.Errorf("storage-engine expected custom implementation with technical benefit, got: %+v", storageMod)
	}

	// hierarchical-query should reuse diesel-cte-ext
	cteMod := plan.Modules[2]
	if cteMod.Strategy != StrategyReuseLibrary || cteMod.MatchedLibrary == nil || cteMod.MatchedLibrary.Name != "diesel-cte-ext" {
		t.Errorf("hierarchical-query expected to reuse diesel-cte-ext, got: %+v", cteMod)
	}

	// pixel-processor should reuse Oxipix (Priority 1)
	pixMod := plan.Modules[3]
	if pixMod.Strategy != StrategyReuseLibrary || pixMod.MatchedLibrary == nil || pixMod.MatchedLibrary.Name != "Oxipix" {
		t.Errorf("pixel-processor expected to reuse Oxipix, got: %+v", pixMod)
	}

	// custom-kernel had explicit user override
	customMod := plan.Modules[4]
	if customMod.Strategy != StrategyDevelopCustom || !customMod.UserRequestedCustom {
		t.Errorf("custom-kernel expected explicit user request, got: %+v", customMod)
	}

	// Error test: empty project name
	_, err = decomposer.DecomposeProject(DecomposeRequest{ProjectName: ""})
	if err != ErrEmptyProjectName {
		t.Errorf("expected ErrEmptyProjectName, got %v", err)
	}
}

func TestDecomposer_REAEvidenceIntegration(t *testing.T) {
	decomposer := NewDecomposer(nil)

	bundle := rea.NewEvidenceBundle(
		"legacy_app.bin",
		rea.FormatMachO,
		"init_renderer",
		"void init_renderer() { wgpu_render_pass(); execute_sql_table(); simd_pixel_blend(); }",
		"mov rax, rbx\nsyscall",
		[]string{"table_cte_recurse", "wgpu_swapchain"},
		[]string{"SELECT * FROM hierarchy", "RGBA_SIMD"},
	)

	plan, err := decomposer.DecomposeREAEvidence(bundle, []string{})
	if err != nil {
		t.Fatalf("expected REA decomposition success, got %v", err)
	}

	if plan.TotalModules == 0 {
		t.Fatal("expected at least 1 decomposed module from REA bundle")
	}

	// Nil bundle error
	_, err = decomposer.DecomposeREAEvidence(nil, nil)
	if err == nil {
		t.Error("expected error on nil bundle")
	}
}

func TestInteropCoordinator_FullLifecycle(t *testing.T) {
	reg := NewDefaultRegistry()
	coordinator := NewInteropCoordinator(reg)

	// Scenario 3: Docula depends on Oxipix. Oxipix lacks blend_modes_simd.
	// Since Oxipix is user-owned (Priority 1), coordinator creates interop ticket and unblocks Docula with fallback.
	req := InteropRequest{
		RequesterProject:  "Docula",
		DependencyProject: "Oxipix",
		FeatureRequested:  "blend_modes_simd",
		AllowFallback:     true,
	}

	coord, err := coordinator.CoordinateFeature(req)
	if err != nil {
		t.Fatalf("failed to coordinate feature: %v", err)
	}

	if !coord.DependencyIsOwned {
		t.Error("expected Oxipix to be flagged as user-owned (Priority 1)")
	}
	if coord.State != InteropUnblockedWithFallback {
		t.Errorf("expected state %s, got %s", InteropUnblockedWithFallback, coord.State)
	}
	if !strings.Contains(coord.FallbackTODO, "TODO(interop:Oxipix)") {
		t.Errorf("expected TODO marker for Oxipix, got %s", coord.FallbackTODO)
	}

	// Verify prompt generation for local LLM (Ollama)
	prompt := coordinator.FormatLocalLLMPrompt(coord)
	if !strings.Contains(prompt, "Docula") || !strings.Contains(prompt, "Oxipix") {
		t.Errorf("malformed local LLM prompt: %s", prompt)
	}

	// oxipix completes feature
	updatedCoord, err := coordinator.CompleteDependencyFeature(coord.ID)
	if err != nil {
		t.Fatalf("failed to complete dependency feature: %v", err)
	}
	if updatedCoord.State != InteropDependencyCompleted {
		t.Errorf("expected state %s, got %s", InteropDependencyCompleted, updatedCoord.State)
	}

	// Reconcile in Docula (removes interim fallback, applies native crate call)
	reconcileResult, err := coordinator.ReconcileProject(coord.ID)
	if err != nil {
		t.Fatalf("failed to reconcile project: %v", err)
	}
	if reconcileResult.Status != "successfully_reconciled" || !strings.Contains(reconcileResult.PatchGenerated, "use oxipix::blend_modes_simd;") {
		t.Errorf("unexpected reconciliation result: %+v", reconcileResult)
	}

	// Get & List coordinations
	retrieved, err := coordinator.GetCoordination(coord.ID)
	if err != nil || retrieved.State != InteropIntegrated {
		t.Errorf("failed to retrieve coordination: %v", err)
	}
	list := coordinator.ListCoordinations()
	if len(list) != 1 {
		t.Errorf("expected 1 coordination, got %d", len(list))
	}

	// Error handling
	_, err = coordinator.GetCoordination("non-existent")
	if err != ErrCoordinationNotFound {
		t.Errorf("expected ErrCoordinationNotFound, got %v", err)
	}
	_, err = coordinator.CoordinateFeature(InteropRequest{})
	if err != ErrEmptyProjectName {
		t.Errorf("expected ErrEmptyProjectName, got %v", err)
	}
}

func TestProjectSpawner_SpawnAndParallel(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dendrite_spawner_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock Plane server for Scenario 4
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/projects/") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			proj := plane.PlaneProject{
				ID:          "plane-proj-carla-sensor",
				Name:        body["name"].(string),
				Identifier:  body["identifier"].(string),
				Description: body["description"].(string),
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(proj)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer planeServer.Close()

	planeClient, err := plane.NewClient(plane.Config{
		BaseURL:       planeServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "main-proj",
		DirectToken:   "dummy-token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to construct plane client: %v", err)
	}

	wsConfigPath := filepath.Join(tempDir, "workspace_config.json")
	wsBaseDir := filepath.Join(tempDir, "workspaces")
	wsReg, err := workspace.NewRegistry(wsConfigPath, wsBaseDir)
	if err != nil {
		t.Fatalf("failed to construct workspace registry: %v", err)
	}

	spawner := NewProjectSpawner(planeClient, wsReg)

	// Scenario 4: A project generates another one (e.g. Carla in Rust generates Carla-Sensor-RS side-project and Plane project)
	rustTarget := filepath.Join(tempDir, "carla-sensor-rs")
	res, err := spawner.Spawn(context.Background(), SpawnRequest{
		ProjectName:        "carla-sensor-rs",
		Identifier:         "CSEN",
		TargetDir:          rustTarget,
		Language:           "rust",
		IsSatellite:        true,
		CreatePlaneProject: true,
		InitGit:            true,
		Description:        "Dedicated sensor suite for 100% Rust Carla rewrite",
	})
	if err != nil {
		t.Fatalf("failed to spawn rust project: %v", err)
	}

	if res.PlaneProjectID != "plane-proj-carla-sensor" {
		t.Errorf("expected PlaneProjectID plane-proj-carla-sensor, got %s", res.PlaneProjectID)
	}
	if !res.GitInitialized {
		t.Error("expected git to be initialized")
	}

	// Verify generated scaffolding files
	if _, err := os.Stat(filepath.Join(rustTarget, "Cargo.toml")); err != nil {
		t.Errorf("Cargo.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rustTarget, "src", "lib.rs")); err != nil {
		t.Errorf("src/lib.rs missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rustTarget, "README.md")); err != nil {
		t.Errorf("README.md missing: %v", err)
	}

	// Verify workspace registered in Dendrite
	regProj, err := wsReg.GetProject("carla-sensor-rs")
	if err != nil || regProj == nil {
		t.Errorf("project not registered in workspace: %v", err)
	}

	// Test parallel spawning (Scenario 2)
	p1 := filepath.Join(tempDir, "parallel-mod-a")
	p2 := filepath.Join(tempDir, "parallel-mod-b")
	requests := []SpawnRequest{
		{
			ProjectName: "parallel-mod-a",
			TargetDir:   p1,
			Language:    "go",
			InitGit:     false,
		},
		{
			ProjectName: "parallel-mod-b",
			TargetDir:   p2,
			Language:    "typescript",
			InitGit:     false,
		},
	}

	parallelResults, err := spawner.SpawnParallel(context.Background(), requests)
	if err != nil {
		t.Fatalf("failed parallel spawning: %v", err)
	}
	if len(parallelResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parallelResults))
	}

	// Verify Go scaffolding
	if _, err := os.Stat(filepath.Join(p1, "go.mod")); err != nil {
		t.Errorf("go.mod missing in parallel-mod-a: %v", err)
	}
	// Verify TS scaffolding
	if _, err := os.Stat(filepath.Join(p2, "package.json")); err != nil {
		t.Errorf("package.json missing in parallel-mod-b: %v", err)
	}

	// Error test: directory already exists
	_, err = spawner.Spawn(context.Background(), SpawnRequest{
		ProjectName: "carla-sensor-rs",
		TargetDir:   rustTarget,
	})
	if err != ErrProjectDirExists {
		t.Errorf("expected ErrProjectDirExists, got %v", err)
	}
}
