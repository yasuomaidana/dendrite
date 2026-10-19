package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"Dendrite/pkg/tracker"
)

// LabelDefinition defines the color and description for semantic labels.
type LabelDefinition struct {
	Name        string
	Color       string
	Description string
}

var (
	// DefaultRequiredLabels are the mandatory semantic governance labels.
	DefaultRequiredLabels = []LabelDefinition{
		{Name: "P0-TOP-BLOCKER", Color: "b60205", Description: "Top 1 priority blocker halting all lower-priority feature work"},
		{Name: "local-llm-delegated", Color: "1d76db", Description: "Work item delegated to zero-cost local LLM"},
		{Name: "needs-vendor-audit", Color: "fbca04", Description: "Speculative local branch awaiting vendor LLM review and rebase"},
	}
)

// CommandExecutor abstracts shell command execution (e.g. `gh` CLI).
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) (string, error)
}

// DefaultCommandExecutor uses os/exec.
type DefaultCommandExecutor struct{}

func (d *DefaultCommandExecutor) Execute(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s failed: %w (stderr: %s)", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GitHubClient handles issue synchronization and label provisioning.
type GitHubClient struct {
	Repo     string
	Executor CommandExecutor
}

// NewGitHubClient creates a new client targeting a repository ("owner/repo"). An empty
// repo lets gh resolve the repository from the current directory's git remote.
func NewGitHubClient(repo string) *GitHubClient {
	return &GitHubClient{
		Repo:     repo,
		Executor: &DefaultCommandExecutor{},
	}
}

// WithExecutor configures a custom executor for testing.
func (c *GitHubClient) WithExecutor(executor CommandExecutor) *GitHubClient {
	c.Executor = executor
	return c
}

// ProvisionLabels ensures required semantic labels exist on the remote repository.
// Existing labels are left untouched (their colors and descriptions are not reset).
func (c *GitHubClient) ProvisionLabels(ctx context.Context, moduleNames ...string) error {
	labels := append([]LabelDefinition{}, DefaultRequiredLabels...)
	for _, mod := range moduleNames {
		if mod != "" {
			labels = append(labels, LabelDefinition{
				Name:        "module:" + mod,
				Color:       "5319e7",
				Description: fmt.Sprintf("Belongs to module %s", mod),
			})
		}
	}

	for _, l := range labels {
		c.ensureLabel(ctx, l)
	}
	return nil
}

// ensureLabel creates a label when it does not exist yet; gh fails harmlessly when it does.
func (c *GitHubClient) ensureLabel(ctx context.Context, l LabelDefinition) {
	_, _ = c.Executor.Execute(ctx, "gh", c.repoArgs("label", "create", l.Name,
		"--color", l.Color,
		"--description", l.Description)...)
}

// BuildIssueBody formats the markdown body for a ticket including DAG dependency links.
func BuildIssueBody(t *tracker.RoutingTicket, idToIssueNum map[string]int) string {
	var sb strings.Builder
	sb.WriteString(t.Description)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("**Plane/Internal Ticket ID**: `%s`\n", t.ID))
	sb.WriteString(fmt.Sprintf("**Difficulty Tier**: `%s`\n", t.Difficulty.String()))
	sb.WriteString(fmt.Sprintf("**Priority**: `%s`\n", t.Priority))
	sb.WriteString(fmt.Sprintf("**Local Feasible**: `%v`\n", t.LocalFeasible))

	if len(t.DependsOn) > 0 {
		sb.WriteString("\n### Dependencies\n")
		for _, dep := range t.DependsOn {
			if num, ok := idToIssueNum[dep]; ok && num > 0 {
				sb.WriteString(fmt.Sprintf("- Depends on #%d (`%s`)\n", num, dep))
			} else {
				sb.WriteString(fmt.Sprintf("- Depends on `%s`\n", dep))
			}
		}
	}

	if len(t.Blocks) > 0 {
		sb.WriteString("\n### Blocks Downstream\n")
		for _, blk := range t.Blocks {
			if num, ok := idToIssueNum[blk]; ok && num > 0 {
				sb.WriteString(fmt.Sprintf("- Blocks #%d (`%s`)\n", num, blk))
			} else {
				sb.WriteString(fmt.Sprintf("- Blocks `%s`\n", blk))
			}
		}
	}

	return sb.String()
}

