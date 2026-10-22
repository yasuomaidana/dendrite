package agentjobs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/dotenv"
)

// AGY model families. Quota is tracked per family: a Gemini 3.8 quota hit does not block Argon.
const (
	FamilyGemini38 = "gemini-3.8"
	FamilyArgon    = "gemini-4-argon"
	FamilyClaude   = "claude"
	FamilyGPTOSS   = "gpt-oss"
)

// AutoModel asks the dispatcher to pick the AGY model from complexity and current load.
const AutoModel = "auto"

// DefaultResumeTokenLimit is the input size above which a transient failure starts a fresh
// conversation instead of resuming: resuming an oversized conversation re-sends the whole context.
const DefaultResumeTokenLimit = 400_000

// agyModelsTTL is how long the cached `agy models` listing stays authoritative.
const agyModelsTTL = 24 * time.Hour

// defaultAGYModels are the models Dendrite schedules on AGY unless DENDRITE_AGY_MODELS overrides them.
// The Claude and GPT-OSS models AGY also offers are opt-in through that variable.
var defaultAGYModels = []string{
	"gemini-3.8-flash-low", "gemini-3.8-flash-medium", "gemini-3.8-flash-high",
	"gemini-4-argon-low", "gemini-4-argon-medium", "gemini-4-argon-high",
}

// counterparts maps a model to the comparable model of the other Gemini family.
var counterparts = map[string]string{
	"gemini-3.8-flash-low":    "gemini-4-argon-low",
	"gemini-3.8-flash-medium": "gemini-4-argon-low",
	"gemini-3.8-flash-high":   "gemini-4-argon-medium",
	"gemini-4-argon-low":      "gemini-3.8-flash-medium",
	"gemini-4-argon-medium":   "gemini-3.8-flash-high",
	"gemini-4-argon-high":     "gemini-3.8-flash-high",
}

// complexityBands lists the preferred models per complexity band, most preferred first.
var complexityBands = []struct {
	MaxComplexity int
	Models        []string
}{
	{3, []string{"gemini-3.8-flash-medium", "gemini-4-argon-low"}},
	{6, []string{"gemini-3.8-flash-high", "gemini-4-argon-medium"}},
	{1 << 30, []string{"gemini-4-argon-high", "gemini-3.8-flash-high"}},
}

// AGYFamily returns the quota family of an AGY model id, or "" when it is not recognised.
func AGYFamily(model string) string {
	switch {
	case strings.HasPrefix(model, "gemini-3.8-"):
		return FamilyGemini38
	case strings.HasPrefix(model, "gemini-4-argon"):
		return FamilyArgon
	case strings.HasPrefix(model, "claude-"):
		return FamilyClaude
	case strings.HasPrefix(model, "gpt-oss"):
		return FamilyGPTOSS
	}
	return ""
}

// AllowedAGYModels returns the AGY models Dendrite may schedule: DENDRITE_AGY_MODELS (comma separated)
// or the Gemini 3.8 and Argon defaults.
func AllowedAGYModels() []string {
	if env := strings.TrimSpace(os.Getenv("DENDRITE_AGY_MODELS")); env != "" {
		var out []string
		for _, m := range strings.Split(env, ",") {
			if m = strings.TrimSpace(m); m != "" {
				out = append(out, m)
			}
		}
		return out
	}
	return append([]string(nil), defaultAGYModels...)
}

// IsAGYModel reports whether model is in the allowed AGY catalog.
func IsAGYModel(model string) bool {
	for _, m := range AllowedAGYModels() {
		if m == model {
			return true
		}
	}
	return false
}

// CounterpartModel returns the other family's comparable model, if it is allowed.
func CounterpartModel(model string) string {
	if alt, ok := counterparts[model]; ok && IsAGYModel(alt) {
		return alt
	}
	return ""
}

// providerKey is the quota bucket of an agent/model pair: one per AGY family, copilot, local, oxillama.
func providerKey(agent, model string) string {
	switch agent {
	case "aider":
		return providerCopilot
	case "local":
		return providerLocal
	case "oxillama":
		return providerOxillama
	}
	if fam := AGYFamily(model); fam != "" {
		return providerAGY + ":" + fam
	}
	return providerAGY
}

// coolingDown reports whether key is exhausted at now. A legacy bare "agy" entry covers every AGY family.
func coolingDown(exhausted map[string]time.Time, key string, now time.Time) bool {
	if until, ok := exhausted[key]; ok && until.After(now) {
		return true
	}
	if strings.HasPrefix(key, providerAGY+":") {
		if until, ok := exhausted[providerAGY]; ok && until.After(now) {
			return true
		}
	}
	return false
}

