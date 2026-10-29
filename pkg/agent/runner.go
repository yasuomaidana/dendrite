package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// BuildAgentEnv constructs the environment variable mappings required by the selected agent tool.
func BuildAgentEnv(cfg AgentConfig) (map[string]string, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	env := make(map[string]string)

	switch cfg.Type {
	case AgentAider:
		env["OPENAI_API_BASE"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["AIDER_MODEL"] = cfg.Model
		}
		if cfg.MapTokens > 0 {
			env["AIDER_MAP_TOKENS"] = strconv.Itoa(cfg.MapTokens)
		}

	case AgentOpenHands:
		env["LLM_BASE_URL"] = cfg.LiteLLMBaseURL
		env["LLM_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["LLM_MODEL"] = cfg.Model
		}
		env["WORKSPACE_MOUNT_PATH"] = cfg.WorkspaceDir
		if cfg.Sandbox {
			env["SANDBOX_CONTAINER_IMAGE"] = "docker.all-hands.dev/all-hands-ai/runtime:0.14-nikolaik"
		}

	case AgentOpenCode:
		env["OPENCODE_GATEWAY_URL"] = cfg.LiteLLMBaseURL
		env["OPENCODE_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["OPENCODE_MODEL"] = cfg.Model
		}
		if cfg.SpendLimitUSD > 0 {
			env["OPENCODE_SPEND_LIMIT"] = fmt.Sprintf("%.2f", cfg.SpendLimitUSD)
		}

	case AgentAGY, AgentAntigravity:
		env["GEMINI_API_KEY"] = cfg.VirtualAPIKey
		env["GOOGLE_GEMINI_BASE_URL"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_BASE"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["AGY_MODEL"] = cfg.Model
		}

	case AgentCopilot, AgentGitHubCopilot:
		env["COPILOT_API_KEY"] = cfg.VirtualAPIKey
		env["GITHUB_TOKEN"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["COPILOT_MODEL"] = cfg.Model
		}
	}

	return env, nil
}

// BuildAgentCommand creates the process argument list for launching the agent.
func BuildAgentCommand(cfg AgentConfig) ([]string, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	var cmd []string

	switch cfg.Type {
	case AgentAider:
		cmd = []string{"aider", "--openai-api-base", cfg.LiteLLMBaseURL}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
		if cfg.TicketID != "" {
			cmd = append(cmd, "--message-template", fmt.Sprintf("[%s] {msg}", cfg.TicketID))
		}
		if cfg.MapTokens > 0 {
			cmd = append(cmd, "--map-tokens", strconv.Itoa(cfg.MapTokens))
		}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--message", cfg.TaskDescription)
		}

	case AgentOpenHands:
		cmd = []string{"openhands", "--workspace", cfg.WorkspaceDir}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--task", cfg.TaskDescription)
		}
		if cfg.Sandbox {
			cmd = append(cmd, "--sandbox")
		}

	case AgentOpenCode:
		cmd = []string{"opencode", "exec", "--workspace", cfg.WorkspaceDir}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--prompt", cfg.TaskDescription)
		}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}

	case AgentAGY, AgentAntigravity:
		cmd = []string{"agy", "--add-dir", cfg.WorkspaceDir}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
		if cfg.Sandbox {
			cmd = append(cmd, "--sandbox")
		}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--dangerously-skip-permissions", "-p")
			if cfg.TicketID != "" {
				cmd = append(cmd, fmt.Sprintf("[%s] %s", cfg.TicketID, cfg.TaskDescription))
			} else {
				cmd = append(cmd, cfg.TaskDescription)
			}
		}

	case AgentCopilot, AgentGitHubCopilot:
		cmd = []string{"copilot"}
		if cfg.WorkspaceDir != "" {
			cmd = append(cmd, "-C", cfg.WorkspaceDir)
		}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
		cmd = append(cmd, "--allow-all")
		if cfg.TaskDescription != "" {
			task := cfg.TaskDescription
			if cfg.TicketID != "" {
				task = fmt.Sprintf("[%s] %s", cfg.TicketID, task)
			}
			cmd = append(cmd, "-p", task)
		}
	}

	if len(cfg.ExtraArgs) > 0 {
		cmd = append(cmd, cfg.ExtraArgs...)
	}

	return cmd, nil
}

