package server

import (
	"context"
	"sync"
	"time"

	"Dendrite/pkg/telemetry"
)

const (
	// frameStaleAfter is the age after which the hub's last frame no longer describes the host.
	frameStaleAfter = 30 * time.Second
	// thermalCacheTTL bounds how often an on-demand temperature read (macOS spawns powermetrics) runs.
	thermalCacheTTL = 30 * time.Second
	// thermalTimeout bounds one read.
	thermalTimeout = 3 * time.Second
)

// thermalCache serves the SoC temperature to snapshot requests, reading the sensor at most once per
// TTL (failures are cached too, so a host without a sensor is not probed on every request).
type thermalCache struct {
	mu     sync.Mutex
	sensor telemetry.ThermalSensor
	now    func() time.Time
	at     time.Time
	temp   float64
	err    error
}

func newThermalCache(sensor telemetry.ThermalSensor) *thermalCache {
	return &thermalCache{sensor: sensor, now: time.Now}
}

// read returns the cached reading, or a fresh one when it is older than the TTL. A zero or negative
// reading counts as a failure: it is not a temperature.
func (c *thermalCache) read(ctx context.Context) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && c.now().Sub(c.at) < thermalCacheTTL {
		return c.temp, c.err
	}
	ctx, cancel := context.WithTimeout(ctx, thermalTimeout)
	defer cancel()
	temp, err := c.sensor.GetSoCTemperature(ctx)
	if err == nil && temp <= 0 {
		err = telemetry.ErrThermalSensorUnavailable
	}
	c.at, c.temp, c.err = c.now(), temp, err
	return temp, err
}
