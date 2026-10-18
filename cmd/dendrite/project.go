package main

import (
	"context"
	"fmt"
	"time"

	"Dendrite/pkg/workspace"
)

func handleProjectCommand(ctx context.Context, action string, args []string) {
	reg, err := workspace.NewRegistry("", "")
	if err != nil {
		fmt.Printf("[-] Failed to open workspace registry: %v\n", err)
		return
	}

	switch action {
	case "prepare", "sync", "update":
		handleEnvPrepareCommand(ctx, args)
		return
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "add":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite project add <name> <path>")
			return
		}
		name, path := args[0], args[1]
		entry, err := reg.AddProject(name, path)
		if err != nil {
			fmt.Printf("[-] Failed to register project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Project '%s' registered successfully (Virtual Key: %s)\n", entry.Name, entry.VirtualKeyAlias)
		fmt.Printf("    - Code Path:   %s\n", entry.Path)
		fmt.Printf("    - Data Dir:    %s\n", entry.DataDir)
		fmt.Printf("    - Graph DB:    %s\n", entry.GraphDBPath)
		fmt.Printf("    - Spend Cache: %s\n", entry.SpendCachePath)

	case "switch":
		if len(args) < 1 {
			fmt.Println("[-] Usage: dendrite project switch <name>")
			return
		}
		name := args[0]
		start := time.Now()
		entry, err := reg.SwitchProject(name)
		duration := time.Since(start)
		if err != nil {
			fmt.Printf("[-] Failed to switch project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Switched active context to '%s' in %v\n", entry.Name, duration)
		fmt.Printf("    - Virtual Key Alias: %s\n", entry.VirtualKeyAlias)
		fmt.Printf("    - Active Workspace:  %s\n", entry.Path)

	case "list":
		projects := reg.ListProjects()
		active, _ := reg.GetActiveProject()
		activeName := ""
		if active != nil {
			activeName = active.Name
		}
		fmt.Printf("[+] Registered Workspaces (%d):\n", len(projects))
		for _, p := range projects {
			mark := " "
			if p.Name == activeName {
				mark = "*"
			}
			fmt.Printf("  [%s] %-15s [%s] -> %s (Key: %s)\n", mark, p.Name, p.State, p.Path, p.VirtualKeyAlias)
		}

	case "remove":
		if len(args) < 1 {
			fmt.Println("[-] Usage: dendrite project remove <name> [--purge]")
			return
		}
		name := args[0]
		purge := false
		if len(args) > 1 && args[1] == "--purge" {
			purge = true
		}
		if err := reg.RemoveProject(name, purge); err != nil {
			fmt.Printf("[-] Failed to remove project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Project '%s' removed (Purge Data: %v)\n", name, purge)

	default:
		fmt.Printf("[-] Unknown project action: %s\n", action)
	}
}
