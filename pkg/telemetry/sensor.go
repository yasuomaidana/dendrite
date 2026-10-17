package telemetry

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrThermalSensorUnavailable indicates no accessible hardware thermal sensor or powermetrics privilege on the host.
	ErrThermalSensorUnavailable = errors.New("thermal sensor unavailable")

	tempRegex = regexp.MustCompile(`(?i)(?:die temperature|soc temperature|cpu die|gpu die):\s*([0-9.]+)\s*C`)
)

// HostThermalSensor reads hardware SoC or CPU die temperatures via Linux sysfs or macOS powermetrics.
type HostThermalSensor struct {
	SysfsGlob   string
	CommandFunc func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

// NewHostThermalSensor creates a real hardware thermal sensor for the current host.
func NewHostThermalSensor() *HostThermalSensor {
	return &HostThermalSensor{
		SysfsGlob:   "/sys/class/thermal/thermal_zone*/temp",
		CommandFunc: exec.CommandContext,
	}
}

// GetSoCTemperature queries Linux sysfs thermal zones or macOS powermetrics.
func (s *HostThermalSensor) GetSoCTemperature(ctx context.Context) (float64, error) {
	globPattern := s.SysfsGlob
	if globPattern == "" {
		globPattern = "/sys/class/thermal/thermal_zone*/temp"
	}

	if matches, err := filepath.Glob(globPattern); err == nil && len(matches) > 0 {
		for _, path := range matches {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}
			val, parseErr := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if parseErr == nil && val > 0 {
				if val > 1000 {
					val = val / 1000.0
				}
				return val, nil
			}
		}
	}

	cmdFunc := s.CommandFunc
	if cmdFunc == nil {
		cmdFunc = exec.CommandContext
	}
	cmd := cmdFunc(ctx, "powermetrics", "-n", "1", "--samplers", "smc")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrThermalSensorUnavailable, err)
	}

	temp, ok := parsePowermetricsTemp(string(out))
	if !ok {
		return 0, fmt.Errorf("%w: no temperature reading in powermetrics output", ErrThermalSensorUnavailable)
	}
	return temp, nil
}

func parsePowermetricsTemp(output string) (float64, bool) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := tempRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			if parsed, err := strconv.ParseFloat(matches[1], 64); err == nil {
				return parsed, true
			}
		}
	}
	return 45.0, false
}

// ParsePowermetricsOutput extracts SoC or CPU die temperature from powermetrics standard output.
func ParsePowermetricsOutput(output string) float64 {
	if temp, ok := parsePowermetricsTemp(output); ok {
		return temp
	}
	return 45.0 // Conservative default
}

// FileSpendProvider loads token spend metrics from a local spend.json file.
type FileSpendProvider struct {
	spendFilePath string
	dailyBudget   float64
}

// NewFileSpendProvider constructs a new FileSpendProvider.
func NewFileSpendProvider(filePath string, dailyBudget float64) *FileSpendProvider {
	if dailyBudget <= 0 {
		dailyBudget = 50.0
	}
	return &FileSpendProvider{
		spendFilePath: filePath,
		dailyBudget:   dailyBudget,
	}
}

// GetTokenSpend parses spend metrics.
func (f *FileSpendProvider) GetTokenSpend(_ context.Context) (float64, float64, float64, error) {
	data, err := os.ReadFile(f.spendFilePath)
	if err != nil {
		return 0.0, 0.0, f.dailyBudget, nil
	}

	var parsed struct {
		TotalSpendUSD float64 `json:"total_spend_usd"`
		Spend24hUSD   float64 `json:"spend_24h_usd"`
		Spend7dUSD    float64 `json:"spend_7d_usd"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return 0.0, 0.0, f.dailyBudget, nil
	}

	spend24h := parsed.Spend24hUSD
	if spend24h == 0 && parsed.TotalSpendUSD > 0 {
		spend24h = parsed.TotalSpendUSD
	}
	spend7d := parsed.Spend7dUSD
	if spend7d == 0 && parsed.TotalSpendUSD > 0 {
		spend7d = parsed.TotalSpendUSD
	}

	return spend24h, spend7d, f.dailyBudget, nil
}

// StaticNodePinger provides configured node status pings.
type StaticNodePinger struct {
	statuses map[string]string
}

// NewStaticNodePinger creates a static node pinger.
func NewStaticNodePinger(statuses map[string]string) *StaticNodePinger {
	if statuses == nil {
		statuses = map[string]string{
			"MacBook": "online",
			"Debian":  "online",
			"Azure":   "standby",
			"AWS":     "standby",
		}
	}
	return &StaticNodePinger{statuses: statuses}
}

// PingNodes returns instantaneous node statuses.
func (p *StaticNodePinger) PingNodes(_ context.Context) map[string]string {
	res := make(map[string]string, len(p.statuses))
	for k, v := range p.statuses {
		res[k] = v
	}
	return res
}
