package tracker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// regex matching: - [x] **DEN-001**: Title (`package`) *(STATUS)*
	ticketRegex = regexp.MustCompile("^-\\s+\\[([ xX])\\]\\s+\\*\\*([A-Za-z0-9-]+)\\*\\*:\\s+(.*?)(?:\\s+\\(`([^`]+)`\\))?(?:\\s+\\*\\(([A-Z_ ]+)\\)\\*)?$")
	// stateMarkerRegex matches the "[State: To Verify" marker written by `plane replicate`.
	stateMarkerRegex = regexp.MustCompile(`\[State:\s*[^,\]]*`)
	// legacySuffixRegex matches a trailing "*(IN PROGRESS)*" status suffix.
	legacySuffixRegex = regexp.MustCompile(`\s+\*\([A-Z_ ]+\)\*$`)
	// checkboxRegex matches the leading "- [ ]" checkbox of a ticket line.
	checkboxRegex = regexp.MustCompile(`^(\s*-\s+\[)[ xX](\])`)
)

// MemoryTracker is an in-memory ticket repository with markdown file synchronization.
type MemoryTracker struct {
	mu           sync.RWMutex
	tickets      map[TicketID]*Ticket
	order        []TicketID
	markdownPath string
	// changed holds tickets whose status changed since loading; Sync rewrites only their lines.
	changed map[TicketID]bool
}

// NewMemoryTracker creates an empty tracker with synchronization capability.
func NewMemoryTracker(markdownPath string) *MemoryTracker {
	return &MemoryTracker{
		tickets:      make(map[TicketID]*Ticket),
		order:        make([]TicketID, 0),
		markdownPath: markdownPath,
		changed:      make(map[TicketID]bool),
	}
}

// LoadFromMarkdown parses tickets out of a DEV_STATUS.md markdown file.
func (m *MemoryTracker) LoadFromMarkdown(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open markdown file: %w", err)
	}
	defer f.Close()

	m.markdownPath = path
	m.tickets = make(map[TicketID]*Ticket)
	m.order = make([]TicketID, 0)
	m.changed = make(map[TicketID]bool)

	scanner := bufio.NewScanner(f)
	currentMilestone := "General"

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "### Milestone") {
			currentMilestone = strings.TrimPrefix(line, "### ")
			continue
		}

		matches := ticketRegex.FindStringSubmatch(line)
		if len(matches) > 3 {
			checked := strings.ToLower(matches[1]) == "x"
			id := TicketID(matches[2])
			title := matches[3]
			targetPkg := ""
			if len(matches) > 4 {
				targetPkg = matches[4]
			}
			customStatus := ""
			if len(matches) > 5 {
				customStatus = matches[5]
			}

			status := StatusTodo
			if checked {
				status = StatusDone
			} else if customStatus == "IN PROGRESS" {
				status = StatusInProgress
			}

			t := &Ticket{
				ID:            id,
				Title:         title,
				Status:        status,
				Milestone:     currentMilestone,
				TargetPackage: targetPkg,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			if _, dup := m.tickets[id]; dup {
				continue // duplicate entries keep the first occurrence; Sync edits each line in place
			}
			m.tickets[id] = t
			m.order = append(m.order, id)
		}
	}

	return scanner.Err()
}

// GetTicket retrieves a ticket by ID.
func (m *MemoryTracker) GetTicket(_ context.Context, id TicketID) (*Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, exists := m.tickets[id]
	if !exists {
		return nil, &ErrTicketNotFound{ID: id}
	}
	cpy := *t
	return &cpy, nil
}

// ListTickets returns all tickets in registered order.
func (m *MemoryTracker) ListTickets(_ context.Context) ([]Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]Ticket, 0, len(m.order))
	for _, id := range m.order {
		if t, ok := m.tickets[id]; ok {
			res = append(res, *t)
		}
	}
	return res, nil
}

// CreateTicket adds a new ticket to the ledger.
func (m *MemoryTracker) CreateTicket(_ context.Context, t Ticket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tickets[t.ID]; exists {
		return fmt.Errorf("ticket already exists: %s", t.ID)
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	t.UpdatedAt = time.Now()

	m.tickets[t.ID] = &t
	m.order = append(m.order, t.ID)
	return nil
}

// UpdateStatus changes the state of an existing ticket.
func (m *MemoryTracker) UpdateStatus(_ context.Context, id TicketID, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tickets[id]
	if !exists {
		return &ErrTicketNotFound{ID: id}
	}

	t.Status = status
	t.UpdatedAt = time.Now()
	m.changed[id] = true
	return nil
}

// Sync writes the current ticket state back into the target DEV_STATUS.md file.
func (m *MemoryTracker) Sync(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.markdownPath == "" {
		return fmt.Errorf("markdown path not configured")
	}

	content, err := os.ReadFile(m.markdownPath)
	if err != nil {
		return fmt.Errorf("failed to read markdown file for sync: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		matches := ticketRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) <= 3 || !m.changed[TicketID(matches[2])] {
			continue // untouched tickets keep their line byte for byte
		}
		lines[i] = rewriteStatus(line, m.tickets[TicketID(matches[2])].Status)
	}
	m.changed = make(map[TicketID]bool)

	return os.WriteFile(m.markdownPath, []byte(strings.Join(lines, "\n")), 0644)
}

// rewriteStatus updates a ticket line's checkbox and status marker in place, keeping
// its title, package and other annotations untouched.
func rewriteStatus(line string, status Status) string {
	mark := " "
	if status == StatusDone {
		mark = "x"
	}
	line = checkboxRegex.ReplaceAllString(line, "${1}"+mark+"${2}")
	if stateMarkerRegex.MatchString(line) {
		return stateMarkerRegex.ReplaceAllString(line, "[State: "+StatusDisplayName(status))
	}
	line = legacySuffixRegex.ReplaceAllString(line, "")
	if status == StatusInProgress {
		line += " *(IN PROGRESS)*"
	}
	return line
}

// StatusDisplayName is the human-readable state name used in DEV_STATUS.md and Plane.
func StatusDisplayName(st Status) string {
	switch st {
	case StatusDone:
		return "Done"
	case StatusInReview:
		return "To Verify"
	case StatusInProgress:
		return "In Progress"
	case StatusTodo:
		return "To Do"
	default:
		return "Backlog"
	}
}
