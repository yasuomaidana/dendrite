package synapsesync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"Dendrite/pkg/synapse"
)

type indexCalls struct {
	n    atomic.Int32
	last atomic.Value // string
	err  error
}

func (c *indexCalls) index(_ context.Context, dir string) (RAGResult, error) {
	c.n.Add(1)
	c.last.Store(dir)
	if c.err != nil {
		return RAGResult{}, c.err
	}
	return RAGResult{FilesChanged: 3, EmbedPending: 7, DurationMS: 12}, nil
}

func ragOptions(t *testing.T, calls *indexCalls, dir string) Options {
	t.Helper()
	return Options{StartDir: dir, StateDir: t.TempDir(), Indexer: calls.index}
}

func TestSyncIndexesAChangedRepoOnceAndSkipsItUntilItChanges(t *testing.T) {
	dir, _ := newRepo(t, "app")
	calls := &indexCalls{}
	opts := ragOptions(t, calls, dir)

	rep, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Members[0].RAG
	if calls.n.Load() != 1 || r == nil || !r.Indexed || r.FilesChanged != 3 || r.EmbedPending != 7 {
		t.Fatalf("first pass: calls=%d rag=%+v", calls.n.Load(), r)
	}

	rep, _ = Run(context.Background(), opts)
	if calls.n.Load() != 1 || rep.Members[0].RAG.Indexed || rep.Members[0].RAG.Skipped != "unchanged since the last index" {
		t.Errorf("second pass over the same commit: calls=%d rag=%+v", calls.n.Load(), rep.Members[0].RAG)
	}

	writeAndCommit(t, dir, "new.go")
	rep, _ = Run(context.Background(), opts)
	if calls.n.Load() != 2 || !rep.Members[0].RAG.Indexed {
		t.Errorf("after a commit: calls=%d rag=%+v", calls.n.Load(), rep.Members[0].RAG)
	}
}

func TestDirtyTreesAreIndexedEveryPassAndDryRunOnlyReports(t *testing.T) {
	dir, _ := newRepo(t, "app")
	if err := os.WriteFile(filepath.Join(dir, "wip.go"), []byte("package wip\n"), 0o644); err != nil { // uncommitted
		t.Fatal(err)
	}
	calls := &indexCalls{}
	opts := ragOptions(t, calls, dir)
	Run(context.Background(), opts)
	Run(context.Background(), opts)
	if calls.n.Load() != 2 {
		t.Errorf("a dirty tree was indexed %d times in 2 passes, want 2 (it is never marked clean)", calls.n.Load())
	}

	opts.DryRun = true
	rep, _ := Run(context.Background(), opts)
	if calls.n.Load() != 2 || rep.Members[0].RAG.Skipped != "dry-run: would refresh the index" {
		t.Errorf("dry run: calls=%d rag=%+v", calls.n.Load(), rep.Members[0].RAG)
	}
}

func TestRAGFailuresAreReportedWithoutStoppingTheRest(t *testing.T) {
	dir, _ := newRepo(t, "app")
	calls := &indexCalls{err: errors.New("dendrite-rag sidecar unavailable")}
	rep, err := Run(context.Background(), ragOptions(t, calls, dir))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() || rep.Members[0].RAG.Indexed || len(rep.Members[0].Errors) != 1 {
		t.Errorf("report = %+v", rep.Members[0])
	}
	if rep.Members[0].Repo == nil {
		t.Error("the git half did not run")
	}
}

func TestNoRAGAndTheSingleHalfModesSkipTheStep(t *testing.T) {
	dir, _ := newRepo(t, "app")
	for name, mutate := range map[string]func(*Options){
		"NoRAG":     func(o *Options) { o.NoRAG = true },
		"GitOnly":   func(o *Options) { o.GitOnly = true },
		"PlaneOnly": func(o *Options) { o.PlaneOnly = true },
	} {
		calls := &indexCalls{}
		opts := ragOptions(t, calls, dir)
		mutate(&opts)
		rep, _ := Run(context.Background(), opts)
		if calls.n.Load() != 0 || rep.Members[0].RAG != nil {
			t.Errorf("%s: indexer ran (%d) or reported %+v", name, calls.n.Load(), rep.Members[0].RAG)
		}
	}
}

func TestRemoteMembersAreSkippedWithoutErrors(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	root, _ := newRepo(t, "root")
	remote := synapse.Member{ProjectID: "dpid_remote", Alias: "lib", Role: synapse.RolePeer, Source: synapse.SourceRemote, Remote: "https://example.invalid/lib.git", PathHint: filepath.Join(t.TempDir(), "absent")}
	st := synapse.NewStore(t.TempDir())
	s := &synapse.Synapse{ID: "pair", Name: "pair", State: synapse.StateActive, Channel: synapse.ChannelDev, Members: []synapse.Member{member(t, root, "root", ""), remote}}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	calls := &indexCalls{}
	opts := Options{StartDir: root, SynapseID: "pair", Store: st, StateDir: t.TempDir(), Indexer: calls.index}
	rep, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		t.Errorf("a remote member produced an error: %+v", rep.Members)
	}
	if calls.n.Load() != 1 {
		t.Errorf("indexed %d repositories, want only the local one", calls.n.Load())
	}
	if len(rep.Synapses) != 1 || rep.Synapses[0].After == synapse.StateBroken {
		t.Errorf("a synapse with a remote-only member must not turn broken: %+v", rep.Synapses)
	}
}
