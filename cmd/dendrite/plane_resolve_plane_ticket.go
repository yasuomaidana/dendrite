package main

import (
	"context"
	"fmt"
	"strconv"
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
	if seq, err := strconv.Atoi(raw); err == nil {
		for _, iss := range issues {
			if iss.SequenceID == seq {
				return iss.ID, nil
			}
		}
	}
	lower := strings.ToLower(raw)
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), "["+lower+"]") {
			return iss.ID, nil
		}
	}
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), lower) {
			return iss.ID, nil
		}
	}
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}
