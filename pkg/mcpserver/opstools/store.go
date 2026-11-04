package opstools

import (
	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
	"context"
	"fmt"
	"time"

	. "Dendrite/pkg/mcpserver"
)

var DefaultMCPStoreInspector = status.NewStoreInspector(DefaultMCPStore)

var DefaultMCPStore, _ = segmented.NewSegmentedStore("")

func registerStoreTools(s *Server, deps Dependencies) {
	if DefaultMCPStore == nil {
		return
	}

	// 38. dendrite_store_status
	AddTool(s, "dendrite_store_status", "Query real-time storage status, segments count, and checkpoint states of the persistent segmented analysis store", PropsSchema(map[string]interface{}{
		"project": TypedProp("string", "Optional project identifier filter"),
	}), storeStatusHandler())

	// 39. dendrite_store_commit_chunk
	s.RegisterTool(Tool{
		Name:        "dendrite_store_commit_chunk",
		Description: "Atomically commit a chunk of analyzed segments and compressed blobs to the persistent store",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":    TypedProp("string", "Project name or ID"),
				"chunk_id":   TypedProp("string", "Unique chunk identifier"),
				"identifier": TypedProp("string", "Symbol, function, or file identifier"),
				"data":       TypedProp("string", "Raw text/code payload for the segment"),
				"type":       map[string]interface{}{"type": "string", "enum": []string{"file", "function", "cfg_block", "ast_graph"}, "description": "Segment type"},
				"is_final":   TypedProp("boolean", "Set true if this is the final completed chunk (StateStored)"),
			},
			"required": []interface{}{"project", "chunk_id", "identifier", "data"},
		},
	}, storeCommitChunkHandler())

	// 40. dendrite_store_query_partial
	s.RegisterTool(Tool{
		Name:        "dendrite_store_query_partial",
		Description: "Query committed segments for a project, including projects currently in ANALYZING_PARTIAL state",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": TypedProp("string", "Project identifier"),
			},
			"required": []interface{}{"project"},
		},
	}, storeQueryPartialHandler())
	// 41. dendrite_store_inspect
	s.RegisterTool(Tool{
		Name:        "dendrite_store_inspect",
		Description: "Detailed chunk-by-chunk and segment analysis breakdown for a specific stored project",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"store_id": TypedProp("string", "Project / Store identifier"),
			},
			"required": []interface{}{"store_id"},
		},
	}, storeInspectHandler())

	// 42. dendrite_store_rm
	s.RegisterTool(Tool{
		Name:        "dendrite_store_rm",
		Description: "Safely removes SQLite analysis records and unlinks compressed blobs with confirmation gate",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"store_id": TypedProp("string", "Project / Store identifier to delete"),
				"force":    TypedProp("boolean", "Mandatory confirmation gate to perform permanent deletion"),
			},
			"required": []interface{}{"store_id"},
		},
	}, storeRmHandler())
}

// storeStatusHandler handles the dendrite_store_status tool.
func storeStatusHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		project, _ := args["project"].(string)

		segs, err := DefaultMCPStore.QueryPartial(ctx, project)
		if err != nil {
			return nil, fmt.Errorf("querying segments: %w", err)
		}

		cps, err := DefaultMCPStore.ListCheckpoints(ctx, project)
		if err != nil {
			return nil, fmt.Errorf("listing checkpoints: %w", err)
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
		for _, seg := range segs {
			totalRawBytes += seg.SizeBytes
			totalCompBytes += seg.CompressedSize
		}

		res := map[string]interface{}{
			"project":           project,
			"total_segments":    len(segs),
			"total_checkpoints": len(cps),
			"stored_chunks":     storedCount,
			"partial_chunks":    partialCount,
			"raw_bytes":         totalRawBytes,
			"compressed_bytes":  totalCompBytes,
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// storeCommitChunkHandler handles the dendrite_store_commit_chunk tool.
func storeCommitChunkHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		project, _ := args["project"].(string)
		chunkID, _ := args["chunk_id"].(string)
		ident, _ := args["identifier"].(string)
		dataStr, _ := args["data"].(string)
		segTypeStr, _ := args["type"].(string)
		isFinal, _ := args["is_final"].(bool)

		if segTypeStr == "" {
			segTypeStr = "file"
		}

		data := []byte(dataStr)
		bs := segmented.NewBlobStore("")
		hash := bs.ComputeHash(data)

		seg := segmented.Segment{
			Hash:       hash,
			Type:       segmented.SegmentType(segTypeStr),
			Identifier: ident,
			Project:    project,
			SizeBytes:  int64(len(data)),
			CreatedAt:  time.Now().UTC(),
		}

		cp, err := DefaultMCPStore.CommitChunk(ctx, project, chunkID, []segmented.Segment{seg}, map[string][]byte{hash: data}, isFinal)
		if err != nil {
			return ErrorResult("[-] Failed to commit chunk: %v", err), nil
		}

		dataRes := IndentJSON(cp)
		return TextResult(string(dataRes)), nil
	}
}

// storeQueryPartialHandler handles the dendrite_store_query_partial tool.
func storeQueryPartialHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		project, _ := args["project"].(string)

		segs, err := DefaultMCPStore.QueryPartial(ctx, project)
		if err != nil {
			return nil, fmt.Errorf("querying partial segments: %w", err)
		}

		data := IndentJSON(segs)
		return TextResult(string(data)), nil
	}
}

// storeInspectHandler handles the dendrite_store_inspect tool.
func storeInspectHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storeID, _ := args["store_id"].(string)
		details, err := DefaultMCPStoreInspector.InspectStoreDetails(ctx, storeID)
		if err != nil {
			return ErrorResult("[-] Inspect failed: %v", err), nil
		}

		data := IndentJSON(details)
		return TextResult(string(data)), nil
	}
}

// storeRmHandler handles the dendrite_store_rm tool.
func storeRmHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storeID, _ := args["store_id"].(string)
		force, _ := args["force"].(bool)

		res, err := DefaultMCPStoreInspector.RemoveStore(ctx, storeID, force)
		if err != nil {
			return ErrorResult("[-] Removal failed: %v", err), nil
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}
