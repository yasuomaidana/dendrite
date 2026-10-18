package main

import (
	"context"
	"fmt"
	"os"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/ollama"
)

func runDoctor(ctx context.Context) {
	fmt.Println("[+] Running Dendrite Doctor diagnostics...")

	status, models, err := devtool.ProbeOllama(ctx, ollama.DefaultBaseURL)
	if err != nil {
		fmt.Printf("  [-] Local Ollama: UNREACHABLE (%v)\n", err)
	} else {
		fmt.Printf("  [✓] Local Ollama: REACHABLE (latency: %v, loaded models: %d)\n", status.Latency, len(models))
		for _, m := range models {
			fmt.Printf("      - %s (size: %.2f GB)\n", m.Name, float64(m.Size)/(1024*1024*1024))
		}
	}

	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Println("  [✓] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Println("  [-] Project Status: DEV_STATUS.md not found in current directory")
	}

	fmt.Println("[+] Pre-flight diagnostics complete.")
}
