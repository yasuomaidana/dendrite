package ticketlock

import (
	"strings"
	"testing"
)

const twoFilePatch = `diff --git a/pkg/a.go b/pkg/a.go
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -1,2 +1,3 @@
 package a
+func New() {}
-func Old() {}
@@ -10,1 +11,2 @@
+// second hunk
+var x = 1
diff --git a/pkg/a_test.go b/pkg/a_test.go
new file mode 100644
--- /dev/null
+++ b/pkg/a_test.go
@@ -0,0 +1,1 @@
+func TestNew(t *testing.T) {}
diff --git a/old.txt b/new.txt
similarity index 100%
rename from old.txt
rename to new.txt
`

func TestSplitHunksCutsEveryFileAtItsHunkHeaders(t *testing.T) {
	hunks := splitHunks(twoFilePatch)
	if len(hunks) != 4 {
		t.Fatalf("%d hunks: %+v", len(hunks), hunks)
	}
	if hunks[0].Path != "pkg/a.go" || !strings.HasPrefix(hunks[0].Text, "@@ -1,2 +1,3 @@") || !strings.Contains(hunks[0].Text, "+func New() {}") || strings.Contains(hunks[0].Text, "second hunk") {
		t.Errorf("first hunk = %+v", hunks[0])
	}
	if hunks[1].Path != "pkg/a.go" || !strings.Contains(hunks[1].Text, "+// second hunk") {
		t.Errorf("second hunk = %+v", hunks[1])
	}
	if hunks[2].Path != "pkg/a_test.go" || !hunks[2].Test || hunks[0].Test {
		t.Errorf("test files are marked: %+v %+v", hunks[2], hunks[0])
	}
	if hunks[3].Path != "new.txt" || !strings.Contains(hunks[3].Text, "rename from old.txt") {
		t.Errorf("a rename has no @@ and is one hunk: %+v", hunks[3])
	}
}

func TestSplitHunksSkipsWhatIsNotAPatch(t *testing.T) {
	for _, in := range []string{"", "no patch here", "garbage\ndiff --git broken\n+x\n"} {
		if got := splitHunks(in); len(got) != 0 {
			t.Errorf("%q gave %+v", in, got)
		}
	}
}

func TestChangeSummaryCountsLinesPerFile(t *testing.T) {
	got := changeSummary(splitHunks(twoFilePatch))
	for _, want := range []string{"new.txt: +0 -0\n", "pkg/a.go: +3 -1\n", "pkg/a_test.go (test): +1 -0\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	if changeSummary(nil) != "" {
		t.Error("no hunks, no summary")
	}
}

func TestLooksLikeTestByNameAndDirectory(t *testing.T) {
	for path, want := range map[string]bool{
		"a_test.go": true, "web/a.test.ts": true, "web/a.spec.tsx": true, "x/tests/it.rs": true, "x/test/it.rs": true,
		"x/__tests__/a.js": true, "py/test_a.py": true, "pkg/x/testdata/a.toml": true,
		"main.go": false, "pkg/attest.go": false, "pkg/contest/a.go": false,
	} {
		if looksLikeTest(path) != want {
			t.Errorf("looksLikeTest(%q) = %v", path, !want)
		}
	}
}

func TestCapTextKeepsTheStartAndSaysItCut(t *testing.T) {
	if capText("short", 10) != "short" {
		t.Error("short text passes")
	}
	if got := capText(strings.Repeat("x", 30), 10); got != strings.Repeat("x", 10)+"\n[cut]\n" {
		t.Errorf("%q", got)
	}
}
