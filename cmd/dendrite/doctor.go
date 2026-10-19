package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/ollama"
)

// doctorOllamaURL is the Ollama endpoint probed by `soma diagnose`.
var doctorOllamaURL = ollama.DefaultBaseURL

// runDoctor runs pre-flight diagnostics to os.Stdout.
func runDoctor(ctx context.Context) {
	if err := runDiagnose(ctx, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
	}
}

// runDiagnose runs environment pre-flight diagnostics and returns an error when Ollama is unreachable.
func runDiagnose(ctx context.Context, out io.Writer) error {
	fmt.Fprintln(out, "[+] Running Dendrite Doctor diagnostics...")

	status, models, probeErr := devtool.ProbeOllama(ctx, doctorOllamaURL)
	if probeErr != nil {
		fmt.Fprintf(out, "  [-] Local Ollama: UNREACHABLE (%v)\n", probeErr)
	} else {
		fmt.Fprintf(out, "  [✓] Local Ollama: REACHABLE (latency: %v, loaded models: %d)\n", status.Latency, len(models))
		for _, m := range models {
			fmt.Fprintf(out, "      - %s (size: %.2f GB)\n", m.Name, float64(m.Size)/(1024*1024*1024))
		}
	}

	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Fprintln(out, "  [✓] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Fprintln(out, "  [-] Project Status: DEV_STATUS.md not found in current directory")
	}

	fmt.Fprintln(out, "[+] Pre-flight diagnostics complete.")
	if probeErr != nil {
		return fmt.Errorf("local ollama unreachable: %w", probeErr)
	}
	return nil
}

