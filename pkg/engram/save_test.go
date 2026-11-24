package engram_test

import (
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/testkit"
)

func TestSaveRunsEngramSaveWithTheFlagsThatAreSet(t *testing.T) {
	for name, c := range map[string]struct{ project, typ, want string }{
		"type and project": {"dendrite", "decision", "save use X because Y --type decision --project dendrite"},
		"neither":          {"", "", "save use X because Y"},
	} {
		t.Run(name, func(t *testing.T) {
			dir, log := tools(t, map[string]string{"engram": engramScript})
			testkit.Check(t, engram.Save(context.Background(), dir+"/engram", c.project, c.typ, "use X", "because Y"))
			testkit.Equal(t, "arguments", strings.TrimSpace(calls(t, log)), c.want)
		})
	}
}

func TestSaveReportsEngramsFailure(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": `echo "database locked" >&2; exit 1`})
	err := engram.Save(context.Background(), dir+"/engram", "p", "note", "t", "b")
	if err == nil || !strings.Contains(err.Error(), "database locked") {
		t.Fatalf("the failure and Engram's message must be reported, got %v", err)
	}
}
