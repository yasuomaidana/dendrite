package showcase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	handlerMu      sync.RWMutex
	customHandlers = make(map[string]Handler)
)

// RegisterHandler registers or overrides a handler for a target.
func RegisterHandler(target string, h Handler) {
	handlerMu.Lock()
	defer handlerMu.Unlock()
	customHandlers[strings.ToLower(strings.TrimSpace(target))] = h
}

// ResetHandlers clears any registered custom handlers.
func ResetHandlers() {
	handlerMu.Lock()
	defer handlerMu.Unlock()
	customHandlers = make(map[string]Handler)
}

// TargetHandlers returns a registry of all available showcase handlers.
func TargetHandlers() map[string]Handler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	m := map[string]Handler{
		TargetAegis:      &AegisHandler{},
		TargetDXBall:     &DXBallHandler{},
		TargetCTF:        &CTFHandler{},
		TargetTH04:       &TH04Handler{},
		TargetMattermost: &MattermostHandler{},
	}
	for k, v := range customHandlers {
		m[k] = v
	}
	return m
}

// GetHandler returns the handler for the specified showcase target.
func GetHandler(target string) (Handler, error) {
	norm := strings.ToLower(strings.TrimSpace(target))
	handlers := TargetHandlers()
	h, exists := handlers[norm]
	if !exists {
		return nil, fmt.Errorf("unknown showcase target '%s' (available: aegis, dxball, ctf, th04, mattermost, all)", target)
	}
	return h, nil
}

// ListTargets returns metadata for all registered showcase targets.
func ListTargets() []ShowcaseMetadata {
	targets := []string{TargetAegis, TargetDXBall, TargetCTF, TargetTH04, TargetMattermost}
	handlers := TargetHandlers()
	var list []ShowcaseMetadata
	for _, t := range targets {
		if h, ok := handlers[t]; ok {
			list = append(list, h.Metadata())
		}
	}
	return list
}

// RunShowcase executes the specified showcase target or all targets if requested.
func RunShowcase(ctx context.Context, target string) (*ShowcaseReport, error) {
	norm := strings.ToLower(strings.TrimSpace(target))
	if norm == "" {
		norm = TargetAll
	}

	var targetsToRun []string
	if norm == TargetAll {
		targetsToRun = []string{TargetAegis, TargetDXBall, TargetCTF, TargetTH04, TargetMattermost}
	} else {
		targetsToRun = []string{norm}
	}

	handlers := TargetHandlers()
	report := &ShowcaseReport{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		TotalTargets: len(targetsToRun),
	}

	totalPassedVectors := 0
	totalVectors := 0

	for _, t := range targetsToRun {
		h, exists := handlers[t]
		if !exists {
			return nil, fmt.Errorf("unknown showcase target '%s'", t)
		}

		res, err := h.Execute(ctx)
		if err != nil {
			report.FailedTargets++
			report.Results = append(report.Results, ShowcaseResult{
				Target:           t,
				PassedValidation: false,
				Error:            err.Error(),
			})
			continue
		}

		if res.PassedValidation {
			report.SucceededTargets++
		} else {
			report.FailedTargets++
		}

		totalVectors += res.Parity.TotalVectors
		totalPassedVectors += res.Parity.PassedVectors
		report.Results = append(report.Results, *res)
	}

	if totalVectors > 0 {
		report.OverallParityPercent = float64(totalPassedVectors) / float64(totalVectors) * 100.0
	}

	if report.FailedTargets == 0 && report.SucceededTargets > 0 {
		report.OverallStatus = "PASSED"
	} else {
		report.OverallStatus = "FAILED"
	}

	return report, nil
}

// RenderMarkdownScorecard formats the showcase report into a markdown summary document.
func RenderMarkdownScorecard(report *ShowcaseReport) string {
	if report == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Open-Source REA Showcase & Cleanroom Parity Scorecard\n\n")
	sb.WriteString(fmt.Sprintf("- **Timestamp**: `%s`\n", report.Timestamp))
	sb.WriteString(fmt.Sprintf("- **Overall Status**: **%s**\n", report.OverallStatus))
	sb.WriteString(fmt.Sprintf("- **Targets Evaluated**: %d (Passed: %d, Failed: %d)\n", report.TotalTargets, report.SucceededTargets, report.FailedTargets))
	sb.WriteString(fmt.Sprintf("- **Aggregate Parity Score**: **%.2f%%**\n\n", report.OverallParityPercent))

	sb.WriteString("## Showcase Targets Overview\n\n")
	sb.WriteString("| Target | Upstream Repo | License | Binary Format | Vectors Tested | Parity % | Status |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :---: | :---: | :---: |\n")

	for _, r := range report.Results {
		status := "✅ PASS"
		if !r.PassedValidation {
			status = "❌ FAIL"
		}
		sb.WriteString(fmt.Sprintf("| **%s** | `%s` | %s | %s | %d | %.2f%% | %s |\n",
			r.Metadata.Name,
			r.Metadata.UpstreamRepo,
			r.Metadata.LicenseSPDX,
			r.Metadata.BinaryFormat,
			r.Parity.TotalVectors,
			r.Parity.ParityPercent,
			status,
		))
	}
	sb.WriteString("\n")

	sb.WriteString("## Cleanroom Verification Details & Invariants\n\n")
	for _, r := range report.Results {
		sb.WriteString(fmt.Sprintf("### %s (`%s`)\n\n", r.Metadata.Name, r.Target))
		sb.WriteString(fmt.Sprintf("- **Capability Extracted**: %s\n", r.Metadata.ExtractedCapability))
		sb.WriteString(fmt.Sprintf("- **Test Oracle**: %s\n", r.Metadata.GroundTruthOracle))
		sb.WriteString(fmt.Sprintf("- **SPDX Compliance**: %t (License: %s, OSI-Approved: %t)\n", r.SPDX.Compliant, r.SPDX.LicenseID, r.SPDX.OSIApproved))
		if r.Evidence != nil {
			sb.WriteString(fmt.Sprintf("- **Token Budget**: %d / 4,000 tokens (Truncated: %t)\n", r.Evidence.TokenEstimate, r.Evidence.Truncated))
		}
		sb.WriteString(fmt.Sprintf("- **Parity Verification**: %s\n", r.Parity.Details))
		sb.WriteString(fmt.Sprintf("- **Execution Time**: %d ms\n\n", r.ExecutionTimeMs))
	}

	return sb.String()
}
