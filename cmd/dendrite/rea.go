package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/rea/showcase"
)

func handleREACommand(ctx context.Context, action string, args []string) {
	switch strings.ToLower(action) {
	case "showcase":
		runREAShowcase(ctx, args)
	case "list", "targets":
		listREATargets()
	default:
		printREAUsage()
	}
}

func printREAUsage() {
	fmt.Println("Usage: dendrite rea <showcase|list> [target] [flags...]")
	fmt.Println("\nSubcommands:")
	fmt.Println("  showcase [target]          Execute automated REA cleanroom analysis & parity verification")
	fmt.Println("  list                       List registered open-source showcase targets and metadata")
	fmt.Println("\nTargets:")
	fmt.Println("  all                        Run all 5 showcase targets (default)")
	fmt.Println("  aegis                      Aegis Authenticator TOTP RFC 6238 cleanroom generator")
	fmt.Println("  dxball                     DX-Ball 1996 position-to-pan helper & 63-byte parity")
	fmt.Println("  ctf                        DownUnderCTF ELF arithmetic SMT constraint solver")
	fmt.Println("  th04                       TH04 PC-98 16-bit BAM fixed-point bullet aiming")
	fmt.Println("  mattermost                 Mattermost Desktop Electron preload bridge & clipboard sanitizer")
	fmt.Println("\nFlags for 'showcase':")
	fmt.Println("  --format <text|json|markdown> Output format (default: text)")
	fmt.Println("  --output <file>            Write output to specified file path")
}

func listREATargets() {
	targets := showcase.ListTargets()
	fmt.Printf("[+] Registered Open-Source REA Showcase Targets (%d):\n\n", len(targets))
	fmt.Printf("    %-12s | %-32s | %-12s | %-8s | %s\n", "ID", "Name", "License (SPDX)", "Format", "Upstream Repository")
	fmt.Println("    " + strings.Repeat("-", 100))
	for _, t := range targets {
		fmt.Printf("    %-12s | %-32s | %-12s | %-8s | %s\n", t.ID, t.Name, t.LicenseSPDX, t.BinaryFormat, t.UpstreamRepo)
		fmt.Printf("      -> Capability: %s\n", t.ExtractedCapability)
		fmt.Printf("      -> Oracle:     %s\n\n", t.GroundTruthOracle)
	}
}

func runREAShowcase(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("rea showcase", flag.ContinueOnError)
	format := fs.String("format", "text", "Output format: text, json, or markdown")
	output := fs.String("output", "", "Optional output file path")

	// Parse flags and extract positional target
	target := "all"
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if (arg == "--format" || arg == "-format" || arg == "--output" || arg == "-output") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if target == "all" && arg != "all" {
			target = arg
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return
	}

	fmt.Printf("[*] Launching Open-Source REA Showcase Verification for '%s'...\n", target)
	report, err := showcase.RunShowcase(ctx, target)
	if err != nil {
		fmt.Printf("[-] Showcase run failed: %v\n", err)
		return
	}

	var outputContent string
	switch strings.ToLower(*format) {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("[-] Failed to serialize JSON report: %v\n", err)
			return
		}
		outputContent = string(data)

	case "markdown", "md":
		outputContent = showcase.RenderMarkdownScorecard(report)

	default: // "text"
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("\n================================================================================\n"))
		sb.WriteString(fmt.Sprintf("  DENDRITE REA CLEANROOM VERIFICATION SCORECARD: %s\n", report.OverallStatus))
		sb.WriteString(fmt.Sprintf("================================================================================\n"))
		sb.WriteString(fmt.Sprintf("  Timestamp:             %s\n", report.Timestamp))
		sb.WriteString(fmt.Sprintf("  Targets Evaluated:     %d (Passed: %d, Failed: %d)\n", report.TotalTargets, report.SucceededTargets, report.FailedTargets))
		sb.WriteString(fmt.Sprintf("  Aggregate Parity:      %.2f%%\n\n", report.OverallParityPercent))

		for _, r := range report.Results {
			icon := "[+]"
			if !r.PassedValidation {
				icon = "[-]"
			}
			sb.WriteString(fmt.Sprintf("  %s %s (%s)\n", icon, r.Metadata.Name, r.Target))
			sb.WriteString(fmt.Sprintf("      Upstream:          %s (%s)\n", r.Metadata.UpstreamRepo, r.Metadata.LicenseSPDX))
			sb.WriteString(fmt.Sprintf("      Capability:        %s\n", r.Metadata.ExtractedCapability))
			sb.WriteString(fmt.Sprintf("      Vectors Tested:    %d/%d passed (%.2f%% parity)\n", r.Parity.PassedVectors, r.Parity.TotalVectors, r.Parity.ParityPercent))
			sb.WriteString(fmt.Sprintf("      SPDX Audit:        Compliant: %t (OSI: %t)\n", r.SPDX.Compliant, r.SPDX.OSIApproved))
			if r.Evidence != nil {
				sb.WriteString(fmt.Sprintf("      Evidence Bundle:   %d tokens (Truncated: %t)\n", r.Evidence.TokenEstimate, r.Evidence.Truncated))
			}
			sb.WriteString(fmt.Sprintf("      Execution Time:    %d ms\n", r.ExecutionTimeMs))
			sb.WriteString(fmt.Sprintf("      ADR Document:      %s: %s\n\n", r.ADR.ID, r.ADR.Title))
		}
		outputContent = sb.String()
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(outputContent), 0644); err != nil {
			fmt.Printf("[-] Failed to write report to %s: %v\n", *output, err)
		} else {
			fmt.Printf("[+] Scorecard written to %s\n", *output)
		}
	} else {
		fmt.Println(outputContent)
	}
}
