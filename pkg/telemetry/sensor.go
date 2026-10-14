package telemetry

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var tempRegex = regexp.MustCompile(`(?i)(?:die temperature|soc temperature|cpu die|gpu die):\s*([0-9.]+)\s*C`)

// MockThermalSensor provides a thread-safe synthetic thermal sensor for testing and non-Apple environments.
type MockThermalSensor struct {
	mu   sync.RWMutex
	temp float64
}

// NewMockThermalSensor creates a new MockThermalSensor initialized with startingTemp.
func NewMockThermalSensor(startingTemp float64) *MockThermalSensor {
	return &MockThermalSensor{temp: startingTemp}
}

// SetTemperature updates the simulated SoC temperature.
func (m *MockThermalSensor) SetTemperature(t float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.temp = t
}

// GetSoCTemperature retrieves the current simulated temperature.
func (m *MockThermalSensor) GetSoCTemperature(_ context.Context) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.temp, nil
}

// ParsePowermetricsOutput extracts SoC or CPU die temperature from powermetrics standard output.
func ParsePowermetricsOutput(output string) float64 {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := tempRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			if parsed, err := strconv.ParseFloat(matches[1], 64); err == nil {
				return parsed
			}
		}
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

// StaticNodePinger provides simulated or configured node status pings.
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
