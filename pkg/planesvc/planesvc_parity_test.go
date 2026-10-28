//go:build sidecar_parity

package planesvc

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"Dendrite/pkg/sidecar"
)

// sidecarBinary finds a built dendrite-plane for the parity test: DENDRITE_PLANE_TEST_BIN, else the
// sibling checkout's release build. It is opt-in (go test -tags sidecar_parity) and fails when neither exists.
func sidecarBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{os.Getenv("DENDRITE_PLANE_TEST_BIN")}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "RustRoverProjects", "dendrite-plane-rs", "target", "release", "dendrite-plane"))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	t.Fatal("no dendrite-plane binary: set DENDRITE_PLANE_TEST_BIN or run `cargo build --release` in ~/RustRoverProjects/dendrite-plane-rs")
	return ""
}

func TestSidecarAndGoFallbackReturnTheSameSnapshot(t *testing.T) {
	bin := sidecarBinary(t)
	t.Setenv("PLANE_API_KEY", "k")
	t.Setenv(sidecar.EnvVar(BinaryName), bin)
	t.Setenv("DENDRITE_PLANE_CACHE_DIR", t.TempDir())

	f := newLocalPlane(t)
	real := New(sidecar.Locator{LookPath: func(string) (string, error) { return "", os.ErrNotExist }})
	defer real.Close()

	viaSidecar, out, err := real.Snapshot(context.Background(), f.client(t), true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Origin != sidecar.OriginSidecar {
		t.Fatalf("sidecar did not answer: %+v", out)
	}
	viaGo, err := goSnapshot(context.Background(), f.client(t))
	if err != nil {
		t.Fatal(err)
	}

	checkSnapshot(t, viaSidecar)
	checkSnapshot(t, viaGo)
	if !reflect.DeepEqual(ids(viaSidecar), ids(viaGo)) || !reflect.DeepEqual(viaSidecar.Statuses, viaGo.Statuses) {
		t.Errorf("issues/statuses differ:\nsidecar %+v\ngo      %+v", viaSidecar.Statuses, viaGo.Statuses)
	}
	for id, want := range viaGo.Relations {
		got := viaSidecar.Relations[id]
		if !reflect.DeepEqual(relIDs(got), relIDs(want)) {
			t.Errorf("relations of %s differ: sidecar %v, go %v", id, relIDs(got), relIDs(want))
		}
	}
	if len(viaSidecar.Relations) != len(viaGo.Relations) {
		t.Errorf("relation sets: sidecar %d, go %d", len(viaSidecar.Relations), len(viaGo.Relations))
	}

	// A second, non-fresh call is served from the sidecar's cache; invalidation brings it back.
	before := len(f.relCalls)
	if _, _, err := real.Snapshot(context.Background(), f.client(t), false); err != nil {
		t.Fatal(err)
	}
	if len(f.relCalls) != before {
		t.Errorf("a warm snapshot made %d relation requests", len(f.relCalls)-before)
	}
	real.InvalidateRelations(context.Background(), f.client(t), "i2")
	if _, _, err := real.Snapshot(context.Background(), f.client(t), false); err != nil {
		t.Fatal(err)
	}
	if got := len(f.relCalls) - before; got != 1 {
		t.Errorf("after invalidating one issue, %d relation requests were made, want 1", got)
	}
}
