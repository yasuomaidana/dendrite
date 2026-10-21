package gateway

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/topology"
)

// DefaultLiteLLMConfig creates a production-grade LiteLLM configuration with the 4-tier model hierarchy.
func DefaultLiteLLMConfig(ollamaBaseURL string) *LiteLLMConfig {
	if strings.TrimSpace(ollamaBaseURL) == "" {
		ollamaBaseURL = "http://127.0.0.1:11434"
	}

	oxillamaBaseURL := os.Getenv("DENDRITE_OXILLAMA_ENDPOINT")
	if oxillamaBaseURL == "" {
		oxillamaBaseURL = "http://127.0.0.1:8080"
	}
	// Local Qwen tiers run on Ollama (GPU). OxiLLaMa prefills on the CPU, so it serves them
	// only on request.
	oxillamaOnline := os.Getenv("DENDRITE_GATEWAY_OXILLAMA_ROUTING") == "true"

	qwen27bModel := "ollama/qwen3.8:27b"
	qwen27bAPIBase := ollamaBaseURL
	qwen27bProvider := "ollama"
	qwen27bAPIKey := ""

	qwen9bModel := "ollama/qwen3.5:9b"
	qwen9bAPIBase := ollamaBaseURL
	qwen9bProvider := "ollama"
	qwen9bAPIKey := ""

	if oxillamaOnline {
		qwen27bModel = "openai/qwen3.8:27b"
		qwen27bAPIBase = strings.TrimRight(oxillamaBaseURL, "/") + "/v1"
		qwen27bProvider = "openai"
		qwen27bAPIKey = "EMPTY"

		qwen9bModel = "openai/qwen3.5:9b"
		qwen9bAPIBase = strings.TrimRight(oxillamaBaseURL, "/") + "/v1"
		qwen9bProvider = "openai"
		qwen9bAPIKey = "EMPTY"
	}

	modelList := []ModelListEntry{
		{
			ModelName: "gemini-2.5-pro",
			LitellmParams: ModelParams{
				Model:     "gemini/gemini-2.5-pro",
				APIKey:    "os.environ/GEMINI_API_KEY",
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 "gemini-2.5-pro",
				Mode:               "chat",
				InputCostPerToken:  0.00000125,
				OutputCostPerToken: 0.000005,
				MaxInputTokens:     1048576,
				MaxOutputTokens:    8192,
				BaseModel:          "gemini/gemini-2.5-pro",
			},
		},
		{
			ModelName: "copilot-sonnet",
			LitellmParams: ModelParams{
				Model:      "github_copilot/" + CopilotModel(),
				APIKey:     "os.environ/COPILOT_API_KEY",
				DropParams: true,
				ExtraHeaders: map[string]string{
					"Editor-Version":         "vscode/1.95.0",
					"Copilot-Integration-Id": "vscode-chat",
				},
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 "copilot-sonnet",
				Mode:               "chat",
				InputCostPerToken:  0.000003,
				OutputCostPerToken: 0.000015,
				MaxInputTokens:     200000,
				MaxOutputTokens:    8192,
				BaseModel:          CopilotModel(),
			},
		},
		{
			ModelName: "gemini-2.5-flash",
			LitellmParams: ModelParams{
				Model:     "gemini/gemini-2.5-flash",
				APIKey:    "os.environ/GEMINI_API_KEY",
				MaxTokens: 8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 "gemini-2.5-flash",
				Mode:               "chat",
				InputCostPerToken:  0.000000075,
				OutputCostPerToken: 0.0000003,
				MaxInputTokens:     1048576,
				MaxOutputTokens:    8192,
				BaseModel:          "gemini/gemini-2.5-flash",
			},
		},
		{
			ModelName: "local-qwen3-27b",
			LitellmParams: ModelParams{
				Model:             qwen27bModel,
				APIBase:           qwen27bAPIBase,
				APIKey:            qwen27bAPIKey,
				CustomLLMProvider: qwen27bProvider,
				MaxTokens:         8192,
			},
			ModelInfo: &ModelInfo{
				ID:                 "local-qwen3-27b",
				Mode:               "chat",
				InputCostPerToken:  0.0,
				OutputCostPerToken: 0.0,
				MaxInputTokens:     32768,
				MaxOutputTokens:    8192,
				BaseModel:          "qwen3.8:27b",
				SkipBudgetChecks:   true,
			},
		},
		{
			ModelName: "local-qwen3-9b",
			LitellmParams: ModelParams{
				Model:             qwen9bModel,
				APIBase:           qwen9bAPIBase,
				APIKey:            qwen9bAPIKey,
				CustomLLMProvider: qwen9bProvider,
				MaxTokens:         4096,
			},
			ModelInfo: &ModelInfo{
				ID:                 "local-qwen3-9b",
				Mode:               "chat",
				InputCostPerToken:  0.0,
				OutputCostPerToken: 0.0,
				MaxInputTokens:     32768,
				MaxOutputTokens:    4096,
				BaseModel:          "qwen3.5:9b",
				SkipBudgetChecks:   true,
			},
		},
		{
			ModelName: "local-qwen-coder",
			LitellmParams: ModelParams{
				Model:             "ollama/qwen2.5-coder:14b",
				APIBase:           ollamaBaseURL,
				CustomLLMProvider: "ollama",
				MaxTokens:         4096,
			},
			ModelInfo: &ModelInfo{
				ID:                 "local-qwen-coder",
				Mode:               "chat",
				InputCostPerToken:  0.0,
				OutputCostPerToken: 0.0,
				MaxInputTokens:     32768,
				MaxOutputTokens:    4096,
				BaseModel:          "qwen2.5-coder:14b",
				SkipBudgetChecks:   true,
			},
		},
	}

	// Apply dynamic zero-cost exemptions to default model list
	for i := range modelList {
		if IsZeroCostSelfHostedModel(modelList[i].ModelName) || IsZeroCostSelfHostedModel(modelList[i].LitellmParams.Model) {
			if modelList[i].ModelInfo != nil {
				modelList[i].ModelInfo.SkipBudgetChecks = true
				modelList[i].ModelInfo.InputCostPerToken = 0.0
				modelList[i].ModelInfo.OutputCostPerToken = 0.0
			}
		}
	}

	routerSettings := &RouterSettings{
		RoutingStrategy:     "usage-based-routing",
		Timeout:             120,
		EnablePreCallChecks: true,
		// LiteLLM requires one model per fallback entry.
		Fallbacks: []map[string][]string{
			{"gemini-2.5-pro": {"copilot-sonnet", "gemini-2.5-flash", "local-qwen3-27b", "local-qwen-coder"}},
			{"copilot-sonnet": {"gemini-2.5-flash", "local-qwen3-27b", "local-qwen-coder"}},
			{"gemini-2.5-flash": {"local-qwen3-27b", "local-qwen-coder"}},
		},
		ContextWindowFallbacks: []map[string]string{
			{"local-qwen-coder": "gemini-2.5-pro"},
			{"local-qwen3-27b": "gemini-2.5-pro"},
			{"local-qwen3-9b": "gemini-2.5-pro"},
			{"gemini-2.5-flash": "gemini-2.5-pro"},
		},
	}

	generalSettings := &GeneralSettings{
		MasterKey:   "os.environ/LITELLM_MASTER_KEY",
		DatabaseURL: "os.environ/DATABASE_URL",
		RedisURL:    "os.environ/REDIS_URL",
	}

	litellmSettings := &LitellmSettings{
		DropParams:     true,
		BudgetDuration: "7d",
	}

	return &LiteLLMConfig{
		ModelList:       modelList,
		RouterSettings:  routerSettings,
		GeneralSettings: generalSettings,
		LitellmSettings: litellmSettings,
	}
}

