package bootstrap

import (
	"path/filepath"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/env/pipeline"
)

// directiveSet is the project's quality directives, or the defaults while it has none.
func directiveSet(root string) (directive.Set, error) {
	if !directive.Exists(root) {
		return directive.Defaults(), nil
	}
	return directive.Load(root)
}

// ensureDirectives gives a project that has none the default quality directives, and keeps the files derived
// from the directives (codecov.yml, .jscpd.json, the lint configurations) current. It reports what it created,
// what it rewrote and what hand-written files it left alone. Changing the limits is `dendrite directive`.
func ensureDirectives(root string, dryRun bool) (created, updated, notes []string, err error) {
	set, err := directiveSet(root)
	if err != nil {
		return nil, nil, nil, err
	}
	if !directive.Exists(root) {
		created = append(created, filepath.FromSlash(directive.Path))
		if !dryRun {
			if _, err := directive.EnsureDefaults(root); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	render := directive.Render
	if dryRun {
		render = directive.Plan
	}
	res, err := render(root, set)
	for _, p := range res.Written {
		if contains(res.Created, p) {
			created = append(created, filepath.FromSlash(p))
		} else {
			updated = append(updated, filepath.FromSlash(p))
		}
	}
	for _, kept := range res.Kept {
		if kept != pipeline.WorkflowPath { // the workflow's own note says how to adopt it
			notes = append(notes, "kept the hand-written "+kept+": it does not carry the managed header, so Dendrite does not overwrite it")
		}
	}
	return created, updated, notes, err
}
