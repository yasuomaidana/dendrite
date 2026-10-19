package github

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// remoteURLRe matches GitHub remote URLs in https, ssh:// and scp-like (git@host:owner/repo) forms.
var remoteURLRe = regexp.MustCompile(`github\.com[:/]+([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// ParseRepoURL extracts "owner/repo" from a GitHub remote URL.
func ParseRepoURL(url string) (string, bool) {
	m := remoteURLRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// DetectRepo returns the "owner/repo" of the origin remote of the git repository at dir.
func DetectRepo(ctx context.Context, exec CommandExecutor, dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	out, err := exec.Execute(ctx, "git", "-C", dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("github: cannot read the origin remote (pass --repo owner/repo): %w", err)
	}
	repo, ok := ParseRepoURL(out)
	if !ok {
		return "", fmt.Errorf("github: origin remote %q is not a GitHub repository (pass --repo owner/repo)", strings.TrimSpace(out))
	}
	return repo, nil
}