// activeStatuses are the job states that count as load on a model.
var activeStatuses = map[Status]bool{
	StatusQueued: true, StatusRunning: true, StatusFailedRetryable: true, StatusEscalated: true,
}

// defaultSharedDir is $DENDRITE_SHARED_STATE_DIR or ~/.dendrite/agentjobs. Test binaries get ""
// (the ledger root), so tests never touch the user's shared state.
func defaultSharedDir() string {
	if dir := os.Getenv("DENDRITE_SHARED_STATE_DIR"); dir != "" {
		return dir
	}
	if testing.Testing() {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dendrite", "agentjobs")
}

// sharedDir is where machine-wide job state lives.
func (d *Dispatcher) sharedDir() string {
	if d.SharedDir != "" {
		return d.SharedDir
	}
	return d.Ledger.Root()
}

func (d *Dispatcher) ledgersPath() string {
	return filepath.Join(d.sharedDir(), "_ledgers.json")
}

// registeredLedgers lists every ledger root that has submitted a job on this machine.
func (d *Dispatcher) registeredLedgers() []string {
	var roots []string
	if data, err := os.ReadFile(d.ledgersPath()); err == nil {
		_ = json.Unmarshal(data, &roots)
	}
	return roots
}

// registerLedger records this dispatcher's ledger so other repositories count its jobs as load.
func (d *Dispatcher) registerLedger() {
	root, err := filepath.Abs(d.Ledger.Root())
	if err != nil {
		return
	}
	roots := d.registeredLedgers()
	for _, r := range roots {
		if r == root {
			return
		}
	}
	data, _ := json.MarshalIndent(append(roots, root), "", "  ")
	if os.MkdirAll(d.sharedDir(), 0o755) == nil {
		_ = writeFileAtomic(d.ledgersPath(), data)
	}
}

// modelLoad counts active AGY jobs per family across every registered ledger, so jobs submitted
// from different repositories (synapse peers) spread across Gemini 3.8 and Argon too.
func (d *Dispatcher) modelLoad() map[string]int {
	load := map[string]int{}
	for _, root := range d.ledgerRoots() {
		jobs, err := NewLedger(root).List()
		if err != nil {
			continue
		}
		for _, j := range jobs {
			if (j.Agent == "agy" || j.Agent == "") && activeStatuses[j.Status] {
				load[AGYFamily(j.Model)]++
			}
		}
	}
	return load
}

// PickAGYModel chooses an AGY model for a job of the given complexity (0 = unknown, treated as medium).
// It takes the band's models in preference order, skips cooling-down families, and picks the family with
// the fewest active jobs, so concurrent jobs spread across Gemini 3.8 and Argon. The reason explains the choice.
func (d *Dispatcher) PickAGYModel(complexity int) (string, string) {
	if complexity <= 0 {
		complexity = 5
	}
	var band []string
	for _, b := range complexityBands {
		if complexity <= b.MaxComplexity {
			band = b.Models
			break
		}
	}
	exhausted := d.loadExhaustedProviders()
	load := d.modelLoad()
	now := d.now()

	best, bestLoad := "", -1
	for _, m := range band {
		if !IsAGYModel(m) || coolingDown(exhausted, providerKey("agy", m), now) {
			continue
		}
		if l := load[AGYFamily(m)]; best == "" || l < bestLoad {
			best, bestLoad = m, l
		}
	}
	if best != "" {
		return best, fmt.Sprintf("complexity %d band; %s family has %d active jobs", complexity, AGYFamily(best), bestLoad)
	}
	// Every preferred family is cooling down or filtered out: fall back to the first allowed model.
	for _, m := range AllowedAGYModels() {
		if !coolingDown(exhausted, providerKey("agy", m), now) {
			return m, "preferred families cooling down; first allowed AGY model"
		}
	}
	if len(band) > 0 {
		return band[0], "all AGY families cooling down; the quota ladder will escalate"
	}
	return defaultAGYModels[2], "default"
}

// agyModelsCache is the on-disk form of the last `agy models` listing.
type agyModelsCache struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []string  `json:"models"`
}

func (d *Dispatcher) agyModelsPath() string {
	return filepath.Join(d.Ledger.Root(), "_agy_models.json")
}

var agyModelLine = regexp.MustCompile(`^([a-z0-9][a-z0-9.\-]+)\t`)

