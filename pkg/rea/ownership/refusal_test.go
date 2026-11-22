package ownership

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectRefusalReadsTheLastAssistantReply(t *testing.T) {
	for name, tc := range map[string]struct {
		file   string
		reason string
		found  bool
	}{
		"api refusal":            {"refusal_stop_reason.jsonl", "stop_reason", true},
		"declined in words":      {"refusal_declined.jsonl", "declined", true},
		"plain string content":   {"refusal_plain_content.jsonl", "stop_reason", true},
		"normal answer":          {"normal.jsonl", "", false},
		"declines but uses tool": {"working_with_tool.jsonl", "", false},
		"odd shapes":             {"odd_shapes.jsonl", "", false},
		"empty transcript":       {"empty.jsonl", "", false},
		"missing transcript":     {"does-not-exist.jsonl", "", false},
	} {
		got, ok := DetectRefusal(filepath.Join("testdata", tc.file))
		if ok != tc.found || got.Reason != tc.reason {
			t.Errorf("%s: got %+v, %t; want reason %q, found %t", name, got, ok, tc.reason, tc.found)
		}
	}
}

func TestDetectRefusalKeepsTheExcerptAndFoldsTheApostrophe(t *testing.T) {
	got, ok := DetectRefusal(filepath.Join("testdata", "refusal_declined.jsonl"))
	if !ok || !strings.Contains(got.Excerpt, "reverse engineering this binary") {
		t.Errorf("excerpt %+v, %t", got, ok)
	}
}

func TestDetectRefusalIgnoresALongAnswerThatStartsWithADecline(t *testing.T) {
	long := `{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"I can't help with the packed section, but ` + strings.Repeat("here is the analysis. ", 60) + `"}]}}` + "\n"
	path := filepath.Join(t.TempDir(), "long.jsonl")
	if err := os.WriteFile(path, []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := DetectRefusal(path); ok {
		t.Errorf("a long answer is work, not a refusal: %+v", got)
	}
}

func TestDetectRefusalTruncatesALongRefusalExcerpt(t *testing.T) {
	line := `{"type":"assistant","message":{"stop_reason":"refusal","content":[{"type":"text","text":"` + strings.Repeat("x", 900) + `"}]}}` + "\n"
	path := filepath.Join(t.TempDir(), "r.jsonl")
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := DetectRefusal(path)
	if !ok || !strings.HasSuffix(got.Excerpt, "...") || len(got.Excerpt) > 410 {
		t.Errorf("excerpt length %d, found %t", len(got.Excerpt), ok)
	}
}

func TestDetectRefusalReadsOnlyTheTailOfAHugeTranscript(t *testing.T) {
	pad := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("p", 1<<20) + `"}}` + "\n"
	refusal := `{"type":"assistant","message":{"stop_reason":"refusal","content":[{"type":"text","text":"No."}]}}` + "\n"
	path := filepath.Join(t.TempDir(), "huge.jsonl")
	if err := os.WriteFile(path, []byte(pad+pad+pad+refusal), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := DetectRefusal(path); !ok || got.Reason != "stop_reason" {
		t.Errorf("the refusal at the end of a 3 MB transcript: %+v, %t", got, ok)
	}
}

func TestDetectRefusalTreatsADirectoryAsUnreadable(t *testing.T) {
	if _, ok := DetectRefusal(t.TempDir()); ok {
		t.Error("a directory is not a transcript")
	}
}
