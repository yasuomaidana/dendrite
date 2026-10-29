package topology_test

import (
	"strings"
	"testing"

	"Dendrite/pkg/topology"
)

type enumTestCase[T interface {
	~string
	IsValid() bool
}] struct {
	groupName       string
	invalidItemName string
	typeName        string
	all             []T
	expectedLen     int
	parse           func(string) (T, error)
	validCases      []string
	invalidCases    []string
}

func testEnumCases[T interface {
	~string
	IsValid() bool
}](t *testing.T, tc enumTestCase[T]) {
	t.Helper()
	if len(tc.all) != tc.expectedLen {
		t.Fatalf("expected %d %s, got %d", tc.expectedLen, tc.groupName, len(tc.all))
	}

	for _, c := range tc.validCases {
		val, err := tc.parse(c)
		if err != nil {
			t.Errorf("expected '%s' to parse successfully, got: %v", c, err)
		}
		if !val.IsValid() {
			t.Errorf("expected '%s' to be valid", val)
		}
	}

	for _, c := range tc.invalidCases {
		_, err := tc.parse(c)
		if err == nil {
			t.Errorf("expected error parsing invalid %s '%s', got nil", tc.invalidItemName, c)
		}
		if T(c).IsValid() {
			t.Errorf("expected %s('%s').IsValid() to be false", tc.typeName, c)
		}
	}
}

func TestScenarios(t *testing.T) {
	testEnumCases(t, enumTestCase[topology.Scenario]{
		groupName:       "scenarios",
		invalidItemName: "scenario",
		typeName:        "Scenario",
		all:             topology.AllScenarios(),
		expectedLen:     4,
		parse:           topology.ParseScenario,
		validCases: []string{
			"local-hybrid",
			"azure-burst",
			"aws-burst",
			"distributed",
			"LOCAL-HYBRID",
			"  azure-burst  ",
		},
		invalidCases: []string{"", "cloud-burst", "unknown", "docker-local"},
	})
}

func TestComputeNodes(t *testing.T) {
	testEnumCases(t, enumTestCase[topology.ComputeNode]{
		groupName:       "compute nodes",
		invalidItemName: "node",
		typeName:        "ComputeNode",
		all:             topology.AllComputeNodes(),
		expectedLen:     4,
		parse:           topology.ParseComputeNode,
		validCases:      []string{"debian", "macbook", "azure", "aws", "DEBIAN", "  aws "},
		invalidCases:    []string{"", "windows", "gcp", "unknown"},
	})
}

func TestStackComponents(t *testing.T) {
	testEnumCases(t, enumTestCase[topology.StackComponent]{
		groupName:       "stack components",
		invalidItemName: "component",
		typeName:        "StackComponent",
		all:             topology.AllStackComponents(),
		expectedLen:     6,
		parse:           topology.ParseStackComponent,
		validCases: []string{
			"control_plane", "inference", "linux_re", "repo_intel", "docs_portal", "database",
		},
		invalidCases: []string{"", "monitoring", "frontend"},
	})
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

func runConfigRoundtripTest(
	t *testing.T,
	scenario topology.Scenario,
	scenarioDesc string,
	formatName string,
	endpoints topology.TailscaleEndpoints,
	marshal func(*topology.TopologyConfig) ([]byte, error),
	unmarshal func([]byte) (*topology.TopologyConfig, error),
	invalidData []byte,
	assertEndpoint func(t *testing.T, loaded *topology.TopologyConfig),
) {
	t.Helper()
	cfg, err := topology.NewTopologyConfig(scenario, endpoints)
	if err != nil {
		t.Fatalf("failed to create %s config: %v", scenarioDesc, err)
	}

	data, err := marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal %s: %v", formatName, err)
	}

	loaded, err := unmarshal(data)
	if err != nil {
		t.Fatalf("failed to unmarshal %s: %v", formatName, err)
	}

	if loaded.Scenario != cfg.Scenario {
		t.Errorf("expected scenario %s, got %s", cfg.Scenario, loaded.Scenario)
	}
	assertEndpoint(t, loaded)

	// Invalid format
	_, err = unmarshal(invalidData)
	if err == nil {
		t.Errorf("expected error on invalid %s", formatName)
	}
}

func TestConfigRoundtrip(t *testing.T) {
	jsonEndpoints := topology.TailscaleEndpoints{ControlPlane: "100.64.0.1", ExecutionPlane: "100.64.0.2", AzureBurst: "100.64.0.3"}
	yamlEndpoints := topology.TailscaleEndpoints{ControlPlane: "100.70.1.10", ExecutionPlane: "100.70.1.20", AWSBurst: "100.70.1.30"}
	t.Run("JSON", func(t *testing.T) {
		runConfigRoundtripTest(t, topology.ScenarioAzureBurst, "azure burst", "JSON", jsonEndpoints,
			(*topology.TopologyConfig).ToJSON, topology.FromJSON, []byte("{invalid-json"),
			func(t *testing.T, loaded *topology.TopologyConfig) {
				if got := loaded.TailscaleEndpoints.ControlPlane; got != jsonEndpoints.ControlPlane {
					t.Errorf("expected control plane %s, got %s", jsonEndpoints.ControlPlane, got)
				}
			})
	})
	t.Run("YAML", func(t *testing.T) {
		runConfigRoundtripTest(t, topology.ScenarioAWSBurst, "aws burst", "YAML", yamlEndpoints,
			(*topology.TopologyConfig).ToYAML, topology.FromYAML, []byte(":::invalid yaml"),
			func(t *testing.T, loaded *topology.TopologyConfig) {
				if got := loaded.TailscaleEndpoints.AWSBurst; got != yamlEndpoints.AWSBurst {
					t.Errorf("expected AWS burst %s, got %s", yamlEndpoints.AWSBurst, got)
				}
			})
	})
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
