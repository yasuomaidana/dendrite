package session

import (
	"Dendrite/cmd/dendrite/internal/clitest"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintSynapsePeers_OneLinePerSynapseAndSilentOtherwise(t *testing.T) {
	clitest.PlainOutput(t)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	self, peer := filepath.Join(t.TempDir(), "docula"), filepath.Join(t.TempDir(), "oxipix")
	for _, d := range []string{self, peer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		clitest.MigrateGit(t, d, "init", "-q")
	}
	clitest.SaveRequestSynapse(t, "mesh", []clitest.ReqMember{{Alias: "doc", Dir: self, Plane: "DOC"}, {Alias: "oxipix", Dir: peer, Plane: "OXI"}})
	clitest.SaveRequestSynapse(t, "other", []clitest.ReqMember{{Alias: "doc", Dir: self, Plane: "DOC"}, {Alias: "tax", Dir: peer, Plane: ""}})

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
