package rea

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner abstracts shell execution for testing and local/container dispatch.
type CommandRunner interface {
	Run(ctx context.Context, command string, args ...string) ([]byte, error)
}

// OSCommandRunner executes commands via the operating system exec package.
type OSCommandRunner struct{}

func (r *OSCommandRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd.CombinedOutput()
}

// Client coordinates binary inspection across local macOS Hopper or remote Linux workers.
type Client struct {
	Target WorkerTarget
	Engine DisassemblerEngine
	Runner CommandRunner
}

// NewClient initializes a REA client.
func NewClient(target WorkerTarget, engine DisassemblerEngine, runner CommandRunner) *Client {
	if runner == nil {
		runner = &OSCommandRunner{}
	}
	return &Client{
		Target: target,
		Engine: engine,
		Runner: runner,
	}
}

// Analyze processes a target binary symbol and returns a bounded evidence bundle.
func (c *Client) Analyze(ctx context.Context, req AnalysisRequest) (*EvidenceBundle, error) {
	if strings.TrimSpace(req.BinaryPath) == "" {
		return nil, ErrEmptyBinaryPath
	}

	// Security guardrail: untrusted Linux ELF or Windows PE binaries must run in container
	if c.Target == TargetLocalMac && (req.Format == FormatELF || req.Format == FormatPE) {
		return nil, fmt.Errorf("%w: cannot execute %s analysis on local macOS host", ErrUntrustedOnHost, req.Format)
	}

	symbol := req.Symbol
	if symbol == "" {
		symbol = "main"
	}

	// Simulated / protocol command execution
	// In production, invokes `npx rea-agents mcp` or headless Ghidra CLI
	pseudocode := fmt.Sprintf("// Decompiled pseudocode for symbol: %s\nvoid %s(void) {\n    // Extracted logic\n}", symbol, symbol)
	disasm := fmt.Sprintf("; Disassembly of %s\n%s:\n    push rbp\n    mov rbp, rsp\n    pop rbp\n    ret", symbol, symbol)
	xrefs := []string{"_caller_entry_point", "_sub_401020"}
	strConstants := []string{"https://api.internal.endpoint", "AES_KEY_HEADER"}

	bundle := NewEvidenceBundle(req.BinaryPath, req.Format, symbol, pseudocode, disasm, xrefs, strConstants)
	return bundle, nil
}

// DraftADR produces an Architecture Decision Record (ADR) backed by reverse-engineering evidence.
func (c *Client) DraftADR(bundle *EvidenceBundle, id, title, ctxText, decision, consequences string, tags []string) (*ADRDocument, string) {
	if id == "" {
		id = "ADR-001"
	}
	if len(tags) == 0 {
		tags = []string{"reverse-engineering", "rea", string(c.Engine)}
	}

	doc := &ADRDocument{
		ID:           id,
		Title:        title,
		Status:       "proposed",
		Repository:   "dendrite",
		Tags:         tags,
		Context:      ctxText,
		Decision:     decision,
		Consequences: consequences,
	}

	renderedMarkdown := RenderADR(doc, bundle)
	return doc, renderedMarkdown
}
