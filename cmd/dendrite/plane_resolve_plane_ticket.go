package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"Dendrite/pkg/tracker/plane"
)

func resolvePlaneTicketUUID(ctx context.Context, client *plane.Client, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if isPlaneUUID(raw) {
		return raw, nil
	}
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", err
	}
	if iss := plane.ResolveIssue(issues, raw); iss != nil {
		return iss.ID, nil
	}
	// IDENT-N of another project of the workspace (e.g. a peer's ticket): look it up there.
	if m := otherProjectTicketRegex.FindStringSubmatch(strings.ToUpper(raw)); m != nil {
		if pc, err := plane.ClientForProject(ctx, client, m[1]); err == nil {
			if others, err := pc.ListPlaneIssues(ctx); err == nil {
				if iss := plane.ResolveIssue(others, m[2]); iss != nil {
					return iss.ID, nil
				}
			}
		}
	}
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}

// otherProjectTicketRegex matches a ticket reference with a Plane project identifier: OXI-12.
var otherProjectTicketRegex = regexp.MustCompile(`^([A-Z][A-Z0-9]{0,11})-(\d+)$`)
