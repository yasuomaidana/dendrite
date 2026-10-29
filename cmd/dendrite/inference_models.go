package main

import (
	"context"
	"fmt"
	"io"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/memfit"

	"github.com/spf13/cobra"
)

func newSpikeModelsCmd() *cobra.Command {
	var ollamaURL, oxiURL string
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List the local models installed on Ollama and OxiLLaMa and whether each fits this machine's memory",
		Long: "Lists what is actually installed (not the static defaults of `spike list`), the memory budget local models may\n" +
			"hold (50% of RAM; set DENDRITE_MODEL_MEM_BUDGET_GB to change it) and, for each model, whether it can run without\n" +
			"starving the machine. Models marked too large get no gateway route and are refused at call time.",
		Example: "dendrite spike models\ndendrite spike models --json",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpikeModels(cmd.Context(), cmd.OutOrStdout(), ollamaURL, oxiURL)
		},
	}
	cmd.Flags().StringVar(&ollamaURL, "ollama", "", "Ollama endpoint (default "+memfit.DefaultOllamaEndpoint+")")
	cmd.Flags().StringVar(&oxiURL, "oxillama", "", "OxiLLaMa endpoint (default "+memfit.DefaultOxiLLaMaEndpoint+")")
	return cmd
}

func runSpikeModels(ctx context.Context, out io.Writer, ollamaURL, oxiURL string) error {
	r := memfit.Assess(ctx, ollamaURL, oxiURL)
	if globals.JSON {
		return writeJSON(out, r)
	}
	fmt.Fprintf(out, "[+] Memory: %s RAM, %s budget for local models, %s held by loaded models\n",
		memfit.GB(r.TotalRAMBytes), memfit.GB(r.BudgetBytes), memfit.GB(r.ResidentBytes))
	if !r.OllamaReachable {
		fmt.Fprintln(out, "[-] Ollama is not reachable; start it to see its models")
	}
	if !r.OxiReachable {
		fmt.Fprintln(out, "[-] OxiLLaMa is not reachable; start it to see its model")
	}
	if len(r.Models) == 0 {
		fmt.Fprintln(out, "    (no local models found)")
		return nil
	}
	fmt.Fprintf(out, "    %-9s %-28s %9s  %-8s %s\n", "ENGINE", "MODEL", "SIZE", "STATE", "FIT")
	for _, m := range r.Models {
		state := "on disk"
		if m.Resident {
			state = "loaded"
		}
		size := "?"
		if m.SizeBytes > 0 {
			size = memfit.GB(uint64(m.SizeBytes))
		}
		fit := "ok"
		switch {
		case !m.Fits:
			fit = "TOO LARGE: " + m.Reason
		case m.Reason != "":
			fit = "ok (" + m.Reason + ")"
		}
		fmt.Fprintf(out, "    %-9s %-28s %9s  %-8s %s\n", m.Engine, m.Name, size, state, fit)
	}
	if best, err := inference.BestLocalModel(ctx, ollamaURL, oxiURL); err == nil {
		start := ""
		if best.NeedsStart {
			start = ", started on demand and stopped when idle"
		}
		fmt.Fprintf(out, "[+] Best available now: %s on %s (score %d%s)\n", best.Model, best.Engine, best.Score, start)
	} else {
		fmt.Fprintf(out, "[-] %v\n", err)
	}
	fmt.Fprintln(out, "    Run `dendrite cleft update` to route the gateway to these models.")
	return nil
}
