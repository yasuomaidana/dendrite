package topology_test

import (
	"strings"
	"testing"

	"Dendrite/pkg/topology"
)

func TestScenarios(t *testing.T) {
	all := topology.AllScenarios()
	if len(all) != 4 {
		t.Fatalf("expected 4 scenarios, got %d", len(all))
	}

	validCases := []string{
		"local-hybrid",
		"azure-burst",
		"aws-burst",
		"distributed",
		"LOCAL-HYBRID",
		"  azure-burst  ",
	}

	for _, tc := range validCases {
		sc, err := topology.ParseScenario(tc)
		if err != nil {
			t.Errorf("expected '%s' to parse successfully, got error: %v", tc, err)
		}
		if !sc.IsValid() {
			t.Errorf("expected scenario '%s' to be valid", sc)
		}
	}

	invalidCases := []string{"", "cloud-burst", "unknown", "docker-local"}
	for _, tc := range invalidCases {
		_, err := topology.ParseScenario(tc)
		if err == nil {
			t.Errorf("expected error parsing invalid scenario '%s', got nil", tc)
		}
		if topology.Scenario(tc).IsValid() {
			t.Errorf("expected Scenario('%s').IsValid() to be false", tc)
		}
	}
}

func TestComputeNodes(t *testing.T) {
	nodes := topology.AllComputeNodes()
	if len(nodes) != 4 {
		t.Fatalf("expected 4 compute nodes, got %d", len(nodes))
	}

	validNodes := []string{"debian", "macbook", "azure", "aws", "DEBIAN", "  aws "}
	for _, vn := range validNodes {
		node, err := topology.ParseComputeNode(vn)
		if err != nil {
			t.Errorf("expected '%s' to parse successfully, got: %v", vn, err)
		}
		if !node.IsValid() {
			t.Errorf("expected '%s' to be valid", node)
		}
	}

	invalidNodes := []string{"", "windows", "gcp", "unknown"}
	for _, in := range invalidNodes {
		_, err := topology.ParseComputeNode(in)
		if err == nil {
			t.Errorf("expected error parsing invalid node '%s', got nil", in)
		}
		if topology.ComputeNode(in).IsValid() {
			t.Errorf("expected ComputeNode('%s').IsValid() to be false", in)
		}
	}
}

func TestStackComponents(t *testing.T) {
	comps := topology.AllStackComponents()
	if len(comps) != 6 {
		t.Fatalf("expected 6 stack components, got %d", len(comps))
	}

	validComps := []string{
		"control_plane", "inference", "linux_re", "repo_intel", "docs_portal", "database",
	}
	for _, vc := range validComps {
		c, err := topology.ParseStackComponent(vc)
		if err != nil {
			t.Errorf("expected '%s' to parse successfully, got: %v", vc, err)
		}
		if !c.IsValid() {
			t.Errorf("expected '%s' to be valid", c)
		}
	}

	invalidComps := []string{"", "monitoring", "frontend"}
	for _, ic := range invalidComps {
		_, err := topology.ParseStackComponent(ic)
		if err == nil {
			t.Errorf("expected error parsing invalid component '%s', got nil", ic)
		}
		if topology.StackComponent(ic).IsValid() {
			t.Errorf("expected StackComponent('%s').IsValid() to be false", ic)
		}
	}
}

func TestDefaultPlacement(t *testing.T) {
	scenarios := []topology.Scenario{
		topology.ScenarioLocalHybrid,
		topology.ScenarioAzureBurst,
		topology.ScenarioAWSBurst,
		topology.ScenarioDistributed,
	}

	for _, sc := range scenarios {
		placement, err := topology.DefaultPlacement(sc)
		if err != nil {
			t.Fatalf("unexpected error getting default placement for %s: %v", sc, err)
		}
		if len(placement) != 6 {
			t.Errorf("expected 6 components placed for %s, got %d", sc, len(placement))
		}
	}

	// Invalid scenario
	_, err := topology.DefaultPlacement("nonexistent")
	if err == nil {
		t.Errorf("expected error for invalid scenario in DefaultPlacement")
	}
}

