package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/distcodebase"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
)

var defaultDistRegistry = distcodebase.NewDefaultRegistry()
var defaultInteropCoordinator = distcodebase.NewInteropCoordinator(defaultDistRegistry)

func handleDistCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "sources", "list-sources":
		sources := defaultDistRegistry.ListSources()
		libs := defaultDistRegistry.ListLibraries()

		fmt.Printf("[+] Distributed Codebase Sources (%d):\n", len(sources))
		for _, s := range sources {
			fmt.Printf("    [%s] Priority %d | %-25s | URL: %s\n", s.Type, s.Priority, s.Name, s.URL)
		}

		fmt.Printf("\n[+] Registered Ecosystem Libraries & Tools (%d):\n", len(libs))
		for _, l := range libs {
			ownedTag := ""
			if l.IsOwned {
				ownedTag = " (USER OWNED - Priority 1)"
			}
			fmt.Printf("    - %-15s (Lang: %-4s, P%d)%s: %s\n", l.Name, l.Language, l.Priority, ownedTag, l.Description)
			fmt.Printf("      Capabilities: %s\n", strings.Join(l.Capabilities, ", "))
		}

	case "resolve":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite dist resolve <capability> [language]")
			return
		}
		capQuery := args[0]
		lang := ""
		if len(args) > 1 {
			lang = args[1]
		}
		lib, found := defaultDistRegistry.ResolveCapability(capQuery, lang)
		if !found {
			fmt.Printf("[-] No matching capability '%s' found in distributed codebase.\n", capQuery)
			return
		}
		fmt.Printf("[+] Highest Priority Match for '%s':\n", capQuery)
		fmt.Printf("    Library:     %s (Priority %d, Owned: %t)\n", lib.Name, lib.Priority, lib.IsOwned)
		fmt.Printf("    Package:     %s\n", lib.RegistryPackage)
		fmt.Printf("    Repository:  %s\n", lib.RepositoryURL)
		fmt.Printf("    Capabilities: %s\n", strings.Join(lib.Capabilities, ", "))

	case "decompose":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite dist decompose <project_name> [target_spec]")
			return
		}
		pName := args[0]
		target := ""
		if len(args) > 1 {
			target = args[1]
		}

		decomposer := distcodebase.NewDecomposer(defaultDistRegistry)
		forced := true
		plan, err := decomposer.DecomposeProject(distcodebase.DecomposeRequest{
			ProjectName:        pName,
			TargetBinaryOrSpec: target,
			Language:           "rust",
			ForcedModular:      &forced,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[-] Decomposition failed: %v\n", err)
			return
		}

		fmt.Printf("[+] Forced Modular Decomposition Plan for '%s':\n", pName)
		fmt.Printf("    Total Modules:     %d\n", plan.TotalModules)
		fmt.Printf("    Reused Libraries:  %d\n", plan.ReusedLibrariesCount)
		fmt.Printf("    Custom Dev:        %d\n", plan.CustomModulesCount)
		fmt.Printf("    Satellite Spawns:  %d\n\n", plan.SatelliteProjectsCount)

		for _, m := range plan.Modules {
			if m.Strategy == distcodebase.StrategyReuseLibrary && m.MatchedLibrary != nil {
				fmt.Printf("    - Module '%s': REUSE -> %s (Priority %d, Repo: %s)\n",
					m.Name, m.MatchedLibrary.Name, m.MatchedLibrary.Priority, m.MatchedLibrary.RepositoryURL)
			} else if m.Strategy == distcodebase.StrategyDevelopCustom {
				fmt.Printf("    - Module '%s': CUSTOM DEV -> Reason: %s\n", m.Name, m.DevelopCustomReason)
			} else {
				fmt.Printf("    - Module '%s': SATELLITE PROJECT -> Spawn as standalone project\n", m.Name)
			}
		}

	case "interop":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite dist interop <coordinate|complete|reconcile|list> [args...]")
			return
		}
		subAction := args[0]
		switch subAction {
		case "coordinate":
			if len(args) < 4 {
				fmt.Println("[-] Usage: dendrite dist interop coordinate <requester> <dependency> <feature>")
				return
			}
			coord, err := defaultInteropCoordinator.CoordinateFeature(distcodebase.InteropRequest{
				RequesterProject:  args[1],
				DependencyProject: args[2],
				FeatureRequested:  args[3],
				AllowFallback:     true,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "[-] Interop coordination failed: %v\n", err)
				return
			}
			fmt.Printf("[+] Interop Coordination Initialized (%s):\n", coord.ID)
			fmt.Printf("    Requester:  %s\n", coord.RequesterProject)
			fmt.Printf("    Dependency: %s (User Owned: %t)\n", coord.DependencyProject, coord.DependencyIsOwned)
			fmt.Printf("    Feature:    %s\n", coord.FeatureRequested)
			fmt.Printf("    Status:     %s\n", coord.State)
			fmt.Printf("    Marker:     %s\n", coord.FallbackTODO)

		case "complete":
			if len(args) < 2 {
				fmt.Println("[-] Usage: dendrite dist interop complete <coordination_id>")
				return
			}
			coord, err := defaultInteropCoordinator.CompleteDependencyFeature(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "[-] Failed to complete feature: %v\n", err)
				return
			}
			fmt.Printf("[+] Dependency feature marked completed for %s. Ready to reconcile.\n", coord.ID)

		case "reconcile":
			if len(args) < 2 {
				fmt.Println("[-] Usage: dendrite dist interop reconcile <coordination_id>")
				return
			}
			rec, err := defaultInteropCoordinator.ReconcileProject(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "[-] Failed to reconcile project: %v\n", err)
				return
			}
			fmt.Printf("[+] Interop Reconciled Successfully (%s):\n", rec.CoordinationID)
			fmt.Printf("    Patch applied to %s:\n%s\n", rec.Requester, rec.PatchGenerated)

		case "list":
			list := defaultInteropCoordinator.ListCoordinations()
			fmt.Printf("[+] Active Interop Coordinations (%d):\n", len(list))
			for _, c := range list {
				fmt.Printf("    - %s: %s -> %s [%s] (Feature: %s)\n", c.ID, c.RequesterProject, c.DependencyProject, c.State, c.FeatureRequested)
			}
		}

	case "spawn":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dist spawn <project_name> <target_dir> [language] [--satellite] [--no-plane]")
			return
		}
		pName := args[0]
		targetDir := args[1]
		lang := "rust"
		if len(args) > 2 && !strings.HasPrefix(args[2], "--") {
			lang = args[2]
		}

		isSat := false
		createPlane := true
		for _, a := range args {
			if a == "--satellite" {
				isSat = true
			}
			if a == "--no-plane" {
				createPlane = false
			}
		}

		var planeClient *plane.Client
		if createPlane {
			planeClient, _ = plane.NewClientFromEnv()
		}
		wsReg, _ := workspace.NewRegistry("", "")
		spawner := distcodebase.NewProjectSpawner(planeClient, wsReg)

		res, err := spawner.Spawn(ctx, distcodebase.SpawnRequest{
			ProjectName:        pName,
			TargetDir:          targetDir,
			Language:           lang,
			IsSatellite:        isSat,
			CreatePlaneProject: createPlane && planeClient != nil,
			InitGit:            true,
			Description:        fmt.Sprintf("Autonomous project %s generated via Dendrite", pName),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[-] Spawning project failed: %v\n", err)
			return
		}

		fmt.Printf("[+] Spawned Project '%s' Successfully:\n", res.ProjectName)
		fmt.Printf("    Path:            %s\n", res.ProjectPath)
		fmt.Printf("    Git Initialized: %t\n", res.GitInitialized)
		if res.PlaneProjectID != "" {
			fmt.Printf("    Plane Project:   %s\n", res.PlaneProjectID)
		}

	default:
		fmt.Println("[+] Dendrite Distributed Codebase & Interoperability Subcommands:")
		fmt.Println("    dendrite dist sources                          - List registered codebase sources & libraries")
		fmt.Println("    dendrite dist resolve <capability> [lang]      - Priority-resolve capability in ecosystem")
		fmt.Println("    dendrite dist decompose <project> [spec]       - Decompose target into reusable modules")
		fmt.Println("    dendrite dist interop <coordinate|complete|reconcile|list> - Coordinate cross-project dependencies")
		fmt.Println("    dendrite dist spawn <name> <dir> [lang]        - Spawn parallel/satellite sub-project")
	}
}
