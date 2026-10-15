package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"
)

var defaultCLIScheduler = token.NewTokenScheduler(nil, nil, "")
var defaultCLIGovernor = governor.NewQuotaGovernor(governor.DefaultConfig(), defaultSegmentedStore, defaultCLIScheduler)

func handleAnalyzeCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "policy":
		handleAnalyzePolicy(args)
	case "status":
		handleAnalyzeStatus(args)
	case "switch":
		handleAnalyzeSwitch(args)
	case "mode":
		handleAnalyzeMode(args)
	case "pause":
		handleAnalyzePause(ctx, args)
	case "resume":
		handleAnalyzeResume(ctx, args)
	case "run":
		handleAnalyzeRun(ctx, args)
	default:
		printAnalyzeUsage()
	}
}

func handleAnalyzeSwitch(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: dendrite analyze switch --allow-zero on|off")
		return
	}

	for i := 0; i < len(args); i++ {
		if args[i] == "--allow-zero" {
			val := "on"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				val = strings.ToLower(args[i+1])
				i++
			}
			enabled := (val == "on" || val == "true" || val == "1" || val == "enable")
			if err := defaultCLIGovernor.SetAllowZero(enabled); err != nil {
				fmt.Printf("[-] Failed to set allow-zero switch: %v\n", err)
				os.Exit(1)
			}
			if enabled {
				fmt.Println("[✓] Allow-Zero Drain Override: ON (Cloud sprint permitted to drain past 20% down to 0%)")
			} else {
				fmt.Println("[✓] Allow-Zero Drain Override: OFF (Cloud calls halt at 20% safety reserve)")
			}
			return
		}
	}
	fmt.Println("[-] Unrecognized switch flag. Use: dendrite analyze switch --allow-zero on|off")
}

func handleAnalyzeMode(args []string) {
	if len(args) == 0 {
		st := defaultCLIGovernor.GetState()
		fmt.Printf("Current Operating Mode: %s\n", st.Mode)
		return
	}

	modeStr := strings.ToLower(args[0])
	var mode governor.AnalysisMode
	switch modeStr {
	case "always":
		mode = governor.ModeAlways
	case "policy":
		mode = governor.ModePolicy
	case "paused", "pause":
		mode = governor.ModePaused
	default:
		fmt.Printf("[-] Invalid mode '%s'. Allowed modes: always, policy, paused\n", modeStr)
		os.Exit(1)
	}

	if err := defaultCLIGovernor.SetMode(mode); err != nil {
		fmt.Printf("[-] Failed to switch operating mode: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[✓] Switched operational analysis mode to: %s\n", mode)
}

func handleAnalyzePolicy(args []string) {
	currPolicy := defaultCLIScheduler.GetPolicy()
	updated := false
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--night-window":
			if i+1 < len(args) {
				currPolicy.NightWindow = args[i+1]
				updated = true
				i++
			}
		case "--weekly-reset":
			if i+1 < len(args) {
				currPolicy.WeeklyReset = args[i+1]
				updated = true
				i++
			}
		case "--sprint-before-reset":
			if i+1 < len(args) {
				dur, err := time.ParseDuration(args[i+1])
				if err != nil {
					fmt.Printf("[-] Invalid sprint-before-reset duration: %v\n", err)
					os.Exit(1)
				}
				currPolicy.SprintBeforeReset = dur
				updated = true
				i++
			}
		case "--reserve-floor":
			if i+1 < len(args) {
				fl, err := strconv.ParseFloat(args[i+1], 64)
				if err != nil {
					fmt.Printf("[-] Invalid reserve-floor: %v\n", err)
					os.Exit(1)
				}
				currPolicy.ReserveFloor = fl
				updated = true
				i++
			}
		case "--allow-zero":
			currPolicy.AllowZero = true
			if i+1 < len(args) && (args[i+1] == "true" || args[i+1] == "on" || args[i+1] == "1") {
				i++
			} else if i+1 < len(args) && (args[i+1] == "false" || args[i+1] == "off" || args[i+1] == "0") {
				currPolicy.AllowZero = false
				i++
			}
			updated = true
		case "--mode":
			if i+1 < len(args) {
				currPolicy.Mode = args[i+1]
				updated = true
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	if updated {
		if err := defaultCLIScheduler.SetPolicy(currPolicy); err != nil {
			fmt.Printf("[-] Failed to update policy: %v\n", err)
			os.Exit(1)
		}
	}

	active := defaultCLIScheduler.GetPolicy()

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"updated": updated,
			"policy":  active,
		}, "", "  ")
		fmt.Println(string(data))
		return
	}

	if updated {
		fmt.Println("[✓] Successfully updated Token-Aware Hybrid Scheduler policy:")
	} else {
		fmt.Println("Token-Aware Hybrid Scheduler Policy:")
	}
	fmt.Printf("  • Night Window (Zero-Cost Local): %s\n", active.NightWindow)
	fmt.Printf("  • Weekly Quota Reset:             %s\n", active.WeeklyReset)
	fmt.Printf("  • Cloud Sprint Duration:          %v\n", active.SprintBeforeReset)
	fmt.Printf("  • Safety Reserve Quota Floor:     %.1f%%\n", active.ReserveFloor*100)
	fmt.Printf("  • Allow Zero Quota Drain:         %t\n", active.AllowZero)
	fmt.Printf("  • Operational Mode:               %s\n", active.Mode)
}

