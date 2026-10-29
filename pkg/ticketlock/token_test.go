package ticketlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

// brokenStore is a store whose directory sits below a regular file, so every write fails.
func brokenStore(t *testing.T) *Store {
	t.Helper()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Store{Dir: filepath.Join(file, "session")}
}

func TestMintTokenWritesOneFilePerSession(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	t.Setenv(SessionIDEnv, "")
	t.Setenv(AgentEnv, "agy")
	t.Setenv(TierEnv, "6")

	a, err := s.MintToken(TokenOptions{Worktree: "/wt"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.MintToken(TokenOptions{SessionID: "Explicit/ID", Agent: "claude", ModelTier: "9"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if a.SessionID == b.SessionID || a.SessionID == "" {
		t.Fatalf("sessions must differ: %q %q", a.SessionID, b.SessionID)
	}
	if a.Agent != "agy" || a.ModelTier != "6" || a.Worktree != "/wt" {
		t.Errorf("env detection: %+v", a)
	}
	if b.Agent != "claude" || b.ModelTier != "9" {
		t.Errorf("explicit options lost: %+v", b)
	}
	got, err := s.LoadToken("Explicit/ID")
	if err != nil || got.SessionID != "Explicit/ID" {
		t.Errorf("LoadToken = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "tokens", "explicit-id.json")); err != nil {
		t.Errorf("token file name is not the slug of the session id: %v", err)
	}
	if _, err := s.LoadToken("nope"); err == nil {
		t.Error("LoadToken of an unknown session must fail")
	}
}

func TestMintTokenTakesSessionIDFromEnvAndReportsWriteErrors(t *testing.T) {
	t.Setenv(SessionIDEnv, "from-env")
	s := &Store{Dir: t.TempDir()}
	tok, err := s.MintToken(TokenOptions{Agent: "human"}, t0)
	if err != nil || tok.SessionID != "from-env" {
		t.Fatalf("tok = %+v, err = %v", tok, err)
	}
	if _, err := brokenStore(t).MintToken(TokenOptions{Agent: "human"}, t0); err == nil {
		t.Error("a store that cannot be written must fail")
	}
}

func TestDetectAgent(t *testing.T) {
	old := psName
	t.Cleanup(func() { psName = old })
	psName = func(int) string { return "" }

	t.Setenv(AgentEnv, "")
	t.Setenv("CLAUDECODE", "")
	if got := DetectAgent(); got != "unknown" {
		t.Errorf("nothing known = %q", got)
	}
	psName = func(int) string { return "agy" }
	if got := DetectAgent(); got != "agy" {
		t.Errorf("parent process = %q", got)
	}
	t.Setenv("CLAUDECODE", "1")
	if got := DetectAgent(); got != "claude" {
		t.Errorf("CLAUDECODE = %q", got)
	}
	t.Setenv(AgentEnv, "copilot")
	if got := DetectAgent(); got != "copilot" {
		t.Errorf("DENDRITE_AGENT = %q", got)
	}
}

func TestPsNameAsksPS(t *testing.T) {
	_, err := exec.LookPath("ps")
	if got := psName(os.Getpid()); (got != "") != (err == nil) {
		t.Errorf("psName(own pid) = %q with ps available = %v", got, err == nil)
	}
	if got := psName(1 << 30); got != "" {
		t.Errorf("a process that does not exist = %q", got)
	}
}

func TestLatestTokenAndPrune(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	mint := func(id string, at time.Time) {
		if _, err := s.MintToken(TokenOptions{SessionID: id, Agent: "claude"}, at); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := s.LatestToken(t0); ok {
		t.Fatal("an empty store has no token")
	}
	mint("old", t0.Add(-TokenTTL-time.Minute))
	if _, ok := s.LatestToken(t0); ok {
		t.Fatal("an expired token must not count")
	}
	mint("older-valid", t0.Add(-2*time.Hour))
	mint("newest", t0.Add(-time.Hour))
	mint("middle", t0.Add(-90*time.Minute))
	if got, ok := s.LatestToken(t0); !ok || got.SessionID != "newest" {
		t.Errorf("latest = %+v, %v", got, ok)
	}

	// Files that are not tokens are ignored and never deleted.
	tokens := filepath.Join(s.Dir, "tokens")
	_ = os.WriteFile(filepath.Join(tokens, "bad.json"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(tokens, "empty.json"), []byte("{}"), 0o644)
	// A token whose file name is not its slug cannot be removed by id, so it is not counted.
	_ = os.WriteFile(filepath.Join(tokens, "weird.json"), []byte(`{"session_id":"weird-id","started_at":"2020-01-01T00:00:00Z"}`), 0o644)

	if n := s.PruneTokens(t0); n != 1 {
		t.Errorf("pruned %d, want 1 (only the expired token with a matching file)", n)
	}
	if _, err := s.LoadToken("old"); err == nil {
		t.Error("the expired token is still there")
	}
	if _, err := s.LoadToken("newest"); err != nil {
		t.Errorf("a valid token was pruned: %v", err)
	}
	if n := s.PruneTokens(t0); n != 0 {
		t.Errorf("second prune removed %d", n)
	}
}

func TestTokenValid(t *testing.T) {
	if (Token{}).Valid(t0) {
		t.Error("a token with no start time is not valid")
	}
	if !(Token{StartedAt: t0.Add(-time.Hour)}).Valid(t0) {
		t.Error("a one-hour-old token is valid")
	}
	if (Token{StartedAt: t0.Add(-TokenTTL)}).Valid(t0) {
		t.Error("a token exactly at its lifetime is expired")
	}
}

func TestSlug(t *testing.T) {
	if got := slug("Agent/DEN-375_x.y z"); got != "agent-den-375_x.y-z" {
		t.Errorf("slug = %q", got)
	}
}

func TestReadJSONReportsBadFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	if _, err := readJSON[Token](p); !os.IsNotExist(err) {
		t.Errorf("missing file: %v", err)
	}
	_ = os.WriteFile(p, []byte("{"), 0o644)
	if _, err := readJSON[Token](p); err == nil {
		t.Error("invalid JSON must fail")
	}
}

func TestNewStoreUsesTheStateDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", dir)
	if got := NewStore().Dir; got != filepath.Join(dir, "session") {
		t.Errorf("Dir = %q", got)
	}
}
