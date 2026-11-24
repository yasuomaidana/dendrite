package coreclient

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/testkit"
)

func TestFunctionsReadsEveryPageInOrder(t *testing.T) {
	list, err := testClient(t).Functions(context.Background(), target(t, "a.exe", "MZ\x90\x00 HARD MANY"))
	testkit.Check(t, err)
	testkit.Equal(t, "functions", len(list.Functions), 103)
	testkit.Equal(t, "total", list.Total, 103)
	testkit.Equal(t, "format", list.Format, "pe")
	testkit.Equal(t, "arch", list.Arch, "x86_64")
	testkit.Equal(t, "first", *list.Functions[0].Name, "main")
	testkit.Equal(t, "first complexity", list.Functions[0].Cyclomatic, 5)
	if list.Functions[3].Name != nil || list.Truncated {
		t.Errorf("an unnamed function has a nil name and the list was not cut: %+v truncated=%t", list.Functions[3], list.Truncated)
	}
	cut, err := testClient(t).Functions(context.Background(), target(t, "b.exe", "MZ BUDGET"))
	testkit.Check(t, err)
	testkit.Equal(t, "truncated", cut.Truncated, true)
}

func TestFunctionsPageHonoursTheOffsetAndTheComplexityFloor(t *testing.T) {
	path := target(t, "a.exe", "MZ\x90\x00 HARD MANY")
	page, err := testClient(t).FunctionsPage(context.Background(), path, 0, 16)
	testkit.Check(t, err)
	testkit.Equal(t, "only the hard one", len(page.Functions), 1)
	testkit.Equal(t, "its name", *page.Functions[0].Name, "crypt_loop")
	if page.NextOffset != nil {
		t.Errorf("one page is all there is: next %v", *page.NextOffset)
	}
	page, err = testClient(t).FunctionsPage(context.Background(), path, 48, 0)
	testkit.Check(t, err)
	testkit.Equal(t, "offset", page.Offset, 48)
	if page.NextOffset == nil || *page.NextOffset != 96 {
		t.Errorf("the second page of 48 points at 96: %v", page.NextOffset)
	}
}

func TestFunctionsRefusalsAndFailures(t *testing.T) {
	ctx := context.Background()
	if _, err := testClient(t).Functions(ctx, " "); err == nil {
		t.Error("an empty path is refused before the sidecar is asked")
	}
	if _, err := testClient(t).Functions(ctx, target(t, "a.txt", "not a pe")); !errors.Is(err, ErrNoFunctions) {
		t.Errorf("not a PE image: %v", err)
	}
	for mode, want := range map[string]error{"nofunctions": ErrFunctionsUnsupported, "funcfail": nil} {
		t.Setenv("TESTSIDECAR_MODE", mode)
		_, err := testClient(t).Functions(ctx, target(t, "a.exe", "MZ"))
		if err == nil || (want != nil && !errors.Is(err, want)) {
			t.Errorf("mode %s: %v, want %v", mode, err, want)
		}
	}
}
