package inference

import (
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/inference/bench"
	"Dendrite/pkg/rag"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatModelsDropsEmbeddersAndOrdersSmallestFirst(t *testing.T) {
	tags := `{"models":[{"name":"big:27b","size":18000000000},{"name":"nomic-embed-text:latest","size":274000000},
		{"name":"small:3b","size":2000000000},{"name":"mid:8b","size":4700000000}]}`
	got, err := chatModels([]byte(tags))
	if err != nil || strings.Join(got, ",") != "small:3b,mid:8b,big:27b" {
		t.Errorf("chatModels = %v, %v", got, err)
	}
	if _, err := chatModels([]byte(`{"models":[{"name":"x-embed","size":1}]}`)); err == nil {
		t.Error("a machine with only embedding models must report that no chat model is installed")
	}
	if _, err := chatModels([]byte(`not json`)); err == nil {
		t.Error("bad JSON accepted")
	}
}

func TestBenchUserMessageMirrorsTheAskPromptShape(t *testing.T) {
	msg := benchUserMessage("What is X?", []rag.Chunk{
		{FilePath: "pkg/a.go", LineStart: 3, LineEnd: 9, Content: "  func A() {}\n"},
		{FilePath: "pkg/b.go", LineStart: 1, LineEnd: 2, Content: "func B() {}"},
	})
	for _, want := range []string{"Retrieved Project Context:", "[1] Source: pkg/a.go (lines 3-9)\nfunc A() {}", "[2] Source: pkg/b.go (lines 1-2)", "Question: What is X?"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if len(benchQuestions) < 5 {
		t.Error("too few benchmark questions to compare models")
	}
}

// Runners over the commands this package builds (the full tree lives in package main).
var cliOutput = clitest.Output(NewSpikeCmd)

func TestMain(m *testing.M) { clitest.Main(m) }

// --save writes each measured model's tokens/s into the capability catalog: a known model is
// updated, an unknown one is added, a model without a measurement is left out.
func TestRecordThroughputWritesTheCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	if err := inference.SaveCatalog(&inference.Catalog{Models: []inference.ModelCapability{inference.DefaultCapability("known:8b", inference.EngineOllama)}}, path); err != nil {
		t.Fatal(err)
	}
	sums := []bench.Summary{{Model: "known:8b", MedianTokPerS: 31.5}, {Model: "new:4b", MedianTokPerS: 55}, {Model: "failed:1b"}}
	n, err := recordThroughput(sums, path)
	if err != nil || n != 2 {
		t.Fatalf("recordThroughput = %d, %v", n, err)
	}
	cat, err := inference.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, m := range cat.Models {
		got[m.ID] = m.TokensPerSec
	}
	if len(got) != 2 || got["known:8b"] != 31.5 || got["new:4b"] != 55 {
		t.Fatalf("catalog = %v", got)
	}

}

// Nothing measured writes nothing; a catalog that cannot be read is an error.
func TestRecordThroughputWithoutMeasurementsOrCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	if err := inference.SaveCatalog(&inference.Catalog{}, path); err != nil {
		t.Fatal(err)
	}
	sums := []bench.Summary{{Model: "new:4b", MedianTokPerS: 55}}
	if n, err := recordThroughput([]bench.Summary{{Model: "failed:1b"}}, path); err != nil || n != 0 {
		t.Fatalf("nothing measured: %d, %v", n, err)
	}
	if _, err := recordThroughput(sums, filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("a catalog that cannot be read must fail")
	}
}
