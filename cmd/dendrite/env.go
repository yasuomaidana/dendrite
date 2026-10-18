package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/env"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/workspace"
	"encoding/json"
	"flag"
)

func handleEnvCommand(ctx context.Context, action string, args []string) {
	detector := env.NewDetector(nil)
	host, err := detector.Detect(ctx)
	if err != nil {
		fmt.Printf("[-] Host detection failed: %v\n", err)
		return
	}

	planner := env.NewPlanner(*host)
	replicator := env.NewReplicator(nil, *host)

	switch action {
	case "prepare", "sync", "update":
		handleEnvPrepareCommand(ctx, args)
		return
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "doctor":
		fmt.Println("[+] Host Environment Discovery:")
	case "plan":
		mode := env.ModeContainer
		withLinear := false
		withPlane := false
		var withPlaneUI *bool
		withPenpot := false
		withGrafana := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--with-linear" {
				withLinear = true
			} else if args[i] == "--with-plane" {
				withPlane = true
			} else if args[i] == "--with-plane-ui" {
				v := true
				withPlaneUI = &v
			} else if args[i] == "--no-plane-ui" || args[i] == "--without-plane-ui" {
				v := false
				withPlaneUI = &v
			} else if args[i] == "--with-penpot" {
				withPenpot = true
			} else if args[i] == "--with-grafana" {
				withGrafana = true
			}
		}
		req := env.ReplicationRequest{
			Mode:        mode,
			WithLinear:  withLinear,
			WithPlane:   withPlane,
			WithPlaneUI: withPlaneUI,
			WithPenpot:  withPenpot,
			WithGrafana: withGrafana,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {
			fmt.Printf("[-] Failed to plan replication: %v\n", err)
			return
		}
		fmt.Printf("[+] Replication Plan (Mode: %s, Est. Time: ~%ds):\n", plan.Mode, plan.EstimatedSecs)
		for i, a := range plan.Actions {
			mandatory := ""
			if a.Mandatory {
				mandatory = "[mandatory]"
			}
			fmt.Printf("    %d. [%s] %s %s\n", i+1, a.Type, a.Description, mandatory)
			fmt.Printf("       Command: %s\n", a.Command)
		}

	case "replicate":
		mode := env.ModeContainer
		withLinear := true
		withPlane := false
		var withPlaneUI *bool
		dryRun := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--no-linear" {
				withLinear = false
			} else if args[i] == "--with-plane" {
				withPlane = true
			} else if args[i] == "--no-plane" {
				withPlane = false
			} else if args[i] == "--with-plane-ui" {
				v := true
				withPlaneUI = &v
			} else if args[i] == "--no-plane-ui" || args[i] == "--without-plane-ui" {
				v := false
				withPlaneUI = &v
			} else if args[i] == "--dry-run" {
				dryRun = true
			}
		}
		req := env.ReplicationRequest{
			Mode:        mode,
			WithLinear:  withLinear,
			WithPlane:   withPlane,
			WithPlaneUI: withPlaneUI,
			DryRun:      dryRun,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {
			fmt.Printf("[-] Failed to plan replication: %v\n", err)
			return
		}
		fmt.Printf("[+] Executing Environment Replication (Mode: %s, DryRun: %v)...\n", mode, dryRun)
		res, err := replicator.ExecutePlan(ctx, plan, dryRun)
		if err != nil {
			fmt.Printf("[-] Replication encountered error: %v\n", err)
		}
		fmt.Printf("[✓] Completed in %v (Success: %v)\n", res.TotalDuration, res.Success)
		if len(res.GeneratedFiles) > 0 {
			fmt.Println("[+] Generated Compose Files:")
			for _, f := range res.GeneratedFiles {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println("[+] Component Status:")
		for _, c := range res.Components {
			re := "UNREACHABLE"
			if c.Reachable {
				re = "REACHABLE"
			}
			fmt.Printf("    - %-12s: %s (Endpoint: %s)\n", c.Target, re, c.Endpoint)
		}

	case "install":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite env install <aider|openhands|copilot|agy|linear|plane|penpot|ollama|litellm|postgres|docker> [--mode container|host] [-y|--yes]")
			return
		}
		targetName := strings.ToLower(args[0])
		aType := agent.AgentType(targetName)
		if aType.IsValid() {
			autoApprove := false
			for _, a := range args[1:] {
				if a == "-y" || a == "--yes" {
					autoApprove = true
				}
			}
			err := agent.PromptAndInstallAgent(ctx, aType, autoApprove, os.Stdin, os.Stdout)
			if err != nil {
				fmt.Printf("[-] Failed to install %s: %v\n", aType, err)
				return
			}
			return
		}
		target := env.ReplicationTarget(args[0])
		mode := env.ModeContainer
		for i := 1; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			}
		}
		req := env.ReplicationRequest{
			Mode:       mode,
			Targets:    []env.ReplicationTarget{target},
			WithLinear: target == env.TargetLinear,
			WithPlane:  target == env.TargetPlane,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {

			fmt.Printf("[-] Failed to plan target installation: %v\n", err)
			return
		}
		res, err := replicator.ExecutePlan(ctx, plan, false)
		if err != nil {
			fmt.Printf("[-] Installation failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Successfully installed %s (Mode: %s) in %v\n", target, mode, res.TotalDuration)

	case "status":
		statuses := replicator.AuditComponents(ctx)
		fmt.Println("[+] Real-Time Environment Component Status:")
		for _, s := range statuses {
			reach := "OFFLINE / UNREACHABLE"
			if s.Reachable {
				reach = "ONLINE / HEALTHY"
			}
			fmt.Printf("    [%s] %-12s - %s (Endpoint: %s)\n", reach, s.Target, s.Details, s.Endpoint)
		}

	default:
		fmt.Printf("[-] Unknown env action: %s. Use doctor, plan, replicate, install, or status.\n", action)
	}
}

func handleEnvPrepareCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("env-prepare", flag.ContinueOnError)
	tgtPath := fs.String("path", ".", "Target project directory path")
	tool := fs.String("tool", "copilot", "Default developer and CI/CD assistant tool")
	noCICD := fs.Bool("no-cicd", false, "Disable CI/CD and Copilot enforcement")
	noSkills := fs.Bool("no-skills", false, "Disable agent rules and skills synchronization")
	dryRun := fs.Bool("dry-run", false, "Preview environment preparation without file mutations")
	asJSON := fs.Bool("json", false, "Output preparation report in JSON format")

	if err := fs.Parse(args); err != nil {
		return
	}

	pathVal := *tgtPath
	if pathVal == "." && len(fs.Args()) > 0 {
		pathVal = fs.Args()[0]
	}
	if reg, err := workspace.NewRegistry("", ""); err == nil {
		if p, err := reg.GetProject(pathVal); err == nil && p != nil {
			pathVal = p.Path
		}
	}

	opts := bootstrap.PrepareOptions{
		ProjectPath: pathVal,
		DefaultTool: *tool,
		EnforceCICD: !*noCICD,
		SyncSkills:  !*noSkills,
		DryRun:      *dryRun,
	}

	prep := bootstrap.NewPreparer()
	report, err := prep.Prepare(ctx, opts)
	if err != nil {
		fmt.Printf("[-] Failed to prepare project environment: %v\n", err)
		return
	}

	if *asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[+] Project Environment Preparation & CI/CD Enforcement:")
	fmt.Printf("    - Project Path:       %s\n", report.ProjectPath)
	fmt.Printf("    - Primary Stack:      %s\n", strings.ToUpper(string(report.Stack.PrimaryStack)))
	if len(report.Stack.SecondaryStacks) > 0 {
		var sec []string
		for _, s := range report.Stack.SecondaryStacks {
			sec = append(sec, strings.ToUpper(string(s)))
		}
		fmt.Printf("    - Secondary Stacks:   %s\n", strings.Join(sec, ", "))
	}
	fmt.Printf("    - Default Tool:       %s (Installed: %v)\n", report.CICD.DefaultTool, report.CICD.ToolInstalled)
	if !report.CICD.ToolInstalled && report.CICD.ToolInstallCommand != "" {
		fmt.Printf("      * Tooling Guide:    %s\n", report.CICD.ToolInstallCommand)
	}
	fmt.Printf("    - Coverage Gate:      >= %.1f%%\n", report.CICD.CoverageThreshold)
	fmt.Printf("    - Copilot Rules:      %s\n", report.CICD.CopilotInstructions)
	fmt.Printf("    - CI/CD Workflow:     %s\n", report.CICD.CIWorkflowPath)
	if len(report.FilesCreated) > 0 {
		fmt.Printf("    - Files Created (%d): %s\n", len(report.FilesCreated), strings.Join(report.FilesCreated, ", "))
	}
	if len(report.FilesUpdated) > 0 {
		fmt.Printf("    - Files Updated (%d): %s\n", len(report.FilesUpdated), strings.Join(report.FilesUpdated, ", "))
	}
	if len(report.SkillsSynced) > 0 {
		fmt.Printf("    - Skills Synced (%d): %s\n", len(report.SkillsSynced), strings.Join(report.SkillsSynced, ", "))
	}
	fmt.Printf("    - Execution Duration: %v\n", report.Duration)
}
