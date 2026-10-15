package tokens

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/scheduler/token"
)

// SpendData defines the file format for spend tracking.
type SpendData struct {
	TotalSpendUSD  float64 `json:"total_spend_usd,omitempty"`
	Spend24hUSD    float64 `json:"spend_24h_usd,omitempty"`
	Spend7dUSD     float64 `json:"spend_7d_usd,omitempty"`
	Tokens24h      int64   `json:"tokens_24h,omitempty"`
	Tokens7d       int64   `json:"tokens_7d,omitempty"`
	RemainingQuota float64 `json:"remaining_quota,omitempty"`
}

// Prober abstracts the AGY CLI detection and probing for testing.
type Prober interface {
	FindCLI() (string, bool)
	GetVersion(ctx context.Context, cliPath string) (string, error)
	ProbeModel(ctx context.Context, cliPath, model string) (bool, time.Duration, error)
}

// DefaultProber implements Prober using the host OS execution environment.
type DefaultProber struct{}

// FindCLI locates the agy executable in standard locations and PATH.
func (DefaultProber) FindCLI() (string, bool) {
	// 1. Check direct standard local bin
	home, _ := os.UserHomeDir()
	if home != "" {
		localPath := filepath.Join(home, ".local", "bin", "agy")
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			return localPath, true
		}
	}
	// 2. Check PATH
	if p, err := exec.LookPath("agy"); err == nil {
		return p, true
	}
	return "", false
}

