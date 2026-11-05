// Package immune wraps the SkillSpector scanner as Dendrite's gate for agent skills. It owns no scanning
// logic: it runs the scanner, reads its JSON report and turns the risk score into a verdict.
package immune

import "fmt"

// Verdict is what the gate decides for a scanned skill.
type Verdict string

const (
	// Approve means the skill may be installed without a human.
	Approve Verdict = "approve"
	// Hold means a human must record a decision before the skill is installed.
	Hold Verdict = "hold"
	// Quarantine means the skill must not be installed.
	Quarantine Verdict = "quarantine"
)

// Thresholds are the highest score each verdict still covers: a score up to ApproveMax is approved, up to
// HoldMax is held, anything above is quarantined.
type Thresholds struct{ ApproveMax, HoldMax int }

// DefaultThresholds are the roadmap policy: 0-20 approve, 21-50 hold, 51+ quarantine.
var DefaultThresholds = Thresholds{ApproveMax: 20, HoldMax: 50}

// Validate rejects thresholds that do not form three ordered bands.
func (t Thresholds) Validate() error {
	if t.ApproveMax < 0 || t.HoldMax <= t.ApproveMax {
		return fmt.Errorf("thresholds must satisfy 0 <= approve-max < hold-max, got %d and %d", t.ApproveMax, t.HoldMax)
	}
	return nil
}

// Classify maps a risk score to a verdict.
func (t Thresholds) Classify(score int) Verdict {
	switch {
	case score <= t.ApproveMax:
		return Approve
	case score <= t.HoldMax:
		return Hold
	default:
		return Quarantine
	}
}
