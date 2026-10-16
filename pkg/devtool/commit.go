package devtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CommitOptions defines the parameters for an autonomous Git commit.
type CommitOptions struct {
	TicketID         string   `json:"ticket_id,omitempty"`
	Message          string   `json:"message,omitempty"`
	Files            []string `json:"files,omitempty"`
	StageAll         bool     `json:"stage_all"`
	UseLocalModel    bool     `json:"use_local_model"`
	Model            string   `json:"model,omitempty"`
	DryRun           bool     `json:"dry_run,omitempty"`
	OxillamaEndpoint string   `json:"oxillama_endpoint,omitempty"`
	OllamaEndpoint   string   `json:"ollama_endpoint,omitempty"`
	LiteLLMEndpoint  string   `json:"litellm_endpoint,omitempty"`
	LiteLLMKey       string   `json:"litellm_key,omitempty"`
	RepoDir          string   `json:"repo_dir,omitempty"`
}

// CommitResult encapsulates the outcome of a commit operation.
type CommitResult struct {
	CommitSHA string   `json:"commit_sha"`
	Message   string   `json:"message"`
	Files     []string `json:"files"`
	TicketID  string   `json:"ticket_id,omitempty"`
	ModelUsed string   `json:"model_used,omitempty"`
	CostUSD   float64  `json:"cost_usd"`
	DryRun    bool     `json:"dry_run,omitempty"`
}

// FormatTicketCommit ensures the commit message has the proper ticket prefix.
func FormatTicketCommit(ticketID, message string) string {
	msg := strings.TrimSpace(message)
	tID := strings.TrimSpace(ticketID)
	if tID == "" {
		return msg
	}
	prefix := fmt.Sprintf("[%s]", tID)
	if strings.HasPrefix(msg, prefix) {
		return msg
	}
	return fmt.Sprintf("%s %s", prefix, msg)
}

// GenerateLocalCommitMessage synthesizes a Conventional Commit message using local models.
func GenerateLocalCommitMessage(ctx context.Context, diffStat, fullDiff, ticketID, model string, ollamaURL, litellmURL, litellmKey string) (string, string, error) {
	if model == "" {
		model = "qwen3.8:27b"
	}

	prompt := fmt.Sprintf(`You are an expert Git and AI engineer following the Conventional Commits specification.
Based on the following staged git diff summary and details, synthesize a clear, professional git commit message.
Format requirements:
- First line: <type>(<scope>): <imperative summary>
- Optional body: Bullet points summarizing key modifications.
- DO NOT wrap in markdown code blocks. Return plain text only.

Git Diff Summary:
%s

Git Diff Content (truncated):
%s`, diffStat, truncateString(fullDiff, 2000))

	client := &http.Client{Timeout: 35 * time.Second}

	// 1. Try OxiLLaMa Native Pure-Rust Zero-FFI Engine
	tryOxillama := strings.Contains(strings.ToLower(model), "oxillama") ||
		ollamaURL == "" || ollamaURL == "http://localhost:11434" || ollamaURL == "http://127.0.0.1:11434"
	if tryOxillama {
		oxillamaPath, _ := exec.LookPath("oxillama")
		if oxillamaPath == "" {
			home, _ := os.UserHomeDir()
			candidate := filepath.Join(home, ".cargo", "bin", "oxillama")
			if _, err := os.Stat(candidate); err == nil {
				oxillamaPath = candidate
			}
		}
		if oxillamaPath != "" {
			home, _ := os.UserHomeDir()
			normModel := strings.ReplaceAll(strings.TrimPrefix(model, "oxillama/"), ":", "_")
			modelDir := filepath.Join(home, ".oxillama", "models", normModel)
			if _, err := os.Stat(modelDir); err == nil {
				cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				oxCmd := exec.CommandContext(cmdCtx, oxillamaPath, "run", "--model", modelDir, prompt, "--max-tokens", "64")
				var outBuf, errBuf bytes.Buffer
				oxCmd.Stdout = &outBuf
				oxCmd.Stderr = &errBuf
				if err := oxCmd.Run(); err == nil {
					cancel()
					outStr := outBuf.String()
					lines := strings.Split(outStr, "\n")
					var filtered []string
					for _, l := range lines {
						trimmed := strings.TrimSpace(l)
						if trimmed == "" || strings.HasPrefix(trimmed, "INFO") || strings.HasPrefix(trimmed, "WARN") || strings.HasPrefix(trimmed, "✓") || strings.HasPrefix(trimmed, "OxiLLaMa") {
							continue
						}
						filtered = append(filtered, l)
					}
					cleaned := cleanCommitText(strings.Join(filtered, "\n"))
					if cleaned != "" {
						return FormatTicketCommit(ticketID, cleaned), model + " (OxiLLaMa Pure-Rust Zero-FFI)", nil
					}
				} else {
					cancel()
				}
			}
		}
	}

	// 2. Try Ollama directly
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaReq := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}
	reqBytes, _ := json.Marshal(ollamaReq)
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(ollamaURL, "/")+"/api/generate", bytes.NewReader(reqBytes))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, rErr := client.Do(req)
		if rErr == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var res struct {
				Response string `json:"response"`
			}
			if decErr := json.NewDecoder(resp.Body).Decode(&res); decErr == nil && strings.TrimSpace(res.Response) != "" {
				cleaned := cleanCommitText(res.Response)
				return FormatTicketCommit(ticketID, cleaned), model + " (Ollama Metal)", nil
			}
		}
	}

	// 3. Try LiteLLM Proxy (:4000)
	if litellmURL == "" {
		litellmURL = "http://localhost:4000/v1"
	}
	if litellmKey == "" {
		litellmKey = "sk-dendrite-master-key"
	}

	litellmModel := "local-qwen3-27b"
	litellmReq := map[string]interface{}{
		"model": litellmModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a git commit message synthesizer following Conventional Commits format."},
			{"role": "user", "content": prompt},
		},
		"max_tokens": 512,
	}
	lBytes, _ := json.Marshal(litellmReq)
	lReq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(litellmURL, "/")+"/chat/completions", bytes.NewReader(lBytes))
	if err == nil {
		lReq.Header.Set("Content-Type", "application/json")
		lReq.Header.Set("Authorization", "Bearer "+litellmKey)
		lResp, lErr := client.Do(lReq)
		if lErr == nil && lResp.StatusCode == http.StatusOK {
			defer lResp.Body.Close()
			var lRes struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if decErr := json.NewDecoder(lResp.Body).Decode(&lRes); decErr == nil && len(lRes.Choices) > 0 {
				cleaned := cleanCommitText(lRes.Choices[0].Message.Content)
				if cleaned != "" {
					return FormatTicketCommit(ticketID, cleaned), litellmModel + " (LiteLLM)", nil
				}
			}
		}
	}

	// 4. Fallback deterministic message
	defaultMsg := "chore(repo): update codebase and synchronizations"
	if diffStat != "" {
		firstLine := strings.Split(diffStat, "\n")[0]
		defaultMsg = fmt.Sprintf("chore: update files (%s)", strings.TrimSpace(firstLine))
	}
	return FormatTicketCommit(ticketID, defaultMsg), "deterministic-fallback", nil
}

