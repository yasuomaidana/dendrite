package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"Dendrite/pkg/fsutil"
)

// noticeWindow is how long `session resume` keeps reporting a mode change.
const noticeWindow = 14 * 24 * time.Hour

// ChecksPath is where the weekly check reports accumulate (one JSON line each).
func ChecksPath(home string) string { return filepath.Join(home, "memory", "checks.jsonl") }

// AppendCheck records a report.
func AppendCheck(home string, r Report) error {
	line, _ := json.Marshal(r)
	return fsutil.AppendLine(ChecksPath(home), line)
}

// MinGap is how much older than a check the report it is compared with must be: a mode change needs the same
// result in two weekly checks, so a check run again the same day agrees with nothing.
const MinGap = 6 * 24 * time.Hour

// LastCheckBefore is the newest report on record made before the given time.
func LastCheckBefore(home string, before time.Time) (Report, bool) {
	data, err := os.ReadFile(ChecksPath(home))
	if err != nil {
		return Report{}, false
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var r Report
		if json.Unmarshal(lines[i], &r) == nil && !r.Time.IsZero() && r.Time.Before(before) {
			return r, true
		}
	}
	return Report{}, false
}

// Decide is the computed mode after a check, given the report from at least MinGap earlier (nil when there is
// none). A mode change needs the same candidate in two checks a week apart, so one odd week moves nothing; an Engram that is missing or unhealthy switches it off at once. A hand-set
// override is never touched (it wins when the mode is resolved).
func Decide(s State, last *Report, r Report, now time.Time) State {
	next, reason := s.Computed, s.Reason
	switch {
	case !r.EngramHealthy:
		next, reason = ClaudeOnly, r.Note
	case last != nil && last.EngramHealthy && last.Candidate == r.Candidate:
		next, reason = r.Candidate, Summary(r)
	}
	if next != s.Computed {
		s.Since = now.UTC()
	}
	s.Computed, s.Reason = next, reason
	return s
}

// ModeNotice tells a session that the mode was switched by measurement; "" when there is nothing to say.
func ModeNotice(s State, now time.Time) string {
	if s.Override != "" || s.Computed == "" || s.Since.IsZero() || now.Sub(s.Since) > noticeWindow {
		return ""
	}
	return fmt.Sprintf("[i] Memory mode is %s since %s: %s. `dendrite memory mode set auto|both|engram|claude|off` overrides it.", s.Computed, s.Since.Format("2006-01-02"), s.Reason)
}
