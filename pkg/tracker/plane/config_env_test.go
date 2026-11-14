package plane

import (
	"errors"
	"strings"
	"testing"
)

func TestConfigFromEnvRequiresToken(t *testing.T) {
	t.Setenv(APIKeyEnv, "")
	_, err := ConfigFromEnv()
	if !errors.Is(err, ErrMissingAPIToken) {
		t.Fatalf("err = %v, want ErrMissingAPIToken", err)
	}
	for _, hint := range []string{"PLANE_API_KEY", "$DENDRITE_HOME/.env"} {
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("error %q does not tell the user about %s", err, hint)
		}
	}
	if _, err := NewClientFromEnv(); !errors.Is(err, ErrMissingAPIToken) {
		t.Errorf("NewClientFromEnv err = %v, want ErrMissingAPIToken", err)
	}
}

func TestConfigFromEnvUsesConfiguredToken(t *testing.T) {
	t.Setenv(APIKeyEnv, "  real-token  ")
	t.Setenv("PLANE_BASE_URL", "http://plane.example")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DirectToken != "real-token" || cfg.BaseURL != "http://plane.example" || cfg.ProjectID != "proj" || cfg.WorkspaceSlug != "ws" {
		t.Errorf("cfg = %+v", cfg)
	}
}
