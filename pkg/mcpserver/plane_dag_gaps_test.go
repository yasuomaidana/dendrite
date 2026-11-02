package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const smartIssues = `{"results":[
 {"id":"u1","name":"[DEN-001] First","sequence_id":1,"state":"st-done","priority":"none","estimate_point":3,"start_date":"2026-01-01","target_date":"2026-01-05"},
 {"id":"u2","name":"No ticket tag","sequence_id":2,"state":"","priority":""},
 {"id":"u3","name":"[DEN-003] Third","sequence_id":3,"state":"st-todo","priority":"high"}]}`

func TestSmartSelectFromAPlaneBoardWithIncompleteIssues(t *testing.T) {
	sp := newScriptedPlane(t,
		on("GET", "/states/", 200, `{"results":[{"id":"st-done","name":"Done","group":"completed"},{"id":"st-todo","name":"To Do","group":"unstarted"}]}`),
		on("GET", "/issues/u2/issue-relation/", 200, `{"blocked_by":[{"id":"outside","name":"[DEN-050] external"},{"id":"u2","name":"self"}]}`),
		on("GET", "/issues/u3/issue-relation/", 200, `{"blocked_by":[{"id":"u1","name":"[DEN-001] First"},{"id":"nobody","name":"no tag"}]}`),
		on("GET", "/issue-relation/", 200, `{}`),
		on("GET", "/issues/", 200, smartIssues),
	)
	srv := toolsWith(t, sp, "DEV_STATUS.md")
	text, isErr := callTool(t, srv, "dendrite_ticket_smart_select", `{}`)
	if isErr || !strings.Contains(text, `"plane_online": true`) {
		t.Fatalf("text = %s, isErr = %v", text, isErr)
	}
}

func TestSmartSelectFromTheLocalLedger(t *testing.T) {
	noEnvPlane(t)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "DEV_STATUS.md")
	content := `- **Active Milestone**: Milestone 1: Core
- **Current Focus**: DEN-001

| Rank | Ticket | Milestone | Feature | Points | Score |
|:--:|:--|:--|:--|:--:|:--:|
| **1** | **DEN-001** | Milestone 1 | FEAT-001 | 3 SP | 9.5 / 10 |
| **2** | **DEN-002** | Milestone 1 | FEAT-002 | 8 SP | 7.5 / 10 |
| **3** | **DEN-003** | Milestone 2 | FEAT-003 | 5 SP | 2.0 / 10 |

### Milestone 1: Core
- [ ] **DEN-001**: First
- [ ] **DEN-002**: Second
### Milestone 2: Later
- [ ] **DEN-003**: Third
- [ ] **DEN-004**: Fourth
`
	if err := os.WriteFile(ledger, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := toolsWith(t, nil, ledger)

	if text, isErr := callTool(t, srv, "dendrite_ticket_smart_select", `{}`); isErr || !strings.Contains(text, `"plane_online": false`) {
		t.Fatalf("whole ledger: %s", text)
	}
	if text, isErr := callTool(t, srv, "dendrite_ticket_smart_select", `{"milestone":"Milestone 2"}`); isErr || strings.Contains(text, "DEN-001") {
		t.Fatalf("the milestone filter must leave out Milestone 1: %s", text)
	}
	if text, isErr := callTool(t, srv, "dendrite_ticket_smart_select", `{"ticket_id":"DEN-999"}`); !isErr || !strings.Contains(text, "smart selection failed") {
		t.Fatalf("unknown ticket: %s (error %v)", text, isErr)
	}
	if text, isErr := callTool(t, srv, "dendrite_ticket_smart_select", `{"file":"`+filepath.Join(dir, "missing.md")+`"}`); !isErr || !strings.Contains(text, "Plane is not available") {
		t.Fatalf("missing ledger: %s (error %v)", text, isErr)
	}
}

func TestRelationLinkAndIssueCreateRejectBadInput(t *testing.T) {
	noEnvPlane(t)
	srv := toolsWith(t, nil, "DEV_STATUS.md")
	runToolFailures(t, srv, []toolFailure{
		{"relation link without a type", "dendrite_plane_relation_link", `{"issue_id":"DEN-1"}`, nil, "all required"},
		{"relation link without a client", "dendrite_plane_relation_link", `{"issue_id":"DEN-1","relation_type":"blocked_by","target_issue_id":"DEN-2"}`, nil, "plane client error"},
		{"create without a name", "dendrite_plane_issue_create", `{"name":"  "}`, nil, "missing required 'name'"},
		{"create without a description", "dendrite_plane_issue_create", `{"name":"[DEN-9] x","description":" "}`, nil, "cannot be empty"},
		{"create without a client", "dendrite_plane_issue_create", `{"name":"[DEN-9] x","context":"real context"}`, nil, "plane client error"},
	})
}

