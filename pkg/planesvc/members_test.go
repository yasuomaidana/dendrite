package planesvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

// Label ids as Plane makes them: UUIDs. The sidecar reads anything else as a label name.
const (
	lblOldAgent = "aaaaaaaa-0000-0000-0000-000000000001"
	lblOldTier  = "aaaaaaaa-0000-0000-0000-000000000002"
	lblKeep     = "aaaaaaaa-0000-0000-0000-000000000003"
)

// boardPlane is a Plane that remembers: the labels of the project, the labels and assignees of one issue and
// its comments. A claim is only right if a second claim leaves the same labels, which needs state.
type boardPlane struct {
	*httptest.Server
	mu        sync.Mutex
	labels    map[string]string // id -> name
	issueLbl  []string          // label ids on the issue
	assignees []string
	comments  []map[string]string
	posted    []string // names of labels created through POST
	patches   []map[string]any
}

func newBoardPlane(t *testing.T) *boardPlane {
	t.Helper()
	b := &boardPlane{
		labels:   map[string]string{lblOldAgent: "agent:old", lblOldTier: "tier:3", lblKeep: "keep"},
		issueLbl: []string{lblOldAgent, lblOldTier, lblKeep},
		comments: []map[string]string{
			{"id": "c-2", "comment_html": "<p>second</p>", "created_at": "2026-10-29T10:00:02Z", "updated_at": "2026-10-29T10:00:02Z"},
			{"id": "c-1", "comment_html": "<p>first</p>", "created_at": "2026-10-29T10:00:01Z", "updated_at": "2026-10-29T10:00:01Z"},
		},
	}
	b.Server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.Close)
	return b
}

func (b *boardPlane) reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// issueJSON is the issue as Plane sends it: unset lists are [], never null.
func (b *boardPlane) issueJSON() map[string]any {
	return map[string]any{"id": issueUUID, "name": "[DEN-5] five", "sequence_id": 5, "state": "s",
		"labels": append([]string{}, b.issueLbl...), "assignees": append([]string{}, b.assignees...)}
}

func (b *boardPlane) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	b.mu.Lock()
	defer b.mu.Unlock()
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/users/me/"):
		b.reply(w, map[string]any{"id": "u-1", "email": "agent@example.com", "display_name": "Agent"})
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/projects/proj/issues/"):
		b.reply(w, map[string]any{"results": []any{b.issueJSON()}, "next_page_results": false})
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/labels/"):
		var list []map[string]string
		for id, name := range b.labels {
			list = append(list, map[string]string{"id": id, "name": name, "color": "#000000"})
		}
		b.reply(w, map[string]any{"results": list})
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/labels/"):
		name, _ := body["name"].(string)
		id := fmt.Sprintf("l-new-%d", len(b.posted)+1)
		b.labels[id] = name
		b.posted = append(b.posted, name)
		b.reply(w, map[string]string{"id": id, "name": name, "color": "#7C3AED"})
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/comments/"):
		b.reply(w, map[string]any{"results": b.comments})
	case r.Method == http.MethodPatch && strings.Contains(p, "/comments/"):
		for _, c := range b.comments {
			if strings.HasSuffix(p, "/comments/"+c["id"]+"/") {
				c["comment_html"], _ = body["comment_html"].(string)
				b.reply(w, c)
				return
			}
		}
		http.NotFound(w, r)
	case r.Method == http.MethodPatch:
		b.patches = append(b.patches, body)
		if v, ok := body["labels"].([]any); ok {
			b.issueLbl = nil
			for _, id := range v {
				b.issueLbl = append(b.issueLbl, id.(string))
			}
		}
		if v, ok := body["assignees"].([]any); ok {
			b.assignees = nil
			for _, id := range v {
				b.assignees = append(b.assignees, id.(string))
			}
		}
		b.reply(w, b.issueJSON())
	case r.Method == http.MethodGet && strings.Contains(p, "/issues/"+issueUUID):
		b.reply(w, b.issueJSON())
	default:
		http.NotFound(w, r)
	}
}

func (b *boardPlane) client(t *testing.T) *plane.Client {
	t.Helper()
	c, err := plane.NewClient(plane.Config{BaseURL: b.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, b.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (b *boardPlane) names(ids []string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, b.labels[id])
	}
	return out
}

func TestPatchLabelsReplacesAgentAndTierLabelsAndIsRepeatable(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	b := newBoardPlane(t)
	svc := realService()
	defer svc.Close()
	ctx := context.Background()
	add := []string{"agent:claude", "tier:7"}
	remove := []string{"agent:", "tier:"}

	issue, names, err := svc.PatchLabels(ctx, b.client(t), "DEN-5", add, remove)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"keep", "agent:claude", "tier:7"}; !reflect.DeepEqual(names, want) {
		t.Errorf("labels after the first claim = %v, want %v", names, want)
	}
	if got := b.names(b.issueLbl); !reflect.DeepEqual(got, names) {
		t.Errorf("Plane holds %v, the call reported %v", got, names)
	}
	if issue.ID != issueUUID {
		t.Errorf("issue = %+v", issue)
	}
	if !reflect.DeepEqual(b.posted, add) {
		t.Errorf("labels created = %v, want %v", b.posted, add)
	}

	// A re-claim, with a different tier: the old agent/tier labels go, nothing piles up.
	_, names, err = svc.PatchLabels(ctx, b.client(t), "DEN-5", []string{"agent:claude", "tier:9"}, remove)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"keep", "agent:claude", "tier:9"}; !reflect.DeepEqual(names, want) {
		t.Errorf("labels after the re-claim = %v, want %v", names, want)
	}

	// The same call again changes nothing and creates nothing.
	created := len(b.posted)
	_, again, err := svc.PatchLabels(ctx, b.client(t), "DEN-5", []string{"agent:claude", "tier:9"}, remove)
	if err != nil || !reflect.DeepEqual(again, names) || len(b.posted) != created {
		t.Errorf("repeat: labels = %v (want %v), created %d more, err = %v", again, names, len(b.posted)-created, err)
	}

	if _, _, err := svc.PatchLabels(ctx, b.client(t), "DEN-99", add, remove); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("unknown ticket err = %v, want plane.ErrIssueNotFound", err)
	}
}

