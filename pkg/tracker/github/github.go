package github

import (
	"bytes"
	"context"
	"encoding/json"
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

// NewGitHubClient creates a new client targeting a repository (e.g. "yasuomaidana/dendrite").
func NewGitHubClient(repo string) *GitHubClient {
	if repo == "" {
		repo = "yasuomaidana/dendrite"
	}
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
		// Try creating label via gh label create
		_, err := c.Executor.Execute(ctx, "gh", "label", "create", l.Name,
			"--repo", c.Repo,
			"--color", l.Color,
			"--description", l.Description,
			"--force")
		if err != nil {
			// If already exists or error, continue
			continue
		}
	}
	return nil
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
		args := []string{"issue", "edit", strconv.Itoa(t.GitHubIssueNum), "--repo", c.Repo, "--title", title, "--body", body}
		if len(labels) > 0 {
			args = append(args, "--add-label", strings.Join(labels, ","))
		}
		_, err := c.Executor.Execute(ctx, "gh", args...)
		if err == nil {
			return t.GitHubIssueNum, nil
		}
	}

	// Check if an issue with the ticket ID already exists in repo
	searchOut, err := c.Executor.Execute(ctx, "gh", "issue", "list",
		"--repo", c.Repo,
		"--search", fmt.Sprintf("[%s]", t.ID),
		"--json", "number,title",
		"--limit", "1")
	if err == nil && searchOut != "" {
		var found []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		}
		if jsonErr := json.Unmarshal([]byte(searchOut), &found); jsonErr == nil && len(found) > 0 {
			issueNum := found[0].Number
			t.GitHubIssueNum = issueNum
			args := []string{"issue", "edit", strconv.Itoa(issueNum), "--repo", c.Repo, "--title", title, "--body", body}
			if len(labels) > 0 {
				args = append(args, "--add-label", strings.Join(labels, ","))
			}
			_, _ = c.Executor.Execute(ctx, "gh", args...)
			return issueNum, nil
		}
	}

	// Create new issue
	args := []string{"issue", "create", "--repo", c.Repo, "--title", title, "--body", body}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	out, err := c.Executor.Execute(ctx, "gh", args...)
	if err != nil {
		return 0, fmt.Errorf("failed to create GitHub issue for %s: %w", t.ID, err)
	}

	// gh issue create returns the URL, e.g. https://github.com/yasuomaidana/dendrite/issues/1
	parts := strings.Split(strings.TrimSpace(out), "/")
	if len(parts) > 0 {
		if num, parseErr := strconv.Atoi(parts[len(parts)-1]); parseErr == nil {
			t.GitHubIssueNum = num
			return num, nil
		}
	}
	return 0, nil
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
	args := []string{"issue", "close", strconv.Itoa(issueNum), "--repo", c.Repo}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	_, err := c.Executor.Execute(ctx, "gh", args...)
	return err
}

// RemoveLabel removes a specific label from an issue.
func (c *GitHubClient) RemoveLabel(ctx context.Context, issueNum int, label string) error {
	_, err := c.Executor.Execute(ctx, "gh", "issue", "edit", strconv.Itoa(issueNum), "--repo", c.Repo, "--remove-label", label)
	return err
}
