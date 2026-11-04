package opstools

import (
	"Dendrite/pkg/agent/triage"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/personalcontext/greeting"
	"Dendrite/pkg/rea/showcase"
	"context"
	"fmt"
	"strings"

	. "Dendrite/pkg/mcpserver"
	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/polyglot/contract"
)

func registerGreetingTools(s *Server, deps Dependencies) {
	// dendrite_pcontext_greeting_generate
	s.RegisterTool(Tool{
		Name:        "dendrite_pcontext_greeting_generate",
		Description: "Generates a dynamic 10-style personalized headline greeting with strict cliché suppression, sliding history tracking, and journey context resolution",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"topic":              TypedProp("string", "User's current topic or project"),
				"decision":           TypedProp("string", "Decision point user is considering"),
				"milestone":          TypedProp("string", "Milestone title achieved or in-flight"),
				"milestone_achieved": TypedProp("boolean", "Whether the milestone was recently completed"),
				"event":              TypedProp("string", "Upcoming event or deadline"),
				"days_until_event":   TypedProp("integer", "Days remaining until event"),
				"options": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Option alternatives user is weighing (e.g. Option A vs B)",
				},
				"unfinished_step":        TypedProp("string", "Pending task left from prior turn"),
				"specific_challenge":     TypedProp("string", "Complex or stressful blocker"),
				"curiosity_angle":        TypedProp("string", "Novel or unexplored angle for topic"),
				"days_since_last_active": TypedProp("integer", "Days since the user last interacted"),
				"turn_count":             TypedProp("integer", "Number of turns in current conversation"),
				"high_stress":            TypedProp("boolean", "Whether user context indicates high stress"),
				"low_pressure_hobby":     TypedProp("boolean", "Whether topic is a low-stakes creative hobby"),
				"recent_styles": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Recently generated style IDs to suppress (e.g. ['G-01', 'G-02'])",
				},
			},
		},
	}, pcontextGreetingGenerateHandler())
}

// pcontextGreetingGenerateHandler handles the dendrite_pcontext_greeting_generate tool.
func pcontextGreetingGenerateHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		var recentStyles []greeting.StyleID
		for _, s := range StringList(args["recent_styles"]) {
			recentStyles = append(recentStyles, greeting.StyleID(s))
		}
		res, err := greeting.NewEngine().Generate(ctx, journeyContext(args), recentStyles)
		if err != nil {
			return ErrorResult("generate greeting: %v", err), nil
		}
		return TextResult(string(IndentJSON(res))), nil
	}
}

// journeyContext reads the user-journey fields a greeting tool call carries.
func journeyContext(args map[string]interface{}) greeting.UserJourneyContext {
	u := greeting.UserJourneyContext{Options: StringList(args["options"])}
	for key, dst := range map[string]*string{
		"topic": &u.Topic, "decision": &u.Decision, "milestone": &u.Milestone, "event": &u.Event,
		"unfinished_step": &u.UnfinishedStep, "specific_challenge": &u.SpecificChallenge, "curiosity_angle": &u.CuriosityAngle,
	} {
		setString(args, key, dst)
	}
	for key, dst := range map[string]*bool{
		"milestone_achieved": &u.MilestoneAchieved, "high_stress": &u.HighStress, "low_pressure_hobby": &u.LowPressureHobby,
	} {
		setBool(args, key, dst)
	}
	for key, dst := range map[string]*int{
		"days_until_event": &u.DaysUntilEvent, "days_since_last_active": &u.DaysSinceLastActive, "turn_count": &u.TurnCount,
	} {
		setInt(args, key, dst)
	}
	return u
}

func setString(args map[string]interface{}, key string, dst *string) {
	if v, ok := args[key].(string); ok {
		*dst = v
	}
}

func setBool(args map[string]interface{}, key string, dst *bool) {
	if v, ok := args[key].(bool); ok {
		*dst = v
	}
}

func setInt(args map[string]interface{}, key string, dst *int) {
	if v, ok := args[key].(float64); ok {
		*dst = int(v)
	}
}

func registerTriageTools(s *Server, deps Dependencies) {
	// 23. dendrite_local_agent_triage
	s.RegisterTool(Tool{
		Name:        "dendrite_local_agent_triage",
		Description: "Intercepts developer prompts, evaluates local compute across multi-provider inference (Ollama, Ollama Validated, Hugging Face, Candle/OxiLLaMa), routes chores to zero-cost local models, and enhances/compacts prompts with project skills for cloud escalation",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"prompt"},
			"properties": map[string]interface{}{
				"prompt":        TypedProp("string", "Developer prompt or chat message"),
				"active_file":   TypedProp("string", "Optional active file path in the IDE editor"),
				"file_content":  TypedProp("string", "Optional source code content of active file"),
				"force_tier":    TypedProp("string", "Optional tier override: 'local', 'hybrid', 'cloud'"),
				"target_engine": TypedProp("string", "Target inference backend: 'ollama', 'ollama-validated', 'huggingface', 'candle', 'oxillama'"),
				"execute_local": TypedProp("boolean", "Execute chore tasks locally if local inference is available"),
			},
		},
	}, localAgentTriageHandler(deps))
}

