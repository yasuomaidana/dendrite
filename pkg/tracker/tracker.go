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
)

// MemoryTracker is an in-memory ticket repository with markdown file synchronization.
type MemoryTracker struct {
	mu           sync.RWMutex
	tickets      map[TicketID]*Ticket
	order        []TicketID
	markdownPath string
}

// NewMemoryTracker creates an empty tracker with synchronization capability.
func NewMemoryTracker(markdownPath string) *MemoryTracker {
	return &MemoryTracker{
		tickets:      make(map[TicketID]*Ticket),
		order:        make([]TicketID, 0),
		markdownPath: markdownPath,
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
	newLines := make([]string, 0, len(lines))

	for _, line := range lines {
		matches := ticketRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) > 3 {
			id := TicketID(matches[2])
			if t, ok := m.tickets[id]; ok {
				checkMark := " "
				suffix := ""
				if t.Status == StatusDone {
					checkMark = "x"
				} else if t.Status == StatusInProgress {
					suffix = " *(IN PROGRESS)*"
				}

				pkgPart := ""
				if t.TargetPackage != "" {
					pkgPart = fmt.Sprintf(" (`%s`)", t.TargetPackage)
				}

				newLine := fmt.Sprintf("- [%s] **%s**: %s%s%s", checkMark, t.ID, t.Title, pkgPart, suffix)
				newLines = append(newLines, newLine)
				continue
			}
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(m.markdownPath, []byte(strings.Join(newLines, "\n")), 0644)
}
