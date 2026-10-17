package migrate

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultRemoteExecutor executes remote commands over SSH and probes HTTP endpoints.
type DefaultRemoteExecutor struct {
	HTTPClient *http.Client
}

// NewDefaultRemoteExecutor instantiates a DefaultRemoteExecutor with standard timeouts.
func NewDefaultRemoteExecutor() *DefaultRemoteExecutor {
	return &DefaultRemoteExecutor{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Execute runs a command on the remote host via ssh binary with zero credential persistence.
func (e *DefaultRemoteExecutor) Execute(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error) {
	if port <= 0 {
		port = 22
	}
	destination := host
	if user != "" && !strings.Contains(host, "@") {
		destination = fmt.Sprintf("%s@%s", user, host)
	}

	args := []string{
		"-p", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=5",
	}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, destination, cmd)

	sshCmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := sshCmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("ssh execution failed on %s: %w (%s)", destination, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// StreamFile copies a local file to the remote host using scp or piped ssh.
func (e *DefaultRemoteExecutor) StreamFile(ctx context.Context, host string, port int, user string, keyPath string, localPath string, remotePath string) error {
	if port <= 0 {
		port = 22
	}
	destination := host
	if user != "" && !strings.Contains(host, "@") {
		destination = fmt.Sprintf("%s@%s", user, host)
	}

	args := []string{
		"-P", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, localPath, fmt.Sprintf("%s:%s", destination, remotePath))

	scpCmd := exec.CommandContext(ctx, "scp", args...)
	out, err := scpCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp stream failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ProbeHealth issues an HTTP GET probe against the remote service endpoint.
func (e *DefaultRemoteExecutor) ProbeHealth(ctx context.Context, url string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	client := e.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 400 {
					return true, nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return false, fmt.Errorf("%w: endpoint %s unreachable after %v", ErrRemoteHealthCheckFailed, url, timeout)
}