// MultiEngineLiteLLMConfig dynamically constructs a LiteLLM configuration from registered InferenceEngines.
func MultiEngineLiteLLMConfig(engines []inference.InferenceEngine) *LiteLLMConfig {
	cfg := DefaultLiteLLMConfig("http://127.0.0.1:11434")

	var engineFallbackNames []string

	for _, eng := range engines {
		ecfg := eng.Config()
		endpoint := strings.TrimRight(ecfg.Endpoint, "/")

		var modelName, upstreamModel, apiBase, apiKey, provider string
		skipBudget := eng.IsSelfHosted()
		cost := 0.0

		switch eng.Type() {
		case inference.EngineOllama:
			modelName = "local-coder"
			mID := "qwen3.8:27b"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "ollama/" + mID
			apiBase = endpoint
			provider = "ollama"
			skipBudget = true

		case inference.EngineOxiLLaMa:
			modelName = "oxillama-rust"
			mID := "qwen2.5-coder-7b"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "openai/" + mID
			apiBase = endpoint + "/v1"
			apiKey = "EMPTY"
			skipBudget = true

		case inference.EngineHuggingFaceTGI:
			modelName = "hf-deepseek"
			mID := "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "openai/" + mID
			apiBase = endpoint + "/v1"
			apiKey = "EMPTY"
			if ecfg.APIKey != "" {
				apiKey = ecfg.APIKey
			}

		case inference.EngineHuggingFaceTEI:
			modelName = "hf-tei-embeddings"
			mID := "BAAI/bge-large-en-v1.5"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "huggingface-tei/" + mID
			apiBase = endpoint
			skipBudget = true

		case inference.EngineVLLM:
			modelName = "vllm-cluster"
			mID := "Qwen/Qwen2.5-Coder-32B-Instruct"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "openai/" + mID
			apiBase = endpoint + "/v1"
			apiKey = "EMPTY"

		case inference.EngineLlamaCpp:
			modelName = "llamacpp-local"
			mID := "mistral-7b-instruct-q4"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "openai/" + mID
			apiBase = endpoint + "/v1"
			apiKey = "EMPTY"
			skipBudget = true

		case inference.EngineHuggingFaceEndpoint:
			modelName = "hf-hosted-llama"
			mID := "meta-llama/Meta-Llama-3-8B-Instruct"
			if len(ecfg.Models) > 0 {
				mID = ecfg.Models[0]
			}
			upstreamModel = "huggingface/" + mID
			apiBase = endpoint
			apiKey = "os.environ/HF_TOKEN"
			skipBudget = false
			cost = 0.000001
		}

		if modelName != "" {
			if eng.IsSelfHosted() || IsZeroCostSelfHostedModel(modelName) || IsZeroCostSelfHostedModel(upstreamModel) {
				skipBudget = true
				cost = 0.0
			}

			entry := ModelListEntry{
				ModelName: modelName,
				LitellmParams: ModelParams{
					Model:             upstreamModel,
					APIBase:           apiBase,
					APIKey:            apiKey,
					CustomLLMProvider: provider,
					MaxTokens:         4096,
				},
				ModelInfo: &ModelInfo{
					ID:                 modelName,
					Mode:               "chat",
					InputCostPerToken:  cost,
					OutputCostPerToken: cost,
					MaxInputTokens:     32768,
					MaxOutputTokens:    4096,
					BaseModel:          upstreamModel,
					SkipBudgetChecks:   skipBudget,
				},
			}
			cfg.ModelList = append(cfg.ModelList, entry)
			if skipBudget {
				engineFallbackNames = append(engineFallbackNames, modelName)
			}
		}
	}

	// Dynamic cascade integration: prioritize smarter local models
	if len(engineFallbackNames) > 0 {
		sort.SliceStable(engineFallbackNames, func(i, j int) bool {
			return inference.ModelIntelligenceScore(engineFallbackNames[i]) > inference.ModelIntelligenceScore(engineFallbackNames[j])
		})
		cascadeList := append([]string{"copilot-sonnet", "gemini-2.5-flash"}, engineFallbackNames...)
		cfg.RouterSettings.Fallbacks = []map[string][]string{
			{"gemini-2.5-pro": cascadeList},
			{"copilot-sonnet": engineFallbackNames},
		}
	}

	// Point local-qwen3 aliases to OxiLLaMa when present in registered engines
	for _, eng := range engines {
		if eng.Type() == inference.EngineOxiLLaMa {
			oxEP := strings.TrimRight(eng.Config().Endpoint, "/") + "/v1"
			for i := range cfg.ModelList {
				switch cfg.ModelList[i].ModelName {
				case "local-qwen3-27b":
					cfg.ModelList[i].LitellmParams.Model = "openai/qwen3.8:27b"
					cfg.ModelList[i].LitellmParams.APIBase = oxEP
					cfg.ModelList[i].LitellmParams.CustomLLMProvider = "openai"
					cfg.ModelList[i].LitellmParams.APIKey = "EMPTY"
				case "local-qwen3-9b":
					cfg.ModelList[i].LitellmParams.Model = "openai/qwen3.5:9b"
					cfg.ModelList[i].LitellmParams.APIBase = oxEP
					cfg.ModelList[i].LitellmParams.CustomLLMProvider = "openai"
					cfg.ModelList[i].LitellmParams.APIKey = "EMPTY"
				}
			}
			break
		}
	}

	return cfg
}

