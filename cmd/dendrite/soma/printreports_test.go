package soma

import (
	"bytes"
	"testing"

	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
)

func TestPrintWorkReportsMarksKeptCopiesAndHidesThemUnlessVerbose(t *testing.T) {
	reports := []workcopy.Report{
		{Path: "/w/a", Action: "removed", Reason: "merged"},
		{Path: "/w/b", Action: "kept", Reason: "dirty"},
	}
	var quiet, verbose bytes.Buffer
	printWorkReports(&quiet, reports, false)
	printWorkReports(&verbose, reports, true)
	testkit.Contains(t, quiet.String(), "[✓] removed /w/a (merged)")
	testkit.NotContains(t, quiet.String(), "/w/b")
	testkit.Contains(t, verbose.String(), "[✓] removed /w/a (merged)", "[=] kept /w/b (dirty)")
}
