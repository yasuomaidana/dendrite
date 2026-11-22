// Package engram wraps the Engram agent-memory binary (https://github.com/Gentleman-Programming/engram).
// Engram is the agent-memory source of truth; rag-rs stays the code RAG. This package owns no memory logic:
// it locates the binary, reports its health, installs it, and fetches its own `context` output once so a
// headless job or a session can start with a bounded memory snapshot.
package engram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BinaryName is the Engram executable; BinaryEnv overrides where it is found.
const (
	BinaryName  = "engram"
	BinaryEnv   = "DENDRITE_ENGRAM_BIN"
	BrewFormula = "gentleman-programming/tap/engram"
)

// ErrMissing means no Engram executable was found.
var ErrMissing = errors.New("engram not found: run `dendrite soma install engram` (brew install " + BrewFormula + ") or set " + BinaryEnv)

// Locate returns the Engram executable: $DENDRITE_ENGRAM_BIN (which must work if set), then PATH.
func Locate() (string, error) {
	if p := os.Getenv(BinaryEnv); p != "" {
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("%s=%s: %w", BinaryEnv, p, err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s=%s: not an executable file", BinaryEnv, p)
		}
		return p, nil
	}
	if p, err := exec.LookPath(BinaryName); err == nil {
		return p, nil
	}
	return "", ErrMissing
}

// Version returns the output of `engram version`, for example "engram 3.7.2".
func Version(ctx context.Context, bin string) (string, error) {
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return "", fmt.Errorf("engram version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Health is the summary of `engram doctor --json`.
type Health struct {
	Status                               string `json:"status"`
	Total, OK, Warnings, Blocked, Errors int
}

// Doctor runs the read-only `engram doctor --json` and returns its summary. Engram exits non-zero when a
// check fails but still prints the report, so the report is read whatever the exit status.
func Doctor(ctx context.Context, bin string) (Health, error) {
	out, runErr := exec.CommandContext(ctx, bin, "doctor", "--json").Output()
	var raw struct {
		Status  string `json:"status"`
		Summary struct {
			Total    int `json:"total"`
			OK       int `json:"ok"`
			Warnings int `json:"warnings"`
			Blocked  int `json:"blocked"`
			Errors   int `json:"errors"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		if runErr != nil {
			return Health{}, fmt.Errorf("engram doctor: %w", runErr)
		}
		return Health{}, fmt.Errorf("engram doctor: unreadable report: %w", err)
	}
	s := raw.Summary
	return Health{Status: raw.Status, Total: s.Total, OK: s.OK, Warnings: s.Warnings, Blocked: s.Blocked, Errors: s.Errors}, nil
}
