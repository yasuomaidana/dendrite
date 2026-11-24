package insight

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/rea/analysis"
)

// DefaultADRDir is where the project keeps its Architecture Decision Records.
const DefaultADRDir = "docs/architecture/decisions"

var adrFile = regexp.MustCompile(`^ADR-(\d+)`)

// nextADRID is the id after the highest ADR-NNN already in dir (ADR-001 for a directory with none).
func nextADRID(dir string) string {
	entries, _ := os.ReadDir(dir)
	highest := 0
	for _, e := range entries {
		if m := adrFile.FindStringSubmatch(e.Name()); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > highest {
				highest = n
			}
		}
	}
	return fmt.Sprintf("ADR-%03d", highest+1)
}

// DraftADR writes a proposed Architecture Decision Record for analysis id into outDir and returns its path. The
// evidence bundle is its context; the decision and the consequences are left to a person, because FEAT-003 bars
// agents from implementing before a human signs the record off.
func DraftADR(svc *analysis.Service, id, outDir, title string) (string, error) {
	st, img, err := svc.Inspected(id)
	if err != nil {
		return "", err
	}
	if st.Stage != analysis.StageDone {
		return "", analysis.ErrNotDone
	}
	if strings.TrimSpace(outDir) == "" {
		outDir = DefaultADRDir
	}
	if strings.TrimSpace(title) == "" {
		title = "Cleanroom recreation of " + filepath.Base(st.Binary)
	}
	adrID := nextADRID(outDir)
	client := rea.NewClient(rea.TargetRemoteLinux, rea.DisassemblerEngine("dendrite-core"), nil)
	bundle := analysis.Bundle(st, img)
	_, markdown := client.DraftADR(bundle, adrID, title,
		fmt.Sprintf("Analysis %s of %s (%s, %s). Evidence follows; search the analysis with `dendrite retro search %s`.", st.ID, filepath.Base(st.Binary), st.Kind, st.Format, st.ID),
		"Pending: a person decides what to recreate and how before any implementation starts.",
		"Pending: to be written by the reviewer who signs this record off.",
		[]string{"reverse-engineering", "rea", st.Format})
	path := filepath.Join(outDir, adrID+".md")
	return path, fsutil.WriteFileAtomic(path, []byte(markdown))
}
