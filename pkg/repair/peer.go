package repair

import (
	"path"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

// Peer is a dependency that is a member of a synapse the consumer belongs to: a repository we can push to.
type Peer struct {
	Alias string
	// Checkout is its local checkout ("" when it is not on this machine).
	Checkout string
	// Remote is its repository URL, when the synapse records one.
	Remote string
	// PlaneProject is the Plane project that tracks its tickets.
	PlaneProject string
}

// FindPeer looks for dep among the synapse peers of the project at consumer: a member whose alias, module or
// last module segment is dep. Archived synapses are skipped.
func FindPeer(st *synapse.Store, consumer, dep string) (Peer, bool) {
	id, err := projectid.Read(projectid.StateRoot(consumer))
	if err != nil {
		return Peer{}, false
	}
	syns, _ := st.ReferencingProject(id)
	for _, s := range syns {
		if s.State == synapse.StateArchived {
			continue
		}
		for _, m := range s.Members {
			if m.ProjectID != id && named(m, dep) {
				return peerOf(m), true
			}
		}
	}
	return Peer{}, false
}

// named reports whether dep is how a manifest would name member m.
func named(m synapse.Member, dep string) bool {
	return m.Alias == dep || m.Module == dep || path.Base(m.Module) == dep
}

func peerOf(m synapse.Member) Peer {
	p := Peer{Alias: m.Alias, Remote: m.Remote, PlaneProject: m.PlaneProject}
	if dir, err := synapse.Resolve(m); err == nil {
		p.Checkout = dir
	}
	return p
}
