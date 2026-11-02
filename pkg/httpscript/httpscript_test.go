package httpscript

import (
	"io"
	"net/http"
	"testing"
)

func get(t *testing.T, method, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestRulesAnswerInOrderAndRequestsAreCounted(t *testing.T) {
	s := New(t,
		Rule{Method: "GET", Contains: "/a", Status: 201, Body: "first"},
		Rule{Contains: "/a", Status: 500, Body: "second"},
	)
	s.HandleFunc("POST", "/do", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) })
	if code, body := get(t, "GET", s.URL+"/a/b"); code != 201 || body != "first" {
		t.Errorf("first rule = %d %q", code, body)
	}
	if code, body := get(t, "DELETE", s.URL+"/a"); code != 500 || body != "second" {
		t.Errorf("method-less rule = %d %q", code, body)
	}
	if code, _ := get(t, "POST", s.URL+"/do"); code != 202 {
		t.Errorf("handler rule = %d", code)
	}
	if code, body := get(t, "GET", s.URL+"/nothing"); code != 200 || body != "{}" {
		t.Errorf("unmatched = %d %q", code, body)
	}
	if s.Called("GET /a") != 1 || s.Called("POST") != 1 || len(s.Calls) != 4 {
		t.Errorf("calls = %v", s.Calls)
	}
}

func TestOnBuildsARule(t *testing.T) {
	if got := On("GET", "/x", 201, "b"); got != (Rule{Method: "GET", Contains: "/x", Status: 201, Body: "b"}) {
		t.Fatalf("On = %+v", got)
	}
}
