// Package prtest puts a local `gh` program on PATH for tests, so the code that opens pull requests runs for real
// without reaching GitHub. The program keeps its pull requests as files in a directory.
package prtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// script is a small `gh`: `pr view <head> --json ...` prints the JSON stored for head (or fails the way gh does
// when there is none), `pr create ... --head <head>` stores an open pull request and prints its URL, and every
// call is appended to calls.log. A file named "broken" in the directory makes every call fail.
const script = `#!/bin/sh
dir="$GH_PRTEST_DIR"
echo "$*" >> "$dir/calls.log"
if [ -f "$dir/broken" ]; then echo "gh: not logged in" >&2; exit 4; fi
if [ "$1 $2" = "pr view" ]; then
  file="$dir/$(echo "$3" | tr '/' '_').json"
  if [ -f "$file" ]; then cat "$file"; exit 0; fi
  echo "no pull requests found for branch \"$3\"" >&2; exit 1
fi
if [ "$1 $2" = "pr create" ]; then
  while [ $# -gt 0 ]; do
    case "$1" in --base) base="$2";; --head) head="$2";; esac
    shift
  done
  n=$(($(ls "$dir" | grep -c '\.json$') + 1))
  url="https://github.example/o/r/pull/$n"
  printf '{"url":"%s","number":%s,"state":"OPEN","baseRefName":"%s","mergeCommit":null}' "$url" "$n" "$base" > "$dir/$(echo "$head" | tr '/' '_').json"
  echo "Creating pull request for $head into $base"
  echo "$url"
  exit 0
fi
echo "unsupported: $*" >&2; exit 2
`

// GitHub is the directory the local gh keeps its pull requests in.
type GitHub struct {
	t   testing.TB
	dir string
}

// New installs the local gh at the front of PATH for the test and returns its store.
func New(t testing.TB) *GitHub {
	t.Helper()
	bin, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_PRTEST_DIR", dir)
	return &GitHub{t: t, dir: dir}
}

func (g *GitHub) file(head string) string {
	return filepath.Join(g.dir, strings.ReplaceAll(head, "/", "_")+".json")
}

// Set stores the pull request of head as raw JSON, as `gh pr view --json` would print it.
func (g *GitHub) Set(head, json string) {
	g.t.Helper()
	if err := os.WriteFile(g.file(head), []byte(json), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

// Merge marks the pull request of head as merged with the given merge commit.
func (g *GitHub) Merge(head, base, sha string) {
	g.Set(head, `{"url":"https://github.example/o/r/pull/1","number":1,"state":"MERGED","baseRefName":"`+base+`","mergeCommit":{"oid":"`+sha+`"}}`)
}

// Break makes every later gh call fail, as an unauthenticated gh does.
func (g *GitHub) Break() {
	g.t.Helper()
	if err := os.WriteFile(filepath.Join(g.dir, "broken"), nil, 0o644); err != nil {
		g.t.Fatal(err)
	}
}

// Calls is every gh command line run so far, one per line.
func (g *GitHub) Calls() string {
	data, _ := os.ReadFile(filepath.Join(g.dir, "calls.log"))
	return string(data)
}
