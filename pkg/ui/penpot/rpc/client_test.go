package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secret = "tok-secret-123"

// fake answers each command with a canned body and records what it was sent.
type fake struct {
	t       *testing.T
	answers map[string]string
	status  map[string]int
	seen    map[string]map[string]any
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	command := strings.TrimPrefix(r.URL.Path, "/api/rpc/command/")
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Token "+secret || r.Header.Get("Accept") != "application/json" {
		f.t.Errorf("%s: method %s auth %q accept %q", command, r.Method, r.Header.Get("Authorization"), r.Header.Get("Accept"))
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.seen[command] = body
	if s := f.status[command]; s != 0 {
		w.WriteHeader(s)
	}
	_, _ = w.Write([]byte(f.answers[command]))
}

func newClient(t *testing.T, answers map[string]string, status map[string]int) (*Client, *fake) {
	t.Helper()
	f := &fake{t: t, answers: answers, status: status, seen: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/", secret, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestTypedCommandsDecodeAndSendKebabParams(t *testing.T) {
	c, f := newClient(t, map[string]string{
		"get-profile":  `{"id":"u1","email":"dev@dendrite.ai","fullname":"Dev"}`,
		"get-teams":    `[{"id":"t1","name":"Default"}]`,
		"get-projects": `[{"id":"p1","name":"UI","team-id":"t1"}]`,
		"get-file":     `{"id":"f1","name":"Library","revn":7,"data":{"pages":[]}}`,
		"update-file":  ``,
	}, nil)
	ctx := context.Background()

	if p, err := c.GetProfile(ctx); err != nil || p.Email != "dev@dendrite.ai" {
		t.Fatalf("profile %+v %v", p, err)
	}
	if teams, err := c.GetTeams(ctx); err != nil || len(teams) != 1 || teams[0].Name != "Default" {
		t.Fatalf("teams %+v %v", teams, err)
	}
	if projects, err := c.GetProjects(ctx, "t1"); err != nil || projects[0].TeamID != "t1" || f.seen["get-projects"]["team-id"] != "t1" {
		t.Fatalf("projects %+v %v seen %v", projects, err, f.seen["get-projects"])
	}
	if file, err := c.GetFile(ctx, "f1"); err != nil || file.Revn != 7 || f.seen["get-file"]["id"] != "f1" {
		t.Fatalf("file %+v %v", file, err)
	}
	if err := c.UpdateFile(ctx, UpdateFileParams{ID: "f1", SessionID: "s", Revn: 7, Changes: []map[string]any{{"type": "mod-obj"}}}); err != nil {
		t.Fatal(err)
	}
	if got := f.seen["update-file"]; got["session-id"] != "s" || got["revn"] != float64(7) {
		t.Errorf("update-file body %v", got)
	}
	if f.seen["get-profile"] == nil || len(f.seen["get-profile"]) != 0 {
		t.Errorf("a command without params sends an empty object, got %v", f.seen["get-profile"])
	}
}

func TestFailuresCarryPenpotsTypeAndHintButNeverTheToken(t *testing.T) {
	c, _ := newClient(t, map[string]string{
		"get-profile": `{"type":"authentication","hint":"token expired"}`,
		"get-teams":   `<html>bad gateway</html>`,
	}, map[string]int{"get-profile": 401, "get-teams": 502})

	_, err := c.GetProfile(context.Background())
	var rpcErr *Error
	if !errors.As(err, &rpcErr) || rpcErr.Status != 401 || rpcErr.Type != "authentication" || !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("err = %v", err)
	}
	_, err = c.GetTeams(context.Background())
	if !errors.As(err, &rpcErr) || rpcErr.Status != 502 || rpcErr.Type != "" || strings.Contains(err.Error(), secret) {
		t.Fatalf("non-JSON failure = %v", err)
	}
}

func TestTransportAndDecodeErrors(t *testing.T) {
	c, _ := newClient(t, map[string]string{"get-profile": `not json`}, nil)
	if _, err := c.GetProfile(context.Background()); err == nil || !strings.Contains(err.Error(), "decoding") {
		t.Fatalf("decode err = %v", err)
	}
	if err := c.Call(context.Background(), "x", make(chan int), nil); err == nil {
		t.Fatal("unmarshalable params must fail")
	}
	if err := c.Call(context.Background(), "bad\x7f", nil, nil); err == nil {
		t.Fatal("a bad URL must fail")
	}
	down, _ := New("http://127.0.0.1:1", secret, nil)
	err := down.Call(context.Background(), "get-profile", nil, nil)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unreachable err = %v", err)
	}
}

func TestRedactHidesTheToken(t *testing.T) {
	if got := redact(errors.New("dial "+secret+" failed"), secret).Error(); strings.Contains(got, secret) {
		t.Fatalf("token leaked: %s", got)
	}
}

func TestNewNeedsATokenAndDefaultsTheBaseURL(t *testing.T) {
	if _, err := New("", "  ", nil); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v", err)
	}
	c, err := New("", secret, nil)
	if err != nil || c.baseURL != DefaultBaseURL || c.http == nil {
		t.Fatalf("client %+v err %v", c, err)
	}
}

func TestNewFromEnvReadsTheSlots(t *testing.T) {
	t.Setenv(TokenEnv, "")
	if _, err := NewFromEnv(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty slot err = %v", err)
	}
	t.Setenv(TokenEnv, secret)
	t.Setenv(BaseURLEnv, "http://example.invalid/")
	c, err := NewFromEnv()
	if err != nil || c.baseURL != "http://example.invalid" {
		t.Fatalf("client %+v err %v", c, err)
	}
}

func TestEmptyAnswerIsFineWhenOutIsRequested(t *testing.T) {
	c, _ := newClient(t, map[string]string{"get-profile": ``}, nil)
	var p Profile
	if err := c.Call(context.Background(), "get-profile", nil, &p); err != nil {
		t.Fatal(err)
	}
}