func TestNewTopologyConfig_Valid(t *testing.T) {
	endpoints := topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	}

	cfg, err := topology.NewTopologyConfig(topology.ScenarioLocalHybrid, endpoints)
	if err != nil {
		t.Fatalf("failed to create valid topology config: %v", err)
	}

	if cfg.Scenario != topology.ScenarioLocalHybrid {
		t.Errorf("expected scenario %s, got %s", topology.ScenarioLocalHybrid, cfg.Scenario)
	}

	node, ok := cfg.GetNodeForComponent(topology.ComponentControlPlane)
	if !ok || node != topology.NodeDebian {
		t.Errorf("expected control_plane on debian, got %s (ok=%v)", node, ok)
	}

	comps := cfg.GetComponentsForNode(topology.NodeDebian)
	if len(comps) == 0 {
		t.Errorf("expected debian to have components, got 0")
	}

	profiles := cfg.ActiveComposeProfiles(topology.NodeDebian)
	if len(profiles) == 0 {
		t.Errorf("expected debian to have active compose profiles, got 0")
	}

	env := cfg.RenderEnv()
	if env["DENDRITE_SCENARIO"] != string(topology.ScenarioLocalHybrid) {
		t.Errorf("expected env DENDRITE_SCENARIO to be %s", topology.ScenarioLocalHybrid)
	}
	if env["TAILSCALE_CONTROL_PLANE_IP"] != "100.84.12.10" {
		t.Errorf("expected control plane IP 100.84.12.10, got %s", env["TAILSCALE_CONTROL_PLANE_IP"])
	}
}

func TestValidationErrors(t *testing.T) {
	// Nil config
	var nilCfg *topology.TopologyConfig
	if err := nilCfg.Validate(); err == nil {
		t.Errorf("expected error validating nil config")
	}

	// Invalid scenario
	badScenarioCfg := &topology.TopologyConfig{
		Scenario: "bad-scenario",
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
			topology.ComponentInference:    topology.NodeMacBook,
		},
	}
	if err := badScenarioCfg.Validate(); err == nil {
		t.Errorf("expected error for invalid scenario")
	}

	// Invalid endpoint host
	badHostCfg := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		TailscaleEndpoints: topology.TailscaleEndpoints{
			ControlPlane: "invalid host!with@symbols",
		},
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
			topology.ComponentInference:    topology.NodeMacBook,
		},
	}
	if err := badHostCfg.Validate(); err == nil {
		t.Errorf("expected error for invalid endpoint host")
	}

	// Empty placement
	emptyPlacementCfg := &topology.TopologyConfig{
		Scenario:       topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{},
	}
	if err := emptyPlacementCfg.Validate(); err == nil {
		t.Errorf("expected error for empty placement")
	}

	// Missing mandatory control_plane
	missingCP := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{
			topology.ComponentInference: topology.NodeMacBook,
		},
	}
	if err := missingCP.Validate(); err == nil {
		t.Errorf("expected error for missing control_plane")
	}

	// Missing mandatory inference
	missingInf := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
		},
	}
	if err := missingInf.Validate(); err == nil {
		t.Errorf("expected error for missing inference")
	}

	// Invalid component
	invalidComp := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
			topology.ComponentInference:    topology.NodeMacBook,
			"invalid_component":            topology.NodeDebian,
		},
	}
	if err := invalidComp.Validate(); err == nil {
		t.Errorf("expected error for invalid component key")
	}

	// Invalid node
	invalidNode := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: "invalid_node",
			topology.ComponentInference:    topology.NodeMacBook,
		},
	}
	if err := invalidNode.Validate(); err == nil {
		t.Errorf("expected error for invalid node value")
	}

	// Local-hybrid with cloud node
	cloudInLocal := &topology.TopologyConfig{
		Scenario: topology.ScenarioLocalHybrid,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
			topology.ComponentInference:    topology.NodeMacBook,
			topology.ComponentLinuxRE:      topology.NodeAzure,
		},
	}
	if err := cloudInLocal.Validate(); err == nil {
		t.Errorf("expected error for cloud node in local-hybrid")
	}

	// Azure-burst with debian node
	debianInAzure := &topology.TopologyConfig{
		Scenario: topology.ScenarioAzureBurst,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeAzure,
			topology.ComponentInference:    topology.NodeMacBook,
			topology.ComponentLinuxRE:      topology.NodeDebian,
		},
	}
	if err := debianInAzure.Validate(); err == nil {
		t.Errorf("expected error for debian node in azure-burst")
	}

	// Azure-burst with control plane on macbook instead of azure
	cpNotAzure := &topology.TopologyConfig{
		Scenario: topology.ScenarioAzureBurst,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeMacBook,
			topology.ComponentInference:    topology.NodeMacBook,
		},
	}
	if err := cpNotAzure.Validate(); err == nil {
		t.Errorf("expected error when control_plane not on azure in azure-burst")
	}

	// AWS-burst with no AWS node
	noAwsInAwsBurst := &topology.TopologyConfig{
		Scenario: topology.ScenarioAWSBurst,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeDebian,
			topology.ComponentInference:    topology.NodeMacBook,
			topology.ComponentLinuxRE:      topology.NodeDebian,
		},
	}
	if err := noAwsInAwsBurst.Validate(); err == nil {
		t.Errorf("expected error when aws-burst has no node on aws")
	}

	// Distributed with control plane not on debian
	distCPNotDebian := &topology.TopologyConfig{
		Scenario: topology.ScenarioDistributed,
		StackPlacement: topology.StackPlacement{
			topology.ComponentControlPlane: topology.NodeAzure,
			topology.ComponentInference:    topology.NodeMacBook,
			topology.ComponentLinuxRE:      topology.NodeAWS,
		},
	}
	if err := distCPNotDebian.Validate(); err == nil {
		t.Errorf("expected error when control_plane is not debian in distributed scenario")
	}
}

