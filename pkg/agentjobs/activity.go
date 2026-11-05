package agentjobs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// AgyLogPath is where a job's agy process writes its own log (agy --log-file). Unlike agent.log, which
// only fills when agy prints its result, this file grows while the agent works: one line per model request.
func (l *Ledger) AgyLogPath(id string) string {
	return filepath.Join(l.JobDir(id), "agy.log")
}

// AgyActivity summarises an agy CLI log.
type AgyActivity struct {
	// Requests counts the model requests the agent has made.
	Requests int `json:"requests"`
	// LastRequest is when the newest request was made; zero when the log has none.
	LastRequest time.Time `json:"last_request,omitzero"`
	// Modified is the last write to the log; zero when there is no log.
	Modified time.Time `json:"modified,omitzero"`
}

// agyRequestLine matches the glog line agy writes for every model request, for example
// "I1028 23:36:53.697760  2321 http_helpers.go:359] URL: …:streamGenerateContent?alt=sse …".
var agyRequestLine = regexp.MustCompile(`^[IWE](\d{2})(\d{2}) (\d{2}):(\d{2}):(\d{2})\.\d+ .*streamGenerateContent`)

// ReadAgyActivity reads the agy log at path. A missing log is not an error: the agent simply has not
// started logging, and the zero value says so.
func ReadAgyActivity(path string) (AgyActivity, error) {
	var a AgyActivity
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return a, err
	}
	a.Modified = fi.ModTime()
	for _, line := range bytes.Split(data, []byte("\n")) {
		m := agyRequestLine.FindSubmatch(line)
		if m == nil {
			continue
		}
		a.Requests++
		a.LastRequest = glogTime(m, fi.ModTime())
	}
	return a, nil
}

// glogTime builds the time of a glog line. The line carries no year, so the year of the log's last write
// is used, and a date that would lie in the future belongs to the year before.
func glogTime(m [][]byte, modified time.Time) time.Time {
	n := func(b []byte) int { v, _ := strconv.Atoi(string(b)); return v }
	t := time.Date(modified.Year(), time.Month(n(m[1])), n(m[2]), n(m[3]), n(m[4]), n(m[5]), 0, modified.Location())
	if t.After(modified.Add(24 * time.Hour)) {
		t = t.AddDate(-1, 0, 0)
	}
	return t
}