// triageTiers and triageEngines name the tiers and engines a caller may force; others mean "decide".
var (
	triageTiers = map[string]triage.ExecutionTier{
		"local": triage.TierLocal, "hybrid": triage.TierHybrid, "cloud": triage.TierCloud,
	}
	triageEngines = map[string]triage.SupportedEngine{
		"ollama": triage.EngineOllama, "ollama-validated": triage.EngineOllamaValidated,
		"huggingface": triage.EngineHuggingFace, "hf": triage.EngineHuggingFace,
		"candle": triage.EngineCandle, "oxillama": triage.EngineOxiLLaMa,
	}
)

// localAgentTriageHandler handles the dendrite_local_agent_triage tool.
func localAgentTriageHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("prompt is required")
		}
		activeFile, _ := args["active_file"].(string)
		fileContent, _ := args["file_content"].(string)
		forceTierStr, _ := args["force_tier"].(string)
		targetEngineStr, _ := args["target_engine"].(string)
		executeLocal, _ := args["execute_local"].(bool)

		forceTier := triageTiers[forceTierStr]
		targetEngine := triageEngines[targetEngineStr]

		var ollamaClient *ollama.Client
		if deps.OllamaURL != "" {
			ollamaClient = ollama.NewClient(ollama.WithBaseURL(deps.OllamaURL))
		}
		engine := triage.NewEngine(ollamaClient)

		req := triage.TriageRequest{
			Prompt:       prompt,
			ActiveFile:   activeFile,
			FileContent:  fileContent,
			ForceTier:    forceTier,
			TargetEngine: targetEngine,
			ExecuteLocal: executeLocal,
		}

		res, _ := engine.Triage(ctx, req) // Triage degrades to a heuristic answer instead of failing

		data := IndentJSON(res)

		return TextResult(string(data)), nil
	}
}

func registerInferenceTools(s *Server, deps Dependencies) {
	// dendrite_inference_migrate_model
	s.RegisterTool(Tool{
		Name:        "dendrite_inference_migrate_model",
		Description: "Migrate models from Ollama to OxiLLaMa pure-Rust inference engine and evict source blobs to reclaim memory",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"model":         TypedProp("string", "Model identifier to migrate (e.g. 'qwen3.5:9b', 'qwen3.8:27b')"),
				"source_engine": TypedProp("string", "Source engine identifier (default: 'ollama')"),
				"target_engine": TypedProp("string", "Target engine identifier (default: 'oxillama')"),
				"delete_source": TypedProp("boolean", "If true, evicts model from source engine upon successful verification (default: false; OxiLLaMa runs prompts on the CPU, so keep the Ollama copy for GPU inference)"),
				"dry_run":       TypedProp("boolean", "If true, simulates migration without modifying disk (default: false)"),
			},
			"required": []interface{}{"model"},
		},
	}, inferenceMigrateModelHandler())
}

// inferenceMigrateModelHandler handles the dendrite_inference_migrate_model tool.
func inferenceMigrateModelHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		model, _ := args["model"].(string)
		if strings.TrimSpace(model) == "" {
			return nil, fmt.Errorf("model name is required")
		}
		sourceEngine := StringArgOr(args, "source_engine", "ollama")
		targetEngine := StringArgOr(args, "target_engine", "oxillama")
		deleteSource := false
		if ds, ok := args["delete_source"].(bool); ok {
			deleteSource = ds
		}
		dryRun, _ := args["dry_run"].(bool)

		migrator := inference.NewModelMigrator("", "", nil)
		res, err := migrator.Migrate(ctx, inference.ModelMigrationOptions{
			ModelName:    model,
			SourceEngine: sourceEngine,
			TargetEngine: targetEngine,
			DeleteSource: deleteSource,
			DryRun:       dryRun,
		})
		if err != nil {
			return nil, err
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

func registerEnvPrepareTools(s *Server, deps Dependencies) {
	// dendrite_env_prepare
	s.RegisterTool(Tool{
		Name:        "dendrite_env_prepare",
		Description: "Prepares or synchronizes project environment, updates agent rules and skills, and enforces CI/CD tooling (default: GitHub Copilot)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": TypedProp("string", "Target workspace project root directory (default: current directory)"),
				"default_tool": TypedProp("string", "Default CI/CD assistant tool (default: 'copilot')"),
				"enforce_cicd": TypedProp("boolean", "Enforce CI/CD practices and GitHub Copilot configuration (default: true)"),
				"dry_run":      TypedProp("boolean", "If true, simulates preparation without writing files"),
			},
		},
	}, envPrepareHandler())
}

