package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

func openSegmentedStore() (*segmented.SegmentedStore, *status.StoreInspector, error) {
	st, err := segmented.NewSegmentedStore("")
	if err != nil {
		return nil, nil, fmt.Errorf("open segmented store: %w", err)
	}
	return st, status.NewStoreInspector(st), nil
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

// newEngramCmd builds `dendrite engram`: persistent segmented store and chunk checkpointer.
func newEngramCmd() *cobra.Command {
	cmd := newGroupCmd("engram", "Persistent segmented store, memory checkpointer, and chunk query", "soma",
		"store", "engram-store", "gram")
	cmd.Example = "dendrite engram status\ndendrite engram inspect dendrite --json\ndendrite engram chunks --project dendrite\ndendrite engram query --project dendrite"

	var statusProject string
	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Inspect persistent segmented store status",
		Args:    noArgs,
		Example: "dendrite engram status\ndendrite engram status --project dendrite --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramStatus(c.Context(), statusProject, globals.JSON, c.OutOrStdout())
		},
	}
	addProjectFlag(statusCmd, &statusProject, "", "Filter by project name")

	inspectCmd := &cobra.Command{
		Use:     "inspect <id>",
		Short:   "Inspect chunk breakdown and sample segments for a store",
		Args:    exactArgs(1),
		Example: "dendrite engram inspect dendrite\ndendrite engram inspect dendrite --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramInspect(c.Context(), args[0], globals.JSON, c.OutOrStdout())
		},
	}

	var rmForce bool
	rmCmd := &cobra.Command{
		Use:     "rm <id>",
		Aliases: []string{"remove"},
		Short:   "Remove all stored analysis segments and blobs for a store",
		Args:    exactArgs(1),
		Example: "dendrite engram rm dendrite --force",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramRm(c.Context(), args[0], rmForce || globals.Yes, c.OutOrStdout())
		},
	}
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Confirm removal of stored analysis")

	var chunksProject string
	chunksCmd := &cobra.Command{
		Use:     "chunks",
		Aliases: []string{"checkpoints"},
		Short:   "List registered chunk checkpoints",
		Args:    noArgs,
		Example: "dendrite engram chunks\ndendrite engram chunks --project dendrite --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramChunks(c.Context(), chunksProject, globals.JSON, c.OutOrStdout())
		},
	}
	addProjectFlag(chunksCmd, &chunksProject, "", "Filter by project name")

	var queryProject string
	queryCmd := &cobra.Command{
		Use:     "query",
		Short:   "Query available segments for a project",
		Args:    noArgs,
		Example: "dendrite engram query --project dendrite\ndendrite engram query --project dendrite --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramQuery(c.Context(), queryProject, globals.JSON, c.OutOrStdout())
		},
	}
	addProjectFlag(queryCmd, &queryProject, "", "Target project name")
	requireFlags(queryCmd, "project")

	var purgeProject string
	purgeCmd := &cobra.Command{
		Use:     "purge",
		Short:   "Purge all segmented analysis data for a project",
		Args:    noArgs,
		Example: "dendrite engram purge --project dendrite",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramPurge(c.Context(), purgeProject, c.OutOrStdout())
		},
	}
	addProjectFlag(purgeCmd, &purgeProject, "", "Target project name")
	requireFlags(purgeCmd, "project")

	var commitOpts engramCommitOptions
	commitCmd := &cobra.Command{
		Use:     "commit",
		Short:   "Commit a file segment into a project chunk checkpoint",
		Args:    noArgs,
		Example: "dendrite engram commit --project dendrite --chunk chk-001 --file main.go --final",
		RunE: func(c *cobra.Command, args []string) error {
			return runEngramCommit(c.Context(), commitOpts, globals.JSON, c.OutOrStdout())
		},
	}
	addProjectFlag(commitCmd, &commitOpts.Project, "", "Target project name")
	commitCmd.Flags().StringVarP(&commitOpts.Chunk, "chunk", "c", "", "Chunk checkpoint identifier")
	addFileFlag(commitCmd, &commitOpts.File, "", "Input file path")
	commitCmd.Flags().BoolVar(&commitOpts.Final, "final", false, "Mark chunk checkpoint as final (STORED)")
	requireFlags(commitCmd, "project", "chunk", "file")

	cmd.AddCommand(statusCmd, inspectCmd, rmCmd, chunksCmd, queryCmd, purgeCmd, commitCmd)
	return cmd
}

type engramCommitOptions struct {
	Project string
	Chunk   string
	File    string
	Final   bool
}

