package planetools

import (
	"Dendrite/pkg/directive"
	. "Dendrite/pkg/mcpserver"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/export"
	"Dendrite/pkg/rea/insight"
	"Dendrite/pkg/rea/ownership"
	"context"
	"errors"
)

// RegisterReaAnalysisTools registers the REA analysis tools, the MCP side of `dendrite retro analyze|pause|resume|
// purge|search`, so AGY and any other agent can run an analysis directly. service builds the analysis service.
func RegisterReaAnalysisTools(s *Server, deps Dependencies, service func() (*analysis.Service, error)) {
	idProp := TypedProp("string", "Analysis id, as printed by dendrite_rea_analyze")
	tierProp := TypedProp("string", "Model tier: local or paid")
	modelProp := TypedProp("string", "Model to use on that tier")
	engineProps := map[string]interface{}{
		"engine":  TypedProp("string", "Engine to run over the target: auto (best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive)"),
		"sandbox": TypedProp("string", "Where the engine runs: container (default) or host"),
		"trusted": TypedProp("boolean", "The target is safe to open on the host (needed for a binary with sandbox=host)"),
	}
	registerReaDefs(s, service, []reaToolDef{
		{"dendrite_rea_analyze", "Analyze any target (PE, ELF, Mach-O, ASAR/ZIP package, JS bundle, unknown file or a directory of source): inspect it, write its structure as documents and index them. Returns an evidence bundle under 4,000 tokens and an analysis id; read more with dendrite_rea_search instead of dumping the target again. A paused analysis is returned with paused=true.",
			[]string{"binary"}, withProps(engineProps, map[string]interface{}{"binary": TypedProp("string", "Path of the file or directory to analyze"), "format": TypedProp("string", "Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree, unknown"),
				"id": TypedProp("string", "Name of the analysis (default: generated)"), "tier": tierProp, "model": modelProp,
				"ticket": TypedProp("string", "Leave a note with the summary and the analysis id on this ticket")}), reaAnalyze(deps)},
		{"dendrite_rea_engines", "List the reverse-engineering engines (open-source: rizin, radare2, objdump, Ghidra; proprietary and documented only: Hopper, IDA, Binary Ninja) with their license, capabilities and whether each is usable in the container or on the host here.",
			nil, map[string]interface{}{}, reaEngines},
		{"dendrite_rea_pause", "Pause an analysis at its next stage boundary; its checkpoint is kept.", []string{"id"}, map[string]interface{}{"id": idProp}, reaPause},
		{"dendrite_rea_resume", "Continue an analysis from its last checkpoint, optionally on another model tier or model.", []string{"id"},
			withProps(engineProps, map[string]interface{}{"id": idProp, "tier": tierProp, "model": modelProp}), reaResume},
		{"dendrite_rea_purge", "Delete an analysis: its documents, its index and its checkpoint. Needs force=true.", []string{"id", "force"},
			map[string]interface{}{"id": idProp, "force": TypedProp("boolean", "Confirm the deletion")}, reaPurge},
		{"dendrite_rea_search", "Search the documents of a finished analysis; returns only the best passages, within the 4,000-token boundary.", []string{"id", "query"},
			map[string]interface{}{"id": idProp, "query": TypedProp("string", "What to look for, e.g. an imported function"), "k": TypedProp("number", "How many passages (default 5, at most 20)")}, reaSearch},
	})
}

// reaToolDef defines one REA tool: its name, description, required arguments, argument schema and operation.
type reaToolDef struct {
	name, desc string
	required   []string
	props      map[string]interface{}
	run        func(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error)
}

