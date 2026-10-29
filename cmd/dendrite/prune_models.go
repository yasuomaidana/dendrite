package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/inference/prune"
)

func newSpikePruneCmd() *cobra.Command {
	var keep []string
	var ollamaURL string
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "List the installed Ollama models nothing uses and remove them to reclaim disk",
		Long: "A model is in use when it is a tier model (dendrite spike bench --save), the fallback, named in --keep,\n" +
			"an override in the environment, or routed in the gateway's litellm_config.yaml. Embedding models are never\n" +
			"removed. Every removal prints the command that restores the model; without -y the command lists and asks.",
		Example: "dendrite spike prune\ndendrite spike prune --keep gpt-oss:20b -y",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpikePrune(cmd.Context(), ollamaURL, keep, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&keep, "keep", nil, "Models to keep besides the ones in use (comma-separated)")
	cmd.Flags().StringVar(&ollamaURL, "ollama", "http://localhost:11434", "Ollama base URL")
	return cmd
}

// modelsInUse are the models the machine's configuration points at.
func modelsInUse(extra []string) []string {
	use := append([]string{inference.FallbackModel, inference.ChatModel(inference.TierAsk), inference.ChatModel(inference.TierChore)}, extra...)
	for _, env := range []string{inference.ChatModelEnv, inference.ChatModelEnv + "_ASK", inference.ChatModelEnv + "_CHORE"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			use = append(use, v)
		}
	}
	if cfg, err := os.ReadFile(gateway.DefaultConfigPath()); err == nil {
		use = append(use, prune.ModelsInConfig(string(cfg))...)
	}
	return use
}

func runSpikePrune(ctx context.Context, ollamaURL string, keep []string, in io.Reader, out io.Writer) error {
	hc := &http.Client{Timeout: 5 * time.Minute}
	installed, err := prune.Installed(ctx, hc, ollamaURL)
	if err != nil {
		return err
	}
	unused := prune.Unused(installed, modelsInUse(keep))
	if len(unused) == 0 {
		fmt.Fprintf(out, "[✓] All %d installed models are in use; nothing to remove.\n", len(installed))
		return nil
	}
	var total int64
	for _, m := range unused {
		total += m.SizeBytes
		fmt.Fprintf(out, "    - %-34s %8s   restore: %s\n", m.Name, prune.GB(m.SizeBytes), prune.RestoreNote(m.Name))
	}
	fmt.Fprintf(out, "[i] %d unused models, %s to reclaim.\n", len(unused), prune.GB(total))
	if !globals.Yes {
		if !isTerminal(in) {
			return usageErrorf("this removes models: run it in a terminal to confirm, or pass -y")
		}
		fmt.Fprint(out, "[?] Remove them? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "[i] Left as it was.")
			return nil
		}
	}
	var freed int64
	for _, m := range unused {
		if err := prune.Remove(ctx, hc, ollamaURL, m.Name); err != nil {
			fmt.Fprintf(out, "[-] %s: %v\n", m.Name, err)
			continue
		}
		freed += m.SizeBytes
		fmt.Fprintf(out, "[✓] removed %s (%s)\n", m.Name, prune.GB(m.SizeBytes))
	}
	fmt.Fprintf(out, "[✓] Reclaimed %s.\n", prune.GB(freed))
	return nil
}
