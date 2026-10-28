package mcpserver

import (
	"strings"
	"testing"
)

func newDefaultServerForProfileTest() *Server {
	s := NewServer(ServerConfig{Name: "t", Version: "1"})
	RegisterDefaultTools(s, Dependencies{DevStatus: "DEV_STATUS.md"})
	return s
}

func TestParseProfile(t *testing.T) {
	cases := []struct {
		in   string
		want Profile
		err  bool
	}{
		{"", ProfileFull, false}, {"core", ProfileCore, false}, {" FULL ", ProfileFull, false}, {"tiny", "", true},
	}
	for _, c := range cases {
		got, err := ParseProfile(c.in, ProfileFull)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("ParseProfile(%q) = %q, %v", c.in, got, err)
		}
	}
}

func TestProfileEnv(t *testing.T) {
	t.Setenv(ProfileEnv, "core")
	if p, err := ProfileFromEnv(ProfileFull); err != nil || p != ProfileCore {
		t.Fatalf("env core = %q, %v", p, err)
	}
	t.Setenv(ProfileEnv, "bogus")
	if _, err := ProfileFromEnv(ProfileFull); err == nil {
		t.Fatal("bogus profile accepted")
	}
}

func TestFullProfileKeepsEveryTool(t *testing.T) {
	src := newDefaultServerForProfileTest()
	if got := ApplyProfile(src, ProfileFull); got != src {
		t.Fatal("full profile must return the server unchanged")
	}
}

func TestCoreProfileExposesOnlyRAGGraphPlane(t *testing.T) {
	src := newDefaultServerForProfileTest()
	core := ApplyProfile(src, ProfileCore)
	tools := core.ListTools()
	if len(tools) == 0 || len(tools) >= len(src.ListTools()) {
		t.Fatalf("core has %d of %d tools", len(tools), len(src.ListTools()))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
		ok := coreToolNames[tool.Name]
		for _, p := range coreToolPrefixes {
			ok = ok || strings.HasPrefix(tool.Name, p)
		}
		if !ok {
			t.Errorf("core exposes %s", tool.Name)
		}
		if _, has := core.Handler(tool.Name); !has {
			t.Errorf("core tool %s lost its handler", tool.Name)
		}
	}
	for _, want := range []string{"dendrite_rag_retrieve", "dendrite_rag_show", "dendrite_rag_ask", "dendrite_plane_issues", "dendrite_graph_explain", "dendrite_ticket_smart_select"} {
		if !seen[want] {
			t.Errorf("core is missing %s", want)
		}
	}
	for _, banned := range []string{"dendrite_store_rm", "dendrite_service_move", "dendrite_dist_spawn"} {
		if seen[banned] {
			t.Errorf("core exposes %s", banned)
		}
	}
}
