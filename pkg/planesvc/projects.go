package planesvc

import (
	"context"

	"Dendrite/pkg/tracker/plane"
)

// ArchiveProject archives the Plane project with this UUID. Plane keeps it and its issues, and the
// project can be restored from the Plane UI.
func (s *Service) ArchiveProject(ctx context.Context, c *plane.Client, projectID string) error {
	return s.call(ctx, false, "project.archive", confirmed(c, projectID), nil)
}

// DeleteProject permanently deletes the Plane project with this UUID and every issue in it.
func (s *Service) DeleteProject(ctx context.Context, c *plane.Client, projectID string) error {
	return s.call(ctx, false, "project.delete", confirmed(c, projectID), nil)
}

// confirmed is the sidecar target for a project-ending call. The sidecar refuses these methods
// without an explicit project_id and confirm: the caller has already asked the user (the typed
// project name for a purge), so the service states both.
func confirmed(c *plane.Client, projectID string) map[string]any {
	params := target(c.ForProject(projectID))
	params["project_id"] = projectID
	params["confirm"] = true
	return params
}
