package plane

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

const issueUUID = "11111111-2222-3333-4444-555555555555"

func planeAt(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := NewClient(Config{BaseURL: baseURL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "token"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func closedPlane() string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

func statusServer(code int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
}

// call is one client operation whose HTTP failures are checked in a table.
type call struct {
	name     string
	run      func(c *Client) error
	notFound error // what a 404 means for this call; nil when it is an ordinary failure
	decodes  bool  // the call fails on a 200 answer that is not JSON
}

func calls() []call {
	ctx := context.Background()
	return []call{
		{"GetIssue", func(c *Client) error { _, err := c.GetIssue(ctx, "DEN-1"); return err }, ErrIssueNotFound, true},
		{"ListPlaneIssues", func(c *Client) error { _, err := c.ListPlaneIssues(ctx); return err }, nil, false},
		{"ListStates", func(c *Client) error { _, err := c.ListStates(ctx); return err }, nil, false},
		{"ListProjects", func(c *Client) error { _, err := c.ListProjects(ctx); return err }, nil, false},
		{"CreateProject", func(c *Client) error { _, err := c.CreateProject(ctx, "n", "id", "d"); return err }, nil, true},
		{"ListModules", func(c *Client) error { _, err := c.ListModules(ctx); return err }, nil, false},
		{"ListModuleIssues", func(c *Client) error { _, err := c.ListModuleIssues(ctx, "m"); return err }, nil, false},
		{"ListPages", func(c *Client) error { _, err := c.ListPages(ctx); return err }, nil, false},
		{"GetPage", func(c *Client) error { _, err := c.GetPage(ctx, "p"); return err }, ErrPageNotFound, true},
		{"GetIssueRelations", func(c *Client) error { _, err := c.GetIssueRelations(ctx, "i"); return err }, ErrIssueNotFound, true},
		{"CreatePlaneIssue", func(c *Client) error {
			_, err := c.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "n", DescriptionHTML: "<p>d</p>"})
			return err
		}, nil, true},
		{"UpdatePlaneIssue", func(c *Client) error {
			_, err := c.UpdatePlaneIssueWithDetails(ctx, "i", UpdateIssueRequest{Name: "n"})
			return err
		}, ErrIssueNotFound, true},
		{"CreateModule", func(c *Client) error { _, err := c.CreateModuleWithDates(ctx, "m", "", "d", nil, nil); return err }, nil, true},
		{"UpdateModule", func(c *Client) error { _, err := c.UpdateModule(ctx, "m", UpdateModuleRequest{Name: "n"}); return err }, ErrModuleNotFound, true},
		{"AddIssuesToModule", func(c *Client) error { return c.AddIssuesToModule(ctx, "m", []string{"i"}) }, ErrModuleNotFound, false},
		{"AddIssueRelation", func(c *Client) error { return c.AddIssueRelation(ctx, "i", "blocked_by", []string{"j"}) }, nil, false},
		{"RemoveIssueRelation", func(c *Client) error { return c.RemoveIssueRelation(ctx, "i", "blocked_by", "j") }, nil, false},
		{"CreatePage", func(c *Client) error { _, err := c.CreatePage(ctx, "n", "<p>x</p>"); return err }, nil, true},
		{"UpdatePage", func(c *Client) error { _, err := c.UpdatePage(ctx, "p", "n", "<p>x</p>"); return err }, ErrPageNotFound, true},
		{"ArchivePage", func(c *Client) error { return c.ArchivePage(ctx, "p") }, ErrPageNotFound, false},
		{"AddComment", func(c *Client) error { return c.AddComment(ctx, tracker.TicketID(issueUUID), "hi") }, ErrIssueNotFound, false},
		{"AddIssueComment", func(c *Client) error { return c.AddIssueComment(ctx, issueUUID, "hi") }, ErrIssueNotFound, false},
		{"LinkPullRequest", func(c *Client) error { return c.LinkPullRequest(ctx, tracker.TicketID(issueUUID), "https://x/pr/1") }, ErrIssueNotFound, false},
	}
}

func TestEveryCallReportsTransportAndURLFailures(t *testing.T) {
	for _, op := range calls() {
		t.Run(op.name+" unreachable", func(t *testing.T) {
			if err := op.run(planeAt(t, closedPlane())); err == nil {
				t.Fatal("expected a transport error")
			}
		})
		t.Run(op.name+" bad base URL", func(t *testing.T) {
			if err := op.run(planeAt(t, "http://%zz")); err == nil {
				t.Fatal("expected a URL error")
			}
		})
	}
}

func TestEveryCallMapsUnsuccessfulStatuses(t *testing.T) {
	for _, op := range calls() {
		for _, code := range []int{401, 403} {
			t.Run(op.name+" "+http.StatusText(code), func(t *testing.T) {
				s := statusServer(code, "")
				defer s.Close()
				if err := op.run(planeAt(t, s.URL)); !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("err = %v, want unauthorized", err)
				}
			})
		}
		t.Run(op.name+" 404", func(t *testing.T) {
			s := statusServer(404, "")
			defer s.Close()
			err := op.run(planeAt(t, s.URL))
			switch {
			case op.notFound != nil && !errors.Is(err, op.notFound):
				t.Fatalf("err = %v, want %v", err, op.notFound)
			case op.notFound == nil && (err == nil || !strings.Contains(err.Error(), "status 404")):
				t.Fatalf("err = %v, want the status named", err)
			}
		})
		t.Run(op.name+" 500", func(t *testing.T) {
			s := statusServer(500, "boom")
			defer s.Close()
			if err := op.run(planeAt(t, s.URL)); err == nil || !strings.Contains(err.Error(), "status 500") {
				t.Fatalf("err = %v, want the status named", err)
			}
		})
		if op.decodes {
			t.Run(op.name+" answer that is not JSON", func(t *testing.T) {
				s := statusServer(200, "not json")
				defer s.Close()
				if err := op.run(planeAt(t, s.URL)); err == nil {
					t.Fatal("expected a decoding error")
				}
			})
		}
	}
}

