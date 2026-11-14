package bootstrap

import (
	"os"
	"testing"
)

// TestMain points DENDRITE_HOME at a temporary directory so Prepare never writes the
// developer's ~/.dendrite/rag.env.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "dendrite-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("DENDRITE_HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
