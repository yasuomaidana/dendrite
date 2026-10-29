package prune

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ollama(t *testing.T, tags string, deleteStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var deleted []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(tags))
		case r.URL.Path == "/api/delete" && r.Method == http.MethodDelete:
			buf := make([]byte, 256)
			n, _ := r.Body.Read(buf)
			deleted = append(deleted, string(buf[:n]))
			if deleteStatus != http.StatusOK {
				http.Error(w, "nope", deleteStatus)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s, &deleted
}

const tags = `{"models":[{"name":"qwen3.8:27b","size":18000000000},{"name":"hermes3:8b","size":4900000000},{"name":"nomic-embed-text:latest","size":270000000},{"name":"old-model:latest","size":2000000000}]}`

func TestInstalledListsLargestFirstAndUnusedSkipsKeptAndEmbeddingModels(t *testing.T) {
	srv, _ := ollama(t, tags, http.StatusOK)
	got, err := Installed(context.Background(), http.DefaultClient, srv.URL+"/")
	if err != nil || len(got) != 4 || got[0].Name != "qwen3.8:27b" {
		t.Fatalf("%+v %v", got, err)
	}
	unused := Unused(got, []string{"hermes3:8b", "old-model"}) // "old-model" matches "old-model:latest"
	if len(unused) != 1 || unused[0].Name != "qwen3.8:27b" {
		t.Errorf("unused = %+v", unused)
	}
}

func TestInstalledReportsAnUnreachableOrUnreadableOllama(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if _, err := Installed(context.Background(), http.DefaultClient, dead.URL); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("unreachable: %v", err)
	}
	srv, _ := ollama(t, "not json", http.StatusOK)
	if _, err := Installed(context.Background(), http.DefaultClient, srv.URL); err == nil {
		t.Error("unreadable list must fail")
	}
	if _, err := Installed(context.Background(), http.DefaultClient, "http://bad\x7fhost"); err == nil {
		t.Error("invalid URL must fail")
	}
}

func TestRemoveDeletesAndSurfacesFailures(t *testing.T) {
	srv, deleted := ollama(t, tags, http.StatusOK)
	if err := Remove(context.Background(), http.DefaultClient, srv.URL, "old-model:latest"); err != nil || len(*deleted) != 1 || !strings.Contains((*deleted)[0], "old-model:latest") {
		t.Fatalf("%v %v", err, *deleted)
	}
	failing, _ := ollama(t, tags, http.StatusNotFound)
	if err := Remove(context.Background(), http.DefaultClient, failing.URL, "x"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a refusal must be an error: %v", err)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if Remove(context.Background(), http.DefaultClient, dead.URL, "x") == nil || Remove(context.Background(), http.DefaultClient, "http://bad\x7fhost", "x") == nil {
		t.Error("an unreachable or invalid URL must fail")
	}
}

func TestConfigRoutesRestoreNoteAndSizes(t *testing.T) {
	cfg := "model: ollama/hermes3:8b\nmodel: ollama_chat/gpt-oss:20b\nmodel: openai/gpt\n"
	got := ModelsInConfig(cfg)
	if len(got) != 2 || got[0] != "hermes3:8b" || got[1] != "gpt-oss:20b" {
		t.Errorf("routes = %v", got)
	}
	if RestoreNote("x:1b") != "ollama pull x:1b" || GB(3<<30) != "3.0 GB" || !Embedding("Nomic-Embed-Text") || Embedding("hermes3") {
		t.Error("helpers")
	}
}
