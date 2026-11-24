package planesvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/sidecar"
)

func TestRequireClearParentNeedsProtocolMinor5(t *testing.T) {
	for protocol, wantErr := range map[string]bool{"1.4.2": true, "1.0.0": true, "bogus": true, "1.5.0": false, "1.6.1": false} {
		err := RequireClearParent(sidecar.Health{Protocol: protocol})
		if (err != nil) != wantErr {
			t.Errorf("protocol %q: err = %v, want error %v", protocol, err, wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "dendrite update --dev") {
			t.Errorf("protocol %q: the error names no fix: %v", protocol, err)
		}
	}
}

func TestHealthComesFromTheSidecar(t *testing.T) {
	svc := realService()
	defer svc.Close()
	h, err := svc.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != BinaryName || h.Minor() == 0 {
		t.Errorf("health = %+v", h)
	}
}

func TestCanClearParentFailsWithoutASidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar(BinaryName), "")
	svc := absentService()
	defer svc.Close()
	if err := svc.CanClearParent(context.Background()); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("err = %v, want ErrSidecarMissing", err)
	}
}
