package tokens_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/tokens"
)

type mockProber struct {
	cliFound bool
	cliPath  string
	version  string
	probeOK  bool
	latency  time.Duration
	probeErr error
}

func (m *mockProber) FindCLI() (string, bool) {
	return m.cliPath, m.cliFound
}

func (m *mockProber) GetVersion(_ context.Context, _ string) (string, error) {
	return m.version, nil
}

func (m *mockProber) ProbeModel(_ context.Context, _, _ string) (bool, time.Duration, error) {
	return m.probeOK, m.latency, m.probeErr
}

func TestEvaluator_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a test scheduler policy
	policyPath := filepath.Join(tmpDir, "policy.json")
	policyContent := `{"night_window":"23:00-06:00","weekly_reset":"SUN 23:59","reserve_floor":0.20,"allow_zero":false,"mode":"auto"}`
	if err := os.WriteFile(policyPath, []byte(policyContent), 0644); err != nil {
		t.Fatalf("failed to write policy: %v", err)
	}

	// Write spend data
	spendPath := filepath.Join(tmpDir, "spend.json")
	spendContent := `{"spend_24h_usd":5.0,"spend_7d_usd":25.0,"tokens_24h":100000,"tokens_7d":500000}`
	if err := os.WriteFile(spendPath, []byte(spendContent), 0644); err != nil {
		t.Fatalf("failed to write spend: %v", err)
	}

	prober := &mockProber{
		cliFound: true,
		cliPath:  "/usr/local/bin/agy",
		version:  "1.3.5",
		probeOK:  true,
		latency:  45 * time.Millisecond,
	}

	fixedTime := time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC) // 14:00 UTC (daytime)
	eval := tokens.NewEvaluator(prober, func() time.Time { return fixedTime })

	cfg := tokens.DefaultConfig()
	cfg.PolicyFile = policyPath
	cfg.SpendFile = spendPath
	cfg.Model = "gemini-3.8-flash-high"

	report, err := eval.Evaluate(context.Background(), cfg, true)
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	// Assertions for Day Window
	if report.DayWindow.TotalCapacity != 1000000 {
		t.Errorf("expected 1,000,000 day capacity, got %d", report.DayWindow.TotalCapacity)
	}
	if report.DayWindow.ConsumedTokens != 100000 {
		t.Errorf("expected 100,000 consumed tokens, got %d", report.DayWindow.ConsumedTokens)
	}
	if report.DayWindow.RemainingTokens != 900000 {
		t.Errorf("expected 900,000 remaining tokens, got %d", report.DayWindow.RemainingTokens)
	}
	if report.DayWindow.RemainingRatio != 0.90 {
		t.Errorf("expected 0.90 remaining ratio, got %f", report.DayWindow.RemainingRatio)
	}

	// Assertions for Week Window
	if report.WeekWindow.ConsumedTokens != 500000 {
		t.Errorf("expected 500,000 consumed week tokens, got %d", report.WeekWindow.ConsumedTokens)
	}
	if report.WeekWindow.RemainingTokens != 6500000 {
		t.Errorf("expected 6,500,000 remaining week tokens, got %d", report.WeekWindow.RemainingTokens)
	}

	// Assertions for AGY Status
	if !report.AGYStatus.AccessConfirmed {
		t.Errorf("expected AGY access to be confirmed")
	}
	if !report.CanRunCloud {
		t.Errorf("expected CanRunCloud to be true")
	}
	if report.AGYStatus.ActiveModel != "gemini-3.8-flash-high" {
		t.Errorf("unexpected model: %s", report.AGYStatus.ActiveModel)
	}
	if !report.AGYStatus.ProbeSuccess {
		t.Errorf("expected probeSuccess to be true")
	}

	// Test Output Formatting
	formatted := tokens.FormatReport(report)
	if !strings.Contains(formatted, "gemini-3.8-flash-high") {
		t.Errorf("formatted report missing model name: %s", formatted)
	}
	if !strings.Contains(formatted, "[✓] CONFIRMED") {
		t.Errorf("formatted report missing confirmed banner: %s", formatted)
	}
}

func TestEvaluator_ReserveFloorBlocked(t *testing.T) {
	tmpDir := t.TempDir()

	policyPath := filepath.Join(tmpDir, "policy.json")
	policyContent := `{"night_window":"23:00-06:00","weekly_reset":"SUN 23:59","reserve_floor":0.40,"allow_zero":false,"mode":"auto"}`
	_ = os.WriteFile(policyPath, []byte(policyContent), 0644)

	// Consume 65% of weekly quota -> only 35% remaining (which is <= 40% reserve floor)
	spendPath := filepath.Join(tmpDir, "spend.json")
	spendContent := `{"tokens_24h":50000,"tokens_7d":4550000}` // 7M - 4.55M = 2.45M (35%)
	_ = os.WriteFile(spendPath, []byte(spendContent), 0644)

	prober := &mockProber{cliFound: true, cliPath: "/usr/local/bin/agy"}
	fixedTime := time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC)
	eval := tokens.NewEvaluator(prober, func() time.Time { return fixedTime })

	cfg := tokens.DefaultConfig()
	cfg.PolicyFile = policyPath
	cfg.SpendFile = spendPath

	report, err := eval.Evaluate(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.AGYStatus.InReserveFloor {
		t.Errorf("expected InReserveFloor to be true when remaining ratio is <= 0.40")
	}
	if report.CanRunCloud {
		t.Errorf("expected CanRunCloud to be false when reserve floor is hit")
	}
	if report.AGYStatus.AccessConfirmed {
		t.Errorf("expected AccessConfirmed to be false when reserve floor is hit")
	}

	formatted := tokens.FormatReport(report)
	if !strings.Contains(formatted, "[!] BLOCKED / HELD") {
		t.Errorf("expected BLOCKED/HELD banner: %s", formatted)
	}
}

