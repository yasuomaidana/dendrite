// Package ticketparent sets and clears the Plane parent of a ticket. The CLI (`dendrite branch parent`) and the
// MCP tool dendrite_ticket_parent both go through it, so they apply the same checks.
package ticketparent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// ErrLoop means the new parent would make a ticket its own ancestor.
var ErrLoop = errors.New("the parent would make a loop")

// maxDepth bounds the walk up the parents when looking for a loop.
const maxDepth = 8

func normal(ref string) string { return strings.ToUpper(strings.TrimSpace(ref)) }

// Set makes parent the Plane parent of child. A parent that is the child itself or one of its descendants
// would make a loop, so it is refused.
func Set(ctx context.Context, b ticketlock.Board, child, parent string) error {
	child, parent = normal(child), normal(parent)
	if child == parent {
		return fmt.Errorf("%w: a ticket cannot be its own parent", ErrLoop)
	}
	for at, depth := parent, 0; depth < maxDepth; depth++ {
		up, ok, err := b.Parent(ctx, at)
		if err != nil || !ok {
			break
		}
		if up.ID == child {
			return fmt.Errorf("%w: %s is below %s already", ErrLoop, parent, child)
		}
		at = up.ID
	}
	p, err := b.Svc.GetIssue(ctx, b.Client, parent)
	if err != nil {
		return fmt.Errorf("parent %s: %w", parent, err)
	}
	if _, err := b.Svc.UpdateIssue(ctx, b.Client, child, plane.UpdateIssueRequest{Parent: &p.ID}); err != nil {
		return fmt.Errorf("set the parent of %s: %w", child, err)
	}
	return nil
}

// Clear removes the Plane parent of child and returns the one it had; had is false when there was none, and
// nothing is sent. A sidecar too old to clear a parent is refused before Plane is touched.
func Clear(ctx context.Context, b ticketlock.Board, child string) (featurebranch.Ticket, bool, error) {
	return clearWith(ctx, b, child, b.Svc.CanClearParent)
}

// clearWith is Clear with the sidecar version check passed in.
func clearWith(ctx context.Context, b ticketlock.Board, child string, canClear func(context.Context) error) (old featurebranch.Ticket, had bool, err error) {
	child = normal(child)
	old, had, err = b.Parent(ctx, child)
	if err != nil {
		return old, false, fmt.Errorf("read the parent of %s: %w", child, err)
	}
	if !had {
		return old, false, nil
	}
	if err := canClear(ctx); err != nil {
		return old, true, err
	}
	none := ""
	if _, err := b.Svc.UpdateIssue(ctx, b.Client, child, plane.UpdateIssueRequest{Parent: &none}); err != nil {
		return old, true, fmt.Errorf("clear the parent of %s: %w", child, err)
	}
	return old, true, nil
}
