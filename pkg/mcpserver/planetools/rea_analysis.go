package planetools

import (
	"Dendrite/pkg/rea/analysis"
	"context"
	"errors"

	. "Dendrite/pkg/mcpserver"
)

// RegisterReaAnalysisTools registers the REA analysis tools, the MCP side of `dendrite retro analyze|pause|resume|
// purge|search`, so AGY and any other agent can run an analysis directly. service builds the analysis service.
func RegisterReaAnalysisTools(s *Server, service func() (*analysis.Service, error)) {
	idProp := TypedProp("string", "Analysis id, as printed by dendrite_rea_analyze")
	tierProp := TypedProp("string", "Model tier: local or paid")
	modelProp := TypedProp("string", "Model to use on that tier")
	tools := []struct {
		name, desc string
		required   []string
		props      map[string]interface{}
		run        func(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error)
	}{
		{"dendrite_rea_analyze", "Analyze a Windows PE image: parse it, write its structure as documents and index them. Returns an evidence bundle under 4,000 tokens and an analysis id; read more with dendrite_rea_search instead of dumping the image again. A paused analysis is returned with paused=true.",
			[]string{"binary"}, map[string]interface{}{"binary": TypedProp("string", "Path of the PE image"), "format": TypedProp("string", "pe32 or pe64 (default pe64)"),
				"id": TypedProp("string", "Name of the analysis (default: generated)"), "tier": tierProp, "model": modelProp}, reaAnalyze},
		{"dendrite_rea_pause", "Pause an analysis at its next stage boundary; its checkpoint is kept.", []string{"id"}, map[string]interface{}{"id": idProp}, reaPause},
		{"dendrite_rea_resume", "Continue an analysis from its last checkpoint, optionally on another model tier or model.", []string{"id"},
			map[string]interface{}{"id": idProp, "tier": tierProp, "model": modelProp}, reaResume},
		{"dendrite_rea_purge", "Delete an analysis: its documents, its index and its checkpoint. Needs force=true.", []string{"id", "force"},
			map[string]interface{}{"id": idProp, "force": TypedProp("boolean", "Confirm the deletion")}, reaPurge},
		{"dendrite_rea_search", "Search the documents of a finished analysis; returns only the best passages, within the 4,000-token boundary.", []string{"id", "query"},
			map[string]interface{}{"id": idProp, "query": TypedProp("string", "What to look for, e.g. an imported function"), "k": TypedProp("number", "How many passages (default 5, at most 20)")}, reaSearch},
	}
	for _, t := range tools {
		s.RegisterTool(Tool{Name: t.name, Description: t.desc, InputSchema: ObjectSchema(t.required, t.props)},
			reaTool(service, t.required, t.run))
	}
}

// reaTool wraps an analysis operation: it checks the required arguments, builds the service and answers with JSON.
func reaTool(service func() (*analysis.Service, error), required []string, run func(context.Context, *analysis.Service, map[string]interface{}) (interface{}, error)) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		for _, key := range required {
			if _, isBool := args[key].(bool); !isBool && stringArg(args, key) == "" {
				return nil, errors.New(key + " is required")
			}
		}
		svc, err := service()
		if err != nil {
			return nil, err
		}
		out, err := run(ctx, svc, args)
		if err != nil {
			return nil, err
		}
		return JsonResult(out)
	}
}

func reaOptions(args map[string]interface{}) analysis.Options {
	return analysis.Options{ID: stringArg(args, "id"), Format: stringArg(args, "format"), Tier: stringArg(args, "tier"), Model: stringArg(args, "model")}
}

// reaOutcome returns the result of a run; a pause the caller asked for is a result (paused=true in its state).
func reaOutcome(res *analysis.Result, err error) (interface{}, error) {
	if errors.Is(err, analysis.ErrPaused) {
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func reaAnalyze(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	res, err := svc.Analyze(ctx, stringArg(args, "binary"), reaOptions(args))
	return reaOutcome(res, err)
}

func reaPause(_ context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	return svc.Pause(stringArg(args, "id"))
}

func reaResume(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	res, err := svc.Resume(ctx, stringArg(args, "id"), reaOptions(args))
	return reaOutcome(res, err)
}

func reaPurge(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	force, _ := args["force"].(bool)
	if err := svc.Purge(ctx, stringArg(args, "id"), force); err != nil {
		return nil, err
	}
	return map[string]string{"purged": stringArg(args, "id")}, nil
}

func reaSearch(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	k, _ := args["k"].(float64)
	return svc.Search(ctx, stringArg(args, "id"), stringArg(args, "query"), int(k))
}
