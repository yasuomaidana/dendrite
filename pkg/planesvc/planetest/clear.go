package planetest

import (
	"strings"
	"testing"
)

// ClearSupported reports what to expect of a call that cleared the parent of ref, given whether the sidecar
// can clear one (protocol 1.5 and newer). A sidecar that cannot must have refused, naming the fix, and left
// the parent alone: that is checked here and false is returned. True means the call should have worked, and
// the caller checks the result.
func (b *Board) ClearSupported(t testing.TB, ref string, err error, supported bool) bool {
	t.Helper()
	if supported {
		return true
	}
	if err == nil || !strings.Contains(err.Error(), "dendrite update --dev") || b.ParentOf(ref) == "" {
		t.Fatalf("a sidecar that cannot clear a parent must refuse and keep it: err %v, parent %q", err, b.ParentOf(ref))
	}
	return false
}
