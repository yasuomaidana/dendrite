package planesvc

import (
	"context"

	"Dendrite/pkg/tracker/plane"
)

// ArchiveProject archives the Plane project with this UUID. Plane keeps it and its issues, and the
// project can be restored from the Plane UI.
func (s *Service) ArchiveProject(ctx context.Context, c *plane.Client, projectID string) error {
	return s.call(ctx, false, "project.archive", target(c.ForProject(projectID)), nil)
}

// DeleteProject permanently deletes the Plane project with this UUID and every issue in it.
func (s *Service) DeleteProject(ctx context.Context, c *plane.Client, projectID string) error {
	return s.call(ctx, false, "project.delete", target(c.ForProject(projectID)), nil)
}
