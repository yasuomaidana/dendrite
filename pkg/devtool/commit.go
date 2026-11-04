package devtool

import (
	"Dendrite/pkg/branchpolicy"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// NoTicketGuard skips the ticket lifecycle check (see EnsureTicketInProgress).
	NoTicketGuard bool `json:"no_ticket_guard,omitempty"`
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
	// Guard is what the ticket lifecycle guard found; nil when it was off.
	Guard       *GuardResult `json:"guard,omitempty"`
	SmartSource string       `json:"smart_source,omitempty"`
	Pushed      bool         `json:"pushed,omitempty"`
	PushSkipped bool         `json:"push_skipped,omitempty"`
	PushReason  string       `json:"push_reason,omitempty"`
	PushError   error        `json:"push_error,omitempty"`
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

var (
	branchTicketRe    = regexp.MustCompile(`\b(DEN-\d+|DEND-\d+)\b`)
	devStatusTicketRe = regexp.MustCompile(`\*\*(DEN-\d+|DEND-\d+)\*\*:\s*([^\n\r]+)`)
)

// ResolveSmartTicket automatically detects active ticket provenance using:
// 1. Current Git branch name
// 2. Authoritative Plane v2 live issues (In Progress, file correlation with staged changes, or recent transitions)
// 3. Repository documentation / DEV_STATUS.md file correlation fallback
func ResolveSmartTicket(ctx context.Context, repoDir string, stagedFiles []string) (*SmartTicketInfo, error) {
	if info := ticketFromBranch(ctx, repoDir, stagedFiles); info != nil {
		return info, nil
	}
	if info := ticketFromPlane(ctx, stagedFiles); info != nil {
		return info, nil
	}
	if info := ticketFromDevStatus(repoDir, stagedFiles); info != nil {
		return info, nil
	}
	// Default smart correlation from changed files.
	scope := inferScopeFromFiles(stagedFiles)
	return &SmartTicketInfo{
		ID:     "",
		Title:  fmt.Sprintf("Update %s module components", scope),
		Scope:  scope,
		Type:   "feat",
		Source: "Heuristic Staged File Inference",
	}, nil
}

// ticketFromBranch reads the ticket out of the checked-out branch's name. The project's protected branches (its
// integration branch, main, master) name no ticket.
func ticketFromBranch(ctx context.Context, repoDir string, stagedFiles []string) *SmartTicketInfo {
	gitBranchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if repoDir != "" {
		gitBranchCmd.Dir = repoDir
	}
	out, err := gitBranchCmd.Output()
	if err != nil {
		return nil
	}
	branch := strings.TrimSpace(string(out))
	if branchpolicy.For(repoDir).IsProtected(branch) {
		return nil
	}
	m := branchTicketRe.FindString(branch)
	if m == "" {
		return nil
	}
	return &SmartTicketInfo{
		ID:     m,
		Title:  fmt.Sprintf("Work from branch %s", branch),
		Scope:  inferScopeFromFiles(stagedFiles),
		Type:   "feat",
		Source: fmt.Sprintf("Git Branch (%s)", branch),
	}
}

// planeTicket is the ticket an issue stands for, found by source.
func planeTicket(iss planeIssueLite, tid, title string, stagedFiles []string, source string) *SmartTicketInfo {
	return &SmartTicketInfo{
		ID:          tid,
		Title:       title,
		Scope:       inferScopeFromFiles(stagedFiles),
		Type:        inferTypeFromTitle(title),
		Source:      source,
		Description: iss.DescriptionHTML,
	}
}

// ticketFromPlane asks the live Plane board: the ticket In Progress, else the one whose text best matches the
// staged files, else a ticket recently moved to To Verify.
func ticketFromPlane(ctx context.Context, stagedFiles []string) *SmartTicketInfo {
	pClient, err := plane.NewClientFromEnv()
	if err != nil {
		return nil
	}
	issues, pErr := fetchPlaneIssues(ctx, pClient)
	if pErr != nil || len(issues) == 0 {
		return nil
	}
	stateOf := planeStateResolver(ctx, pClient)
	if info := firstInState(issues, stateOf, tracker.StatusInProgress, "Plane v2 (In Progress)", stagedFiles); info != nil {
		return info
	}
	if info := bestCorrelated(issues, stateOf, stagedFiles); info != nil {
		return info
	}
	return firstInState(issues, stateOf, tracker.StatusInReview, "Plane v2 (Recent To Verify)", stagedFiles)
}

// firstInState is the first issue in the state that carries a ticket id.
func firstInState(issues []planeIssueLite, stateOf func(planeIssueLite) tracker.Status, want tracker.Status, source string, stagedFiles []string) *SmartTicketInfo {
	for _, iss := range issues {
		if stateOf(iss) != want {
			continue
		}
		if tid, title := extractTicketIDAndTitle(iss.Name); tid != "" {
			return planeTicket(iss, tid, title, stagedFiles, source)
		}
	}
	return nil
}

// correlationScore rates how much of an issue's text mentions the staged files: a full path counts most, then
// the file name, then its directory; a ticket in To Verify gets a bonus.
func correlationScore(corpus string, status tracker.Status, stagedFiles []string) int {
	score := 0
	for _, f := range stagedFiles {
		fLower := strings.ToLower(f)
		baseLower := strings.ToLower(filepath.Base(f))
		dirLower := strings.ToLower(filepath.Dir(f))
		switch {
		case strings.Contains(corpus, fLower):
			score += 15
		case strings.Contains(corpus, baseLower) && len(baseLower) > 4:
			score += 8
		case dirLower != "." && strings.Contains(corpus, dirLower):
			score += 5
		}
	}
	if status == tracker.StatusInReview {
		score += 10
	}
	return score
}

// bestCorrelated is the issue whose text matches the staged files best, or nil when none matches.
func bestCorrelated(issues []planeIssueLite, stateOf func(planeIssueLite) tracker.Status, stagedFiles []string) *SmartTicketInfo {
	var best *SmartTicketInfo
	bestScore := 0
	for _, iss := range issues {
		tid, title := extractTicketIDAndTitle(iss.Name)
		if tid == "" {
			continue
		}
		score := correlationScore(strings.ToLower(iss.Name+" "+iss.DescriptionHTML), stateOf(iss), stagedFiles)
		if score > bestScore {
			bestScore = score
			best = planeTicket(iss, tid, title, stagedFiles, fmt.Sprintf("Plane v2 (Diff Correlation Score %d)", score))
		}
	}
	return best
}

// ticketFromDevStatus matches the staged files against the tickets listed in DEV_STATUS.md.
func ticketFromDevStatus(repoDir string, stagedFiles []string) *SmartTicketInfo {
	data, err := os.ReadFile(filepath.Join(repoDir, "DEV_STATUS.md"))
	if err != nil {
		return nil
	}
	for _, m := range devStatusTicketRe.FindAllStringSubmatch(string(data), -1) {
		title := strings.TrimSpace(m[2])
		if mentionsStagedFile(strings.ToLower(title), stagedFiles) {
			return &SmartTicketInfo{ID: m[1], Title: title, Scope: inferScopeFromFiles(stagedFiles), Type: inferTypeFromTitle(title), Source: "DEV_STATUS.md (File Correlation)"}
		}
	}
	return nil
}

// mentionsStagedFile reports whether the lower-cased text names a staged file or its directory.
func mentionsStagedFile(text string, stagedFiles []string) bool {
	for _, f := range stagedFiles {
		dir := filepath.Dir(f)
		if strings.Contains(text, strings.ToLower(filepath.Base(f))) || (dir != "." && strings.Contains(text, strings.ToLower(dir))) {
			return true
		}
	}
	return false
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
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/?order_by=-updated_at",
		client.WorkspaceSlug(), client.ProjectID())
	req, err := client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

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
