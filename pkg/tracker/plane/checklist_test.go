package plane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker"
)

const checklistSample = `# Status

## Milestone Progress Summary
| a | b |

### Milestone 1: Core
- [x] **OXI-101**: Aligned Storage
- [ ] Ticket 9.2.1: DAG solver for pool (` + "`embedag-core`" + `)
  - [ ] nested acceptance criterion is ignored
- [~] DOC-102 Layered Loader — fluent builder and overrides
### Milestone 2: Extras
- [ ] Untitled idea without an ID
- [x]
`

func TestParseChecklist_FormatsAndMilestones(t *testing.T) {
	items := ParseChecklist(checklistSample)
	if len(items) != 4 {
		t.Fatalf("got %d items; want 4: %+v", len(items), items)
	}
	want := []ChecklistItem{
		{Key: "OXI-101", Title: "Aligned Storage", Milestone: "Milestone 1: Core", Status: tracker.StatusDone},
		{Key: "9.2.1", Title: "DAG solver for pool (`embedag-core`)", Milestone: "Milestone 1: Core", Status: tracker.StatusTodo},
		{Key: "DOC-102", Title: "Layered Loader", Detail: "fluent builder and overrides", Milestone: "Milestone 1: Core", Status: tracker.StatusInProgress},
	}
	for i, w := range want {
		if items[i] != w {
			t.Errorf("item %d = %+v; want %+v", i, items[i], w)
		}
	}
	last := items[3]
	if !strings.HasPrefix(last.Key, "item-") || last.Milestone != "Milestone 2: Extras" {
		t.Errorf("unkeyed item = %+v; want hashed key under Milestone 2", last)
	}
	if again := ParseChecklist(checklistSample); again[3].Key != last.Key {
		t.Errorf("hashed keys must be stable")
	}
}

func TestImportChecklist_CreatesOncePerKeyWithStatesAndModules(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}
	var moduleLinks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/modules/m") && strings.HasSuffix(r.URL.Path, "/issues/"):
			moduleLinks++
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"results": [
				{"id": "st-todo", "name": "Todo", "group": "unstarted"},
				{"id": "st-prog", "name": "In Progress", "group": "started"},
				{"id": "st-verify", "name": "To Verify", "group": "started"},
				{"id": "st-done", "name": "Done", "group": "completed"}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			_, _ = w.Write([]byte(`{"results": [{"id": "old", "name": "[OXI-101] Aligned Storage", "external_id": "OXI-101", "external_source": "dendrite-checklist"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/"):
			var b map[string]interface{}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &b)
			bodies = append(bodies, b)
			_, _ = w.Write([]byte(`{"id": "new"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			_, _ = w.Write([]byte(`{"results": [{"id": "m1", "name": "Milestone 1: Core"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/modules/"):
			_, _ = w.Write([]byte(`{"id": "m2", "name": "Milestone 2: Extras"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := NewClient(Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	rep, err := ImportChecklist(context.Background(), c, ParseChecklist(checklistSample), "DEV_STATUS.md")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created != 3 || rep.Existing != 1 || rep.Modules != 1 || moduleLinks != 3 {
		t.Fatalf("report = %+v, module links %d; want 3 created, 1 existing, 1 new module, 3 links", rep, moduleLinks)
	}
	if bodies[0]["name"] != "[9.2.1] DAG solver for pool (`embedag-core`)" || bodies[0]["state"] != "st-todo" {
		t.Errorf("first created issue = %v", bodies[0])
	}
	if bodies[1]["state"] != "st-prog" || !strings.Contains(bodies[1]["description_html"].(string), "fluent builder") {
		t.Errorf("in-progress item = %v", bodies[1])
	}
	for _, b := range bodies {
		if b["external_source"] != ChecklistSource || b["external_id"] == "" {
			t.Errorf("issue %v lacks its import marker", b["name"])
		}
	}
}
