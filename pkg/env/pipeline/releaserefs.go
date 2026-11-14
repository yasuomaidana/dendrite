package pipeline

import (
	_ "embed"
	"strings"

	"Dendrite/pkg/directive"
)

// ReleaseRefsScriptPath is the sibling-reference check, relative to the project root.
const ReleaseRefsScriptPath = "scripts/check-release-refs.sh"

// releaseRefsJobName is the job (and so the status check) every pull request must pass.
const releaseRefsJobName = "release refs"

// releaseRefsScript fails when a sibling repository is referenced by anything but a release or its main branch.
//
//go:embed releaserefs.sh
var releaseRefsScript []byte

func init() {
	directive.Register(directive.Artifact{Path: ReleaseRefsScriptPath, Content: releaseRefsContent, Owns: ownsDBAccess})
}

// releaseRefsContent writes the script for every project: a sibling can be referenced from any stack.
func releaseRefsContent(string, directive.Set) []byte { return releaseRefsScript }

// writeReleaseRefsJob adds the release-references job.
func writeReleaseRefsJob(b *strings.Builder) {
	b.WriteString("  release-refs:\n    name: " + scalar(releaseRefsJobName) + "\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - name: Checkout\n        uses: actions/checkout@v4\n" +
		"      - name: Siblings are referenced by release or main only\n        run: bash " + ReleaseRefsScriptPath + "\n")
}