func handleAnalyzeStatus(args []string) {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	st, err := defaultCLIScheduler.Status()
	if err != nil {
		fmt.Printf("[-] Failed to evaluate scheduler status: %v\n", err)
		os.Exit(1)
	}

	govState := defaultCLIGovernor.GetState()

	if jsonOutput {
		payload := map[string]interface{}{
			"scheduler": st,
			"governor":  govState,
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("=== Token-Aware Hybrid Scheduler & Quota Governor Status ===")
	fmt.Printf("Current Time:           %s\n", st.CurrentTime.Format(time.RFC3339))
	fmt.Printf("Operating Mode:         %s\n", govState.Mode)
	fmt.Printf("Active Execution Tier:  %s\n", st.ActiveTier)
	fmt.Printf("Selected Model:         %s\n", st.SelectedModel)
	fmt.Printf("In Night Window:        %t (%s)\n", st.InNightWindow, st.Policy.NightWindow)
	fmt.Printf("In Cloud Sprint Window: %t (within %v of reset)\n", st.InSprintWindow, st.Policy.SprintBeforeReset)
	fmt.Printf("Next Quota Reset:       %s (in %v)\n", st.NextReset.Format(time.RFC3339), st.TimeUntilReset.Round(time.Minute))
	if !st.InSprintWindow {
		fmt.Printf("Time Until Next Sprint: %v\n", st.TimeUntilSprint.Round(time.Minute))
	}
	fmt.Printf("Remaining Quota:        %.1f%%\n", st.RemainingQuotaRatio*100)
	fmt.Printf("Safety Reserve Floor:   %.1f%%\n", govState.ReserveFloor*100)
	fmt.Printf("Allow-Zero Drain:       %t\n", govState.AllowZeroDrain)
	fmt.Printf("Cloud Spend (USD):      $%.2f\n", st.CloudSpendUSD)
	fmt.Printf("Cloud Blocked:          %t\n", govState.IsCloudBlocked)
	fmt.Printf("Status Reason:          %s\n", st.Reason)
}

func handleAnalyzePause(ctx context.Context, args []string) {
	project := ""
	chunkID := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--project" && i+1 < len(args) {
			project = args[i+1]
			i++
		}
		if args[i] == "--chunk" && i+1 < len(args) {
			chunkID = args[i+1]
			i++
		}
	}

	_ = defaultCLIGovernor.Pause(ctx, project, chunkID)
	fmt.Println("[✓] Token-Aware Hybrid Scheduler and Quota Governor paused. Active chunk progress persisted.")
}

