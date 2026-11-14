package tracker

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// SpecRegex matches the feature and decision ids (FEAT-n, ADR-n) a ticket links to.
var SpecRegex = regexp.MustCompile(`\b(FEAT-\d+|ADR-\d+)\b`)

// LinkedSpecs returns the distinct FEAT-n / ADR-n ids in content, in first-seen order.
func LinkedSpecs(content string) []string {
	matches := SpecRegex.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// CredentialsTimeout is how long a credential check may take.
const CredentialsTimeout = 1500 * time.Millisecond

// CheckCredentials sends the request build makes and maps the answer: errTimeout when the deadline
// passed, errUnauthorized for 401 and 403, a "credential validation failed" error for other 4xx
// and 5xx statuses, nil otherwise.
func CheckCredentials(ctx context.Context, hc *http.Client, build func(context.Context) (*http.Request, error), errTimeout, errUnauthorized error) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, CredentialsTimeout)
	defer cancel()

	req, err := build(timeoutCtx)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return errTimeout
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errUnauthorized
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("credential validation failed: status %d", resp.StatusCode)
	}
	return nil
}
