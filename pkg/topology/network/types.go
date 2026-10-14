package network

import (
	"errors"
)

// MeshType defines the underlying mesh or overlay network technology.
type MeshType string

const (
	MeshTailscale  MeshType = "tailscale"
	MeshCloudflare MeshType = "cloudflare"
	MeshSSHTunnel  MeshType = "ssh-tunnel"
	MeshWireGuard  MeshType = "wireguard"
	MeshDirectLAN  MeshType = "direct-lan"
)

var (
	ErrInvalidConfig   = errors.New("network provider: invalid configuration")
	ErrProviderUnknown = errors.New("network provider: unknown provider type")
	ErrTunnelFailed    = errors.New("network provider: tunnel execution failed")
)

// NetworkProvider abstracts zero-trust overlays, port forwarders, and direct LAN topologies.
type NetworkProvider interface {
	// Type returns the mesh provider technology type.
	Type() MeshType
	// Validate audits configuration sanity before bootstrapping.
	Validate() error
	// BootstrapPackages returns Linux packages required for the provider.
	BootstrapPackages() []string
	// BootstrapCommands returns system initialization shell commands for cloud-init.
	BootstrapCommands(hostname string) []string
	// ResolveEndpoint returns the routable host or IP:port for a target service.
	ResolveEndpoint(serviceName string, defaultPort int) string
}
