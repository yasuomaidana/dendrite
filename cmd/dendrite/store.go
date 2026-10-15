package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

var defaultSegmentedStore, _ = segmented.NewSegmentedStore("")
var defaultStoreInspector = status.NewStoreInspector(defaultSegmentedStore)

func handleStoreCommand(ctx context.Context, action string, args []string) {
	if defaultSegmentedStore == nil {
		fmt.Println("[-] Segmented store unavailable")
		os.Exit(1)
	}

	switch action {
	case "status":
		handleStoreStatus(ctx, args)
	case "inspect":
		handleStoreInspect(ctx, args)
	case "rm", "remove":
		handleStoreRm(ctx, args)
	case "chunks", "checkpoints":
		handleStoreChunks(ctx, args)
	case "query":
		handleStoreQuery(ctx, args)
	case "purge":
		handleStorePurge(ctx, args)
	case "commit":
		handleStoreCommit(ctx, args)
	default:
		printStoreUsage()
	}
}

func handleStoreStatus(ctx context.Context, args []string) {
	project := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		if (args[i] == "--project" || args[i] == "-p") && i+1 < len(args) {
			project = args[i+1]
			i++
		}
		if args[i] == "--json" {
			jsonOutput = true
		}
	}

	report, err := defaultStoreInspector.InspectStore(ctx, project)
	if err != nil {
		fmt.Printf("[-] Failed to inspect store status: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[+] Persistent Segmented Store Status:")
	fmt.Print(status.FormatTable(report))
	fmt.Printf("\nTotal: %d projects | %d segments | %d bytes raw -> %d bytes compressed\n",
		report.TotalStores, report.TotalSegments, report.TotalRawBytes, report.TotalCompBytes)
}

func handleStoreInspect(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("[-] Usage: dendrite store inspect <id> [--json]")
		return
	}

	storeID := args[0]
	jsonOutput := false
	for _, a := range args[1:] {
		if a == "--json" {
			jsonOutput = true
		}
	}

	details, err := defaultStoreInspector.InspectStoreDetails(ctx, storeID)
	if err != nil {
		fmt.Printf("[-] Failed to inspect store '%s': %v\n", storeID, err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(details, "", "  ")
		fmt.Println(string(data))
		return
	}

	s := details.Summary
	fmt.Printf("[+] Store Analysis Details: %s\n", s.StoreID)
	fmt.Printf("    Kind:          %s\n", s.Kind)
	fmt.Printf("    Overall State: %s (%.1f%% complete)\n", s.State, s.Progress)
	fmt.Printf("    Segments:      %d\n", s.SegmentsCount)
	fmt.Printf("    Storage:       %d bytes raw -> %d bytes compressed\n", s.RawBytes, s.CompressedBytes)
	fmt.Printf("    Est. Tokens:   %d tokens\n", s.TokensEstimated)
	fmt.Printf("    Updated:       %s\n\n", s.UpdatedAt.Format("2006-01-02 15:04:05"))

	fmt.Printf("    Chunk Breakdown (%d checkpoints):\n", len(details.Checkpoints))
	for _, cp := range details.Checkpoints {
		fmt.Printf("      • %-16s [%-18s] %d/%d segments (%.1f%%) updated %s\n",
			cp.ChunkID, cp.State, cp.CommittedSegments, cp.TotalSegments, cp.PercentComplete, cp.UpdatedAt.Format("15:04:05"))
	}

	if len(details.Segments) > 0 {
		fmt.Printf("\n    Sample Segments (first %d):\n", min(len(details.Segments), 5))
		for _, seg := range details.Segments[:min(len(details.Segments), 5)] {
			fmt.Printf("      - %-12s [%-12s] %s (%d bytes)\n", seg.Type, seg.Hash[:12], seg.Identifier, seg.SizeBytes)
		}
	}
}

func handleStoreRm(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("[-] Usage: dendrite store rm <id> [--force]")
		return
	}

	storeID := args[0]
	force := false
	for _, a := range args[1:] {
		if a == "--force" || a == "-f" {
			force = true
		}
	}

	if !force {
		fmt.Printf("Are you sure you want to delete all stored analysis for '%s'? (pass --force to confirm)\n", storeID)
		return
	}

	res, err := defaultStoreInspector.RemoveStore(ctx, storeID, true)
	if err != nil {
		fmt.Printf("[-] Failed to remove store '%s': %v\n", storeID, err)
		os.Exit(1)
	}

	fmt.Printf("[+] Safely removed store '%s': purged %d segments, unlinked %d blobs, freed %d bytes.\n",
		res.StoreID, res.SegmentsPurged, res.BlobsUnlinked, res.BytesFreed)
}

