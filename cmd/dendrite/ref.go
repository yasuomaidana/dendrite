package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/query/reuse"
)

var defaultRefMgr = reuse.NewRefManager(".dendrite/refs.json")

func handleRefCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "list":
		handleRefList(args)
	case "add":
		handleRefAdd(args)
	case "remove", "rm":
		handleRefRemove(args)
	default:
		printRefUsage()
	}
}

func handleRefList(args []string) {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	refs, err := defaultRefMgr.ListRefs()
	if err != nil {
		fmt.Printf("[-] Failed to list source references: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(refs, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[+] Registered Source References (Tier 1: %d repos):\n", len(refs))
	for _, r := range refs {
		fmt.Printf("  • %-18s [%-10s] %s\n", r.ID, r.Language, r.Repository)
		if len(r.Capabilities) > 0 {
			fmt.Printf("    Capabilities: %s\n", strings.Join(r.Capabilities, ", "))
		}
	}
}

func handleRefAdd(args []string) {
	var ref reuse.SourceReference
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 < len(args) {
				ref.ID = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				ref.Name = args[i+1]
				i++
			}
		case "--repo", "--repository":
			if i+1 < len(args) {
				ref.Repository = args[i+1]
				i++
			}
		case "--lang", "--language":
			if i+1 < len(args) {
				ref.Language = args[i+1]
				i++
			}
		case "--caps", "--capabilities":
			if i+1 < len(args) {
				raw := strings.Split(args[i+1], ",")
				for _, c := range raw {
					trimmed := strings.TrimSpace(c)
					if trimmed != "" {
						ref.Capabilities = append(ref.Capabilities, trimmed)
					}
				}
				i++
			}
		case "--license":
			if i+1 < len(args) {
				ref.License = args[i+1]
				i++
			}
		}
	}

	if ref.ID == "" || ref.Repository == "" {
		fmt.Println("[-] Usage: dendrite ref add --id <id> --name <name> --repo <url> [--lang <lang>] [--caps <c1,c2>]")
		return
	}
	if ref.Name == "" {
		ref.Name = ref.ID
	}

	if err := defaultRefMgr.AddRef(ref); err != nil {
		fmt.Printf("[-] Failed to register reference: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[+] Registered Tier 1 Source Reference: %s (%s)\n", ref.Name, ref.Repository)
}

func handleRefRemove(args []string) {
	if len(args) == 0 {
		fmt.Println("[-] Usage: dendrite ref remove <id>")
		return
	}
	id := args[0]
	if err := defaultRefMgr.RemoveRef(id); err != nil {
		fmt.Printf("[-] Failed to remove reference: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[+] Removed Tier 1 Source Reference: %s\n", id)
}

func printRefUsage() {
	fmt.Println("Usage: dendrite ref <list|add|remove> [flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  dendrite ref list [--json]")
	fmt.Println("  dendrite ref add --id <id> --name <name> --repo <url> [--lang <lang>] [--caps <c1,c2>]")
	fmt.Println("  dendrite ref remove <id>")
}
