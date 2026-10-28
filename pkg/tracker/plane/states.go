package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// StateSpec describes one workflow state every Dendrite-governed Plane project must carry.
type StateSpec struct {
	Name     string
	Group    string
	Color    string
	Sequence float64
}

// CanonicalStates is the single source of truth for Plane workflow states.
// It codifies the DEN (Dendrite Core) board: its names and ordering, with Plane's
// stock palette filling the colors DEN left blank and To Verify keeping DEN's amber.
// Every project creation and reconcile path converges boards onto this list.
var CanonicalStates = []StateSpec{
	{Name: "Backlog", Group: "backlog", Color: "#A3A3A3", Sequence: 15000},
	{Name: "To Do", Group: "unstarted", Color: "#3A3A3A", Sequence: 25000},
	{Name: "In Progress", Group: "started", Color: "#F59E0B", Sequence: 35000},
	{Name: "To Verify", Group: "started", Color: "#FCB900", Sequence: 40000},
	{Name: "Blocked", Group: "started", Color: "#DC2626", Sequence: 42000},
	{Name: "Done", Group: "completed", Color: "#16A34A", Sequence: 45000},
	{Name: "Cancelled", Group: "cancelled", Color: "#EF4444", Sequence: 55000},
}

// StateAliases maps lower-cased legacy names (Plane defaults, older boards) onto canonical names.
var StateAliases = map[string]string{
	"todo":     "To Do",
	"to-do":    "To Do",
	"canceled": "Cancelled",
}

// StateChange records one create or update applied (or planned) by ReconcileStates.
type StateChange struct {
	Action string   `json:"action"` // "create" or "update"
	Name   string   `json:"name"`
	From   string   `json:"from,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

// StateReconcileReport summarizes a reconcile pass over one project.
type StateReconcileReport struct {
	ProjectID string        `json:"project_id"`
	DryRun    bool          `json:"dry_run"`
	Changes   []StateChange `json:"changes"`
	Unmanaged []string      `json:"unmanaged,omitempty"`
}

// Created returns the canonical names of states created (or that would be created).
func (r *StateReconcileReport) Created() []string {
	var out []string
	for _, ch := range r.Changes {
		if ch.Action == "create" {
			out = append(out, ch.Name)
		}
	}
	return out
}

func canonicalName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := StateAliases[n]; ok {
		return alias
	}
	for _, spec := range CanonicalStates {
		if strings.EqualFold(spec.Name, n) {
			return spec.Name
		}
	}
	return ""
}

// ReconcileStates converges a project's workflow states onto CanonicalStates:
// missing states are created, drifted name/group/color/sequence are patched, and
// states outside the canonical set are left untouched and reported as unmanaged.
// With dryRun, the plan is returned without writing anything.
func (c *Client) ReconcileStates(ctx context.Context, projectID string, dryRun bool) (*StateReconcileReport, error) {
	if strings.TrimSpace(projectID) == "" {
		projectID = c.projectID
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrEmptyProjectID
	}

	existing, err := c.ListStatesForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Exact canonical names win over aliases so a board holding both "Todo" and "To Do" keeps "To Do".
	matched := make(map[string]PlaneState)
	for _, s := range existing {
		if cn := canonicalName(s.Name); cn != "" && strings.EqualFold(strings.TrimSpace(s.Name), cn) {
			if _, taken := matched[cn]; !taken {
				matched[cn] = s
			}
		}
	}
	used := make(map[string]bool)
	for _, s := range matched {
		used[s.ID] = true
	}
	for _, s := range existing {
		if used[s.ID] {
			continue
		}
		if cn := canonicalName(s.Name); cn != "" {
			if _, taken := matched[cn]; !taken {
				matched[cn] = s
				used[s.ID] = true
			}
		}
	}

	report := &StateReconcileReport{ProjectID: projectID, DryRun: dryRun}
	for _, s := range existing {
		if !used[s.ID] {
			report.Unmanaged = append(report.Unmanaged, s.Name)
		}
	}

	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.workspaceSlug, projectID)
	for _, spec := range CanonicalStates {
		cur, ok := matched[spec.Name]
		if !ok {
			report.Changes = append(report.Changes, StateChange{Action: "create", Name: spec.Name})
			if dryRun {
				continue
			}
			body := map[string]interface{}{
				"name":     spec.Name,
				"group":    spec.Group,
				"color":    spec.Color,
				"sequence": spec.Sequence,
			}
			created, err := c.sendState(ctx, http.MethodPost, endpoint, body)
			if err != nil {
				return report, fmt.Errorf("create state %q: %w", spec.Name, err)
			}
			// Plane ignores "sequence" on create and appends the state last; move it into place.
			if created.ID != "" && created.Sequence != spec.Sequence {
				fix := map[string]interface{}{"sequence": spec.Sequence}
				if _, err := c.sendState(ctx, http.MethodPatch, endpoint+created.ID+"/", fix); err != nil {
					return report, fmt.Errorf("order state %q: %w", spec.Name, err)
				}
			}
			continue
		}

		body := map[string]interface{}{}
		var fields []string
		if cur.Name != spec.Name {
			body["name"] = spec.Name
			fields = append(fields, "name")
		}
		if cur.Group != spec.Group {
			body["group"] = spec.Group
			fields = append(fields, "group")
		}
		if !strings.EqualFold(cur.Color, spec.Color) {
			body["color"] = spec.Color
			fields = append(fields, "color")
		}
		if cur.Sequence != spec.Sequence {
			body["sequence"] = spec.Sequence
			fields = append(fields, "sequence")
		}
		if len(fields) == 0 {
			continue
		}
		change := StateChange{Action: "update", Name: spec.Name, Fields: fields}
		if cur.Name != spec.Name {
			change.From = cur.Name
		}
		report.Changes = append(report.Changes, change)
		if dryRun {
			continue
		}
		if _, err := c.sendState(ctx, http.MethodPatch, endpoint+cur.ID+"/", body); err != nil {
			return report, fmt.Errorf("update state %q: %w", spec.Name, err)
		}
	}
	return report, nil
}

// sendState writes a state and returns Plane's stored copy (zero value when the body is not a state).
func (c *Client) sendState(ctx context.Context, method, endpoint string, body map[string]interface{}) (PlaneState, error) {
	var stored PlaneState
	req, err := c.newRequest(ctx, method, endpoint, body)
	if err != nil {
		return stored, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return stored, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return stored, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return stored, fmt.Errorf("plane state %s failed: status %d", method, resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&stored)
	return stored, nil
}

// ResolveProject finds a project by UUID or (case-insensitive) identifier such as "DGIT".
func (c *Client) ResolveProject(ctx context.Context, ref string) (*PlaneProject, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrEmptyProjectID
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == ref || strings.EqualFold(projects[i].Identifier, ref) {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("plane project %q not found", ref)
}

// ForProject returns a copy of the client scoped to another project, sharing credentials and transport.
func (c *Client) ForProject(projectID string) *Client {
	clone := *c
	clone.projectID = projectID
	return &clone
}

// ClientForProject resolves a project identifier or UUID and returns a client scoped to it.
func ClientForProject(ctx context.Context, base *Client, ref string) (*Client, error) {
	proj, err := base.ResolveProject(ctx, ref)
	if err != nil {
		return nil, err
	}
	return base.ForProject(proj.ID), nil
}
