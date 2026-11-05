package agentjobs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	reqLine   = "I1028 23:%02d:%02d.697760     604 http_helpers.go:359] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse Trace: 0x1\n"
	otherLine = "I1028 23:39:31.064907    2658 http_helpers.go:359] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels Trace: 0x2\n"
)

func writeLogAt(t *testing.T, content string, mtime time.Time) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agy.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadAgyActivityCountsModelRequests(t *testing.T) {
	mtime := time.Date(2026, 10, 28, 23, 45, 0, 0, time.Local)
	content := strings.Join([]string{
		sprintfReq(34, 14), otherLine, sprintfReq(35, 7), "E1028 23:38:34.427244 29 client.go:83] Post failed: unexpected EOF\n", sprintfReq(36, 53), "not a log line\n",
	}, "")
	a, err := ReadAgyActivity(writeLogAt(t, content, mtime))
	if err != nil {
		t.Fatal(err)
	}
	if a.Requests != 3 {
		t.Errorf("Requests = %d, want 3 (other lines are not model requests)", a.Requests)
	}
	if want := time.Date(2026, 10, 28, 23, 36, 53, 0, time.Local); !a.LastRequest.Equal(want) {
		t.Errorf("LastRequest = %v, want %v", a.LastRequest, want)
	}
	if !a.Modified.Equal(mtime) {
		t.Errorf("Modified = %v, want %v", a.Modified, mtime)
	}
}

func sprintfReq(min, sec int) string {
	return fmt.Sprintf(reqLine, min, sec)
}

func TestReadAgyActivityKeepsTheLogsYearAcrossNewYear(t *testing.T) {
	mtime := time.Date(2027, 1, 2, 10, 0, 0, 0, time.Local)
	a, err := ReadAgyActivity(writeLogAt(t, "I1231 23:59:59.100000 1 x.go:1] streamGenerateContent\n", mtime))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 12, 31, 23, 59, 59, 0, time.Local); !a.LastRequest.Equal(want) {
		t.Errorf("LastRequest = %v, want the December of the year before the log was written: %v", a.LastRequest, want)
	}
}

func TestReadAgyActivityOfLogWithoutRequests(t *testing.T) {
	a, err := ReadAgyActivity(writeLogAt(t, otherLine, time.Now()))
	if err != nil || a.Requests != 0 || !a.LastRequest.IsZero() || a.Modified.IsZero() {
		t.Errorf("activity = %+v, err = %v", a, err)
	}
}

func TestReadAgyActivityErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.log")
	if a, err := ReadAgyActivity(missing); err != nil || a != (AgyActivity{}) {
		t.Errorf("a missing log is the zero activity: %+v, %v", a, err)
	}
	if _, err := ReadAgyActivity(t.TempDir()); err == nil {
		t.Error("a directory cannot be read as a log")
	}
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, nil, 0o644)
	if _, err := ReadAgyActivity(filepath.Join(file, "agy.log")); err == nil {
		t.Error("a path below a regular file is an error, not 'missing'")
	}
}

func TestAgyLogPathSitsInTheJobDirectory(t *testing.T) {
	l := NewLedger("/ledger")
	if got, want := l.AgyLogPath("J-1"), filepath.Join("/ledger", "J-1", "agy.log"); got != want {
		t.Errorf("AgyLogPath = %q, want %q", got, want)
	}
}