// GetVersion executes agy --version to inspect installed version.
func (DefaultProber) GetVersion(ctx context.Context, cliPath string) (string, error) {
	cmd := exec.CommandContext(ctx, cliPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ProbeModel runs a minimal ping turn against AGY with the designated model.
func (DefaultProber) ProbeModel(ctx context.Context, cliPath, model string) (bool, time.Duration, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, cliPath, "--model", model, "-p", "ping")
	err := cmd.Run()
	latency := time.Since(start)
	if err != nil {
		return false, latency, err
	}
	return true, latency, nil
}

// Evaluator manages token windows and AGY status evaluation.
type Evaluator struct {
	prober Prober
	nowFn  func() time.Time
}

// NewEvaluator creates a new Evaluator with default OS prober and time source.
func NewEvaluator(prober Prober, nowFn func() time.Time) *Evaluator {
	if prober == nil {
		prober = DefaultProber{}
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Evaluator{
		prober: prober,
		nowFn:  nowFn,
	}
}

// Evaluate evaluates both Day and Week token windows and confirms AGY connectivity.
func (e *Evaluator) Evaluate(ctx context.Context, cfg TokenTrackerConfig, probeAGY bool) (*TokenStatusReport, error) {
	now := e.nowFn()

	// 1. Load Spend & Usage Data if available
	var spend SpendData
	if cfg.SpendFile != "" {
		if data, err := os.ReadFile(cfg.SpendFile); err == nil {
			_ = json.Unmarshal(data, &spend)
		}
	}

	// 2. Load Scheduler Policy for Week Reset & Reserve Floor (with resilient partial fallback)
	pol := token.DefaultPolicy()
	if cfg.PolicyFile != "" {
		if data, err := os.ReadFile(cfg.PolicyFile); err == nil {
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err == nil {
				if nw, ok := raw["night_window"].(string); ok && nw != "" {
					pol.NightWindow = nw
				}
				if wr, ok := raw["weekly_reset"].(string); ok && wr != "" {
					pol.WeeklyReset = wr
				}
				if rf, ok := raw["reserve_floor"].(float64); ok {
					pol.ReserveFloor = rf
				}
				if az, ok := raw["allow_zero"].(bool); ok {
					pol.AllowZero = az
				}
				if m, ok := raw["mode"].(string); ok && m != "" {
					pol.Mode = m
				}
				if sbr, ok := raw["sprint_before_reset"].(float64); ok && sbr > 0 {
					pol.SprintBeforeReset = time.Duration(sbr)
				}
			}
		}
	}

	// 3. Compute Day Window (24h) Token Metrics
	consumed24h := spend.Tokens24h
	if consumed24h <= 0 && spend.Spend24hUSD > 0 {
		consumed24h = int64(spend.Spend24hUSD / 0.000005)
	}
	if consumed24h > cfg.DayCapacityTokens {
		consumed24h = cfg.DayCapacityTokens
	}
	remDayTokens := cfg.DayCapacityTokens - consumed24h
	if remDayTokens < 0 {
		remDayTokens = 0
	}
	remDayRatio := float64(remDayTokens) / float64(cfg.DayCapacityTokens)
	if cfg.DayCapacityTokens == 0 {
		remDayRatio = 0.0
	}
	spent24hUSD := spend.Spend24hUSD
	if spent24hUSD <= 0 && consumed24h > 0 {
		spent24hUSD = float64(consumed24h) * 0.000005
	}
	remDayUSD := cfg.DayBudgetUSD - spent24hUSD
	if remDayUSD < 0 {
		remDayUSD = 0
	}

	// Calculate Day Reset (Next Midnight)
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	timeUntilDayReset := midnight.Sub(now)

	dayMetrics := WindowTokenMetrics{
		WindowName:      "Day (24h Window)",
		TotalCapacity:   cfg.DayCapacityTokens,
		ConsumedTokens:  consumed24h,
		RemainingTokens: remDayTokens,
		RemainingRatio:  remDayRatio,
		BudgetUSD:       cfg.DayBudgetUSD,
		SpentUSD:        spent24hUSD,
		RemainingUSD:    remDayUSD,
		ResetTimestamp:  midnight,
		TimeUntilReset:  timeUntilDayReset,
	}

	// 4. Compute Week Window (7-Day Quota) Metrics
	consumed7d := spend.Tokens7d
	if consumed7d <= 0 && spend.Spend7dUSD > 0 {
		consumed7d = int64(spend.Spend7dUSD / 0.000005)
	}
	if consumed7d > cfg.WeekCapacityTokens {
		consumed7d = cfg.WeekCapacityTokens
	}
	remWeekTokens := cfg.WeekCapacityTokens - consumed7d
	if remWeekTokens < 0 {
		remWeekTokens = 0
	}
	remWeekRatio := float64(remWeekTokens) / float64(cfg.WeekCapacityTokens)
	if cfg.WeekCapacityTokens == 0 {
		remWeekRatio = 0.0
	}
	if spend.RemainingQuota > 0 && spend.RemainingQuota <= 1.0 {
		remWeekRatio = spend.RemainingQuota
		remWeekTokens = int64(float64(cfg.WeekCapacityTokens) * remWeekRatio)
		consumed7d = cfg.WeekCapacityTokens - remWeekTokens
	}
	spent7dUSD := spend.Spend7dUSD
	if spent7dUSD <= 0 && consumed7d > 0 {
		spent7dUSD = float64(consumed7d) * 0.000005
	}
	remWeekUSD := cfg.WeekBudgetUSD - spent7dUSD
	if remWeekUSD < 0 {
		remWeekUSD = 0
	}

	nextWeeklyReset, err := token.NextWeeklyReset(now, &pol)
	var timeUntilWeekReset time.Duration
	if err == nil {
		timeUntilWeekReset = nextWeeklyReset.Sub(now)
	}

	weekMetrics := WindowTokenMetrics{
		WindowName:      fmt.Sprintf("Week (7d Quota Reset: %s)", pol.WeeklyReset),
		TotalCapacity:   cfg.WeekCapacityTokens,
		ConsumedTokens:  consumed7d,
		RemainingTokens: remWeekTokens,
		RemainingRatio:  remWeekRatio,
		BudgetUSD:       cfg.WeekBudgetUSD,
		SpentUSD:        spent7dUSD,
		RemainingUSD:    remWeekUSD,
		ResetTimestamp:  nextWeeklyReset,
		TimeUntilReset:  timeUntilWeekReset,
	}

	// 5. Evaluate Quota Governor Reserve Floor & Night Window
	inNight, _ := token.IsNightWindow(now, &pol)
	inReserveFloor := remWeekRatio <= pol.ReserveFloor && !pol.AllowZero

	// 6. Evaluate AGY Connectivity
	cliPath, cliAvailable := e.prober.FindCLI()
	cliVersion := ""
	if cliAvailable {
		vCtx, vCancel := context.WithTimeout(ctx, 2*time.Second)
		defer vCancel()
		if ver, vErr := e.prober.GetVersion(vCtx, cliPath); vErr == nil {
			cliVersion = ver
		}
	}

	probeSuccess := false
	var probeLatency time.Duration
	if cliAvailable && probeAGY {
		pCtx, pCancel := context.WithTimeout(ctx, 5*time.Second)
		defer pCancel()
		ok, lat, _ := e.prober.ProbeModel(pCtx, cliPath, cfg.Model)
		probeSuccess = ok
		probeLatency = lat
	}

	canRunCloud := cliAvailable && remDayTokens > 0 && remWeekTokens > 0 && !inReserveFloor

	statusReason := "AGY access confirmed: Tokens available in both Day and Week windows"
	if !cliAvailable {
		statusReason = "AGY binary not detected in PATH or ~/.local/bin/agy"
	} else if remDayTokens <= 0 {
		statusReason = "Day window tokens exhausted: Waiting for daily reset"
	} else if inReserveFloor {
		statusReason = fmt.Sprintf("Remaining weekly quota reached safety floor (%.1f%% <= %.1f%%): Cloud calls halted to preserve reserve", remWeekRatio*100, pol.ReserveFloor*100)
	} else if inNight {
		statusReason = fmt.Sprintf("AGY online (%.1f%% quota remaining), but Night Window active (%s): Zero-cost local execution preferred", remWeekRatio*100, pol.NightWindow)
	}

	agyStatus := AGYAccessStatus{
		AccessConfirmed:   canRunCloud,
		StatusReason:      statusReason,
		ActiveModel:       cfg.Model,
		GatewayBaseURL:    cfg.GatewayBaseURL,
		CLIAvailable:      cliAvailable,
		CLIPath:           cliPath,
		CLIVersion:        cliVersion,
		InNightWindow:     inNight,
		InReserveFloor:    inReserveFloor,
		ReserveFloorRatio: pol.ReserveFloor,
		ProbeSuccess:      probeSuccess,
		ProbeLatency:      probeLatency,
	}

	return &TokenStatusReport{
		Timestamp:   now,
		DayWindow:   dayMetrics,
		WeekWindow:  weekMetrics,
		AGYStatus:   agyStatus,
		CanRunCloud: canRunCloud,
	}, nil
}

// FormatReport creates a clear, structured ASCII terminal presentation of the token windows and AGY status.
func FormatReport(r *TokenStatusReport) string {
	var sb strings.Builder

	sb.WriteString("=== Dendrite Token Windows & AGY Access Confirmation ===\n")
	sb.WriteString(fmt.Sprintf("Timestamp:               %s\n\n", r.Timestamp.Format(time.RFC3339)))

	// Day Window
	sb.WriteString(fmt.Sprintf("[+] %s:\n", r.DayWindow.WindowName))
	sb.WriteString(fmt.Sprintf("    • Total Capacity:    %s tokens ($%.2f ceiling)\n", formatNumber(r.DayWindow.TotalCapacity), r.DayWindow.BudgetUSD))
	sb.WriteString(fmt.Sprintf("    • Consumed (24h):    %s tokens ($%.2f spent)\n", formatNumber(r.DayWindow.ConsumedTokens), r.DayWindow.SpentUSD))
	sb.WriteString(fmt.Sprintf("    • Remaining in Day:  %s tokens (%.1f%%) [$%.2f remaining]\n", formatNumber(r.DayWindow.RemainingTokens), r.DayWindow.RemainingRatio*100, r.DayWindow.RemainingUSD))
	sb.WriteString(fmt.Sprintf("    • Next Daily Reset:  %s (in %s)\n\n", r.DayWindow.ResetTimestamp.Format("15:04:05"), r.DayWindow.TimeUntilReset.Round(time.Minute)))

	// Week Window
	floorTokens := int64(float64(r.WeekWindow.TotalCapacity) * r.AGYStatus.ReserveFloorRatio)
	usableTokens := r.WeekWindow.RemainingTokens - floorTokens
	if usableTokens < 0 {
		usableTokens = 0
	}

	sb.WriteString(fmt.Sprintf("[+] %s:\n", r.WeekWindow.WindowName))
	sb.WriteString(fmt.Sprintf("    • Total Capacity:    %s tokens ($%.2f ceiling)\n", formatNumber(r.WeekWindow.TotalCapacity), r.WeekWindow.BudgetUSD))
	sb.WriteString(fmt.Sprintf("    • Consumed (7d):     %s tokens ($%.2f spent)\n", formatNumber(r.WeekWindow.ConsumedTokens), r.WeekWindow.SpentUSD))
	sb.WriteString(fmt.Sprintf("    • Remaining in Week: %s tokens (%.1f%%) [$%.2f remaining]\n", formatNumber(r.WeekWindow.RemainingTokens), r.WeekWindow.RemainingRatio*100, r.WeekWindow.RemainingUSD))
	sb.WriteString(fmt.Sprintf("    • Reserve Floor:     %.1f%% (%s tokens preserved)\n", r.AGYStatus.ReserveFloorRatio*100, formatNumber(floorTokens)))
	sb.WriteString(fmt.Sprintf("    • Usable Pre-Floor:  %s tokens\n", formatNumber(usableTokens)))
	sb.WriteString(fmt.Sprintf("    • Next Weekly Reset: %s (in %s)\n\n", r.WeekWindow.ResetTimestamp.Format(time.RFC3339), r.WeekWindow.TimeUntilReset.Round(time.Minute)))

	// AGY Status
	cliInfo := "NOT DETECTED"
	if r.AGYStatus.CLIAvailable {
		cliInfo = fmt.Sprintf("Installed (%s", r.AGYStatus.CLIPath)
		if r.AGYStatus.CLIVersion != "" {
			cliInfo += fmt.Sprintf(" v%s", r.AGYStatus.CLIVersion)
		}
		cliInfo += ")"
	}

	accessBanner := "[✓] CONFIRMED"
	if !r.CanRunCloud {
		accessBanner = "[!] BLOCKED / HELD"
	}

	sb.WriteString("[+] Google Antigravity (AGY) Access Confirmation:\n")
	sb.WriteString(fmt.Sprintf("    • AGY CLI Engine:    %s\n", cliInfo))
	sb.WriteString(fmt.Sprintf("    • Active Model:      %s\n", r.AGYStatus.ActiveModel))
	sb.WriteString(fmt.Sprintf("    • Gateway Base URL:  %s\n", r.AGYStatus.GatewayBaseURL))
	sb.WriteString(fmt.Sprintf("    • Night Window Tier: %t\n", r.AGYStatus.InNightWindow))
	sb.WriteString(fmt.Sprintf("    • Reserve Floor Hold:%t\n", r.AGYStatus.InReserveFloor))
	if r.AGYStatus.ProbeSuccess {
		sb.WriteString(fmt.Sprintf("    • Live Probe:        SUCCESS (latency: %v)\n", r.AGYStatus.ProbeLatency.Round(time.Millisecond)))
	}
	sb.WriteString(fmt.Sprintf("    • Access Status:     %s - %s\n", accessBanner, r.AGYStatus.StatusReason))

	return sb.String()
}

func formatNumber(n int64) string {
	in := fmt.Sprintf("%d", n)
	if len(in) <= 3 {
		return in
	}
	var out []string
	rem := len(in) % 3
	if rem > 0 {
		out = append(out, in[:rem])
	}
	for i := rem; i < len(in); i += 3 {
		out = append(out, in[i:i+3])
	}
	return strings.Join(out, ",")
}
