package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"Dendrite/pkg/tracker"
)

// Standard errors for session ledger operations.
var (
	ErrNilSession = errors.New("session summary cannot be nil")
)

// SessionLedger records historical autonomous agent sessions and orchestrates issue tracker synchronization.
type SessionLedger struct {
	mu         sync.RWMutex
	sessions   map[string]*SessionSummary
	order      []string
	tracker    tracker.IssueTracker
	ledgerPath string
}

// NewSessionLedger creates a thread-safe session ledger with optional issue tracker binding.
func NewSessionLedger(tr tracker.IssueTracker, ledgerPath string) *SessionLedger {
	return &SessionLedger{
		sessions:   make(map[string]*SessionSummary),
		order:      make([]string, 0),
		tracker:    tr,
		ledgerPath: ledgerPath,
	}
}

// RecordSession saves a completed or terminated session and syncs the associated ticket in the issue tracker.
func (l *SessionLedger) RecordSession(ctx context.Context, s *SessionSummary) error {
	if s == nil {
		return ErrNilSession
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	cpy := *s
	l.sessions[s.SessionID] = &cpy
	l.order = append(l.order, s.SessionID)

	// Sync with issue tracker if bound and ticket ID is present
	if l.tracker != nil && s.TicketID != "" {
		newStatus := tracker.StatusInReview
		if s.Status == "completed" {
			newStatus = tracker.StatusDone
		}
		_ = l.tracker.UpdateStatus(ctx, tracker.TicketID(s.TicketID), newStatus)
		_ = l.tracker.Sync(ctx)
	}

	// Persist ledger to disk if file path configured
	if l.ledgerPath != "" {
		data, err := json.MarshalIndent(l.getAllSessionsUnlocked(), "", "  ")
		if err == nil {
			_ = os.WriteFile(l.ledgerPath, data, 0644)
		}
	}

	return nil
}

// GetSession retrieves a session record by its unique identifier.
func (l *SessionLedger) GetSession(sessionID string) (*SessionSummary, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	s, exists := l.sessions[sessionID]
	if !exists {
		return nil, false
	}
	cpy := *s
	return &cpy, true
}

func (l *SessionLedger) filterSessions(predicate func(*SessionSummary) bool) []*SessionSummary {
	l.mu.RLock()
	defer l.mu.RUnlock()

	res := make([]*SessionSummary, 0)
	for _, id := range l.order {
		s := l.sessions[id]
		if s != nil && predicate(s) {
			cpy := *s
			res = append(res, &cpy)
		}
	}
	return res
}

// ListSessionsByTicket returns all agent sessions associated with a specific ticket ID.
func (l *SessionLedger) ListSessionsByTicket(ticketID string) []*SessionSummary {
	return l.filterSessions(func(s *SessionSummary) bool {
		return s.TicketID == ticketID
	})
}

// ListSessionsByAgent returns all sessions driven by a specific agent type.
func (l *SessionLedger) ListSessionsByAgent(agentType AgentType) []*SessionSummary {
	return l.filterSessions(func(s *SessionSummary) bool {
		return s.AgentType == agentType
	})
}

// TotalSpendUSD returns the cumulative monetary cost of all recorded sessions.
func (l *SessionLedger) TotalSpendUSD() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var total float64
	for _, s := range l.sessions {
		total += s.EstimatedCostUSD
	}
	return total
}

// TotalTokens returns the total tokens consumed across all recorded sessions.
func (l *SessionLedger) TotalTokens() int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var total int
	for _, s := range l.sessions {
		total += s.TokensConsumed
	}
	return total
}

func (l *SessionLedger) getAllSessionsUnlocked() []*SessionSummary {
	res := make([]*SessionSummary, 0, len(l.order))
	for _, id := range l.order {
		if s, ok := l.sessions[id]; ok {
			res = append(res, s)
		}
	}
	return res
}

// ExportJSON serializes all recorded sessions to indented JSON bytes.
func (l *SessionLedger) ExportJSON() ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return json.MarshalIndent(l.getAllSessionsUnlocked(), "", "  ")
}

// LoadJSON restores session history from JSON bytes.
func (l *SessionLedger) LoadJSON(data []byte) error {
	var list []*SessionSummary
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("failed to parse session ledger JSON: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.sessions = make(map[string]*SessionSummary)
	l.order = make([]string, 0, len(list))

	for _, s := range list {
		if s != nil && s.SessionID != "" {
			l.sessions[s.SessionID] = s
			l.order = append(l.order, s.SessionID)
		}
	}

	return nil
}
