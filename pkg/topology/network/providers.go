package network

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// TailscaleProvider implements NetworkProvider for Tailscale overlay networks.
type TailscaleProvider struct {
	AuthKey         string            `yaml:"auth_key" json:"auth_key"`
	EnableSSH       bool              `yaml:"enable_ssh" json:"enable_ssh"`
	ControlPlaneIP  string            `yaml:"control_plane_ip" json:"control_plane_ip"`
	ExecutionNodeIP string            `yaml:"execution_node_ip" json:"execution_node_ip"`
	Routes          map[string]string `yaml:"routes" json:"routes"`
}

func (p *TailscaleProvider) Type() MeshType {
	return MeshTailscale
}

func (p *TailscaleProvider) Validate() error {
	if strings.TrimSpace(p.AuthKey) == "" && strings.TrimSpace(p.ControlPlaneIP) == "" {
		return fmt.Errorf("%w: tailscale requires auth_key or control_plane_ip", ErrInvalidConfig)
	}
	return nil
}

func (p *TailscaleProvider) BootstrapPackages() []string {
	return []string{"curl", "iptables"}
}

func (p *TailscaleProvider) BootstrapCommands(hostname string) []string {
	cmds := []string{
		"curl -fsSL https://tailscale.com/install.sh | sh",
		"systemctl enable --now tailscaled",
	}
	if p.AuthKey != "" {
		upCmd := fmt.Sprintf("tailscale up --authkey=%s --hostname=%s", p.AuthKey, hostname)
		if p.EnableSSH {
			upCmd += " --ssh"
		}
		cmds = append(cmds, upCmd)
	}
	return cmds
}

func resolveHostEndpoint(routes map[string]string, defaultHost string, serviceName string, defaultPort int) string {
	if routes != nil {
		if target, ok := routes[serviceName]; ok {
			return target
		}
	}
	if defaultHost != "" {
		return fmt.Sprintf("%s:%d", defaultHost, defaultPort)
	}
	return fmt.Sprintf("127.0.0.1:%d", defaultPort)
}

func (p *TailscaleProvider) ResolveEndpoint(serviceName string, defaultPort int) string {
	return resolveHostEndpoint(p.Routes, p.ControlPlaneIP, serviceName, defaultPort)
}

// CloudflareIngressRule defines hostname to local port routing for cloudflared.
type CloudflareIngressRule struct {
	Hostname string `yaml:"hostname" json:"hostname"`
	Service  string `yaml:"service" json:"service"`
}

// CloudflareProvider implements NetworkProvider for Cloudflare Zero Trust Tunnels.
type CloudflareProvider struct {
	TunnelToken  string                  `yaml:"tunnel_token" json:"tunnel_token"`
	TunnelID     string                  `yaml:"tunnel_id" json:"tunnel_id"`
	AccountID    string                  `yaml:"account_id" json:"account_id"`
	Domain       string                  `yaml:"domain" json:"domain"`
	IngressRules []CloudflareIngressRule `yaml:"ingress" json:"ingress"`
}

func (p *CloudflareProvider) Type() MeshType {
	return MeshCloudflare
}

func (p *CloudflareProvider) Validate() error {
	if strings.TrimSpace(p.TunnelToken) == "" && strings.TrimSpace(p.TunnelID) == "" {
		return fmt.Errorf("%w: cloudflare provider requires tunnel_token or tunnel_id", ErrInvalidConfig)
	}
	return nil
}

func (p *CloudflareProvider) BootstrapPackages() []string {
	return []string{"curl"}
}

func (p *CloudflareProvider) BootstrapCommands(hostname string) []string {
	cmds := []string{
		"curl -L --output cloudflared.deb https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb",
		"dpkg -i cloudflared.deb && rm cloudflared.deb",
	}
	if p.TunnelToken != "" {
		cmds = append(cmds, fmt.Sprintf("cloudflared service install %s", p.TunnelToken))
		cmds = append(cmds, "systemctl enable --now cloudflared")
	}
	return cmds
}

