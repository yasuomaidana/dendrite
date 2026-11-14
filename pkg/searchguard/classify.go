// Package searchguard keeps agents on Dendrite's retrieval tools. A recursive `rg` or `grep -r`
// returns every raw match and is the most token-expensive way to explore a repository; Dendrite's
// ranked, budgeted retrieval answers most questions in a fraction of the tokens. The guard sits in
// an agent's pre-tool hook: the first time a session attempts such a search it is stopped with the
// Dendrite equivalent, and repeating the same command lets it through (warn, then allow). Every
// decision is logged so `dendrite sensory usage` can show how well the discipline holds.
package searchguard

import (
	"path/filepath"
	"strings"
)

// Kinds of tool use the guard records.
const (
	KindRg          = "rg"
	KindGrep        = "grep"
	KindAg          = "ag"
	KindAck         = "ack"
	KindGrepTool    = "grep-tool"
	KindDendrite    = "dendrite"
	KindDendriteMCP = "dendrite-mcp"
)

// IsRawSearch reports whether kind is a raw text search rather than a Dendrite call.
func IsRawSearch(kind string) bool {
	switch kind {
	case KindRg, KindGrep, KindAg, KindAck, KindGrepTool:
		return true
	}
	return false
}

// ToolInput is the part of an agent's tool call the guard reads.
type ToolInput struct {
	Command string `json:"command"` // Bash
	Pattern string `json:"pattern"` // Grep
	Path    string `json:"path"`    // Grep
}

// Classify returns the raw-search kinds a tool call would run and whether it calls Dendrite.
// Searches over piped input (`dendrite ... | rg foo`) are filters, not repository searches.
func Classify(toolName string, in ToolInput) (searches []string, usesDendrite bool) {
	switch {
	case strings.HasPrefix(toolName, "mcp__dendrite"):
		return nil, true
	case toolName == "Grep":
		return []string{KindGrepTool}, false
	case toolName == "Bash":
		return classifyCommand(in.Command)
	}
	return nil, false
}

type segment struct {
	tokens []string
	// pipeIndex is the segment's position in its pipeline: 0 is the first command.
	pipeIndex int
}

func classifyCommand(command string) (searches []string, usesDendrite bool) {
	seen := map[string]bool{}
	for _, seg := range splitSegments(command) {
		head, args := commandHead(seg.tokens)
		if head == "" {
			continue
		}
		switch head {
		case "dendrite":
			usesDendrite = true
		case "rg", "ag", "ack":
			if !filtersStdin(seg, args) && !seen[head] {
				seen[head] = true
				searches = append(searches, head)
			}
		case "grep", "egrep", "fgrep", "ugrep", "ug":
			if isRecursiveGrep(args) && !filtersStdin(seg, args) && !seen[KindGrep] {
				seen[KindGrep] = true
				searches = append(searches, KindGrep)
			}
		}
	}
	return searches, usesDendrite
}

// commandHead returns the program a segment runs, skipping NAME=value assignments and wrappers
// such as sudo or time, and the arguments after it.
func commandHead(tokens []string) (string, []string) {
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if isAssignment(t) {
			i++
			continue
		}
		switch filepath.Base(t) {
		case "command", "sudo", "time", "nice", "env", "exec", "nohup":
			i++
			continue
		}
		return filepath.Base(t), tokens[i+1:]
	}
	return "", nil
}

func isAssignment(t string) bool {
	eq := strings.IndexByte(t, '=')
	if eq <= 0 {
		return false
	}
	for _, r := range t[:eq] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// valueFlags take the next argument as their value, so it is not a pattern or a path.
var valueFlags = map[string]bool{
	"-e": true, "-f": true, "-g": true, "-t": true, "-T": true, "-m": true, "-A": true, "-B": true,
	"-C": true, "-j": true, "-M": true, "-E": true, "-r": false, "--glob": true, "--type": true,
	"--type-not": true, "--max-count": true, "--regexp": true, "--file": true, "--iglob": true,
	"--after-context": true, "--before-context": true, "--context": true, "--threads": true,
	"--max-columns": true, "--replace": true, "--max-depth": true, "--sort": true, "--exclude": true,
	"--include": true, "--exclude-dir": true,
}

// positionals counts the arguments that are neither flags nor flag values.
func positionals(args []string) int {
	n := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			n += len(args) - i - 1
			return n
		case strings.HasPrefix(a, "--") && strings.Contains(a, "="):
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if valueFlags[a] {
				i++
			}
		default:
			n++
		}
	}
	return n
}

// filtersStdin reports whether a segment reads a pipe: it is not the first command of its pipeline
// and names at most a pattern (no path), so it narrows another command's output.
func filtersStdin(seg segment, args []string) bool {
	return seg.pipeIndex > 0 && positionals(args) <= 1
}

func isRecursiveGrep(args []string) bool {
	for i, a := range args {
		switch {
		case a == "--recursive" || a == "--dereference-recursive" || a == "--directories=recurse":
			return true
		case a == "-d" && i+1 < len(args) && args[i+1] == "recurse":
			return true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a[1:], "rR"):
			// A short-flag cluster such as -rn or -R. A pattern that merely starts with '-' is rare.
			return !strings.ContainsAny(a, "=/ ")
		}
	}
	return false
}

// splitSegments breaks a shell command line into simple commands at ;, &, && , || , | and newlines,
// honoring quotes. It is an approximation of a shell parser, enough to see which programs run.
func splitSegments(command string) []segment {
	var segs []segment
	var cur []string
	var tok strings.Builder
	inTok := false
	var quote rune
	pipeIndex := 0

	flushTok := func() {
		if inTok {
			cur = append(cur, tok.String())
			tok.Reset()
			inTok = false
		}
	}
	endSegment := func(nextPipe int) {
		flushTok()
		if len(cur) > 0 {
			segs = append(segs, segment{tokens: cur, pipeIndex: pipeIndex})
		}
		cur = nil
		pipeIndex = nextPipe
	}

	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && i+1 < len(runes) {
				i++
				tok.WriteRune(runes[i])
			} else {
				tok.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote, inTok = r, true
		case '\\':
			if i+1 < len(runes) {
				i++
				tok.WriteRune(runes[i])
				inTok = true
			}
		case ' ', '\t':
			flushTok()
		case '\n', ';':
			endSegment(0)
		case '&':
			if i+1 < len(runes) && runes[i+1] == '&' {
				i++
			}
			endSegment(0)
		case '|':
			if i+1 < len(runes) && runes[i+1] == '|' {
				i++
				endSegment(0)
			} else {
				endSegment(pipeIndex + 1)
			}
		default:
			tok.WriteRune(r)
			inTok = true
		}
	}
	endSegment(0)
	return segs
}