// RefreshAGYModels runs `agy models` and caches the model ids it lists.
func (d *Dispatcher) RefreshAGYModels(ctx context.Context) ([]string, error) {
	bin := d.AgyPath
	if bin == "" {
		bin = "agy"
	}
	out, err := exec.CommandContext(ctx, bin, "models").Output()
	if err != nil {
		return nil, fmt.Errorf("agentjobs: agy models failed: %w", err)
	}
	var models []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if m := agyModelLine.FindStringSubmatch(sc.Text()); m != nil {
			models = append(models, m[1])
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("agentjobs: agy models listed no models")
	}
	sort.Strings(models)
	data, _ := json.MarshalIndent(agyModelsCache{FetchedAt: d.now(), Models: models}, "", "  ")
	if err := os.MkdirAll(d.Ledger.Root(), 0o755); err != nil {
		return models, err
	}
	return models, writeFileAtomic(d.agyModelsPath(), data)
}

// cachedAGYModels returns the cached `agy models` listing while it is fresh, or nil.
func (d *Dispatcher) cachedAGYModels() []string {
	data, err := os.ReadFile(d.agyModelsPath())
	if err != nil {
		return nil
	}
	var c agyModelsCache
	if json.Unmarshal(data, &c) != nil || d.now().Sub(c.FetchedAt) > agyModelsTTL {
		return nil
	}
	return c.Models
}

// validateAGYModel rejects a model AGY would refuse, before a job is spawned for it.
func (d *Dispatcher) validateAGYModel(model string) error {
	if !IsAGYModel(model) {
		return fmt.Errorf("agentjobs: %q is not an allowed AGY model (allowed: %s; set DENDRITE_AGY_MODELS to change)",
			model, strings.Join(AllowedAGYModels(), ", "))
	}
	if live := d.cachedAGYModels(); live != nil {
		for _, m := range live {
			if m == model {
				return nil
			}
		}
		return fmt.Errorf("agentjobs: AGY does not offer %q (from the cached `agy models` listing)", model)
	}
	return nil
}

// ProviderUnavailable marks output from a provider that cannot serve the job (for example a rejected
// LiteLLM key). The classifier treats it like quota, so the provider cools down and the job escalates.
const ProviderUnavailable = "DENDRITE_PROVIDER_UNAVAILABLE"

// servedStatuses are job states in which the agent completed a turn.
var servedStatuses = map[Status]bool{
	StatusSucceeded: true, StatusVerified: true, StatusCommitted: true, StatusNeedsReview: true, StatusEarlyExit: true,
}

// ledgerRoots is this dispatcher's ledger plus every other registered one.
func (d *Dispatcher) ledgerRoots() []string {
	roots := []string{d.Ledger.Root()}
	if own, err := filepath.Abs(d.Ledger.Root()); err == nil {
		for _, r := range d.registeredLedgers() {
			if r != own {
				roots = append(roots, r)
			}
		}
	}
	return roots
}

// familyServedSince reports whether another job on the same quota bucket finished a turn after since.
func (d *Dispatcher) familyServedSince(job *Job, since time.Time) bool {
	key := providerKey(job.Agent, job.Model)
	for _, root := range d.ledgerRoots() {
		jobs, err := NewLedger(root).List()
		if err != nil {
			continue
		}
		for _, j := range jobs {
			if j.ID != job.ID && servedStatuses[j.Status] && j.UpdatedAt.After(since) && providerKey(j.Agent, j.Model) == key {
				return true
			}
		}
	}
	return false
}

// liteLLMKey resolves LITELLM_MASTER_KEY for a job: exported, then the job worktree's own
// .env, then the global $DENDRITE_HOME/.env (detached runners do not inherit a shell).
func (d *Dispatcher) liteLLMKey(worktree string) string {
	if worktree == "" {
		worktree = "."
	}
	return dotenv.Lookup(worktree, "LITELLM_MASTER_KEY")
}

// resumeTokenLimit honours DENDRITE_AGY_RESUME_TOKEN_LIMIT.
func resumeTokenLimit() int {
	var n int
	if _, err := fmt.Sscanf(os.Getenv("DENDRITE_AGY_RESUME_TOKEN_LIMIT"), "%d", &n); err == nil && n > 0 {
		return n
	}
	return DefaultResumeTokenLimit
}

// inputTokens reads input_tokens from an AGY usage map.
func inputTokens(usage map[string]any) int {
	switch v := usage["input_tokens"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}
