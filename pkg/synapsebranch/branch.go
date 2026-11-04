// Package synapsebranch reads the branch settings a synapse member declares for a project.
package synapsebranch

import (
	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/synapse"
)

// BranchOverride is what the synapses that contain projectID declare about its integration branch and
// remotes: the first non-archived synapse (by id) whose member sets any of them wins. A project in no
// synapse, or one whose members declare nothing, overrides nothing.
func BranchOverride(st *synapse.Store, projectID string) branchpolicy.Override {
	syns, err := st.ReferencingProject(projectID)
	if err != nil {
		return branchpolicy.Override{}
	}
	for _, s := range syns {
		if s.State == synapse.StateArchived {
			continue
		}
		m, _ := s.Member(projectID)
		ov := branchpolicy.Override{Branch: m.Branch, PushRemote: m.PushRemote, TargetRemote: m.TargetRemote}
		if ov != (branchpolicy.Override{}) {
			return ov
		}
	}
	return branchpolicy.Override{}
}
