package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/pool/dag"
)

var defaultPoolManager = dag.NewPoolManager("")
var defaultDAGSolver = dag.NewDAGSolver()

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
}
