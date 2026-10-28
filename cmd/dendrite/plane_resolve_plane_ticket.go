package main

import (
	"context"
	"fmt"
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
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}
