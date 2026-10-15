package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/query/slice"
)

var defaultSlicer = slice.NewIntentSlicer()

func handleQueryCommand(ctx context.Context, action string, args []string) {
	req := slice.SliceRequest{
		SourcesPath: ".",
		Direction:   slice.DirectionForward,
	}

	allArgs := append([]string{action}, args...)
	var intentParts []string
	jsonOutput := false

	for i := 0; i < len(allArgs); i++ {
		arg := allArgs[i]
		switch arg {
		case "slice":
			// subcommand marker
		case "--sources":
			if i+1 < len(allArgs) {
				req.SourcesPath = allArgs[i+1]
				i++
			}
		case "--entrypoint":
			if i+1 < len(allArgs) {
				req.EntryPointSymbol = allArgs[i+1]
				i++
			}
		case "--direction":
			if i+1 < len(allArgs) {
				req.Direction = slice.SliceDirection(allArgs[i+1])
				i++
			}
		case "--output-dir":
			if i+1 < len(allArgs) {
				req.OutputDir = allArgs[i+1]
				i++
			}
		case "--json":
			jsonOutput = true
		default:
			if !strings.HasPrefix(arg, "-") {
				intentParts = append(intentParts, arg)
			}
		}
	}

	req.Intent = strings.Join(intentParts, " ")
	if req.EntryPointSymbol == "" {
		printQueryUsage()
		return
	}

	res, err := defaultSlicer.Slice(ctx, req)
	if err != nil {
		fmt.Printf("[-] Universal query slice failed: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[+] Universal Intent Reachability Slicing:")
	fmt.Printf("    Entrypoint:        %s\n", res.EntryPoint)
	fmt.Printf("    Intent:            %s\n", res.Intent)
	fmt.Printf("    Original Symbols:  %d\n", res.OriginalSymbolsCount)
	fmt.Printf("    Retained Symbols:  %d (%s)\n", res.RetainedSymbolsCount, strings.Join(res.RetainedSymbols, ", "))
	fmt.Printf("    Debloat Reduction: %.1f%%\n", res.PruneRatio*100)
	fmt.Printf("    Golden Vectors:    %d parity vectors generated\n", len(res.GoldenVectors))
	if res.EmittedProjectPath != "" {
		fmt.Printf("    Emitted Project:   %s\n", res.EmittedProjectPath)
	}
}

func printQueryUsage() {
	fmt.Println("Usage: dendrite query [slice] --sources <path> --entrypoint <symbol> [flags] \"<intent>\"")
	fmt.Println("\nAvailable Flags:")
	fmt.Println("  --sources <path>         Target source directory or binary")
	fmt.Println("  --entrypoint <symbol>    Root function or method symbol for reachability traversal")
	fmt.Println("  --direction <dir>        Traversal direction: forward, backward, bidirectional (default: forward)")
	fmt.Println("  --output-dir <dir>       Emit debloated standalone project and golden test harness")
	fmt.Println("  --json                   Output structured JSON result")
}
