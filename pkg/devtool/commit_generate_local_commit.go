package devtool

import (
	"Dendrite/pkg/memfit"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/quality"
)

// GenerateLocalCommitMessageWithContext synthesizes Conventional Commit with active ticket context.
func GenerateLocalCommitMessageWithContext(ctx context.Context, diffStat, fullDiff, ticketID string, smartTicket *SmartTicketInfo, model string, ollamaURL, litellmURL, litellmKey string, stagedFiles []string) (string, string, error) {
	// A default that is not installed, or too large for this machine, is replaced by the smartest
	// installed model that fits; an unreachable Ollama keeps the old default.
	if !strings.Contains(strings.ToLower(model), "llamacpp") {
		model = memfit.ResolveOllama(ctx, ollamaURL, model)
	}
	if model == "" {
		model = inference.ChatModel(inference.TierChore)
	}

	ticketContext := ""
	if ticketID != "" {
		ticketContext = fmt.Sprintf("\nActive Ticket: [%s]", ticketID)
		if smartTicket != nil && smartTicket.Title != "" {
			ticketContext = fmt.Sprintf("\nActive Ticket: [%s] %s", ticketID, smartTicket.Title)
		}
	}

	prompt := fmt.Sprintf(`You are an expert Git and AI engineer following the Conventional Commits specification.%s
Based on the following staged git diff summary and details, synthesize a clear, professional git commit message.
Format requirements:
- First line: <type>(<scope>): <imperative summary>
- Optional body: Bullet points summarizing key modifications.
- DO NOT wrap in markdown code blocks. Return plain text only.

Git Diff Summary:
%s

Git Diff Content (truncated):
%s`, ticketContext, diffStat, truncateString(fullDiff, 2000))

	client := &http.Client{Timeout: 4 * time.Second}

	// 1. Try the local llama-server (OpenAI-compatible)
	llamaURL := os.Getenv("DENDRITE_LLAMACPP_ENDPOINT")
	if llamaURL == "" {
		llamaURL = "http://127.0.0.1:8080"
	}
	tryLlama := strings.Contains(strings.ToLower(model), "llamacpp") ||
		ollamaURL == "" || ollamaURL == "http://localhost:11434" || ollamaURL == "http://127.0.0.1:11434" ||
		os.Getenv("DENDRITE_COMMIT_PREFER_LOCAL") == "true"

	if tryLlama {
		llamaModel := strings.TrimPrefix(model, "llamacpp/")
		if llamaModel == "" || !strings.Contains(llamaModel, "qwen") {
			// llama-server serves the one model it was started with; name it in DENDRITE_LLAMA_MODEL.
			llamaModel = llamaServedModel()
		}
		payload := map[string]interface{}{
			"model": llamaModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"max_tokens": 256,
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(llamaURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			llamaClient := &http.Client{Timeout: 3 * time.Second}
			resp, doErr := llamaClient.Do(req)
			if doErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var res struct {
						Choices []struct {
							Message struct {
								Content string `json:"content"`
							} `json:"message"`
						} `json:"choices"`
					}
					if json.NewDecoder(resp.Body).Decode(&res) == nil && len(res.Choices) > 0 {
						cleaned := cleanCommitText(res.Choices[0].Message.Content)
						if !isCannedOrInvalidCommitMessage(cleaned) {
							return FormatTicketCommit(ticketID, cleaned), llamaModel + " (llama.cpp HTTP)", nil
						}
					}
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
				if !isCannedOrInvalidCommitMessage(cleaned) {
					return FormatTicketCommit(ticketID, cleaned), model + " (Ollama Metal)", nil
				}
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
				if !isCannedOrInvalidCommitMessage(cleaned) {
					return FormatTicketCommit(ticketID, cleaned), litellmModel + " (LiteLLM)", nil
				}
			}
		}
	}

	// 4. Fallback deterministic message
	defaultMsg := synthesizeDeterministicCommitMessage(ticketID, smartTicket, diffStat, stagedFiles)
	return FormatTicketCommit(ticketID, defaultMsg), "deterministic-fallback", nil
}

