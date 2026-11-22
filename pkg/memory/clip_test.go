package memory_test

import (
	"testing"

	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

func TestClipCutsOnACharacterBoundary(t *testing.T) {
	testkit.Equal(t, "short", memory.Clip("abc", 5), "abc")
	testkit.Equal(t, "cut", memory.Clip("abcdef", 3), "abc...")
	testkit.Equal(t, "multi-byte", memory.Clip("aé", 2), "a...") // é is 2 bytes: cutting at 2 would split it
}
