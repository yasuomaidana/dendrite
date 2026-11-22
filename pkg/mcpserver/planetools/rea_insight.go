package planetools

import (
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/insight"
	"context"

	. "Dendrite/pkg/mcpserver"
)

// RegisterReaInsightTools registers the tools that put a finished analysis to use: dendrite_rea_reuse,
// dendrite_rea_rank and dendrite_rea_adr, the MCP side of `dendrite retro reuse|rank|adr`.
func RegisterReaInsightTools(s *Server, service func() (*analysis.Service, error)) {
	idProp := TypedProp("string", "Analysis id, as printed by dendrite_rea_analyze")
	tools := []struct {
		name, desc string
		required   []string
		props      map[string]interface{}
		run        func(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error)
	}{
		{"dendrite_rea_reuse", "Find code in the project and its synapse that already does what an analysis found (exports, strings, non-system dependencies); ranked passages under 4,000 tokens. Use it before writing cleanroom code.",
			[]string{"id"}, map[string]interface{}{"id": idProp, "project": TypedProp("string", "Project directory to search (default: the server's directory)"),
				"synapse": TypedProp("string", "Synapse to search (default: the one the project belongs to)"), "k": TypedProp("number", "How many passages (default 10)")}, reaReuse},
		{"dendrite_rea_rank", "Say which documents of an analysis to read first: the overview, then the most connected ones.", []string{"id"}, map[string]interface{}{"id": idProp}, reaRank},
		{"dendrite_rea_adr", "Draft a proposed Architecture Decision Record from a finished analysis (next free ADR number; the decision and consequences are left for a person).",
			[]string{"id"}, map[string]interface{}{"id": idProp, "title": TypedProp("string", "Title of the record"), "output": TypedProp("string", "Directory of decision records (default docs/architecture/decisions)")}, reaADR},
	}
	for _, t := range tools {
		s.RegisterTool(Tool{Name: t.name, Description: t.desc, InputSchema: ObjectSchema(t.required, t.props)}, reaTool(service, t.required, t.run))
	}
}

func reaReuse(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	k, _ := args["k"].(float64)
	return insight.Reuse(ctx, svc, stringArg(args, "id"), insight.ReuseOptions{Project: stringArg(args, "project"), Synapse: stringArg(args, "synapse"), K: int(k)})
}

func reaRank(_ context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	st, img, err := svc.Inspected(stringArg(args, "id"))
	if err != nil {
		return nil, err
	}
	return insight.Rank(st, img), nil
}

func reaADR(_ context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	path, err := insight.DraftADR(svc, stringArg(args, "id"), stringArg(args, "output"), stringArg(args, "title"))
	if err != nil {
		return nil, err
	}
	return map[string]string{"adr": path, "status": "proposed"}, nil
}
