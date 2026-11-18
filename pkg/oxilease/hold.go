package oxilease

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Port is the port the manager's server answers on.
func (m *Manager) Port() int { return m.o.Port }

// Hold takes a lease for holder like a session does. When no server answers it starts one on the
// installed model served as model, or, when none is named, on the largest that fits the memory
// budget. It returns the lease and the id of the model Dendrite serves ("" when the server was
// started by hand). Releasing the lease does not stop the server: its supervisor does, once no
// session has held it for the idle grace.
func (m *Manager) Hold(ctx context.Context, holder, model string) (*Lease, string, error) {
	req := Request{Holder: holder}
	if !m.Running() {
		picked, err := Pick(ctx, model)
		if err != nil {
			return nil, "", err
		}
		req.ModelPath, req.ModelID = picked.Path, picked.ID
	}
	lease, err := m.Acquire(ctx, req)
	if err != nil {
		return nil, "", err
	}
	st, err := m.Status()
	if err != nil {
		_ = lease.Release()
		return nil, "", err
	}
	return lease, st.Model, nil
}

// Pick is the installed model to start: the one served as want, or, when none is named, the
// largest that fits the memory budget.
func Pick(ctx context.Context, want string) (Model, error) {
	if want == "" {
		return BestFitting(ctx)
	}
	var ids []string
	for _, m := range Installed() {
		if m.ID == want {
			return m, nil
		}
		ids = append(ids, m.ID)
	}
	return Model{}, fmt.Errorf("model %q is not installed under ~/.oxillama/models (installed: %s)", want, strings.Join(ids, ", "))
}

// KeepAlive heartbeats the lease every third of its lifetime until the returned stop is called,
// so work that outlasts the lease keeps the server.
func (l *Lease) KeepAlive() (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		beat := time.NewTicker(l.m.TTL() / 3)
		defer beat.Stop()
		for {
			select {
			case <-done:
				return
			case <-beat.C:
				_ = l.Heartbeat()
			}
		}
	}()
	return func() { close(done); <-finished }
}
