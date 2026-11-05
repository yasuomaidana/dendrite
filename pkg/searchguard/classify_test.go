package searchguard

import (
	"reflect"
	"testing"
)

func TestClassifyBash(t *testing.T) {
	cases := []struct {
		cmd      string
		searches []string
		dendrite bool
	}{
		{`rg "func Foo" pkg/`, []string{"rg"}, false},
		{`rg -n --glob '*.go' Foo`, []string{"rg"}, false},
		{`cd repo && rg Foo`, []string{"rg"}, false},
		{`grep -rn "Foo" .`, []string{"grep"}, false},
		{`grep -R Foo src`, []string{"grep"}, false},
		{`grep --recursive Foo`, []string{"grep"}, false},
		{`grep -d recurse Foo .`, []string{"grep"}, false},
		{`GREP_COLOR=1 rg Foo`, []string{"rg"}, false},
		{`sudo rg Foo`, []string{"rg"}, false},
		{`rg Foo; grep -r Bar .`, []string{"rg", "grep"}, false},
		{`ag Foo`, []string{"ag"}, false},
		// Filters over piped input are not repository searches.
		{`git log --oneline | rg fix`, nil, false},
		{`dendrite rag search "x" | grep -n Foo`, nil, true},
		{`cat file | grep -r pattern`, nil, false},
		// grep without recursion reads named files or stdin: fine.
		{`grep -n Foo main.go`, nil, false},
		{`grep -i Foo`, nil, false},
		// A pipeline command with a path still searches that path.
		{`echo x | rg Foo src/`, []string{"rg"}, false},
		// The pattern may mention rg; the program is what counts.
		{`echo "rg Foo" > notes.txt`, nil, false},
		{`dendrite rag grep 'func Foo'`, nil, true},
		{`dendrite ask "where is Foo?"`, nil, true},
		{`ls -la`, nil, false},
		{``, nil, false},
	}
	for _, c := range cases {
		s, d := Classify("Bash", ToolInput{Command: c.cmd})
		if !reflect.DeepEqual(s, c.searches) || d != c.dendrite {
			t.Errorf("%q = %v dendrite=%v, want %v dendrite=%v", c.cmd, s, d, c.searches, c.dendrite)
		}
	}
}

func TestClassifyOtherTools(t *testing.T) {
	if s, _ := Classify("Grep", ToolInput{Pattern: "Foo"}); !reflect.DeepEqual(s, []string{KindGrepTool}) {
		t.Errorf("Grep tool = %v", s)
	}
	if s, d := Classify("mcp__dendrite__dendrite_rag_retrieve", ToolInput{}); s != nil || !d {
		t.Errorf("dendrite MCP = %v %v", s, d)
	}
	if s, d := Classify("Read", ToolInput{}); s != nil || d {
		t.Errorf("Read = %v %v", s, d)
	}
	if !IsRawSearch(KindRg) || !IsRawSearch(KindGrepTool) || IsRawSearch(KindDendrite) {
		t.Error("IsRawSearch misclassifies kinds")
	}
}

func TestSplitSegmentsHonorsQuotesAndOperators(t *testing.T) {
	segs := splitSegments(`a "b | c" && d 'e;f' | g || h; i`)
	var heads []string
	for _, s := range segs {
		heads = append(heads, s.tokens[0])
	}
	if !reflect.DeepEqual(heads, []string{"a", "d", "g", "h", "i"}) {
		t.Errorf("heads = %v", heads)
	}
	if segs[2].pipeIndex != 1 || segs[1].pipeIndex != 0 {
		t.Errorf("pipe indexes = %d %d", segs[1].pipeIndex, segs[2].pipeIndex)
	}
	if got := segs[0].tokens[1]; got != "b | c" {
		t.Errorf("quoted token = %q", got)
	}
}
