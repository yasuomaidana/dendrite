package exporter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/telemetry"
)

// PrometheusExporter formats and exposes Dendrite telemetry as Prometheus metrics.
type PrometheusExporter struct {
	daemon         *telemetry.Daemon
	gatewayReqs    map[string]int64
	fallbackEvents map[string]int64
	customGauges   map[string]float64
	activeSessions int
	mu             sync.RWMutex
}

// NewPrometheusExporter creates a new exporter instance bound to a telemetry daemon.
func NewPrometheusExporter(daemon *telemetry.Daemon) *PrometheusExporter {
	return &PrometheusExporter{
		daemon:         daemon,
		gatewayReqs:    make(map[string]int64),
		fallbackEvents: make(map[string]int64),
		customGauges:   make(map[string]float64),
	}
}

// RecordGatewayRequest increments the request counter for a given inference engine and response status.
func (e *PrometheusExporter) RecordGatewayRequest(engine, status string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%s|%s", engine, status)
	e.gatewayReqs[key]++
}

// RecordFallbackEvent increments the cascading fallback counter when a failover occurs.
func (e *PrometheusExporter) RecordFallbackEvent(fromEngine, toEngine string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%s|%s", fromEngine, toEngine)
	e.fallbackEvents[key]++
}

// SetActiveSessions updates the number of currently running agent sessions.
func (e *PrometheusExporter) SetActiveSessions(count int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeSessions = count
}

// SetCustomGauge sets an arbitrary named gauge metric value.
func (e *PrometheusExporter) SetCustomGauge(name string, value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.customGauges[name] = value
}

