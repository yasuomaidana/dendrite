package ownership

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/tracker/plane"
)

// LoadRule reads the directives that apply to the working copy in dir: those of its repository, or of dir
// itself when it is not inside one. A project with no directives file has the defaults.
func LoadRule(dir string) (directive.Set, error) {
	root := dir
	if top, err := plane.FindRepoRoot(dir); err == nil {
		root = top
	}
	return directive.Load(root)
}
