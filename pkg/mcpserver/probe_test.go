package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProbeToolsClassifiesOutcomes(t *testing.T) {
	s := NewServer(ServerConfig{})
	reg := func(name string, h ToolHandler) { s.RegisterTool(Tool{Name: name}, h) }
	reg("ok", func(context.Context, map[string]any) (*CallToolResult, error) {
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: "fine"}}}, nil
	})
	reg("err", func(context.Context, map[string]any) (*CallToolResult, error) {
		return nil, errors.New("plane unreachable\nstack")
	})
	reg("tool-error", func(context.Context, map[string]any) (*CallToolResult, error) {
		return &CallToolResult{IsError: true, Content: []ContentItem{{Type: "text", Text: "bad input"}}}, nil
	})
	reg("slow", func(ctx context.Context, _ map[string]any) (*CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	reg("panics", func(context.Context, map[string]any) (*CallToolResult, error) { panic("boom") })

	got := ProbeTools(context.Background(), s, []ProbeSpec{
		{Tool: "ok"}, {Tool: "err"}, {Tool: "tool-error"},
		{Tool: "slow", Timeout: 20 * time.Millisecond}, {Tool: "panics"}, {Tool: "missing"},
	})
	want := map[string][2]any{ // ok, skipped
		"ok": {true, false}, "err": {false, false}, "tool-error": {false, false},
		"slow": {false, false}, "panics": {false, false}, "missing": {false, true},
	}
	for _, r := range got {
		w := want[r.Tool]
		if r.OK != w[0] || r.Skipped != w[1] {
			t.Errorf("%s = ok:%v skipped:%v (%s)", r.Tool, r.OK, r.Skipped, r.Detail)
		}
	}
	var buf bytes.Buffer
	if fails := WriteProbeMatrix(&buf, got); fails != 4 {
		t.Errorf("fails = %d\n%s", fails, buf.String())
	}
	for _, s := range []string{"PASS", "FAIL", "SKIP", "plane unreachable", "timeout after"} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("matrix lacks %q:\n%s", s, buf.String())
		}
	}
}

func TestDefaultProbeSpecsOnlyReadOnlyRegisteredTools(t *testing.T) {
	src := newDefaultServerForProfileTest()
	writers := []string{"issue_create", "push_status", "doc_sync", "ticket_update", "store_rm", "service_move", "penpot_sync", "relation_link", "commit"}
	for _, spec := range DefaultProbeSpecs() {
		if _, ok := src.Handler(spec.Tool); !ok {
			t.Errorf("probe spec %s is not a registered tool", spec.Tool)
		}
		for _, w := range writers {
			if strings.Contains(spec.Tool, w) {
				t.Errorf("probe spec %s is a writer", spec.Tool)
			}
		}
	}
}
