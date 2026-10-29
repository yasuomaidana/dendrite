package planesvc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

const issueUUID = "11111111-2222-3333-4444-555555555555"

// recordedReq is one request the fake Plane saw.
type recordedReq struct {
	Method, Path string
	Body         map[string]any
}

// issuePlane speaks the issue routes: list (for ref resolution and external_id lookups), get,
// create, patch and comments.
type issuePlane struct {
	*httptest.Server
	mu       sync.Mutex
	reqs     []recordedReq
	existing string // JSON row listed by the issues list, if any
}

func newIssuePlane(t *testing.T) *issuePlane {
	t.Helper()
	f := &issuePlane{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recordedReq{r.Method, r.URL.Path, body})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/issues/"):
			rows := `{"id":"` + issueUUID + `","sequence_id":5,"name":"[DEN-5] five","state":"s"}`
			if f.existing != "" {
				rows = f.existing
			}
			_, _ = w.Write([]byte(`{"results":[` + rows + `],"next_page_results":false}`))
		case r.Method == http.MethodGet && strings.Contains(p, "/issues/missing"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"` + issueUUID + `","name":"[DEN-5] five","description_stripped":"body","state_detail":{"name":"To Do","group":"unstarted"}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/issues/"):
			_, _ = w.Write([]byte(`{"id":"new-1","name":"` + body["name"].(string) + `","sequence_id":9,"state":"s"}`))
		case r.Method == http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"` + issueUUID + `","name":"renamed","sequence_id":5,"state":"s"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/comments/"):
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *issuePlane) client(t *testing.T) *plane.Client {
	t.Helper()
	c, err := plane.NewClient(plane.Config{BaseURL: f.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, f.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *issuePlane) lastBody(t *testing.T, method string) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].Method == method {
			return f.reqs[i].Body
		}
	}
	t.Fatalf("no %s request in %+v", method, f.reqs)
	return nil
}

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }

func TestGetIssueThroughTheSidecar(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	svc := realService()
	defer svc.Close()
	got, err := svc.GetIssue(context.Background(), f.client(t), "DEN-5")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != issueUUID || got.DescriptionStripped != "body" || got.StateDetail.Group != "unstarted" {
		t.Errorf("issue = %+v", got)
	}
	if _, err := svc.GetIssue(context.Background(), f.client(t), "DEN-99"); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("unknown ticket err = %v, want plane.ErrIssueNotFound", err)
	}
	if _, err := svc.GetIssue(context.Background(), f.client(t), "aaaaaaaa-missing-aaaa-aaaa-aaaaaaaaaaaa"); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("404 err = %v, want plane.ErrIssueNotFound", err)
	}
}

func TestCreateIssueSendsTheBodyTheGoClientSent(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	svc := realService()
	defer svc.Close()
	got, created, err := svc.CreateIssue(context.Background(), f.client(t), plane.CreateIssueRequest{
		Name: "n", DescriptionHTML: "<p>goal</p>", Priority: "high", EstimatePoint: intp(40),
		StartDate: strp("2026-10-27"), Labels: []string{"l1"}, ExternalID: "X-1", ExternalSource: "dendrite",
	})
	if err != nil || !created || got.ID != "new-1" {
		t.Fatalf("got %+v created=%v err=%v", got, created, err)
	}
	want := map[string]any{
		"name": "n", "description_html": "<p>goal</p>", "priority": "high", "estimate_point": float64(12),
		"start_date": "2026-10-27", "labels": []any{"l1"}, "external_id": "X-1", "external_source": "dendrite",
	}
	if body := f.lastBody(t, http.MethodPost); !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v\nwant   %v", body, want)
	}
}

func TestCreateIssueIsIdempotentOnExternalID(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	f.existing = `{"id":"old","name":"x","sequence_id":1,"external_id":"X-1"}`
	svc := realService()
	defer svc.Close()
	got, created, err := svc.CreateIssue(context.Background(), f.client(t), plane.CreateIssueRequest{
		Name: "x", DescriptionHTML: "<p>d</p>", ExternalID: "X-1",
	})
	if err != nil || created || got.ID != "old" {
		t.Fatalf("got %+v created=%v err=%v", got, created, err)
	}
	for _, r := range f.reqs {
		if r.Method == http.MethodPost {
			t.Errorf("a duplicate was posted: %+v", r)
		}
	}
}

func TestCreateIssueRefusesEmptyInput(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	svc := realService()
	defer svc.Close()
	_, _, err := svc.CreateIssue(context.Background(), f.client(t), plane.CreateIssueRequest{Name: "n", DescriptionHTML: "<p>&nbsp;</p>"})
	if !errors.Is(err, plane.ErrEmptyTicketContext) {
		t.Errorf("empty description err = %v, want plane.ErrEmptyTicketContext", err)
	}
	_, _, err = svc.CreateIssue(context.Background(), f.client(t), plane.CreateIssueRequest{Name: " ", DescriptionHTML: "<p>d</p>"})
	if err == nil || !strings.Contains(err.Error(), "name cannot be empty") {
		t.Errorf("empty name err = %v", err)
	}
	if len(f.reqs) != 0 {
		t.Errorf("Plane was called: %+v", f.reqs)
	}
}

func TestUpdateIssueSendsOnlyTheSetFields(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	svc := realService()
	defer svc.Close()
	got, err := svc.UpdateIssue(context.Background(), f.client(t), "DEN-5", plane.UpdateIssueRequest{Name: "renamed", EstimatePoint: intp(-3)})
	if err != nil || got.Name != "renamed" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	want := map[string]any{"name": "renamed", "estimate_point": float64(0)}
	if body := f.lastBody(t, http.MethodPatch); !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
}

func TestCommentWrapsTheTextInAParagraph(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newIssuePlane(t)
	svc := realService()
	defer svc.Close()
	if err := svc.Comment(context.Background(), f.client(t), "DEN-5", "hello"); err != nil {
		t.Fatal(err)
	}
	if body := f.lastBody(t, http.MethodPost); body["comment_html"] != "<p>hello</p>" {
		t.Errorf("body = %v", body)
	}
	if err := svc.Comment(context.Background(), f.client(t), "DEN-99", "hi"); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("unknown ticket err = %v, want plane.ErrIssueNotFound", err)
	}
}
