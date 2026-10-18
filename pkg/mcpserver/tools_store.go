package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

var defaultMCPStoreInspector = status.NewStoreInspector(defaultMCPStore)

var defaultMCPStore, _ = segmented.NewSegmentedStore("")

func registerStoreTools(s *Server, deps Dependencies) {
	if defaultMCPStore == nil {
		return
	}

	// 38. dendrite_store_status
	s.RegisterTool(Tool{
		Name:        "dendrite_store_status",
		Description: "Query real-time storage status, segments count, and checkpoint states of the persistent segmented analysis store",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string", "description": "Optional project identifier filter"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		project, _ := args["project"].(string)

		segs, err := defaultMCPStore.QueryPartial(ctx, project)
		if err != nil {
			return nil, fmt.Errorf("querying segments: %w", err)
		}

		cps, err := defaultMCPStore.ListCheckpoints(ctx, project)
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

		data, _ := json.MarshalIndent(res, "", "  ")
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})

	// 39. dendrite_store_commit_chunk
	s.RegisterTool(Tool{
		Name:        "dendrite_store_commit_chunk",
		Description: "Atomically commit a chunk of analyzed segments and compressed blobs to the persistent store",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":    map[string]interface{}{"type": "string", "description": "Project name or ID"},
				"chunk_id":   map[string]interface{}{"type": "string", "description": "Unique chunk identifier"},
				"identifier": map[string]interface{}{"type": "string", "description": "Symbol, function, or file identifier"},
				"data":       map[string]interface{}{"type": "string", "description": "Raw text/code payload for the segment"},
				"type":       map[string]interface{}{"type": "string", "enum": []string{"file", "function", "cfg_block", "ast_graph"}, "description": "Segment type"},
				"is_final":   map[string]interface{}{"type": "boolean", "description": "Set true if this is the final completed chunk (StateStored)"},
			},
			"required": []interface{}{"project", "chunk_id", "identifier", "data"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		cp, err := defaultMCPStore.CommitChunk(ctx, project, chunkID, []segmented.Segment{seg}, map[string][]byte{hash: data}, isFinal)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Failed to commit chunk: %v", err),
				}},
				IsError: true,
			}, nil
		}

		dataRes, _ := json.MarshalIndent(cp, "", "  ")
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(dataRes)}}}, nil
	})

	// 40. dendrite_store_query_partial
	s.RegisterTool(Tool{
		Name:        "dendrite_store_query_partial",
		Description: "Query committed segments for a project, including projects currently in ANALYZING_PARTIAL state",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{"type": "string", "description": "Project identifier"},
			},
			"required": []interface{}{"project"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		project, _ := args["project"].(string)

		segs, err := defaultMCPStore.QueryPartial(ctx, project)
		if err != nil {
			return nil, fmt.Errorf("querying partial segments: %w", err)
		}

		data, _ := json.MarshalIndent(segs, "", "  ")
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})
	// 41. dendrite_store_inspect
	s.RegisterTool(Tool{
		Name:        "dendrite_store_inspect",
		Description: "Detailed chunk-by-chunk and segment analysis breakdown for a specific stored project",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"store_id": map[string]interface{}{"type": "string", "description": "Project / Store identifier"},
			},
			"required": []interface{}{"store_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storeID, _ := args["store_id"].(string)
		details, err := defaultMCPStoreInspector.InspectStoreDetails(ctx, storeID)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Inspect failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling inspect result: %w", err)
		}
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})

	// 42. dendrite_store_rm
	s.RegisterTool(Tool{
		Name:        "dendrite_store_rm",
		Description: "Safely removes SQLite analysis records and unlinks compressed blobs with confirmation gate",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"store_id": map[string]interface{}{"type": "string", "description": "Project / Store identifier to delete"},
				"force":    map[string]interface{}{"type": "boolean", "description": "Mandatory confirmation gate to perform permanent deletion"},
			},
			"required": []interface{}{"store_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		storeID, _ := args["store_id"].(string)
		force, _ := args["force"].(bool)

		res, err := defaultMCPStoreInspector.RemoveStore(ctx, storeID, force)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Removal failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling removal result: %w", err)
		}
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})
}
