package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/pool/split"
	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/ui/modal"
)

var defaultPoolManager = dag.NewPoolManager("")
var defaultDAGSolver = dag.NewDAGSolver()
var defaultSwitcher = switcher.NewDependencySwitcher()
var defaultSplitter = split.NewProjectSplitter()

func handlePoolCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "add":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite pool add <dir1> [dir2 ...]")
			return
		}
		added, err := defaultPoolManager.Add(args...)
		if err != nil {
			fmt.Printf("[-] Error adding project to pool: %v\n", err)
			return
		}
		if err := defaultPoolManager.Save(); err != nil {
			fmt.Printf("[-] Warning: Failed to persist pool config: %v\n", err)
		}
		fmt.Printf("[+] Successfully registered %d project(s) in pool:\n", len(added))
		for _, p := range added {
			ecoStr := "none"
			if len(p.Ecosystems) > 0 {
				var ecos []string
				for _, e := range p.Ecosystems {
					ecos = append(ecos, string(e))
				}
				ecoStr = strings.Join(ecos, ", ")
			}
			fmt.Printf("    * %-25s | Ecosystems: %-15s | Path: %s\n", p.Name, ecoStr, p.Path)
			if len(p.Dependencies) > 0 {
				fmt.Printf("      Declared Dependencies (%d): %s\n", len(p.Dependencies), strings.Join(p.Dependencies, ", "))
			}
		}

	case "remove", "rm":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite pool remove <name_or_path>")
			return
		}
		if err := defaultPoolManager.Remove(args[0]); err != nil {
			fmt.Printf("[-] Error removing project from pool: %v\n", err)
			return
		}
		if err := defaultPoolManager.Save(); err != nil {
			fmt.Printf("[-] Warning: Failed to persist pool config: %v\n", err)
		}
		fmt.Printf("[+] Removed '%s' from pool.\n", args[0])

	case "list", "ls":
		projects := defaultPoolManager.List()
		if len(args) > 0 && args[0] == "--json" {
			data, _ := json.MarshalIndent(projects, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("[+] Registered Pool Members (%d):\n", len(projects))
		if len(projects) == 0 {
			fmt.Println("    (No projects registered. Use 'dendrite pool add <dir>' to add members)")
			return
		}
		for i, p := range projects {
			var ecos []string
			for _, e := range p.Ecosystems {
				ecos = append(ecos, string(e))
			}
			fmt.Printf("  %2d. %-25s [%s] -> %s\n", i+1, p.Name, strings.Join(ecos, ", "), p.Path)
			if len(p.Manifests) > 0 {
				fmt.Printf("      Manifests: %s\n", strings.Join(p.Manifests, ", "))
			}
		}

	case "resolve":
		projects := defaultPoolManager.List()
		if len(projects) == 0 {
			fmt.Println("[-] Pool is empty. Register directories first using 'dendrite pool add <dir>'")
			return
		}

		res, err := defaultDAGSolver.Resolve(projects)
		if err != nil {
			fmt.Printf("[-] Dependency DAG resolution failed: %v\n", err)
			os.Exit(1)
		}

		if len(args) > 0 && args[0] == "--json" {
			data, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Println("[+] Autonomous Multi-Directory DAG Resolution:")
		fmt.Printf("    Nodes: %d | Edges: %d\n", len(res.Nodes), len(res.Edges))
		fmt.Printf("    Topological Build Order (Foundational First):\n")
		for i, id := range res.TopologicalOrder {
			node := res.Nodes[id]
			depCount := len(node.DependsOn)
			fmt.Printf("      %2d. %-25s (depends on %d upstream)\n", i+1, id, depCount)
		}

		if len(res.Edges) > 0 {
			fmt.Println("\n    Discovered Dependency Edges:")
			for _, edge := range res.Edges {
				fmt.Printf("      * %s -> %s (via %s)\n", edge.From, edge.To, edge.Ecosystem)
			}
		}

		if len(res.LeafNodes) > 0 {
			fmt.Printf("\n    Leaf Nodes (Foundational / Zero Dependencies): %s\n", strings.Join(res.LeafNodes, ", "))
		}
		if len(res.RootNodes) > 0 {
			fmt.Printf("    Root Nodes (Top-Level / No Dependents):        %s\n", strings.Join(res.RootNodes, ", "))
		}

	case "switch":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite pool switch <dep_name> --in <project_dir> --mode <published|branch|local|fork> [flags]")
			return
		}

		depName := args[0]
		req := switcher.SwitchRequest{
			DependencyName:   depName,
			TargetProjectDir: ".",
			SyncIDE:          true,
		}

		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--in":
				if i+1 < len(args) {
					req.TargetProjectDir = args[i+1]
					i++
				}
			case "--mode":
				if i+1 < len(args) {
					req.Mode = switcher.DependencyMode(args[i+1])
					i++
				}
			case "--version":
				if i+1 < len(args) {
					req.Version = args[i+1]
					i++
				}
			case "--branch":
				if i+1 < len(args) {
					req.BranchName = args[i+1]
					i++
				}
			case "--commit":
				if i+1 < len(args) {
					req.CommitSHA = args[i+1]
					i++
				}
			case "--local-path":
				if i+1 < len(args) {
					req.LocalPath = args[i+1]
					i++
				}
			case "--fork-url":
				if i+1 < len(args) {
					req.ForkURL = args[i+1]
					i++
				}
			case "--fork-branch":
				if i+1 < len(args) {
					req.ForkBranch = args[i+1]
					i++
				}
			case "--no-sync-ide":
				req.SyncIDE = false
			}
		}

		result, err := defaultSwitcher.Switch(ctx, req)
		if err != nil {
			fmt.Printf("[-] Dependency mode switch failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[+] Successfully switched '%s' dependency mode:\n", result.DependencyName)
		fmt.Printf("    Target Project:     %s\n", result.TargetProject)
		fmt.Printf("    Transition:         %s -> %s\n", result.PreviousMode, result.NewMode)
		fmt.Printf("    Modified Files (%d): %s\n", len(result.ModifiedFiles), strings.Join(result.ModifiedFiles, ", "))
		if result.IDEScopesUpdated {
			fmt.Println("    JetBrains Scopes:   Synchronized (.idea/scopes/Dendrite_Local_Pool.xml)")
		}

	case "split":
		req := split.SplitRequest{
			SourceProjectDir: ".",
			WireLocalMode:    true,
			RegisterInPool:   true,
			Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
				return m.RenderCLI(os.Stdin, os.Stdout)
			},
		}

		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--from":
				if i+1 < len(args) {
					req.SourceProjectDir = args[i+1]
					i++
				}
			case "--module":
				if i+1 < len(args) {
					req.ModuleSubpath = args[i+1]
					i++
				}
			case "--name":
				if i+1 < len(args) {
					req.NewMemberName = args[i+1]
					i++
				}
			case "--target-dir":
				if i+1 < len(args) {
					req.TargetDirectory = args[i+1]
					i++
				}
			case "--no-wire-local":
				req.WireLocalMode = false
			case "--no-register-pool":
				req.RegisterInPool = false
			}
		}

		if req.ModuleSubpath == "" || req.NewMemberName == "" {
			fmt.Println("[-] Usage: dendrite pool split --from <dir> --module <pkg> --name <member> [--target-dir <dir>]")
			return
		}

		result, err := defaultSplitter.Split(ctx, req)
		if err != nil {
			fmt.Printf("[-] Project split failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[+] Successfully split module into new pool member '%s':\n", result.NewMemberName)
		fmt.Printf("    Destination:      %s\n", result.TargetDirectory)
		fmt.Printf("    Manifest Created: %s\n", result.ManifestCreated)
		fmt.Printf("    Extracted Files:  %d files moved\n", len(result.ExtractedFiles))
		if result.ParentWiredLocal {
			fmt.Println("    Parent Wired:     Local dependency mode active in parent project")
		}
		if result.PoolRegistered {
			fmt.Println("    Pool Registered:  Member added to .dendrite/pool.json")
		}

	default:
		printPoolUsage()
	}
}

func printPoolUsage() {
	fmt.Println("Usage: dendrite pool <command> [args...]")
	fmt.Println("\nAvailable Subcommands:")
	fmt.Println("  add <dir1> [dir2...]    Register project directories into .dendrite/pool.json")
	fmt.Println("  remove <name_or_path>   Remove a registered project from the pool")
	fmt.Println("  list [--json]           List all registered pool member projects")
	fmt.Println("  resolve [--json]        Compute deterministic topological DAG & detect cycles")
	fmt.Println("  split [flags]           Extract module slice into a new project with mandatory directory gate")
	fmt.Println("                          Flags: --from <dir> --module <pkg> --name <member> [--target-dir <dir>]")
	fmt.Println("  switch <dep> [flags]    Switch dependency mode: published, branch, local, fork")
	fmt.Println("                          Flags: --in <dir> --mode <m> --version <v> --branch <b>")
	fmt.Println("                                 --local-path <p> --fork-url <u> --fork-branch <b>")
}
