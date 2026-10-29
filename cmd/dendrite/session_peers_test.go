package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintSynapsePeers_OneLinePerSynapseAndSilentOtherwise(t *testing.T) {
	plainOutput(t)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	self, peer := filepath.Join(t.TempDir(), "docula"), filepath.Join(t.TempDir(), "oxipix")
	for _, d := range []string{self, peer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		migrateGit(t, d, "init", "-q")
	}
	saveRequestSynapse(t, "mesh", []reqMember{{"doc", self, "DOC"}, {"oxipix", peer, "OXI"}})
	saveRequestSynapse(t, "other", []reqMember{{"doc", self, "DOC"}, {"tax", peer, ""}})

	var out bytes.Buffer
	printSynapsePeers(self, &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one line per synapse, got:\n%s", out.String())
	}
	if !strings.Contains(lines[0], "synapse mesh: you are doc; peers oxipix(peer,OXI)") || !strings.Contains(lines[1], "synapse other: you are doc; peers tax(peer,?)") {
		t.Errorf("unexpected lines:\n%s", out.String())
	}

	out.Reset()
	printSynapsePeers(t.TempDir(), &out) // not a repository
	if out.Len() != 0 {
		t.Errorf("outside a repository nothing is printed, got %q", out.String())
	}
}
