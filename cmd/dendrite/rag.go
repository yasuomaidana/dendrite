package main

import (
	"Dendrite/pkg/ticketindex"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/rag"

	"github.com/spf13/cobra"
)

// newRagCmd builds `dendrite rag`: direct orchestration of the dendrite-rag retrieval sidecar.
func newRagCmd() *cobra.Command {
	cmd := newGroupCmd("rag", "Code & documentation retrieval sidecar orchestration (dendrite-rag)", "cortex",
		"retrieval", "retriever")
	cmd.AddCommand(
		newRagIndexCmd(),
		newRagSearchCmd(),
		newRagContextCmd(),
		newRagShowCmd(),
		newRagStatusCmd(),
		newRagSettingsCmd(),
		newRagEmbeddersCmd(),
		newRagSymbolsCmd(),
		newRagGrepCmd(),
		newRagHistoryCmd(),
		newRagGraphCmd(),
		newRagSyncCmd(),
		newRagPullCmd(),
		newRagPushCmd(),
		newRagPurgeCmd(),
	)
	return cmd
}

func newRagIndexCmd() *cobra.Command {
	var project, focus string
	var full bool
	var embedBudgetMS int64
	var embedBudgetTokens int
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Index project incrementally or from scratch with --full",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := rag.NewRetriever().Index(cmd.Context(), project, rag.IndexOptions{
				Full:              full,
				EmbedBudgetMS:     embedBudgetMS,
				EmbedBudgetTokens: embedBudgetTokens,
				Focus:             focus,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, rep)
			}
			fmt.Fprintf(out, "[✓] Indexed %s in %d ms (walk: %d ms, embed: %d ms)\n", project, rep.DurationMS, rep.WalkMS, rep.EmbedMS)
			fmt.Fprintf(out, "    Files: %d scanned, %d changed, %d removed\n", rep.FilesScanned, rep.FilesChanged, rep.FilesRemoved)
			fmt.Fprintf(out, "    Chunks: %d added, %d removed, %d embedded (%d pending)\n", rep.ChunksAdded, rep.ChunksRemoved, rep.Embedded, rep.EmbedPending)
			if rep.BudgetTokensRemaining != nil {
				fmt.Fprintf(out, "    Tokens: %d spent, %d remaining in budget\n", rep.EmbedTokens, *rep.BudgetTokensRemaining)
			}
			if rep.EmbedError != "" {
				fmt.Fprintf(out, "    [!] Embed error: %s\n", rep.EmbedError)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory to index")
	cmd.Flags().BoolVar(&full, "full", false, "Force full re-indexing")
	cmd.Flags().Int64Var(&embedBudgetMS, "embed-budget-ms", 0, "Stop embedding after this many milliseconds")
	cmd.Flags().IntVar(&embedBudgetTokens, "embed-budget-tokens", 0, "Stop embedding after this many tokens")
	cmd.Flags().StringVar(&focus, "focus", "", "Under a budget, prioritize embedding chunks this question matches")
	return cmd
}

func newRagSearchCmd() *cobra.Command {
	var project, corpus string
	var topK int
	var minScore float64
	cmd := &cobra.Command{
		Use:   "search <query...>",
		Short: "Hybrid search over the project index",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			searchProject, searchCorpus, err := ticketindex.Resolve(cmd.Context(), project, corpus)
			if err != nil {
				return err
			}
			res, err := rag.NewRetriever().Retrieve(cmd.Context(), searchProject, query, rag.SearchOptions{
				TopK:     topK,
				MinScore: float32(minScore),
				Corpus:   searchCorpus,
			}, false)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, res)
			}
			fmt.Fprintf(out, "[✓] Hits for %q (%d chunks, %s):\n", query, len(res.Chunks), res.IndexStatus)
			for i, c := range res.Chunks {
				scope := ""
				if c.Heading != "" {
					scope = "  " + c.Heading
				}
				fmt.Fprintf(out, "%2d. [%.4f] %s:%d-%d%s\n", i+1, c.Score, c.FilePath, c.LineStart, c.LineEnd, scope)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory to search")
	cmd.Flags().IntVarP(&topK, "top-k", "k", 10, "Maximum number of hits")
	cmd.Flags().StringVar(&corpus, "corpus", "all", "Corpus filter: all, code, docs, or tickets (the rendered Plane tickets)")
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "Minimum relevance score filter")
	return cmd
}

