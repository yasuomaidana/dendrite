package synapse

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
)

func writePlaneJSON(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".dendrite", "plane.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPeersOfReadsAPeersOwnPlaneProject(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("ws/a")
	bDir, bID := e.project("ws/b") // plane.json only
	cDir, cID := e.project("ws/c") // unreadable plane.json
	dDir, dID := e.project("ws/d") // no plane.json
	writePlaneJSON(t, bDir, `{"identifier":"BEE","project_id":"uuid-b","name":"b","workspace":"ws"}`)
	writePlaneJSON(t, cDir, `{not json`)
	e.synapse("s1",
		Member{ProjectID: aID, Alias: "a", Role: RoleConsumer, PathHint: aDir},
		Member{ProjectID: bID, Alias: "b", Role: RoleProducer, PathHint: bDir},
		Member{ProjectID: cID, Alias: "c", Role: RolePeer, PathHint: cDir},
		Member{ProjectID: dID, Alias: "d", Role: RolePeer, PathHint: dDir})

	groups, err := PeersOf(e.store, e.index, aID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("%+v, %v", groups, err)
	}
	got := map[string]string{}
	for _, p := range groups[0].Peers {
		got[p.Alias] = p.PlaneProject
	}
	if got["b"] != "BEE" || got["c"] != "" || got["d"] != "" {
		t.Errorf("Plane projects = %v: a peer's own plane.json is the last source, and a broken or missing one gives none", got)
	}
}

func TestBackfillPlaneFillsOnlyWhatIsMissing(t *testing.T) {
	e := newHealEnv(t)
	xDir, xID := e.project("ws/x") // has plane.json, no binding in the index
	yDir, yID := e.project("ws/y") // already bound
	zDir, zID := e.project("ws/z") // nothing to read
	writePlaneJSON(t, xDir, `{"identifier":"EX","project_id":"uuid-x"}`)
	writePlaneJSON(t, yDir, `{"identifier":"WHY","project_id":"uuid-y"}`)
	e.index.SetPlane(yID, planeBinding("KEEP"))
	_ = zDir

	n, err := BackfillPlane(e.index)
	if err != nil || n != 1 {
		t.Fatalf("filled %d, %v: only x qualifies", n, err)
	}
	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{xID: "EX", yID: "KEEP", zID: ""} {
		entry, _ := reloaded.Lookup(id)
		got := ""
		if entry.Plane != nil {
			got = entry.Plane.Identifier
		}
		if got != want {
			t.Errorf("%s: Plane %q, want %q", id, got, want)
		}
	}
	if n, err := BackfillPlane(e.index); n != 0 || err != nil {
		t.Errorf("a second run has nothing to do: %d, %v", n, err)
	}
}

func TestRefreshIndexApplyBackfillsEvenWhenNothingIsStale(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("ws/a")
	writePlaneJSON(t, dir, `{"identifier":"AAA","project_id":"uuid-a"}`)

	if _, err := RefreshIndex(e.store, e.index, RefreshOptions{}); err != nil {
		t.Fatal(err)
	}
	if entry, _ := e.index.Lookup(id); entry.Plane != nil {
		t.Fatal("a dry refresh must not write the index")
	}
	if _, err := RefreshIndex(e.store, e.index, RefreshOptions{Apply: true}); err != nil {
		t.Fatal(err)
	}
	if entry, _ := e.index.Lookup(id); entry.Plane == nil || entry.Plane.Identifier != "AAA" {
		t.Errorf("entry = %+v", entry)
	}
}
