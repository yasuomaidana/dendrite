package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"

	"github.com/spf13/cobra"
)

// openSegmentedStore opens the persistent store on use; tests replace it with a temp store.
var openSegmentedStore = func() (*segmented.SegmentedStore, error) { return segmented.NewSegmentedStore("") }

// withStore opens the segmented store and runs fn with it.
func withStore(fn func(*segmented.SegmentedStore) error) error {
	st, err := openSegmentedStore()
	if err != nil {
		return fmt.Errorf("open segmented store: %w", err)
	}
	return fn(st)
}

// shortHash returns the first 12 characters of a content hash (all of it when shorter).
func shortHash(h string) string { return h[:min(12, len(h))] }

// newEngramCmd builds `dendrite engram` (alias store): the persistent segmented analysis store.
func newEngramCmd() *cobra.Command {
	cmd := newGroupCmd("engram", "Persistent segmented store, memory checkpointer, and chunk query", "soma",
		"store", "engram-store", "gram")
	cmd.Example = "dendrite engram status\n" +
		"dendrite engram inspect dendrite --json\n" +
		"dendrite engram commit --project dendrite --chunk ast-1 --file graph.json --final\n" +
		"dendrite engram rm old-project --force"

	var project string
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Summarize every stored project (segments, bytes, progress)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				return runEngramStatus(cmd.Context(), st, project, cmd.OutOrStdout())
			})
		},
	}
	addProjectFlag(statusCmd, &project, "", "Only this project")

	inspect := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show one store's checkpoints and sample segments",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				return runEngramInspect(cmd.Context(), st, args[0], cmd.OutOrStdout())
			})
		},
	}

	var force bool
	rm := &cobra.Command{
		Use:     "rm <id>",
		Aliases: []string{"remove"},
		Short:   "Delete a store's segments and blobs (needs --force or --yes)",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !globals.Yes {
				return usageErrorf("deleting all stored analysis for '%s' needs --force (or --yes)", args[0])
			}
			return withStore(func(st *segmented.SegmentedStore) error {
				res, err := status.NewStoreInspector(st).RemoveStore(cmd.Context(), args[0], true)
				if err != nil {
					return fmt.Errorf("remove store '%s': %w", args[0], err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Safely removed store '%s': purged %d segments, unlinked %d blobs, freed %d bytes.\n",
					res.StoreID, res.SegmentsPurged, res.BlobsUnlinked, res.BytesFreed)
				return nil
			})
		},
	}
	rm.Flags().BoolVarP(&force, "force", "f", false, "Confirm the deletion")

	var chunksProject string
	chunks := &cobra.Command{
		Use:     "chunks",
		Aliases: []string{"checkpoints"},
		Short:   "List chunk checkpoints",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				cps, err := st.ListCheckpoints(cmd.Context(), chunksProject)
				if err != nil {
					return fmt.Errorf("list checkpoints: %w", err)
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, cps)
				}
				fmt.Fprintf(out, "[+] Registered Chunk Checkpoints (%d):\n", len(cps))
				for _, cp := range cps {
					fmt.Fprintf(out, "  • %-15s [%-18s] %-15s (%d/%d segments)\n",
						cp.ChunkID, cp.State, cp.Project, cp.CommittedSegments, cp.TotalSegments)
				}
				return nil
			})
		},
	}
	addProjectFlag(chunks, &chunksProject, "", "Only this project")

	var queryProject string
	query := &cobra.Command{
		Use:   "query",
		Short: "List the committed segments of a project, including partial chunks",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				segs, err := st.QueryPartial(cmd.Context(), queryProject)
				if err != nil {
					return fmt.Errorf("query: %w", err)
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, segs)
				}
				fmt.Fprintf(out, "[+] Querying Segments for Project '%s' (%d available):\n", queryProject, len(segs))
				for _, s := range segs {
					fmt.Fprintf(out, "  • %-12s [%-12s] %s (%d bytes)\n", s.Type, shortHash(s.Hash), s.Identifier, s.SizeBytes)
				}
				return nil
			})
		},
	}
	addProjectFlag(query, &queryProject, "", "Project to query")
	requireFlags(query, "project")

	var purgeProject string
	purge := &cobra.Command{
		Use:   "purge",
		Short: "Delete all segmented analysis data of a project",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				if err := st.PurgeProject(cmd.Context(), purgeProject); err != nil {
					return fmt.Errorf("purge: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Explicitly purged all segmented analysis data for project: %s\n", purgeProject)
				return nil
			})
		},
	}
	addProjectFlag(purge, &purgeProject, "", "Project to purge")
	requireFlags(purge, "project")

	cmd.AddCommand(statusCmd, inspect, rm, chunks, query, purge, newEngramCommitCmd())
	return cmd
}

