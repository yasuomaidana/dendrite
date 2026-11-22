package engram_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/testkit"
)

const exportJSON = `{"observations":[{"type":"note","title":"a","content":"alpha","created_at":"2026-11-22 19:32:15"},{"type":"decision","title":"b","content":"beta","created_at":"garbage"},{"type":"note","title":"gone","content":"x","created_at":"2026-11-22 19:32:15","deleted_at":"2026-11-22 20:00:00"}]}`

func TestExportReadsTheProjectsObservationsInTheGivenDirectory(t *testing.T) {
	dir, log := tools(t, map[string]string{"engram": `echo "$(pwd) $*" >> "$LOG"; printf '%s' '` + exportJSON + `' > "$2"`})
	work := t.TempDir()
	got, err := engram.Export(context.Background(), dir+"/engram", work)
	testkit.Check(t, err)
	if len(got) != 2 || got[0].Title != "a" || got[1].Content != "beta" { // "gone" is deleted: left out
		t.Fatalf("observations = %+v", got)
	}
	testkit.Equal(t, "created", got[0].Created(), time.Date(2026, 11, 22, 19, 32, 15, 0, time.UTC))
	if !got[1].Created().IsZero() {
		t.Fatal("an unreadable stamp is the zero time")
	}
	if !strings.Contains(calls(t, log), "export") {
		t.Fatalf("calls = %s", calls(t, log))
	}
}

func TestExportReportsEveryWayItCanFail(t *testing.T) {
	for name, body := range map[string]string{
		"engram fails":        `echo boom >&2; exit 1`,
		"writes no file":      `exit 0`,
		"writes garbage json": `echo '{nope' > "$2"`,
	} {
		t.Run(name, func(t *testing.T) {
			dir, _ := tools(t, map[string]string{"engram": body})
			if _, err := engram.Export(context.Background(), dir+"/engram", t.TempDir()); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSearchReturnsTheTitlesOfTheHits(t *testing.T) {
	dir, log := tools(t, map[string]string{"engram": `echo "$*" >> "$LOG"
cat <<'EOF2'
Found 2 memories:

[1] #1 (feedback) — agent-delegation-timeout
    "text"

[2] #4 (project) — the-other-one
EOF2`})
	got, err := engram.Search(context.Background(), dir+"/engram", t.TempDir(), "delegate coding", 5)
	testkit.Check(t, err)
	testkit.Equal(t, "titles", strings.Join(got, ","), "agent-delegation-timeout,the-other-one")
	testkit.Equal(t, "arguments", strings.TrimSpace(calls(t, log)), "search delegate coding --match any --limit 5")
}

func TestSearchWithNoHitsAndWithAFailure(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": `echo 'No memories found for: "x"'`})
	got, err := engram.Search(context.Background(), dir+"/engram", t.TempDir(), "x", 5)
	testkit.Check(t, err)
	testkit.Equal(t, "no hits", len(got), 0)

	dir, _ = tools(t, map[string]string{"engram": `exit 1`})
	if _, err := engram.Search(context.Background(), dir+"/engram", t.TempDir(), "x", 5); err == nil {
		t.Fatal("a failing search must be reported")
	}
}
