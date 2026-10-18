package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

// synapseFixture saves a two-member synapse whose members are real directories with project IDs.
func synapseFixture(t *testing.T) (*synapse.Store, map[string]string) {
	t.Helper()
	root := t.TempDir()
	ids := map[string]string{"api": "dpid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "web": "dpid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	dirs := map[string]string{}
	var members []synapse.Member
	for _, alias := range []string{"api", "web"} {
		dir := filepath.Join(root, alias)
		if err := os.MkdirAll(filepath.Join(dir, ".dendrite"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".dendrite", "project.id"), []byte(ids[alias]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dirs[alias] = dir
		members = append(members, synapse.Member{ProjectID: ids[alias], Alias: alias, Role: synapse.RolePeer, PathHint: dir})
	}
	st := synapse.NewStore(filepath.Join(root, "synapses"))
	err := st.Save(&synapse.Synapse{
		ID: "web-api", Name: "Web API", State: synapse.StateActive, Members: members,
		Contracts: []synapse.Contract{{Kind: "openapi", Producer: "api", Path: "openapi.yaml", Consumers: []synapse.Consumer{{Alias: "web", Path: "src/types.ts"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, dirs
}

func rpc(t *testing.T, s *Server, method string, params interface{}) *JSONRPCResponse {
	t.Helper()
	raw, _ := json.Marshal(params)
	req, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: raw})
	resp := s.HandleRequest(context.Background(), req)
	if resp == nil {
		t.Fatalf("%s: nil response", method)
	}
	return resp
}

func TestSynapseServerExposesOnlySynapseTools(t *testing.T) {
	st, _ := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	tools := s.ListTools()
	if len(tools) == 0 {
		t.Fatal("scoped server has no tools")
	}
	for _, tool := range tools {
		if !strings.HasPrefix(tool.Name, "synapse_") {
			t.Fatalf("scoped server leaked non-synapse tool %q", tool.Name)
		}
	}
	init := rpc(t, s, "initialize", map[string]interface{}{})
	if !strings.Contains(mustJSON(t, init.Result), "web-api") {
		t.Fatalf("server name should identify the synapse: %s", mustJSON(t, init.Result))
	}
}

func TestNewSynapseServerUnknownID(t *testing.T) {
	st, _ := synapseFixture(t)
	if _, err := NewSynapseServer(SynapseScope{Store: st, ID: "nope"}, "test"); err == nil {
		t.Fatal("expected error for unknown synapse")
	}
}

func TestSynapseInfo(t *testing.T) {
	st, dirs := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	call := func() map[string]interface{} {
		resp := rpc(t, s, "tools/call", CallToolParams{Name: "synapse_info", Arguments: map[string]interface{}{}})
		var res CallToolResult
		if err := json.Unmarshal([]byte(mustJSON(t, resp.Result)), &res); err != nil {
			t.Fatal(err)
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
			t.Fatalf("synapse_info text is not JSON: %v\n%s", err, res.Content[0].Text)
		}
		out["isError"] = res.IsError
		return out
	}

	out := call()
	if out["id"] != "web-api" || out["state"] != "active" {
		t.Fatalf("unexpected info: %v", out)
	}
	members := out["members"].([]interface{})
	if members[0].(map[string]interface{})["status"] != "OK" {
		t.Fatalf("member status = %v", members[0])
	}

	// Health is re-read on every call.
	if err := os.RemoveAll(dirs["web"]); err != nil {
		t.Fatal(err)
	}
	members = call()["members"].([]interface{})
	if members[1].(map[string]interface{})["status"] != "MISSING" {
		t.Fatalf("web status after removal = %v", members[1])
	}

	// Manifest deleted under a running server -> tool-level error, not a crash.
	if err := st.Delete("web-api"); err != nil {
		t.Fatal(err)
	}
	resp := rpc(t, s, "tools/call", CallToolParams{Name: "synapse_info", Arguments: map[string]interface{}{}})
	if !strings.Contains(mustJSON(t, resp.Result), `"isError":true`) {
		t.Fatalf("expected isError result, got %s", mustJSON(t, resp.Result))
	}
}

func TestConfineToMember(t *testing.T) {
	st, dirs := synapseFixture(t)
	syn, err := st.Load("web-api")
	if err != nil {
		t.Fatal(err)
	}
	ok := []struct{ alias, rel, want string }{
		{"api", "", dirs["api"]},
		{"api", "openapi.yaml", filepath.Join(dirs["api"], "openapi.yaml")},
		{"web", "src/../src/types.ts", filepath.Join(dirs["web"], "src", "types.ts")},
	}
	for _, c := range ok {
		got, err := ConfineToMember(syn, c.alias, c.rel)
		if err != nil || got != c.want {
			t.Fatalf("ConfineToMember(%s, %q) = %q, %v; want %q", c.alias, c.rel, got, err, c.want)
		}
	}
	bad := []struct{ alias, rel string }{
		{"nope", "x"},
		{"api", "/etc/passwd"},
		{"api", "../web/src/types.ts"},
		{"api", ".."},
	}
	for _, c := range bad {
		if _, err := ConfineToMember(syn, c.alias, c.rel); err == nil {
			t.Fatalf("ConfineToMember(%s, %q) should be rejected", c.alias, c.rel)
		}
	}
	if err := os.RemoveAll(dirs["api"]); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfineToMember(syn, "api", "x"); err == nil || !strings.Contains(err.Error(), "heal") {
		t.Fatalf("unresolved member should point at heal, got %v", err)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestJSONResultUnmarshalableValue(t *testing.T) {
	if _, err := jsonResult(map[string]interface{}{"c": make(chan int)}); err == nil {
		t.Fatal("expected marshal error for a channel value")
	}
}

func TestSynapseSyncStatus(t *testing.T) {
	st, dirs := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	call := func(args map[string]interface{}) CallToolResult {
		resp := rpc(t, s, "tools/call", CallToolParams{Name: "synapse_sync_status", Arguments: args})
		var res CallToolResult
		if err := json.Unmarshal([]byte(mustJSON(t, resp.Result)), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Fixture members are plain directories, not git checkouts: each reports an error.
	res := call(map[string]interface{}{"fetch": true})
	var rep synapse.SyncReport
	if err := json.Unmarshal([]byte(res.Content[0].Text), &rep); err != nil {
		t.Fatalf("not a sync report: %v\n%s", err, res.Content[0].Text)
	}
	if rep.Synapse != "web-api" || len(rep.Members) != len(dirs) || rep.Members[0].Error == "" {
		t.Fatalf("report = %+v", rep)
	}
	// Read-only: the manifest on disk is unchanged.
	if syn, _ := st.Load("web-api"); syn.State != synapse.StateActive {
		t.Fatalf("manifest state changed to %s", syn.State)
	}

	if err := st.Delete("web-api"); err != nil {
		t.Fatal(err)
	}
	if res := call(map[string]interface{}{}); !res.IsError {
		t.Fatal("expected isError after manifest deletion")
	}
}