func TestRelationLinkReportsEachWayItCanFail(t *testing.T) {
	uuid := "11111111-2222-3333-4444-555555555555"
	args := func(a, b string) string {
		return `{"issue_id":"` + a + `","relation_type":"blocked_by","target_issue_id":"` + b + `"}`
	}
	t.Run("the issue list cannot be read", func(t *testing.T) {
		sp := newScriptedPlane(t, on("GET", "/issues/", 500, ""))
		if text, isErr := callTool(t, toolsWith(t, sp, ""), "dendrite_plane_relation_link", args("DEN-1", uuid)); !isErr {
			t.Fatalf("text = %s", text)
		}
	})
	t.Run("the target ticket cannot be found", func(t *testing.T) {
		sp := newScriptedPlane(t, on("GET", "/issues/", 200, `{"results":[]}`))
		if text, isErr := callTool(t, toolsWith(t, sp, ""), "dendrite_plane_relation_link", args(uuid, "DEN-404")); !isErr || !strings.Contains(text, "not found in Plane") {
			t.Fatalf("text = %s (error %v)", text, isErr)
		}
	})
	t.Run("the relation is refused", func(t *testing.T) {
		sp := newScriptedPlane(t, on("POST", "/issue-relation/", 500, ""))
		if text, isErr := callTool(t, toolsWith(t, sp, ""), "dendrite_plane_relation_link", args(uuid, "99999999-2222-3333-4444-555555555555")); !isErr || !strings.Contains(text, "failed to create relation") {
			t.Fatalf("text = %s (error %v)", text, isErr)
		}
	})
}

func TestIssueCreatePlacesTheTicketInAModuleAndLinksItsBlockers(t *testing.T) {
	uuid := "11111111-2222-3333-4444-555555555555"
	sp := newScriptedPlane(t,
		on("GET", "/modules/", 200, `{"results":[{"id":"m1","name":"Milestone 1: Core"}]}`),
		on("POST", "/modules/", 201, `{"id":"m-new","name":"Brand new"}`),
		on("GET", "/issues/", 200, `{"results":[{"id":"u9","name":"[DEN-9] Blocker"}]}`),
		on("POST", "/issues/", 201, `{"id":"created","name":"[DEN-100] x","sequence_id":100}`),
	)
	srv := toolsWith(t, sp, "")

	text, isErr := callTool(t, srv, "dendrite_plane_issue_create",
		`{"name":"[DEN-100] x","description":"details","milestone":"Brand new","blocked_by":["DEN-9","`+uuid+`","",7,"unknown"]}`)
	if isErr || !strings.Contains(text, `"module_id": "m-new"`) || !strings.Contains(text, `"blockers_count": 2`) || !strings.Contains(text, `"priority": "medium"`) {
		t.Fatalf("text = %s (error %v)", text, isErr)
	}
	if text, isErr = callTool(t, srv, "dendrite_plane_issue_create", `{"name":"[DEN-101] y","description":"details","milestone":"core","estimate":8,"start_date":"2026-01-01"}`); isErr || !strings.Contains(text, `"module_id": "m1"`) {
		t.Fatalf("existing module: %s", text)
	}

	failing := newScriptedPlane(t, on("POST", "/issues/", 500, "no"))
	if text, isErr = callTool(t, toolsWith(t, failing, ""), "dendrite_plane_issue_create", `{"name":"[DEN-102] z","description":"details"}`); !isErr || !strings.Contains(text, "failed to create issue") {
		t.Fatalf("a refused create: %s (error %v)", text, isErr)
	}
}

func TestSmartSelectReportsALedgerItCannotScan(t *testing.T) {
	noEnvPlane(t)
	ledger := filepath.Join(t.TempDir(), "DEV_STATUS.md")
	if err := os.WriteFile(ledger, []byte(strings.Repeat("x", 70*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := callTool(t, toolsWith(t, nil, ledger), "dendrite_ticket_smart_select", `{}`); !isErr || !strings.Contains(text, "failed to parse dev status") {
		t.Fatalf("text = %s (error %v)", text, isErr)
	}
}
