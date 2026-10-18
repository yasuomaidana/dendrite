package synapse

import (
	"errors"
	"testing"
	"time"
)

func validSynapseFixture() *Synapse {
	return &Synapse{
		ID:        "web-api",
		Name:      "Web API Link",
		State:     StateActive,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
		UpdatedAt: time.Now().UTC().Add(-time.Minute),
		Members: []Member{
			{
				ProjectID: "dpid_0123456789abcdef0123456789abcdef",
				Alias:     "api-gateway",
				Role:      RoleProducer,
				PathHint:  "/repos/gateway",
			},
			{
				ProjectID: "dpid_abcdef0123456789abcdef0123456789",
				Alias:     "client-web",
				Role:      RoleConsumer,
				PathHint:  "/repos/web",
			},
		},
		Contracts: []Contract{
			{
				Kind:     "openapi",
				Producer: "api-gateway",
				Path:     "api/openapi.yaml",
				Consumers: []Consumer{
					{Alias: "client-web", Path: "src/api/types.ts"},
				},
			},
		},
		Sync: Sync{
			Strategy: "go.work",
		},
		Parity: Parity{
			Commands: map[string]string{
				"api-gateway": "go test ./...",
				"client-web":  "bun test",
			},
		},
	}
}

func TestValidateTransition(t *testing.T) {
	allStates := []State{StateDraft, StateActive, StateDrifted, StateBroken, StateArchived}

	// Matrix of expected results for 25 state transitions.
	// Allowed:
	// draft -> active, draft -> archived
	// active -> drifted, active -> broken, active -> archived
	// drifted -> active, drifted -> broken, drifted -> archived
	// broken -> active, broken -> archived
	allowedTransitions := map[State]map[State]bool{
		StateDraft: {
			StateActive:   true,
			StateArchived: true,
		},
		StateActive: {
			StateDrifted:  true,
			StateBroken:   true,
			StateArchived: true,
		},
		StateDrifted: {
			StateActive:   true,
			StateBroken:   true,
			StateArchived: true,
		},
		StateBroken: {
			StateActive:   true,
			StateArchived: true,
		},
		StateArchived: {},
	}

	for _, from := range allStates {
		for _, to := range allStates {
			from, to := from, to
			name := string(from) + "_to_" + string(to)
			t.Run(name, func(t *testing.T) {
				err := ValidateTransition(from, to)
				expectedAllowed := allowedTransitions[from][to]
				if expectedAllowed && err != nil {
					t.Fatalf("expected transition from %s to %s to be allowed, got error: %v", from, to, err)
				}
				if !expectedAllowed {
					if err == nil {
						t.Fatalf("expected transition from %s to %s to fail, got nil", from, to)
					}
					if !errors.Is(err, ErrIllegalStateTransition) {
						t.Fatalf("expected ErrIllegalStateTransition, got %v", err)
					}
				}
			})
		}
	}

	// Extra edge cases: unknown states
	unknownTests := []struct {
		name string
		from State
		to   State
	}{
		{"unknown_from", State("unknown"), StateActive},
		{"unknown_to", StateDraft, State("unknown")},
		{"unknown_to_archived", State("invalid"), StateArchived},
	}
	for _, tc := range unknownTests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateTransition(tc.from, tc.to); err == nil {
				t.Fatalf("expected error for transition %s -> %s, got nil", tc.from, tc.to)
			}
		})
	}
}

func TestSynapseTransition(t *testing.T) {
	s := validSynapseFixture()
	s.State = StateDraft
	before := s.UpdatedAt

	// Valid transition draft -> active
	if err := s.Transition(StateActive); err != nil {
		t.Fatalf("unexpected error on valid transition: %v", err)
	}
	if s.State != StateActive {
		t.Fatalf("expected state %s, got %s", StateActive, s.State)
	}
	if !s.UpdatedAt.After(before) {
		t.Fatalf("expected UpdatedAt to be updated: before=%v, after=%v", before, s.UpdatedAt)
	}

	// Invalid transition active -> draft
	if err := s.Transition(StateDraft); err == nil {
		t.Fatalf("expected error on illegal transition active -> draft, got nil")
	}
	if s.State != StateActive {
		t.Fatalf("expected state to remain %s after failed transition, got %s", StateActive, s.State)
	}

	// Nil receiver
	var nilSynapse *Synapse
	if err := nilSynapse.Transition(StateActive); err == nil {
		t.Fatalf("expected error on nil synapse transition, got nil")
	}
}

