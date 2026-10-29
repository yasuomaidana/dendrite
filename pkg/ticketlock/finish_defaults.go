package ticketlock

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/assert"
	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/rag"
)

// DirectivesPath is the tracked file that holds a project's quality directives.
const DirectivesPath = ".github/dendrite/directives.toml"

// gateTimeout bounds the test run of the default gate.
const gateTimeout = 30 * time.Minute

// DefaultFinishChecks wires `finish` to the real world for the working copy in dir: the project's own test
// and coverage run as the gate, the local model behind the LiteLLM gateway as the acceptance reviewer, the
// branch's diff against develop, and the project's `[done] auto_done` directive (on when unset).
func DefaultFinishChecks(dir string) FinishChecks {
	return FinishChecks{Gate: toolchainGate, Review: gatewayReviewer(dir).Review, Diff: branchDiff, AutoDone: AutoDone(dir)}
}

// toolchainGate runs the tests of the detected toolchain. It is the floor of the quality gate: the directive
// gates (production lines, duplication, coverage floor) join it as they land.
func toolchainGate(ctx context.Context, dir string) (GateResult, error) {
	adapter, err := assert.DetectToolchain(dir)
	if err != nil {
		return GateResult{}, err
	}
	argv := adapter.TestCommand("", nil)
	for _, a := range argv { // the profile's directory (.dendrite/cov) does not exist in a fresh checkout
		if path, ok := strings.CutPrefix(a, "-coverprofile="); ok {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, gateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = cleanEnv(os.Environ())
	out, runErr := cmd.CombinedOutput()
	return GateResult{Passed: runErr == nil, Report: gateReport(argv, string(out), runErr == nil)}, nil
}

// failureLine matches what test runners print for a failure, whatever the language.
var failureLine = regexp.MustCompile(`(?i)^(FAIL\b|--- FAIL|panic:|error[:\[]|FAILED)|DATA RACE|\bFAILED\b`)

// maxFailureLines bounds the failures a report lists.
const maxFailureLines = 30

// gateReport describes a gate run. A passing run is the command and the last lines. A failing run lists the
// failing lines first, because the last lines of a long run are usually the packages that passed.
func gateReport(argv []string, output string, passed bool) string {
	head := "`" + strings.Join(argv, " ") + "`\n"
	if passed {
		return head + tail(output, 40)
	}
	var failures []string
	for _, line := range strings.Split(output, "\n") {
		if failureLine.MatchString(strings.TrimSpace(line)) && len(failures) < maxFailureLines {
			failures = append(failures, line)
		}
	}
	if len(failures) == 0 {
		return head + tail(output, 40)
	}
	return head + "Failures:\n" + strings.Join(failures, "\n") + "\n\nEnd of the output:\n" + tail(output, 15)
}

// cleanEnvPrefixes are the variables a test run must not inherit: the CLI loads them from the user's .env, and a
// test that sees them reaches the live Plane, gateway and Dendrite state instead of its own fixtures.
var cleanEnvPrefixes = []string{"PLANE_", "DENDRITE_", "LITELLM_"}

// cleanEnv returns environ without the variables that point at the user's live services.
func cleanEnv(environ []string) []string {
	kept := make([]string, 0, len(environ))
next:
	for _, kv := range environ {
		for _, p := range cleanEnvPrefixes {
			if strings.HasPrefix(kv, p) {
				continue next
			}
		}
		kept = append(kept, kv)
	}
	return kept
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

// diffExcludes leave out of a review diff what is large and says nothing about the work.
var diffExcludes = []string{":(exclude)*.lock", ":(exclude)go.sum", ":(exclude)package-lock.json", ":(exclude)pnpm-lock.yaml", ":(exclude)bun.lockb"}

// branchDiff is the work done for a ticket: the branch's changes against develop while they are not merged, and
// the ticket's own commits (their subject starts with [TICKET]) once they are, so finishing after the merge
// still shows the work.
func branchDiff(dir, ticket string) string {
	for _, base := range []string{"origin/develop", "develop"} {
		args := append([]string{"diff", base + "...HEAD", "--", "."}, diffExcludes...)
		if out, err := gitBytes(dir, args...); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return string(out)
		}
	}
	var b strings.Builder
	shas, _ := ticketCommits(dir, ticket)
	for _, sha := range shas {
		args := append([]string{"show", "--format=", "--patch", sha, "--", "."}, diffExcludes...)
		if out, err := gitBytes(dir, args...); err == nil {
			b.Write(out)
		}
	}
	return b.String()
}

// ticketCommits lists, oldest first, the commits whose subject starts with [TICKET]: their hashes and
// "short-hash subject" lines.
func ticketCommits(dir, ticket string) (shas, lines []string) {
	out, err := gitBytes(dir, "log", "--reverse", "--no-merges", "--format=%H %s", "--fixed-strings", "--grep=["+ticket+"]")
	if err != nil {
		return nil, nil
	}
	for _, l := range strings.Split(string(out), "\n") {
		sha, subject, ok := strings.Cut(strings.TrimSpace(l), " ")
		if ok && strings.HasPrefix(subject, "["+ticket+"]") {
			shas, lines = append(shas, sha), append(lines, sha[:min(7, len(sha))]+" "+subject)
		}
	}
	return shas, lines
}

// preferLocalModel is a seam for the choice of the review model.
var preferLocalModel = rag.PreferLocalModel

func gatewayReviewer(dir string) GatewayReviewer {
	base := strings.TrimSuffix(strings.TrimRight(firstNonEmpty(os.Getenv("LITELLM_PROXY_URL"), "http://localhost:4000"), "/"), "/v1")
	key := dotenv.Lookup(dir, "LITELLM_MASTER_KEY")
	model := rag.DefaultLocalModel
	if choice, ok := preferLocalModel(context.Background(), key); ok {
		model = choice.Model
	}
	return GatewayReviewer{URL: base + "/v1/chat/completions", Key: key, Model: model, HTTP: nil}
}

// AutoDone reads `auto_done` from the `[done]` table of the project's directives; Done is automatic unless
// the file says `auto_done = false`.
func AutoDone(dir string) bool {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(DirectivesPath)))
	if err != nil {
		return true
	}
	defer f.Close()
	inDone := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "["):
			inDone = line == "[done]"
		case inDone && strings.HasPrefix(line, "auto_done"):
			_, value, _ := strings.Cut(line, "=")
			return strings.TrimSpace(strings.SplitN(value, "#", 2)[0]) != "false"
		}
	}
	return true
}
