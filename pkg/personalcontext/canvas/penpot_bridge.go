package canvas

import (
	"Dendrite/pkg/statedir"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/env"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/ui/penpot"
)

var (
	ErrMissingTicketID    = errors.New("ticket_id is required")
	ErrMissingFeatureName = errors.New("feature_name is required")
	ErrMissingFilePath    = errors.New("file_path is required")
	ErrMissingTargetState = errors.New("target_state is required")
	ErrRelationNotFound   = errors.New("ticket relation not found")
)

// PenpotUIBridge coordinates Plane tickets, UI features, and Penpot visual frames.
type PenpotUIBridge struct {
	mu             sync.RWMutex
	relations      map[string]*TicketFeatureRelation // key: TicketID
	planeClient    *plane.Client
	penpotEndpoint string
	storagePath    string
	parser         *penpot.Parser
	synchronizer   *penpot.Synchronizer
}

// NewPenpotUIBridge constructs an interactive bridge.
func NewPenpotUIBridge(planeClient *plane.Client, endpoint string) *PenpotUIBridge {
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	bridge := &PenpotUIBridge{
		relations:      make(map[string]*TicketFeatureRelation),
		planeClient:    planeClient,
		penpotEndpoint: endpoint,
		parser:         penpot.NewParser(),
		synchronizer:   penpot.NewSynchronizer(planeClient),
	}

	// Default persistence location
	defaultStorage := statedir.Resolve(filepath.Join(".dendrite", "canvas_relations.json"))
	if _, err := os.Stat(statedir.Resolve(".dendrite")); err == nil {
		bridge.storagePath = defaultStorage
		_ = bridge.load()
	}

	return bridge
}

// SetStoragePath overrides the persistence file path for relations.
func (b *PenpotUIBridge) SetStoragePath(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.storagePath = path
	_ = b.loadLocked()
}

func (b *PenpotUIBridge) load() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadLocked()
}

func (b *PenpotUIBridge) loadLocked() error {
	if b.storagePath == "" {
		return nil
	}
	data, err := os.ReadFile(b.storagePath)
	if err != nil {
		return err
	}
	var loaded map[string]*TicketFeatureRelation
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	b.relations = loaded
	return nil
}

func (b *PenpotUIBridge) saveLocked() error {
	if b.storagePath == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(b.storagePath), 0755)
	data, err := json.MarshalIndent(b.relations, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.storagePath, data, 0644)
}

// RelateTicketToFeature establishes and persists a relationship between a Plane ticket and UI feature.
func (b *PenpotUIBridge) RelateTicketToFeature(ctx context.Context, rel TicketFeatureRelation) error {
	if strings.TrimSpace(rel.TicketID) == "" {
		return ErrMissingTicketID
	}
	if strings.TrimSpace(rel.FeatureName) == "" {
		return ErrMissingFeatureName
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	ticketID := strings.TrimSpace(rel.TicketID)
	frameID := rel.PenpotFrameID
	if frameID == "" {
		frameID = strings.ToLower(strings.ReplaceAll(rel.FeatureName, " ", "-"))
	}

	frameURL := rel.PenpotFrameURL
	if frameURL == "" {
		frameURL = fmt.Sprintf("%s/#/workspace/dendrite/project/ui?frame=%s", b.penpotEndpoint, frameID)
	}

	currentState := rel.CurrentState
	if currentState == "" {
		currentState = "IN_PROGRESS"
	}

	persisted := &TicketFeatureRelation{
		TicketID:       ticketID,
		FeatureName:    rel.FeatureName,
		Description:    rel.Description,
		ComponentPath:  rel.ComponentPath,
		PenpotProject:  rel.PenpotProject,
		PenpotFrameID:  frameID,
		PenpotFrameURL: frameURL,
		CurrentState:   currentState,
		LastSyncedAt:   time.Now().UTC(),
	}

	b.relations[ticketID] = persisted
	_ = b.saveLocked()

	// If planeClient is active, post a status note linking Penpot Frame
	if b.planeClient != nil && rel.Description != "" {
		comment := fmt.Sprintf("UI Feature '%s' linked to Penpot Frame: %s", rel.FeatureName, frameURL)
		_ = b.planeClient.AddComment(ctx, tracker.TicketID(ticketID), comment)
	}

	return nil
}

// GetTicketFeatureRelation returns the relation for a specific ticket if registered.
func (b *PenpotUIBridge) GetTicketFeatureRelation(ticketID string) (*TicketFeatureRelation, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rel, ok := b.relations[strings.TrimSpace(ticketID)]
	if !ok {
		return nil, false
	}
	cpy := *rel
	return &cpy, true
}

// ListTicketRelations returns a snapshot of all registered ticket-feature relations.
func (b *PenpotUIBridge) ListTicketRelations() []*TicketFeatureRelation {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := make([]*TicketFeatureRelation, 0, len(b.relations))
	for _, r := range b.relations {
		cpy := *r
		list = append(list, &cpy)
	}
	return list
}

// SyncComponentWithPenpot inspects a frontend file, parses tokens, checks Penpot reachability, and links to ticket.
func (b *PenpotUIBridge) SyncComponentWithPenpot(ctx context.Context, ticketID, filePath string, dryRun bool) (*UIBridgeResult, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, ErrMissingFilePath
	}

	ticketID = strings.TrimSpace(ticketID)
	spec, err := b.parser.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse UI component: %w", err)
	}

	isHealthy := env.CheckPenpotHealth(ctx, b.penpotEndpoint, 1*time.Second)
	frameSlug := strings.ToLower(filepath.Base(filePath))
	frameSlug = strings.ReplaceAll(frameSlug, ".", "-")
	penpotURL := fmt.Sprintf("%s/#/workspace/dendrite/project/ui?frame=%s", b.penpotEndpoint, frameSlug)

	result := &UIBridgeResult{
		TicketID:       ticketID,
		FeatureName:    spec.Name,
		PenpotFrameURL: penpotURL,
		ComponentPath:  filePath,
		Tokens:         spec.Tokens,
		ProjectState:   "IN_PROGRESS",
		PlaneUpdated:   false,
	}

	if isHealthy {
		result.Details = "Penpot frame reachable and tokens extracted"
	} else {
		result.Details = fmt.Sprintf("Penpot endpoint (%s) offline; local tokens extracted with synthetic frame URL", b.penpotEndpoint)
	}

	b.mu.Lock()
	if ticketID != "" {
		rel, exists := b.relations[ticketID]
		if !exists {
			rel = &TicketFeatureRelation{
				TicketID:       ticketID,
				FeatureName:    spec.Name,
				ComponentPath:  filePath,
				PenpotFrameID:  frameSlug,
				PenpotFrameURL: penpotURL,
				CurrentState:   "IN_PROGRESS",
				LastSyncedAt:   time.Now().UTC(),
			}
			b.relations[ticketID] = rel
		} else {
			rel.ComponentPath = filePath
			rel.PenpotFrameURL = penpotURL
			rel.LastSyncedAt = time.Now().UTC()
		}
		_ = b.saveLocked()
	}
	b.mu.Unlock()

	if !dryRun && b.planeClient != nil && ticketID != "" {
		comment := fmt.Sprintf("🎨 Penpot UI Canvas Synced: Component '%s' (%s)\nTokens: %d design tokens extracted\nDeep-Link: %s",
			spec.Name, spec.Framework, len(spec.Tokens), penpotURL)
		if err := b.planeClient.AddComment(ctx, tracker.TicketID(ticketID), comment); err == nil {
			result.PlaneUpdated = true
		}
	}

	return result, nil
}

