package devtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// ErrNothingToCommit is returned by ExecuteCommit when the working tree has nothing to stage.
var ErrNothingToCommit = errors.New("no staged changes to commit")

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
			stateOf := planeStateResolver(ctx, pClient)
			// 2a. Look for an In Progress ticket
			for _, iss := range planeIssues {
				// State group or slug check for in-progress
				if stateOf(iss) == tracker.StatusInProgress {
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
				if stateOf(iss) == tracker.StatusInReview {
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
				if stateOf(iss) == tracker.StatusInReview {
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

// planeStateResolver maps an issue to its tracker status through the project's own workflow
// states, so it works on every board; boards differ in state UUIDs, which must never be hard-coded.
// An issue whose state cannot be resolved falls back to the legacy state_slug field.
func planeStateResolver(ctx context.Context, client *plane.Client) func(planeIssueLite) tracker.Status {
	byID := map[string]tracker.Status{}
	if states, err := client.ListStates(ctx); err == nil {
		for _, st := range states {
			byID[st.ID] = plane.StatusFor(st.Group, st.Name)
		}
	}
	return func(iss planeIssueLite) tracker.Status {
		if st, ok := byID[iss.State]; ok {
			return st
		}
		switch iss.StateSlug {
		case "in-progress":
			return tracker.StatusInProgress
		case "to-verify":
			return tracker.StatusInReview
		}
		return tracker.StatusBacklog
	}
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
	re := regexp.MustCompile(`\[([A-Z][A-Z0-9]*-\d+)\]\s*(.*)`)
	matches := re.FindStringSubmatch(name)
	if len(matches) >= 3 {
		return matches[1], strings.TrimSpace(matches[2])
	}
	// Unbracketed ids: any project identifier at the start of the title, or a legacy DEN id anywhere.
	reSolo := regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d+\b|\b(?:DEN|DEND)-\d+\b`)
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
