package showcase

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSPDXAuditor(t *testing.T) {
	tests := []struct {
		target   string
		license  string
		wantOK   bool
		wantCopy bool
	}{
		{"aegis", "GPL-3.0", true, true},
		{"aegis-variant", "GPL-3.0-only", true, true},
		{"dxball", "MIT", true, false},
		{"mattermost", "Apache-2.0", true, false},
		{"bsd", "BSD-3-Clause", true, false},
		{"cc", "CC-BY-4.0", true, false},
		{"case-insensitive", "apache-2.0", true, false},
		{"proprietary", "Commercial-Closed-Source", false, false},
		{"unknown", "CustomNonFree-1.0", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			res := AuditLicense(tc.target, tc.license)
			if res.Compliant != tc.wantOK {
				t.Errorf("license %s: got Compliant=%t, want %t", tc.license, res.Compliant, tc.wantOK)
			}
			if res.IsCopyleft != tc.wantCopy {
				t.Errorf("license %s: got IsCopyleft=%t, want %t", tc.license, res.IsCopyleft, tc.wantCopy)
			}
			if tc.wantOK && !strings.Contains(res.Notes, "Compliant") {
				t.Errorf("expected compliant notes, got: %s", res.Notes)
			}
			if !tc.wantOK && !strings.Contains(res.Notes, "Unrecognized") {
				t.Errorf("expected unrecognized notes, got: %s", res.Notes)
			}
		})
	}
}

