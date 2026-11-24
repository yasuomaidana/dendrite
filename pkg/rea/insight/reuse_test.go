package insight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/synapse"
)

func TestQueriesAreWhatTheAnalysisFoundNotWhatEveryImageHas(t *testing.T) {
	img := &analysis.Image{
		Exports:   []string{"ComputeChecksum", "ComputeChecksum", "", "e3", "e4", "e5", "e6", "e7", "e8"},
		Strings:   []string{"  https://api.internal.endpoint  ", "short", strings.Repeat("x", 81), "config.ini"},
		Libraries: []coreclient.ImageLibrary{{Name: "KERNEL32.dll"}, {Name: "libc.so.6"}, {Name: "/usr/lib/libSystem.B.dylib"}, {Name: "lodash"}, {Name: "libcurl.so.4"}},
	}
	got := Queries(img)
	want := []string{"ComputeChecksum", "e3", "e4", "e5", "e6", "e7", "https://api.internal.endpoint", "config.ini", "lodash", "libcurl.so.4"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("queries\n got %v\nwant %v", got, want)
	}
	many := &analysis.Image{}
	for i := 0; i < 30; i++ {
		many.Strings = append(many.Strings, fmt.Sprintf("string number %d", i))
	}
	if n := len(Queries(many)); n != maxQueries {
		t.Errorf("at most %d queries, got %d", maxQueries, n)
	}
}

func TestReuseFindsWhatTheProjectAlreadyHas(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	project := t.TempDir()
	writeFile(t, project, "checksum.go", "package main\n\nfunc ComputeChecksum(b []byte) uint32 { return 0 }\n")
	writeFile(t, project, "other.go", "package main\n\nfunc unrelated() {}\n")
	hits, err := Reuse(ctx(), svc, id, ReuseOptions{Project: project})
	if err != nil || len(hits) == 0 {
		t.Fatalf("%+v, %v", hits, err)
	}
	if h := hits[0]; h.Source != "project" || !strings.HasSuffix(h.File, "checksum.go") || !strings.Contains(h.Text, "ComputeChecksum") || h.Queries[0] != "ComputeChecksum" {
		t.Errorf("hit %+v", h)
	}
}

func TestReuseSearchesTheCurrentDirectoryByDefault(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	project := t.TempDir()
	writeFile(t, project, "checksum.go", "package main\n\nfunc ComputeChecksum() {}\n")
	t.Chdir(project)
	if hits, err := Reuse(ctx(), svc, id, ReuseOptions{}); err != nil || len(hits) == 0 {
		t.Errorf("%+v, %v", hits, err)
	}
}

func TestReuseWithNothingToAskReturnsNothing(t *testing.T) {
	svc, id := finishedAnalysis(t, "blob.bin", "bytes")
	hits, err := Reuse(ctx(), svc, id, ReuseOptions{Project: t.TempDir()})
	if err != nil || len(hits) != 0 {
		t.Errorf("%+v, %v", hits, err)
	}
}

