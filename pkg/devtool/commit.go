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
	"regexp"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/tracker/plane"
)

// CommitOptions defines the parameters for an autonomous Git commit.
type CommitOptions struct {
	TicketID         string           `json:"ticket_id,omitempty"`
	Message          string           `json:"message,omitempty"`
	Files            []string         `json:"files,omitempty"`
	StageAll         bool             `json:"stage_all"`
	UseLocalModel    bool             `json:"use_local_model"`
	Model            string           `json:"model,omitempty"`
	DryRun           bool             `json:"dry_run,omitempty"`
	Amend            bool             `json:"amend,omitempty"`
	NoPush           bool             `json:"no_push,omitempty"`
	NoSmart          bool             `json:"no_smart,omitempty"`
	Smart            bool             `json:"smart"`
	SmartTicket      *SmartTicketInfo `json:"smart_ticket,omitempty"`
	OxillamaEndpoint string           `json:"oxillama_endpoint,omitempty"`
	OllamaEndpoint   string           `json:"ollama_endpoint,omitempty"`
	LiteLLMEndpoint  string           `json:"litellm_endpoint,omitempty"`
	LiteLLMKey       string           `json:"litellm_key,omitempty"`
	RepoDir          string           `json:"repo_dir,omitempty"`
	AllowMocks       bool             `json:"allow_mocks,omitempty"`
}

// CommitResult encapsulates the outcome of a commit operation.
type CommitResult struct {
	CommitSHA   string           `json:"commit_sha"`
	Message     string           `json:"message"`
	Files       []string         `json:"files"`
	TicketID    string           `json:"ticket_id,omitempty"`
	ModelUsed   string           `json:"model_used,omitempty"`
	CostUSD     float64          `json:"cost_usd"`
	DryRun      bool             `json:"dry_run,omitempty"`
	SmartTicket *SmartTicketInfo `json:"smart_ticket,omitempty"`
	SmartSource string           `json:"smart_source,omitempty"`
	Pushed      bool             `json:"pushed,omitempty"`
	PushSkipped bool             `json:"push_skipped,omitempty"`
	PushReason  string           `json:"push_reason,omitempty"`
	PushError   error            `json:"push_error,omitempty"`
}

// SmartTicketInfo represents ticket intelligence resolved automatically.
type SmartTicketInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Scope       string `json:"scope"`
	Type        string `json:"type"`
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
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

