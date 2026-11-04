// Package mcptest holds the helpers the tests of the MCP tool packages share: calling a tool, a scripted Plane,
// offline stand-ins and the package TestMain.
package mcptest

import (
	"Dendrite/pkg/httpscript"
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
)

// ApiRule and ScriptedPlane are the shared scripted HTTP server under the names these tests use.
type (
	ApiRule       = httpscript.Rule
	ScriptedPlane = httpscript.Server
)

func AssertToolCallSuccess(t *testing.T, srv *Server, name, argsJSON string) {
	t.Helper()
	req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": %q, "arguments": %s}}`, name, argsJSON)
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp == nil || resp.Error != nil {
		t.Fatalf("tool %s with args %s failed: %v", name, argsJSON, resp)
	}
}

func CallTool(t *testing.T, srv *Server, name, args string) (string, bool) {
	t.Helper()
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp.Error != nil {
		return resp.Error.Message, true
	}
	res, ok := resp.Result.(*CallToolResult)
	if !ok {
		v := resp.Result.(CallToolResult)
		res = &v
	}
	return res.Content[0].Text, res.IsError
}

// InEmptyDir runs the test in a fresh directory that is not a git repository and whose .dendrite
// is a file, so every tool that reads the working tree or its state fails.
func InEmptyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, ".dendrite"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func NewDefaultServerForProfileTest() *Server {
	s := NewServer(ServerConfig{Name: "t", Version: "1"})
	catalog.RegisterDefaultTools(s, Dependencies{DevStatus: "DEV_STATUS.md"})
	return s
}

// NoEnvPlane makes plane.NewClientFromEnv fail, as it does where no API key is configured.
func NoEnvPlane(t *testing.T) {
	t.Helper()
	t.Setenv("PLANE_API_KEY", "")
}

// on writes a rule as a call.
var On = httpscript.On

// PlaneClientFor is a Plane client for the scripted server.
func PlaneClientFor(t *testing.T, sp *ScriptedPlane) *plane.Client {
	t.Helper()
	return testkit.Build(t, func() (*plane.Client, error) {
		return plane.NewClient(plane.Config{BaseURL: sp.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "token"}, nil)
	})
}

func Rpc(t *testing.T, s *Server, method string, params interface{}) *JSONRPCResponse {
	t.Helper()
	raw, _ := json.Marshal(params)
	req, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: raw})
	resp := s.HandleRequest(context.Background(), req)
	if resp == nil {
		t.Fatalf("%s: nil response", method)
	}
	return resp
}

func RunToolFailures(t *testing.T, srv *Server, cases []ToolFailure) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			for k, v := range c.Env {
				t.Setenv(k, v)
			}
			text, isErr := CallTool(t, srv, c.Tool, c.Args)
			if !isErr || !strings.Contains(text, c.Want) {
				t.Fatalf("%s %s = %q (error %v), want an error containing %q", c.Tool, c.Args, text, isErr, c.Want)
			}
		})
	}
}

// ToolFailure is one tool call that must come back as an error naming what went wrong.
type ToolFailure struct {
	Name string
	Tool string
	Args string
	Env  map[string]string
	Want string
}

// ToolsWith registers every default tool against a Plane client built from sp (nil for none).
func ToolsWith(t *testing.T, sp *ScriptedPlane, devStatus string) *Server {
	t.Helper()
	srv := NewServer(ServerConfig{})
	deps := Dependencies{DevStatus: devStatus}
	if sp != nil {
		deps.PlaneClient = PlaneClientFor(t, sp)
	}
	catalog.RegisterDefaultTools(srv, deps)
	return srv
}

// Main runs a package's tests with retrieval served from the dendrite-rag stand-in, never from a sidecar that
// happens to be installed on the machine, and Plane from the real dendrite-plane build.
func Main(m *testing.M) {
	cleanRag, err := ragtest.Install()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cleanPlane, err := planetest.Install()
	if err != nil {
		cleanRag()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanPlane()
	cleanRag()
	os.Exit(code)
}
