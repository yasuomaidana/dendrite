package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"Dendrite/pkg/tokens"
)

func handleTokensCommand(ctx context.Context, args []string) {
	cfg := tokens.DefaultConfig()
	jsonOutput := false
	probeAGY := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "status":
			// optional action verb: dendrite tokens status
		case "--json":
			jsonOutput = true
		case "--probe":
			probeAGY = true
		case "--model", "-m":
			if i+1 < len(args) {
				cfg.Model = args[i+1]
				i++
			}
		case "--day-tokens":
			if i+1 < len(args) {
				if n, err := strconv.ParseInt(args[i+1], 10, 64); err == nil {
					cfg.DayCapacityTokens = n
				}
				i++
			}
		case "--week-tokens":
			if i+1 < len(args) {
				if n, err := strconv.ParseInt(args[i+1], 10, 64); err == nil {
					cfg.WeekCapacityTokens = n
				}
				i++
			}
		case "--spend-file":
			if i+1 < len(args) {
				cfg.SpendFile = args[i+1]
				i++
			}
		case "--policy-file":
			if i+1 < len(args) {
				cfg.PolicyFile = args[i+1]
				i++
			}
		}
	}

	eval := tokens.NewEvaluator(nil, nil)
	report, err := eval.Evaluate(ctx, cfg, probeAGY)
	if err != nil {
		fmt.Printf("[-] Failed to evaluate token status: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Print(tokens.FormatReport(report))
}