// registerReaDefs registers each definition on s, over the analysis service that service builds.
func registerReaDefs(s *Server, service func() (*analysis.Service, error), defs []reaToolDef) {
	for _, t := range defs {
		s.RegisterTool(Tool{Name: t.name, Description: t.desc, InputSchema: ObjectSchema(t.required, t.props)}, reaTool(service, t.required, t.run))
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

// withProps is base with the extra properties added.
func withProps(extra, base map[string]interface{}) map[string]interface{} {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func reaOptions(args map[string]interface{}) analysis.Options {
	trusted, _ := args["trusted"].(bool)
	return analysis.Options{ID: stringArg(args, "id"), Format: stringArg(args, "format"), Tier: stringArg(args, "tier"), Model: stringArg(args, "model"),
		Engine: stringArg(args, "engine"), Sandbox: stringArg(args, "sandbox"), Trusted: trusted}
}

// reaEngineDefaults fills the engine and sandbox of a new analysis from the environment and the [rea] directive of
// the server's project; a directive that cannot be read leaves the defaults.
func reaEngineDefaults(o analysis.Options) analysis.Options {
	set, err := ownership.LoadRule(".")
	if err != nil {
		set = directive.Defaults()
	}
	return o.WithEngineDefaults(set.Rea)
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

// analyzed is an analysis result with what happened to the ticket note, when one was asked for.
type analyzed struct {
	*analysis.Result
	TicketNote string `json:"ticket_note"`
}

// reaAnalyze runs an analysis and, for a finished one with a ticket argument, leaves a note on the ticket. A note
// that cannot be posted is reported in the result and never fails the analysis.
func reaAnalyze(deps Dependencies) func(context.Context, *analysis.Service, map[string]interface{}) (interface{}, error) {
	return func(ctx context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
		res, err := svc.Analyze(ctx, stringArg(args, "binary"), reaEngineDefaults(reaOptions(args)))
		out, err := reaOutcome(res, err)
		ticket := stringArg(args, "ticket")
		if err != nil || ticket == "" || res.State.Stage != analysis.StageDone {
			return out, err
		}
		return analyzed{Result: res, TicketNote: postTicketNote(ctx, deps, args, ticket, res)}, nil
	}
}

func postTicketNote(ctx context.Context, deps Dependencies, args map[string]interface{}, ticket string, res *analysis.Result) string {
	l, err := lifecycleFor(deps, args)
	if err == nil {
		err = insight.PostNote(ctx, l, ticket, res)
	}
	if err != nil {
		return "not posted: " + err.Error()
	}
	return "posted"
}

func reaEngines(ctx context.Context, _ *analysis.Service, _ map[string]interface{}) (interface{}, error) {
	return engine.Statuses(ctx, engine.NewService().Env), nil
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

// RegisterReaInsightTools registers the tools that put a finished analysis to use: dendrite_rea_reuse,
// dendrite_rea_rank and dendrite_rea_adr, the MCP side of `dendrite retro reuse|rank|adr`.
func RegisterReaInsightTools(s *Server, service func() (*analysis.Service, error)) {
	idProp := TypedProp("string", "Analysis id, as printed by dendrite_rea_analyze")
	registerReaDefs(s, service, []reaToolDef{
		{"dendrite_rea_reuse", "Find code in the project and its synapse that already does what an analysis found (exports, strings, non-system dependencies); ranked passages under 4,000 tokens. Use it before writing cleanroom code.",
			[]string{"id"}, map[string]interface{}{"id": idProp, "project": TypedProp("string", "Project directory to search (default: the server's directory)"),
				"synapse": TypedProp("string", "Synapse to search (default: the one the project belongs to)"), "k": TypedProp("number", "How many passages (default 10)")}, reaReuse},
		{"dendrite_rea_rank", "Say which documents of an analysis to read first: the overview, then the most connected ones.", []string{"id"}, map[string]interface{}{"id": idProp}, reaRank},
		{"dendrite_rea_adr", "Draft a proposed Architecture Decision Record from a finished analysis (next free ADR number; the decision and consequences are left for a person).",
			[]string{"id"}, map[string]interface{}{"id": idProp, "title": TypedProp("string", "Title of the record"), "output": TypedProp("string", "Directory of decision records (default docs/architecture/decisions)")}, reaADR},
		{"dendrite_rea_export", "Write a finished analysis of any format (PE, ELF, Mach-O, package, source tree, ...) as a data module in rust, go, python or typescript under output: sections, libraries, exports, files, notes and the functions an engine found.",
			[]string{"id", "output"}, map[string]interface{}{"id": idProp, "language": TypedProp("string", "rust (default), go, python or typescript"), "output": TypedProp("string", "Directory to write <id>.<ext> into")}, reaExport},
	})
}

func reaExport(_ context.Context, svc *analysis.Service, args map[string]interface{}) (interface{}, error) {
	language := orDefaultArg(stringArg(args, "language"), "rust")
	path, err := export.Write(svc, stringArg(args, "id"), language, stringArg(args, "output"))
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": stringArg(args, "id"), "language": language, "file": path}, nil
}

func orDefaultArg(v, def string) string {
	if v == "" {
		return def
	}
	return v
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