// ExecuteCommit performs git staging, smart ticket resolution, commit synthesis, and commit execution.
func ExecuteCommit(ctx context.Context, opts CommitOptions) (*CommitResult, error) {
	runGit := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		if opts.RepoDir != "" {
			cmd.Dir = opts.RepoDir
		}
		if opts.NoTicketGuard { // git's pre-commit hook honours the same bypass
			cmd.Env = append(os.Environ(), TicketGuardEnv+"=off")
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
	// Check if there are already staged files in index
	// pathspec limits the commit to opts.Files: changes the developer staged
	// separately stay staged instead of being swept into this commit.
	var pathspec []string
	alreadyStaged, _ := runGit("diff", "--cached", "--name-only")
	if strings.TrimSpace(alreadyStaged) != "" && !opts.StageAll && len(opts.Files) == 0 {
		// Developer or agent already staged specific files; do not clobber with git add -A
	} else if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			if strings.TrimSpace(f) != "" {
				if _, err := runGit("add", strings.TrimSpace(f)); err != nil {
					return nil, err
				}
				pathspec = append(pathspec, strings.TrimSpace(f))
			}
		}
	} else if opts.StageAll {
		if _, err := runGit("add", "-A"); err != nil {
			return nil, err
		}
	} else if strings.TrimSpace(alreadyStaged) == "" && !opts.Amend {
		if _, err := runGit("add", "-A"); err != nil {
			return nil, err
		}
	}

	// 2. Query staged files and diff
	limit := func(args ...string) []string {
		if len(pathspec) == 0 {
			return args
		}
		return append(append(args, "--"), pathspec...)
	}
	stagedStatus, err := runGit(limit("diff", "--cached", "--name-only")...)
	if err != nil {
		return nil, fmt.Errorf("failed to check staged files: %w", err)
	}
	if strings.TrimSpace(stagedStatus) == "" && !opts.Amend {
		return nil, ErrNothingToCommit
	}

	var stagedFiles []string
	if strings.TrimSpace(stagedStatus) != "" {
		stagedFiles = strings.Split(strings.TrimSpace(stagedStatus), "\n")
	} else if opts.Amend {
		// When amending with no new staged changes, examine HEAD files
		if headFiles, hErr := runGit("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); hErr == nil {
			stagedFiles = strings.Split(strings.TrimSpace(headFiles), "\n")
		}
	}

	diffStat, _ := runGit(limit("diff", "--cached", "--stat")...)
	fullDiff, _ := runGit(limit("diff", "--cached")...)
	if opts.Amend && diffStat == "" {
		diffStat, _ = runGit("show", "--stat", "HEAD")
		fullDiff, _ = runGit("show", "HEAD")
	}

	// 2a. Zero-Mock Policy Enforcement Gate
	if !opts.AllowMocks {
		if hasMocks, mockLocs, _ := quality.ScanFilesForMocks(opts.RepoDir, stagedFiles); hasMocks {
			return nil, fmt.Errorf("zero-mock policy violation: staged changes contain banned mocks/stubs:\n%s\nMust implement real production services (or pass --allow-mocks for emergency bypass)", strings.Join(mockLocs, "\n"))
		}
	}

	// 3. Smart Ticket Resolution
	var smartTicket *SmartTicketInfo
	smartSource := ""
	if opts.TicketID == "" && !opts.NoSmart {
		resolved, sErr := ResolveSmartTicket(ctx, opts.RepoDir, stagedFiles)
		if sErr == nil && resolved != nil && resolved.ID != "" {
			opts.TicketID = resolved.ID
			smartTicket = resolved
			smartSource = resolved.Source
		}
	} else if opts.SmartTicket != nil {
		smartTicket = opts.SmartTicket
		smartSource = smartTicket.Source
	}

	// 3a. Ticket lifecycle guard: the work must belong to a ticket that is In Progress.
	var guard *GuardResult
	if !opts.NoTicketGuard && GuardEnabled() {
		g, gErr := EnsureTicketInProgress(ctx, opts.RepoDir, opts.TicketID, GuardCommit, opts.DryRun)
		if gErr != nil {
			return nil, gErr
		}
		guard = &g
	}

	// 4. Determine commit message
	commitMsg := strings.TrimSpace(opts.Message)
	modelUsed := "user-specified"
	if commitMsg == "" {
		if opts.UseLocalModel {
			genMsg, mUsed, genErr := GenerateLocalCommitMessageWithContext(
				ctx,
				diffStat,
				fullDiff,
				opts.TicketID,
				smartTicket,
				opts.Model,
				opts.OllamaEndpoint,
				opts.LiteLLMEndpoint,
				opts.LiteLLMKey,
				stagedFiles,
			)
			if genErr == nil && genMsg != "" {
				commitMsg = genMsg
				modelUsed = mUsed
			} else {
				commitMsg = FormatTicketCommit(opts.TicketID, synthesizeDeterministicCommitMessage(opts.TicketID, smartTicket, diffStat, stagedFiles))
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
		res := &CommitResult{
			CommitSHA:   "dry-run-preview-sha",
			Message:     commitMsg,
			Files:       stagedFiles,
			TicketID:    opts.TicketID,
			ModelUsed:   modelUsed,
			CostUSD:     0.00,
			DryRun:      true,
			SmartTicket: smartTicket,
			Guard:       guard,
			SmartSource: smartSource,
		}
		_ = pushAfterCommit(ctx, opts, res, runGit)
		return res, nil
	}

	// 5. Execute git commit
	gitArgs := []string{"commit"}
	if opts.Amend {
		gitArgs = append(gitArgs, "--amend")
	}
	gitArgs = append(gitArgs, "-m", commitMsg)
	if _, err := runGit(limit(gitArgs...)...); err != nil {
		return nil, fmt.Errorf("git commit failed: %w", err)
	}

	// 6. Get commit SHA
	sha, err := runGit("rev-parse", "HEAD")
	if err != nil {
		sha = "unknown-sha"
	}

	res := &CommitResult{
		CommitSHA:   sha,
		Message:     commitMsg,
		Files:       stagedFiles,
		TicketID:    opts.TicketID,
		ModelUsed:   modelUsed,
		CostUSD:     0.00,
		DryRun:      false,
		SmartTicket: smartTicket,
		Guard:       guard,
		SmartSource: smartSource,
	}

	_ = pushAfterCommit(ctx, opts, res, runGit)

	return res, nil
}

func pushAfterCommit(ctx context.Context, opts CommitOptions, res *CommitResult, runGit func(args ...string) (string, error)) error {
	if opts.DryRun || res.DryRun {
		res.PushSkipped = true
		res.PushReason = "dry-run mode"
		return nil
	}

	if opts.NoPush {
		res.PushSkipped = true
		res.PushReason = "--no-push flag specified"
		return nil
	}

	if strings.EqualFold(os.Getenv("DENDRITE_COMMIT_PUSH"), "false") {
		res.PushSkipped = true
		res.PushReason = "DENDRITE_COMMIT_PUSH=false"
		return nil
	}

	out := pushBranch(runGit, pushRemoteFor(opts.RepoDir))
	res.Pushed = out.Pushed
	res.PushSkipped = out.Skipped
	res.PushReason = out.Reason
	res.PushError = out.Err
	return out.Err
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

// llamaServedModel is the model name sent to a llama-server when the requested one is not a Qwen
// model. It is the server's own model, not the Ollama chat model, so it has its own setting.
func llamaServedModel() string {
	if v := strings.TrimSpace(os.Getenv("DENDRITE_LLAMA_MODEL")); v != "" {
		return v
	}
	return "qwen3.8:27b"
}
