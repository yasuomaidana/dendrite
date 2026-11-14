package pipeline

import (
	"bytes"
	_ "embed"
	"strings"

	"Dendrite/pkg/directive"
)

// DBAccessScriptPath is the Diesel-only check, relative to the project root.
const DBAccessScriptPath = "scripts/check-db-access.sh"

// dbAccessJobName is the job (and so the status check) a Rust project's pull requests must pass.
const dbAccessJobName = "db-access (diesel only)"

// dbAccessScript fails when Rust code reaches a database other than through Diesel.
//
//go:embed dbaccess.sh
var dbAccessScript []byte

func init() {
	directive.Register(directive.Artifact{Path: DBAccessScriptPath, Content: dbAccessContent, Owns: ownsDBAccess})
}

// ownsDBAccess: the script is Dendrite's while it keeps the generated marker line.
func ownsDBAccess(b []byte) bool {
	return bytes.Contains(firstLines(b, 3), []byte("managed by dendrite"))
}

func firstLines(b []byte, n int) []byte {
	for i, c := range b {
		if c == '\n' {
			if n--; n == 0 {
				return b[:i]
			}
		}
	}
	return b
}

// dbAccessContent writes the script only for a project with a Rust component.
func dbAccessContent(dir string, _ directive.Set) []byte {
	if !present(dir, Rust) {
		return nil
	}
	return dbAccessScript
}

// writeDBAccessJob adds the Diesel-only job of a Rust project.
func writeDBAccessJob(b *strings.Builder) {
	b.WriteString("  db-access:\n    name: " + scalar(dbAccessJobName) + "\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - name: Checkout\n        uses: actions/checkout@v4\n" +
		"      - name: Rust reaches databases only through Diesel\n        run: bash " + DBAccessScriptPath + "\n")
}
