package immune_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"Dendrite/pkg/immune"
	"Dendrite/pkg/testkit"
)

func newLedger(t *testing.T) immune.Ledger {
	t.Helper()
	return immune.Ledger{Path: filepath.Join(t.TempDir(), "immune", "scans.jsonl")}
}

func entry(path, digest string) immune.Entry {
	return immune.Entry{Time: time.Unix(1, 0).UTC(), Path: path, Skill: "s", Digest: digest, Score: 3, Verdict: immune.Approve, ScannerVersion: "9.9"}
}

// lines counts the ledger's lines as they are on disk.
func lines(t *testing.T, l immune.Ledger) int {
	t.Helper()
	data, err := os.ReadFile(l.Path)
	testkit.Check(t, err)
	return strings.Count(string(data), "\n")
}

// The same result for the same path is recorded once, whenever it is taken again; a new result
// for that path, or the old one coming back after it, is a new line.
func TestLedgerRecordIsIdempotent(t *testing.T) {
	l := newLedger(t)
	later := entry("/skills/a", "d1")
	later.Time = time.Unix(99, 0).UTC()
	for i, step := range []struct {
		e     immune.Entry
		added bool
	}{
		{entry("/skills/a", "d1"), true},
		{later, false},
		{entry("/skills/b", "d1"), true},
		{entry("/skills/a", "d1"), false},
		{entry("/skills/a", "d2"), true},
		{entry("/skills/a", "d1"), true},
	} {
		added, err := l.Record(step.e)
		testkit.Check(t, err)
		if added != step.added {
			t.Errorf("step %d: added = %v, want %v", i, added, step.added)
		}
	}
	testkit.Equal(t, "lines", lines(t, l), 4)
}

func TestLedgerFindReturnsTheLatestEntryOfADigest(t *testing.T) {
	l := newLedger(t)
	if _, found, err := l.Find("d1"); err != nil || found {
		t.Fatalf("empty ledger: found = %v, err = %v", found, err)
	}
	held := entry("/skills/a", "d1")
	held.Verdict, held.Score = immune.Hold, 30
	for _, e := range []immune.Entry{entry("/skills/a", "d1"), entry("/skills/b", "d2"), held} {
		_, err := l.Record(e)
		testkit.Check(t, err)
	}
	got, found, err := l.Find("d1")
	testkit.Check(t, err)
	if !found || got.Verdict != immune.Hold {
		t.Fatalf("Find(d1) = %+v, %v; want the later, held entry", got, found)
	}
	if _, found, _ = l.Find("d3"); found {
		t.Fatal("Find of a digest never scanned must not find anything")
	}
}

// Sessions scanning at the same time neither lose nor interleave lines, and the same result
// arriving from many of them is still recorded once.
func TestLedgerRecordIsSafeUnderConcurrency(t *testing.T) {
	l := newLedger(t)
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := l.Record(entry(fmt.Sprintf("/skills/%d", i), "d")); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := l.Record(entry("/skills/shared", "d")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	entries, err := l.Entries()
	testkit.Check(t, err)
	testkit.Equal(t, "entries", len(entries), 25)
	testkit.Equal(t, "lines", lines(t, l), 25)
}

// A ledger that cannot be read or written is an error, never an empty ledger.
func TestLedgerFailures(t *testing.T) {
	l := newLedger(t)
	testkit.WriteFile(t, l.Path, "{\"path\":\"/a\"}\n\nnot json\n", 0o644)
	if _, err := l.Entries(); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("a broken line must be named: %v", err)
	}
	if _, err := l.Record(entry("/a", "d")); err == nil {
		t.Fatal("Record on a broken ledger must fail")
	}
	if _, _, err := l.Find("d"); err == nil {
		t.Fatal("Find on a broken ledger must fail")
	}

	blocked := immune.Ledger{Path: filepath.Join(testkit.RegularFile(t), "scans.jsonl")}
	if _, err := blocked.Record(entry("/a", "d")); err == nil {
		t.Fatal("a ledger below a regular file must fail")
	}
	dir := immune.Ledger{Path: t.TempDir()}
	if _, err := dir.Record(entry("/a", "d")); err == nil {
		t.Fatal("a ledger that is a directory must fail")
	}
	locked := newLedger(t)
	testkit.Check(t, os.MkdirAll(locked.Path+".lock", 0o755))
	if _, err := locked.Record(entry("/a", "d")); err == nil {
		t.Fatal("a ledger whose lock cannot be taken must fail")
	}
	readOnly := newLedger(t)
	_, err := readOnly.Record(entry("/a", "d"))
	testkit.Check(t, err)
	testkit.Check(t, os.Chmod(filepath.Dir(readOnly.Path), 0o500))
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(readOnly.Path), 0o700) })
	if _, err := readOnly.Record(entry("/b", "d")); err == nil {
		t.Fatal("a ledger that cannot be replaced must fail")
	}
}

