package plane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// rule answers requests whose method is method and whose path contains contains (either may be
// empty to match anything) with status and body.
type rule struct {
	method, contains string
	status           int
	body             string
}

// scriptedPlane answers from an ordered list of rules (first match wins) and records every request
// as "METHOD path". Unmatched requests get 200 with an empty JSON object.
type scriptedPlane struct {
	*httptest.Server
	mu    sync.Mutex
	rules []rule
	Calls []string
}

func newScriptedPlane(t *testing.T, rules ...rule) *scriptedPlane {
	t.Helper()
	sp := &scriptedPlane{rules: rules}
	sp.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sp.mu.Lock()
		sp.Calls = append(sp.Calls, r.Method+" "+r.URL.Path)
		sp.mu.Unlock()
		for _, ru := range sp.rules {
			if (ru.method == "" || ru.method == r.Method) && strings.Contains(r.URL.Path, ru.contains) {
				w.WriteHeader(ru.status)
				_, _ = w.Write([]byte(ru.body))
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(sp.Close)
	return sp
}

func (sp *scriptedPlane) called(prefix string) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	n := 0
	for _, c := range sp.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