// CreateOrUpdateIssue creates or updates an issue on GitHub.
func (c *GitHubClient) CreateOrUpdateIssue(ctx context.Context, t *tracker.RoutingTicket, idToIssueNum map[string]int) (int, error) {
	title := fmt.Sprintf("[%s] %s", t.ID, t.Title)
	body := BuildIssueBody(t, idToIssueNum)

	var labels []string
	if t.Priority == "P0_TOP_BLOCKER" || t.Priority == "urgent" {
		labels = append(labels, "P0-TOP-BLOCKER")
	}
	if t.LocalFeasible {
		labels = append(labels, "local-llm-delegated")
	}
	if t.RequiresAudit {
		labels = append(labels, "needs-vendor-audit")
	}
	if t.Module != "" {
		labels = append(labels, "module:"+t.Module)
	}

	// If issue number is already known, check if update is needed
	if t.GitHubIssueNum > 0 {
		args := c.repoArgs("issue", "edit", strconv.Itoa(t.GitHubIssueNum), "--title", title, "--body", body)
		if len(labels) > 0 {
			args = append(args, "--add-label", strings.Join(labels, ","))
		}
		_, err := c.Executor.Execute(ctx, "gh", args...)
		if err == nil {
			return t.GitHubIssueNum, nil
		}
	}

	// Reuse an issue whose title carries exactly this ticket ID ("[DEN-70]" never matches "[DEN-701]").
	if issueNum, ok := c.findTicketIssue(ctx, t.ID); ok {
		t.GitHubIssueNum = issueNum
		args := c.repoArgs("issue", "edit", strconv.Itoa(issueNum), "--title", title, "--body", body)
		if len(labels) > 0 {
			args = append(args, "--add-label", strings.Join(labels, ","))
		}
		_, _ = c.Executor.Execute(ctx, "gh", args...)
		return issueNum, nil
	}

	// Create new issue
	args := c.repoArgs("issue", "create", "--title", title, "--body", body)
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	out, err := c.Executor.Execute(ctx, "gh", args...)
	if err != nil {
		return 0, fmt.Errorf("failed to create GitHub issue for %s: %w", t.ID, err)
	}

	if num, ok := issueNumberFromURL(out); ok {
		t.GitHubIssueNum = num
		return num, nil
	}
	return 0, nil
}

// issueNumberFromURL parses the number from the URL `gh issue create` prints
// (https://github.com/<owner>/<repo>/issues/<n>).
func issueNumberFromURL(out string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(out), "/")
	num, err := strconv.Atoi(parts[len(parts)-1])
	return num, err == nil
}

// SyncTickets synchronizes a slice of sequenced tickets to GitHub Issues in topological order.
func (c *GitHubClient) SyncTickets(ctx context.Context, tickets []*tracker.RoutingTicket) ([]*tracker.RoutingTicket, error) {
	// 1. Gather all module names and provision labels
	var modules []string
	for _, t := range tickets {
		if t.Module != "" {
			modules = append(modules, t.Module)
		}
	}
	_ = c.ProvisionLabels(ctx, modules...)

	idToIssueNum := make(map[string]int)
	for _, t := range tickets {
		if t.GitHubIssueNum > 0 {
			idToIssueNum[t.ID] = t.GitHubIssueNum
		}
	}

	// 2. Publish or update issues
	for _, t := range tickets {
		num, err := c.CreateOrUpdateIssue(ctx, t, idToIssueNum)
		if err == nil && num > 0 {
			t.GitHubIssueNum = num
			idToIssueNum[t.ID] = num
		}
	}

	// 3. Second pass: update dependencies now that all issue numbers are registered
	for _, t := range tickets {
		if len(t.DependsOn) > 0 || len(t.Blocks) > 0 {
			_, _ = c.CreateOrUpdateIssue(ctx, t, idToIssueNum)
		}
	}

	return tickets, nil
}

// CloseIssue closes an issue with an optional comment.
func (c *GitHubClient) CloseIssue(ctx context.Context, issueNum int, comment string) error {
	args := c.repoArgs("issue", "close", strconv.Itoa(issueNum))
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	_, err := c.Executor.Execute(ctx, "gh", args...)
	return err
}

// RemoveLabel removes a specific label from an issue.
func (c *GitHubClient) RemoveLabel(ctx context.Context, issueNum int, label string) error {
	_, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "edit", strconv.Itoa(issueNum), "--remove-label", label)...)
	return err
}