func TestUpdateIssueSendsAssigneesAndLabelsAsFullLists(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	b := newBoardPlane(t)
	svc := realService()
	defer svc.Close()
	ctx := context.Background()

	if _, err := svc.UpdateIssue(ctx, b.client(t), "DEN-5", plane.UpdateIssueRequest{Assignees: []string{"u-1"}, Labels: []string{lblKeep, "brand-new"}}); err != nil {
		t.Fatal(err)
	}
	patch := b.patches[len(b.patches)-1]
	if !reflect.DeepEqual(patch["assignees"], []any{"u-1"}) {
		t.Errorf("assignees = %v", patch["assignees"])
	}
	// A value that is not a UUID is a name: it is created and sent as an id.
	if want := []any{lblKeep, "l-new-1"}; !reflect.DeepEqual(patch["labels"], want) {
		t.Errorf("labels = %v, want %v (names are resolved to ids)", patch["labels"], want)
	}

	// An empty, non-nil list clears; nil leaves the field out.
	if _, err := svc.UpdateIssue(ctx, b.client(t), "DEN-5", plane.UpdateIssueRequest{Assignees: []string{}, Name: "kept"}); err != nil {
		t.Fatal(err)
	}
	patch = b.patches[len(b.patches)-1]
	if !reflect.DeepEqual(patch["assignees"], []any{}) {
		t.Errorf("assignees = %#v, want an empty list that clears them", patch["assignees"])
	}
	if _, has := patch["labels"]; has {
		t.Errorf("labels were not set and must not be sent: %v", patch)
	}
	if len(b.assignees) != 0 {
		t.Errorf("Plane still has assignees %v", b.assignees)
	}
}

func TestMeReturnsTheAccountOfTheAPIKey(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	b := newBoardPlane(t)
	svc := realService()
	defer svc.Close()
	u, err := svc.Me(context.Background(), b.client(t))
	if err != nil || u.ID != "u-1" || u.Email != "agent@example.com" || u.DisplayName != "Agent" {
		t.Errorf("user = %+v, err = %v", u, err)
	}
}

func TestCommentsAreListedOldestFirstAndEditedAsGiven(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	b := newBoardPlane(t)
	svc := realService()
	defer svc.Close()
	ctx := context.Background()

	got, err := svc.Comments(ctx, b.client(t), "DEN-5")
	if err != nil || len(got) != 2 || got[0].ID != "c-1" || got[1].ID != "c-2" || got[0].CommentHTML != "<p>first</p>" {
		t.Fatalf("comments = %+v, err = %v", got, err)
	}

	if err := svc.UpdateComment(ctx, b.client(t), "DEN-5", "c-2", "<h1>plan v2</h1>"); err != nil {
		t.Fatal(err)
	}
	if html := b.comments[0]["comment_html"]; html != "<h1>plan v2</h1>" {
		t.Errorf("stored comment = %q, want the HTML exactly as given (no paragraph wrapping)", html)
	}

	if err := svc.UpdateComment(ctx, b.client(t), "DEN-5", "c-404", "<p>x</p>"); !errors.Is(err, plane.ErrCommentNotFound) || errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("unknown comment err = %v, want plane.ErrCommentNotFound and not a missing ticket", err)
	}
	if err := svc.UpdateComment(ctx, b.client(t), "DEN-99", "c-1", "<p>x</p>"); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("unknown ticket err = %v, want plane.ErrIssueNotFound", err)
	}
	if _, err := svc.Comments(ctx, b.client(t), "DEN-99"); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("comments of an unknown ticket err = %v", err)
	}
	if err := svc.UpdateComment(ctx, b.client(t), "DEN-5", "c-1", "  "); err == nil {
		t.Error("an empty comment is refused")
	}
}

func TestNonNilTurnsANilListIntoAnEmptyOne(t *testing.T) {
	if got := nonNil(nil); got == nil || len(got) != 0 {
		t.Errorf("nonNil(nil) = %#v, want an empty non-nil list", got)
	}
	in := []string{"a"}
	if got := nonNil(in); !reflect.DeepEqual(got, in) {
		t.Errorf("nonNil(%v) = %v", in, got)
	}
}

func TestMembersCallsReportAMissingSidecar(t *testing.T) {
	var svc *Service
	ctx := context.Background()
	c, err := plane.NewClient(plane.Config{BaseURL: "http://127.0.0.1:1", WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Me(ctx, c); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("Me: %v", err)
	}
	if _, _, err := svc.PatchLabels(ctx, c, "DEN-1", nil, nil); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("PatchLabels: %v", err)
	}
	if _, err := svc.Comments(ctx, c, "DEN-1"); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("Comments: %v", err)
	}
	if err := svc.UpdateComment(ctx, c, "DEN-1", "c", "x"); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("UpdateComment: %v", err)
	}
}
