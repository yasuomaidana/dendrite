package main

import (
	"fmt"
	"strings"

	"Dendrite/pkg/rag"

	"github.com/spf13/cobra"
)

func newRagSymbolsCmd() *cobra.Command {
	var project string
	var refs bool
	cmd := &cobra.Command{
		Use:   "symbols <name>",
		Short: "Find definitions or references of a symbol name",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chunks, err := rag.NewRetriever().Symbols(cmd.Context(), project, args[0], refs)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, chunks)
			}
			action := "Definitions"
			if refs {
				action = "References"
			}
			fmt.Fprintf(out, "[✓] %s for %q (%d chunks):\n", action, args[0], len(chunks))
			for i, c := range chunks {
				fmt.Fprintf(out, "%2d. %s:%d-%d\n", i+1, c.FilePath, c.LineStart, c.LineEnd)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	cmd.Flags().BoolVar(&refs, "refs", false, "Find callers and users instead of definitions")
	return cmd
}

func newRagGrepCmd() *cobra.Command {
	var project string
	var paths []string
	var maxResults int
	cmd := &cobra.Command{
		Use:   "grep <pattern>",
		Short: "Regex search over indexable repository files",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			matches, err := rag.NewRetriever().Grep(cmd.Context(), project, args[0], paths, maxResults)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, matches)
			}
			for _, m := range matches {
				fmt.Fprintf(out, "%s:%d: %s\n", m.Path, m.Line, m.Text)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	cmd.Flags().StringSliceVar(&paths, "path", nil, "Filter glob patterns (repeatable)")
	cmd.Flags().IntVar(&maxResults, "max", 200, "Maximum matches")
	return cmd
}

func newRagHistoryCmd() *cobra.Command {
	var project string
	var topK int
	cmd := &cobra.Command{
		Use:   "history <query...>",
		Short: "Search Git commit messages and per-file diffs",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			hits, err := rag.NewRetriever().History(cmd.Context(), project, query, topK)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, hits)
			}
			fmt.Fprintf(out, "[✓] History hits for %q:\n", query)
			for _, h := range hits {
				sha := h.SHA
				if len(sha) > 10 {
					sha = sha[:10]
				}
				fmt.Fprintf(out, "%.2f  %s  %s  %s\n", h.Score, sha, h.Subject, h.Path)
			}
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	cmd.Flags().IntVarP(&topK, "top-k", "k", 10, "Maximum commits")
	return cmd
}

func newRagGraphCmd() *cobra.Command {
	var project string
	cmd := newGroupCmd("graph", "Query Leiden communities, god nodes, paths, and explanations", "")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "communities",
			Short: "List Leiden communities of files",
			RunE: func(cmd *cobra.Command, args []string) error {
				comms, err := rag.NewRetriever().Communities(cmd.Context(), project, rag.CommunitiesOptions{Limit: 50, MinSize: 2})
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, comms)
				}
				for _, c := range comms {
					fmt.Fprintf(out, "Community #%d: %s (%d files)\n", c.ID, c.Label, c.Size)
				}
				return nil
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
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, nodes)
				}
				for _, n := range nodes {
					fmt.Fprintf(out, "%s (%s:%d) - InDegree: %d\n", n.Symbol, n.Path, n.Line, n.InDegree)
				}
				return nil
			},
		},
	)
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagSyncCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Replay journaled offline commits to shared tier",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pending, err := rag.NewRetriever().Sync(cmd.Context(), project)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Sync complete: %d commits pending\n", pending)
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagPullCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch missing embeddings from shared tier for offline use",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := rag.NewRetriever().Pull(cmd.Context(), project)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Pull complete: %d chunks, %d cached vectors\n", rep.Chunks, rep.Cached)
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	return cmd
}

func newRagPushCmd() *cobra.Command {
	var project string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload local index to PostgreSQL shared tier",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := rag.NewRetriever().Push(cmd.Context(), project, dryRun)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, rep)
			}
			fmt.Fprintf(out, "[✓] Push complete (dry_run: %t): commit %s, uploaded %d embeddings\n", rep.DryRun, rep.Commit, rep.EmbeddingsUploaded)
			return nil
		},
	}
	addProjectFlag(cmd, &project, ".", "Project directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate push without modifying shared tier")
	return cmd
}