func TestJSONRoundtrip(t *testing.T) {
	endpoints := topology.TailscaleEndpoints{
		ControlPlane:   "100.64.0.1",
		ExecutionPlane: "100.64.0.2",
		AzureBurst:     "100.64.0.3",
	}
	cfg, err := topology.NewTopologyConfig(topology.ScenarioAzureBurst, endpoints)
	if err != nil {
		t.Fatalf("failed to create azure burst config: %v", err)
	}

	data, err := cfg.ToJSON()
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	loaded, err := topology.FromJSON(data)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if loaded.Scenario != cfg.Scenario {
		t.Errorf("expected scenario %s, got %s", cfg.Scenario, loaded.Scenario)
	}
	if loaded.TailscaleEndpoints.ControlPlane != endpoints.ControlPlane {
		t.Errorf("expected control plane %s, got %s", endpoints.ControlPlane, loaded.TailscaleEndpoints.ControlPlane)
	}

	// Invalid JSON
	_, err = topology.FromJSON([]byte("{invalid-json"))
	if err == nil {
		t.Errorf("expected error on invalid JSON")
	}
}

func TestYAMLRoundtrip(t *testing.T) {
	endpoints := topology.TailscaleEndpoints{
		ControlPlane:   "100.70.1.10",
		ExecutionPlane: "100.70.1.20",
		AWSBurst:       "100.70.1.30",
	}
	cfg, err := topology.NewTopologyConfig(topology.ScenarioAWSBurst, endpoints)
	if err != nil {
		t.Fatalf("failed to create aws burst config: %v", err)
	}

	data, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	loaded, err := topology.FromYAML(data)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if loaded.Scenario != cfg.Scenario {
		t.Errorf("expected scenario %s, got %s", cfg.Scenario, loaded.Scenario)
	}
	if loaded.TailscaleEndpoints.AWSBurst != endpoints.AWSBurst {
		t.Errorf("expected AWS burst %s, got %s", endpoints.AWSBurst, loaded.TailscaleEndpoints.AWSBurst)
	}

	// Invalid YAML
	_, err = topology.FromYAML([]byte(":::invalid yaml"))
	if err == nil {
		t.Errorf("expected error on invalid YAML")
	}
}

