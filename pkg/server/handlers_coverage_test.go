package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *Server {
	return NewServer(ServerConfig{Host: "127.0.0.1", Port: 0}, nil, nil)
}

func TestGraphHandlersRejectBadRequests(t *testing.T) {
	s := newTestServer()
	missing := t.TempDir() + "/does-not-exist"

	cases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		body    string
		want    int
	}{
		{"path wants POST", s.handleGraphPath, http.MethodGet, "", http.StatusMethodNotAllowed},
		{"path with a broken body", s.handleGraphPath, http.MethodPost, "{", http.StatusBadRequest},
		{"path without from and to", s.handleGraphPath, http.MethodPost, `{"from":"a"}`, http.StatusBadRequest},
		{"path in a missing project", s.handleGraphPath, http.MethodPost, `{"project_path":"` + missing + `","from":"a","to":"b"}`, http.StatusInternalServerError},
		{"explain with a broken body", s.handleGraphExplain, http.MethodPost, "{", http.StatusBadRequest},
		{"explain in a missing project", s.handleGraphExplain, http.MethodPost, `{"project_path":"` + missing + `","target":"x"}`, http.StatusInternalServerError},
		{"communities with a broken body", s.handleGraphCommunities, http.MethodPost, "{", http.StatusBadRequest},
		{"communities in a missing project", s.handleGraphCommunities, http.MethodPost, `{"project_path":"` + missing + `"}`, http.StatusInternalServerError},
		{"god nodes in a missing project", s.handleGraphGodNodes, http.MethodPost, `{"project_path":"` + missing + `"}`, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.handler(rec, httptest.NewRequest(c.method, "/x", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status = %d (%s), want %d", rec.Code, strings.TrimSpace(rec.Body.String()), c.want)
			}
		})
	}
}

func TestDecodePostAllowsAnAbsentBodyOnlyWhenOptional(t *testing.T) {
	var out struct{ A int }

	noBody := &http.Request{Method: http.MethodPost}
	if !decodePost(httptest.NewRecorder(), noBody, &out, true) {
		t.Fatal("an absent body must be accepted when optional")
	}

	rec := httptest.NewRecorder()
	if decodePost(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("")), &out, false) || rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty body must be a 400 when required, got %d", rec.Code)
	}
	if !decodePost(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("")), &out, true) {
		t.Fatal("an empty body must be accepted when optional")
	}
}
