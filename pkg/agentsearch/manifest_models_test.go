package agentsearch

import (
	"testing"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/agentjobs"
)

// AGY rejects models outside its catalog at once ("invalid model selection"), so the router
// must never prefer one.
func TestDefaultManifests_AGYModelIsInAGYCatalog(t *testing.T) {
	for _, m := range DefaultManifests() {
		if m.Type != agent.AgentAGY && m.Type != agent.AgentAntigravity {
			continue
		}
		if !agentjobs.IsAGYModel(m.PreferredModel) {
			t.Errorf("manifest %s prefers %q, which AGY does not offer (allowed: %v)", m.ID, m.PreferredModel, agentjobs.AllowedAGYModels())
		}
	}
}
