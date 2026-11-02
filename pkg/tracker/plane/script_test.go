package plane

import (
	"testing"

	"Dendrite/pkg/httpscript"
)

// rule and scriptedPlane are the shared scripted HTTP server under the names these tests use.
type (
	rule          = httpscript.Rule
	scriptedPlane = httpscript.Server
)

// on writes a rule as a call.
var on = httpscript.On

func newScriptedPlane(t *testing.T, rules ...rule) *scriptedPlane {
	t.Helper()
	return httpscript.New(t, rules...)
}
