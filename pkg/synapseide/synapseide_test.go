package synapseide

import (
	"Dendrite/pkg/synapse"
	"encoding/json"
	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeMemberProjectID(t *testing.T, dir, projectID string) {
	t.Helper()
	dendriteDir := filepath.Join(dir, ".dendrite")
	if err := os.MkdirAll(dendriteDir, 0755); err != nil {
		t.Fatalf("failed to create .dendrite dir: %v", err)
	}
	idFile := filepath.Join(dendriteDir, "project.id")
	if err := os.WriteFile(idFile, []byte(projectID+"\n"), 0644); err != nil {
		t.Fatalf("failed to write project.id file: %v", err)
	}
}

func TestSynapseServerName(t *testing.T) {
	cases := []struct {
		id       string
		expected string
	}{
		{"core", "dendrite-synapse-core"},
		{"payment-service", "dendrite-synapse-payment-service"},
		{"mesh-1-v2", "dendrite-synapse-mesh-1-v2"},
	}

	for _, tc := range cases {
		got := SynapseServerName(tc.id)
		if got != tc.expected {
			t.Errorf("SynapseServerName(%q) = %q; expected %q", tc.id, got, tc.expected)
		}
	}
}

func TestSynapseScopeName(t *testing.T) {
	cases := []struct {
		id       string
		expected string
	}{
		{"core", "Synapse_core"},
		{"payment-service", "Synapse_payment_service"},
		{"multi-part-id-test", "Synapse_multi_part_id_test"},
	}

	for _, tc := range cases {
		got := SynapseScopeName(tc.id)
		if got != tc.expected {
			t.Errorf("SynapseScopeName(%q) = %q; expected %q", tc.id, got, tc.expected)
		}
	}
}

func TestSynapseScopeXML_ProducerMember(t *testing.T) {
	s := &synapse.Synapse{
		ID: "user-synapse",
		Members: []synapse.Member{
			{Alias: "auth-srv", Role: synapse.RoleProducer},
			{Alias: "web-client", Role: synapse.RoleConsumer},
		},
		Contracts: []synapse.Contract{
			{
				Kind:     "proto",
				Producer: "auth-srv",
				Path:     "proto/auth.proto",
				Consumers: []synapse.Consumer{
					{Alias: "web-client", Path: "src/proto/auth.proto"},
				},
			},
		},
	}

	xml, ok := SynapseScopeXML(s, "auth-srv")
	if !ok {
		t.Fatalf("expected SynapseScopeXML to return ok=true for producer member")
	}

	expectedXML := "<component name=\"DependencyValidationManager\">\n  <scope name=\"Synapse_user_synapse\" pattern=\"file:proto/auth.proto\" />\n</component>\n"
	if xml != expectedXML {
		t.Errorf("unexpected XML:\ngot:\n%s\nwant:\n%s", xml, expectedXML)
	}
}

func TestSynapseScopeXML_ConsumerMember(t *testing.T) {
	s := &synapse.Synapse{
		ID: "user-synapse",
		Members: []synapse.Member{
			{Alias: "auth-srv", Role: synapse.RoleProducer},
			{Alias: "web-client", Role: synapse.RoleConsumer},
		},
		Contracts: []synapse.Contract{
			{
				Kind:     "proto",
				Producer: "auth-srv",
				Path:     "proto/auth.proto",
				Consumers: []synapse.Consumer{
					{Alias: "web-client", Path: "src/proto/auth.proto"},
				},
			},
		},
	}

	xml, ok := SynapseScopeXML(s, "web-client")
	if !ok {
		t.Fatalf("expected SynapseScopeXML to return ok=true for consumer member")
	}

	expectedXML := "<component name=\"DependencyValidationManager\">\n  <scope name=\"Synapse_user_synapse\" pattern=\"file:src/proto/auth.proto\" />\n</component>\n"
	if xml != expectedXML {
		t.Errorf("unexpected XML:\ngot:\n%s\nwant:\n%s", xml, expectedXML)
	}
}

func TestSynapseScopeXML_MemberWithNoContracts(t *testing.T) {
	s := &synapse.Synapse{
		ID: "user-synapse",
		Members: []synapse.Member{
			{Alias: "auth-srv", Role: synapse.RoleProducer},
			{Alias: "unrelated", Role: synapse.RolePeer},
		},
		Contracts: []synapse.Contract{
			{
				Kind:     "proto",
				Producer: "auth-srv",
				Path:     "proto/auth.proto",
			},
		},
	}

	xml, ok := SynapseScopeXML(s, "unrelated")
	if ok || xml != "" {
		t.Errorf("expected ok=false and empty xml for member with no contracts, got xml=%q, ok=%v", xml, ok)
	}
}

func TestSynapseScopeXML_NilOrEmpty(t *testing.T) {
	if xml, ok := SynapseScopeXML(nil, "auth-srv"); ok || xml != "" {
		t.Errorf("expected false for nil synapse")
	}

	s := &synapse.Synapse{ID: "test"}
	if xml, ok := SynapseScopeXML(s, ""); ok || xml != "" {
		t.Errorf("expected false for empty alias")
	}

	// No contracts
	if xml, ok := SynapseScopeXML(s, "member"); ok || xml != "" {
		t.Errorf("expected false for synapse with no contracts")
	}

	// Contract with empty path
	sEmptyContract := &synapse.Synapse{
		ID: "test",
		Contracts: []synapse.Contract{
			{Producer: "auth-srv", Path: ""},
			{Producer: "other", Path: "x", Consumers: []synapse.Consumer{{Alias: "auth-srv", Path: ""}}},
		},
	}
	if xml, ok := SynapseScopeXML(sEmptyContract, "auth-srv"); ok || xml != "" {
		t.Errorf("expected false for empty contract paths")
	}
}

func TestSynapseScopeXML_Escaping(t *testing.T) {
	s := &synapse.Synapse{
		ID: "test-id",
		Contracts: []synapse.Contract{
			{
				Producer: "srv",
				Path:     "proto/foo&bar\"baz.proto",
			},
		},
	}

	xml, ok := SynapseScopeXML(s, "srv")
	if !ok {
		t.Fatalf("expected ok=true")
	}

	// Expect & to be &amp; and " to be &#34;
	expectedPattern := "file:proto/foo&amp;bar&#34;baz.proto"
	if !strings.Contains(xml, "pattern=\""+expectedPattern+"\"") {
		t.Errorf("XML pattern did not properly escape & and \": %s", xml)
	}
}

func TestSynapseScopeXML_MultipleContractsAndDuplicates(t *testing.T) {
	s := &synapse.Synapse{
		ID: "mesh",
		Contracts: []synapse.Contract{
			{
				Producer: "peer1",
				Path:     "contracts/a.json",
			},
			{
				Producer: "peer2",
				Path:     "contracts/b.json",
				Consumers: []synapse.Consumer{
					{Alias: "peer1", Path: "contracts/b.json"},
				},
			},
			{
				Producer: "peer1",
				Path:     "contracts/a.json", // duplicate path should be ignored
			},
		},
	}

	xml, ok := SynapseScopeXML(s, "peer1")
	if !ok {
		t.Fatalf("expected ok=true")
	}

	expectedPattern := "file:contracts/a.json||file:contracts/b.json"
	if !strings.Contains(xml, "pattern=\""+expectedPattern+"\"") {
		t.Errorf("expected joined pattern %q, got XML:\n%s", expectedPattern, xml)
	}
}

func TestSyncSynapseIDE_Integration(t *testing.T) {
	dir1 := t.TempDir()
	pid1 := "dpid_0123456789abcdef0123456789abcdef"
	writeMemberProjectID(t, dir1, pid1)

	// Pre-create .idea/scopes/Other.xml in dir1
	otherScopeDir := filepath.Join(dir1, ".idea", "scopes")
	if err := os.MkdirAll(otherScopeDir, 0755); err != nil {
		t.Fatalf("failed to create otherScopeDir: %v", err)
	}
	otherScopePath := filepath.Join(otherScopeDir, "Other.xml")
	otherScopeContent := "<component name=\"DependencyValidationManager\">\n  <scope name=\"Other\" pattern=\"file:existing/*\" />\n</component>\n"
	if err := os.WriteFile(otherScopePath, []byte(otherScopeContent), 0644); err != nil {
		t.Fatalf("failed to write Other.xml: %v", err)
	}

	dir2 := t.TempDir()
	pid2 := "dpid_fedcba9876543210fedcba9876543210"
	writeMemberProjectID(t, dir2, pid2)

	// dir3 does not exist -> unresolved member
	dir3 := filepath.Join(t.TempDir(), "nonexistent-dir")
	pid3 := "dpid_11112222333344445555666677778888"

	syn := &synapse.Synapse{
		ID:   "demo-synapse",
		Name: "Demo Synapse",
		Members: []synapse.Member{
			{ProjectID: pid1, Alias: "producer-repo", Role: synapse.RoleProducer, PathHint: dir1},
			{ProjectID: pid2, Alias: "consumer-repo", Role: synapse.RoleConsumer, PathHint: dir2},
			{ProjectID: pid3, Alias: "unresolved-repo", Role: synapse.RolePeer, PathHint: dir3},
		},
		Contracts: []synapse.Contract{
			{
				Kind:     "proto",
				Producer: "producer-repo",
				Path:     "contracts/api.proto",
				Consumers: []synapse.Consumer{
					{Alias: "consumer-repo", Path: "client/api.proto"},
				},
			},
		},
	}

	binaryPath := "/usr/local/bin/dendrite"
	written, skipped, err := SyncSynapseIDE(syn, binaryPath)
	if err != nil {
		t.Fatalf("SyncSynapseIDE failed: %v", err)
	}

	// 1. Verify skipped contains unresolved-repo
	if len(skipped) != 1 || skipped[0] != "unresolved-repo" {
		t.Fatalf("expected skipped to be ['unresolved-repo'], got %+v", skipped)
	}

	// 2. Verify Other.xml in dir1 is untouched
	otherData, err := os.ReadFile(otherScopePath)
	if err != nil {
		t.Fatalf("failed to read Other.xml: %v", err)
	}
	if string(otherData) != otherScopeContent {
		t.Fatalf("Other.xml was modified! Got:\n%s", string(otherData))
	}

	// 3. Verify written files count and sorted order
	expectedWritten := []string{
		filepath.Join(dir1, ".ai", "mcp", "mcp.json"),
		filepath.Join(dir1, ".idea", "scopes", "Synapse_demo_synapse.xml"),
		filepath.Join(dir2, ".ai", "mcp", "mcp.json"),
		filepath.Join(dir2, ".idea", "scopes", "Synapse_demo_synapse.xml"),
	}
	sort.Strings(expectedWritten)

	if !sort.StringsAreSorted(written) {
		t.Errorf("written files are not sorted: %+v", written)
	}
	if len(written) != len(expectedWritten) {
		t.Fatalf("expected %d written files, got %d: %+v", len(expectedWritten), len(written), written)
	}
	for i := range written {
		if written[i] != expectedWritten[i] {
			t.Errorf("written[%d] = %s, want %s", i, written[i], expectedWritten[i])
		}
	}

	// 4. Verify scope XML content in dir1
	scope1Data, err := os.ReadFile(filepath.Join(dir1, ".idea", "scopes", "Synapse_demo_synapse.xml"))
	if err != nil {
		t.Fatalf("failed to read scope1 file: %v", err)
	}
	if !strings.Contains(string(scope1Data), "pattern=\"file:contracts/api.proto\"") {
		t.Errorf("unexpected scope1 pattern:\n%s", string(scope1Data))
	}

	// 5. Verify scope XML content in dir2
	scope2Data, err := os.ReadFile(filepath.Join(dir2, ".idea", "scopes", "Synapse_demo_synapse.xml"))
	if err != nil {
		t.Fatalf("failed to read scope2 file: %v", err)
	}
	if !strings.Contains(string(scope2Data), "pattern=\"file:client/api.proto\"") {
		t.Errorf("unexpected scope2 pattern:\n%s", string(scope2Data))
	}

	// 6. Verify mcp.json in dir1
	mcp1Data, err := os.ReadFile(filepath.Join(dir1, ".ai", "mcp", "mcp.json"))
	if err != nil {
		t.Fatalf("failed to read mcp1 file: %v", err)
	}
	var mcp1Cfg jetbrains.MCPConfig
	if err := json.Unmarshal(mcp1Data, &mcp1Cfg); err != nil {
		t.Fatalf("invalid json in mcp1: %v", err)
	}
	srv1, ok := mcp1Cfg.MCPServers["dendrite-synapse-demo-synapse"]
	if !ok {
		t.Fatalf("missing synapse server entry in mcp1: %+v", mcp1Cfg.MCPServers)
	}
	if srv1.Command != binaryPath {
		t.Errorf("expected command %s, got %s", binaryPath, srv1.Command)
	}
	if len(srv1.Args) != 4 || srv1.Args[0] != "inter" || srv1.Args[1] != "serve" || srv1.Args[2] != "--synapse" || srv1.Args[3] != "demo-synapse" {
		t.Errorf("unexpected args: %v", srv1.Args)
	}
	if len(srv1.Env) != 0 {
		t.Errorf("scoping is carried by --synapse; expected no env, got %v", srv1.Env)
	}
}

func TestSyncSynapseIDE_MemberWithNoContracts(t *testing.T) {
	dir := t.TempDir()
	pid := "dpid_1234567890abcdef1234567890abcdef"
	writeMemberProjectID(t, dir, pid)

	syn := &synapse.Synapse{
		ID: "no-contracts-syn",
		Members: []synapse.Member{
			{ProjectID: pid, Alias: "standalone", Role: synapse.RolePeer, PathHint: dir},
		},
		Contracts: []synapse.Contract{}, // no contracts
	}

	written, skipped, err := SyncSynapseIDE(syn, "/bin/dendrite")
	if err != nil {
		t.Fatalf("SyncSynapseIDE failed: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("expected 0 skipped, got %v", skipped)
	}

	// Scope XML file should NOT have been created
	scopePath := filepath.Join(dir, ".idea", "scopes", "Synapse_no_contracts_syn.xml")
	if _, err := os.Stat(scopePath); !os.IsNotExist(err) {
		t.Fatalf("scope file should not exist when SynapseScopeXML is not ok")
	}

	// MCP file should have been written
	mcpPath := filepath.Join(dir, ".ai", "mcp", "mcp.json")
	if len(written) != 1 || written[0] != mcpPath {
		t.Fatalf("expected only mcp.json in written, got %+v", written)
	}
	if _, err := os.Stat(mcpPath); err != nil {
		t.Fatalf("expected mcp.json to exist: %v", err)
	}
}

func TestSyncSynapseIDE_NilOrInvalidSynapse(t *testing.T) {
	if _, _, err := SyncSynapseIDE(nil, "bin"); err == nil {
		t.Fatalf("expected error for nil synapse")
	}

	if _, _, err := SyncSynapseIDE(&synapse.Synapse{ID: ""}, "bin"); err == nil {
		t.Fatalf("expected error for empty synapse id")
	}

	// Empty members
	written, skipped, err := SyncSynapseIDE(&synapse.Synapse{ID: "valid-id"}, "bin")
	if err != nil {
		t.Fatalf("unexpected error for empty members: %v", err)
	}
	if len(written) != 0 || len(skipped) != 0 {
		t.Fatalf("expected empty written and skipped")
	}
}

func TestSyncSynapseIDE_WriteScopeError(t *testing.T) {
	dir := t.TempDir()
	pid := "dpid_1234567890abcdef1234567890abcdef"
	writeMemberProjectID(t, dir, pid)

	// Create .idea/scopes/Synapse_error_syn.xml as a directory so writing file fails
	conflictPath := filepath.Join(dir, ".idea", "scopes", "Synapse_error_syn.xml")
	if err := os.MkdirAll(conflictPath, 0755); err != nil {
		t.Fatalf("failed to create conflict dir: %v", err)
	}

	syn := &synapse.Synapse{
		ID: "error-syn",
		Members: []synapse.Member{
			{ProjectID: pid, Alias: "prod", Role: synapse.RoleProducer, PathHint: dir},
		},
		Contracts: []synapse.Contract{
			{Producer: "prod", Path: "file.proto"},
		},
	}

	_, _, err := SyncSynapseIDE(syn, "bin")
	if err == nil {
		t.Fatalf("expected error when writing scope file fails")
	}
}

func TestSyncSynapseIDE_UpsertMCPError(t *testing.T) {
	dir := t.TempDir()
	pid := "dpid_1234567890abcdef1234567890abcdef"
	writeMemberProjectID(t, dir, pid)

	// Create .ai/mcp/mcp.json as a directory so jetbrains.UpsertMCPServer fails
	conflictPath := filepath.Join(dir, ".ai", "mcp", "mcp.json")
	if err := os.MkdirAll(conflictPath, 0755); err != nil {
		t.Fatalf("failed to create conflict dir: %v", err)
	}

	syn := &synapse.Synapse{
		ID: "error-syn2",
		Members: []synapse.Member{
			{ProjectID: pid, Alias: "prod", Role: synapse.RoleProducer, PathHint: dir},
		},
	}

	_, _, err := SyncSynapseIDE(syn, "bin")
	if err == nil {
		t.Fatalf("expected error when upserting mcp fails")
	}
}