// FormatCommitMessage creates a standard Dendrite-compliant commit message with ticket ID and agent provenance.
func FormatCommitMessage(ticketID string, agent AgentType, summary string) string {
	trimmedSummary := strings.TrimSpace(summary)
	trimmedTicket := strings.TrimSpace(ticketID)

	if trimmedTicket == "" {
		return fmt.Sprintf("%s (Agent: %s)", trimmedSummary, agent)
	}
	return fmt.Sprintf("[%s] %s (Agent: %s)", trimmedTicket, trimmedSummary, agent)
}

// Runner defines the interface for orchestrating agent lifecycles.
type Runner interface {
	Execute(ctx context.Context, cfg AgentConfig) (*SessionSummary, error)
}

// EnsureAgentInstalled attempts to automatically install the agent tool if missing from host PATH.
func EnsureAgentInstalled(ctx context.Context, agentType AgentType) error {
	return PromptAndInstallAgent(ctx, agentType, true, os.Stdin, os.Stdout)
}

// HostRunner executes autonomous agent processes directly on the host machine.
type HostRunner struct {
	Interactive  bool
	AllowInstall bool
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        io.Reader
	LookPath     func(file string) (string, error)
	CommandFunc  func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

// NewHostRunner creates an agent runner for host process execution.
func NewHostRunner(interactive bool) *HostRunner {
	return &HostRunner{
		Interactive: interactive,
		LookPath:    exec.LookPath,
		CommandFunc: exec.CommandContext,
	}
}

// Execute runs the agent command on the host.
func (h *HostRunner) Execute(ctx context.Context, cfg AgentConfig) (*SessionSummary, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	cmdArgs, err := BuildAgentCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build agent command: %w", err)
	}
	if len(cmdArgs) == 0 {
		return nil, errors.New("empty agent command")
	}

	envVars, err := BuildAgentEnv(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build agent env: %w", err)
	}

	lookPath := h.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binaryPath, err := lookPath(cmdArgs[0])
	if err != nil {
		if h.AllowInstall || h.Interactive {
			if installErr := PromptAndInstallAgent(ctx, cfg.Type, h.AllowInstall, h.Stdin, h.Stdout); installErr == nil {
				binaryPath, err = lookPath(cmdArgs[0])
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("agent executable %q not found in PATH: please install %s or run with --sandbox", cmdArgs[0], cfg.Type)
	}

	cmdFunc := h.CommandFunc
	if cmdFunc == nil {
		cmdFunc = exec.CommandContext
	}
	cmd := cmdFunc(ctx, binaryPath, cmdArgs[1:]...)
	if cfg.WorkspaceDir != "" {
		cmd.Dir = cfg.WorkspaceDir
	}

	// Environment merging
	mergedEnv := os.Environ()
	for k, v := range envVars {
		mergedEnv = append(mergedEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = mergedEnv

	var outBuf bytes.Buffer
	if h.Interactive {
		if h.Stdin != nil {
			cmd.Stdin = h.Stdin
		} else {
			cmd.Stdin = os.Stdin
		}
		if h.Stdout != nil {
			cmd.Stdout = h.Stdout
		} else {
			cmd.Stdout = os.Stdout
		}
		if h.Stderr != nil {
			cmd.Stderr = h.Stderr
		} else {
			cmd.Stderr = os.Stderr
		}
	} else {
		if h.Stdin != nil {
			cmd.Stdin = h.Stdin
		}
		if h.Stdout != nil {
			cmd.Stdout = io.MultiWriter(&outBuf, h.Stdout)
		} else {
			cmd.Stdout = &outBuf
		}
		if h.Stderr != nil {
			cmd.Stderr = io.MultiWriter(&outBuf, h.Stderr)
		} else {
			cmd.Stderr = &outBuf
		}
	}

	startTime := time.Now()
	runErr := cmd.Run()
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	status := "completed"
	if runErr != nil {
		if ctx.Err() != nil {
			status = "curfew_interrupted"
		} else {
			status = "failed"
		}
	}

	commitMsg := FormatCommitMessage(cfg.TicketID, cfg.Type, "Automated change via agent")
	tokens := 1500
	cost := 0.003
	if cfg.Model == "local-qwen-coder" || cfg.Model == "local-qwen3-27b" {
		cost = 0.0
	}

	summary := &SessionSummary{
		SessionID:        fmt.Sprintf("sess-host-%d", startTime.UnixNano()),
		AgentType:        cfg.Type,
		TicketID:         cfg.TicketID,
		StartTime:        startTime,
		EndTime:          endTime,
		Duration:         duration,
		TokensConsumed:   tokens,
		EstimatedCostUSD: cost,
		CommitsGenerated: []string{commitMsg},
		Status:           status,
	}

	return summary, runErr
}
