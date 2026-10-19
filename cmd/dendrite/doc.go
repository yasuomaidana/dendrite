package main

import (
	"context"
	"fmt"
	"os"

	"Dendrite/pkg/docgen/reflect"
)

func handleDocCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "generate":
		handleDocusaurusCommand(ctx, append([]string{"generate"}, args...))
	case "audit":
		handleDocusaurusCommand(ctx, append([]string{"audit"}, args...))
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "plane":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite doc plane <push|pull|sync> [--dir docs]")
			return
		}
		dir := "docs"
		for i := 1; i < len(args); i++ {
			if (args[i] == "--dir" || args[i] == "-d") && i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		}
		if err := runPlaneDocSync(ctx, args[0], dir, os.Stdout); err != nil {
			fmt.Printf("[-] %v\n", err)
		}

	case "reflect":
		refDir := "docs"
		srcDir := "."
		targetDir := "reflected_docs"
		for i := 0; i < len(args); i++ {
			if args[i] == "--reference" && i+1 < len(args) {
				refDir = args[i+1]
				i++
			} else if args[i] == "--source" && i+1 < len(args) {
				srcDir = args[i+1]
				i++
			} else if args[i] == "--target" && i+1 < len(args) {
				targetDir = args[i+1]
				i++
			}
		}
		fmt.Printf("[+] Reflecting documentation taxonomy from '%s' to '%s' (source: '%s')...\n", refDir, targetDir, srcDir)
		result, err := reflect.ReflectTaxonomy(refDir, srcDir, targetDir)
		if err != nil {
			fmt.Printf("[-] Reflection failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Reflected %d files in %dms (Sidebar Synced: %v)\n", len(result.FilesWritten), result.DurationMs, result.SidebarSynced)
		for _, f := range result.FilesWritten {
			fmt.Printf("    - %s\n", f)
		}
	default:
		fmt.Printf("Unknown doc action: %s\n", action)
	}
}
