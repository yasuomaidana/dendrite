package main

import (
	"strings"
	"testing"

	"Dendrite/pkg/rag"
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
