package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/polyglot/contract"
)

var defaultStoryMonitor = contract.NewContractMonitor(nil)

func handleStoryCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "start":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite story start \"<story-name>\" [--sync-contracts] [--schema-dir <dir>]")
			return
		}
		handleStoryStart(ctx, args[0], args[1:])
	case "check-contracts", "sync-contracts":
		handleStoryCheckContracts(ctx, args)
	default:
		printStoryUsage()
	}
}

func handleStoryStart(ctx context.Context, storyName string, args []string) {
	syncContracts := false
	schemaDirs := []string{"schemas", "proto", "api"}
	codeDirs := map[contract.TargetLanguage]string{
		contract.LangGo:         "pkg",
		contract.LangRust:       "crates",
		contract.LangTypeScript: "src",
		contract.LangPython:     "python",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sync-contracts":
			syncContracts = true
		case "--schema-dir":
			if i+1 < len(args) {
				schemaDirs = []string{args[i+1]}
				i++
			}
		case "--code-dir":
			if i+1 < len(args) {
				parts := strings.Split(args[i+1], "=")
				if len(parts) == 2 {
					codeDirs[contract.TargetLanguage(parts[0])] = parts[1]
				}
				i++
			}
		}
	}

	fmt.Printf("[✓] Story '%s' initialized.\n", storyName)

	if syncContracts {
		fmt.Println("[+] Synchronizing Quad-IDE cross-language contracts...")
		report, err := defaultStoryMonitor.CheckDrift(schemaDirs, codeDirs)
		if err != nil {
			fmt.Printf("[-] Schema scan notice: %v\n", err)
			return
		}

		printDriftReport(report)
		if report.TotalViolations > 0 {
			_ = defaultStoryMonitor.NotifyIDE(ctx, report)
			fmt.Println("[!] Emitted diagnostic markers to connected JetBrains IDE.")
		} else {
			fmt.Println("[✓] Zero contract drift detected across Quad-IDE models.")
		}
	}
}

func handleStoryCheckContracts(ctx context.Context, args []string) {
	schemaDirs := []string{"schemas", "proto", "api"}
	codeDirs := map[contract.TargetLanguage]string{
		contract.LangGo:         "pkg",
		contract.LangRust:       "crates",
		contract.LangTypeScript: "src",
		contract.LangPython:     "python",
	}
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--schema-dir":
			if i+1 < len(args) {
				schemaDirs = []string{args[i+1]}
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	report, err := defaultStoryMonitor.CheckDrift(schemaDirs, codeDirs)
	if err != nil {
		fmt.Printf("[-] Failed to check contracts: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	printDriftReport(report)
}

func printDriftReport(report *contract.DriftReport) {
	fmt.Printf("=== Quad-IDE Cross-Language Contract Drift Report ===\n")
	fmt.Printf("Scanned Contract Specifications: %d\n", report.ScannedContracts)
	fmt.Printf("Scanned Codebase Models:         %d\n", report.ScannedCodeModels)
	fmt.Printf("Total Drift Violations:          %d\n", report.TotalViolations)
	fmt.Printf("Status:                          ")
	if report.HasErrors {
		fmt.Printf("CRITICAL DRIFT DETECTED\n")
	} else if report.TotalViolations > 0 {
		fmt.Printf("WARNINGS DETECTED\n")
	} else {
		fmt.Printf("PARITY CONFIRMED (CLEAN)\n")
	}

	for i, v := range report.Violations {
		fmt.Printf("\n  [%d] [%s] %s (in %s)\n", i+1, v.Severity, v.Type, v.TargetLang)
		fmt.Printf("      Schema:   %s.%s\n", v.SchemaName, v.FieldName)
		fmt.Printf("      File:     %s:%d\n", v.TargetFile, v.Line)
		fmt.Printf("      Expected: %s\n", v.Expected)
		fmt.Printf("      Actual:   %s\n", v.Actual)
		fmt.Printf("      Message:  %s\n", v.Message)
	}
}

func printStoryUsage() {
	fmt.Println("Usage: dendrite story <start|check-contracts> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  start \"<name>\" [--sync-contracts] Initialize story context with cross-language contract monitoring")
	fmt.Println("  check-contracts [--schema-dir <d>] Audit schema drift across Protobuf, OpenAPI, Go, Rust, TS, and Python")
}
