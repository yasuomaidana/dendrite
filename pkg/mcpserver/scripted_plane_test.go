package mcpserver

import (
	"testing"

	"Dendrite/pkg/httpscript"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
)

// apiRule and scriptedPlane are the shared scripted HTTP server under the names these tests use.
type (
	apiRule       = httpscript.Rule
	scriptedPlane = httpscript.Server
)

// on writes a rule as a call.
var on = httpscript.On

func newScriptedPlane(t *testing.T, rules ...apiRule) *scriptedPlane {
	t.Helper()
	return httpscript.New(t, rules...)
}

// planeClientFor is a Plane client for the scripted server.
func planeClientFor(t *testing.T, sp *scriptedPlane) *plane.Client {
	t.Helper()
	return testkit.Build(t, func() (*plane.Client, error) {
		return plane.NewClient(plane.Config{BaseURL: sp.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "token"}, nil)
	})
}

// toolsWith registers every default tool against a Plane client built from sp (nil for none).
func toolsWith(t *testing.T, sp *scriptedPlane, devStatus string) *Server {
	t.Helper()
	srv := NewServer(ServerConfig{})
	deps := Dependencies{DevStatus: devStatus}
	if sp != nil {
		deps.PlaneClient = planeClientFor(t, sp)
	}
	RegisterDefaultTools(srv, deps)
	return srv
}

// noEnvPlane makes plane.NewClientFromEnv fail, as it does where no API key is configured.
func noEnvPlane(t *testing.T) {
	t.Helper()
	t.Setenv("PLANE_API_KEY", "")
}
