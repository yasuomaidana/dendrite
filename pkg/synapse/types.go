package synapse

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// State represents the lifecycle typestate of a Synapse manifest.
type State string

const (
	StateDraft    State = "draft"
	StateActive   State = "active"
	StateDrifted  State = "drifted"
	StateBroken   State = "broken"
	StateArchived State = "archived"
)

// Role defines the relationship role of a member repository in a Synapse.
type Role string

const (
	RoleProducer Role = "producer"
	RoleConsumer Role = "consumer"
	RolePeer     Role = "peer"
)

// Standard typed errors for Synapse lifecycle and validation.
var (
	ErrIllegalStateTransition = errors.New("synapse: illegal state transition")
	ErrInvalidSynapse         = errors.New("synapse: invalid synapse")
)

var (
	idRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

	validStates = map[State]bool{
		StateDraft:    true,
		StateActive:   true,
		StateDrifted:  true,
		StateBroken:   true,
		StateArchived: true,
	}

	validContractKinds = map[string]bool{
		"proto":      true,
		"openapi":    true,
		"jsonschema": true,
		"go":         true,
		"rust":       true,
		"ts":         true,
		"pydantic":   true,
		"ffi":        true,
		"migrations": true,
	}

	validSyncStrategies = map[string]bool{
		"":               true,
		"go.work":        true,
		"cargo-patch":    true,
		"pnpm-workspace": true,
		"none":           true,
	}
)

// Source selects where a member's code is consumed from in develop mode.
type Source string

const (
	// SourceLocal links the member's checkout at PathHint (go.work use, cargo path patch).
	// The empty Source means SourceLocal.
	SourceLocal Source = "local"
	// SourceRemote consumes the member from Remote at Ref (go require/replace, cargo git patch).
	SourceRemote Source = "remote"
)

// Channel selects the branch every member tracks unless it sets its own Ref.
type Channel string

const (
	// ChannelStable tracks main: what a normal install runs.
	ChannelStable Channel = "stable"
	// ChannelDev tracks develop: what a develop-mode install runs.
	ChannelDev Channel = "dev"
)

// DefaultRef is the branch members on c track when they set no Ref; empty for no channel,
// which keeps following each checkout's own upstream.
func (c Channel) DefaultRef() string {
	switch c {
	case ChannelStable:
		return "main"
	case ChannelDev:
		return "develop"
	}
	return ""
}

// ParseChannel accepts stable|main and dev|develop.
func ParseChannel(s string) (Channel, error) {
	switch s {
	case "stable", "main":
		return ChannelStable, nil
	case "dev", "develop":
		return ChannelDev, nil
	}
	return "", fmt.Errorf("invalid channel %q (want stable or dev)", s)
}

// Member models a repository participating in a Synapse cross-project link.
type Member struct {
	ProjectID string `toml:"project_id" json:"project_id"`
	Alias     string `toml:"alias" json:"alias"`
	Role      Role   `toml:"role" json:"role"`
	PathHint  string `toml:"path_hint" json:"path_hint,omitempty"`
	// Source is local (default) or remote.
	Source Source `toml:"source" json:"source,omitempty"`
	// Remote is the git URL consumed when Source is remote, and fetched by sync.
	Remote string `toml:"remote" json:"remote,omitempty"`
	// Ref is the branch, tag or commit to track. Empty tracks the synapse channel's branch,
	// or the checkout's current branch upstream when the synapse has no channel.
	Ref string `toml:"ref" json:"ref,omitempty"`
	// Module is the Go module path or Cargo crate name consumers link against.
	Module string `toml:"module" json:"module,omitempty"`
	// LockedSHA and LockedAt record the commit Ref resolved to at the last sync.
	LockedSHA string    `toml:"locked_sha" json:"locked_sha,omitempty"`
	LockedAt  time.Time `toml:"locked_at" json:"locked_at,omitempty"`
	// PlaneProject is the Plane project identifier (e.g. "DGIT") tracking this member\'s tickets.
	PlaneProject string `toml:"plane_project" json:"plane_project,omitempty"`
}

// IsRemote reports whether the member is consumed from its remote rather than its local checkout.
func (m Member) IsRemote() bool { return m.Source == SourceRemote }

// TrackedRef is the ref the member follows on channel c: its own Ref (an override) when
// set, otherwise the channel's branch.
func (m Member) TrackedRef(c Channel) string {
	if m.Ref != "" {
		return m.Ref
	}
	return c.DefaultRef()
}

// Consumer specifies a consuming repository alias and the relative file path where the contract is consumed.
type Consumer struct {
	Alias string `toml:"alias" json:"alias"`
	Path  string `toml:"path" json:"path"`
}

// Contract represents an interface specification published by a producer and consumed by member repositories.
type Contract struct {
	Kind      string     `toml:"kind" json:"kind"`
	Producer  string     `toml:"producer" json:"producer"`
	Path      string     `toml:"path" json:"path"`
	Consumers []Consumer `toml:"consumers" json:"consumers"`
}

// Sync declares the workspace-level linking strategy across member repositories.
type Sync struct {
	Strategy string `toml:"strategy" json:"strategy"`
}

// Parity holds contract test and parity commands keyed by member repository alias.
type Parity struct {
	Commands map[string]string `toml:"commands" json:"commands"`
}

// Synapse is the core manifest defining typed cross-project links between autonomous repositories.
type Synapse struct {
	ID        string    `toml:"id" json:"id"`
	Name      string    `toml:"name" json:"name"`
	State     State     `toml:"state" json:"state"`
	CreatedAt time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
	// Channel picks the branch members track by default (stable: main, dev: develop).
	Channel   Channel    `toml:"channel" json:"channel,omitempty"`
	Members   []Member   `toml:"members" json:"members"`
	Contracts []Contract `toml:"contracts" json:"contracts"`
	Sync      Sync       `toml:"sync" json:"sync"`
	Parity    Parity     `toml:"parity" json:"parity"`
}