func TestTOTPGenerator_ErrorsAndDefaults(t *testing.T) {
	// Empty secret error
	_, err := GenerateTOTP(nil, 12345, DefaultTOTPConfig())
	if err == nil {
		t.Fatal("expected error on empty secret")
	}

	// Invalid digits <= 0
	cfg := DefaultTOTPConfig()
	cfg.Digits = 0
	_, err = GenerateTOTP([]byte("secret"), 12345, cfg)
	if err == nil {
		t.Fatal("expected error on digits = 0")
	}

	// Invalid digits > 10
	cfg.Digits = 11
	_, err = GenerateTOTP([]byte("secret"), 12345, cfg)
	if err == nil {
		t.Fatal("expected error on digits > 10")
	}

	// Period <= 0 should default to 30
	cfg.Digits = 6
	cfg.Period = 0
	code, err := GenerateTOTP([]byte("12345678901234567890"), 59, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "287082" {
		t.Errorf("got code %s, want 287082", code)
	}
}

func TestAegisHandler_Full(t *testing.T) {
	h := &AegisHandler{}
	meta := h.Metadata()
	if meta.ID != TargetAegis {
		t.Errorf("got ID %s, want %s", meta.ID, TargetAegis)
	}

	res, err := h.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !res.PassedValidation {
		t.Errorf("expected PassedValidation = true, got false")
	}
	if res.Parity.ParityPercent != 100.0 {
		t.Errorf("expected 100%% parity, got %.2f%%", res.Parity.ParityPercent)
	}
	if res.Evidence.Truncated {
		t.Errorf("expected bundle within 4,000 tokens, got truncated")
	}
}

func TestDXBallHandler_Full(t *testing.T) {
	h := &DXBallHandler{}
	meta := h.Metadata()
	if meta.ID != TargetDXBall {
		t.Errorf("got ID %s, want %s", meta.ID, TargetDXBall)
	}

	// Unit test edge cases of CalculatePositionPan
	if pan := CalculatePositionPan(320); pan != 0 {
		t.Errorf("center pan: got %d, want 0", pan)
	}
	if pan := CalculatePositionPan(0); pan != -10000 {
		t.Errorf("left pan: got %d, want -10000", pan)
	}
	if pan := CalculatePositionPan(640); pan != 10000 {
		t.Errorf("right pan: got %d, want 10000", pan)
	}
	if pan := CalculatePositionPan(-5000); pan != -10000 {
		t.Errorf("clamped left pan: got %d, want -10000", pan)
	}
	if pan := CalculatePositionPan(5000); pan != 10000 {
		t.Errorf("clamped right pan: got %d, want 10000", pan)
	}

	res, err := h.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !res.PassedValidation {
		t.Errorf("expected PassedValidation = true, got false")
	}
	if res.Parity.TotalVectors != 3205 {
		t.Errorf("expected 3205 vectors, got %d", res.Parity.TotalVectors)
	}
	if res.Parity.ParityPercent != 100.0 {
		t.Errorf("expected 100%% parity, got %.2f%%", res.Parity.ParityPercent)
	}
}

func TestCTFHandler_Full(t *testing.T) {
	h := &CTFHandler{}
	meta := h.Metadata()
	if meta.ID != TargetCTF {
		t.Errorf("got ID %s, want %s", meta.ID, TargetCTF)
	}

	// Test solver error cases
	_, err := SolveConstraints(nil)
	if err == nil {
		t.Fatal("expected error on empty constraints")
	}

	// Test unsatisfiable constraint
	impossible := []ConstraintEquation{
		{Index: 0, Multiplier: 0, Addend: 0, Result: 999, XorKey: 0, XorResult: 0},
	}
	_, err = SolveConstraints(impossible)
	if err == nil {
		t.Fatal("expected error on unsatisfiable constraints")
	}

	// Test oracle checks
	if VerifyFlagAgainstOracle("random") {
		t.Error("expected false for random string")
	}
	if VerifyFlagAgainstOracle("DUCTF{missing_close") {
		t.Error("expected false for missing closing brace")
	}
	if VerifyFlagAgainstOracle("invalid_prefix}") {
		t.Error("expected false for missing prefix")
	}
	if VerifyFlagAgainstOracle("DUCTF{wrong_body}") {
		t.Error("expected false for wrong body")
	}
	if !VerifyFlagAgainstOracle(ExpectedCTFFlag) {
		t.Error("expected true for exact expected flag")
	}

	res, err := h.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !res.PassedValidation {
		t.Errorf("expected PassedValidation = true, got false")
	}
	if res.Parity.ParityPercent != 100.0 {
		t.Errorf("expected 100%% parity, got %.2f%%", res.Parity.ParityPercent)
	}
}

func TestTH04Handler_Full(t *testing.T) {
	h := &TH04Handler{}
	meta := h.Metadata()
	if meta.ID != TargetTH04 {
		t.Errorf("got ID %s, want %s", meta.ID, TargetTH04)
	}

	// Test aim quadrants
	angle, vx, vy := FixedPointAim(0, 0, 100, 0, 256)
	if angle != 0 || vx != 256 || vy != 0 {
		t.Errorf("due east aim error: angle=%d, vx=%d, vy=%d", angle, vx, vy)
	}

	res, err := h.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !res.PassedValidation {
		t.Errorf("expected PassedValidation = true, got false")
	}
	if res.Parity.ParityPercent != 100.0 {
		t.Errorf("expected 100%% parity, got %.2f%%", res.Parity.ParityPercent)
	}
}

func TestMattermostHandler_Full(t *testing.T) {
	h := &MattermostHandler{}
	meta := h.Metadata()
	if meta.ID != TargetMattermost {
		t.Errorf("got ID %s, want %s", meta.ID, TargetMattermost)
	}

	// Test channel permissions
	if !IsChannelPermitted("desktop-notification") {
		t.Error("desktop-notification should be permitted")
	}
	if IsChannelPermitted("unauthorized-exec") {
		t.Error("unauthorized-exec should not be permitted")
	}

	// Test HTML sanitization
	dirty := `<p>Clean text</p><script type="text/javascript">steal()</script><div onmouseover="hack()">Hover</div><a href="javascript:void(0)">Click</a>`
	payload := SanitizeAndSerializeClipboard("Clean text", dirty)
	if strings.Contains(payload.HTML, "<script") || strings.Contains(payload.HTML, "onmouseover=") || strings.Contains(payload.HTML, "javascript:") {
		t.Errorf("sanitization failed: %s", payload.HTML)
	}
	if !strings.Contains(payload.HTML, "<p>Clean text</p>") {
		t.Errorf("expected clean HTML body retained, got: %s", payload.HTML)
	}

	res, err := h.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !res.PassedValidation {
		t.Errorf("expected PassedValidation = true, got false")
	}
	if res.Parity.ParityPercent != 100.0 {
		t.Errorf("expected 100%% parity, got %.2f%%", res.Parity.ParityPercent)
	}
}

type mockErrHandler struct{}

func (m *mockErrHandler) Metadata() ShowcaseMetadata {
	return ShowcaseMetadata{ID: "mock-err", Name: "Mock Error"}
}

func (m *mockErrHandler) Execute(ctx context.Context) (*ShowcaseResult, error) {
	return nil, fmt.Errorf("injected mock execution error")
}

type mockFailValidationHandler struct{}

func (m *mockFailValidationHandler) Metadata() ShowcaseMetadata {
	return ShowcaseMetadata{ID: "mock-fail", Name: "Mock Fail Validation"}
}

func (m *mockFailValidationHandler) Execute(ctx context.Context) (*ShowcaseResult, error) {
	return &ShowcaseResult{
		Target:           "mock-fail",
		PassedValidation: false,
		Parity: TestParityResult{
			TotalVectors:  0,
			PassedVectors: 0,
		},
	}, nil
}

func TestRunner_Operations(t *testing.T) {
	defer ResetHandlers()

	// 1. List targets
	targets := ListTargets()
	if len(targets) != 5 {
		t.Errorf("expected 5 targets, got %d", len(targets))
	}

	// 2. GetHandler valid & invalid
	h, err := GetHandler("aegis")
	if err != nil || h == nil {
		t.Fatalf("GetHandler(aegis) failed: %v", err)
	}
	_, err = GetHandler("nonexistent")
	if err == nil {
		t.Fatal("expected error on nonexistent target")
	}

	// 3. RunShowcase single target
	singleRep, err := RunShowcase(context.Background(), "th04")
	if err != nil {
		t.Fatalf("RunShowcase(th04) failed: %v", err)
	}
	if singleRep.TotalTargets != 1 || singleRep.SucceededTargets != 1 {
		t.Errorf("unexpected single report stats: %+v", singleRep)
	}

	// 4. RunShowcase all targets
	allRep, err := RunShowcase(context.Background(), "all")
	if err != nil {
		t.Fatalf("RunShowcase(all) failed: %v", err)
	}
	if allRep.TotalTargets != 5 || allRep.SucceededTargets != 5 {
		t.Errorf("unexpected all report stats: %+v", allRep)
	}
	if allRep.OverallStatus != "PASSED" {
		t.Errorf("expected PASSED, got %s", allRep.OverallStatus)
	}
	if allRep.OverallParityPercent != 100.0 {
		t.Errorf("expected 100%% overall parity, got %.2f%%", allRep.OverallParityPercent)
	}

	// 5. RunShowcase empty string defaults to all
	defaultRep, err := RunShowcase(context.Background(), "")
	if err != nil {
		t.Fatalf("RunShowcase('') failed: %v", err)
	}
	if defaultRep.TotalTargets != 5 {
		t.Errorf("expected 5 targets on empty string, got %d", defaultRep.TotalTargets)
	}

	// 6. RunShowcase unknown target
	_, err = RunShowcase(context.Background(), "unknown_target_xyz")
	if err == nil {
		t.Fatal("expected error on unknown target")
	}

	// 7. Markdown scorecard rendering
	md := RenderMarkdownScorecard(allRep)
	if !strings.Contains(md, "# Open-Source REA Showcase & Cleanroom Parity Scorecard") {
		t.Errorf("markdown scorecard missing header: %s", md)
	}
	if !strings.Contains(md, "100.00%") {
		t.Errorf("markdown scorecard missing 100.00%%: %s", md)
	}
	if RenderMarkdownScorecard(nil) != "" {
		t.Error("expected empty string for nil report")
	}

	// Render with failure
	failedRep := &ShowcaseReport{
		Timestamp:            "2026-10-14T00:00:00Z",
		OverallStatus:        "FAILED",
		TotalTargets:         1,
		FailedTargets:        1,
		OverallParityPercent: 50.0,
		Results: []ShowcaseResult{
			{
				Target:           "mock",
				PassedValidation: false,
				Metadata: ShowcaseMetadata{
					Name: "Mock Target",
				},
				Parity: TestParityResult{
					TotalVectors:  10,
					PassedVectors: 5,
					ParityPercent: 50.0,
				},
			},
		},
	}
	failedMd := RenderMarkdownScorecard(failedRep)
	if !strings.Contains(failedMd, "❌ FAIL") {
		t.Errorf("expected ❌ FAIL in markdown, got: %s", failedMd)
	}

	// 8. Custom handler returning error
	RegisterHandler("mock-err", &mockErrHandler{})
	errRep, err := RunShowcase(context.Background(), "mock-err")
	if err != nil {
		t.Fatalf("unexpected error running mock-err: %v", err)
	}
	if errRep.FailedTargets != 1 || errRep.OverallStatus != "FAILED" {
		t.Errorf("expected failed status for mock-err, got %+v", errRep)
	}

	// 9. Custom handler returning PassedValidation: false and 0 vectors
	RegisterHandler("mock-fail", &mockFailValidationHandler{})
	failRep, err := RunShowcase(context.Background(), "mock-fail")
	if err != nil {
		t.Fatalf("unexpected error running mock-fail: %v", err)
	}
	if failRep.FailedTargets != 1 || failRep.OverallStatus != "FAILED" {
		t.Errorf("expected failed status for mock-fail, got %+v", failRep)
	}
}
