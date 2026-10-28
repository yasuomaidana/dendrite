package rag

import (
	"context"
	"testing"
)

func TestRetrieversShareOnePooledSidecar(t *testing.T) {
	r1, _ := standInRetriever(t)
	dir := ragProject(t)
	for i := 0; i < 3; i++ {
		// A new Retriever per request, as the MCP tools do.
		r := &Retriever{Locator: r1.Locator, EmbedBudget: r1.EmbedBudget}
		if _, err := r.Retrieve(context.Background(), dir, "where is LocateBinary", SearchOptions{TopK: 2}, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r1.Status(context.Background(), dir); err != nil {
		t.Fatal(err)
	}

	poolsMu.Lock()
	defer poolsMu.Unlock()
	var found bool
	for key, e := range pools {
		st := e.pool.Stats()
		if st.Served < 7 {
			continue
		}
		found = true
		if st.Starts != 1 {
			t.Errorf("pool %q started %d sidecar processes for %d calls, want 1", key, st.Starts, st.Served)
		}
	}
	if !found {
		t.Error("no pool served the retrievals")
	}
}

func TestEnvChangesGetTheirOwnSidecar(t *testing.T) {
	a := envFingerprint(nil)
	t.Setenv("DENDRITE_RAG_TEST_FINGERPRINT", "changed")
	if b := envFingerprint(nil); a == b {
		t.Error("a changed environment kept the same pool key")
	}
	if envFingerprint([]string{"X=1"}) == envFingerprint([]string{"X=2"}) {
		t.Error("extra environment lines are not part of the key")
	}
}