// ResolveSmartTicket automatically detects active ticket provenance using:
// 1. Current Git branch name
// 2. Authoritative Plane v2 live issues (In Progress, file correlation with staged changes, or recent transitions)
// 3. Repository documentation / DEV_STATUS.md file correlation fallback
func ResolveSmartTicket(ctx context.Context, repoDir string, stagedFiles []string) (*SmartTicketInfo, error) {
	// 1. Try Git Branch
	gitBranchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if repoDir != "" {
		gitBranchCmd.Dir = repoDir
	}
	if out, err := gitBranchCmd.Output(); err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "main" && branch != "master" && branch != "develop" {
			re := regexp.MustCompile(`\b(DEN-\d+|DEND-\d+)\b`)
			if m := re.FindString(branch); m != "" {
				return &SmartTicketInfo{
					ID:     m,
					Title:  fmt.Sprintf("Work from branch %s", branch),
					Scope:  inferScopeFromFiles(stagedFiles),
					Type:   "feat",
					Source: fmt.Sprintf("Git Branch (%s)", branch),
				}, nil
			}
		}
	}

	// 2. Query Plane v2 live API if available
	if pClient, err := plane.NewClientFromEnv(); err == nil {
		planeIssues, pErr := fetchPlaneIssues(ctx, pClient)
		if pErr == nil && len(planeIssues) > 0 {
			// 2a. Look for an In Progress ticket
			for _, iss := range planeIssues {
				// State group or slug check for in-progress
				if iss.StateSlug == "in-progress" || iss.State == "8e46611a-87ed-457f-8be0-3c238b37e6eb" {
					tid, title := extractTicketIDAndTitle(iss.Name)
					if tid != "" {
						return &SmartTicketInfo{
							ID:          tid,
							Title:       title,
							Scope:       inferScopeFromFiles(stagedFiles),
							Type:        inferTypeFromTitle(title),
							Source:      "Plane v2 (In Progress)",
							Description: iss.DescriptionHTML,
						}, nil
					}
				}
			}

			// 2b. Match staged files against Plane issues
			type scoredTicket struct {
				info  SmartTicketInfo
				score int
			}
			var scored []scoredTicket

			for _, iss := range planeIssues {
				tid, title := extractTicketIDAndTitle(iss.Name)
				if tid == "" {
					continue
				}
				searchCorpus := strings.ToLower(iss.Name + " " + iss.DescriptionHTML)
				score := 0
				for _, f := range stagedFiles {
					fLower := strings.ToLower(f)
					baseLower := strings.ToLower(filepath.Base(f))
					dirLower := strings.ToLower(filepath.Dir(f))
					if strings.Contains(searchCorpus, fLower) {
						score += 15
					} else if strings.Contains(searchCorpus, baseLower) && len(baseLower) > 4 {
						score += 8
					} else if dirLower != "." && strings.Contains(searchCorpus, dirLower) {
						score += 5
					}
				}

				// Bonus if recently updated
				if iss.State == "f36249b3-f67a-427a-ab8a-cf1693ebdada" || iss.StateSlug == "to-verify" {
					score += 10
				}

				if score > 0 {
					scored = append(scored, scoredTicket{
						info: SmartTicketInfo{
							ID:          tid,
							Title:       title,
							Scope:       inferScopeFromFiles(stagedFiles),
							Type:        inferTypeFromTitle(title),
							Source:      fmt.Sprintf("Plane v2 (Diff Correlation Score %d)", score),
							Description: iss.DescriptionHTML,
						},
						score: score,
					})
				}
			}

			if len(scored) > 0 {
				sort.Slice(scored, func(i, j int) bool {
					return scored[i].score > scored[j].score
				})
				best := scored[0].info
				return &best, nil
			}

			// 2c. Most recent ticket transitioned to "To Verify"
			for _, iss := range planeIssues {
				if iss.State == "f36249b3-f67a-427a-ab8a-cf1693ebdada" || iss.StateSlug == "to-verify" {
					tid, title := extractTicketIDAndTitle(iss.Name)
					if tid != "" {
						return &SmartTicketInfo{
							ID:          tid,
							Title:       title,
							Scope:       inferScopeFromFiles(stagedFiles),
							Type:        inferTypeFromTitle(title),
							Source:      "Plane v2 (Recent To Verify)",
							Description: iss.DescriptionHTML,
						}, nil
					}
				}
			}
		}
	}

	// 3. Fallback: Parse DEV_STATUS.md
	devStatusPath := filepath.Join(repoDir, "DEV_STATUS.md")
	if data, err := os.ReadFile(devStatusPath); err == nil {
		content := string(data)
		reTicket := regexp.MustCompile(`\*\*(DEN-\d+|DEND-\d+)\*\*:\s*([^\n\r]+)`)
		matches := reTicket.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			tid := m[1]
			title := strings.TrimSpace(m[2])
			for _, f := range stagedFiles {
				base := filepath.Base(f)
				dir := filepath.Dir(f)
				if strings.Contains(strings.ToLower(title), strings.ToLower(base)) ||
					(dir != "." && strings.Contains(strings.ToLower(title), strings.ToLower(dir))) {
					return &SmartTicketInfo{
						ID:     tid,
						Title:  title,
						Scope:  inferScopeFromFiles(stagedFiles),
						Type:   inferTypeFromTitle(title),
						Source: "DEV_STATUS.md (File Correlation)",
					}, nil
				}
			}
		}
	}

	// 4. Fallback: Default smart correlation from changed files
	scope := inferScopeFromFiles(stagedFiles)
	return &SmartTicketInfo{
		ID:     "",
		Title:  fmt.Sprintf("Update %s module components", scope),
		Scope:  scope,
		Type:   "feat",
		Source: "Heuristic Staged File Inference",
	}, nil
}

