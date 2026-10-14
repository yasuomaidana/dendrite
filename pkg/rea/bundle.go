package rea

import (
	"fmt"
	"strings"
	"time"
)

const (
	// MaxEvidenceTokens enforces the < 4,000 token context boundary from FEAT-003.
	MaxEvidenceTokens = 4000
	// CharactersPerToken heuristic for English / code text.
	CharactersPerToken = 4
	MaxEvidenceChars   = MaxEvidenceTokens * CharactersPerToken
)

// EstimateTokens calculates an approximate token count from character length.
func EstimateTokens(text string) int {
	chars := len(strings.TrimSpace(text))
	tokens := chars / CharactersPerToken
	if chars > 0 && tokens == 0 {
		return 1
	}
	return tokens
}

// NewEvidenceBundle packages disassembler outputs into a bounded, token-capped bundle.
func NewEvidenceBundle(binaryPath string, format BinaryFormat, symbol, pseudocode, disasm string, xRefs, strConstants []string) *EvidenceBundle {
	b := &EvidenceBundle{
		TargetBinary:    binaryPath,
		Format:          format,
		FunctionSymbol:  symbol,
		Pseudocode:      pseudocode,
		Disassembly:     disasm,
		CrossReferences: xRefs,
		StringConstants: strConstants,
	}

	b.compactToBudget()
	return b
}

func (b *EvidenceBundle) compactToBudget() {
	rawText := b.Pseudocode + b.Disassembly + strings.Join(b.CrossReferences, " ") + strings.Join(b.StringConstants, " ")
	tokens := EstimateTokens(rawText)

	if tokens <= MaxEvidenceTokens {
		b.TokenEstimate = tokens
		b.Truncated = false
		return
	}

	// Token budget exceeded -> compact disassembly first, then pseudocode if needed
	b.Truncated = true

	// Limit disassembly if too long
	if len(b.Disassembly) > 4000 {
		b.Disassembly = b.Disassembly[:4000] + "\n... [Disassembly truncated for context budget]"
	}

	// Limit pseudocode if still exceeds budget
	currentChars := len(b.Pseudocode) + len(b.Disassembly) + len(strings.Join(b.CrossReferences, " ")) + len(strings.Join(b.StringConstants, " "))
	if currentChars > MaxEvidenceChars {
		overflow := currentChars - MaxEvidenceChars
		if len(b.Pseudocode) > overflow+500 {
			cutPoint := len(b.Pseudocode) - overflow - 500
			b.Pseudocode = b.Pseudocode[:cutPoint] + "\n... [Pseudocode truncated for context budget]"
		}
	}

	finalText := b.Pseudocode + b.Disassembly + strings.Join(b.CrossReferences, " ") + strings.Join(b.StringConstants, " ")
	b.TokenEstimate = EstimateTokens(finalText)
}

// ToMarkdown converts the evidence bundle into bounded Markdown suitable for LLM injection.
func (b *EvidenceBundle) ToMarkdown() string {
	if b == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### REA Evidence Bundle: `%s` (%s)\n\n", b.FunctionSymbol, b.Format))
	sb.WriteString(fmt.Sprintf("- **Target Binary**: `%s`\n", b.TargetBinary))
	sb.WriteString(fmt.Sprintf("- **Estimated Token Cost**: ~%d tokens\n", b.TokenEstimate))
	if b.Truncated {
		sb.WriteString("- **Status**: *(Truncated to stay within < 4,000 token boundary)*\n")
	}
	sb.WriteString("\n")

	if len(b.StringConstants) > 0 {
		sb.WriteString("#### String Constants & Signatures:\n")
		for _, s := range b.StringConstants {
			sb.WriteString(fmt.Sprintf("- `\"%s\"`\n", s))
		}
		sb.WriteString("\n")
	}

	if len(b.CrossReferences) > 0 {
		sb.WriteString("#### Call Graph & Cross References:\n")
		for _, xr := range b.CrossReferences {
			sb.WriteString(fmt.Sprintf("- `%s`\n", xr))
		}
		sb.WriteString("\n")
	}

	if b.Pseudocode != "" {
		sb.WriteString("#### Recovered Pseudocode:\n```c\n")
		sb.WriteString(b.Pseudocode)
		sb.WriteString("\n```\n\n")
	}

	if b.Disassembly != "" {
		sb.WriteString("#### Disassembly Slice:\n```asm\n")
		sb.WriteString(b.Disassembly)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// RenderADR serializes an Architecture Decision Record with mandatory YAML frontmatter.
func RenderADR(doc *ADRDocument, bundle *EvidenceBundle) string {
	if doc == nil {
		return ""
	}

	dateStr := doc.Date
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	status := doc.Status
	if status == "" {
		status = "proposed"
	}
	repo := doc.Repository
	if repo == "" {
		repo = "dendrite"
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.ID))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", doc.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", status))
	sb.WriteString(fmt.Sprintf("date: %s\n", dateStr))
	if doc.PlaneEpic != "" {
		sb.WriteString(fmt.Sprintf("plane_epic: %s\n", doc.PlaneEpic))
	}
	sb.WriteString(fmt.Sprintf("repository: %s\n", repo))
	sb.WriteString("tags:\n")
	for _, tag := range doc.Tags {
		sb.WriteString(fmt.Sprintf("  - %s\n", tag))
	}
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", doc.ID, doc.Title))
	sb.WriteString("## 1. Context & Architectural Problem\n")
	sb.WriteString(doc.Context + "\n\n")

	sb.WriteString("## 2. Decision & Re-creation Strategy\n")
	sb.WriteString(doc.Decision + "\n\n")

	sb.WriteString("## 3. Consequences & Trade-offs\n")
	sb.WriteString(doc.Consequences + "\n\n")

	if bundle != nil {
		sb.WriteString("## 4. Reverse Engineering Evidence\n\n")
		sb.WriteString(bundle.ToMarkdown())
	}

	return sb.String()
}
