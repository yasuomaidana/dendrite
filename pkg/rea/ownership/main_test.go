package ownership

import (
	"testing"

	"Dendrite/pkg/planesvc/planetest"
)

// The board adapter talks to Plane through the real dendrite-plane sidecar, like the ticketlock tests.
func TestMain(m *testing.M) { planetest.Main(m) }
