package tracker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLinkedSpecsReturnsDistinctIDsInFirstSeenOrder(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"none", "nothing to link", nil},
		{"one of each kind", "see ADR-7 and FEAT-12", []string{"ADR-7", "FEAT-12"}},
		{"repeats collapse", "FEAT-1 FEAT-2 FEAT-1 ADR-3 FEAT-2", []string{"FEAT-1", "FEAT-2", "ADR-3"}},
		{"needs word boundaries", "XFEAT-1 FEAT-", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LinkedSpecs(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("LinkedSpecs(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestCheckCredentialsMapsTheAnswer(t *testing.T) {
	errTimeout := errors.New("timed out")
	errDenied := errors.New("denied")

	serve := func(status int, delay time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(status)
		}))
	}
	get := func(url string) func(context.Context) (*http.Request, error) {
		return func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		}
	}

	ok, denied, forbidden, broken := serve(200, 0), serve(401, 0), serve(403, 0), serve(500, 0)
	slow := serve(200, CredentialsTimeout+500*time.Millisecond)
	defer func() {
		for _, s := range []*httptest.Server{ok, denied, forbidden, broken, slow} {
			s.Close()
		}
	}()

	cases := []struct {
		name    string
		build   func(context.Context) (*http.Request, error)
		wantErr error
		wantMsg string
	}{
		{"200 passes", get(ok.URL), nil, ""},
		{"401 is unauthorized", get(denied.URL), errDenied, ""},
		{"403 is unauthorized", get(forbidden.URL), errDenied, ""},
		{"500 names the status", get(broken.URL), nil, "credential validation failed: status 500"},
		{"a slow server times out", get(slow.URL), errTimeout, ""},
		{"a request that cannot be built", func(context.Context) (*http.Request, error) { return nil, errors.New("bad request") }, nil, "bad request"},
		{"an unreachable server", get("http://127.0.0.1:1"), nil, "connect"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckCredentials(context.Background(), http.DefaultClient, c.build, errTimeout, errDenied)
			switch {
			case c.wantErr == nil && c.wantMsg == "":
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			case c.wantErr != nil:
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), c.wantMsg) {
					t.Fatalf("err = %v, want it to mention %q", err, c.wantMsg)
				}
			}
		})
	}
}
