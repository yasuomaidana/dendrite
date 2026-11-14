package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil/fsutiltest"
)

// runReleaseRefs runs the generated script inside a git project made of files.
func runReleaseRefs(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		fsutiltest.WriteFile(t, root, rel, content)
	}
	script := filepath.Join(root, ReleaseRefsScriptPath)
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, releaseRefsScript, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestTheReleaseRefsScriptAcceptsOnlyReleasesAndMain(t *testing.T) {
	const core = "https://github.com/yasuomaidana/dendrite-core-rs"
	ckout := func(ref string) string {
		return "steps:\n  - uses: actions/checkout@v4\n    with:\n      repository: yasuomaidana/dendrite-rag-rs\n      ref: " + ref + "\n      path: x\n"
	}
	cases := map[string]struct {
		files map[string]string
		fail  string // empty: passes
	}{
		"nothing to check":    {map[string]string{"a.txt": "x"}, ""},
		"git dep with tag":    {map[string]string{"Cargo.toml": "[dependencies]\ncore = { git = \"" + core + "\", tag = \"v0.1.0\" }\n"}, ""},
		"git dep on main":     {map[string]string{"Cargo.toml": "[dependencies]\ncore = { git = \"" + core + "\", branch = \"main\" }\n"}, ""},
		"git dep unpinned":    {map[string]string{"Cargo.toml": "[dependencies]\ncore = { git = \"" + core + "\", version = \"0.1.0\" }\n"}, "must name tag"},
		"git dep on develop":  {map[string]string{"Cargo.toml": "[dependencies]\ncore = { git = \"" + core + "\", branch = \"develop\" }\n"}, "must name tag"},
		"git dep on a rev":    {map[string]string{"Cargo.toml": "[dependencies]\ncore = { git = \"" + core + "\", rev = \"abc1234\" }\n"}, "must name tag"},
		"foreign git dep":     {map[string]string{"Cargo.toml": "[dependencies]\nx = { git = \"https://github.com/other/thing\" }\n"}, ""},
		"sibling path dep":    {map[string]string{"Cargo.toml": "[dependencies]\ncore = { path = \"../dendrite-core-rs\" }\n"}, "path dependency"},
		"own crate path dep":  {map[string]string{"Cargo.toml": "[dependencies]\nembedag-core = { path = \"crates/embedag-core\", version = \"0.1.1\" }\n"}, ""},
		"go release":          {map[string]string{"go.mod": "require github.com/yasuomaidana/dendrite-mcp-hub v0.1.0\n"}, ""},
		"go pseudo-version":   {map[string]string{"go.mod": "require github.com/yasuomaidana/dendrite-mcp-hub v0.0.0-20261028060513-27fd9116f441\n"}, "pseudo-version"},
		"go local replace":    {map[string]string{"go.mod": "replace github.com/yasuomaidana/dendrite-mcp-hub => ../dendrite-mcp-hub\n"}, "local path"},
		"checkout main":       {map[string]string{".github/workflows/ci.yml": ckout("main")}, ""},
		"checkout tag":        {map[string]string{".github/workflows/ci.yml": ckout("${{ vars.RAG || 'v0.1.0' }}")}, ""},
		"checkout default br": {map[string]string{".github/workflows/ci.yml": "steps:\n  - uses: actions/checkout@v4\n    with:\n      repository: yasuomaidana/dendrite-rag-rs\n"}, ""},
		"checkout develop":    {map[string]string{".github/workflows/ci.yml": ckout("${{ vars.RAG || 'develop' }}")}, "sibling checkout"},
		"checkout own repo":   {map[string]string{".github/workflows/ci.yml": "steps:\n  - uses: actions/checkout@v4\n    with:\n      ref: develop\n"}, ""},
		"link to main":        {map[string]string{"a.md": "[x](https://github.com/yasuomaidana/dendrite-rag-rs/blob/main/README.md)\n"}, ""},
		"link to develop":     {map[string]string{"a.md": "[x](https://github.com/yasuomaidana/embedag/blob/develop/README.md)\n"}, "working branch"},
		"changelog is exempt": {map[string]string{"CHANGELOG.md": "see https://github.com/yasuomaidana/embedag/blob/develop/x\n"}, ""},
		"allowed by path":     {map[string]string{"a.md": "https://github.com/yasuomaidana/embedag/tree/develop/x\n", ".github/dendrite/release-refs.allow": "path:a.md # historical report\n"}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runReleaseRefs(t, tc.files)
			switch {
			case tc.fail == "" && err != nil:
				t.Errorf("must pass, failed: %v\n%s", err, out)
			case tc.fail != "" && (err == nil || !strings.Contains(out, tc.fail)):
				t.Errorf("must fail with %q, got err=%v\n%s", tc.fail, err, out)
			}
		})
	}
}

func TestTheCommittedReleaseRefsScriptIsTheGeneratedOne(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("..", "..", "..", ReleaseRefsScriptPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(releaseRefsScript) {
		t.Errorf("%s differs from the embedded script: copy pkg/env/pipeline/releaserefs.sh over it", ReleaseRefsScriptPath)
	}
}

func TestEveryProjectGetsTheReleaseRefsScript(t *testing.T) {
	if got := releaseRefsContent(t.TempDir(), directive.Defaults()); string(got) != string(releaseRefsScript) {
		t.Error("a project of any stack is written the release-refs script")
	}
}
