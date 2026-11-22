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
	sc := scan(toolName, in)
	return sc.searches, sc.usesDendrite
}

// scanResult is everything the guard reads from one tool call.
type scanResult struct {
	searches     []string
	usesDendrite bool
	// retrieval is set for a Dendrite lookup (rag, ask, brief, graph), which earns a raw-search retry.
	retrieval bool
	// targets holds the paths each raw search names; an empty list means the working directory.
	targets [][]string
}

func scan(toolName string, in ToolInput) scanResult {
	switch {
	case strings.HasPrefix(toolName, "mcp__dendrite"):
		return scanResult{usesDendrite: true, retrieval: isRetrievalMCP(toolName)}
	case toolName == "Grep":
		var t []string
		if in.Path != "" {
			t = []string{in.Path}
		}
		return scanResult{searches: []string{KindGrepTool}, targets: [][]string{t}}
	case toolName == "Bash":
		return scanCommand(in.Command)
	}
	return scanResult{}
}

// retrievalMCP names the MCP tools that look something up in the index.
var retrievalMCP = []string{"rag_", "graph_", "query_context", "synapse_search"}

func isRetrievalMCP(tool string) bool {
	for _, k := range retrievalMCP {
		if strings.Contains(tool, k) {
			return true
		}
	}
	return false
}

// retrievalCommands are the `dendrite` subcommands that retrieve context.
var retrievalCommands = map[string]bool{
	"rag": true, "ask": true, "q": true, "query-project": true, "rag-ask": true, "brief": true, "graph": true,
}

// firstArg returns the first argument that is not a flag.
func firstArg(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

type segment struct {
	tokens []string
	// pipeIndex is the segment's position in its pipeline: 0 is the first command.
	pipeIndex int
}

func scanCommand(command string) scanResult {
	var sc scanResult
	seen := map[string]bool{}
	for _, seg := range splitSegments(command) {
		head, args := commandHead(seg.tokens)
		if head == "" {
			continue
		}
		switch head {
		case "dendrite":
			sc.usesDendrite = true
			if retrievalCommands[firstArg(args)] {
				sc.retrieval = true
			}
		case "rg", "ag", "ack":
			if !filtersStdin(seg, args) {
				sc.addSearch(seen, head, args)
			}
		case "grep", "egrep", "fgrep", "ugrep", "ug":
			if isRecursiveGrep(args) && !filtersStdin(seg, args) {
				sc.addSearch(seen, KindGrep, args)
			}
		}
	}
	return sc
}

func (sc *scanResult) addSearch(seen map[string]bool, kind string, args []string) {
	sc.targets = append(sc.targets, searchPaths(args))
	if !seen[kind] {
		seen[kind] = true
		sc.searches = append(sc.searches, kind)
	}
}

// searchPaths returns the paths a search names: the positionals after the pattern, or all of them
// when the pattern comes from -e, -f or --regexp.
func searchPaths(args []string) []string {
	pos := positionalArgs(args)
	for _, a := range args {
		if a == "-e" || a == "-f" || a == "--regexp" || a == "--file" || strings.HasPrefix(a, "--regexp=") {
			return pos
		}
	}
	if len(pos) == 0 {
		return nil
	}
	return pos[1:]
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

// positionalArgs returns the arguments that are neither flags nor flag values.
func positionalArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(out, args[i+1:]...)
		case strings.HasPrefix(a, "--") && strings.Contains(a, "="):
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if valueFlags[a] {
				i++
			}
		default:
			out = append(out, a)
		}
	}
	return out
}

func positionals(args []string) int { return len(positionalArgs(args)) }

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
