package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/agent/triage"
	"Dendrite/pkg/ollama"

	"github.com/spf13/cobra"
)

var triageTiers = map[string]triage.ExecutionTier{
	"local":  triage.TierLocal,
	"hybrid": triage.TierHybrid,
	"cloud":  triage.TierCloud,
}

var triageEngines = map[string]triage.SupportedEngine{
	"ollama":           triage.EngineOllama,
	"ollama-validated": triage.EngineOllamaValidated,
	"huggingface":      triage.EngineHuggingFace,
	"hf":               triage.EngineHuggingFace,
	"candle":           triage.EngineCandle,
	"oxillama":         triage.EngineOxiLLaMa,
}

type triageOptions struct {
	File   string
	Tier   string
	Engine string
	Local  bool
	RAG    bool
}

// newTriageCmd builds `dendrite inhibitory-triage` (alias triage): route a prompt to a local, hybrid or cloud tier.
func newTriageCmd() *cobra.Command {
	var o triageOptions
	cmd := &cobra.Command{
		Use:     "inhibitory-triage <prompt...>",
		Aliases: []string{"triage", "trg"},
		Short:   "Resolve local agent task triage and priority routing",
		GroupID: "effectors",
		Args:    minArgs(1),
		Example: "dendrite inhibitory-triage \"rename this variable\" --file main.go\n" +
			"dendrite triage \"design a sharded cache\" --tier cloud\n" +
			"dendrite triage \"summarize the docs\" --rag --local",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := triage.TriageRequest{
				Prompt:       strings.Join(args, " "),
				ActiveFile:   o.File,
				ExecuteLocal: o.Local,
				EnableRAG:    o.RAG,
				WorkspaceDir: ".",
			}
			if o.Tier != "" {
				t, ok := triageTiers[strings.ToLower(o.Tier)]
				if !ok {
					return usageErrorf("invalid --tier %q: use local, hybrid or cloud", o.Tier)
				}
				req.ForceTier = t
			}
			if o.Engine != "" {
				e, ok := triageEngines[strings.ToLower(o.Engine)]
				if !ok {
					return usageErrorf("invalid --engine %q: use ollama, ollama-validated, huggingface, candle or oxillama", o.Engine)
				}
				req.TargetEngine = e
			}
			if o.File != "" {
				data, err := os.ReadFile(o.File)
				if err != nil {
					return fmt.Errorf("read %s: %w", o.File, err)
				}
				req.FileContent = string(data)
			}
			return runTriage(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &o.File, "", "Active file whose content is triaged with the prompt")
	cmd.Flags().StringVar(&o.Tier, "tier", "", "Force a tier: local, hybrid or cloud")
	cmd.Flags().StringVarP(&o.Engine, "engine", "e", "", "Local engine: ollama, ollama-validated, huggingface, candle or oxillama")
	cmd.Flags().BoolVarP(&o.Local, "local", "l", false, "Execute on the local engine when the tier allows it")
	cmd.Flags().BoolVarP(&o.RAG, "rag", "r", false, "Retrieve workspace context (RAG)")
	return cmd
}

func runTriage(ctx context.Context, req triage.TriageRequest, out io.Writer) error {
	res, err := triage.NewEngine(ollama.NewClient()).Triage(ctx, req)
	if err != nil {
		return fmt.Errorf("triage: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}
	fmt.Fprintln(out, "[+] Local Agent Triage Decision:")
	fmt.Fprintf(out, "    - Assigned Tier:      %s\n", strings.ToUpper(string(res.Tier)))
	fmt.Fprintf(out, "    - Assigned Model:     %s\n", res.AssignedModel)
	fmt.Fprintf(out, "    - Assigned Engine:    %s (%s)\n", res.AssignedEngine, res.LocalEngineName)
	fmt.Fprintf(out, "    - Local Compute:      %v\n", res.LocalComputeAvailable)
	fmt.Fprintf(out, "    - Decision Reason:    %s\n", res.Reason)
	if len(res.SelectedSkills) > 0 {
		fmt.Fprintf(out, "    - Matched Skills:     %s\n", strings.Join(res.SelectedSkills, ", "))
	}
	if len(res.RetrievedRAGChunks) > 0 {
		fmt.Fprintf(out, "    - RAG Context:        %d chunks retrieved (%d tokens)\n", len(res.RetrievedRAGChunks), res.RAGTokens)
		for i, c := range res.RetrievedRAGChunks {
			heading := ""
			if c.Heading != "" {
				heading = fmt.Sprintf(" (%s)", c.Heading)
			}
			fmt.Fprintf(out, "      * [%d] %s:%d-%d%s\n", i+1, c.FilePath, c.LineStart, c.LineEnd, heading)
		}
	}
	fmt.Fprintf(out, "    - Original Tokens:    %d\n", res.OriginalTokens)
	fmt.Fprintf(out, "    - Compacted Tokens:   %d\n", res.CompactedTokens)
	fmt.Fprintf(out, "    - Token Savings:      %.1f%%\n", res.TokenSavingsPct)
	fmt.Fprintf(out, "    - Estimated Cost:     $%.6f (saved $%.6f)\n", res.EstimatedCostUSD, res.CostWithoutTriageUSD-res.EstimatedCostUSD)
	if res.LocalExecutionResult != "" {
		fmt.Fprintf(out, "    - Local Execution:    %s\n", res.LocalExecutionResult)
	}
	if res.IsDeclarativeOrchestrator && res.DeclarativePlan != nil {
		fmt.Fprintln(out, "    - Orchestrator Mode:  Declarative Cloud Orchestrator -> Local Worker")
		fmt.Fprintf(out, "    - Orchestrator Model: %s\n", res.OrchestratorModel)
		fmt.Fprintf(out, "    - Local Worker Model: %s (%s)\n", res.WorkerModel, res.WorkerEngine)
		fmt.Fprintf(out, "    - Output Savings:     %.1f%% (%d plan tokens vs %d direct tokens)\n",
			res.DeclarativePlan.OutputTokenSavingsPct,
			res.DeclarativePlan.EstimatedOrchestratorTokens,
			res.DeclarativePlan.EstimatedDirectOutputTokens)
		fmt.Fprintf(out, "    - Declarative Steps:  %d instructions, %d skill constraints\n",
			len(res.DeclarativePlan.Instructions), len(res.DeclarativePlan.Constraints))
		if res.LocalWorkerExecutionResult != "" {
			fmt.Fprintf(out, "    - Local Worker Run:   %s\n", res.LocalWorkerExecutionResult)
		}
	}
	fmt.Fprintf(out, "    - Triage Latency:     %v\n", res.ExecutionDuration)
	return nil
}
