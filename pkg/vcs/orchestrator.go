package vcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/tracker"
)

// AuditQueueStatus represents the audit lifecycle state of a speculative local branch.
type AuditQueueStatus string

const (
	StatusAwaitingAudit AuditQueueStatus = "AWAITING_AUDIT"
	StatusInAudit       AuditQueueStatus = "IN_AUDIT"
	StatusMerged        AuditQueueStatus = "MERGED"
	StatusAuditFailed   AuditQueueStatus = "AUDIT_FAILED"
)

// AuditQueueItem records metadata for a stacked branch requiring vendor LLM audit.
type AuditQueueItem struct {
	Branch      string           `json:"branch"`
	BaseBranch  string           `json:"base_branch"`
	Module      string           `json:"module"`
	Status      AuditQueueStatus `json:"status"`
	Tickets     []string         `json:"tickets"`
	BaseSHA     string           `json:"base_sha,omitempty"`
	HeadSHA     string           `json:"head_sha,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	AuditErrors []string         `json:"audit_errors,omitempty"`
}

// StackedBranchOrchestrator manages speculative stacked local branches and audit queues.
type StackedBranchOrchestrator struct {
	Guard        *GitGuard
	ManifestPath string
	mu           sync.Mutex
}

// NewStackedBranchOrchestrator creates an orchestrator instance.
func NewStackedBranchOrchestrator(guard *GitGuard, manifestPath string) *StackedBranchOrchestrator {
	if manifestPath == "" {
		manifestPath = filepath.Join(guard.RepoDir, ".dendrite", "audit_queue.json")
	}
	return &StackedBranchOrchestrator{
		Guard:        guard,
		ManifestPath: manifestPath,
	}
}

// IsDegradedBudget evaluates whether execution must enter DegradedBudgetMode based on <= 15% threshold.
func IsDegradedBudget(remainingTokens, totalTokens int64, forceLocal bool) bool {
	if forceLocal {
		return true
	}
	if totalTokens <= 0 {
		return false
	}
	ratio := float64(remainingTokens) / float64(totalTokens)
	return ratio <= 0.15
}

// LoadManifest reads the audit queue manifest from disk.
func (o *StackedBranchOrchestrator) LoadManifest() ([]*AuditQueueItem, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	data, err := os.ReadFile(o.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*AuditQueueItem{}, nil
		}
		return nil, fmt.Errorf("failed to read audit manifest: %w", err)
	}

	var items []*AuditQueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal audit manifest: %w", err)
	}
	return items, nil
}

// SaveManifest writes the audit queue manifest atomically.
func (o *StackedBranchOrchestrator) SaveManifest(items []*AuditQueueItem) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	dir := filepath.Dir(o.ManifestPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal audit manifest: %w", err)
	}

	tmpFile := o.ManifestPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp manifest: %w", err)
	}
	return os.Rename(tmpFile, o.ManifestPath)
}

// StartModuleBranch creates or checks out a stacked branch 'local/<module>' branched from 'develop' or parent branch.
func (o *StackedBranchOrchestrator) StartModuleBranch(ctx context.Context, module string, parentModule string) (string, error) {
	if module == "" {
		return "", errors.New("module name cannot be empty")
	}

	targetBranch := "local/" + strings.TrimPrefix(module, "local/")

	// Ensure base branch is set up
	baseBranch := "develop"
	if parentModule != "" {
		baseBranch = "local/" + strings.TrimPrefix(parentModule, "local/")
	}

	// Verify base exists or checkout
	_, _ = o.Guard.runGit(ctx, "checkout", baseBranch)

	// Check if target branch already exists
	_, err := o.Guard.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+targetBranch)
	if err == nil {
		// Target branch exists; checkout
		if _, chkErr := o.Guard.runGit(ctx, "checkout", targetBranch); chkErr != nil {
			return "", fmt.Errorf("failed to checkout %s: %w", targetBranch, chkErr)
		}
	} else {
		// Create stacked branch from baseBranch
		if _, bErr := o.Guard.runGit(ctx, "checkout", "-b", targetBranch, baseBranch); bErr != nil {
			return "", fmt.Errorf("failed to create branch %s from %s: %w", targetBranch, baseBranch, bErr)
		}
	}

	// Record initial base SHA
	baseSHA, _ := o.Guard.runGit(ctx, "rev-parse", baseBranch)

	// Update queue item
	items, _ := o.LoadManifest()
	var found *AuditQueueItem
	for _, it := range items {
		if it.Branch == targetBranch {
			found = it
			break
		}
	}

	now := time.Now()
	if found == nil {
		items = append(items, &AuditQueueItem{
			Branch:     targetBranch,
			BaseBranch: baseBranch,
			Module:     module,
			Status:     StatusInAudit,
			BaseSHA:    baseSHA,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	} else {
		found.UpdatedAt = now
		if found.BaseSHA == "" {
			found.BaseSHA = baseSHA
		}
	}

	if err := o.SaveManifest(items); err != nil {
		return targetBranch, err
	}

	return targetBranch, nil
}

// RecordTicketCommit executes local validation gate, commits with issue reference, and pushes to origin/local/<module>.
func (o *StackedBranchOrchestrator) RecordTicketCommit(ctx context.Context, module string, ticket *tracker.RoutingTicket, customSummary string) error {
	currBranch, err := o.Guard.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	expectedBranch := "local/" + strings.TrimPrefix(module, "local/")
	if currBranch != expectedBranch {
		return fmt.Errorf("active branch '%s' does not match target module branch '%s'", currBranch, expectedBranch)
	}

	// Format commit message
	summary := customSummary
	if summary == "" {
		summary = ticket.Title
	}
	ref := ticket.ID
	if ticket.GitHubIssueNum > 0 {
		ref = fmt.Sprintf("#%d", ticket.GitHubIssueNum)
	}
	commitMsg := fmt.Sprintf("feat(%s): complete %s - %s", module, ref, summary)

	// Commit and push atomically
	if err := o.Guard.CommitAndPush(ctx, commitMsg); err != nil {
		return fmt.Errorf("failed atomic commit and push for %s: %w", ticket.ID, err)
	}

	// Update manifest with ticket ID
	items, _ := o.LoadManifest()
	headSHA, _ := o.Guard.runGit(ctx, "rev-parse", "HEAD")
	for _, it := range items {
		if it.Branch == expectedBranch {
			hasTicket := false
			for _, tid := range it.Tickets {
				if tid == ticket.ID {
					hasTicket = true
					break
				}
			}
			if !hasTicket {
				it.Tickets = append(it.Tickets, ticket.ID)
			}
			it.HeadSHA = headSHA
			it.UpdatedAt = time.Now()
			break
		}
	}
	return o.SaveManifest(items)
}

// FinalizeModuleForAudit transitions the module branch to AWAITING_AUDIT state in the manifest.
func (o *StackedBranchOrchestrator) FinalizeModuleForAudit(ctx context.Context, module string) error {
	branch := "local/" + strings.TrimPrefix(module, "local/")
	items, err := o.LoadManifest()
	if err != nil {
		return err
	}

	headSHA, _ := o.Guard.runGit(ctx, "rev-parse", branch)
	for _, it := range items {
		if it.Branch == branch {
			it.Status = StatusAwaitingAudit
			it.HeadSHA = headSHA
			it.UpdatedAt = time.Now()
			return o.SaveManifest(items)
		}
	}

	// If not found in manifest, append it
	now := time.Now()
	items = append(items, &AuditQueueItem{
		Branch:     branch,
		BaseBranch: "develop",
		Module:     module,
		Status:     StatusAwaitingAudit,
		HeadSHA:    headSHA,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return o.SaveManifest(items)
}
