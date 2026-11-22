package llamalease_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"Dendrite/pkg/llamalease"
	"Dendrite/pkg/llamalease/llamaleasetest"
)

func TestPickFindsTheNamedModelOrSaysWhatIsInstalled(t *testing.T) {
	llamaleasetest.New(t)
	m, err := llamalease.Pick(context.Background(), "small")
	if err != nil || m.ID != "small" || m.SizeBytes != 2 {
		t.Fatalf("Pick(small) = %+v, %v", m, err)
	}
	if _, err := llamalease.Pick(context.Background(), "absent"); err == nil || err.Error() != fmt.Sprintf(`model "absent" is not installed under %s (installed: big, small)`, llamalease.ModelsDir()) {
		t.Fatalf("Pick(absent) = %v", err)
	}
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if _, err := llamalease.Pick(context.Background(), ""); err == nil {
		t.Fatal("without an installed model there is nothing to pick")
	}
}

// Hold starts the server on the named model and reports the model served; a model that is not
// installed or does not fit is refused.
func TestHoldStartsTheNamedModel(t *testing.T) {
	r := llamaleasetest.New(t)
	ctx := context.Background()
	if r.M.Port() != 8123 {
		t.Fatalf("Port = %d", r.M.Port())
	}
	if _, _, err := r.M.Hold(ctx, "a", "absent"); err == nil {
		t.Fatal("a model that is not installed cannot be held")
	}
	lease, model, err := r.M.Hold(ctx, "a", "small")
	if err != nil || model != "small" {
		t.Fatalf("Hold = %q, %v", model, err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}

	cold := llamaleasetest.New(t)
	cold.Fits = errors.New("too big")
	if _, _, err := cold.M.Hold(ctx, "c", "big"); !errors.Is(err, llamalease.ErrNotFit) {
		t.Fatalf("a model that does not fit = %v", err)
	}
}

// A second Hold joins the running server without naming a model, and KeepAlive keeps a hold past
// the lease's lifetime.
func TestHoldJoinsARunningServerAndKeepAliveKeepsIt(t *testing.T) {
	r := llamaleasetest.New(t)
	ctx := context.Background()
	first, _, err := r.M.Hold(ctx, "a", "small")
	if err != nil {
		t.Fatal(err)
	}
	stop := first.KeepAlive()
	second, model, err := r.M.Hold(ctx, "b", "")
	if err != nil || model != "small" {
		t.Fatalf("second Hold = %q, %v", model, err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(r.M.TTL() + r.M.TTL()/4) // past the lease's lifetime: only the heartbeat keeps it
	if got := r.Holders(t); len(got) != 1 || got[0] != "a" {
		t.Fatalf("holders with KeepAlive running = %v", got)
	}
	stop()
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
}