// RenderForInferenceManager renders LiteLLM YAML for an inference Manager instance.
func RenderForInferenceManager(mgr *inference.Manager) ([]byte, error) {
	if mgr == nil {
		mgr = inference.DefaultManager()
	}
	cfg := MultiEngineLiteLLMConfig(mgr.ListEngines())
	return RenderYAML(cfg)
}

// Validate verifies the completeness and sanity of the LiteLLM configuration.
func (c *LiteLLMConfig) Validate() error {
	if c == nil {
		return &GatewayValidationError{Field: "config", Message: "gateway config cannot be nil"}
	}
	if len(c.ModelList) == 0 {
		return &GatewayValidationError{Field: "model_list", Message: "model list cannot be empty", Err: ErrEmptyModelList}
	}

	for i, entry := range c.ModelList {
		if strings.TrimSpace(entry.ModelName) == "" {
			return &GatewayValidationError{
				Field:   fmt.Sprintf("model_list[%d].model_name", i),
				Message: "model name cannot be empty",
				Err:     ErrInvalidModelName,
			}
		}
		if strings.TrimSpace(entry.LitellmParams.Model) == "" {
			return &GatewayValidationError{
				Field:   fmt.Sprintf("model_list[%d].litellm_params.model", i),
				Message: "upstream provider model cannot be empty",
				Err:     ErrMissingProvider,
			}
		}
	}

	if c.RouterSettings != nil {
		for i, fb := range c.RouterSettings.Fallbacks {
			if len(fb) != 1 {
				return &GatewayValidationError{
					Field:   fmt.Sprintf("router_settings.fallbacks[%d]", i),
					Message: fmt.Sprintf("each fallback entry must map exactly one model, got %d", len(fb)),
				}
			}
		}
		for i, fb := range c.RouterSettings.ContextWindowFallbacks {
			if len(fb) != 1 {
				return &GatewayValidationError{
					Field:   fmt.Sprintf("router_settings.context_window_fallbacks[%d]", i),
					Message: fmt.Sprintf("each fallback entry must map exactly one model, got %d", len(fb)),
				}
			}
		}
	}

	return nil
}

// RenderYAML marshals the configuration into standard LiteLLM Proxy YAML.
func RenderYAML(cfg *LiteLLMConfig) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return data, nil
}

// RenderForTopology derives the Ollama base URL from an active TopologyConfig and generates the YAML.
func RenderForTopology(top *topology.TopologyConfig) ([]byte, error) {
	if top == nil {
		return nil, &GatewayValidationError{Field: "topology", Message: "topology configuration cannot be nil"}
	}
	if err := top.Validate(); err != nil {
		return nil, fmt.Errorf("invalid topology: %w", err)
	}

	execIP := top.TailscaleEndpoints.ExecutionPlane
	if strings.TrimSpace(execIP) == "" {
		execIP = "127.0.0.1"
	}
	ollamaURL := fmt.Sprintf("http://%s:11434", execIP)

	cfg := DefaultLiteLLMConfig(ollamaURL)
	return RenderYAML(cfg)
}
