// Package penpotstack probes the local Penpot stack (frontend, backend, exporter).
package penpotstack

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"Dendrite/pkg/env"
)

// Probe is the reachability of one Penpot service.
type Probe struct {
	Name    string `json:"name"`
	Addr    string `json:"addr"`
	Healthy bool   `json:"healthy"`
}

// Check probes the three services a Penpot client needs: the frontend (HTTP 200), the backend
// that answers /api/rpc/command/* and the exporter used for asset and SVG export (both by TCP, since
// neither serves a page).
func Check(ctx context.Context, host string, cfg env.PenpotConfig, timeout time.Duration) []Probe {
	cfg = cfg.WithDefaults()
	frontend := fmt.Sprintf("%s:%d", host, cfg.FrontendPort)
	return []Probe{
		{Name: "frontend", Addr: frontend, Healthy: env.CheckPenpotHealth(ctx, "http://"+frontend, timeout)},
		tcpProbe(ctx, "backend", fmt.Sprintf("%s:%d", host, cfg.BackendPort), timeout),
		tcpProbe(ctx, "exporter", fmt.Sprintf("%s:%d", host, cfg.ExporterPort), timeout),
	}
}

// Healthy reports whether every probed service is reachable.
func Healthy(probes []Probe) bool {
	for _, p := range probes {
		if !p.Healthy {
			return false
		}
	}
	return len(probes) > 0
}

// Summary renders one line per probe for `motor health`.
func Summary(probes []Probe) string {
	var b strings.Builder
	for _, p := range probes {
		status := "UP"
		if !p.Healthy {
			status = "DOWN"
		}
		fmt.Fprintf(&b, "    - %-9s %-22s %s\n", p.Name, p.Addr, status)
	}
	return b.String()
}

func tcpProbe(ctx context.Context, name, addr string, timeout time.Duration) Probe {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err == nil {
		_ = conn.Close()
	}
	return Probe{Name: name, Addr: addr, Healthy: err == nil}
}