func runEngramStatus(ctx context.Context, st *segmented.SegmentedStore, project string, out io.Writer) error {
	report, err := status.NewStoreInspector(st).InspectStore(ctx, project)
	if err != nil {
		return fmt.Errorf("inspect store status: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, report)
	}
	fmt.Fprintln(out, "[+] Persistent Segmented Store Status:")
	fmt.Fprint(out, status.FormatTable(report))
	fmt.Fprintf(out, "\nTotal: %d projects | %d segments | %d bytes raw -> %d bytes compressed\n",
		report.TotalStores, report.TotalSegments, report.TotalRawBytes, report.TotalCompBytes)
	return nil
}

func runEngramInspect(ctx context.Context, st *segmented.SegmentedStore, id string, out io.Writer) error {
	details, err := status.NewStoreInspector(st).InspectStoreDetails(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect store '%s': %w", id, err)
	}
	if globals.JSON {
		return writeJSON(out, details)
	}
	s := details.Summary
	fmt.Fprintf(out, "[+] Store Analysis Details: %s\n", s.StoreID)
	fmt.Fprintf(out, "    Kind:          %s\n", s.Kind)
	fmt.Fprintf(out, "    Overall State: %s (%.1f%% complete)\n", s.State, s.Progress)
	fmt.Fprintf(out, "    Segments:      %d\n", s.SegmentsCount)
	fmt.Fprintf(out, "    Storage:       %d bytes raw -> %d bytes compressed\n", s.RawBytes, s.CompressedBytes)
	fmt.Fprintf(out, "    Est. Tokens:   %d tokens\n", s.TokensEstimated)
	fmt.Fprintf(out, "    Updated:       %s\n\n", s.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "    Chunk Breakdown (%d checkpoints):\n", len(details.Checkpoints))
	for _, cp := range details.Checkpoints {
		fmt.Fprintf(out, "      • %-16s [%-18s] %d/%d segments (%.1f%%) updated %s\n",
			cp.ChunkID, cp.State, cp.CommittedSegments, cp.TotalSegments, cp.PercentComplete, cp.UpdatedAt.Format("15:04:05"))
	}
	if n := min(len(details.Segments), 5); n > 0 {
		fmt.Fprintf(out, "\n    Sample Segments (first %d):\n", n)
		for _, seg := range details.Segments[:n] {
			fmt.Fprintf(out, "      - %-12s [%-12s] %s (%d bytes)\n", seg.Type, shortHash(seg.Hash), seg.Identifier, seg.SizeBytes)
		}
	}
	return nil
}

type engramCommitOptions struct {
	Project string
	Chunk   string
	File    string
	Final   bool
}

func newEngramCommitCmd() *cobra.Command {
	var o engramCommitOptions
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit a file as one segment of a chunk checkpoint",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(st *segmented.SegmentedStore) error {
				return runEngramCommit(cmd.Context(), st, o, cmd.OutOrStdout())
			})
		},
	}
	addProjectFlag(cmd, &o.Project, "", "Project the chunk belongs to")
	cmd.Flags().StringVarP(&o.Chunk, "chunk", "c", "", "Chunk ID")
	addFileFlag(cmd, &o.File, "", "File to store")
	cmd.Flags().BoolVar(&o.Final, "final", false, "Mark the chunk as complete")
	requireFlags(cmd, "project", "chunk", "file")
	return cmd
}

func runEngramCommit(ctx context.Context, st *segmented.SegmentedStore, o engramCommitOptions, out io.Writer) error {
	data, err := os.ReadFile(o.File)
	if err != nil {
		return fmt.Errorf("read input file: %w", err)
	}
	hash := st.ComputeHash(data)
	seg := segmented.Segment{
		Hash:       hash,
		Type:       segmented.SegmentFile,
		Identifier: o.File,
		Project:    o.Project,
		SizeBytes:  int64(len(data)),
		CreatedAt:  time.Now().UTC(),
	}
	cp, err := st.CommitChunk(ctx, o.Project, o.Chunk, []segmented.Segment{seg}, map[string][]byte{hash: data}, o.Final)
	if err != nil {
		return fmt.Errorf("commit chunk: %w", err)
	}
	fmt.Fprintf(out, "[+] Committed chunk '%s' for project '%s' (State: %s, Hash: %s)\n",
		cp.ChunkID, cp.Project, cp.State, shortHash(hash))
	return nil
}
