package ticketlock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/statedir"
)

const (
	// TokenTTL is how long a session token stays valid.
	TokenTTL = 24 * time.Hour

	// Environment variables that describe the calling agent.
	SessionIDEnv = "DENDRITE_SESSION_ID"
	AgentEnv     = "DENDRITE_AGENT"
	TierEnv      = "DENDRITE_MODEL_TIER"
)

// Token identifies one agent session. Parallel agents share one .dendrite/, so each session has its
// own file under session/tokens/.
type Token struct {
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent"`
	ModelTier string    `json:"model_tier,omitempty"`
	StartedAt time.Time `json:"started_at"`
	Worktree  string    `json:"worktree,omitempty"`
}

// Store keeps tokens and claims under Dir (the session state directory).
type Store struct {
	Dir string
}

// NewStore returns the store of the current project: .dendrite/session in the main checkout.
func NewStore() *Store {
	return &Store{Dir: statedir.Resolve(".dendrite/session")}
}

// TokenOptions are the inputs of MintToken; empty fields are detected from the environment.
type TokenOptions struct {
	SessionID string
	Agent     string
	ModelTier string
	Worktree  string
}

// psName is a seam: the command name of a process, from ps.
var psName = func(pid int) string {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}

// DetectAgent names the calling agent: DENDRITE_AGENT, then Claude Code's CLAUDECODE=1, then the
// parent process name, then "unknown".
func DetectAgent() string {
	if a := os.Getenv(AgentEnv); a != "" {
		return a
	}
	if os.Getenv("CLAUDECODE") == "1" {
		return "claude"
	}
	if n := psName(os.Getppid()); n != "" {
		return n
	}
	return "unknown"
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "s-" + hex.EncodeToString(b)
}

// MintToken writes a token for a new session, or refreshes the one with the same session id.
func (s *Store) MintToken(o TokenOptions, now time.Time) (Token, error) {
	t := Token{SessionID: o.SessionID, Agent: o.Agent, ModelTier: o.ModelTier, StartedAt: now.UTC(), Worktree: o.Worktree}
	if t.SessionID == "" {
		t.SessionID = os.Getenv(SessionIDEnv)
	}
	if t.SessionID == "" {
		t.SessionID = newSessionID()
	}
	if t.Agent == "" {
		t.Agent = DetectAgent()
	}
	if t.ModelTier == "" {
		t.ModelTier = os.Getenv(TierEnv)
	}
	data, _ := json.MarshalIndent(t, "", "  ")
	if err := fsutil.WriteFileAtomic(s.tokenPath(t.SessionID), append(data, '\n')); err != nil {
		return Token{}, fmt.Errorf("write session token: %w", err)
	}
	return t, nil
}

func (s *Store) tokenPath(id string) string {
	return filepath.Join(s.Dir, "tokens", slug(id)+".json")
}

// LoadToken reads one token by session id.
func (s *Store) LoadToken(id string) (Token, error) {
	return readJSON[Token](s.tokenPath(id))
}

// Valid reports whether the token is still inside its lifetime at now.
func (t Token) Valid(now time.Time) bool {
	return !t.StartedAt.IsZero() && now.Sub(t.StartedAt) < TokenTTL
}

// LatestToken returns the newest token that is still valid at now.
func (s *Store) LatestToken(now time.Time) (Token, bool) {
	var best Token
	found := false
	for _, t := range s.tokens() {
		if t.Valid(now) && (!found || t.StartedAt.After(best.StartedAt)) {
			best, found = t, true
		}
	}
	return best, found
}

// PruneTokens deletes the tokens that are no longer valid at now and returns how many it removed.
func (s *Store) PruneTokens(now time.Time) int {
	removed := 0
	for _, t := range s.tokens() {
		if !t.Valid(now) && os.Remove(s.tokenPath(t.SessionID)) == nil {
			removed++
		}
	}
	return removed
}

// tokens lists every readable token; unreadable files are skipped.
func (s *Store) tokens() []Token {
	entries, _ := os.ReadDir(filepath.Join(s.Dir, "tokens"))
	var out []Token
	for _, e := range entries {
		if t, err := readJSON[Token](filepath.Join(s.Dir, "tokens", e.Name())); err == nil && t.SessionID != "" {
			out = append(out, t)
		}
	}
	return out
}

// slug makes a session id or branch name safe as a file name.
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

func readJSON[T any](path string) (T, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// ErrNoClaim is returned when a worktree has no claim file.
var ErrNoClaim = errors.New("no claim")
