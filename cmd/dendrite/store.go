package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"Dendrite/pkg/store/segmented"
)

var defaultSegmentedStore, _ = segmented.NewSegmentedStore("")

func handleStoreCommand(ctx context.Context, action string, args []string) {
	if defaultSegmentedStore == nil {
		fmt.Println("[-] Segmented store unavailable")
		os.Exit(1)
	}

	switch action {
	case "status":
		handleStoreStatus(ctx, args)
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

	segs, err := defaultSegmentedStore.QueryPartial(ctx, project)
	if err != nil {
		fmt.Printf("[-] Failed to query store segments: %v\n", err)
		os.Exit(1)
	}

	cps, err := defaultSegmentedStore.ListCheckpoints(ctx, project)
	if err != nil {
		fmt.Printf("[-] Failed to list checkpoints: %v\n", err)
		os.Exit(1)
	}

	var storedCount, partialCount int
	var totalRawBytes, totalCompBytes int64
	for _, cp := range cps {
		if cp.State == segmented.StateStored {
			storedCount++
		} else if cp.State == segmented.StateAnalyzingPartial {
			partialCount++
		}
	}
	for _, s := range segs {
		totalRawBytes += s.SizeBytes
		totalCompBytes += s.CompressedSize
	}

	statusMap := map[string]interface{}{
		"project":            project,
		"total_segments":     len(segs),
		"total_checkpoints":  len(cps),
		"stored_chunks":      storedCount,
		"partial_chunks":     partialCount,
		"raw_bytes":          totalRawBytes,
		"compressed_bytes":   totalCompBytes,
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(statusMap, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[+] Persistent Segmented Store Status:")
	if project != "" {
		fmt.Printf("    Target Project:     %s\n", project)
	}
	fmt.Printf("    Total Segments:     %d (files, functions, AST blocks)\n", len(segs))
	fmt.Printf("    Checkpoints:        %d (Stored: %d, Partial: %d)\n", len(cps), storedCount, partialCount)
	fmt.Printf("    Storage Volume:     %d bytes raw -> %d bytes compressed\n", totalRawBytes, totalCompBytes)
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
	fmt.Println("Usage: dendrite store <status|chunks|query|commit|purge> [flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  dendrite store status [--project <name>] [--json]")
	fmt.Println("  dendrite store chunks [--project <name>] [--json]")
	fmt.Println("  dendrite store query --project <name> [--json]")
	fmt.Println("  dendrite store commit --project <name> --chunk <id> --file <path> [--final]")
	fmt.Println("  dendrite store purge --project <name>")
}
