package bootstrap

import (
	"path/filepath"

	"Dendrite/pkg/directive"
)

// ensureDirectives gives a project that has none the default quality directives and the files derived from
// them (codecov.yml, .jscpd.json), and reports what it created and what hand-written files it left alone. A
// project that has directives is not touched: changing them is `dendrite directive`.
func ensureDirectives(root string, dryRun bool) (created, notes []string, err error) {
	if directive.Exists(root) {
		return nil, nil, nil
	}
	created = append(created, filepath.FromSlash(directive.Path))
	if dryRun {
		return created, nil, nil
	}
	if _, err := directive.EnsureDefaults(root); err != nil {
		return nil, nil, err
	}
	res, err := directive.Render(root, directive.Defaults())
	created = append(created, res.Written...)
	for _, kept := range res.Kept {
		notes = append(notes, "kept the hand-written "+kept+": it does not carry the managed header, so Dendrite does not overwrite it")
	}
	return created, notes, err
}