func runEngramStatus(ctx context.Context, project string, asJSON bool, out io.Writer) error {
	_, inspector, err := openSegmentedStore()
	if err != nil {
		return err
	}

	report, err := inspector.InspectStore(ctx, project)
	if err != nil {
		return fmt.Errorf("failed to inspect store status: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal store status: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintln(out, "[+] Persistent Segmented Store Status:")
	fmt.Fprint(out, status.FormatTable(report))
	fmt.Fprintf(out, "\nTotal: %d projects | %d segments | %d bytes raw -> %d bytes compressed\n",
		report.TotalStores, report.TotalSegments, report.TotalRawBytes, report.TotalCompBytes)
	return nil
}

func runEngramInspect(ctx context.Context, storeID string, asJSON bool, out io.Writer) error {
	_, inspector, err := openSegmentedStore()
	if err != nil {
		return err
	}

	details, err := inspector.InspectStoreDetails(ctx, storeID)
	if err != nil {
		return fmt.Errorf("failed to inspect store %q: %w", storeID, err)
	}

	if asJSON {
		data, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal store details: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
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

	if len(details.Segments) > 0 {
		n := min(len(details.Segments), 5)
		fmt.Fprintf(out, "\n    Sample Segments (first %d):\n", n)
		for _, seg := range details.Segments[:n] {
			fmt.Fprintf(out, "      - %-12s [%-12s] %s (%d bytes)\n", seg.Type, shortHash(seg.Hash), seg.Identifier, seg.SizeBytes)
		}
	}
	return nil
}

func runEngramRm(ctx context.Context, storeID string, confirmed bool, out io.Writer) error {
	if !confirmed {
		return usageErrorf("deleting stored analysis for %q requires confirmation: pass --force (or --yes)", storeID)
	}

	_, inspector, err := openSegmentedStore()
	if err != nil {
		return err
	}

	res, err := inspector.RemoveStore(ctx, storeID, true)
	if err != nil {
		return fmt.Errorf("failed to remove store %q: %w", storeID, err)
	}

	if globals.JSON {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal removal result: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Safely removed store '%s': purged %d segments, unlinked %d blobs, freed %d bytes.\n",
		res.StoreID, res.SegmentsPurged, res.BlobsUnlinked, res.BytesFreed)
	return nil
}

func runEngramChunks(ctx context.Context, project string, asJSON bool, out io.Writer) error {
	st, _, err := openSegmentedStore()
	if err != nil {
		return err
	}

	cps, err := st.ListCheckpoints(ctx, project)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(cps, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal checkpoints: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Registered Chunk Checkpoints (%d):\n", len(cps))
	for _, cp := range cps {
		fmt.Fprintf(out, "  • %-15s [%-18s] %-15s (%d/%d segments)\n",
			cp.ChunkID, cp.State, cp.Project, cp.CommittedSegments, cp.TotalSegments)
	}
	return nil
}

func runEngramQuery(ctx context.Context, project string, asJSON bool, out io.Writer) error {
	st, _, err := openSegmentedStore()
	if err != nil {
		return err
	}

	segs, err := st.QueryPartial(ctx, project)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(segs, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal segments: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Querying Segments for Project '%s' (%d available):\n", project, len(segs))
	for _, s := range segs {
		fmt.Fprintf(out, "  • %-12s [%-10s] %s (%d bytes)\n", s.Type, shortHash(s.Hash), s.Identifier, s.SizeBytes)
	}
	return nil
}

func runEngramPurge(ctx context.Context, project string, out io.Writer) error {
	st, _, err := openSegmentedStore()
	if err != nil {
		return err
	}

	if err := st.PurgeProject(ctx, project); err != nil {
		return fmt.Errorf("purge failed: %w", err)
	}
	fmt.Fprintf(out, "[+] Explicitly purged all segmented analysis data for project: %s\n", project)
	return nil
}

func runEngramCommit(ctx context.Context, o engramCommitOptions, asJSON bool, out io.Writer) error {
	st, _, err := openSegmentedStore()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(o.File)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
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
		return fmt.Errorf("commit failed: %w", err)
	}

	if asJSON {
		outData, err := json.MarshalIndent(cp, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal checkpoint: %w", err)
		}
		fmt.Fprintln(out, string(outData))
		return nil
	}

	fmt.Fprintf(out, "[+] Committed chunk '%s' for project '%s' (State: %s, Hash: %s)\n",
		cp.ChunkID, cp.Project, cp.State, shortHash(hash))
	return nil
}

func handleStoreCommand(ctx context.Context, action string, args []string) {
	_ = invokeCmd(ctx, newEngramCmd(), append([]string{action}, args...)...)
}

func printStoreUsage() {
	_ = newEngramCmd().Help()
}
