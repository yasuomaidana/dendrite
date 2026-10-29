package mcpserver

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// ProbeSpec is one read-only tool call used to check that a tool answers.
type ProbeSpec struct {
	Tool    string
	Args    map[string]any
	Timeout time.Duration // zero means the probe default
}

// ProbeResult is the outcome of one ProbeSpec.
type ProbeResult struct {
	Tool     string
	OK       bool
	Skipped  bool
	Duration time.Duration
	Detail   string
}

// DefaultProbeTimeout bounds every probe that does not set its own.
const DefaultProbeTimeout = 5 * time.Second

// DefaultProbeSpecs are the tools that only read: they never create, update or delete tickets,
// files, indexes, services or remote state. Tools that write or reconfigure (for example
// dendrite_plane_issue_create, dendrite_plane_push_status, dendrite_plane_doc_sync,
// dendrite_ticket_update, dendrite_store_rm, dendrite_service_move, dendrite_ui_penpot_sync)
// are deliberately absent and must be judged from their own tests, never by calling them.
func DefaultProbeSpecs() []ProbeSpec {
	return []ProbeSpec{
		{Tool: "dendrite_rag_retrieve", Args: map[string]any{"query": "register mcp tools", "detail": "map"}, Timeout: 15 * time.Second},
		{Tool: "dendrite_rag_ask", Args: map[string]any{"question": "what does RegisterDefaultTools do?"}, Timeout: 120 * time.Second},
		{Tool: "dendrite_graph_communities", Args: map[string]any{"limit": 3}, Timeout: 30 * time.Second},
		{Tool: "dendrite_graph_god_nodes", Args: map[string]any{"limit": 3}, Timeout: 30 * time.Second},
		{Tool: "dendrite_graph_explain", Args: map[string]any{"target": "RegisterDefaultTools"}, Timeout: 30 * time.Second},
		{Tool: "dendrite_graph_path", Args: map[string]any{"from": "RegisterDefaultTools", "to": "registerRAGTools"}, Timeout: 30 * time.Second},
		{Tool: "dendrite_plane_issues"},
		{Tool: "dendrite_plane_ticket_smart_select", Timeout: 20 * time.Second},
		{Tool: "dendrite_plane_replicate", Args: map[string]any{"dry_run": true}, Timeout: 20 * time.Second},
		{Tool: "dendrite_ticket_smart_select", Timeout: 20 * time.Second},
		{Tool: "dendrite_ticket_list"},
		{Tool: "dendrite_hitl_sync", Args: map[string]any{"dry_run": true}, Timeout: 30 * time.Second},
		{Tool: "dendrite_pool_list"},
		{Tool: "dendrite_ref_list"},
		{Tool: "dendrite_dist_sources"},
		{Tool: "dendrite_env_status"},
		{Tool: "dendrite_inference_list"},
		{Tool: "dendrite_inference_models"},
		{Tool: "dendrite_store_status"},
		{Tool: "dendrite_tokens_status"},
		{Tool: "dendrite_analyze_status"},
		{Tool: "dendrite_telemetry"},
	}
}

// ProbeTools calls every spec against srv sequentially. A tool the server does not register is
// reported as skipped (it may be outside the active profile), not as a failure.
func ProbeTools(ctx context.Context, srv *Server, specs []ProbeSpec) []ProbeResult {
	results := make([]ProbeResult, 0, len(specs))
	for _, spec := range specs {
		h, ok := srv.Handler(spec.Tool)
		if !ok {
			results = append(results, ProbeResult{Tool: spec.Tool, Skipped: true, Detail: "not registered"})
			continue
		}
		timeout := spec.Timeout
		if timeout <= 0 {
			timeout = DefaultProbeTimeout
		}
		results = append(results, probeOne(ctx, spec, h, timeout))
	}
	return results
}

func probeOne(ctx context.Context, spec ProbeSpec, h ToolHandler, timeout time.Duration) ProbeResult {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := spec.Args
	if args == nil {
		args = map[string]any{}
	}
	start := time.Now()
	type outcome struct {
		res *CallToolResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- outcome{err: fmt.Errorf("panic: %v", r)}
			}
		}()
		res, err := h(cctx, args)
		done <- outcome{res, err}
	}()
	pr := ProbeResult{Tool: spec.Tool}
	select {
	case o := <-done:
		pr.Duration = time.Since(start)
		switch {
		case o.err != nil:
			pr.Detail = firstLine(o.err.Error())
		case o.res == nil:
			pr.Detail = "nil result"
		case o.res.IsError:
			pr.Detail = firstLine(contentText(o.res))
		default:
			pr.OK = true
		}
	case <-cctx.Done():
		pr.Duration = time.Since(start)
		pr.Detail = fmt.Sprintf("timeout after %s", timeout)
	}
	return pr
}

func contentText(r *CallToolResult) string {
	var sb strings.Builder
	for _, c := range r.Content {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// WriteProbeMatrix prints the pass/fail matrix and returns the number of failures.
func WriteProbeMatrix(w io.Writer, results []ProbeResult) int {
	sorted := append([]ProbeResult(nil), results...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Tool < sorted[j].Tool })
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fails := 0
	for _, r := range sorted {
		status := "PASS"
		switch {
		case r.Skipped:
			status = "SKIP"
		case !r.OK:
			status = "FAIL"
			fails++
		}
		fmt.Fprintf(tw, "%s\t%s\t%dms\t%s\n", status, r.Tool, r.Duration.Milliseconds(), r.Detail)
	}
	tw.Flush()
	return fails
}
