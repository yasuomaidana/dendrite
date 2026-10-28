package searchguard

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AllowEnv, when set to 1, lets every search through (and logs it as a bypass).
const AllowEnv = "DENDRITE_ALLOW_RG"

// RetryWindow is how long a blocked command stays allowed to be repeated.
const RetryWindow = 30 * time.Minute

// Decisions the guard records.
const (
	DecisionBlocked      = "blocked"
	DecisionAllowedRetry = "allowed-retry"
	DecisionBypass       = "bypass"
	DecisionUsed         = "used"
)

// HookInput is what an agent's pre-tool hook receives on stdin.
type HookInput struct {
	SessionID string    `json:"session_id"`
	Cwd       string    `json:"cwd"`
	ToolName  string    `json:"tool_name"`
	ToolInput ToolInput `json:"tool_input"`
}

// Event is one line of the usage log.
type Event struct {
	Time     time.Time `json:"ts"`
	Session  string    `json:"session,omitempty"`
	Project  string    `json:"project,omitempty"`
	Kind     string    `json:"kind"`
	Decision string    `json:"decision"`
	Command  string    `json:"command,omitempty"`
}

// Decision is the guard's verdict on one tool call.
type Decision struct {
	Block bool
	// Message is shown to the agent when Block is set.
	Message string
}

// Guard decides on search commands and records what happened. Dir holds its state and the usage
// log; it is created on first use.
type Guard struct {
	Dir    string
	Now    func() time.Time
	Getenv func(string) string
}

// New returns a guard that keeps its files in dir.
func New(dir string) *Guard {
	return &Guard{Dir: dir, Now: time.Now, Getenv: os.Getenv}
}

// LogPath is the usage log.
func (g *Guard) LogPath() string { return filepath.Join(g.Dir, "tool-usage.jsonl") }

func (g *Guard) statePath() string { return filepath.Join(g.Dir, "search-guard.json") }

// Check judges one tool call. Raw searches are blocked the first time a session attempts the exact
// command and allowed when it is repeated within RetryWindow; calls to Dendrite are only logged.
func (g *Guard) Check(in HookInput) Decision {
	searches, usesDendrite := Classify(in.ToolName, in.ToolInput)
	now := g.Now()
	ev := Event{Time: now, Session: in.SessionID, Project: in.Cwd}

	if len(searches) == 0 {
		if usesDendrite {
			ev.Kind = KindDendrite
			if strings.HasPrefix(in.ToolName, "mcp__") {
				ev.Kind = KindDendriteMCP
				ev.Command = in.ToolName
			} else {
				ev.Command = truncate(in.ToolInput.Command)
			}
			ev.Decision = DecisionUsed
			g.log(ev)
		}
		return Decision{}
	}

	text := in.ToolInput.Command
	if in.ToolName == "Grep" {
		text = in.ToolInput.Pattern + "\x00" + in.ToolInput.Path
	}
	ev.Command = truncate(strings.ReplaceAll(text, "\x00", " "))

	if g.Getenv(AllowEnv) == "1" || strings.Contains(in.ToolInput.Command, AllowEnv+"=1") {
		for _, kind := range searches {
			ev.Kind, ev.Decision = kind, DecisionBypass
			g.log(ev)
		}
		return Decision{}
	}

	key := fmt.Sprintf("%x", sha256.Sum256([]byte(in.SessionID+"\x00"+strings.Join(strings.Fields(text), " "))))[:16]
	state := g.loadState(now)
	if _, repeated := state[key]; repeated {
		for _, kind := range searches {
			ev.Kind, ev.Decision = kind, DecisionAllowedRetry
			g.log(ev)
		}
		return Decision{}
	}
	state[key] = now.Unix()
	g.saveState(state)
	for _, kind := range searches {
		ev.Kind, ev.Decision = kind, DecisionBlocked
		g.log(ev)
	}
	return Decision{Block: true, Message: Hint}
}

// Hint is what a blocked agent is told.
const Hint = "dendrite-first: a raw recursive search (rg, grep -r, the Grep tool) returns every match and costs many tokens. " +
	"Try, cheapest first:\n" +
	"  dendrite rag search \"<what you are looking for>\" -k 8   ranked, token-budgeted (MCP: dendrite_rag_retrieve with detail \"map\", then dendrite_rag_show)\n" +
	"  dendrite rag symbols <Name>                               definitions and references\n" +
	"  dendrite graph explain <symbol-or-file>                   callers, callees, structure (MCP: dendrite_graph_explain)\n" +
	"  dendrite rag history \"<query>\"                            past changes and their reasons\n" +
	"  dendrite rag grep '<regex>'                               exact pattern, indexed and capped\n" +
	"If those found nothing, run this same command again and it will go through; scope it to a path and cap its output."

func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// loadState returns the commands seen within RetryWindow, keyed by hash.
func (g *Guard) loadState(now time.Time) map[string]int64 {
	state := map[string]int64{}
	if data, err := os.ReadFile(g.statePath()); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	for k, at := range state {
		if now.Sub(time.Unix(at, 0)) > RetryWindow {
			delete(state, k)
		}
	}
	return state
}

func (g *Guard) saveState(state map[string]int64) {
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := os.MkdirAll(g.Dir, 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", g.statePath(), os.Getpid())
	if os.WriteFile(tmp, data, 0o644) == nil {
		if os.Rename(tmp, g.statePath()) != nil {
			_ = os.Remove(tmp)
		}
	}
}

// log appends an event; a failure to log never affects the agent.
func (g *Guard) log(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := os.MkdirAll(g.Dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(g.LogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// Summary totals the usage log.
type Summary struct {
	Since time.Time `json:"since"`
	// Dendrite counts calls to dendrite (CLI) and its MCP tools.
	Dendrite int `json:"dendrite"`
	// RawSearches counts raw searches that actually ran (allowed on retry, or bypassed), by kind.
	RawSearches map[string]int `json:"raw_searches"`
	// Blocked counts first attempts the guard stopped, by kind.
	Blocked map[string]int `json:"blocked"`
	// Share is Dendrite calls / (Dendrite calls + raw searches that ran); 1 when there are none.
	Share float64 `json:"share"`
}

// Summarize reads a usage log, counting events at or after since.
func Summarize(r io.Reader, since time.Time) Summary {
	s := Summary{Since: since, RawSearches: map[string]int{}, Blocked: map[string]int{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Time.Before(since) {
			continue
		}
		switch {
		case ev.Decision == DecisionUsed:
			s.Dendrite++
		case ev.Decision == DecisionBlocked:
			s.Blocked[ev.Kind]++
		case IsRawSearch(ev.Kind):
			s.RawSearches[ev.Kind]++
		}
	}
	raw := 0
	for _, n := range s.RawSearches {
		raw += n
	}
	s.Share = 1
	if total := s.Dendrite + raw; total > 0 {
		s.Share = float64(s.Dendrite) / float64(total)
	}
	return s
}

// Kinds lists the kinds present in counts, sorted.
func Kinds(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
