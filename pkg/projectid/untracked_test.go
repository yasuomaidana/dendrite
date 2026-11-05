package projectid

import (
	"path/filepath"
	"testing"
)

func TestUntracked_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "untracked.json")
	u, err := LoadUntracked(path)
	if err != nil {
		t.Fatal(err)
	}
	if u.Has("dpid_x") {
		t.Fatal("a new set is empty")
	}
	u.Add("dpid_x", "Kalman-Filter", "/p/Kalman-Filter")
	if err := u.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUntracked(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Has("dpid_x") || got.Projects["dpid_x"].Name != "Kalman-Filter" || got.Projects["dpid_x"].RemovedAt.IsZero() {
		t.Fatalf("round trip lost data: %+v", got.Projects)
	}
	if ids := got.IDs(); len(ids) != 1 || ids[0] != "dpid_x" {
		t.Fatalf("IDs = %v", ids)
	}
}
