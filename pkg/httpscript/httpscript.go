// Package httpscript serves canned HTTP answers to tests. Rules are tried in order and the first
// one whose method and path match answers; every request is recorded.
package httpscript

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// T is the part of *testing.T the server uses.
type T interface {
	Helper()
	Cleanup(func())
}

// Rule answers requests whose method equals Method and whose path contains Contains (either may
// be empty to match anything) with Status and Body.
type Rule struct {
	Method, Contains string
	Status           int
	Body             string
}

// entry is a rule, or a handler for requests matching the same way.
type entry struct {
	Rule
	do http.HandlerFunc
}

// Server is a started test server with the requests it has seen, as "METHOD /path".
type Server struct {
	*httptest.Server
	mu    sync.Mutex
	rules []entry
	Calls []string
}

// New starts a server that answers from rules; unmatched requests get 200 and an empty JSON object.
// The server closes when the test ends.
func New(t T, rules ...Rule) *Server {
	t.Helper()
	s := &Server{}
	for _, r := range rules {
		s.rules = append(s.rules, entry{Rule: r})
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// HandleFunc makes fn answer requests matching method and contains, ahead of the rules given before.
func (s *Server) HandleFunc(method, contains string, fn http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append([]entry{{Rule: Rule{Method: method, Contains: contains}, do: fn}}, s.rules...)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.Calls = append(s.Calls, r.Method+" "+r.URL.Path)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	s.mu.Lock()
	rules := s.rules
	s.mu.Unlock()
	for _, ru := range rules {
		if (ru.Method == "" || ru.Method == r.Method) && strings.Contains(r.URL.Path, ru.Contains) {
			if ru.do != nil {
				ru.do(w, r)
				return
			}
			w.WriteHeader(ru.Status)
			_, _ = w.Write([]byte(ru.Body))
			return
		}
	}
	_, _ = w.Write([]byte(`{}`))
}

// Called counts the recorded requests that start with prefix (for example "POST /api/v1/").
func (s *Server) Called(prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// On is a Rule written as a call: method, path part, status and body.
func On(method, contains string, status int, body string) Rule {
	return Rule{Method: method, Contains: contains, Status: status, Body: body}
}