// WriteMetrics writes standard Prometheus text exposition format (0.0.4) to the writer.
func (e *PrometheusExporter) WriteMetrics(ctx context.Context, w io.Writer) error {
	start := time.Now()

	var report telemetry.TelemetryReport
	if e.daemon != nil {
		report = e.daemon.CollectOnce(ctx)
	}

	e.mu.RLock()
	activeSessions := e.activeSessions
	reqsCopy := make(map[string]int64, len(e.gatewayReqs))
	for k, v := range e.gatewayReqs {
		reqsCopy[k] = v
	}
	fallbacksCopy := make(map[string]int64, len(e.fallbackEvents))
	for k, v := range e.fallbackEvents {
		fallbacksCopy[k] = v
	}
	customCopy := make(map[string]float64, len(e.customGauges))
	for k, v := range e.customGauges {
		customCopy[k] = v
	}
	e.mu.RUnlock()

	var b strings.Builder

	// Header comments and SoC temperature
	b.WriteString("# HELP dendrite_soc_temperature_celsius Apple Silicon or host die temperature in Celsius.\n")
	b.WriteString("# TYPE dendrite_soc_temperature_celsius gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_soc_temperature_celsius %.2f\n\n", report.SoCTempC))

	// Thermal alerts
	b.WriteString("# HELP dendrite_thermal_alert_status Instantaneous thermal warning flag (1 = warning, 0 = normal).\n")
	b.WriteString("# TYPE dendrite_thermal_alert_status gauge\n")
	alertVal := 0
	if report.ThermalAlert {
		alertVal = 1
	}
	b.WriteString(fmt.Sprintf("dendrite_thermal_alert_status %d\n\n", alertVal))

	b.WriteString("# HELP dendrite_thermal_critical_status Instantaneous thermal critical throttle flag (1 = critical, 0 = normal).\n")
	b.WriteString("# TYPE dendrite_thermal_critical_status gauge\n")
	criticalVal := 0
	if report.ThermalCritical {
		criticalVal = 1
	}
	b.WriteString(fmt.Sprintf("dendrite_thermal_critical_status %d\n\n", criticalVal))

	// Spend metrics
	b.WriteString("# HELP dendrite_spend_24h_usd Cumulative token spend over the last 24 hours in USD.\n")
	b.WriteString("# TYPE dendrite_spend_24h_usd gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_spend_24h_usd %.4f\n\n", report.Spend24hUSD))

	b.WriteString("# HELP dendrite_spend_7d_usd Cumulative token spend over the last 7 days in USD.\n")
	b.WriteString("# TYPE dendrite_spend_7d_usd gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_spend_7d_usd %.4f\n\n", report.Spend7dUSD))

	b.WriteString("# HELP dendrite_daily_budget_usd Configured daily financial token budget ceiling in USD.\n")
	b.WriteString("# TYPE dendrite_daily_budget_usd gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_daily_budget_usd %.2f\n\n", report.DailyBudgetUSD))

	// Active agent sessions
	b.WriteString("# HELP dendrite_active_agent_sessions Number of currently running autonomous agent coding sessions.\n")
	b.WriteString("# TYPE dendrite_active_agent_sessions gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_active_agent_sessions %d\n\n", activeSessions))

	// Node status
	b.WriteString("# HELP dendrite_node_reachable Status of mesh compute nodes (1 = reachable/healthy, 0 = unreachable).\n")
	b.WriteString("# TYPE dendrite_node_reachable gauge\n")
	if len(report.NodeStatus) > 0 {
		nodes := make([]string, 0, len(report.NodeStatus))
		for k := range report.NodeStatus {
			nodes = append(nodes, k)
		}
		sort.Strings(nodes)
		for _, node := range nodes {
			st := report.NodeStatus[node]
			reachable := 0
			if strings.EqualFold(st, "reachable") || strings.EqualFold(st, "healthy") || strings.EqualFold(st, "online") {
				reachable = 1
			}
			b.WriteString(fmt.Sprintf("dendrite_node_reachable{node=\"%s\",status=\"%s\"} %d\n", node, st, reachable))
		}
	} else {
		b.WriteString("dendrite_node_reachable{node=\"localhost\",status=\"healthy\"} 1\n")
	}
	b.WriteString("\n")

	// Gateway request counters
	b.WriteString("# HELP dendrite_gateway_requests_total Total number of inference gateway requests routed.\n")
	b.WriteString("# TYPE dendrite_gateway_requests_total counter\n")
	if len(reqsCopy) > 0 {
		keys := make([]string, 0, len(reqsCopy))
		for k := range reqsCopy {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts := strings.SplitN(k, "|", 2)
			engine, status := parts[0], parts[1]
			b.WriteString(fmt.Sprintf("dendrite_gateway_requests_total{engine=\"%s\",status=\"%s\"} %d\n", engine, status, reqsCopy[k]))
		}
	} else {
		b.WriteString("dendrite_gateway_requests_total{engine=\"default\",status=\"200\"} 0\n")
	}
	b.WriteString("\n")

	// Cascade fallback counters
	b.WriteString("# HELP dendrite_gateway_fallback_events_total Total cascade fallback events triggered by engine latency or errors.\n")
	b.WriteString("# TYPE dendrite_gateway_fallback_events_total counter\n")
	if len(fallbacksCopy) > 0 {
		keys := make([]string, 0, len(fallbacksCopy))
		for k := range fallbacksCopy {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts := strings.SplitN(k, "|", 2)
			fromEngine, toEngine := parts[0], parts[1]
			b.WriteString(fmt.Sprintf("dendrite_gateway_fallback_events_total{from=\"%s\",to=\"%s\"} %d\n", fromEngine, toEngine, fallbacksCopy[k]))
		}
	} else {
		b.WriteString("dendrite_gateway_fallback_events_total{from=\"none\",to=\"none\"} 0\n")
	}
	b.WriteString("\n")

	// Custom gauges
	if len(customCopy) > 0 {
		b.WriteString("# HELP dendrite_custom_metric Custom application gauge.\n")
		b.WriteString("# TYPE dendrite_custom_metric gauge\n")
		keys := make([]string, 0, len(customCopy))
		for k := range customCopy {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("dendrite_custom_metric{name=\"%s\"} %.4f\n", k, customCopy[k]))
		}
		b.WriteString("\n")
	}

	// Scrape duration
	durationSec := time.Since(start).Seconds()
	b.WriteString("# HELP dendrite_exporter_scrape_duration_seconds Scrape duration of the Prometheus exporter.\n")
	b.WriteString("# TYPE dendrite_exporter_scrape_duration_seconds gauge\n")
	b.WriteString(fmt.Sprintf("dendrite_exporter_scrape_duration_seconds %.6f\n", durationSec))

	_, err := io.WriteString(w, b.String())
	return err
}

// ServeHTTP satisfies http.Handler for exposing the /metrics endpoint.
func (e *PrometheusExporter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = e.WriteMetrics(r.Context(), w)
}

// Handler returns the HTTP handler for Prometheus scraping.
func (e *PrometheusExporter) Handler() http.Handler {
	return e
}