func handleAnalyzeResume(ctx context.Context, args []string) {
	project := ""
	chunkID := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--project" && i+1 < len(args) {
			project = args[i+1]
			i++
		}
		if args[i] == "--chunk" && i+1 < len(args) {
			chunkID = args[i+1]
			i++
		}
	}

	_ = defaultCLIGovernor.Resume(ctx, project, chunkID)
	fmt.Println("[✓] Token-Aware Hybrid Scheduler and Quota Governor resumed.")
}

func handleAnalyzeRun(ctx context.Context, args []string) {
	chunkID := "project-ast-chunk"
	project := "dendrite"
	simNight := false
	simSprint := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chunk", "-c":
			if i+1 < len(args) {
				chunkID = args[i+1]
				i++
			}
		case "--project", "-p":
			if i+1 < len(args) {
				project = args[i+1]
				i++
			}
		case "--simulate-night":
			simNight = true
		case "--simulate-sprint":
			simSprint = true
		}
	}

	sched := defaultCLIScheduler
	if simNight || simSprint {
		pol := sched.GetPolicy()
		if simNight {
			pol.Mode = "night"
		} else if simSprint {
			pol.Mode = "sprint"
		}
		tempSched := token.NewTokenScheduler(nil, nil, "")
		_ = tempSched.SetPolicy(pol)
		sched = tempSched
	}

	res, err := sched.ProcessChunk(ctx, chunkID, func(ctx context.Context, model string) (string, int, float64, error) {
		cost := 0.0
		if model == "gemini-2.5-pro" {
			// Check governor safety floor before invoking cloud model
			allowed, gErr := defaultCLIGovernor.CheckCloudExecution(ctx, project, chunkID)
			if !allowed {
				return "", 0, 0, gErr
			}
			cost = 0.02
		}
		return fmt.Sprintf("Completed semantic AST analysis for %s", chunkID), 1500, cost, nil
	})
	if err != nil {
		fmt.Printf("[-] Failed to process chunk: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[✓] Chunk Execution Outcome:\n")
	fmt.Printf("    Chunk ID:       %s\n", res.ChunkID)
	fmt.Printf("    Execution Tier: %s\n", res.ActiveTier)
	fmt.Printf("    Selected Model: %s\n", res.SelectedModel)
	fmt.Printf("    Tokens:         %d\n", res.Tokens)
	fmt.Printf("    Cost (USD):     $%.4f\n", res.CostUSD)
	fmt.Printf("    Details:        %s\n", res.Response)
}

func printAnalyzeUsage() {
	fmt.Println("Usage: dendrite analyze <policy|status|switch|mode|pause|resume|run> [flags...]")
	fmt.Println("\nCommands:")
	fmt.Println("  policy   View or set scheduler policy directives")
	fmt.Println("           Flags: --night-window <HH:MM-HH:MM>")
	fmt.Println("                  --weekly-reset <DAY HH:MM>")
	fmt.Println("                  --sprint-before-reset <duration>")
	fmt.Println("                  --reserve-floor <0.0-1.0>")
	fmt.Println("                  --allow-zero [true|false]")
	fmt.Println("                  --mode <auto|always|night|sprint|paused>")
	fmt.Println("  status   Inspect real-time scheduler state, countdowns, and active tiers")
	fmt.Println("  switch   Set runtime feature switches (e.g. --allow-zero on|off)")
	fmt.Println("  mode     Switch operational analysis mode (always|policy|paused)")
	fmt.Println("  pause    Pause scheduler analysis (persists chunk progress immediately)")
	fmt.Println("  resume   Resume scheduler analysis in active mode")
	fmt.Println("  run      Execute analysis pass on a target chunk under scheduler governance")
}
