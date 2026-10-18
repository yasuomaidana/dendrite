package gateway

import "testing"

func TestAddContextWindowFallbacks(t *testing.T) {
	rs := &RouterSettings{ContextWindowFallbacks: []map[string]string{{"local-a": "cloud"}}}
	addContextWindowFallbacks(rs, []string{"local-a", "local-b", "cloud", "local-b"}, "cloud")

	if len(rs.ContextWindowFallbacks) != 2 {
		t.Fatalf("expected 2 entries (existing local-a + new local-b), got %v", rs.ContextWindowFallbacks)
	}
	if rs.ContextWindowFallbacks[1]["local-b"] != "cloud" {
		t.Fatalf("expected local-b -> cloud, got %v", rs.ContextWindowFallbacks[1])
	}
	for _, e := range rs.ContextWindowFallbacks {
		if len(e) != 1 {
			t.Fatalf("every entry must map exactly one model: %v", e)
		}
		if _, self := e["cloud"]; self {
			t.Fatal("the fallback target must not fall back to itself")
		}
	}
}
