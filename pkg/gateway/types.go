package gateway

import (
	"errors"
	"fmt"
)

// Standard error definitions for gateway configuration.
var (
	ErrEmptyModelList   = errors.New("model list cannot be empty")
	ErrInvalidModelName = errors.New("model name cannot be empty")
	ErrMissingProvider  = errors.New("upstream provider/model cannot be empty")
)

// ModelParams defines the provider-level parameters for an LLM endpoint.
type ModelParams struct {
	Model             string            `yaml:"model" json:"model"`
	APIKey            string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIBase           string            `yaml:"api_base,omitempty" json:"api_base,omitempty"`
	CustomLLMProvider string            `yaml:"custom_llm_provider,omitempty" json:"custom_llm_provider,omitempty"`
	MaxTokens         int               `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	DropParams        bool              `yaml:"drop_params,omitempty" json:"drop_params,omitempty"`
	ExtraHeaders      map[string]string `yaml:"extra_headers,omitempty" json:"extra_headers,omitempty"`
}

// ModelInfo defines token limits, billing metrics, and cost metadata.
type ModelInfo struct {
	ID                 string  `yaml:"id,omitempty" json:"id,omitempty"`
	Mode               string  `yaml:"mode,omitempty" json:"mode,omitempty"`
	InputCostPerToken  float64 `yaml:"input_cost_per_token" json:"input_cost_per_token"`
	OutputCostPerToken float64 `yaml:"output_cost_per_token" json:"output_cost_per_token"`
	MaxInputTokens     int     `yaml:"max_input_tokens,omitempty" json:"max_input_tokens,omitempty"`
	MaxOutputTokens    int     `yaml:"max_output_tokens,omitempty" json:"max_output_tokens,omitempty"`
	BaseModel          string  `yaml:"base_model,omitempty" json:"base_model,omitempty"`
	SkipBudgetChecks   bool    `yaml:"skip_budget_checks,omitempty" json:"skip_budget_checks,omitempty"`
}

// ModelListEntry represents a single model endpoint registered in LiteLLM.
type ModelListEntry struct {
	ModelName     string      `yaml:"model_name" json:"model_name"`
	LitellmParams ModelParams `yaml:"litellm_params" json:"litellm_params"`
	ModelInfo     *ModelInfo  `yaml:"model_info,omitempty" json:"model_info,omitempty"`
}

// RouterSettings configures fallback cascades, context window escalation, and routing strategies.
type RouterSettings struct {
	ModelGroupAlias        map[string]string     `yaml:"model_group_alias,omitempty" json:"model_group_alias,omitempty"`
	Fallbacks              []map[string][]string `yaml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
	ContextWindowFallbacks []map[string]string   `yaml:"context_window_fallbacks,omitempty" json:"context_window_fallbacks,omitempty"`
	RoutingStrategy        string                `yaml:"routing_strategy,omitempty" json:"routing_strategy,omitempty"`
	Timeout                int                   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// EnablePreCallChecks makes LiteLLM check the context window before calling a deployment (DEN-945).
	EnablePreCallChecks bool `yaml:"enable_pre_call_checks,omitempty" json:"enable_pre_call_checks,omitempty"`
}

// addContextWindowFallbacks appends one single-key context-window fallback to target for every
// model that does not have one yet, so oversized prompts are re-routed instead of failing.
func addContextWindowFallbacks(rs *RouterSettings, models []string, target string) {
	for _, name := range models {
		if name == target || hasContextWindowFallback(rs.ContextWindowFallbacks, name) {
			continue
		}
		rs.ContextWindowFallbacks = append(rs.ContextWindowFallbacks, map[string]string{name: target})
	}
}

func hasContextWindowFallback(entries []map[string]string, model string) bool {
	for _, entry := range entries {
		if _, ok := entry[model]; ok {
			return true
		}
	}
	return false
}

// GeneralSettings configures LiteLLM proxy authentication, storage, and telemetry.
type GeneralSettings struct {
	MasterKey   string   `yaml:"master_key,omitempty" json:"master_key,omitempty"`
	DatabaseURL string   `yaml:"database_url,omitempty" json:"database_url,omitempty"`
	RedisURL    string   `yaml:"redis_url,omitempty" json:"redis_url,omitempty"`
	Alerting    []string `yaml:"alerting,omitempty" json:"alerting,omitempty"`
}

// LitellmSettings sets global request-handling and callback behaviors.
type LitellmSettings struct {
	DropParams      bool     `yaml:"drop_params,omitempty" json:"drop_params,omitempty"`
	SetVerbose      bool     `yaml:"set_verbose,omitempty" json:"set_verbose,omitempty"`
	SuccessCallback []string `yaml:"success_callback,omitempty" json:"success_callback,omitempty"`
	FailureCallback []string `yaml:"failure_callback,omitempty" json:"failure_callback,omitempty"`
	BudgetDuration  string   `yaml:"budget_duration,omitempty" json:"budget_duration,omitempty"`
}

// LiteLLMConfig encapsulates the full declarative configuration for the LiteLLM Proxy Gateway.
type LiteLLMConfig struct {
	ModelList       []ModelListEntry `yaml:"model_list" json:"model_list"`
	RouterSettings  *RouterSettings  `yaml:"router_settings,omitempty" json:"router_settings,omitempty"`
	GeneralSettings *GeneralSettings `yaml:"general_settings,omitempty" json:"general_settings,omitempty"`
	LitellmSettings *LitellmSettings `yaml:"litellm_settings,omitempty" json:"litellm_settings,omitempty"`
}

// GatewayValidationError describes an error during configuration validation.
type GatewayValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *GatewayValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("gateway validation error on %s: %s (%v)", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("gateway validation error on %s: %s", e.Field, e.Message)
}

func (e *GatewayValidationError) Unwrap() error {
	return e.Err
}
