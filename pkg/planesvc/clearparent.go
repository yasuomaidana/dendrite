package planesvc

import (
	"context"
	"fmt"

	"Dendrite/pkg/sidecar"
)

// ClearParentProtocolMinor is the first dendrite-plane protocol minor (1.5) whose issue.update clears a
// parent: it sends an empty string as null, the only value Plane takes for it.
const ClearParentProtocolMinor = 5

// Health asks the sidecar for its handshake: name, version and protocol.
func (s *Service) Health(ctx context.Context) (sidecar.Health, error) {
	var h sidecar.Health
	err := s.call(ctx, false, "health", nil, &h)
	return h, err
}

// RequireClearParent refuses a sidecar too old to clear a parent, naming the fix.
func RequireClearParent(h sidecar.Health) error {
	if h.Minor() < ClearParentProtocolMinor {
		return fmt.Errorf("dendrite-plane protocol %s cannot clear a parent (it needs 1.%d): run `dendrite update --dev`", h.Protocol, ClearParentProtocolMinor)
	}
	return nil
}

// CanClearParent checks that the running sidecar is new enough to clear a parent.
func (s *Service) CanClearParent(ctx context.Context) error {
	h, err := s.Health(ctx)
	if err != nil {
		return err
	}
	return RequireClearParent(h)
}
