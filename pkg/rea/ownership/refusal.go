// Package ownership decides who owns a reverse-engineering (REA) ticket: when a Claude session refuses the
// work, the ticket moves to Antigravity (AGY), as the project's `[rea]` directive says.
package ownership

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// maxTranscriptTail bounds how much of a Claude Code transcript is read: the end of it holds the last turn.
const maxTranscriptTail = 2 << 20

// maxDeclineLength is the longest reply taken for a plain refusal; a long answer that happens to start with
// "I can't" is work with a caveat, not a refusal.
const maxDeclineLength = 600

// declinePhrases open a reply that refuses the task. Matched on the lower-cased start of the reply, with the
// typographic apostrophe folded to a plain one.
var declinePhrases = []string{
	"i can't help with", "i cannot help with", "i can't assist", "i cannot assist",
	"i'm not able to help", "i am not able to help", "i'm unable to help", "i am unable to help",
	"i won't be able to help", "i must decline", "i'm going to decline", "i can't continue with this",
	"i can't do that", "i cannot do that", "sorry, but i can't", "sorry, but i cannot",
}

// Refusal is a Claude reply that declined the task.
type Refusal struct {
	// Reason is "stop_reason" when the API ended the turn with a refusal, or "declined" when the reply says so.
	Reason string `json:"reason"`
	// Excerpt is the start of the reply, kept on the ticket as evidence.
	Excerpt string `json:"excerpt"`
}

// transcriptLine is the part of one Claude Code transcript line (JSONL) this package reads.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

// contentBlock is one block of an assistant message.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// DetectRefusal reads the end of the transcript at path and reports whether the last assistant reply refused
// the task. An unreadable or empty transcript is not a refusal: a hook must never act on what it cannot judge.
func DetectRefusal(path string) (Refusal, bool) {
	tail, err := readTail(path, maxTranscriptTail)
	if err != nil {
		return Refusal{}, false
	}
	last, ok := lastAssistant(tail)
	if !ok {
		return Refusal{}, false
	}
	text, usedTool := replyText(last.Message.Content)
	if last.Message.StopReason == "refusal" {
		return Refusal{Reason: "stop_reason", Excerpt: excerpt(text)}, true
	}
	if usedTool || len(text) > maxDeclineLength || !declines(text) {
		return Refusal{}, false
	}
	return Refusal{Reason: "declined", Excerpt: excerpt(text)}, true
}

// readTail returns at most n bytes from the end of the file, starting at a line boundary.
func readTail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// A file shorter than n cannot seek that far back: read it whole. A failed read is reported by ReadAll.
	_, seekErr := f.Seek(-n, io.SeekEnd)
	cut := seekErr == nil
	if !cut {
		_, _ = f.Seek(0, io.SeekStart)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if cut { // the first line is probably cut in half
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, nil
}

// lastAssistant returns the last assistant line of a JSONL transcript.
func lastAssistant(data []byte) (transcriptLine, bool) {
	var last transcriptLine
	found := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), maxTranscriptTail)
	for sc.Scan() {
		var line transcriptLine
		if json.Unmarshal(sc.Bytes(), &line) == nil && line.Type == "assistant" {
			last, found = line, true
		}
	}
	return last, found
}

// replyText joins the text blocks of a reply and reports whether it also called a tool, which means the
// session is working, not refusing. Content may be a plain string or a list of blocks.
func replyText(raw json.RawMessage) (text string, usedTool bool) {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain), false
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "tool_use":
			usedTool = true
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), usedTool
}

func declines(text string) bool {
	lower := strings.ToLower(strings.ReplaceAll(text, "’", "'"))
	for _, p := range declinePhrases {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func excerpt(text string) string {
	const max = 400
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