func handleStoreChunks(ctx context.Context, args []string) {
	project := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		if (args[i] == "--project" || args[i] == "-p") && i+1 < len(args) {
			project = args[i+1]
			i++
		}
		if args[i] == "--json" {
			jsonOutput = true
		}
	}

	cps, err := defaultSegmentedStore.ListCheckpoints(ctx, project)
	if err != nil {
		fmt.Printf("[-] Failed to list checkpoints: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(cps, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[+] Registered Chunk Checkpoints (%d):\n", len(cps))
	for _, cp := range cps {
		fmt.Printf("  • %-15s [%-18s] %-15s (%d/%d segments)\n",
			cp.ChunkID, cp.State, cp.Project, cp.CommittedSegments, cp.TotalSegments)
	}
}

func handleStoreQuery(ctx context.Context, args []string) {
	project := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		if (args[i] == "--project" || args[i] == "-p") && i+1 < len(args) {
			project = args[i+1]
			i++
		}
		if args[i] == "--json" {
			jsonOutput = true
		}
	}

	if project == "" {
		fmt.Println("[-] Usage: dendrite store query --project <name> [--json]")
		return
	}

	segs, err := defaultSegmentedStore.QueryPartial(ctx, project)
	if err != nil {
		fmt.Printf("[-] Query failed: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(segs, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[+] Querying Segments for Project '%s' (%d available):\n", project, len(segs))
	for _, s := range segs {
		fmt.Printf("  • %-12s [%-10s] %s (%d bytes)\n", s.Type, s.Hash[:12], s.Identifier, s.SizeBytes)
	}
}

func handleStorePurge(ctx context.Context, args []string) {
	project := ""
	for i := 0; i < len(args); i++ {
		if (args[i] == "--project" || args[i] == "-p") && i+1 < len(args) {
			project = args[i+1]
			i++
		}
	}

	if project == "" {
		fmt.Println("[-] Usage: dendrite store purge --project <name>")
		return
	}

	if err := defaultSegmentedStore.PurgeProject(ctx, project); err != nil {
		fmt.Printf("[-] Purge failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[+] Explicitly purged all segmented analysis data for project: %s\n", project)
}

func handleStoreCommit(ctx context.Context, args []string) {
	project := ""
	chunkID := ""
	filePath := ""
	isFinal := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "-p":
			if i+1 < len(args) {
				project = args[i+1]
				i++
			}
		case "--chunk", "-c":
			if i+1 < len(args) {
				chunkID = args[i+1]
				i++
			}
		case "--file", "-f":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		case "--final":
			isFinal = true
		}
	}

	if project == "" || chunkID == "" || filePath == "" {
		fmt.Println("[-] Usage: dendrite store commit --project <name> --chunk <id> --file <path> [--final]")
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("[-] Failed to read input file: %v\n", err)
		os.Exit(1)
	}

	hash := defaultSegmentedStore.ComputeHash(data)
	seg := segmented.Segment{
		Hash:       hash,
		Type:       segmented.SegmentFile,
		Identifier: filePath,
		Project:    project,
		SizeBytes:  int64(len(data)),
		CreatedAt:  time.Now().UTC(),
	}

	cp, err := defaultSegmentedStore.CommitChunk(ctx, project, chunkID, []segmented.Segment{seg}, map[string][]byte{hash: data}, isFinal)
	if err != nil {
		fmt.Printf("[-] Commit failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[+] Committed chunk '%s' for project '%s' (State: %s, Hash: %s)\n",
		cp.ChunkID, cp.Project, cp.State, hash[:12])
}

func printStoreUsage() {
	fmt.Println("Usage: dendrite store <status|inspect|rm|chunks|query|commit|purge> [flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  dendrite store status [--project <name>] [--json]")
	fmt.Println("  dendrite store inspect <id> [--json]")
	fmt.Println("  dendrite store rm <id> [--force]")
	fmt.Println("  dendrite store chunks [--project <name>] [--json]")
	fmt.Println("  dendrite store query --project <name> [--json]")
	fmt.Println("  dendrite store commit --project <name> --chunk <id> --file <path> [--final]")
	fmt.Println("  dendrite store purge --project <name>")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
