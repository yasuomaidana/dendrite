package telemetry

import (
	"context"
	"sync"
	"time"
)

// Daemon coordinates hardware telemetry sampling, spend budget tracking, and alert dispatching.
type Daemon struct {
	cfg           DaemonConfig
	thermalSensor ThermalSensor
	spendProvider SpendProvider
	nodePinger    NodeHealthChecker
	onReport      func(TelemetryReport)
	stopCh        chan struct{}
	mu            sync.Mutex
	running       bool
}

// NewDaemon constructs a new telemetry Daemon instance.
func NewDaemon(
	cfg DaemonConfig,
	thermal ThermalSensor,
	spend SpendProvider,
	pinger NodeHealthChecker,
	onReport func(TelemetryReport),
) *Daemon {
	if cfg.Interval <= 0 {
		cfg.Interval = 1 * time.Second
	}
	if cfg.WarningTempC <= 0 {
		cfg.WarningTempC = 80.0
	}
	if cfg.CriticalTempC <= 0 {
		cfg.CriticalTempC = 85.0
	}
	if thermal == nil {
		thermal = NewHostThermalSensor()
	}
	if spend == nil {
		spend = NewFileSpendProvider("", 50.0)
	}
	if pinger == nil {
		pinger = NewStaticNodePinger(nil)
	}

	return &Daemon{
		cfg:           cfg,
		thermalSensor: thermal,
		spendProvider: spend,
		nodePinger:    pinger,
		onReport:      onReport,
		stopCh:        make(chan struct{}),
	}
}

// CollectOnce executes an instantaneous telemetry sweep across all providers.
func (d *Daemon) CollectOnce(ctx context.Context) TelemetryReport {
	temp, err := d.thermalSensor.GetSoCTemperature(ctx)
	if err != nil {
		temp = 45.0
	}

	spend24h, spend7d, budget, _ := d.spendProvider.GetTokenSpend(ctx)
	nodes := d.nodePinger.PingNodes(ctx)

	alert := temp >= d.cfg.WarningTempC
	critical := temp >= d.cfg.CriticalTempC

	return TelemetryReport{
		Timestamp:       time.Now().UTC(),
		SoCTempC:        temp,
		ThermalAlert:    alert,
		ThermalCritical: critical,
		Spend24hUSD:     spend24h,
		Spend7dUSD:      spend7d,
		DailyBudgetUSD:  budget,
		NodeStatus:      nodes,
	}
}

// Start initiates periodic background polling until ctx is canceled or Stop() is invoked.
func (d *Daemon) Start(ctx context.Context) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	ticker := time.NewTicker(d.cfg.Interval)
	defer ticker.Stop()

	// Initial immediate collection
	report := d.CollectOnce(ctx)
	if d.onReport != nil {
		d.onReport(report)
	}

	for {
		select {
		case <-ctx.Done():
			d.mu.Lock()
			d.running = false
			d.mu.Unlock()
			return
		case <-d.stopCh:
			d.mu.Lock()
			d.running = false
			d.mu.Unlock()
			return
		case <-ticker.C:
			rep := d.CollectOnce(ctx)
			if d.onReport != nil {
				d.onReport(rep)
			}
		}
	}
}

// Stop terminates background polling gracefully.
func (d *Daemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		select {
		case <-d.stopCh:
		default:
			close(d.stopCh)
		}
		d.running = false
	}
}