func TestFailedCreateQuotesPlanesAnswer(t *testing.T) {
	s := statusServer(400, "name already used")
	defer s.Close()
	_, err := planeAt(t, s.URL).CreatePlaneIssueWithDetails(context.Background(), CreateIssueRequest{Name: "n", DescriptionHTML: "<p>d</p>"})
	if err == nil || !strings.Contains(err.Error(), "status 400, err: name already used") {
		t.Fatalf("err = %v, want Plane's answer quoted", err)
	}
}

func TestAnswerCutShortIsAnError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("{"))
	}))
	defer s.Close()
	if _, err := planeAt(t, s.URL).ListStates(context.Background()); err == nil {
		t.Fatal("a truncated answer must be an error")
	}
}

func TestListPlaneIssuesStopsAtAnUnreadablePage(t *testing.T) {
	s := statusServer(500, "")
	defer s.Close()
	if _, err := planeAt(t, s.URL).ListPlaneIssues(context.Background()); err == nil {
		t.Fatal("expected the page error")
	}
}

func TestCreateAndUpdateSendEveryOptionalField(t *testing.T) {
	var bodies []map[string]interface{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		_ = json.Unmarshal(data, &m)
		bodies = append(bodies, m)
		_, _ = w.Write([]byte(`{"id":"` + issueUUID + `"}`))
	}))
	defer s.Close()
	c := planeAt(t, s.URL)
	ctx := context.Background()
	str := func(v string) *string { return &v }
	num := func(v int) *int { return &v }

	for _, c2 := range []struct {
		estimate *int
		want     float64
	}{{num(99), 12}, {num(-4), 0}, {num(5), 5}} {
		bodies = nil
		if _, err := c.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{
			Name: "n", DescriptionHTML: "<p>d</p>", EstimatePoint: c2.estimate, Parent: str("p1"), Labels: []string{"a"},
			StartDate: str("2026-01-01"), TargetDate: str("2026-02-01"), Priority: "high", StateID: "s",
			ExternalID: "x", ExternalSource: "y",
		}); err != nil {
			t.Fatal(err)
		}
		if got := bodies[0]["estimate_point"]; got != c2.want {
			t.Errorf("create estimate = %v, want %v", got, c2.want)
		}
		if bodies[0]["parent"] != "p1" || bodies[0]["labels"] == nil {
			t.Errorf("create body = %v", bodies[0])
		}

		bodies = nil
		if _, err := c.UpdatePlaneIssueWithDetails(ctx, "i", UpdateIssueRequest{
			Name: "n", DescriptionHTML: "<p>d</p>", EstimatePoint: c2.estimate, Parent: str("p1"),
			StartDate: str("2026-01-01"), TargetDate: str("2026-02-01"), Priority: "high", StateID: "s",
		}); err != nil {
			t.Fatal(err)
		}
		if got := bodies[0]["estimate_point"]; got != c2.want {
			t.Errorf("update estimate = %v, want %v", got, c2.want)
		}
		if bodies[0]["parent"] != "p1" {
			t.Errorf("update body = %v", bodies[0])
		}
	}

	bodies = nil
	if _, err := c.UpdateModule(ctx, "m", UpdateModuleRequest{Status: "completed", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	if bodies[0]["status"] != "completed" || bodies[0]["description"] != "d" {
		t.Errorf("module body = %v", bodies[0])
	}
	bodies = nil
	if _, err := c.CreateModuleWithDates(ctx, "m", "", "d", str("2026-01-01"), str("2026-02-01")); err != nil {
		t.Fatal(err)
	}
	if bodies[0]["status"] != "in-progress" || bodies[0]["start_date"] != "2026-01-01" {
		t.Errorf("new module body = %v", bodies[0])
	}
}

func TestNewRequestIsTheSameAsTheInternalOne(t *testing.T) {
	c := planeAt(t, "http://example.invalid")
	req, err := c.NewRequest(context.Background(), http.MethodPost, "/api/v1/x", map[string]string{"a": "b"})
	if err != nil || req.Header.Get("x-api-key") != "token" || req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("request = %+v, err = %v", req, err)
	}
	if _, err := c.NewRequest(context.Background(), http.MethodPost, "/x", make(chan int)); err == nil {
		t.Fatal("a body that cannot be encoded must be an error")
	}
}
