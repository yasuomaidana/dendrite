package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/fsutil"
)

// CyclomaticLimit is the highest cyclomatic complexity the local model is trusted with: a function above it is
// left for a paid model, and the analysis stops at a pause gate before one runs.
const CyclomaticLimit = 15

// maxHardListed bounds the functions named in the triage document, which is indexed with the rest of the workspace.
const maxHardListed = 200

// Tiers a function is routed to.
const (
	TierLocal = "local"
	TierPaid  = "paid"
)

// FunctionLister lists the functions of a target; the dendrite-core client is the real one.
type FunctionLister interface {
	Functions(ctx context.Context, path string) (*coreclient.FunctionList, error)
}

// Triage is the summary of the routing of an analysis, kept in its checkpoint. The tier of every function is in
// triage.json beside it.
type Triage struct {
	Limit     int  `json:"limit"`
	Local     int  `json:"local"`
	Paid      int  `json:"paid"`
	Truncated bool `json:"truncated,omitempty"`
}

// Routed is one function and the tier chosen for it.
type Routed struct {
	Start      string `json:"start"`
	Name       string `json:"name,omitempty"`
	Cyclomatic int    `json:"cyclomatic"`
	Tier       string `json:"tier"`
}

// triageDoc is triage.json.
type triageDoc struct {
	Triage
	Functions []Routed `json:"functions"`
}

// routeTier chooses the tier of a function by its cyclomatic complexity.
func routeTier(cyclomatic int) string {
	if cyclomatic <= CyclomaticLimit {
		return TierLocal
	}
	return TierPaid
}

func (s *Service) triagePath(id string) string { return filepath.Join(s.Store.Dir(id), "triage.json") }

// Routing reads the tier chosen for each function of analysis id; it is empty when the target has none listed.
func (s *Service) Routing(id string) ([]Routed, error) {
	var doc triageDoc
	if err := readJSON(s.triagePath(id), &doc); err != nil {
		return nil, err
	}
	return doc.Functions, nil
}

// triage routes each function of the target: complexity up to CyclomaticLimit goes to the local model, the rest is
// left for a paid one and stops the analysis at the pause gate unless the analysis already runs on the paid tier.
// A target whose functions cannot be listed (another format, an older dendrite-core) is not triaged.
func (s *Service) triage(ctx context.Context, st *State) error {
	list, err := s.Functions.Functions(ctx, st.Binary)
	switch {
	case errors.Is(err, coreclient.ErrFunctionsUnsupported), errors.Is(err, coreclient.ErrNoFunctions):
		st.Stage = StageTriaged
		return nil
	case err != nil:
		return err
	}
	doc := triageDoc{Triage: Triage{Limit: CyclomaticLimit, Truncated: list.Truncated}}
	for _, f := range list.Functions {
		r := Routed{Start: f.Start, Cyclomatic: f.Cyclomatic, Tier: routeTier(f.Cyclomatic)}
		if f.Name != nil {
			r.Name = *f.Name
		}
		if r.Tier == TierLocal {
			doc.Local++
		} else {
			doc.Paid++
		}
		doc.Functions = append(doc.Functions, r)
	}
	if err := writeJSON(s.triagePath(st.ID), doc); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(s.Store.WorkspaceDir(st.ID), "triage.md"), []byte(triageText(doc))); err != nil {
		return err
	}
	st.Triage, st.Stage = &doc.Triage, StageTriaged
	if st.Tier != TierPaid && (doc.Paid > 0 || doc.Truncated) {
		st.Gate = gateReason(doc.Triage)
	}
	return nil
}

// gateReason says why the analysis waits, and how to go on.
func gateReason(t Triage) string {
	why := fmt.Sprintf("%d function(s) have cyclomatic complexity above %d and need a paid model", t.Paid, t.Limit)
	if t.Truncated {
		why += "; the sidecar spent its analysis budget, so some complexity may be undercounted"
	}
	return why + ": resume with --tier paid to allow it"
}

// triageText is the triage document of the workspace, so the routing can be searched like the rest of the analysis.
func triageText(d triageDoc) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Triage\n\n- Local model (cyclomatic <= %d): %d functions\n- Paid model (above): %d functions\n", d.Limit, d.Local, d.Paid)
	if d.Truncated {
		sb.WriteString("- The sidecar spent its analysis budget: the list may be incomplete and some complexity undercounted.\n")
	}
	listed := 0
	for _, f := range d.Functions {
		if f.Tier != TierPaid {
			continue
		}
		if listed == maxHardListed {
			sb.WriteString("- ... more functions above the limit are in triage.json\n")
			break
		}
		if listed == 0 {
			sb.WriteString("\n## Functions above the limit\n\n")
		}
		fmt.Fprintf(&sb, "- %s %s: cyclomatic %d\n", f.Start, orDefault(f.Name, "(unnamed)"), f.Cyclomatic)
		listed++
	}
	return sb.String()
}

func writeJSON(path string, v any) error {
	data, _ := json.MarshalIndent(v, "", "  ") // plain structs of strings and numbers always marshal
	return fsutil.WriteFileAtomic(path, data)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