func (p *CloudflareProvider) GenerateIngressConfig() (string, error) {
	rules := make([]map[string]string, 0, len(p.IngressRules)+1)
	for _, r := range p.IngressRules {
		rules = append(rules, map[string]string{
			"hostname": r.Hostname,
			"service":  r.Service,
		})
	}
	// Fallback 404 rule required by cloudflared
	rules = append(rules, map[string]string{
		"service": "http_status:404",
	})

	cfg := map[string]interface{}{
		"tunnel":  p.TunnelID,
		"ingress": rules,
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (p *CloudflareProvider) ResolveEndpoint(serviceName string, defaultPort int) string {
	for _, r := range p.IngressRules {
		if strings.Contains(strings.ToLower(r.Hostname), strings.ToLower(serviceName)) {
			return r.Hostname
		}
	}
	if p.Domain != "" {
		return fmt.Sprintf("%s.%s", serviceName, p.Domain)
	}
	return fmt.Sprintf("127.0.0.1:%d", defaultPort)
}

// PortForwardRule specifies local or remote port redirection over SSH.
type PortForwardRule struct {
	LocalPort  int    `yaml:"local_port" json:"local_port"`
	RemoteHost string `yaml:"remote_host" json:"remote_host"`
	RemotePort int    `yaml:"remote_port" json:"remote_port"`
	IsReverse  bool   `yaml:"is_reverse" json:"is_reverse"` // If true, remote listens and forwards back to local (-R)
}

// SSHTunnelProvider manages dynamic and reverse SSH port forwarding without third-party overlays.
type SSHTunnelProvider struct {
	Host         string            `yaml:"host" json:"host"`
	SSHPort      int               `yaml:"ssh_port" json:"ssh_port"`
	User         string            `yaml:"user" json:"user"`
	KeyPath      string            `yaml:"key_path" json:"key_path"`
	ForwardRules []PortForwardRule `yaml:"forward_rules" json:"forward_rules"`
}

func (p *SSHTunnelProvider) Type() MeshType {
	return MeshSSHTunnel
}

func (p *SSHTunnelProvider) Validate() error {
	if strings.TrimSpace(p.Host) == "" {
		return fmt.Errorf("%w: ssh-tunnel requires host", ErrInvalidConfig)
	}
	if p.SSHPort <= 0 {
		p.SSHPort = 22
	}
	if p.User == "" {
		p.User = "deployer"
	}
	return nil
}

func (p *SSHTunnelProvider) BootstrapPackages() []string {
	return []string{"openssh-server"}
}

func (p *SSHTunnelProvider) BootstrapCommands(hostname string) []string {
	return []string{
		"systemctl enable --now ssh",
		"sed -i 's/#GatewayPorts no/GatewayPorts clientspecified/g' /etc/ssh/sshd_config",
		"systemctl reload ssh || systemctl restart sshd",
	}
}

// BuildForwardArgs constructs command-line arguments for standard ssh CLI invocation.
func (p *SSHTunnelProvider) BuildForwardArgs() []string {
	args := []string{
		"-N", // Do not execute remote command
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-p", fmt.Sprintf("%d", p.SSHPort),
	}
	if p.KeyPath != "" {
		args = append(args, "-i", p.KeyPath)
	}
	for _, rule := range p.ForwardRules {
		rHost := rule.RemoteHost
		if rHost == "" {
			rHost = "127.0.0.1"
		}
		if rule.IsReverse {
			args = append(args, "-R", fmt.Sprintf("%d:%s:%d", rule.RemotePort, rHost, rule.LocalPort))
		} else {
			args = append(args, "-L", fmt.Sprintf("%d:%s:%d", rule.LocalPort, rHost, rule.RemotePort))
		}
	}
	args = append(args, fmt.Sprintf("%s@%s", p.User, p.Host))
	return args
}

func (p *SSHTunnelProvider) ResolveEndpoint(serviceName string, defaultPort int) string {
	for _, rule := range p.ForwardRules {
		if rule.RemotePort == defaultPort {
			return fmt.Sprintf("127.0.0.1:%d", rule.LocalPort)
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", defaultPort)
}

// WireGuardPeer represents a peer in a WireGuard network.
type WireGuardPeer struct {
	PublicKey           string   `yaml:"public_key" json:"public_key"`
	Endpoint            string   `yaml:"endpoint" json:"endpoint"`
	AllowedIPs          []string `yaml:"allowed_ips" json:"allowed_ips"`
	PersistentKeepalive int      `yaml:"persistent_keepalive" json:"persistent_keepalive"`
}

// WireGuardProvider implements NetworkProvider for standalone WireGuard VPN tunnels.
type WireGuardProvider struct {
	InterfaceName string          `yaml:"interface" json:"interface"` // Defaults to "wg0"
	Address       string          `yaml:"address" json:"address"`     // e.g. "10.0.0.2/24"
	PrivateKey    string          `yaml:"private_key" json:"private_key"`
	ListenPort    int             `yaml:"listen_port" json:"listen_port"`
	Peers         []WireGuardPeer `yaml:"peers" json:"peers"`
}

func (p *WireGuardProvider) Type() MeshType {
	return MeshWireGuard
}

func (p *WireGuardProvider) Validate() error {
	if strings.TrimSpace(p.Address) == "" {
		return fmt.Errorf("%w: wireguard requires address CIDR", ErrInvalidConfig)
	}
	if p.InterfaceName == "" {
		p.InterfaceName = "wg0"
	}
	return nil
}

func (p *WireGuardProvider) BootstrapPackages() []string {
	return []string{"wireguard", "wireguard-tools", "resolvconf"}
}

func (p *WireGuardProvider) BootstrapCommands(hostname string) []string {
	return []string{
		"systemctl enable --now systemd-resolved",
		fmt.Sprintf("systemctl enable --now wg-quick@%s", p.InterfaceName),
	}
}

// GenerateWGQuickConfig renders the /etc/wireguard/wg0.conf text format.
func (p *WireGuardProvider) GenerateWGQuickConfig() string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("Address = %s\n", p.Address))
	if p.PrivateKey != "" {
		b.WriteString(fmt.Sprintf("PrivateKey = %s\n", p.PrivateKey))
	}
	if p.ListenPort > 0 {
		b.WriteString(fmt.Sprintf("ListenPort = %d\n", p.ListenPort))
	}

	for _, peer := range p.Peers {
		b.WriteString("\n[Peer]\n")
		b.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))
		if peer.Endpoint != "" {
			b.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
		}
		if len(peer.AllowedIPs) > 0 {
			b.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", ")))
		}
		if peer.PersistentKeepalive > 0 {
			b.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", peer.PersistentKeepalive))
		}
	}

	return b.String()
}

