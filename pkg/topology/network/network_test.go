package network

import (
	"strings"
	"testing"
)

func TestTailscaleProvider(t *testing.T) {
	p := &TailscaleProvider{
		AuthKey:        "tskey-auth-mock-12345",
		EnableSSH:      true,
		ControlPlaneIP: "100.64.0.1",
		Routes: map[string]string{
			"custom-db": "100.64.0.99:5432",
		},
	}

	if p.Type() != MeshTailscale {
		t.Errorf("expected MeshTailscale, got: %s", p.Type())
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	pkgs := p.BootstrapPackages()
	if len(pkgs) == 0 {
		t.Error("expected non-empty bootstrap packages")
	}

	cmds := p.BootstrapCommands("node-alpha")
	hasSSHFlag := false
	for _, cmd := range cmds {
		if strings.Contains(cmd, "--ssh") {
			hasSSHFlag = true
		}
	}
	if !hasSSHFlag {
		t.Error("expected --ssh in tailscale up command")
	}

	epDB := p.ResolveEndpoint("custom-db", 5432)
	if epDB != "100.64.0.99:5432" {
		t.Errorf("expected routed endpoint, got: %s", epDB)
	}

	epDef := p.ResolveEndpoint("web", 3456)
	if epDef != "100.64.0.1:3456" {
		t.Errorf("expected control plane endpoint, got: %s", epDef)
	}

	// Validate error on empty config
	empty := &TailscaleProvider{}
	if err := empty.Validate(); err == nil {
		t.Error("expected validation error for empty tailscale provider")
	}
}

func TestCloudflareProvider(t *testing.T) {
	p := &CloudflareProvider{
		TunnelToken: "eyJhIjoiY2xvdWRmbGFyZS10b2tlbiJ9",
		TunnelID:    "cf-tunnel-987",
		Domain:      "dendrite.mesh",
		IngressRules: []CloudflareIngressRule{
			{Hostname: "api.dendrite.mesh", Service: "http://localhost:3456"},
			{Hostname: "plane.dendrite.mesh", Service: "http://localhost:8090"},
		},
	}

	if p.Type() != MeshCloudflare {
		t.Errorf("expected MeshCloudflare, got: %s", p.Type())
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	cmds := p.BootstrapCommands("cloud-vm")
	if len(cmds) < 2 {
		t.Errorf("expected install and service commands, got: %v", cmds)
	}

	cfgStr, err := p.GenerateIngressConfig()
	if err != nil {
		t.Fatalf("failed to generate ingress config: %v", err)
	}
	if !strings.Contains(cfgStr, "api.dendrite.mesh") || !strings.Contains(cfgStr, "http_status:404") {
		t.Errorf("ingress config missing expected rules:\n%s", cfgStr)
	}

	ep := p.ResolveEndpoint("api", 3456)
	if ep != "api.dendrite.mesh" {
		t.Errorf("expected api.dendrite.mesh, got: %s", ep)
	}

	epFallback := p.ResolveEndpoint("unknown", 8080)
	if epFallback != "unknown.dendrite.mesh" {
		t.Errorf("expected unknown.dendrite.mesh, got: %s", epFallback)
	}

	// Validate error on empty
	empty := &CloudflareProvider{}
	if err := empty.Validate(); err == nil {
		t.Error("expected validation error for empty cloudflare provider")
	}
}

func TestSSHTunnelProvider(t *testing.T) {
	p := &SSHTunnelProvider{
		Host:    "192.168.1.50",
		SSHPort: 2222,
		User:    "admin",
		KeyPath: "/root/.ssh/id_rsa",
		ForwardRules: []PortForwardRule{
			{LocalPort: 4000, RemoteHost: "127.0.0.1", RemotePort: 4000, IsReverse: false},
			{LocalPort: 3456, RemoteHost: "127.0.0.1", RemotePort: 3456, IsReverse: true},
		},
	}

	if p.Type() != MeshSSHTunnel {
		t.Errorf("expected MeshSSHTunnel, got: %s", p.Type())
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmds := p.BootstrapCommands("remote-host")
	if len(cmds) == 0 {
		t.Error("expected bootstrap commands")
	}

	args := p.BuildForwardArgs()
	argsStr := strings.Join(args, " ")
	if !strings.Contains(argsStr, "-L 4000:127.0.0.1:4000") || !strings.Contains(argsStr, "-R 3456:127.0.0.1:3456") {
		t.Errorf("expected -L and -R arguments, got: %s", argsStr)
	}

	ep := p.ResolveEndpoint("litellm", 4000)
	if ep != "127.0.0.1:4000" {
		t.Errorf("expected 127.0.0.1:4000, got: %s", ep)
	}

	// Validation error
	empty := &SSHTunnelProvider{}
	if err := empty.Validate(); err == nil {
		t.Error("expected error for empty ssh provider host")
	}
}

func TestWireGuardProvider(t *testing.T) {
	p := &WireGuardProvider{
		InterfaceName: "wg0",
		Address:       "10.8.0.2/24",
		PrivateKey:    "aW50ZXJmYWNlX3ByaXZhdGVfa2V5X21vY2s=",
		ListenPort:    51820,
		Peers: []WireGuardPeer{
			{
				PublicKey:           "cGVlcl9wdWJsaWNfa2V5X21vY2s=",
				Endpoint:            "203.0.113.5:51820",
				AllowedIPs:          []string{"10.8.0.1/32"},
				PersistentKeepalive: 25,
			},
		},
	}

	if p.Type() != MeshWireGuard {
		t.Errorf("expected MeshWireGuard, got: %s", p.Type())
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkgs := p.BootstrapPackages()
	if len(pkgs) < 2 {
		t.Errorf("expected wireguard tools in packages, got: %v", pkgs)
	}

	cfg := p.GenerateWGQuickConfig()
	if !strings.Contains(cfg, "[Interface]") || !strings.Contains(cfg, "Address = 10.8.0.2/24") || !strings.Contains(cfg, "[Peer]") {
		t.Errorf("invalid wireguard config output:\n%s", cfg)
	}

	ep := p.ResolveEndpoint("gateway", 4000)
	if ep != "10.8.0.1:4000" {
		t.Errorf("expected 10.8.0.1:4000, got: %s", ep)
	}

	empty := &WireGuardProvider{}
	if err := empty.Validate(); err == nil {
		t.Error("expected error for empty address")
	}
}

func TestDirectLANProvider(t *testing.T) {
	p := &DirectLANProvider{
		Subnet:   "192.168.1.0/24",
		StaticIP: "192.168.1.100",
		Routes: map[string]string{
			"db": "192.168.1.50:5432",
		},
	}

	if p.Type() != MeshDirectLAN {
		t.Errorf("expected MeshDirectLAN, got: %s", p.Type())
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmds := p.BootstrapCommands("local-box")
	if len(cmds) == 0 {
		t.Error("expected commands")
	}

	epDB := p.ResolveEndpoint("db", 5432)
	if epDB != "192.168.1.50:5432" {
		t.Errorf("expected db endpoint, got: %s", epDB)
	}

	epStatic := p.ResolveEndpoint("other", 9000)
	if epStatic != "192.168.1.100:9000" {
		t.Errorf("expected 192.168.1.100:9000, got: %s", epStatic)
	}

	empty := &DirectLANProvider{}
	if err := empty.Validate(); err == nil {
		t.Error("expected error for empty subnet and static IP")
	}
}

func TestManager(t *testing.T) {
	mgr := NewManager()

	if mgr.ActiveType() != MeshDirectLAN {
		t.Errorf("expected default active provider DirectLAN, got: %s", mgr.ActiveType())
	}

	ts := &TailscaleProvider{
		AuthKey:        "ts-test-key",
		ControlPlaneIP: "100.80.0.1",
	}
	mgr.Register(ts)

	if err := mgr.SetActive(MeshTailscale); err != nil {
		t.Fatalf("failed to switch to tailscale: %v", err)
	}

	if mgr.ActiveType() != MeshTailscale {
		t.Errorf("expected active type MeshTailscale, got: %s", mgr.ActiveType())
	}

	if mgr.ActiveProvider().Type() != MeshTailscale {
		t.Errorf("expected active provider MeshTailscale")
	}

	// Try unknown provider
	if err := mgr.SetActive(MeshWireGuard); err == nil {
		t.Error("expected error switching to unregistered provider")
	}
}
