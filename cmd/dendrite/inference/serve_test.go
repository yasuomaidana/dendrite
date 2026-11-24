package inference

import (
	"Dendrite/pkg/llamalease/llamaleasetest"
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a buffer the command writes to while the test reads it. ready is closed once the
// banner's last line has been written.
type syncBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	once  sync.Once
	ready chan struct{}
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{ready: make(chan struct{})} }

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	if strings.Contains(b.buf.String(), "Press Ctrl+C") {
		b.once.Do(func() { close(b.ready) })
	}
	return n, err
}

// awaitBanner waits until the banner is complete; the command ending first is a failure.
func (b *syncBuffer) awaitBanner(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case <-b.ready:
	case err := <-done:
		t.Fatalf("serve ended before it announced the server: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the banner")
	}
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// serve starts the server on the named model through the lease manager, holds a lease while it
// runs, keeps the lease alive, and releases it when interrupted.
func TestSpike_ServeHoldsTheServerUntilInterrupted(t *testing.T) {
	r := llamaleasetest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	out := newSyncBuffer()
	done := make(chan error, 1)
	go func() { done <- runSpikeServe(ctx, r.M, 8123, "small", out) }()

	out.awaitBanner(t, done)
	for _, want := range []string{"llama-server serves small at http://127.0.0.1:8123", "http://127.0.0.1:8123/v1/chat/completions", "it stops 300ms after the last session"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("banner lacks %q:\n%s", want, out.String())
		}
	}
	if got := r.Holders(t); len(got) != 1 || got[0] != "serve" {
		t.Fatalf("holders while serving = %v", got)
	}
	time.Sleep(r.M.TTL() + r.M.TTL()/4) // past the lease's lifetime: only the heartbeat keeps the hold
	if got := r.Holders(t); len(got) != 1 {
		t.Fatalf("the hold expired while serving: %v", got)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := r.Holders(t); len(got) != 0 {
		t.Fatalf("holders after the interrupt = %v", got)
	}
}

// A server that already answers and was not started by Dendrite is held, not restarted, and the
// banner says Dendrite will not stop it.
func TestSpike_ServeHoldsAServerStartedByHand(t *testing.T) {
	r := llamaleasetest.New(t)
	r.Answering.Store(true)
	out := newSyncBuffer()
	if err := holdServer(context.Background(), r.M, holdOptions{Holder: "hold", For: 20 * time.Millisecond}, func() { _, _ = out.Write([]byte(serveBanner(r.M, 8123))) }); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"serves a model started by hand", "keeps running"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("banner lacks %q:\n%s", want, out.String())
		}
	}
}

// What cannot be served is an error before anything starts.
func TestSpike_ServeRefusesWhatItCannotStart(t *testing.T) {
	r := llamaleasetest.New(t)
	ctx := context.Background()
	out := newSyncBuffer()
	err := runSpikeServe(ctx, r.M, 8123, "absent", out)
	if err == nil || !strings.Contains(err.Error(), `model "absent" is not installed`) || !strings.Contains(err.Error(), "big, small") {
		t.Fatalf("an unknown model must be named with what is installed: %v", err)
	}

	r.Fits = errors.New("the model does not fit the memory budget")
	if err := runSpikeServe(ctx, r.M, 8123, "big", out); err == nil || !strings.Contains(err.Error(), "does not fit") {
		t.Fatalf("a model that does not fit: %v", err)
	}

	t.Setenv("DENDRITE_HOME", t.TempDir())
	if err := runSpikeServe(ctx, r.M, 8123, "", out); err == nil || !strings.Contains(err.Error(), "no GGUF model installed") {
		t.Fatalf("without an installed model: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("nothing may be announced: %q", out.String())
	}
}
