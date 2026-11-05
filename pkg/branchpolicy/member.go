package branchpolicy

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"

	"Dendrite/pkg/projectid"
)

// synapsesDir is where Dendrite keeps one manifest per synapse, under the Dendrite home. pkg/synapse owns the
// format; this package reads the few member fields it needs, because pkg/synapse imports the packages that
// call For and a call back would be an import cycle.
const synapsesDir = "synapses"

// manifest is the part of a synapse manifest that can override a project's branches.
type manifest struct {
	State   string `toml:"state"`
	Members []struct {
		ProjectID    string `toml:"project_id"`
		Branch       string `toml:"branch"`
		PushRemote   string `toml:"push_remote"`
		TargetRemote string `toml:"target_remote"`
	} `toml:"members"`
}

// MemberOverride is what the synapses that contain the project at dir declare about its integration branch and
// remotes: the first non-archived synapse (by file name) whose member sets any of them wins. A directory that is
// not a Dendrite project, a project in no synapse, or unreadable manifests override nothing.
func MemberOverride(dir string) Override {
	id, err := projectid.Read(projectid.StateRoot(dir))
	if err != nil {
		return Override{}
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return Override{}
	}
	files, _ := filepath.Glob(filepath.Join(home, synapsesDir, "*.toml"))
	sort.Strings(files)
	for _, f := range files {
		if ov := memberIn(f, id); ov != (Override{}) {
			return ov
		}
	}
	return Override{}
}

// memberIn is the override the manifest at path declares for the project id.
func memberIn(path, id string) Override {
	data, err := os.ReadFile(path)
	if err != nil {
		return Override{}
	}
	var m manifest
	if toml.Unmarshal(data, &m) != nil || m.State == "archived" {
		return Override{}
	}
	for _, mem := range m.Members {
		if mem.ProjectID == id {
			return Override{Branch: mem.Branch, PushRemote: mem.PushRemote, TargetRemote: mem.TargetRemote}
		}
	}
	return Override{}
}
