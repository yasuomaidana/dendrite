package directive

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/llmchat/llmchattest"
)

func TestTheSharedExampleIsTheDefaults(t *testing.T) {
	data, err := os.ReadFile("testdata/directives.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("the example both sides parse must be valid: %v", err)
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("example %+v differs from the defaults %+v: change both (and the Rust side) together", got, Defaults())
	}
}

func TestEncodeRoundTripsAndFormatsNumbers(t *testing.T) {
	s := Defaults()
	s.Coverage.MinPercent = 99.5
	s.Duplication.MaxPercent = 2
	s.Loc.Exclude = []string{`a "quoted" glob`, "b/**"}
	text := string(Encode(s))
	for _, want := range []string{Header + "\n", "min_percent = 99.5\n", "max_percent = 2.0\n", `exclude = ["a \"quoted\" glob", "b/**"]`, "\n[done]\nauto_done = true\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("encoded text lacks %q:\n%s", want, text)
		}
	}
	back, err := Decode([]byte(text))
	if err != nil || !reflect.DeepEqual(back, s) {
		t.Errorf("round trip: %+v, %v", back, err)
	}
	if string(Encode(back)) != text {
		t.Error("the same directives always encode to the same text")
	}
}

func TestDecodeKeepsDefaultsAndRefusesWhatIsWrong(t *testing.T) {
	partial, err := Decode([]byte("[loc]\nmax_production_lines = 300\nwarn_at = 250\n"))
	if err != nil || partial.Loc.MaxProductionLines != 300 || partial.Coverage.MinPercent != 100 {
		t.Fatalf("a partial file keeps the other defaults: %+v, %v", partial, err)
	}
	for name, tc := range map[string]struct{ toml, want string }{
		"a typo":         {"[loc]\nmax_production_line = 300\n", "unknown key"},
		"bad toml":       {"[loc\n", "not a valid directives file"},
		"out of range":   {"[coverage]\nmin_percent = 250.0\n", "coverage.min_percent"},
		"wrong schema":   {"schema = 2\n", "schema must be 1"},
		"warn above max": {"[loc]\nmax_production_lines = 100\n", "loc.warn_at (450)"},
		"wrong type":     {"[loc]\nmax_production_lines = \"many\"\n", "not a valid directives file"},
	} {
		if _, err := Decode([]byte(tc.toml)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

func TestValidateNamesTheBrokenRule(t *testing.T) {
	for name, mutate := range map[string]func(*Set){
		"loc.max_production_lines": func(s *Set) { s.Loc.MaxProductionLines = 10; s.Loc.WarnAt = 5 },
		"loc.warn_at":              func(s *Set) { s.Loc.WarnAt = -1 },
		"coverage.min_percent":     func(s *Set) { s.Coverage.MinPercent = -1 },
		"coverage.scope":           func(s *Set) { s.Coverage.Scope = "path" },
		"duplication.max_percent":  func(s *Set) { s.Duplication.MaxPercent = 101 },
		"duplication.min_tokens":   func(s *Set) { s.Duplication.MinTokens = 1 },
		"duplication.min_lines":    func(s *Set) { s.Duplication.MinLines = 1 },
		"lint.strictness":          func(s *Set) { s.Lint.Strictness = "wild" },
		"security.fail_on":         func(s *Set) { s.Security.FailOn = "never" },
		"languages.preference":     func(s *Set) { s.Languages.Preference = nil },
	} {
		s := Defaults()
		mutate(&s)
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestApplyChangesOneSettingAndRefusesBadOnesUnchanged(t *testing.T) {
	s := Defaults()
	for _, c := range []struct {
		key   string
		value any
	}{
		{"coverage.min_percent", 95}, {"coverage.min_percent", 95.5}, {"loc.max_production_lines", 600.0},
		{"loc.exclude", []string{"x/**"}}, {"loc.exclude", []any{"y/**", "z/**"}}, {"lint.strictness", " Standard "},
		{"done.auto_done", false}, {"duplication.include_tests", false},
	} {
		if err := s.Apply(c.key, c.value); err != nil {
			t.Errorf("%s = %v: %v", c.key, c.value, err)
		}
	}
	if s.Coverage.MinPercent != 95.5 || s.Loc.MaxProductionLines != 600 || s.Lint.Strictness != "standard" || s.Done.AutoDone || !reflect.DeepEqual(s.Loc.Exclude, []string{"y/**", "z/**"}) {
		t.Errorf("set = %+v", s)
	}
	before := s
	for name, c := range map[string]struct {
		key   string
		value any
		want  string
	}{
		"unknown key":       {"coverage.target", 1, "unknown directive"},
		"text for a number": {"loc.warn_at", "many", "whole number"},
		"fraction for int":  {"loc.warn_at", 1.5, "whole number"},
		"text for float":    {"coverage.min_percent", "high", "want a number"},
		"number for bool":   {"done.auto_done", 1, "true or false"},
		"empty word":        {"lint.strictness", "  ", "want a word"},
		"number for list":   {"loc.exclude", 3, "want a list"},
		"list of numbers":   {"loc.exclude", []any{1}, "list of text"},
		"out of range":      {"coverage.min_percent", 120, "between 0 and 100"},
	} {
		if err := s.Apply(c.key, c.value); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	if !reflect.DeepEqual(s, before) {
		t.Error("a refused change leaves the set unchanged")
	}
}

func TestApplyAllValidatesOnceAtTheEnd(t *testing.T) {
	s := Defaults()
	if err := s.ApplyAll([]Change{{Key: "loc.max_production_lines", Value: 300}, {Key: "loc.warn_at", Value: 270}}); err != nil {
		t.Fatalf("a lower limit with a lower warning level is valid together: %v", err)
	}
	before := s
	for name, changes := range map[string][]Change{
		"unknown key":    {{Key: "nope", Value: 1}},
		"bad type":       {{Key: "loc.warn_at", Value: "x"}},
		"only the limit": {{Key: "loc.max_production_lines", Value: 100}},
	} {
		if err := s.ApplyAll(changes); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if !reflect.DeepEqual(s, before) {
		t.Error("a refused batch leaves the set unchanged")
	}
}

func TestParseValueReadsTypedText(t *testing.T) {
	for _, c := range []struct {
		key, text string
		want      any
	}{
		{"loc.warn_at", " 400 ", 400}, {"coverage.min_percent", "95%", 95.0}, {"done.auto_done", "Yes", true},
		{"security.secrets", "off", false}, {"lint.strictness", "strict", "strict"}, {"loc.exclude", "a/**, b ,,c", []string{"a/**", "b", "c"}},
	} {
		got, err := ParseValue(c.key, c.text)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s=%q: %#v, %v", c.key, c.text, got, err)
		}
	}
	for _, c := range []struct{ key, text, want string }{
		{"nope", "1", "unknown directive"}, {"loc.warn_at", "x", "loc.warn_at"}, {"coverage.min_percent", "high", "coverage.min_percent"}, {"done.auto_done", "maybe", "true or false"},
	} {
		if _, err := ParseValue(c.key, c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s=%q: %v, want %q", c.key, c.text, err, c.want)
		}
	}
}

func TestKeysAndDescribeCoverEverySetting(t *testing.T) {
	ks := Keys()
	if len(ks) != 17 || ks[0] != "coverage.exclude" {
		t.Errorf("keys = %v", ks)
	}
	text := Describe()
	for _, k := range ks {
		if !strings.Contains(text, k+" (") {
			t.Errorf("Describe lacks %s", k)
		}
	}
	if (Change{Key: "a.b", Value: 3}).String() != "a.b = 3" {
		t.Error("Change.String")
	}
}

func TestLoadSaveAndEnsureDefaults(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Fatal("no file yet")
	}
	if s, err := Load(dir); err != nil || !reflect.DeepEqual(s, Defaults()) {
		t.Fatalf("a project with no file has the defaults: %+v, %v", s, err)
	}
	if created, err := EnsureDefaults(dir); err != nil || !created || !Exists(dir) {
		t.Fatalf("created %v, %v", created, err)
	}
	if created, err := EnsureDefaults(dir); err != nil || created {
		t.Errorf("a second call changes nothing: %v, %v", created, err)
	}
	s := Defaults()
	s.Coverage.MinPercent = 90
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(dir); err != nil || got.Coverage.MinPercent != 90 {
		t.Errorf("%+v, %v", got, err)
	}
	bad := Defaults()
	bad.Loc.MaxProductionLines = 1
	if err := Save(dir, bad); err == nil {
		t.Error("an invalid set is not saved")
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(Path)), []byte("[loc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), Path) {
		t.Errorf("a broken file is an error naming the file: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(Path))); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(Path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("a directory where the file should be is an error")
	}
}

func TestParseKnownSentences(t *testing.T) {
	for _, c := range []struct {
		text string
		want []Change
	}{
		{"enforce max 500 lines of code ignoring tests", []Change{{Key: "loc.max_production_lines", Value: 500}, {Key: "loc.warn_at", Value: 450}}},
		{"no file over 300 lines", []Change{{Key: "loc.max_production_lines", Value: 300}, {Key: "loc.warn_at", Value: 270}}},
		{"limit files to 400 lines, warn at 350", []Change{{Key: "loc.max_production_lines", Value: 400}, {Key: "loc.warn_at", Value: 350}}},
		{"enforce 95% code coverage", []Change{{Key: "coverage.min_percent", Value: 95.0}}},
		{"we want 99.5 percent test coverage", []Change{{Key: "coverage.min_percent", Value: 99.5}}},
		{"coverage of 80", []Change{{Key: "coverage.min_percent", Value: 80.0}}},
		{"enforce zero duplicated code", []Change{{Key: "duplication.max_percent", Value: 0.0}}},
		{"no duplication please", []Change{{Key: "duplication.max_percent", Value: 0.0}}},
		{"allow up to 3% duplicated code", []Change{{Key: "duplication.max_percent", Value: 3.0}}},
		{"allow agents to set done", []Change{{Key: "done.auto_done", Value: true}}},
		{"require human review", []Change{{Key: "done.auto_done", Value: false}}},
		{"agents never set done", []Change{{Key: "done.auto_done", Value: false}}},
		{"max 200 lines and 90% coverage, zero duplicated code", []Change{{Key: "loc.max_production_lines", Value: 200}, {Key: "loc.warn_at", Value: 180}, {Key: "coverage.min_percent", Value: 90.0}, {Key: "duplication.max_percent", Value: 0.0}}},
	} {
		got := Parse(c.text)
		var pairs []Change
		for _, g := range got {
			pairs = append(pairs, Change{Key: g.Key, Value: g.Value})
		}
		if !reflect.DeepEqual(pairs, c.want) {
			t.Errorf("%q\n got %v\nwant %v", c.text, pairs, c.want)
		}
	}
	for _, text := range []string{"make the code nicer", "", "keep the lines short"} {
		if got := Parse(text); len(got) != 0 {
			t.Errorf("%q is not a known rule: %v", text, got)
		}
	}
}

func TestInterpretChecksWhatTheModelProposes(t *testing.T) {
	ctx := context.Background()
	gw := llmchattest.New(t, "```json\n[{\"key\":\"coverage.min_percent\",\"value\":95,\"rationale\":\"asked for 95\"},{\"key\":\"lint.strictness\",\"value\":\"standard\"}]\n```")
	got, err := Interpret(ctx, gw.Client, "be a bit more relaxed about lint but keep ninety five percent coverage")
	if err != nil || len(got) != 2 || got[0].Value != 95.0 || got[0].Reason != "asked for 95" || got[1].Value != "standard" {
		t.Fatalf("%+v, %v", got, err)
	}
	if !strings.Contains(gw.Last().System, "coverage.min_percent (float)") {
		t.Error("the model is told every setting and its kind")
	}
	for name, tc := range map[string]struct {
		chat llmchat.Completer
		want string
	}{
		"unreachable":  {llmchat.Client{URL: "http://127.0.0.1:1"}, "could not read the rule"},
		"gateway down": {llmchattest.Failing(t, 503).Client, "HTTP 503"},
		"no array":     {llmchattest.New(t, "I am not sure").Client, "not usable"},
		"broken json":  {llmchattest.New(t, "[{oops]").Client, "not usable"},
		"wrong shape":  {llmchattest.New(t, "[1,2]").Client, "not valid JSON"},
		"empty answer": {llmchattest.New(t, "[]").Client, "no directive matches"},
		"unknown key":  {llmchattest.New(t, `[{"key":"make.it.nice","value":1}]`).Client, "invalid change"},
		"out of range": {llmchattest.New(t, `[{"key":"coverage.min_percent","value":950}]`).Client, "between 0 and 100"},
		"wrong kind":   {llmchattest.New(t, `[{"key":"loc.warn_at","value":"lots"}]`).Client, "whole number"},
	} {
		if _, err := Interpret(ctx, tc.chat, "x"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

func TestInterpretReturnsTheStoredValueOfEveryKey(t *testing.T) {
	sample := map[string]string{
		"loc.max_production_lines": "600", "loc.warn_at": "100", "loc.exclude": `["a"]`,
		"coverage.min_percent": "50", "coverage.scope": `"branch"`, "coverage.exclude": `["b"]`,
		"duplication.max_percent": "5", "duplication.min_tokens": "20", "duplication.min_lines": "3", "duplication.include_tests": "false", "duplication.strategies": `["macro"]`,
		"lint.strictness": `"standard"`, "security.fail_on": `"medium"`, "security.secrets": "false",
		"done.auto_done": "false", "done.human_review_labels": `["x"]`, "languages.preference": `["go"]`,
	}
	for _, key := range Keys() {
		answer := `[{"key":"` + key + `","value":` + sample[key] + `}]`
		got, err := Interpret(context.Background(), llmchattest.New(t, answer).Client, "x")
		if err != nil || len(got) != 1 || got[0].Key != key || got[0].Value == nil {
			t.Errorf("%s: %+v, %v", key, got, err)
		}
	}
}

func TestDiffShowsOnlyWhatChangedWithContext(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\nh\n"
	if Diff(old, old) != "" {
		t.Error("nothing changed, nothing shown")
	}
	got := Diff(old, "a\nb\nc\nD\ne\nf\ng\nh\ni\n")
	want := "  c\n- d\n+ D\n  e\n  ...\n  h\n+ i\n"
	if got != want {
		t.Errorf("diff:\n%s\nwant:\n%s", got, want)
	}
	if got := Diff("", "x\n"); got != "+ x\n" {
		t.Errorf("a new file: %q", got)
	}
	if got := Diff("x\ny\n", "y\n"); got != "- x\n  y\n" {
		t.Errorf("a removed line: %q", got)
	}
}

func TestRenderWritesOnlyWhatItOwns(t *testing.T) {
	dir := t.TempDir()
	s := Defaults()
	s.Coverage.MinPercent = 95
	s.Duplication.IncludeTests = false
	res, err := Render(dir, s)
	if err != nil || !reflect.DeepEqual(res.Written, []string{"codecov.yml", ".jscpd.json"}) || len(res.Kept) != 0 {
		t.Fatalf("%+v, %v", res, err)
	}
	codecov, _ := os.ReadFile(filepath.Join(dir, "codecov.yml"))
	jscpd, _ := os.ReadFile(filepath.Join(dir, ".jscpd.json"))
	if !strings.HasPrefix(string(codecov), Header) || strings.Count(string(codecov), "target: 95%") != 2 {
		t.Errorf("codecov.yml:\n%s", codecov)
	}
	if !strings.Contains(string(jscpd), `"threshold": 0`) || !strings.Contains(string(jscpd), "**/*_test.*") || !strings.Contains(string(jscpd), jscpdMarker) {
		t.Errorf(".jscpd.json:\n%s", jscpd)
	}

	if res, err := Render(dir, s); err != nil || len(res.Written) != 0 {
		t.Errorf("an unchanged render writes nothing: %+v, %v", res, err)
	}
	s.Coverage.MinPercent = 99.5
	if res, _ := Render(dir, s); !reflect.DeepEqual(res.Written, []string{"codecov.yml"}) {
		t.Errorf("only the changed file is rewritten: %+v", res)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "codecov.yml")); !strings.Contains(string(body), "target: 99.5%") {
		t.Errorf("codecov.yml:\n%s", body)
	}
}

func TestRenderNeverTouchesAHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"codecov.yml": "coverage: {}\n", ".jscpd.json": `{"threshold": 7}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Render(dir, Defaults())
	if err != nil || len(res.Written) != 0 || !reflect.DeepEqual(res.Kept, []string{"codecov.yml", ".jscpd.json"}) {
		t.Fatalf("%+v, %v", res, err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, ".jscpd.json")); string(body) != `{"threshold": 7}` {
		t.Errorf("hand-written file changed: %s", body)
	}
}

func TestRenderReportsWriteFailuresAndTakesNewArtifacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "codecov.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(dir, Defaults()); err == nil {
		t.Error("a directory where a file belongs is an error")
	}
	n := len(artifacts)
	defer func() { artifacts = artifacts[:n] }()
	Register(Artifact{Path: "extra/x.txt", Content: func(Set) []byte { return []byte("x") }, Owns: func([]byte) bool { return true }})
	clean := t.TempDir()
	if res, err := Render(clean, Defaults()); err != nil || !contains(res.Written, "extra/x.txt") {
		t.Errorf("a registered artifact is rendered: %+v, %v", res, err)
	}
	Register(Artifact{Path: "readonly/y.txt", Content: func(Set) []byte { return []byte("y") }, Owns: func([]byte) bool { return true }})
	if err := os.Mkdir(filepath.Join(clean, "readonly"), 0o500); err != nil {
		t.Fatal(err)
	}
	// Whoever may write into a read-only directory (root) gets a write; everyone else gets the error.
	canWrite := os.WriteFile(filepath.Join(clean, "readonly", "probe"), nil, 0o644) == nil
	_, err := Render(clean, Defaults())
	if canWrite && err != nil {
		t.Errorf("this user can write into the directory, so Render must succeed: %v", err)
	}
	if !canWrite && (err == nil || !strings.Contains(err.Error(), "write readonly/y.txt")) {
		t.Errorf("a file that cannot be written is an error naming it: %v", err)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestStrategiesRuleParsesValidatesAndAdvises(t *testing.T) {
	for text, want := range map[string][]string{
		"use macros and helper functions to remove duplicated code": {"helper_function", "macro"},
		"avoid copy-paste: extract a helper":                        {"helper_function"},
		"repeated code goes into macros":                            {"macro"},
		"use rstest parameterized tests against repeated tests":     {"rstest"},
		"use macros": nil, // no word about duplication
	} {
		var got []string
		for _, c := range Parse(text) {
			if c.Key == "duplication.strategies" {
				got = c.Value.([]string)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
	s := Defaults()
	if err := s.Apply("duplication.strategies", []string{"macro", "magic"}); err == nil || !strings.Contains(err.Error(), "helper_function, macro and rstest") {
		t.Errorf("an unknown strategy was accepted: %v", err)
	}
	if err := s.Apply("duplication.strategies", []string{"macro"}); err != nil || s.Duplication.Strategies[0] != "macro" {
		t.Errorf("%v %v", err, s.Duplication.Strategies)
	}
	advice := Defaults().Duplication.StrategyAdvice()
	for _, w := range []string{"helper function", "macro", `rstest = "0.28.0"`, "#[case]", "#[fixture]"} {
		if !strings.Contains(advice, w) {
			t.Errorf("advice misses %q: %s", w, advice)
		}
	}
	if (Duplication{}).StrategyAdvice() != "" {
		t.Error("no strategies, no advice")
	}
}
