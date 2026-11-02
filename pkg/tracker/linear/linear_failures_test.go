package linear

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func linearAt(t *testing.T, endpoint string) *Client {
	t.Helper()
	t.Setenv("LINEAR_FAILURE_KEY", "lin_secret")
	c, err := NewClient(Config{APITokenEnv: "LINEAR_FAILURE_KEY", Endpoint: endpoint}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func answer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
}

func TestExecuteQueryMapsEveryFailure(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	cases := []struct {
		name     string
		endpoint func() (string, func())
		vars     map[string]interface{}
		check    func(error) bool
	}{
		{"variables that cannot be encoded", serve(`{}`), map[string]interface{}{"x": make(chan int)}, func(err error) bool { return err != nil }},
		{"an endpoint that is not a URL", func() (string, func()) { return "http://%zz", func() {} }, nil, func(err error) bool { return err != nil }},
		{"a server that is down", func() (string, func()) { return closed.URL, func() {} }, nil, func(err error) bool { return err != nil }},
		{"a body that is not JSON", serve(`not json`), nil, func(err error) bool { return err != nil }},
		{"an authentication error", serve(`{"errors":[{"message":"Authentication required"}]}`), nil, func(err error) bool { return errors.Is(err, ErrUnauthorized) }},
		{"a not found error", serve(`{"errors":[{"message":"Entity not found"}]}`), nil, func(err error) bool { return errors.Is(err, ErrIssueNotFound) }},
		{"any other graphql error", serve(`{"errors":[{"message":"rate limited"}]}`), nil, func(err error) bool {
			return err != nil && strings.Contains(err.Error(), "graphql error: rate limited")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			endpoint, stop := c.endpoint()
			defer stop()
			if _, err := linearAt(t, endpoint).executeQuery(context.Background(), "query {}", c.vars); !c.check(err) {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func serve(body string) func() (string, func()) {
	return func() (string, func()) {
		s := answer(body)
		return s.URL, s.Close
	}
}

func TestOperationsRejectAnswersTheyCannotUse(t *testing.T) {
	t.Run("an issue that is not in the answer", func(t *testing.T) {
		s := answer(`{"data":{"issue":null}}`)
		defer s.Close()
		if _, err := linearAt(t, s.URL).GetIssue(context.Background(), "DEN-1"); !errors.Is(err, ErrIssueNotFound) {
			t.Fatalf("err = %v, want not found", err)
		}
	})
	t.Run("workflow states that are not a list", func(t *testing.T) {
		s := answer(`{"data":{"workflowStates":"broken"}}`)
		defer s.Close()
		if err := linearAt(t, s.URL).UpdateStatus(context.Background(), "DEN-1", "DONE"); err == nil {
			t.Fatal("expected a decoding error")
		}
	})
}

func TestValidateCredentialsTimesOut(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := linearAt(t, slow.URL).ValidateCredentials(ctx); !errors.Is(err, ErrValidationTimeout) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}
