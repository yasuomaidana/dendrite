package memory

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/engram"
)

// ClaudeMemory is one file of Claude Code's own file memory.
type ClaudeMemory struct {
	Name, Description, Type, Body string
	// File is the memory's file name (as MEMORY.md links it) and Modified when it last changed.
	File     string
	Modified time.Time
}

// maxMirrorBody is the longest body mirrored into Engram.
const maxMirrorBody = 4000

var notAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// ClaudeDir is the memory folder Claude Code keeps for the repository at repo, under the user's home:
// ~/.claude/projects/<repo path with every non-alphanumeric character replaced by "-">/memory.
func ClaudeDir(userHome, repo string) string {
	return filepath.Join(userHome, ".claude", "projects", notAlnum.ReplaceAllString(repo, "-"), "memory")
}

// ReadClaude reads the memory files of dir (the .md files except the MEMORY.md index). A file with
// frontmatter gives its name, description and type; one without is a note named after the file.
func ReadClaude(dir string) ([]ClaudeMemory, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []ClaudeMemory
	for _, e := range entries {
		name := e.Name()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if !strings.HasSuffix(name, ".md") || name == "MEMORY.md" || err != nil {
			continue
		}
		m := parseClaude(strings.TrimSuffix(name, ".md"), string(data))
		m.File = name
		if info, err := e.Info(); err == nil {
			m.Modified = info.ModTime()
		}
		out = append(out, m)
	}
	return out, nil
}

// parseClaude splits a memory file into its frontmatter fields and body.
func parseClaude(fallbackName, text string) ClaudeMemory {
	m := ClaudeMemory{Name: fallbackName, Type: "note", Body: text}
	rest, ok := strings.CutPrefix(text, "---\n")
	front, body, found := strings.Cut(rest, "\n---")
	if !ok || !found {
		return m
	}
	m.Body = strings.TrimLeft(body, "-\n")
	for _, line := range strings.Split(front, "\n") {
		key, val, _ := strings.Cut(strings.TrimSpace(line), ":")
		val = strings.TrimSpace(val)
		switch {
		case key == "name" && val != "":
			m.Name = val
		case key == "description":
			m.Description = val
		case key == "type" && val != "":
			m.Type = val
		}
	}
	return m
}

// MirrorClaude copies every Claude file memory of dir into Engram, one `engram save` each, and returns how
// many it saved. It is idempotent (Engram dedupes an identical memory) and never changes the files. With
// Engram off by the mode, or not installed, it does nothing.
func MirrorClaude(ctx context.Context, home, dir string) (int, error) {
	if !LoadState(home).Resolve().Engram() {
		return 0, nil
	}
	return Mirror(ctx, dir)
}

// Mirror is MirrorClaude whatever the mode is: the weekly check mirrors so that Engram is measured on the same
// facts even while it is switched off, which is how it can switch back on. It returns how many memories it
// saved: those Engram already holds with the same title and content are skipped (Engram's own duplicate guard
// only covers a short window, so a weekly mirror would otherwise add every memory again).
func Mirror(ctx context.Context, dir string) (int, error) {
	bin, err := engram.Locate()
	if err != nil {
		return 0, nil
	}
	memories, err := ReadClaude(dir)
	if err != nil {
		return 0, err
	}
	existing, err := engram.Export(ctx, bin, "")
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, o := range existing {
		have[o.Title+"\x00"+o.Content] = true
	}
	saved := 0
	for _, m := range memories {
		body := Clip(strings.TrimSpace(m.Description+"\n\n"+m.Body), maxMirrorBody)
		if have[m.Name+"\x00"+body] {
			continue
		}
		if err := engram.Save(ctx, bin, "", m.Type, m.Name, body); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}