func TestReuseReportsWhatItCannotDo(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	if _, err := Reuse(ctx(), svc, "no-such", ReuseOptions{}); err == nil {
		t.Error("an unknown analysis is an error")
	}
	t.Setenv("STANDIN_FAIL", "search")
	if _, err := Reuse(ctx(), svc, id, ReuseOptions{Project: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "search for") {
		t.Errorf("a failing search: %v", err)
	}
	t.Setenv("STANDIN_FAIL", "")
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := Reuse(ctx(), svc, id, ReuseOptions{Project: t.TempDir()}); err == nil {
		t.Error("a synapse store that cannot be opened is an error")
	}
}

// joinSynapse registers dirs as members (alias -> directory) of a synapse and returns the project id of each.
func joinSynapse(t *testing.T, id string, members map[string]string) {
	t.Helper()
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	syn := &synapse.Synapse{ID: id, Name: id, State: synapse.StateActive}
	for alias, dir := range members {
		e, err := ix.Register(dir)
		if err != nil {
			t.Fatal(err)
		}
		syn.Members = append(syn.Members, synapse.Member{ProjectID: e.ID, Alias: alias, Role: synapse.RolePeer, PathHint: dir})
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	store, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(syn); err != nil {
		t.Fatal(err)
	}
}

func TestReuseSearchesTheSynapseTheProjectBelongsTo(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	project, peer := t.TempDir(), t.TempDir()
	writeFile(t, project, "main.go", "package main\n")
	writeFile(t, peer, "checks.go", "package peer\n\nfunc ComputeChecksum() {}\n")
	joinSynapse(t, "eco", map[string]string{"control": project, "libs": peer})

	for name, o := range map[string]ReuseOptions{
		"found through the project's own synapse": {Project: project},
		"named explicitly":                        {Project: t.TempDir(), Synapse: "eco"},
	} {
		hits, err := Reuse(ctx(), svc, id, o)
		if err != nil || len(hits) == 0 || hits[0].Source != "libs" || !strings.HasSuffix(hits[0].File, "checks.go") {
			t.Errorf("%s: %+v, %v", name, hits, err)
		}
	}
}

// register gives dir an identity in the project index and returns nothing else: the project is in no synapse.
func register(t *testing.T, dir string) {
	t.Helper()
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Register(dir); err != nil {
		t.Fatal(err)
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestSynapseForFindsNoSynapseForAProjectInNone(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if syn, err := synapseFor(t.TempDir(), ""); syn != nil || err != nil {
		t.Errorf("a project with no identity has no synapse: %v, %v", syn, err)
	}
	registered := t.TempDir()
	register(t, registered)
	if syn, err := synapseFor(registered, ""); syn != nil || err != nil {
		t.Errorf("a project that is in no synapse: %v, %v", syn, err)
	}
}

func TestSynapseForReportsAmbiguityAndUnknownNames(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	project := t.TempDir()
	joinSynapse(t, "one", map[string]string{"a": project, "b": t.TempDir()})
	joinSynapse(t, "two", map[string]string{"a": project, "c": t.TempDir()})
	if _, err := synapseFor(project, ""); err == nil || !strings.Contains(err.Error(), "several synapses") {
		t.Errorf("a project in two synapses: %v", err)
	}
	if _, err := synapseFor(project, "no-such"); err == nil {
		t.Error("an unknown synapse is an error")
	}
}

func TestSynapseForNeedsAHomeDirectory(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := synapseFor(t.TempDir(), ""); err == nil {
		t.Error("no home directory is an error")
	}
}

func TestRankedCapsTheHitsAndTheTokens(t *testing.T) {
	merged := map[string]*ReuseHit{}
	long := strings.Repeat("y", maxReuseText*3)
	for i := 0; i < 40; i++ {
		accumulate(merged, "q", []ReuseHit{{Source: "project", File: fmt.Sprintf("f%02d.go", i), Text: long}})
	}
	all := ranked(merged, 100)
	if len(all) == 0 || len(all) >= 40 || !strings.HasSuffix(all[0].Text, "...") {
		t.Errorf("%d hits kept", len(all))
	}
	if few := ranked(merged, 3); len(few) != 3 {
		t.Errorf("k is honoured: %d", len(few))
	}
	if def := ranked(merged, 0); len(def) != defaultReuse {
		t.Errorf("the default is %d hits, got %d", defaultReuse, len(def))
	}
}

func TestAccumulateScoresHigherRanksAndRepeatedFindsMore(t *testing.T) {
	merged := map[string]*ReuseHit{}
	accumulate(merged, "q1", []ReuseHit{{Source: "project", File: "a.go"}, {Source: "project", File: "b.go"}})
	accumulate(merged, "q2", []ReuseHit{{Source: "project", File: "b.go"}})
	a, b := merged["project|a.go"], merged["project|b.go"]
	if a.Score != 1 || b.Score != 1.5 || len(b.Queries) != 2 || b.Queries[1] != "q2" {
		t.Errorf("a %+v, b %+v", a, b)
	}
	if got := ranked(merged, 5); got[0].File != "b.go" {
		t.Errorf("the file found twice ranks first: %+v", got)
	}
}

func TestQueriesAddTheNamesOfEngineFunctionsButNotGeneratedOnes(t *testing.T) {
	img := &analysis.Image{Exports: []string{"Export"}}
	got := Queries(img, "main", "sub_401000", "FUN_00401060", "fcn.00401080", "sym.imp.puts", "loc_1", "nullsub_1", "thunk_FUN_1", "0x401000", "entry0", "_start", "parse_header", "main", "check_flag", "decode", "extra")
	want := []string{"Export", "main", "parse_header", "check_flag", "decode"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("queries\n got %v\nwant %v", got, want)
	}
	if only := Queries(img); len(only) != 1 {
		t.Errorf("without functions the queries are as before: %v", only)
	}
}

func TestReuseAsksForTheFunctionsAnEngineFound(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	svc, id := analysisWith(t, "sample", "\x7fELF", engine.NewService())
	project := t.TempDir()
	writeFile(t, project, "helpers.go", "package main\n\nfunc helper() {}\n")
	hits, err := Reuse(ctx(), svc, id, ReuseOptions{Project: project})
	if err != nil || len(hits) == 0 || !strings.HasSuffix(hits[0].File, "helpers.go") || !containsString(hits[0].Queries, "helper") {
		t.Errorf("a function name finds code: %+v, %v", hits, err)
	}
	if err := os.WriteFile(filepath.Join(svc.Store.Dir(id), "engine.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Reuse(ctx(), svc, id, ReuseOptions{Project: project}); err == nil {
		t.Error("an engine result that cannot be read is an error")
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
