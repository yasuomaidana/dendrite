package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/rag"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// ragLeaf builds a rag subcommand over one project: call asks the sidecar and show prints the answer
// (as JSON with --json).
func ragLeaf[R any](use, short string, args cobra.PositionalArgs, flags func(*cobra.Command),
	call func(ctx context.Context, project string, args []string) (R, error),
	show func(out io.Writer, result R, args []string)) *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := call(cmd.Context(), project, args)
			if err != nil {
				return err
			}
			return printResult(cmd, result, func(out io.Writer) { show(out, result, args) })
		},
	}
	cli.AddProjectFlag(cmd, &project, ".", "Project directory")
	if flags != nil {
		flags(cmd)
	}
	return cmd
}

func newRagSymbolsCmd() *cobra.Command {
	var refs bool
	return ragLeaf("symbols <name>", "Find definitions or references of a symbol name", cli.ExactArgs(1),
		func(c *cobra.Command) {
			c.Flags().BoolVar(&refs, "refs", false, "Find callers and users instead of definitions")
		},
		func(ctx context.Context, project string, args []string) ([]rag.Chunk, error) {
			return rag.NewRetriever().Symbols(ctx, project, args[0], refs)
		},
		func(out io.Writer, chunks []rag.Chunk, args []string) {
			action := "Definitions"
			if refs {
				action = "References"
			}
			fmt.Fprintf(out, "[✓] %s for %q (%d chunks):\n", action, args[0], len(chunks))
			for i, c := range chunks {
				fmt.Fprintf(out, "%2d. %s:%d-%d\n", i+1, c.FilePath, c.LineStart, c.LineEnd)
			}
		})
}

func newRagGrepCmd() *cobra.Command {
	var paths []string
	var maxResults int
	return ragLeaf("grep <pattern>", "Regex search over indexable repository files", cli.ExactArgs(1),
		func(c *cobra.Command) {
			c.Flags().StringSliceVar(&paths, "path", nil, "Filter glob patterns (repeatable)")
			c.Flags().IntVar(&maxResults, "max", 200, "Maximum matches")
		},
		func(ctx context.Context, project string, args []string) ([]rag.GrepMatch, error) {
			return rag.NewRetriever().Grep(ctx, project, args[0], paths, maxResults)
		},
		func(out io.Writer, matches []rag.GrepMatch, _ []string) {
			for _, m := range matches {
				fmt.Fprintf(out, "%s:%d: %s\n", m.Path, m.Line, m.Text)
			}
		})
}

func newRagHistoryCmd() *cobra.Command {
	var topK int
	return ragLeaf("history <query...>", "Search Git commit messages and per-file diffs", cli.MinArgs(1),
		func(c *cobra.Command) { c.Flags().IntVarP(&topK, "top-k", "k", 10, "Maximum commits") },
		func(ctx context.Context, project string, args []string) ([]rag.HistoryHit, error) {
			return rag.NewRetriever().History(ctx, project, strings.Join(args, " "), topK)
		},
		func(out io.Writer, hits []rag.HistoryHit, args []string) {
			fmt.Fprintf(out, "[✓] History hits for %q:\n", strings.Join(args, " "))
			for _, h := range hits {
				sha := h.SHA
				if len(sha) > 10 {
					sha = sha[:10]
				}
				fmt.Fprintf(out, "%.2f  %s  %s  %s\n", h.Score, sha, h.Subject, h.Path)
			}
		})
}

func newRagGraphCmd() *cobra.Command {
	var project string
	cmd := cli.NewGroupCmd("graph", "Query Leiden communities, god nodes, paths, and explanations", "")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "communities",
			Short: "List Leiden communities of files",
			RunE: func(cmd *cobra.Command, args []string) error {
				comms, err := rag.NewRetriever().Communities(cmd.Context(), project, rag.CommunitiesOptions{Limit: 50, MinSize: 2})
				if err != nil {
					return err
				}
				return printResult(cmd, comms, func(out io.Writer) {
					for _, c := range comms {
						fmt.Fprintf(out, "Community #%d: %s (%d files)\n", c.ID, c.Label, c.Size)
					}
				})
			},
		},
		&cobra.Command{
			Use:   "god-nodes",
			Short: "Symbols with the most callers and type users",
			RunE: func(cmd *cobra.Command, args []string) error {
				nodes, err := rag.NewRetriever().GodNodes(cmd.Context(), project, rag.GodNodesOptions{MinInDegree: 21, Limit: 50})
				if err != nil {
					return err
				}
				return printResult(cmd, nodes, func(out io.Writer) {
					for _, n := range nodes {
						fmt.Fprintf(out, "%s (%s:%d) - InDegree: %d\n", n.Symbol, n.Path, n.Line, n.InDegree)
					}
				})
			},
		},
	)
	cli.AddProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagSyncCmd() *cobra.Command {
	return ragLeaf("sync", "Replay journaled offline commits to shared tier", cli.NoArgs, nil,
		func(ctx context.Context, project string, _ []string) (int, error) {
			return rag.NewRetriever().Sync(ctx, project)
		},
		func(out io.Writer, pending int, _ []string) {
			fmt.Fprintf(out, "[✓] Sync complete: %d commits pending\n", pending)
		})
}

func newRagPullCmd() *cobra.Command {
	return ragLeaf("pull", "Fetch missing embeddings from shared tier for offline use", cli.NoArgs, nil,
		func(ctx context.Context, project string, _ []string) (rag.PullReport, error) {
			return rag.NewRetriever().Pull(ctx, project)
		},
		func(out io.Writer, rep rag.PullReport, _ []string) {
			fmt.Fprintf(out, "[✓] Pull complete: %d chunks, %d cached vectors\n", rep.Chunks, rep.Cached)
		})
}

func newRagPushCmd() *cobra.Command {
	var dryRun bool
	return ragLeaf("push", "Upload local index to PostgreSQL shared tier", cli.NoArgs,
		func(c *cobra.Command) {
			c.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate push without modifying shared tier")
		},
		func(ctx context.Context, project string, _ []string) (rag.PushReport, error) {
			return rag.NewRetriever().Push(ctx, project, dryRun)
		},
		func(out io.Writer, rep rag.PushReport, _ []string) {
			fmt.Fprintf(out, "[✓] Push complete (dry_run: %t): commit %s, uploaded %d embeddings\n", rep.DryRun, rep.Commit, rep.EmbeddingsUploaded)
		})
}

// printResult writes v as JSON with --json, otherwise lets text print the human form.
func printResult(cmd *cobra.Command, v interface{}, text func(out io.Writer)) error {
	out := cmd.OutOrStdout()
	if cli.Globals.JSON {
		return cli.WriteJSON(out, v)
	}
	text(out)
	return nil
}
