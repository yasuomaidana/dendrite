package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"Dendrite/pkg/rag"

	"github.com/spf13/cobra"
)

type askOptions struct {
	Paid        bool
	Hybrid      bool
	UpdateIndex bool
	Server      string
	Tier        string
	Project     string
	PaidModel   string
	LocalModel  string
	TopK        int
	Docs        bool
	Corpus      string
	DocsShare   float64
}

// newAskCmd builds `dendrite ask`: questions about a project answered from its RAG index.
func newAskCmd() *cobra.Command {
	var o askOptions
	cmd := &cobra.Command{
		Use:     "ask <question...>",
		Aliases: []string{"query-project", "rag-ask", "q"},
		Short:   "Ask questions to Dendrite about a project with local RAG and interactive paid fallback",
		Long: "Answers from the project's RAG index with a zero-cost local model first. When the local model\n" +
			"fails it asks before escalating to the paid model; --yes escalates without asking.\n\n" +
			"The context comes from the project's code; -d/--docs adds its documentation (Markdown, txt, rst)\n" +
			"for how-to and setup questions, and --corpus docs reads only the documentation (design and\n" +
			"conceptual questions). With both, --docs-share sets how much of the context the documentation\n" +
			"keeps (default: [ask] docs_share in .dendrite/rag.toml or DENDRITE_RAG_ASK_DOCS_SHARE, e.g. in\n" +
			"$DENDRITE_HOME/rag.env, else 0.2).",
		GroupID: "cortex",
		Args:    minArgs(1),
		Example: "dendrite ask \"how do I check the available LLMs?\"\n" +
			"dendrite ask --hybrid \"how does the token gateway work?\"\n" +
			"dendrite ask --server http://100.84.12.10:3456 \"where is telemetry defined?\"\n" +
			"dendrite ask -d --update-index --json \"what does soma prepare write?\"\n" +
			"dendrite ask -d \"how do I check the available LLMs?\"\n" +
			"dendrite ask --corpus docs \"what is the design of the session handoff?\"\n" +
			"dendrite ask -d --docs-share 0.5 \"how do agents start a session?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(o.Tier) {
			case "":
			case "paid", "cloud":
				o.Paid = true
			case "hybrid":
				o.Hybrid = true
			case "local":
			default:
				return usageErrorf("invalid --tier %q: use local, hybrid or paid", o.Tier)
			}
			if o.TopK < 0 {
				return usageErrorf("--top-k must not be negative")
			}
			corpus := strings.ToLower(o.Corpus)
			switch corpus {
			case "", "code", "all", "docs":
			default:
				return usageErrorf("invalid --corpus %q: use code, all or docs", o.Corpus)
			}
			shared := cmd.Flags().Changed("docs-share")
			if o.Docs || (shared && corpus == "") {
				if corpus != "" && corpus != "all" {
					return usageErrorf("-d/--docs searches code and documentation; it conflicts with --corpus %s", corpus)
				}
				corpus = "all"
			}
			if corpus == "" {
				corpus = "code"
			}
			var docsShare *float64
			if shared {
				if corpus != "all" {
					return usageErrorf("--docs-share splits code and documentation; it needs --corpus all (or -d), not %s", corpus)
				}
				if o.DocsShare < 0 || o.DocsShare > 1 {
					return usageErrorf("--docs-share must be from 0 to 1, got %v", o.DocsShare)
				}
				docsShare = &o.DocsShare
			}
			req := rag.AskRequest{
				Corpus:          corpus,
				DocsShare:       docsShare,
				Question:        strings.Join(args, " "),
				ProjectPath:     o.Project,
				AutoApprovePaid: globals.Yes,
				ForcePaid:       o.Paid,
				HybridMode:      o.Hybrid,
				UpdateIndex:     o.UpdateIndex,
				ServerURL:       o.Server,
				PaidModel:       o.PaidModel,
				LocalModel:      o.LocalModel,
				TopK:            o.TopK,
				Input:           cmd.InOrStdin(),
				Output:          cmd.OutOrStdout(),
			}
			return runAsk(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Paid, "paid", false, "Skip the local model and ask the paid model with the RAG context")
	f.BoolVar(&o.Hybrid, "hybrid", false, "Draft locally, then refine and verify with the paid model")
	f.BoolVar(&o.UpdateIndex, "update-index", false, "Rebuild the project index and ask the model again instead of reusing a cached answer")
	f.StringVar(&o.Server, "server", "", "Ask a central Dendrite RAG service instead (e.g. http://host:3456)")
	f.StringVar(&o.Tier, "tier", "", "Shortcut for the tier: local, hybrid or paid")
	addProjectFlag(cmd, &o.Project, ".", "Project directory to index and search")
	f.StringVar(&o.PaidModel, "paid-model", "gemini-2.5-pro", "Paid fallback model")
	f.StringVar(&o.LocalModel, "local-model", "", "Local model on the LiteLLM gateway (default: [ask] local_model in .dendrite/rag.toml, else the gateway's local model on the most preferred reachable GPU engine; never OxiLLaMa, which prefills on the CPU)")
	f.IntVarP(&o.TopK, "top-k", "k", 0, "Retrieve the N best chunks instead of a token-budgeted context pack")
	f.BoolVarP(&o.Docs, "docs", "d", false, "Also search the documentation (default: code only); same as --corpus all")
	f.StringVar(&o.Corpus, "corpus", "", "Corpora searched: code (default), all (code and documentation) or docs (documentation only, for design and conceptual questions)")
	f.Float64Var(&o.DocsShare, "docs-share", 0, "Share of the context kept for documentation with --corpus all, 0 to 1 (default: [ask] docs_share in .dendrite/rag.toml or DENDRITE_RAG_ASK_DOCS_SHARE, else 0.2); implies --corpus all")
	cmd.MarkFlagsMutuallyExclusive("paid", "hybrid")
	return cmd
}