// ExecuteCommit performs git staging, commit synthesis, and commit execution.
func ExecuteCommit(ctx context.Context, opts CommitOptions) (*CommitResult, error) {
	runGit := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		if opts.RepoDir != "" {
			cmd.Dir = opts.RepoDir
		}
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()
		if err != nil {
			return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return strings.TrimSpace(out.String()), nil
	}

	// 1. Staging
	if opts.StageAll || len(opts.Files) == 0 {
		if _, err := runGit("add", "-A"); err != nil {
			return nil, err
		}
	} else {
		for _, f := range opts.Files {
			if strings.TrimSpace(f) != "" {
				if _, err := runGit("add", strings.TrimSpace(f)); err != nil {
					return nil, err
				}
			}
		}
	}

	// 2. Query staged files and diff
	stagedStatus, err := runGit("diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to check staged files: %w", err)
	}
	if strings.TrimSpace(stagedStatus) == "" {
		return nil, fmt.Errorf("no staged changes to commit")
	}

	stagedFiles := strings.Split(strings.TrimSpace(stagedStatus), "\n")
	diffStat, _ := runGit("diff", "--cached", "--stat")
	fullDiff, _ := runGit("diff", "--cached")

	// 3. Determine commit message
	commitMsg := strings.TrimSpace(opts.Message)
	modelUsed := "user-specified"
	if commitMsg == "" {
		if opts.UseLocalModel {
			genMsg, mUsed, genErr := GenerateLocalCommitMessage(
				ctx,
				diffStat,
				fullDiff,
				opts.TicketID,
				opts.Model,
				opts.OllamaEndpoint,
				opts.LiteLLMEndpoint,
				opts.LiteLLMKey,
			)
			if genErr == nil && genMsg != "" {
				commitMsg = genMsg
				modelUsed = mUsed
			} else {
				commitMsg = FormatTicketCommit(opts.TicketID, "chore: autonomous repository update")
				modelUsed = "fallback"
			}
		} else {
			commitMsg = FormatTicketCommit(opts.TicketID, "chore: repository update")
			modelUsed = "manual-format"
		}
	} else {
		commitMsg = FormatTicketCommit(opts.TicketID, commitMsg)
	}

	if opts.DryRun {
		return &CommitResult{
			CommitSHA: "dry-run-preview-sha",
			Message:   commitMsg,
			Files:     stagedFiles,
			TicketID:  opts.TicketID,
			ModelUsed: modelUsed,
			CostUSD:   0.00,
			DryRun:    true,
		}, nil
	}

	// 4. Execute git commit
	if _, err := runGit("commit", "-m", commitMsg); err != nil {
		return nil, fmt.Errorf("git commit failed: %w", err)
	}

	// 5. Get commit SHA
	sha, err := runGit("rev-parse", "HEAD")
	if err != nil {
		sha = "unknown-sha"
	}

	return &CommitResult{
		CommitSHA: sha,
		Message:   commitMsg,
		Files:     stagedFiles,
		TicketID:  opts.TicketID,
		ModelUsed: modelUsed,
		CostUSD:   0.00,
		DryRun:    false,
	}, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n...[truncated]"
}

func cleanCommitText(raw string) string {
	lines := strings.Split(raw, "\n")
	var cleanedLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		cleanedLines = append(cleanedLines, l)
	}
	return strings.TrimSpace(strings.Join(cleanedLines, "\n"))
}