func TestDigest(t *testing.T) {
	skill := fixture(t, map[string]string{"SKILL.md": skillHead, "scripts/run.sh": "echo hi\n", ".git/HEAD": "ref: refs/heads/main\n"})
	first, err := immune.Digest(skill)
	testkit.Check(t, err)
	again, err := immune.Digest(skill)
	testkit.Check(t, err)
	testkit.Equal(t, "repeat", again, first)

	testkit.WriteFile(t, filepath.Join(skill, ".git", "HEAD"), "ref: refs/heads/other\n", 0o644)
	unchanged, _ := immune.Digest(skill)
	testkit.Equal(t, "git directory is not part of the skill", unchanged, first)

	for name, change := range map[string]func(){
		"content": func() { testkit.WriteFile(t, filepath.Join(skill, "scripts/run.sh"), "echo ho\n", 0o644) },
		"rename": func() {
			testkit.Check(t, os.Rename(filepath.Join(skill, "scripts/run.sh"), filepath.Join(skill, "scripts/go.sh")))
		},
		"added": func() { testkit.WriteFile(t, filepath.Join(skill, "extra.md"), "", 0o644) },
	} {
		before, _ := immune.Digest(skill)
		change()
		after, _ := immune.Digest(skill)
		if after == before {
			t.Errorf("%s: the digest did not change", name)
		}
	}

	file, err := immune.Digest(filepath.Join(skill, "SKILL.md"))
	testkit.Check(t, err)
	if file == "" || file == first {
		t.Fatalf("digest of one file = %q", file)
	}
	if _, err := immune.Digest(filepath.Join(skill, "absent")); err == nil {
		t.Fatal("a missing path has no digest")
	}

	// A link is not content of the skill; a file that cannot be read has no digest.
	before, _ := immune.Digest(skill)
	testkit.Check(t, os.Symlink(filepath.Join(skill, "SKILL.md"), filepath.Join(skill, "link.md")))
	linked, err := immune.Digest(skill)
	testkit.Check(t, err)
	testkit.Equal(t, "with a link", linked, before)
	testkit.WriteFile(t, filepath.Join(skill, "secret"), "x", 0o000)
	if _, err := immune.Digest(skill); err == nil {
		t.Fatal("an unreadable file must fail the digest")
	}
	if _, err := (immune.Scanner{Bin: versionedScanner(t, "1.0")}).Scan(context.Background(), skill); err == nil || !strings.Contains(err.Error(), "scan target") {
		t.Fatalf("a skill that cannot be read must fail the scan: %v", err)
	}
}

// versionedScanner stands in for SkillSpector at one version; every skill scores 0.
func versionedScanner(t *testing.T, version string) string {
	t.Helper()
	return scannerScript(t, `if [ "$1" = "--version" ]; then echo "SkillSpector v`+version+`"; exit 0; fi
echo '{"skill":{"name":"s"},"risk_assessment":{"score":0,"severity":"LOW"},"issues":[]}' > "$6"`)
}

func TestScanRefusesAScannerThatIsNotThePinnedVersion(t *testing.T) {
	skill := fixture(t, map[string]string{"SKILL.md": skillHead})
	bin := versionedScanner(t, "2.12.0")
	l := newLedger(t)
	_, err := immune.Scanner{Bin: bin, Pin: "2.13.0", Ledger: &l}.Scan(context.Background(), skill)
	if !errors.Is(err, immune.ErrPinMismatch) || !strings.Contains(err.Error(), "dendrite immune update") {
		t.Fatalf("another version than the pin: %v", err)
	}
	if entries, _ := l.Entries(); len(entries) != 0 {
		t.Fatalf("a refused scan must not reach the ledger: %+v", entries)
	}
	r, err := immune.Scanner{Bin: bin, Pin: "2.12.0"}.Scan(context.Background(), skill)
	testkit.Check(t, err)
	testkit.Equal(t, "verdict", r.Verdict, immune.Approve)
}

// Every scan reaches the ledger with the digest of what was scanned; scanning unchanged content
// again adds nothing, changed content adds a line.
func TestScanRecordsEveryResultInTheLedger(t *testing.T) {
	skill := fixture(t, map[string]string{"SKILL.md": skillHead})
	l := newLedger(t)
	clock := time.Unix(1000, 0)
	s := immune.Scanner{Bin: versionedScanner(t, "2.12.0"), Ledger: &l, Now: func() time.Time { clock = clock.Add(time.Minute); return clock }}
	r, err := s.Scan(context.Background(), skill)
	testkit.Check(t, err)
	want, _ := immune.Digest(skill)
	testkit.Equal(t, "report digest", r.Digest, want)
	_, err = s.Scan(context.Background(), skill)
	testkit.Check(t, err)
	testkit.Equal(t, "lines after the same scan twice", lines(t, l), 1)

	testkit.WriteFile(t, filepath.Join(skill, "SKILL.md"), skillHead+"More.\n", 0o644)
	_, err = s.Scan(context.Background(), skill)
	testkit.Check(t, err)
	entries, err := l.Entries()
	testkit.Check(t, err)
	testkit.Equal(t, "entries", len(entries), 2)
	testkit.Equal(t, "version", entries[0].ScannerVersion, "SkillSpector v2.12.0")
	testkit.Equal(t, "first digest", entries[0].Digest, want)
	testkit.Equal(t, "first time", entries[0].Time, time.Unix(1060, 0).UTC())

	// Without a clock of its own the scanner stamps entries with the time of the scan.
	other := newLedger(t)
	_, err = immune.Scanner{Bin: versionedScanner(t, "2.12.0"), Ledger: &other}.Scan(context.Background(), skill)
	testkit.Check(t, err)
	stamped, err := other.Entries()
	testkit.Check(t, err)
	if len(stamped) != 1 || time.Since(stamped[0].Time) > time.Minute {
		t.Fatalf("entry stamped by the default clock = %+v", stamped)
	}

	// A ledger that cannot be written fails the scan: a result that is not recorded did not pass.
	broken := immune.Ledger{Path: filepath.Join(testkit.RegularFile(t), "scans.jsonl")}
	s.Ledger = &broken
	if _, err := s.Scan(context.Background(), skill); err == nil || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("unwritable ledger: %v", err)
	}
}