func runAsk(ctx context.Context, req rag.AskRequest, out io.Writer) error {
	if req.ServerURL != "" {
		fmt.Fprintf(out, "[+] Forwarding query to Central RAG Service at %s for: %q\n", req.ServerURL, req.Question)
	} else {
		fmt.Fprintf(out, "[+] Querying Dendrite RAG index for: %q\n", req.Question)
	}
	res, err := rag.NewAskEngine().Ask(ctx, req)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}

	tierDisplay := "Zero-Cost Local LLM"
	switch res.SourceTier {
	case "paid-cloud":
		tierDisplay = "Paid Cloud LLM (Escalated)"
	case "hybrid":
		tierDisplay = "Hybrid Mode (Local First + Paid Cloud Refinement)"
	case "central-rag":
		tierDisplay = "Central Shared RAG Service"
	}
	cacheNote := ""
	if res.IndexStatus != "" {
		cacheNote = fmt.Sprintf(" | Index: %s", res.IndexStatus)
	}
	if res.Cached {
		cacheNote += " | cached answer (--update-index asks again)"
	}
	rule := strings.Repeat("=", 80)
	fmt.Fprintln(out, "\n"+rule)
	fmt.Fprintf(out, "[✓] Answer (%s | Model: %s | Time: %v%s):\n", tierDisplay, res.Model, res.Duration.Round(10*time.Millisecond), cacheNote)
	fmt.Fprintln(out, rule)
	fmt.Fprintln(out, res.Answer)
	fmt.Fprintln(out, rule)
	if t := res.Timings; t != nil {
		fmt.Fprintf(out, "[i] Timing: sidecar start %d ms, index %d ms (embedding %d ms), retrieval %d ms, local model %d ms, paid model %d ms\n",
			t.Spawn, t.Index, t.Embed, t.Retrieval, t.Local, t.Paid)
	}
	if res.LocalDraft != "" && res.SourceTier == "hybrid" {
		fmt.Fprintln(out, "\n[+] Note: Initial draft was synthesized locally at zero cost, then verified and refined via cloud LLM.")
	}
	if len(res.RetrievedChunks) > 0 {
		fmt.Fprintln(out, "\n[+] Retrieved Project Documentation & Code Context:")
		for i, c := range res.RetrievedChunks {
			heading := ""
			if c.Heading != "" {
				heading = fmt.Sprintf(" > %s", c.Heading)
			}
			fmt.Fprintf(out, "    %d. %s (lines %d-%d)%s\n", i+1, c.FilePath, c.LineStart, c.LineEnd, heading)
		}
	}
	return nil
}