func TestSynapseValidate(t *testing.T) {
	t.Run("nil_synapse", func(t *testing.T) {
		var s *Synapse
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for nil synapse, got nil")
		}
	})

	t.Run("valid_fixture", func(t *testing.T) {
		s := validSynapseFixture()
		if err := s.Validate(); err != nil {
			t.Fatalf("expected valid fixture to pass, got: %v", err)
		}
	})

	t.Run("invalid_ids", func(t *testing.T) {
		invalidIDs := []string{
			"",
			"-invalid",
			"Invalid",
			"has_underscore",
			"has.dot",
			"has space",
			"has/slash",
		}
		for _, id := range invalidIDs {
			s := validSynapseFixture()
			s.ID = id
			if err := s.Validate(); err == nil {
				t.Fatalf("expected error for invalid id %q, got nil", id)
			}
		}
	})

	t.Run("valid_ids", func(t *testing.T) {
		validIDs := []string{
			"a",
			"0",
			"web-api",
			"service-1-v2",
			"abc-123-def",
		}
		for _, id := range validIDs {
			s := validSynapseFixture()
			s.ID = id
			if err := s.Validate(); err != nil {
				t.Fatalf("expected valid id %q to pass, got: %v", id, err)
			}
		}
	})

	t.Run("invalid_state", func(t *testing.T) {
		s := validSynapseFixture()
		s.State = State("in-progress")
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for invalid state, got nil")
		}
	})

	t.Run("insufficient_members", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members = nil
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for 0 members, got nil")
		}

		s.Members = []Member{
			{ProjectID: "dpid_1", Alias: "a", Role: RolePeer},
		}
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for 1 member, got nil")
		}
	})

	t.Run("member_empty_alias", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members[0].Alias = ""
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for empty member alias, got nil")
		}
	})

	t.Run("member_duplicate_alias", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members[1].Alias = s.Members[0].Alias
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for duplicate member alias, got nil")
		}
	})

	t.Run("member_empty_project_id", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members[0].ProjectID = ""
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for empty member project_id, got nil")
		}
	})

	t.Run("member_duplicate_project_id", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members[1].ProjectID = s.Members[0].ProjectID
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for duplicate member project_id, got nil")
		}
	})

	t.Run("member_invalid_role", func(t *testing.T) {
		s := validSynapseFixture()
		s.Members[0].Role = Role("maintainer")
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for invalid role, got nil")
		}
	})

	t.Run("contract_valid_kinds", func(t *testing.T) {
		kinds := []string{"proto", "openapi", "jsonschema", "go", "rust", "ts", "pydantic", "ffi", "migrations"}
		for _, k := range kinds {
			s := validSynapseFixture()
			s.Contracts[0].Kind = k
			if err := s.Validate(); err != nil {
				t.Fatalf("expected valid kind %q to pass, got: %v", k, err)
			}
		}
	})

	t.Run("contract_invalid_kind", func(t *testing.T) {
		s := validSynapseFixture()
		s.Contracts[0].Kind = "graphql"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for invalid contract kind, got nil")
		}
	})

	t.Run("contract_producer_not_member", func(t *testing.T) {
		s := validSynapseFixture()
		s.Contracts[0].Producer = "nonexistent-member"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error when producer is not member alias, got nil")
		}
	})

	t.Run("contract_consumer_not_member", func(t *testing.T) {
		s := validSynapseFixture()
		s.Contracts[0].Consumers[0].Alias = "nonexistent-consumer"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error when consumer is not member alias, got nil")
		}
	})

	t.Run("sync_strategies", func(t *testing.T) {
		validStrategies := []string{"", "go.work", "cargo-patch", "pnpm-workspace", "none"}
		for _, strat := range validStrategies {
			s := validSynapseFixture()
			s.Sync.Strategy = strat
			if err := s.Validate(); err != nil {
				t.Fatalf("expected valid sync strategy %q to pass, got: %v", strat, err)
			}
		}

		s := validSynapseFixture()
		s.Sync.Strategy = "invalid-strategy"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error for invalid sync strategy, got nil")
		}
	})

	t.Run("parity_key_not_member", func(t *testing.T) {
		s := validSynapseFixture()
		s.Parity.Commands["external-repo"] = "make test"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error when parity command key is not a member alias, got nil")
		}
	})
}

func TestSynapseMemberLookup(t *testing.T) {
	s := validSynapseFixture()

	// By alias
	m, ok := s.Member("api-gateway")
	if !ok || m == nil {
		t.Fatalf("expected to find member by alias 'api-gateway'")
	}
	if m.Role != RoleProducer {
		t.Fatalf("expected RoleProducer, got %s", m.Role)
	}

	// By project ID
	m2, ok := s.Member("dpid_abcdef0123456789abcdef0123456789")
	if !ok || m2 == nil {
		t.Fatalf("expected to find member by project ID")
	}
	if m2.Alias != "client-web" {
		t.Fatalf("expected Alias client-web, got %s", m2.Alias)
	}

	// Not found
	if _, ok := s.Member("unknown"); ok {
		t.Fatalf("expected member lookup for 'unknown' to return false")
	}

	// Empty string
	if _, ok := s.Member(""); ok {
		t.Fatalf("expected member lookup for empty string to return false")
	}

	// Nil synapse
	var nilSynapse *Synapse
	if _, ok := nilSynapse.Member("api-gateway"); ok {
		t.Fatalf("expected member lookup on nil synapse to return false")
	}
}
