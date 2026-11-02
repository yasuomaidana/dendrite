package ticketlock

import (
	"Dendrite/pkg/testkit"
	"testing"

	"Dendrite/pkg/planesvc/planetest"
)

// The lifecycle talks to Plane through the real dendrite-plane sidecar, like the planesvc tests.
func TestMain(m *testing.M) { planetest.Main(m) }

// writeRegularFile makes a regular file; a path below it cannot be created, which breaks any store placed there.
func writeRegularFile(t *testing.T) string { return testkit.RegularFile(t) }
