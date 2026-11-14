package plane

import (
	"testing"

	"Dendrite/pkg/tracker"
)

const devStatusFixture = `# Status

- **Active Milestone**: Milestone 2: Core
- **Current Focus**: DEN-020 and more
- **Last Updated**: 2026-11-01
- **Summary**: 1 | 2

| Rank | Ticket | Milestone | Feature | Points | Score |
|:--:|:--|:--|:--|:--:|:--:|
| **1** | **DEN-010** | Milestone 1 | FEAT-001 | 3 SP | 2.0 / 10 |

### Milestone 1: Base
- [x] **DEN-010**: Base work (` + "`pkg/base`" + `) [State: Done, Priority: High]
- [ ] **DEN-020**: Focused work
- [ ] **DEN-030**: Queued work [State: To Verify]

### Milestone 2: Core (Active)
- [ ] **DEN-040**: Core work

## Detailed Specs

### Ticket ` + "`DEN-010`" + `: Base
- **Target Package**: ` + "`pkg/from-spec`" + `
- **Governing Specs**: FEAT-001
- **Goal**: Build the base
- [ ] first criterion
- [x] second criterion
### Not a ticket header
- **Goal**: ignored after the header above

### Ticket DEN-040: Core
- **Target Package**: ` + "`pkg/core`" + `
## Next section
- **Goal**: also ignored
`

func TestParseDevStatusReadsEveryPart(t *testing.T) {
	data, err := ParseDevStatus(devStatusFixture)
	if err != nil {
		t.Fatal(err)
	}
	if data.ActiveMilestone != "Milestone 2: Core" || data.CurrentFocus != "DEN-020 and more" || data.LastUpdated != "2026-11-01" || data.Summary != "1 | 2" {
		t.Errorf("header = %+v", data)
	}
	if len(data.Milestones) != 2 {
		t.Errorf("milestones = %v", data.Milestones)
	}
	byID := map[string]ParsedDevTicket{}
	for _, tk := range data.Tickets {
		byID[tk.ID] = tk
	}
	base := byID["DEN-010"]
	if base.Status != tracker.StatusDone || base.TargetPackage != "pkg/base" || base.FeatureID != "FEAT-001" || base.StoryPoints != "3" || base.ComplexityScore != "2.0" {
		t.Errorf("DEN-010 = %+v", base)
	}
	if base.GoverningSpecs != "FEAT-001" || base.Goal != "Build the base" || len(base.AcceptanceCriteria) != 2 {
		t.Errorf("DEN-010 spec = %+v", base)
	}
	if byID["DEN-020"].Status != tracker.StatusInProgress {
		t.Errorf("a ticket named in the current focus is in progress, got %s", byID["DEN-020"].Status)
	}
	if byID["DEN-030"].Status != tracker.StatusInReview {
		t.Errorf("DEN-030 = %s", byID["DEN-030"].Status)
	}
	if byID["DEN-040"].Status != tracker.StatusTodo || byID["DEN-040"].TargetPackage != "pkg/core" {
		t.Errorf("DEN-040 = %+v", byID["DEN-040"])
	}
}