func TestEvaluator_NightWindow(t *testing.T) {
	tmpDir := t.TempDir()

	policyPath := filepath.Join(tmpDir, "policy.json")
	policyContent := `{"night_window":"23:00-06:00","weekly_reset":"SUN 23:59","reserve_floor":0.20,"allow_zero":false,"mode":"auto"}`
	_ = os.WriteFile(policyPath, []byte(policyContent), 0644)

	prober := &mockProber{cliFound: true, cliPath: "/usr/local/bin/agy"}
	fixedTime := time.Date(2026, 10, 14, 23, 30, 0, 0, time.UTC) // 23:30 (night window)
	eval := tokens.NewEvaluator(prober, func() time.Time { return fixedTime })

	cfg := tokens.DefaultConfig()
	cfg.PolicyFile = policyPath

	report, err := eval.Evaluate(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.AGYStatus.InNightWindow {
		t.Errorf("expected InNightWindow to be true at 23:30")
	}
	if !strings.Contains(report.AGYStatus.StatusReason, "Night Window active") {
		t.Errorf("unexpected status reason: %s", report.AGYStatus.StatusReason)
	}
}

func TestEvaluator_MissingCLI(t *testing.T) {
	prober := &mockProber{cliFound: false}
	eval := tokens.NewEvaluator(prober, nil)

	report, err := eval.Evaluate(context.Background(), tokens.DefaultConfig(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.AGYStatus.CLIAvailable {
		t.Errorf("expected CLIAvailable to be false")
	}
	if report.CanRunCloud {
		t.Errorf("expected CanRunCloud to be false when CLI is missing")
	}
}

func TestEvaluator_DayTokensExhausted(t *testing.T) {
	tmpDir := t.TempDir()
	spendPath := filepath.Join(tmpDir, "spend.json")
	spendContent := `{"tokens_24h":1000000,"tokens_7d":1000000}`
	_ = os.WriteFile(spendPath, []byte(spendContent), 0644)

	prober := &mockProber{cliFound: true, cliPath: "/bin/agy"}
	eval := tokens.NewEvaluator(prober, nil)

	cfg := tokens.DefaultConfig()
	cfg.SpendFile = spendPath

	report, err := eval.Evaluate(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.DayWindow.RemainingTokens != 0 {
		t.Errorf("expected 0 remaining day tokens, got %d", report.DayWindow.RemainingTokens)
	}
	if report.CanRunCloud {
		t.Errorf("expected CanRunCloud to be false when day tokens exhausted")
	}
}

func TestEvaluator_SpendFallbackAndProbeError(t *testing.T) {
	tmpDir := t.TempDir()
	spendPath := filepath.Join(tmpDir, "spend.json")
	// spend with USD amounts only and remaining_quota
	spendContent := `{"spend_24h_usd":2.5,"spend_7d_usd":17.5,"remaining_quota":0.80}`
	_ = os.WriteFile(spendPath, []byte(spendContent), 0644)

	prober := &mockProber{
		cliFound: true,
		cliPath:  "/bin/agy",
		version:  "1.2.0",
		probeOK:  false,
		probeErr: errors.New("timeout"),
	}
	eval := tokens.NewEvaluator(prober, nil)

	cfg := tokens.DefaultConfig()
	cfg.SpendFile = spendPath

	report, err := eval.Evaluate(context.Background(), cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.AGYStatus.ProbeSuccess {
		t.Errorf("expected probeSuccess to be false on probeErr")
	}
	if report.WeekWindow.RemainingRatio != 0.80 {
		t.Errorf("expected 0.80 remaining quota ratio, got %f", report.WeekWindow.RemainingRatio)
	}
}

func TestDefaultProber(t *testing.T) {
	prober := tokens.DefaultProber{}
	_, _ = prober.FindCLI()
}

func TestDefaultConfig(t *testing.T) {
	cfg := tokens.DefaultConfig()
	if cfg.DayCapacityTokens <= 0 || cfg.WeekCapacityTokens <= 0 {
		t.Errorf("invalid default config: %+v", cfg)
	}
	data, err := json.Marshal(cfg)
	if err != nil || len(data) == 0 {
		t.Errorf("failed to marshal default config: %v", err)
	}
}
