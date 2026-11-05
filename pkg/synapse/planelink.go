package synapse

import (
	"encoding/json"
	"os"
	"path/filepath"

	"Dendrite/pkg/projectid"
)

// readPlaneLink reads the Plane binding a repository keeps in .dendrite/plane.json (written by
// `dendrite chronos project ensure`). It is nil when the file is missing, unreadable or names no identifier.
func readPlaneLink(dir string) *projectid.PlaneBinding {
	data, err := os.ReadFile(filepath.Join(dir, ".dendrite", "plane.json"))
	if err != nil {
		return nil
	}
	var b projectid.PlaneBinding
	if json.Unmarshal(data, &b) != nil || b.Identifier == "" {
		return nil
	}
	return &b
}

// BackfillPlane records, for every indexed project that resolves and has no Plane binding, the binding its
// own plane.json holds. It saves the index when it changed anything and returns how many entries it filled.
func BackfillPlane(ix *projectid.Index) (int, error) {
	filled := 0
	for _, e := range ix.List() {
		if e.Plane != nil || !entryResolves(e) {
			continue
		}
		if b := readPlaneLink(e.Path); b != nil && ix.SetPlane(e.ID, b) {
			filled++
		}
	}
	if filled == 0 {
		return 0, nil
	}
	return filled, ix.Save()
}