// UpdateProjectState updates the project state in both Penpot relations and Plane v2.
func (b *PenpotUIBridge) UpdateProjectState(ctx context.Context, req ProjectStateUpdateRequest) (*ProjectStateUpdateResponse, error) {
	ticketID := strings.TrimSpace(req.TicketID)
	if ticketID == "" {
		return nil, ErrMissingTicketID
	}
	targetState := strings.TrimSpace(req.TargetState)
	if targetState == "" {
		return nil, ErrMissingTargetState
	}

	b.mu.Lock()
	rel, ok := b.relations[ticketID]
	prevState := "UNKNOWN"
	if ok {
		prevState = rel.CurrentState
		rel.CurrentState = targetState
		if req.PenpotFrameURL != "" {
			rel.PenpotFrameURL = req.PenpotFrameURL
		}
		rel.LastSyncedAt = time.Now().UTC()
	} else {
		rel = &TicketFeatureRelation{
			TicketID:       ticketID,
			FeatureName:    ticketID,
			CurrentState:   targetState,
			PenpotFrameURL: req.PenpotFrameURL,
			LastSyncedAt:   time.Now().UTC(),
		}
		b.relations[ticketID] = rel
	}
	_ = b.saveLocked()
	b.mu.Unlock()

	resp := &ProjectStateUpdateResponse{
		TicketID:      ticketID,
		PreviousState: prevState,
		NewState:      targetState,
		PlaneSynced:   false,
		Message:       fmt.Sprintf("Project state transitioned from '%s' to '%s'", prevState, targetState),
		Timestamp:     time.Now().UTC(),
	}

	if req.PushToPlane && b.planeClient != nil {
		planeStatus := mapTargetStateToTrackerStatus(targetState)
		if err := b.planeClient.UpdateStatus(ctx, tracker.TicketID(ticketID), planeStatus); err == nil {
			resp.PlaneSynced = true
			resp.Message += " (Synchronized with Plane v2)"
		} else {
			resp.Message += fmt.Sprintf(" (Plane sync error: %v)", err)
		}

		if req.Comment != "" {
			_ = b.planeClient.AddComment(ctx, tracker.TicketID(ticketID), req.Comment)
		}
	}

	return resp, nil
}

func mapTargetStateToTrackerStatus(s string) tracker.Status {
	normalized := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(normalized, "done") || strings.Contains(normalized, "complete"):
		return tracker.StatusDone
	case strings.Contains(normalized, "verify") || strings.Contains(normalized, "review"):
		return tracker.StatusInReview
	case strings.Contains(normalized, "progress") || strings.Contains(normalized, "active"):
		return tracker.StatusInProgress
	case strings.Contains(normalized, "todo") || strings.Contains(normalized, "to-do"):
		return tracker.StatusTodo
	default:
		return tracker.StatusBacklog
	}
}