type planeIssueLite struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	State           string `json:"state"`
	StateSlug       string `json:"state_slug"`
	DescriptionHTML string `json:"description_html"`
	UpdatedAt       string `json:"updated_at"`
}

func fetchPlaneIssues(ctx context.Context, client *plane.Client) ([]planeIssueLite, error) {
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/projects/%s/issues/?order_by=-updated_at",
		client.BaseURL(), client.WorkspaceSlug(), client.ProjectID())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv("PLANE_API_KEY")
	if apiKey == "" {
		apiKey = "plane_sec_dendrite_token_12345"
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	httpCli := &http.Client{Timeout: 4 * time.Second}
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plane api status %d", resp.StatusCode)
	}

	var res struct {
		Results []planeIssueLite `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Results, nil
}

func extractTicketIDAndTitle(name string) (string, string) {
	re := regexp.MustCompile(`\[(DEN-\d+|DEND-\d+)\]\s*(.*)`)
	matches := re.FindStringSubmatch(name)
	if len(matches) >= 3 {
		return matches[1], strings.TrimSpace(matches[2])
	}
	reSolo := regexp.MustCompile(`\b(DEN-\d+|DEND-\d+)\b`)
	if tid := reSolo.FindString(name); tid != "" {
		return tid, strings.TrimSpace(strings.ReplaceAll(name, tid, ""))
	}
	return "", strings.TrimSpace(name)
}

func inferScopeFromFiles(files []string) string {
	for _, f := range files {
		clean := filepath.ToSlash(f)
		if strings.HasPrefix(clean, "pkg/devtool") || strings.Contains(clean, "commit") {
			return "devtool"
		}
		if strings.HasPrefix(clean, "pkg/gateway") {
			return "gateway"
		}
		if strings.HasPrefix(clean, "pkg/rag") || strings.HasPrefix(clean, "cmd/dendrite/ask") {
			return "rag"
		}
		if strings.HasPrefix(clean, "pkg/agent/triage") {
			return "triage"
		}
		if strings.HasPrefix(clean, "pkg/inference") {
			return "inference"
		}
		if strings.HasPrefix(clean, "pkg/env") {
			return "env"
		}
		if strings.HasPrefix(clean, "cmd/dendrite") {
			return "cli"
		}
		if strings.HasPrefix(clean, "docs/") {
			return "docs"
		}
		if strings.HasPrefix(clean, "scripts/") {
			return "scripts"
		}
	}
	return "repo"
}

func inferTypeFromTitle(title string) string {
	lower := strings.ToLower(title)
	if strings.HasPrefix(lower, "fix") || strings.Contains(lower, "bug") {
		return "fix"
	}
	if strings.HasPrefix(lower, "doc") || strings.Contains(lower, "guide") {
		return "docs"
	}
	if strings.Contains(lower, "refactor") {
		return "refactor"
	}
	if strings.Contains(lower, "test") {
		return "test"
	}
	if strings.Contains(lower, "ci") || strings.Contains(lower, "pipeline") {
		return "ci"
	}
	return "feat"
}

func isCannedOrInvalidCommitMessage(msg string) bool {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "zero cloud tokens") ||
		strings.Contains(lower, "processed your request safely") ||
		strings.Contains(lower, "pure-rust zero-ffi runner") ||
		strings.Contains(lower, "responded via") ||
		strings.Contains(lower, "as an ai") ||
		strings.Contains(lower, "here is the commit message") ||
		strings.Contains(lower, "here's the commit message") {
		return true
	}

	firstLine := strings.Split(trimmed, "\n")[0]
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "[") && strings.Contains(firstLine, "]") {
		idx := strings.Index(firstLine, "]")
		firstLine = strings.TrimSpace(firstLine[idx+1:])
	}

	validTypes := []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"}
	hasType := false
	for _, vt := range validTypes {
		if strings.HasPrefix(firstLine, vt+"(") || strings.HasPrefix(firstLine, vt+":") {
			hasType = true
			break
		}
	}
	return !hasType
}

func synthesizeDeterministicCommitMessage(ticketID string, smart *SmartTicketInfo, diffStat string, stagedFiles []string) string {
	scope := "repo"
	commitType := "feat"
	summary := "autonomous repository update"

	if smart != nil {
		if smart.Scope != "" {
			scope = smart.Scope
		}
		if smart.Type != "" {
			commitType = smart.Type
		}
		if smart.Title != "" {
			cleanTitle := strings.TrimSpace(smart.Title)
			cleanTitle = strings.TrimPrefix(cleanTitle, fmt.Sprintf("[%s]", smart.ID))
			cleanTitle = strings.TrimSpace(cleanTitle)
			if len(cleanTitle) > 0 {
				r := []rune(cleanTitle)
				summary = strings.ToLower(string(r[0])) + string(r[1:])
			}
		}
	} else if diffStat != "" {
		firstLine := strings.Split(diffStat, "\n")[0]
		return fmt.Sprintf("chore: update files (%s)", strings.TrimSpace(firstLine))
	} else if len(stagedFiles) > 0 {
		scope = inferScopeFromFiles(stagedFiles)
		summary = fmt.Sprintf("update %s capabilities and synchronizations", scope)
	}

	var sb strings.Builder
	firstLine := fmt.Sprintf("%s(%s): %s", commitType, scope, summary)
	sb.WriteString(firstLine)

	if len(stagedFiles) > 0 {
		sb.WriteString("\n\nKey changes:")
		limit := min(8, len(stagedFiles))
		for i := 0; i < limit; i++ {
			sb.WriteString(fmt.Sprintf("\n- Modify %s", stagedFiles[i]))
		}
		if len(stagedFiles) > limit {
			sb.WriteString(fmt.Sprintf("\n- ...and %d additional files", len(stagedFiles)-limit))
		}
	}

	return sb.String()
}

// GenerateLocalCommitMessage synthesizes a Conventional Commit message using local models.
func GenerateLocalCommitMessage(ctx context.Context, diffStat, fullDiff, ticketID, model string, ollamaURL, litellmURL, litellmKey string) (string, string, error) {
	return GenerateLocalCommitMessageWithContext(ctx, diffStat, fullDiff, ticketID, nil, model, ollamaURL, litellmURL, litellmKey, nil)
}

// GenerateLocalCommitMessageWithContext synthesizes Conventional Commit with active ticket context.
func GenerateLocalCommitMessageWithContext(ctx context.Context, diffStat, fullDiff, ticketID string, smartTicket *SmartTicketInfo, model string, ollamaURL, litellmURL, litellmKey string, stagedFiles []string) (string, string, error) {
	if model == "" {
		model = "qwen3.8:27b"
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

	// 1. Try OxiLLaMa Pure-Rust Zero-FFI Engine
	oxillamaURL := os.Getenv("DENDRITE_OXILLAMA_ENDPOINT")
	if oxillamaURL == "" {
		oxillamaURL = "http://127.0.0.1:8080"
	}
	tryOxillama := strings.Contains(strings.ToLower(model), "oxillama") ||
		ollamaURL == "" || ollamaURL == "http://localhost:11434" || ollamaURL == "http://127.0.0.1:11434" ||
		os.Getenv("DENDRITE_COMMIT_PREFER_OXILLAMA") == "true"

	if tryOxillama {
		oxModel := strings.TrimPrefix(model, "oxillama/")
		if oxModel == "" || !strings.Contains(oxModel, "qwen") {
			oxModel = "qwen3.8:27b"
		}
		oxPayload := map[string]interface{}{
			"model": oxModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"max_tokens": 256,
		}
		oxBytes, _ := json.Marshal(oxPayload)
		oxReq, oxErr := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(oxillamaURL, "/")+"/v1/chat/completions", bytes.NewReader(oxBytes))
		if oxErr == nil {
			oxReq.Header.Set("Content-Type", "application/json")
			oxClient := &http.Client{Timeout: 3 * time.Second}
			oxResp, oxDoErr := oxClient.Do(oxReq)
			if oxDoErr == nil {
				defer oxResp.Body.Close()
				if oxResp.StatusCode == http.StatusOK {
					var oxRes struct {
						Choices []struct {
							Message struct {
								Content string `json:"content"`
							} `json:"message"`
						} `json:"choices"`
					}
					if json.NewDecoder(oxResp.Body).Decode(&oxRes) == nil && len(oxRes.Choices) > 0 {
						cleaned := cleanCommitText(oxRes.Choices[0].Message.Content)
						if !isCannedOrInvalidCommitMessage(cleaned) {
							return FormatTicketCommit(ticketID, cleaned), oxModel + " (OxiLLaMa HTTP)", nil
						}
					}
				}
			}
		}

		// Fallback to OxiLLaMa CLI ($DENDRITE_OXILLAMA_BIN, synapse member builds,
		// $DENDRITE_HOME/bin, PATH, ~/.cargo/bin)
		oxillamaPath, _ := sidecar.DefaultLocator().Locate("oxillama")
		if oxillamaPath != "" {
			modelBaseDir := os.Getenv("DENDRITE_OXILLAMA_MODELS_DIR")
			if modelBaseDir == "" {
				home, _ := os.UserHomeDir()
				modelBaseDir = filepath.Join(home, ".oxillama", "models")
			}
			normModel := strings.ReplaceAll(strings.TrimPrefix(model, "oxillama/"), ":", "_")
			modelDir := filepath.Join(modelBaseDir, normModel)
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
					if !isCannedOrInvalidCommitMessage(cleaned) {
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
	alreadyStaged, _ := runGit("diff", "--cached", "--name-only")
	if strings.TrimSpace(alreadyStaged) != "" && !opts.StageAll && len(opts.Files) == 0 {
		// Developer or agent already staged specific files; do not clobber with git add -A
	} else if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			if strings.TrimSpace(f) != "" {
				if _, err := runGit("add", strings.TrimSpace(f)); err != nil {
					return nil, err
				}
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
	stagedStatus, err := runGit("diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to check staged files: %w", err)
	}
	if strings.TrimSpace(stagedStatus) == "" && !opts.Amend {
		return nil, fmt.Errorf("no staged changes to commit")
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

	diffStat, _ := runGit("diff", "--cached", "--stat")
	fullDiff, _ := runGit("diff", "--cached")
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
	if _, err := runGit(gitArgs...); err != nil {
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

	remotesOut, err := runGit("remote")
	if err != nil {
		res.PushSkipped = true
		res.PushReason = "no remote configured"
		return nil
	}

	var remotes []string
	for _, r := range strings.Split(remotesOut, "\n") {
		r = strings.TrimSpace(r)
		if r != "" {
			remotes = append(remotes, r)
		}
	}
	if len(remotes) == 0 {
		res.PushSkipped = true
		res.PushReason = "no remote configured"
		return nil
	}

	branch, _ := runGit("rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)

	if branch == "main" || branch == "master" {
		refuseErr := fmt.Errorf("pushing %s directly is refused (branch guard)", branch)
		res.PushSkipped = true
		res.PushReason = refuseErr.Error()
		res.PushError = refuseErr
		return refuseErr
	}

	targetRemote := ""
	for _, r := range remotes {
		if r == "origin" {
			targetRemote = "origin"
			break
		}
	}
	if targetRemote == "" {
		targetRemote = remotes[0]
	}

	hasUpstream := false
	if _, uErr := runGit("rev-parse", "--abbrev-ref", "@{u}"); uErr == nil {
		hasUpstream = true
	}

	var pushErr error
	if hasUpstream {
		_, pushErr = runGit("push")
	} else {
		_, pushErr = runGit("push", "-u", targetRemote, branch)
	}

	if pushErr != nil {
		res.PushError = pushErr
		res.Pushed = false
		return pushErr
	}

	res.Pushed = true
	return nil
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
