package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"
)

// AuditFinishedTicket executes anti-mock verification, local LLM analysis, and 100% coverage gap generation.
func AuditFinishedTicket(
	ctx context.Context,
	repoDir string,
	ticket *IssueSpec,
	pkgPath string,
	localLLM AnalyzerClient,
	agyClient AnalyzerClient,
) (gapTickets []*IssueSpec, err error) {
	// 1. Remap legacy "DONE" or "FINISHED" to "TO_VERIFY"
	if ticket.State == StateDone || ticket.State == "FINISHED" || ticket.State == "COMPLETED" {
		ticket.State = StateToVerify
	}

	// 2. Deterministic Anti-Mock Static Scan
	hasMocks, mockLocations, err := ScanForMocks(repoDir, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("anti-mock scan failed for %s: %w", pkgPath, err)
	}
	if hasMocks {
		// Demote back to TO_DO immediately
		ticket.State = StateToDo
		ticket.ProposedResolution += fmt.Sprintf("\n[AUDIT REJECTION]: Mocks/stubs detected at %v. Must replace with real implementations and fixtures.", mockLocations)
		return nil, nil
	}

	// 3. Local LLM Analysis with single-attempt before AGY escalation
	if localLLM == nil {
		localLLM = &LocalOllamaAnalyzer{Model: "qwen3.8:27b"}
	}
	if agyClient == nil {
		agyClient = &AGYFallbackAnalyzer{LogPath: filepath.Join(repoDir, ".dendrite", "agy_escalations.log")}
	}

	res, lErr := localLLM.AnalyzeFeature(ctx, pkgPath, "")
	if lErr != nil || res.TooComplexForLocal {
		res, lErr = agyClient.AnalyzeFeature(ctx, pkgPath, "")
		if lErr != nil {
			return nil, fmt.Errorf("AGY escalation failed for %s: %w", ticket.ID, lErr)
		}
	}

	if res.HasLogicalBugs {
		ticket.State = StateToDo
		ticket.ProposedResolution += "\n[AUDIT REJECTION]: " + res.DiagnosticNotes
		return nil, nil
	}

	// 4. Execute Tests & Coverage Profiling
	uncoveredBlocks, _, testFailed, testOutput := RunCoverageAudit(ctx, repoDir, pkgPath)
	if testFailed {
		ticket.State = StateToDo
		ticket.ReproductionSteps = fmt.Sprintf("Run `go test -race -v %s`\nTest Failure Output:\n%s", pkgPath, testOutput)
		return nil, nil
	}

	// 5. Generate Coverage Gap Tickets for every uncovered block to reach 100%
	for _, block := range uncoveredBlocks {
		gapTicket := &IssueSpec{
			ID:               fmt.Sprintf("%s-COV-%d", ticket.ID, len(gapTickets)+1),
			Title:            fmt.Sprintf("Coverage Gap: Reach 100%% coverage in %s", block),
			ProblemStatement: fmt.Sprintf("Coverage audit detected untested statement block at `%s` in package `%s`.", block, pkgPath),
			ReproductionSteps: fmt.Sprintf("Run: go test -race -covermode=atomic -coverprofile=coverage.out %s && go tool cover -func=coverage.out\nObserve uncovered statement: %s",
				pkgPath, block),
			CodebaseLocation:   block,
			ProposedResolution: fmt.Sprintf("Write non-mocked integration/unit tests exercising the code block at `%s`.", block),
			AcceptanceCriteria: []string{
				"Zero mocks or stubs used in test implementation",
				fmt.Sprintf("100.0%% statement coverage verified for block %s", block),
				"Passes go test -race ./...",
			},
			Priority:       "P0_TOP_BLOCKER",
			DifficultyTier: tracker.TierL2Low,
			LocalFeasible:  true,
			State:          StateToDo,
			CreatedByLLM:   true,
			DependsOn:      []string{ticket.ID},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		gapTickets = append(gapTickets, gapTicket)
	}

	return gapTickets, nil
}

// GitHubHITLPoller manages Human-in-the-Loop verification state sync with GitHub Issues.
type GitHubHITLPoller struct {
	Repo     string
	Executor github.CommandExecutor
}

func NewGitHubHITLPoller(repo string, exec github.CommandExecutor) *GitHubHITLPoller {
	if exec == nil {
		exec = &github.DefaultCommandExecutor{}
	}
	return &GitHubHITLPoller{Repo: repo, Executor: exec}
}

// SyncValidationState polls GitHub issues for tickets in TO_VERIFY.
// Case 1: If issue is closed -> transition to DONE.
// Case 2: If open with new comments -> parse feedback, create follow-up ticket, move parent back to TO_DO.
func (p *GitHubHITLPoller) SyncValidationState(ctx context.Context, ticket *IssueSpec) (followUpTicket *IssueSpec, err error) {
	if ticket.GitHubIssueNum <= 0 {
		return nil, nil
	}

	// Fetch issue status and comments via gh issue view
	args := []string{"issue", "view", strconv.Itoa(ticket.GitHubIssueNum), "--json", "state,comments,updatedAt"}
	if p.Repo != "" { // empty: gh resolves the repository from the git remote
		args = append(args, "--repo", p.Repo)
	}
	out, err := p.Executor.Execute(ctx, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub issue #%d: %w", ticket.GitHubIssueNum, err)
	}

	var issueData struct {
		State     string `json:"state"` // OPEN or CLOSED
		UpdatedAt string `json:"updatedAt"`
		Comments  []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body      string `json:"body"`
			CreatedAt string `json:"createdAt"`
		} `json:"comments"`
	}

	if jErr := json.Unmarshal([]byte(out), &issueData); jErr != nil {
		return nil, fmt.Errorf("failed to parse issue JSON: %w", jErr)
	}

	// Case 1: User closed the validation issue -> human sign-off approved -> DONE
	if strings.ToUpper(issueData.State) == "CLOSED" {
		ticket.State = StateDone
		ticket.UpdatedAt = time.Now()
		return nil, nil
	}

	// Case 2: Issue remains open; check for human feedback comments after ticket.LastSyncedAt
	var newHumanFeedback []string
	for _, c := range issueData.Comments {
		// Ignore bot or self comments if desired
		if strings.TrimSpace(c.Body) != "" {
			commentTime, _ := time.Parse(time.RFC3339, c.CreatedAt)
			if ticket.LastSyncedAt.IsZero() || commentTime.After(ticket.LastSyncedAt) {
				newHumanFeedback = append(newHumanFeedback, fmt.Sprintf("@%s: %s", c.Author.Login, c.Body))
			}
		}
	}

	if len(newHumanFeedback) > 0 {
		feedbackText := strings.Join(newHumanFeedback, "\n\n")

		// Demote parent to TO_DO
		ticket.State = StateToDo
		ticket.UpdatedAt = time.Now()

		// Generate new follow-up remediation ticket
		followUpTicket = &IssueSpec{
			ID:               fmt.Sprintf("%s-HITL-FIX", ticket.ID),
			Title:            fmt.Sprintf("HITL Feedback Remediation: %s", ticket.Title),
			ProblemStatement: fmt.Sprintf("Human-in-the-loop validation failed with reviewer feedback:\n%s", feedbackText),
			ReproductionSteps: fmt.Sprintf("Review feedback on GitHub Issue #%d:\n%s",
				ticket.GitHubIssueNum, feedbackText),
			CodebaseLocation:   ticket.CodebaseLocation,
			ProposedResolution: fmt.Sprintf("Implement reviewer requested changes:\n%s", feedbackText),
			AcceptanceCriteria: []string{
				"All reviewer feedback points addressed",
				"Zero mocks/stubs used",
				"100.0% test coverage verified",
			},
			Priority:       "P0_TOP_BLOCKER",
			DifficultyTier: ticket.DifficultyTier,
			LocalFeasible:  ticket.LocalFeasible,
			State:          StateToDo,
			CreatedByLLM:   true,
			Blocks:         []string{ticket.ID},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	ticket.LastSyncedAt = time.Now()
	return followUpTicket, nil
}