// envPrepareHandler handles the dendrite_env_prepare tool.
func envPrepareHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		path := "."
		if p, ok := args["project_path"].(string); ok && strings.TrimSpace(p) != "" {
			path = p
		}
		tool := "copilot"
		if t, ok := args["default_tool"].(string); ok && strings.TrimSpace(t) != "" {
			tool = t
		}
		enforceCICD := true
		if e, ok := args["enforce_cicd"].(bool); ok {
			enforceCICD = e
		}
		dryRun := false
		if d, ok := args["dry_run"].(bool); ok {
			dryRun = d
		}

		prep := bootstrap.NewPreparer()
		report, err := prep.Prepare(ctx, bootstrap.PrepareOptions{
			ProjectPath: path,
			DefaultTool: tool,
			EnforceCICD: enforceCICD,
			SyncSkills:  true,
			DryRun:      dryRun,
		})
		if err != nil {
			return nil, fmt.Errorf("environment preparation failed: %w", err)
		}

		data := IndentJSON(report)
		return TextResult(string(data)), nil
	}
}

func registerREATools(s *Server, deps Dependencies) {
	// 21. dendrite_rea_showcase
	s.RegisterTool(Tool{
		Name:        "dendrite_rea_showcase",
		Description: "Execute automated open-source REA cleanroom analysis, evidence extraction, property test verification, and ADR generation across showcase targets (aegis, dxball, ctf, th04, mattermost, all)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": TypedProp("string", "Showcase target: 'all', 'aegis', 'dxball', 'ctf', 'th04', 'mattermost' (default: 'all')"),
				"format": TypedProp("string", "Output representation: 'json', 'markdown', or 'summary' (default: 'summary')"),
			},
		},
	}, reaShowcaseHandler())
}

// reaShowcaseHandler handles the dendrite_rea_showcase tool.
func reaShowcaseHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		target := StringArgOr(args, "target", "all")
		format := StringArgOr(args, "format", "summary")

		report, err := showcase.RunShowcase(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("showcase execution failed: %w", err)
		}

		jsonData := IndentJSON(report)

		markdownScorecard := showcase.RenderMarkdownScorecard(report)

		switch strings.ToLower(format) {
		case "json":
			return TextResult(string(jsonData)), nil
		case "markdown", "md":
			return TextResult(markdownScorecard), nil
		default: // "summary"
			summaryText := fmt.Sprintf("[+] REA Showcase Verification (%s): %d/%d targets passed (%.2f%% aggregate parity)",
				report.OverallStatus, report.SucceededTargets, report.TotalTargets, report.OverallParityPercent)
			return &CallToolResult{
				Content: []ContentItem{
					{Type: "text", Text: summaryText},
					{Type: "text", Text: "\n" + markdownScorecard},
				},
			}, nil
		}
	}
}

var defaultContractMonitor = contract.NewContractMonitor(nil)

func registerContractTools(s *Server, deps Dependencies) {
	// 45. dendrite_contract_drift_check
	s.RegisterTool(Tool{
		Name:        "dendrite_contract_drift_check",
		Description: "Analyze cross-language schema and IPC contract drift across Protobuf, OpenAPI, and JSON-Schema definitions against Go, Rust, TS, and Python models in Quad-IDE projects",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"schema_dirs": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of directories containing .proto, openapi.yaml, or .json schema definitions",
				},
				"code_dirs": TypedProp("object", "Map of target languages ('go', 'rust', 'typescript', 'python') to repository source directories"),
			},
			"required": []interface{}{"schema_dirs"},
		},
	}, contractDriftCheckHandler())
}

// contractDriftCheckHandler handles the dendrite_contract_drift_check tool.
func contractDriftCheckHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		schemaDirs := NonEmptyStrings(args["schema_dirs"])
		if len(schemaDirs) == 0 {
			if raw, ok := args["schema_dirs"].([]interface{}); !ok || len(raw) == 0 {
				return nil, fmt.Errorf("missing required 'schema_dirs' parameter")
			}
		}
		codeDirs := make(map[contract.TargetLanguage]string)
		cMap, _ := args["code_dirs"].(map[string]interface{})
		for langStr, pathVal := range cMap {
			if pStr, ok := pathVal.(string); ok && pStr != "" {
				codeDirs[contract.TargetLanguage(langStr)] = pStr
			}
		}
		report, err := defaultContractMonitor.CheckDrift(schemaDirs, codeDirs)
		if err != nil {
			return nil, fmt.Errorf("contract drift check failed: %w", err)
		}
		return TextResult(string(IndentJSON(report))), nil
	}
}
