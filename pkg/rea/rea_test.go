package rea_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/rea"
)

type mockRunner struct {
	output []byte
	err    error
}

func (m *mockRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func TestEstimateTokens(t *testing.T) {
	if rea.EstimateTokens("") != 0 {
		t.Errorf("expected 0 tokens for empty string")
	}
	if rea.EstimateTokens("hi") != 1 {
		t.Errorf("expected 1 token for short string")
	}
	if rea.EstimateTokens("12345678") != 2 {
		t.Errorf("expected 2 tokens for 8 chars")
	}
}

func TestEvidenceBundle_Compaction(t *testing.T) {
	// Small bundle
	bundle := rea.NewEvidenceBundle(
		"/usr/bin/sample",
		rea.FormatMachO,
		"verify_license",
		"int verify_license() { return 1; }",
		"mov eax, 1\nret",
		[]string{"_main"},
		[]string{"LICENSE_OK"},
	)

	if bundle.Truncated {
		t.Errorf("expected small bundle not to be truncated")
	}
	if bundle.TokenEstimate <= 0 {
		t.Errorf("expected positive token estimate")
	}

	md := bundle.ToMarkdown()
	if !strings.Contains(md, "verify_license") || !strings.Contains(md, "LICENSE_OK") {
		t.Errorf("expected markdown to contain symbol and string constants")
	}

	// Large bundle that exceeds 4,000 token boundary
	largeDisasm := strings.Repeat("push rbp\nmov rbp, rsp\npop rbp\nret\n", 500)
	largePseudo := strings.Repeat("int x = 42;\ncall_func(x);\n", 1200)

	largeBundle := rea.NewEvidenceBundle(
		"/usr/bin/large",
		rea.FormatELF,
		"heavy_func",
		largePseudo,
		largeDisasm,
		nil,
		nil,
	)

	if !largeBundle.Truncated {
		t.Errorf("expected oversized bundle to be truncated")
	}
	if largeBundle.TokenEstimate > rea.MaxEvidenceTokens {
		t.Errorf("expected token estimate <= %d, got %d", rea.MaxEvidenceTokens, largeBundle.TokenEstimate)
	}

	// Nil bundle
	var nilBundle *rea.EvidenceBundle
	if nilBundle.ToMarkdown() != "" {
		t.Errorf("expected empty string for nil bundle ToMarkdown()")
	}
}

func TestRenderADR(t *testing.T) {
	doc := &rea.ADRDocument{
		ID:           "ADR-003",
		Title:        "Reverse Engineered License Cryptography",
		Status:       "proposed",
		PlaneEpic:    "PROJ-E-03",
		Repository:   "dendrite",
		Tags:         []string{"crypto", "rea", "hopper"},
		Context:      "Legacy shared library contained obfuscated signature checking.",
		Decision:     "Re-implement signature verification in clean Go using standard crypto/ed25519.",
		Consequences: "Removes native C binding and enables multi-platform compilation.",
	}

	bundle := rea.NewEvidenceBundle("/lib/liblegacy.dylib", rea.FormatMachO, "verify_signature", "void verify() {}", "", nil, nil)
	rendered := rea.RenderADR(doc, bundle)

	if !strings.HasPrefix(rendered, "---\n") {
		t.Errorf("expected ADR to start with YAML frontmatter")
	}
	if !strings.Contains(rendered, "id: ADR-003") {
		t.Errorf("expected ADR ID in frontmatter")
	}
	if !strings.Contains(rendered, "## 1. Context & Architectural Problem") {
		t.Errorf("expected Context section")
	}
	if !strings.Contains(rendered, "## 4. Reverse Engineering Evidence") {
		t.Errorf("expected Evidence section")
	}

	// Nil doc
	if rea.RenderADR(nil, bundle) != "" {
		t.Errorf("expected empty string for nil ADR document")
	}
}

func TestClient_AnalysisAndSecurityGuardrails(t *testing.T) {
	runner := &mockRunner{output: []byte("ok"), err: nil}
	localClient := rea.NewClient(rea.TargetLocalMac, rea.EngineHopper, runner)

	ctx := context.Background()

	// 1. Mach-O on local macOS succeeds
	reqMachO := rea.AnalysisRequest{
		BinaryPath: "/Applications/TargetApp.app/Contents/MacOS/TargetApp",
		Format:     rea.FormatMachO,
		Symbol:     "_decrypt_payload",
		Engine:     rea.EngineHopper,
	}
	bundle, err := localClient.Analyze(ctx, reqMachO)
	if err != nil {
		t.Fatalf("unexpected error analyzing Mach-O on macOS: %v", err)
	}
	if bundle.FunctionSymbol != "_decrypt_payload" {
		t.Errorf("expected symbol _decrypt_payload, got %s", bundle.FunctionSymbol)
	}

	// 2. Untrusted ELF on local macOS must be rejected by security guardrail
	reqELF := rea.AnalysisRequest{
		BinaryPath: "/tmp/untrusted_payload.elf",
		Format:     rea.FormatELF,
		Symbol:     "main",
		Engine:     rea.EngineGhidra,
	}
	_, errELF := localClient.Analyze(ctx, reqELF)
	if !errors.Is(errELF, rea.ErrUntrustedOnHost) {
		t.Errorf("expected ErrUntrustedOnHost when analyzing ELF on macOS, got: %v", errELF)
	}

	// 3. Remote Linux worker can analyze ELF safely
	remoteClient := rea.NewClient(rea.TargetRemoteLinux, rea.EngineGhidra, runner)
	bundleELF, errRemote := remoteClient.Analyze(ctx, reqELF)
	if errRemote != nil {
		t.Fatalf("unexpected error analyzing ELF on remote Linux worker: %v", errRemote)
	}
	if bundleELF.Format != rea.FormatELF {
		t.Errorf("expected ELF format, got %s", bundleELF.Format)
	}

	// 4. Empty binary path error
	reqEmpty := rea.AnalysisRequest{BinaryPath: ""}
	_, errEmpty := remoteClient.Analyze(ctx, reqEmpty)
	if !errors.Is(errEmpty, rea.ErrEmptyBinaryPath) {
		t.Errorf("expected ErrEmptyBinaryPath for empty binary path, got: %v", errEmpty)
	}

	// 5. DraftADR test
	doc, adrMarkdown := remoteClient.DraftADR(bundleELF, "ADR-010", "ELF Decryption Routine", "Context text", "Decision text", "Consequence text", nil)
	if doc.ID != "ADR-010" {
		t.Errorf("expected ADR-010, got %s", doc.ID)
	}
	if !strings.Contains(adrMarkdown, "ADR-010") {
		t.Errorf("expected markdown to contain ADR ID")
	}

	// 6. Test OSCommandRunner
	osRunner := &rea.OSCommandRunner{}
	_, _ = osRunner.Run(ctx, "echo", "test")
}
