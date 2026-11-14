package catalog_test

import (
	"testing"

	. "Dendrite/pkg/mcpserver"
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/mcpserver/codetools"
	"Dendrite/pkg/mcpserver/mcptest"
	"Dendrite/pkg/mcpserver/opstools"
	"Dendrite/pkg/mcpserver/planetools"
)

func TestMain(m *testing.M) { mcptest.Main(m) }

// names lists the tools registered on a server.
func names(s *Server) map[string]bool {
	out := map[string]bool{}
	for _, t := range s.ListTools() {
		out[t.Name] = true
	}
	return out
}

func TestTheCatalogRegistersEveryToolPackageAndTheSynapsePeersTool(t *testing.T) {
	all := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(all, Dependencies{})
	got := names(all)

	for pkg, register := range map[string]func(*Server, Dependencies){"planetools": planetools.Register, "codetools": codetools.Register, "opstools": opstools.Register} {
		s := NewServer(ServerConfig{})
		register(s, Dependencies{DevStatus: "DEV_STATUS.md"})
		if len(names(s)) == 0 {
			t.Errorf("%s registers no tools", pkg)
		}
		for name := range names(s) {
			if !got[name] {
				t.Errorf("%s registers %s, which the catalog lacks", pkg, name)
			}
		}
	}
	if !got["dendrite_synapse_peers"] || !got["dendrite_rag_retrieve"] || !got["dendrite_ticket_claim"] || !got["dendrite_pool_list"] {
		t.Errorf("the catalog lacks a core tool: %d tools", len(got))
	}
}

func TestTheCatalogFillsInTheDefaultsForDevStatusAndOllama(t *testing.T) {
	s := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(s, Dependencies{DevStatus: "custom.md", OllamaURL: "http://x"})
	if len(names(s)) == 0 {
		t.Fatal("no tools registered")
	}
}
