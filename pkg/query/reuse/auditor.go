package reuse

import (
	"Dendrite/pkg/statedir"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/store"
)

// PoolMemberInfo minimal representation of registered pool members.
type PoolMemberInfo struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Ecosystem    string   `json:"ecosystem"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Auditor coordinates the strict 3-tier library reuse hierarchy verification.
type Auditor struct {
	store      *store.Store
	refManager *RefManager
	poolPath   string
}

// NewAuditor creates a new library reuse auditor.
func NewAuditor(s *store.Store, refMgr *RefManager, poolPath string) *Auditor {
	if s == nil {
		s = store.NewStore(".dendrite/store", "docs/adr")
	}
	if refMgr == nil {
		refMgr = NewRefManager(".dendrite/refs.json")
	}
	if poolPath == "" {
		poolPath = statedir.Resolve(".dendrite/pool.json")
	}
	return &Auditor{
		store:      s,
		refManager: refMgr,
		poolPath:   poolPath,
	}
}

// AuditReuse executes the strict 3-tier evaluation:
// Tier 1 (Source References) > Tier 2 (Pool Members) > Tier 3 (Ecosystem Registries).
func (a *Auditor) AuditReuse(ctx context.Context, req AuditRequest) (*AuditResult, error) {
	if strings.TrimSpace(req.Capability) == "" {
		return nil, ErrEmptyCapability
	}

	preCheckHash := a.store.ComputeHash([]byte(fmt.Sprintf("%s:%s:%s", req.Intent, req.Capability, req.Language)))

	// 1. Evaluate Tier 1: Source References (dendrite ref)
	if res, matched := a.checkTier1(req); matched {
		res.PreCheckHash = preCheckHash
		return a.recordDecision(req, res)
	}

	// 2. Evaluate Tier 2: Active Pool Members (dendrite pool)
	if res, matched := a.checkTier2(req); matched {
		res.PreCheckHash = preCheckHash
		return a.recordDecision(req, res)
	}

	// 3. Evaluate Tier 3: Ecosystem Registries (crates.io, pkg.go.dev, npm, PyPI)
	if res, matched := a.checkTier3(req); matched {
		res.PreCheckHash = preCheckHash
		return a.recordDecision(req, res)
	}

	// 4. Fallback: Unresolved
	fallback := &AuditResult{
		TierSelected:  TierUnresolved,
		CandidateName: req.Capability,
		Action:        "synthesize_new",
		Rationale:     "No suitable library found across 3-tier hierarchy; synthesizing fresh module.",
		PreCheckHash:  preCheckHash,
	}
	return a.recordDecision(req, fallback)
}

func (a *Auditor) checkTier1(req AuditRequest) (*AuditResult, bool) {
	refs, err := a.refManager.ListRefs()
	if err != nil {
		return nil, false
	}

	capLower := strings.ToLower(req.Capability)
	for _, ref := range refs {
		if req.Language != "" && !strings.EqualFold(ref.Language, req.Language) {
			continue
		}
		for _, c := range ref.Capabilities {
			if strings.EqualFold(c, req.Capability) || strings.Contains(capLower, strings.ToLower(c)) {
				return &AuditResult{
					TierSelected:  Tier1SourceRef,
					CandidateName: ref.Name,
					PackageSpec:   fmt.Sprintf("%s (ref: %s)", ref.Name, ref.ID),
					SourceURI:     ref.Repository,
					ImportPath:    ref.Path,
					Action:        "slice_and_import",
					Rationale:     fmt.Sprintf("Matched Tier 1 Source Reference '%s' registered via dendrite ref.", ref.Name),
				}, true
			}
		}
	}
	return nil, false
}

func (a *Auditor) checkTier2(req AuditRequest) (*AuditResult, bool) {
	members := a.loadPoolMembers()
	capLower := strings.ToLower(req.Capability)

	for _, m := range members {
		if strings.Contains(strings.ToLower(m.Name), capLower) {
			return &AuditResult{
				TierSelected:  Tier2PoolMember,
				CandidateName: m.Name,
				PackageSpec:   fmt.Sprintf("path = \"%s\"", m.Path),
				SourceURI:     m.Path,
				ImportPath:    m.Name,
				Action:        "wire_pool_edge",
				Rationale:     fmt.Sprintf("Found active Tier 2 pool member '%s'; wiring workspace dependency edge.", m.Name),
			}, true
		}
	}
	return nil, false
}

func (a *Auditor) checkTier3(req AuditRequest) (*AuditResult, bool) {
	pkg, found := LookupEcosystemPackage(req.Capability, req.Language)
	if !found {
		return nil, false
	}

	return &AuditResult{
		TierSelected:  Tier3Registry,
		CandidateName: pkg.Name,
		PackageSpec:   pkg.PackageSpec,
		SourceURI:     pkg.SourceURI,
		ImportPath:    pkg.ImportPath,
		Action:        "add_registry_dep",
		Rationale:     fmt.Sprintf("Mapped canonical ecosystem package '%s' from public registry.", pkg.Name),
	}, true
}

func (a *Auditor) recordDecision(req AuditRequest, res *AuditResult) (*AuditResult, error) {
	ticketID := req.TicketID
	if ticketID == "" {
		ticketID = "DEN-058"
	}

	title := fmt.Sprintf("Adopt %s for %s", res.CandidateName, req.Capability)
	evalText := fmt.Sprintf("- **Capability Requested**: `%s`\n- **Target Language**: `%s`\n- **Tier 1 (Source References)**: %s\n- **Tier 2 (Pool Members)**: %s\n- **Tier 3 (Ecosystem Registries)**: %s\n- **Selected Tier**: `%s`",
		req.Capability, req.Language,
		boolLabel(res.TierSelected == Tier1SourceRef),
		boolLabel(res.TierSelected == Tier2PoolMember),
		boolLabel(res.TierSelected == Tier3Registry),
		res.TierSelected,
	)

	adr := &store.ADR{
		TicketID:     ticketID,
		Title:        title,
		Status:       "Accepted",
		Date:         time.Now().UTC(),
		TierSelected: string(res.TierSelected),
		PackageName:  res.CandidateName,
		SourceURI:    res.SourceURI,
		Rationale:    res.Rationale,
		Context:      fmt.Sprintf("Intent: %s\nEvaluated library reuse options for capability '%s'.", req.Intent, req.Capability),
		Evaluation:   evalText,
		Consequences: fmt.Sprintf("- Action: %s\n- Package Spec: `%s`\n- Import Path: `%s`", res.Action, res.PackageSpec, res.ImportPath),
	}

	path, err := a.store.SaveADR(adr)
	if err != nil {
		return nil, fmt.Errorf("saving ADR: %w", err)
	}

	res.ADRPath = path
	res.ADRID = adr.ID
	return res, nil
}

func (a *Auditor) loadPoolMembers() []PoolMemberInfo {
	if _, err := os.Stat(a.poolPath); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(a.poolPath)
	if err != nil {
		return nil
	}

	var parsed struct {
		Projects []struct {
			Name      string `json:"name"`
			Path      string `json:"path"`
			Ecosystem string `json:"ecosystem"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}

	members := make([]PoolMemberInfo, 0, len(parsed.Projects))
	for _, p := range parsed.Projects {
		members = append(members, PoolMemberInfo{
			Name:      p.Name,
			Path:      filepath.Clean(p.Path),
			Ecosystem: p.Ecosystem,
		})
	}
	return members
}

func boolLabel(b bool) string {
	if b {
		return "Matched (Selected)"
	}
	return "No Match"
}
