package server

import (
	"errors"
	"time"
)

var (
	ErrServerClosed       = errors.New("server already closed")
	ErrInvalidPort        = errors.New("invalid port number")
	ErrUnauthorizedAccess = errors.New("unauthorized request: invalid token")
)

// ServerConfig configures the embedded HTTP and WebSocket control server.
type ServerConfig struct {
	Host        string        `yaml:"host" json:"host"`
	Port        int           `yaml:"port" json:"port"`
	AuthToken   string        `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`
}

// TelemetryFrame represents a real-time system and AI operational metrics snapshot.
type TelemetryFrame struct {
	Timestamp       time.Time         `json:"timestamp"`
	ActiveScenario  string            `json:"active_scenario"`
	ActiveWorkspace string            `json:"active_workspace"`
	SoCTemperatureC float64           `json:"soc_temperature_c"`
	ThermalAlert    bool              `json:"thermal_alert"` // True if temp > 80.0C
	TokenSpend24h   float64           `json:"token_spend_24h_usd"`
	TokenSpend7d    float64           `json:"token_spend_7d_usd"`
	DailyBudgetUSD  float64           `json:"daily_budget_usd"`
	NodeHealth      map[string]string `json:"node_health"`
}
