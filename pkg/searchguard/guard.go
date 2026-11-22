package searchguard

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
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
	// DecisionDenied is a retry with no Dendrite lookup since the search was stopped.
	DecisionDenied = "denied-retry"
	// DecisionScoped is a search whose targets are all single files: not a repository search.
	DecisionScoped = "scoped"
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

// Check judges one tool call. A raw recursive search is blocked the first time a session attempts the
// exact command. The same command is allowed again within RetryWindow only after the session made a
// Dendrite lookup (rag, ask, brief, graph) since it was stopped; without one it is denied again.
// Searches that name only single files are never blocked. Calls to Dendrite are logged.
func (g *Guard) Check(in HookInput) Decision {
	sc := scan(in.ToolName, in.ToolInput)
	now := g.Now()
	ev := Event{Time: now, Session: in.SessionID, Project: in.Cwd}

	if sc.retrieval {
		st := g.loadState(now)
		st.Retrieval[in.SessionID] = now.UnixNano()
		g.saveState(st)
	}
	if len(sc.searches) == 0 {
		if sc.usesDendrite {
			g.logDendrite(ev, in)
		}
		return Decision{}
	}

	text := in.ToolInput.Command
	if in.ToolName == "Grep" {
		text = in.ToolInput.Pattern + "\x00" + in.ToolInput.Path
	}
	ev.Command = truncate(strings.ReplaceAll(text, "\x00", " "))

	if g.Getenv(AllowEnv) == "1" || strings.Contains(in.ToolInput.Command, AllowEnv+"=1") {
		g.logAll(ev, sc.searches, DecisionBypass)
		return Decision{}
	}
	if onlyFiles(in.Cwd, sc.targets) {
		g.logAll(ev, sc.searches, DecisionScoped)
		return Decision{}
	}

	key := fmt.Sprintf("%x", sha256.Sum256([]byte(in.SessionID+"\x00"+strings.Join(strings.Fields(text), " "))))[:16]
	st := g.loadState(now)
	decision, d := judgeSearch(st, key, in.SessionID)
	if decision == DecisionBlocked {
		st.Blocked[key] = now.UnixNano()
		g.saveState(st)
	}
	g.logAll(ev, sc.searches, decision)
	return d
}

// judgeSearch decides on a raw search given what the session did before: the first attempt is
// blocked, a repeat goes through only when a lookup came after the block.
func judgeSearch(st state, key, session string) (string, Decision) {
	blockedAt, repeated := st.Blocked[key]
	switch {
	case !repeated:
		return DecisionBlocked, Decision{Block: true, Message: Hint}
	case st.Retrieval[session] > blockedAt:
		return DecisionAllowedRetry, Decision{}
	}
	return DecisionDenied, Decision{Block: true, Message: DeniedHint}
}

func (g *Guard) logDendrite(ev Event, in HookInput) {
	ev.Kind = KindDendrite
	ev.Command = truncate(in.ToolInput.Command)
	if strings.HasPrefix(in.ToolName, "mcp__") {
		ev.Kind = KindDendriteMCP
		ev.Command = in.ToolName
	}
	ev.Decision = DecisionUsed
	g.log(ev)
}

func (g *Guard) logAll(ev Event, kinds []string, decision string) {
	for _, kind := range kinds {
		ev.Kind, ev.Decision = kind, decision
		g.log(ev)
	}
}

// onlyFiles reports whether every search names at least one path and all of them are regular files
// (relative paths resolve against cwd): reading named files is not exploring the repository.
func onlyFiles(cwd string, targets [][]string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, paths := range targets {
		if len(paths) == 0 {
			return false
		}
		for _, p := range paths {
			if !filepath.IsAbs(p) {
				p = filepath.Join(cwd, p)
			}
			if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
				return false
			}
		}
	}
	return true
}

// Hint is what a blocked agent is told.
const Hint = "dendrite-first: a raw recursive search (rg, grep -r, the Grep tool) returns every match and costs many tokens. " +
	"Try, cheapest first:\n" +
	"  dendrite rag search \"<what you are looking for>\" -k 8   ranked, token-budgeted (MCP: dendrite_rag_retrieve with detail \"map\", then dendrite_rag_show)\n" +
	"  dendrite rag symbols <Name>                               definitions and references\n" +
	"  dendrite graph explain <symbol-or-file>                   callers, callees, structure (MCP: dendrite_graph_explain)\n" +
	"  dendrite rag history \"<query>\"                            past changes and their reasons\n" +
	"  dendrite rag grep '<regex>'                               exact pattern, indexed and capped\n" +
	"Searching named files (rg Foo path/to/file.go) is never stopped. " +
	"After one of the lookups above, run this same command again and it will go through; scope it to a path and cap its output."

// DeniedHint is what an agent is told when it repeats a stopped search without a lookup in between.
const DeniedHint = "dendrite-first: this search was stopped and no Dendrite lookup has run since. " +
	"Run one first (dendrite rag search \"<question>\" -k 8, dendrite rag symbols <Name>, dendrite graph explain <x>, or an MCP dendrite_rag_* tool), " +
	"then repeat this command and it will go through. " + AllowEnv + "=1 overrides."

func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// state is what the guard remembers: when each command was stopped and when each session last made
// a Dendrite lookup, both in Unix nanoseconds.
type state struct {
	Blocked   map[string]int64 `json:"blocked"`
	Retrieval map[string]int64 `json:"retrieval"`
}

// stamp reads a stored time: files written before lookups were tracked hold Unix seconds.
func stamp(v int64) time.Time {
	if v < 1e12 {
		return time.Unix(v, 0)
	}
	return time.Unix(0, v)
}

// loadState returns the entries seen within RetryWindow.
func (g *Guard) loadState(now time.Time) state {
	st := state{Blocked: map[string]int64{}, Retrieval: map[string]int64{}}
	if data, err := os.ReadFile(g.statePath()); err == nil {
		st = parseState(data)
	}
	st.prune(now)
	return st
}

// parseState reads either format of the state file.
func parseState(data []byte) state {
	st := state{Blocked: map[string]int64{}, Retrieval: map[string]int64{}}
	var cur state
	if json.Unmarshal(data, &cur) == nil && (cur.Blocked != nil || cur.Retrieval != nil) {
		maps.Copy(st.Blocked, cur.Blocked)
		maps.Copy(st.Retrieval, cur.Retrieval)
		return st
	}
	// The earlier format: a flat map of command hash to Unix seconds.
	_ = json.Unmarshal(data, &st.Blocked)
	return st
}

func (st state) prune(now time.Time) {
	for _, m := range []map[string]int64{st.Blocked, st.Retrieval} {
		for k, at := range m {
			if now.Sub(stamp(at)) > RetryWindow {
				delete(m, k)
			}
		}
	}
}

func (g *Guard) saveState(st state) {
	data, err := json.Marshal(st)
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
	// Denied counts retries refused because no Dendrite lookup came in between.
	Denied int `json:"denied"`
	// Scoped counts searches over named files, which are never stopped.
	Scoped int `json:"scoped"`
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
		s.count(ev)
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

func (s *Summary) count(ev Event) {
	switch {
	case ev.Decision == DecisionUsed:
		s.Dendrite++
	case ev.Decision == DecisionBlocked:
		s.Blocked[ev.Kind]++
	case ev.Decision == DecisionDenied:
		s.Denied++
	case ev.Decision == DecisionScoped:
		s.Scoped++
	case IsRawSearch(ev.Kind):
		s.RawSearches[ev.Kind]++
	}
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
