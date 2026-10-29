package ticketlock

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/assert"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/llmchat"
)

// gateTimeout bounds the test run of the default gate.
const gateTimeout = 30 * time.Minute

// DefaultFinishChecks wires `finish` to the real world for the working copy in dir: the project's own test
// and coverage run as the gate, the local model behind the LiteLLM gateway as the acceptance reviewer, the
// branch's diff against develop, and the project's `[done] auto_done` directive (on when unset).
func DefaultFinishChecks(dir string) FinishChecks {
	return FinishChecks{Gate: toolchainGate, Review: proofReviewer(dir).Review, Diff: branchDiff, AutoDone: AutoDone(dir)}
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

// generatedPaths are left out of the patch the review reads: files Dendrite or a package manager writes, which say
// nothing about the work.
var generatedPaths = []string{
	":(exclude)*.lock", ":(exclude)go.sum", ":(exclude)package-lock.json", ":(exclude)pnpm-lock.yaml", ":(exclude)bun.lockb",
	":(exclude)docs/cli/**", ":(exclude)docs/mcp/**", ":(exclude).agents/**", ":(exclude).claude/**", ":(exclude).gemini/**",
}

// branchDiff is the work done for a ticket: the branch's changes against develop while they are not merged, and
// the ticket's own commits (their subject starts with [TICKET]) once they are, so finishing after the merge
// still shows the work.
func branchDiff(dir, ticket string) string {
	for _, base := range []string{"origin/develop", "develop"} {
		args := append([]string{"diff", base + "...HEAD", "--", "."}, generatedPaths...)
		if out, err := gitBytes(dir, args...); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return string(out)
		}
	}
	var b strings.Builder
	shas, _ := ticketCommits(dir, ticket)
	for _, sha := range shas {
		args := append([]string{"show", "--format=", "--patch", sha, "--", "."}, generatedPaths...)
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

// proofReviewer is the reviewer for the project at dir: the gateway's model, asked for a JSON object, and the
// project's sidecar to find the hunks that matter to each criterion.
func proofReviewer(dir string) ProofReviewer {
	chat := llmchat.Gateway(dir)
	chat.JSON = true
	return ProofReviewer{Chat: chat, Rank: sidecarRanker(dir)}
}

// AutoDone says whether a clean finish may set Done: the `[done] auto_done` directive of the project, on when
// the project has no directives or they cannot be read (a broken file must not silently hand Done to agents,
// so an unreadable file turns it off).
func AutoDone(dir string) bool {
	set, err := directive.Load(dir)
	return err == nil && set.Done.AutoDone
}
