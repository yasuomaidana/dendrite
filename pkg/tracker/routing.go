package tracker

import (
	"sort"
)

// DifficultyTier represents the architectural and algorithmic difficulty of a work item.
type DifficultyTier int

const (
	TierL1Trivial  DifficultyTier = 1 // Boilerplate, config, simple structs/CLI flags
	TierL2Low      DifficultyTier = 2 // Standard CRUD, wrappers, unit tests
	TierL3Medium   DifficultyTier = 3 // Multi-file logic, state machines, SQL queries
	TierL4High     DifficultyTier = 4 // Complex concurrency, DAG resolution, cross-module refactor
	TierL5Critical DifficultyTier = 5 // Core architecture, security, novel algorithms
)

// String returns the canonical identifier for the difficulty tier.
func (d DifficultyTier) String() string {
	switch d {
	case TierL1Trivial:
		return "L1_TRIVIAL"
	case TierL2Low:
		return "L2_LOW"
	case TierL3Medium:
		return "L3_MEDIUM"
	case TierL4High:
		return "L4_HIGH"
	case TierL5Critical:
		return "L5_CRITICAL"
	default:
		return "L3_MEDIUM"
	}
}

// InferenceEngine classifies the model capability required to develop the ticket.
type InferenceEngine string

const (
	EngineLocalLLM        InferenceEngine = "LOCAL_LLM"
	EngineVendorFast      InferenceEngine = "VENDOR_FAST"
	EngineVendorReasoning InferenceEngine = "VENDOR_REASONING"
)

// AuditStatus tracks the review lifecycle for tickets developed speculatively by local models.
type AuditStatus string

const (
	AuditNone              AuditStatus = "NONE"
	AuditPendingVendor     AuditStatus = "PENDING_VENDOR_AUDIT"
	AuditValidatedMerged   AuditStatus = "AUDITED_AND_MERGED"
	AuditEscalatedToVendor AuditStatus = "ESCALATED_TO_VENDOR"
)

// RoutingTicket encapsulates all routing, governance, and dependency attributes for an issue.
type RoutingTicket struct {
	ID             string         `json:"id"`
	GitHubIssueNum int            `json:"github_issue_num,omitempty"`
	SequenceOrder  int            `json:"sequence_order"`
	Module         string         `json:"module"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Priority       string         `json:"priority"` // e.g. "P0_TOP_BLOCKER", "urgent"
	LocalFeasible  bool           `json:"local_feasible"`
	Difficulty     DifficultyTier `json:"difficulty_tier"`
	DependsOn      []string       `json:"depends_on,omitempty"`
	Blocks         []string       `json:"blocks,omitempty"`
	RequiresAudit  bool           `json:"requires_audit"`
	AuditState     AuditStatus    `json:"audit_state"`
	Completed      bool           `json:"completed"`
}

// EnrichTicketClassification enforces deterministic routing rules during ticket creation or triage.
func EnrichTicketClassification(t *RoutingTicket) {
	if t.Difficulty <= TierL2Low {
		t.LocalFeasible = true
	} else if t.Difficulty >= TierL4High {
		t.LocalFeasible = false
	}

	if t.LocalFeasible {
		t.RequiresAudit = true
		if t.AuditState == "" {
			t.AuditState = AuditNone
		}
	}
}

// SelectInferenceEngine decides which model executes the ticket based on budget mode and difficulty.
func (t *RoutingTicket) SelectInferenceEngine(lowBudgetMode bool) (InferenceEngine, bool) {
	// Rule 1: Tickets flagged local_feasible ALWAYS use Local LLM for development
	// and get reviewed later by the Vendor LLM via `audit-changes`.
	if t.LocalFeasible {
		return EngineLocalLLM, true
	}

	// Rule 2: In Low-Budget mode, attempt L3 tickets locally, but defer L4/L5 tickets.
	if lowBudgetMode {
		if t.Difficulty <= TierL3Medium {
			t.RequiresAudit = true
			return EngineLocalLLM, true
		}
		return "", false // Defer ticket until vendor budget is restored
	}

	// Rule 3: Normal budget mode for non-local-feasible tickets
	if t.Difficulty >= TierL4High {
		return EngineVendorReasoning, true
	}
	return EngineVendorFast, true
}

// SortForLowBudgetQueue orders unblocked tickets so `local_feasible` and lowest-difficulty
// tickets are completed first, maximizing completed work before hitting complex blockers.
func SortForLowBudgetQueue(tickets []*RoutingTicket) {
	sort.SliceStable(tickets, func(i, j int) bool {
		// 1. P0_TOP_BLOCKER / urgent always comes first
		isTopI := tickets[i].Priority == "P0_TOP_BLOCKER" || tickets[i].Priority == "urgent"
		isTopJ := tickets[j].Priority == "P0_TOP_BLOCKER" || tickets[j].Priority == "urgent"
		if isTopI != isTopJ {
			return isTopI
		}

		// 2. LocalFeasible tickets first
		if tickets[i].LocalFeasible != tickets[j].LocalFeasible {
			return tickets[i].LocalFeasible
		}

		// 3. Lowest difficulty tier first (L1 -> L2 -> L3)
		if tickets[i].Difficulty != tickets[j].Difficulty {
			return tickets[i].Difficulty < tickets[j].Difficulty
		}

		// 4. Preserve topological sequence order
		return tickets[i].SequenceOrder < tickets[j].SequenceOrder
	})
}
