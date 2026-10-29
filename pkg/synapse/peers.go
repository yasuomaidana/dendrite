package synapse

import (
	"sort"

	"Dendrite/pkg/projectid"
)

// Peer is another member of a synapse, as seen from one project of it.
type Peer struct {
	Alias        string       `json:"alias"`
	Role         Role         `json:"role"`
	ProjectID    string       `json:"project_id"`
	PlaneProject string       `json:"plane_project,omitempty"` // Plane identifier, "" when unknown
	Path         string       `json:"path,omitempty"`          // where the checkout resolves now ("" when it does not)
	Status       MemberStatus `json:"status"`
}

// PeerGroup is one synapse that contains the project, with the project's other members.
type PeerGroup struct {
	Synapse string `json:"synapse"`
	Name    string `json:"name"`
	Self    string `json:"self"` // the project's own alias in this synapse
	Peers   []Peer `json:"peers"`
}

// PeersOf lists, per synapse that contains projectID, the other members. Archived synapses are
// skipped. A peer without a Plane identifier in the manifest takes the one the global index
// records for it (ix may be nil), and failing that the one in its own checkout. A project in several synapses appears in several groups, which
// is how a shared project knows which synapse a request or a ticket belongs to.
func PeersOf(st *Store, ix *projectid.Index, projectID string) ([]PeerGroup, error) {
	syns, err := st.ReferencingProject(projectID)
	if err != nil {
		return nil, err
	}
	var groups []PeerGroup
	for _, s := range syns {
		if s.State == StateArchived {
			continue
		}
		g := PeerGroup{Synapse: s.ID, Name: s.Name}
		for _, m := range s.Members {
			if m.ProjectID == projectID {
				g.Self = m.Alias
				continue
			}
			p := Peer{Alias: m.Alias, Role: m.Role, ProjectID: m.ProjectID, PlaneProject: m.PlaneProject, Status: StatusOf(m)}
			if dir, err := Resolve(m); err == nil {
				p.Path = dir
			}
			if p.PlaneProject == "" && ix != nil {
				if e, ok := ix.Lookup(m.ProjectID); ok && e.Plane != nil {
					p.PlaneProject = e.Plane.Identifier
				}
			}
			if p.PlaneProject == "" && p.Path != "" { // the checkout knows its own Plane project
				if b := readPlaneLink(p.Path); b != nil {
					p.PlaneProject = b.Identifier
				}
			}
			g.Peers = append(g.Peers, p)
		}
		sort.Slice(g.Peers, func(i, j int) bool { return g.Peers[i].Alias < g.Peers[j].Alias })
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Synapse < groups[j].Synapse })
	return groups, nil
}