func newRagContextCmd() *cobra.Command {
	var project, corpus, detail string
	var budget int
	var docsShare float64
	cmd := &cobra.Command{
		Use:   "context <query...>",
		Short: "Generate a token-budgeted prompt context pack",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			var dsPtr *float64
			if cmd.Flags().Changed("docs-share") {
				dsPtr = &docsShare
			}
			ctxProject, ctxCorpus, rerr := ticketindex.Resolve(cmd.Context(), project, corpus)
			if rerr != nil {
				return rerr
			}
			res, err := rag.NewRetriever().ContextWith(cmd.Context(), ctxProject, query, budget, rag.ContextOptions{
				Corpus:    ctxCorpus,
				Detail:    detail,
				DocsShare: dsPtr,
			}, false)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, res)
			}
			if res.Abstained {
				fmt.Fprintln(out, "[!] Abstained: no project context met the reranker relevance threshold.")
				return nil
			}
			fmt.Fprintf(out, "[✓] Context pack: ~%d tokens across %d spans\n", res.Tokens, len(res.Chunks))
			for i, c := range res.Chunks {
				tag := ""
				if c.Outline {
					tag = " [outline]"
				}
				fmt.Fprintf(out, "--- Span %d: %s:%d-%d%s ---\n%s\n", i+1, c.FilePath, c.LineStart, c.LineEnd, tag, c.Content)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	cmd.Flags().IntVar(&budget, "budget", 2500, "Estimated token budget")
	cmd.Flags().StringVar(&corpus, "corpus", "all", "Corpus: all, code, docs, or tickets (the rendered Plane tickets)")
	cmd.Flags().StringVar(&detail, "detail", "auto", "Detail level: auto, full, or map")
	cmd.Flags().Float64Var(&docsShare, "docs-share", 0.2, "Share of budget kept for documentation")
	return cmd
}

func newRagShowCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "show <chunk_id...>",
		Short: "Display the full content of chunk IDs",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chunks, err := rag.NewRetriever().Show(cmd.Context(), project, args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, chunks)
			}
			for _, c := range chunks {
				fmt.Fprintf(out, "=== %s (%s:%d-%d) ===\n%s\n", c.ID, c.FilePath, c.LineStart, c.LineEnd, c.Content)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagStatusCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display RAG index metrics, coverage, and warnings",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := rag.NewRetriever().Status(cmd.Context(), project)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, st)
			}
			cov := 100.0
			if st.EmbedCoverage != nil {
				cov = *st.EmbedCoverage * 100.0
			}
			fmt.Fprintf(out, "[+] RAG Status for %s:\n", project)
			fmt.Fprintf(out, "    Files:         %d\n", st.Files)
			fmt.Fprintf(out, "    Chunks:        %d\n", st.Chunks)
			fmt.Fprintf(out, "    Vectors:       %d (%.1f%% coverage)\n", st.Vectors, cov)
			fmt.Fprintf(out, "    Embed Model:   %s\n", st.EmbedModel)
			if st.LastIndexed != nil {
				fmt.Fprintf(out, "    Last Indexed:  %s\n", time.Unix(*st.LastIndexed, 0).Format(time.RFC3339))
			}
			for _, w := range st.Warnings {
				fmt.Fprintf(out, "    [!] Warning:   %s\n", w)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagSettingsCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Inspect the resolved .dendrite/rag.toml settings and env overrides",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := rag.NewRetriever().Settings(cmd.Context(), project)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return writeJSON(out, s)
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagEmbeddersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embedders",
		Short: "List compiled and available embedding backends in dendrite-rag",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			backends, err := rag.NewRetriever().Embedders(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, backends)
			}
			fmt.Fprintf(out, "%-10s %-13s %-34s %s\n", "BACKEND", "STATUS", "DEFAULT MODEL", "DESCRIPTION")
			for _, b := range backends {
				status := "not compiled"
				if b.Compiled {
					status = "available"
				}
				model := "-"
				if b.DefaultModel != nil {
					model = *b.DefaultModel
				}
				fmt.Fprintf(out, "%-10s %-13s %-34s %s\n", b.Backend, status, model, b.Description)
			}
			return nil
		},
	}
	return cmd
}

func newRagPurgeCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Delete local RAG index for project",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rag.NewRetriever().Purge(cmd.Context(), project); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Purged RAG index for %s\n", project)
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}