func TestValidateEndpointHost(t *testing.T) {
	validHosts := []string{
		"",
		"100.84.12.10",
		"127.0.0.1",
		"::1",
		"2001:db8::1",
		"localhost",
		"debian-node.ts.net",
		"macbook-pro.ts.net",
		"myhost.local",
	}

	for _, h := range validHosts {
		if err := topology.ValidateEndpointHost(h); err != nil {
			t.Errorf("expected host '%s' to be valid, got: %v", h, err)
		}
	}

	invalidHosts := []string{
		"http://100.84.12.10",
		"debian-node.com",
		"invalid space host",
	}

	for _, h := range invalidHosts {
		if err := topology.ValidateEndpointHost(h); err == nil {
			t.Errorf("expected host '%s' to fail validation", h)
		}
	}
}

func TestRenderEnvAndHelpers(t *testing.T) {
	var nilCfg *topology.TopologyConfig
	if env := nilCfg.RenderEnv(); env != nil {
		t.Errorf("expected nil env for nil config")
	}

	// Distributed scenario with burst endpoints
	cfg, err := topology.NewTopologyConfig(topology.ScenarioDistributed, topology.TailscaleEndpoints{
		ControlPlane:   "100.1.1.1",
		ExecutionPlane: "100.1.1.2",
		AzureBurst:     "100.1.1.3",
		AWSBurst:       "100.1.1.4",
	})
	if err != nil {
		t.Fatalf("failed to build distributed config: %v", err)
	}

	env := cfg.RenderEnv()
	if env["TAILSCALE_AZURE_BURST_IP"] != "100.1.1.3" {
		t.Errorf("expected azure burst env, got %v", env)
	}
	if env["TAILSCALE_AWS_BURST_IP"] != "100.1.1.4" {
		t.Errorf("expected aws burst env, got %v", env)
	}

	// Empty endpoints fallback to 127.0.0.1
	cfgEmptyEndpoints, _ := topology.NewTopologyConfig(topology.ScenarioDistributed, topology.TailscaleEndpoints{})
	emptyEnv := cfgEmptyEndpoints.RenderEnv()
	if emptyEnv["TAILSCALE_CONTROL_PLANE_IP"] != "127.0.0.1" {
		t.Errorf("expected default fallback 127.0.0.1, got %s", emptyEnv["TAILSCALE_CONTROL_PLANE_IP"])
	}

	// Nil receiver tests on methods
	if _, ok := nilCfg.GetNodeForComponent(topology.ComponentControlPlane); ok {
		t.Errorf("expected false for nil config GetNodeForComponent")
	}
	if comps := nilCfg.GetComponentsForNode(topology.NodeDebian); comps != nil {
		t.Errorf("expected nil components for nil config GetComponentsForNode")
	}
}

func TestValidationErrorUnwrap(t *testing.T) {
	vErr := &topology.ValidationError{
		Field:   "test_field",
		Message: "test message",
		Err:     topology.ErrInvalidScenario,
	}

	if !strings.Contains(vErr.Error(), "test_field") {
		t.Errorf("expected field name in error string")
	}
	if vErr.Unwrap() != topology.ErrInvalidScenario {
		t.Errorf("expected unwrapped error to match ErrInvalidScenario")
	}

	vErrNoCause := &topology.ValidationError{
		Field:   "field_only",
		Message: "some message",
	}
	if !strings.Contains(vErrNoCause.Error(), "field_only") {
		t.Errorf("expected field name in error string")
	}
}
