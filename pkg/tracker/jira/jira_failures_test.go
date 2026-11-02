package jira

import (
	"Dendrite/pkg/testkit"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

// jiraAt returns a client that talks to baseURL.
func jiraAt(t *testing.T, baseURL string) *Client {
	t.Helper()
	t.Setenv("JIRA_FAILURE_TOKEN", "secret")
	return testkit.Build(t, func() (*Client, error) {
		return NewClient(Config{BaseURL: baseURL, Email: "dev@dendrite.ai", APITokenEnv: "JIRA_FAILURE_TOKEN"}, nil)
	})
}

// closedURL is the address of a server that has already shut down, so every request fails.
func closedURL() string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

// hangUpOnPost answers GETs with body and drops the connection on any other method.
func hangUpOnPost(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(body))
			return
		}
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	})
}

func status(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
}

func reply(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
}

type operation struct {
	name string
	run  func(c *Client) error
}

var operations = []operation{
	{"GetIssue", func(c *Client) error { _, err := c.GetIssue(context.Background(), "DEN-1"); return err }},
	{"UpdateStatus", func(c *Client) error { return c.UpdateStatus(context.Background(), "DEN-1", tracker.StatusDone) }},
	{"AddComment", func(c *Client) error { return c.AddComment(context.Background(), "DEN-1", "text") }},
	{"LinkPullRequest", func(c *Client) error { return c.LinkPullRequest(context.Background(), "DEN-1", "https://x/pr/1") }},
}

func TestEveryOperationReportsTransportAndURLFailures(t *testing.T) {
	for _, op := range operations {
		t.Run(op.name+" unreachable", func(t *testing.T) {
			if err := op.run(jiraAt(t, closedURL())); err == nil {
				t.Fatal("expected a transport error")
			}
		})
		t.Run(op.name+" bad base URL", func(t *testing.T) {
			if err := op.run(jiraAt(t, "http://%zz")); err == nil {
				t.Fatal("expected a URL error")
			}
		})
	}
}

func TestEveryOperationMapsUnsuccessfulStatuses(t *testing.T) {
	for _, op := range operations {
		for _, c := range []struct {
			code int
			want error
		}{{401, ErrUnauthorized}, {403, ErrUnauthorized}, {404, ErrIssueNotFound}} {
			t.Run(op.name+" "+http.StatusText(c.code), func(t *testing.T) {
				srv := httptest.NewServer(status(c.code))
				defer srv.Close()
				if err := op.run(jiraAt(t, srv.URL)); !errors.Is(err, c.want) {
					t.Fatalf("err = %v, want %v", err, c.want)
				}
			})
		}
		t.Run(op.name+" 500", func(t *testing.T) {
			srv := httptest.NewServer(status(500))
			defer srv.Close()
			if err := op.run(jiraAt(t, srv.URL)); err == nil || !strings.Contains(err.Error(), "status 500") {
				t.Fatalf("err = %v, want the status named", err)
			}
		})
	}
}

func TestResponsesThatAreNotJSONAreErrors(t *testing.T) {
	srv := httptest.NewServer(reply("not json"))
	defer srv.Close()
	c := jiraAt(t, srv.URL)
	if _, err := c.GetIssue(context.Background(), "DEN-1"); err == nil {
		t.Error("GetIssue accepted a broken body")
	}
	if err := c.UpdateStatus(context.Background(), "DEN-1", tracker.StatusDone); err == nil {
		t.Error("UpdateStatus accepted a broken transition list")
	}
}

func TestUpdateStatusReportsTheTransitionRequestFailing(t *testing.T) {
	transitions := `{"transitions":[{"id":"41","name":"Done"}]}`

	hangup := httptest.NewServer(hangUpOnPost(transitions))
	defer hangup.Close()
	if err := jiraAt(t, hangup.URL).UpdateStatus(context.Background(), "DEN-1", tracker.StatusDone); err == nil {
		t.Error("expected the dropped POST to fail")
	}

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(transitions))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer refusing.Close()
	err := jiraAt(t, refusing.URL).UpdateStatus(context.Background(), "DEN-1", tracker.StatusDone)
	if err == nil || !strings.Contains(err.Error(), "transition execution failed: status 400") {
		t.Fatalf("err = %v, want the refused transition", err)
	}
}

func TestNewRequestReportsABodyItCannotEncode(t *testing.T) {
	c := jiraAt(t, "http://example.invalid")
	if _, err := c.newRequest(context.Background(), http.MethodPost, "/x", make(chan int)); err == nil {
		t.Fatal("expected an encoding error")
	}
}

func TestADFEdgeCases(t *testing.T) {
	if got := ParseADFToMarkdown([]byte(`[1,2,3]`)); got != "[1,2,3]" {
		t.Errorf("a document that is neither text nor ADF is returned as written, got %q", got)
	}
	unknown := `{"type":"doc","content":[{"type":"panel","content":[{"type":"text","text":"inside"}]}]}`
	if got := ParseADFToMarkdown([]byte(unknown)); got != "inside" {
		t.Errorf("an unknown node renders its children, got %q", got)
	}
	built := BuildADFFromMarkdown("one\n\n  \n\ntwo")
	if content, _ := built["content"].([]map[string]interface{}); len(content) != 2 {
		t.Errorf("blank paragraphs must be skipped, got %v", built["content"])
	}
}
