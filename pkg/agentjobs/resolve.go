package agentjobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// ErrNoMatchingJobs indicates no jobs were found for the ticket in any registered project.
var ErrNoMatchingJobs = errors.New("no matching jobs found")

var jobIDRegex = regexp.MustCompile(`^(.*?)-\d{8}T\d{6}Z-[0-9a-fA-F]+$`)

// TicketFromJobID extracts the ticket identifier from a job ID.
// E.g. "DRAG-30-20261021T180000Z-abcd" -> "DRAG-30".
// If the ID does not match the timestamped suffix format, it returns the ID as is.
func TicketFromJobID(id string) string {
	if m := jobIDRegex.FindStringSubmatch(strings.TrimSpace(id)); len(m) > 1 {
		return m[1]
	}
	return id
}

// TicketPrefix extracts the uppercase project identifier prefix from a ticket identifier.
// E.g. "DEN-1048" -> "DEN", "drag-30" -> "DRAG", "FOO_BAR" -> "FOO", "PROJ" -> "PROJ".
func TicketPrefix(ticket string) string {
	t := strings.Trim(strings.TrimSpace(ticket), "[]")
	if idx := strings.IndexAny(t, "-_"); idx > 0 {
		return strings.ToUpper(t[:idx])
	}
	return strings.ToUpper(t)
}

// FindByTicket returns all jobs matching ticket in the ledger, sorted chronologically.
func (l *Ledger) FindByTicket(ticket string) ([]*Job, error) {
	if l == nil {
		return nil, errors.New("agentjobs: ledger is nil")
	}
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	var matches []*Job
	for _, j := range all {
		if strings.EqualFold(j.Ticket, ticket) {
			matches = append(matches, j)
		}
	}
	return matches, nil
}

// LatestByTicket returns the latest job matching ticket in the ledger, or nil if none exist.
func (l *Ledger) LatestByTicket(ticket string) (*Job, error) {
	matches, err := l.FindByTicket(ticket)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return Latest(matches), nil
}

// Latest is the most recently created job of the list (the larger ID breaks a tie), or nil when it is empty.
func Latest(jobs []*Job) *Job {
	var latest *Job
	for _, j := range jobs {
		if latest == nil || j.CreatedAt.After(latest.CreatedAt) || (j.CreatedAt.Equal(latest.CreatedAt) && j.ID > latest.ID) {
			latest = j
		}
	}
	return latest
}

// ResolveTicketLedger searches for jobs matching ticket.
// It first inspects current. If current has matching jobs, it returns (current, jobs, nil).
// If current has no such jobs, it resolves the ticket's project from its identifier prefix
// across registered projects (the global project index, synapses, and known ledgers).
// If matching jobs are found in a resolved project ledger, it returns (resolvedLedger, jobs, nil).
// If no matching jobs are found anywhere, it returns (nil, nil, fmt.Errorf("no matching jobs found for ticket %s", ticket)).
func ResolveTicketLedger(current *Ledger, ticket string) (*Ledger, []*Job, error) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return nil, nil, errors.New("ticket cannot be empty")
	}

	if current == nil {
		current = NewLedger("")
	}

	// 1. Check current ledger first
	if matches, err := current.FindByTicket(ticket); err == nil && len(matches) > 0 {
		return current, matches, nil
	}

	// 2. Resolve ticket project from its identifier prefix
	prefix := TicketPrefix(ticket)
	if prefix == "" {
		return nil, nil, fmt.Errorf("%w for ticket %s", ErrNoMatchingJobs, ticket)
	}

	currentRoot := current.Root()
	if abs, err := filepath.Abs(currentRoot); err == nil {
		currentRoot = abs
	}
	if r, err := filepath.EvalSymlinks(currentRoot); err == nil {
		currentRoot = r
	}

	candidates := findCandidateProjectDirs(prefix)
	for _, candDir := range candidates {
		candRoot := filepath.Join(candDir, ".dendrite", "jobs")
		if abs, err := filepath.Abs(candRoot); err == nil {
			candRoot = abs
		}
		if r, err := filepath.EvalSymlinks(candRoot); err == nil {
			candRoot = r
		}
		if candRoot == currentRoot {
			continue // Already checked current ledger
		}
		candLedger := NewLedger(candRoot)
		if matches, err := candLedger.FindByTicket(ticket); err == nil && len(matches) > 0 {
			return candLedger, matches, nil
		}
	}

	return nil, nil, fmt.Errorf("%w for ticket %s", ErrNoMatchingJobs, ticket)
}

// findCandidateProjectDirs returns directories of registered projects whose .dendrite/plane.json
// matches prefix (case-insensitive).
func findCandidateProjectDirs(prefix string) []string {
	seen := make(map[string]bool)
	var dirs []string

	addDir := func(dir string) {
		if dir == "" {
			return
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		if seen[abs] {
			return
		}
		seen[abs] = true

		// Check if this directory has .dendrite/plane.json matching prefix
		link, err := plane.ReadLink(abs)
		if err != nil {
			// Fallback: read .dendrite/plane.json directly in case project_id is missing or informal
			data, rErr := os.ReadFile(filepath.Join(abs, ".dendrite", "plane.json"))
			if rErr == nil {
				var rawLink plane.ProjectLink
				if json.Unmarshal(data, &rawLink) == nil && strings.TrimSpace(rawLink.Identifier) != "" {
					link = &rawLink
					err = nil
				}
			}
		}
		if err == nil && link != nil && strings.EqualFold(link.Identifier, prefix) {
			dirs = append(dirs, abs)
		}
	}

	// 1. Global project index (DENDRITE_HOME/projects.json)
	if indexPath, err := projectid.DefaultIndexPath(); err == nil {
		if idx, err := projectid.LoadIndex(indexPath); err == nil {
			for _, entry := range idx.List() {
				addDir(entry.Path)
			}
		}
	}

	// 2. Synapse store (DENDRITE_HOME/synapses/*.toml)
	if st, err := synapse.DefaultStore(); err == nil {
		if syns, err := st.List(); err == nil {
			for _, s := range syns {
				for _, m := range s.Members {
					if dir, err := synapse.Resolve(m); err == nil {
						addDir(dir)
					} else if m.PathHint != "" {
						addDir(m.PathHint)
					}
				}
			}
		}
	}

	// 3. Registered ledgers (_ledgers.json)
	if DefaultDispatcher != nil {
		for _, ledgerRoot := range DefaultDispatcher.registeredLedgers() {
			repoRoot := filepath.Dir(filepath.Dir(ledgerRoot))
			addDir(repoRoot)
		}
	}

	// 4. Current repo and parents/siblings if in git
	if root, err := plane.FindRepoRoot("."); err == nil {
		addDir(root)
		parent := filepath.Dir(root)
		if entries, err := os.ReadDir(parent); err == nil {
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					addDir(filepath.Join(parent, e.Name()))
				}
			}
		}
	}

	return dirs
}