// ValidateTransition asserts whether a state transition adheres to the Synapse lifecycle state machine.
// Allowed transitions:
//   - draft -> active
//   - active -> drifted
//   - drifted -> active
//   - active -> broken
//   - drifted -> broken
//   - broken -> active
//   - any (non-archived) -> archived
//
// Same-state transitions, transitions from archived, and all other combinations are rejected.
func ValidateTransition(from, to State) error {
	if from == to {
		return ErrIllegalStateTransition
	}
	if from == StateArchived {
		return ErrIllegalStateTransition
	}
	if to == StateArchived {
		switch from {
		case StateDraft, StateActive, StateDrifted, StateBroken:
			return nil
		default:
			return ErrIllegalStateTransition
		}
	}
	switch from {
	case StateDraft:
		if to == StateActive {
			return nil
		}
	case StateActive:
		if to == StateDrifted || to == StateBroken {
			return nil
		}
	case StateDrifted:
		if to == StateActive || to == StateBroken {
			return nil
		}
	case StateBroken:
		if to == StateActive {
			return nil
		}
	}
	return ErrIllegalStateTransition
}

// Transition validates the requested state transition and updates State and UpdatedAt on success.
func (s *Synapse) Transition(to State) error {
	if s == nil {
		return errors.New("synapse is nil")
	}
	if err := ValidateTransition(s.State, to); err != nil {
		return err
	}
	s.State = to
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// Validate asserts that the Synapse manifest satisfies all structural and integrity constraints:
//   - id matches ^[a-z0-9][a-z0-9-]*$
//   - state is valid if specified
//   - at least 2 members are defined
//   - member aliases are non-empty and unique
//   - member project IDs are non-empty and unique
//   - member roles are valid (producer, consumer, peer)
//   - member sources are local, remote or empty; remote members carry a remote URL
//   - contracts have valid kind (proto, openapi, jsonschema, go, rust, ts, pydantic, ffi, migrations)
//   - contracts' producer and consumer aliases must be registered member aliases
//   - sync strategy is valid or empty (go.work, cargo-patch, pnpm-workspace, none, "")
//   - parity commands keys must be registered member aliases
func (s *Synapse) Validate() error {
	if s == nil {
		return errors.New("synapse is nil")
	}
	if !idRegex.MatchString(s.ID) {
		return fmt.Errorf("invalid synapse id %q: must match ^[a-z0-9][a-z0-9-]*$", s.ID)
	}
	if s.State != "" && !validStates[s.State] {
		return fmt.Errorf("invalid synapse state %q", s.State)
	}
	if s.Channel != "" && s.Channel.DefaultRef() == "" {
		return fmt.Errorf("invalid synapse channel %q (want stable or dev)", s.Channel)
	}
	if len(s.Members) < 2 {
		return fmt.Errorf("synapse must have at least 2 members, got %d", len(s.Members))
	}

	seenAliases := make(map[string]bool, len(s.Members))
	seenProjects := make(map[string]bool, len(s.Members))
	for i, m := range s.Members {
		if m.Alias == "" {
			return fmt.Errorf("member %d: alias cannot be empty", i)
		}
		if seenAliases[m.Alias] {
			return fmt.Errorf("duplicate member alias: %q", m.Alias)
		}
		seenAliases[m.Alias] = true

		if m.ProjectID == "" {
			return fmt.Errorf("member %q: project_id cannot be empty", m.Alias)
		}
		if seenProjects[m.ProjectID] {
			return fmt.Errorf("duplicate member project_id: %q", m.ProjectID)
		}
		seenProjects[m.ProjectID] = true

		switch m.Role {
		case RoleProducer, RoleConsumer, RolePeer:
		default:
			return fmt.Errorf("member %q: invalid role %q", m.Alias, m.Role)
		}

		switch m.Source {
		case "", SourceLocal:
		case SourceRemote:
			if m.Remote == "" {
				return fmt.Errorf("member %q: source remote requires a remote URL", m.Alias)
			}
		default:
			return fmt.Errorf("member %q: invalid source %q (want local or remote)", m.Alias, m.Source)
		}
	}

	for i, c := range s.Contracts {
		if !validContractKinds[c.Kind] {
			return fmt.Errorf("contract %d: invalid kind %q", i, c.Kind)
		}
		if !seenAliases[c.Producer] {
			return fmt.Errorf("contract %d: producer %q is not a member alias", i, c.Producer)
		}
		for j, cons := range c.Consumers {
			if !seenAliases[cons.Alias] {
				return fmt.Errorf("contract %d consumer %d: alias %q is not a member alias", i, j, cons.Alias)
			}
		}
	}

	if !validSyncStrategies[s.Sync.Strategy] {
		return fmt.Errorf("invalid sync strategy %q", s.Sync.Strategy)
	}

	for alias := range s.Parity.Commands {
		if !seenAliases[alias] {
			return fmt.Errorf("parity command key %q is not a member alias", alias)
		}
	}

	return nil
}

// Member retrieves a member repository by alias or project ID.
func (s *Synapse) Member(aliasOrID string) (*Member, bool) {
	if s == nil || aliasOrID == "" {
		return nil, false
	}
	for i := range s.Members {
		if s.Members[i].Alias == aliasOrID || s.Members[i].ProjectID == aliasOrID {
			return &s.Members[i], true
		}
	}
	return nil, false
}
