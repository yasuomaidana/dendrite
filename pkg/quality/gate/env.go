package gate

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// liveEnvPrefixes are the variables a test run must not inherit: the CLI loads them from the user's .env, and a
// test that sees them reaches the live Plane, gateway and Dendrite state instead of its own fixtures.
var liveEnvPrefixes = []string{"PLANE_", "DENDRITE_", "LITELLM_"}

// CleanEnv returns environ without the variables that point at the user's live services.
func CleanEnv(environ []string) []string {
	kept := make([]string, 0, len(environ))
next:
	for _, kv := range environ {
		for _, p := range liveEnvPrefixes {
			if strings.HasPrefix(kv, p) {
				continue next
			}
		}
		kept = append(kept, kv)
	}
	return kept
}

// cleanRunner runs a command in dir with the live-service variables removed from its environment.
func cleanRunner(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = CleanEnv(os.Environ())
	return cmd.CombinedOutput()
}
