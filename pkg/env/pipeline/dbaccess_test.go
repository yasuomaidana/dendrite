package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/fsutil/fsutiltest"
)

// runDBAccess runs the generated script over a project made of files.
func runDBAccess(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		fsutiltest.WriteFile(t, root, rel, content)
	}
	script := filepath.Join(root, DBAccessScriptPath)
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, dbAccessScript, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestTheDBAccessScriptHoldsRustToDiesel(t *testing.T) {
	const diesel = "[dependencies]\ndiesel = { version = \"2\" }\n"
	cases := map[string]struct {
		files map[string]string
		fail  string // empty: passes
	}{
		"no manifest":              {map[string]string{"a.txt": "x"}, ""},
		"diesel stack":             {map[string]string{"Cargo.toml": diesel + "diesel-async = \"0.9\"\ntokio-postgres = \"0.7\"\nlibsql = \"0.10\"\n"}, ""},
		"features are not deps":    {map[string]string{"Cargo.toml": "[features]\npostgres = [\"dep:diesel\"]\n" + diesel}, ""},
		"sqlx":                     {map[string]string{"Cargo.toml": "[dependencies]\nsqlx = \"0.8\"\n"}, "declares 'sqlx'"},
		"rusqlite in dev deps":     {map[string]string{"Cargo.toml": "[dev-dependencies]\nrusqlite = { version = \"0.31\" }\n"}, "declares 'rusqlite'"},
		"sea-orm as a table":       {map[string]string{"Cargo.toml": "[dependencies.sea-orm]\nversion = \"1\"\n"}, "declares 'sea-orm'"},
		"mysql_async":              {map[string]string{"Cargo.toml": "[workspace.dependencies]\nmysql_async = \"0.34\"\n"}, "declares 'mysql_async'"},
		"bare postgres, no diesel": {map[string]string{"Cargo.toml": "[dependencies]\ntokio-postgres = \"0.7\"\n"}, "no diesel crate"},
		"allowed by name":          {map[string]string{"Cargo.toml": "[dependencies]\nrusqlite = \"0.31\"\n", ".github/dendrite/db-access.allow": "rusqlite # legacy import tool\n"}, ""},
		"raw sql outside raw":      {map[string]string{"Cargo.toml": diesel, "src/store.rs": "fn f() { diesel::sql_query(\"select 1\"); }\n"}, "raw SQL outside a 'raw' module"},
		"raw sql in raw.rs":        {map[string]string{"Cargo.toml": diesel, "src/store/raw.rs": "fn f() { diesel::sql_query(\"select 1\"); }\n"}, ""},
		"raw sql in raw dir":       {map[string]string{"Cargo.toml": diesel, "src/raw/lock.rs": "fn f() { sql_query(\"select 1\"); }\n"}, ""},
		"raw sql in tests":         {map[string]string{"Cargo.toml": diesel, "tests/db.rs": "fn f() { diesel::sql_query(\"x\"); }\n", "src/store/tests.rs": "fn f() { diesel::sql_query(\"x\"); }\n"}, ""},
		"raw sql path allowed":     {map[string]string{"Cargo.toml": diesel, "gen/codegen.rs": "fn f() { diesel::sql_query(\"x\"); }\n", ".github/dendrite/db-access.allow": "path:gen/ # code generator\n"}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runDBAccess(t, tc.files)
			switch {
			case tc.fail == "" && err != nil:
				t.Errorf("must pass, failed: %v\n%s", err, out)
			case tc.fail != "" && (err == nil || !strings.Contains(out, tc.fail)):
				t.Errorf("must fail with %q, got err=%v\n%s", tc.fail, err, out)
			}
		})
	}
}
