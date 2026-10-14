package telemetry

import (
	"context"
	"time"
)

// ThermalSensor abstracts SoC die thermal measurement.
type ThermalSensor interface {
	GetSoCTemperature(ctx context.Context) (float64, error)
}

// SpendProvider abstracts 24h and 7d token financial metrics.
type SpendProvider interface {
	GetTokenSpend(ctx context.Context) (spend24h, spend7d, dailyBudget float64, err error)
}

// NodeHealthChecker abstracts reachability and latency health checks across mesh nodes.
type NodeHealthChecker interface {
	PingNodes(ctx context.Context) map[string]string
}

// DaemonConfig holds settings for the background telemetry polling daemon.
type DaemonConfig struct {
	Interval      time.Duration `yaml:"interval" json:"interval"`
	WarningTempC  float64       `yaml:"warning_temp_c" json:"warning_temp_c"`   // Defaults to 80.0C
	CriticalTempC float64       `yaml:"critical_temp_c" json:"critical_temp_c"` // Defaults to 85.0C
}

// TelemetryReport aggregates instantaneous hardware and spend telemetry.
type TelemetryReport struct {
	Timestamp       time.Time         `json:"timestamp"`
	SoCTempC        float64           `json:"soc_temp_c"`
	ThermalAlert    bool              `json:"thermal_alert"`
	ThermalCritical bool              `json:"thermal_critical"`
	Spend24hUSD     float64           `json:"spend_24h_usd"`
	Spend7dUSD      float64           `json:"spend_7d_usd"`
	DailyBudgetUSD  float64           `json:"daily_budget_usd"`
	NodeStatus      map[string]string `json:"node_status"`
}