func (p *WireGuardProvider) ResolveEndpoint(serviceName string, defaultPort int) string {
	for _, peer := range p.Peers {
		for _, ip := range peer.AllowedIPs {
			cleanIP := strings.Split(ip, "/")[0]
			return fmt.Sprintf("%s:%d", cleanIP, defaultPort)
		}
	}
	cleanAddr := strings.Split(p.Address, "/")[0]
	return fmt.Sprintf("%s:%d", cleanAddr, defaultPort)
}

// DirectLANProvider implements zero-overlay networking directly over local LAN subnets.
type DirectLANProvider struct {
	Subnet   string            `yaml:"subnet" json:"subnet"`
	Gateway  string            `yaml:"gateway" json:"gateway"`
	StaticIP string            `yaml:"static_ip" json:"static_ip"`
	DNS      []string          `yaml:"dns" json:"dns"`
	Routes   map[string]string `yaml:"routes" json:"routes"`
}

func (p *DirectLANProvider) Type() MeshType {
	return MeshDirectLAN
}

func (p *DirectLANProvider) Validate() error {
	if p.Subnet == "" && p.StaticIP == "" {
		return fmt.Errorf("%w: direct-lan requires subnet or static_ip", ErrInvalidConfig)
	}
	return nil
}

func (p *DirectLANProvider) BootstrapPackages() []string {
	return []string{"net-tools", "iproute2"}
}

func (p *DirectLANProvider) BootstrapCommands(hostname string) []string {
	return []string{
		"echo '[+] Direct LAN initialized for zero-overlay local routing'",
	}
}

func (p *DirectLANProvider) ResolveEndpoint(serviceName string, defaultPort int) string {
	return resolveHostEndpoint(p.Routes, p.StaticIP, serviceName, defaultPort)
}

// Manager orchestrates registered NetworkProviders and tracks the active mesh mode.
type Manager struct {
	providers map[MeshType]NetworkProvider
	active    MeshType
}

// NewManager initializes a Manager instance with default direct LAN and Tailscale providers.
func NewManager() *Manager {
	m := &Manager{
		providers: make(map[MeshType]NetworkProvider),
		active:    MeshDirectLAN,
	}
	m.Register(&DirectLANProvider{StaticIP: "127.0.0.1"})
	return m
}

// Register adds or overrides a provider for its MeshType.
func (m *Manager) Register(p NetworkProvider) {
	if p != nil {
		m.providers[p.Type()] = p
	}
}

// SetActive switches the active network provider.
func (m *Manager) SetActive(t MeshType) error {
	p, ok := m.providers[t]
	if !ok {
		return fmt.Errorf("%w: %s", ErrProviderUnknown, t)
	}
	if err := p.Validate(); err != nil {
		return err
	}
	m.active = t
	return nil
}

// ActiveProvider returns the currently active network provider.
func (m *Manager) ActiveProvider() NetworkProvider {
	if p, ok := m.providers[m.active]; ok {
		return p
	}
	return &DirectLANProvider{StaticIP: "127.0.0.1"}
}

// ActiveType returns the MeshType of the active provider.
func (m *Manager) ActiveType() MeshType {
	return m.active
}
