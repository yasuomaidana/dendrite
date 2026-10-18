// Package sidecar locates and talks to the dendrite-* helper binaries built by the
// sibling repositories (dendrite-core-rs, dendrite-git-rs, dendrite-rag-rs, ...).
//
// A sidecar speaks JSON-RPC 2.0 over stdio, one JSON message per line. On start the
// client calls "health", which must answer {"name","version","protocol"}; the protocol
// is a semver string whose major version must equal the one the caller was built for.
// Every call site keeps an in-process Go implementation: Call falls back to it whenever
// the binary is missing, incompatible or fails, so Dendrite works with or without the
// sidecars installed.
package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

// ErrNotFound is returned by Locate when no candidate binary exists.
var ErrNotFound = errors.New("sidecar: binary not found")

// ErrIncompatible is returned by Start when the sidecar's protocol major version differs.
var ErrIncompatible = errors.New("sidecar: incompatible protocol version")

// Locator resolves a sidecar name (e.g. "dendrite-core") to an executable path.
type Locator struct {
	// MemberDirs are local checkouts whose build outputs (target/release, bin) are searched,
	// typically the local members of the synapses Dendrite belongs to.
	MemberDirs []string
	// Home is $DENDRITE_HOME; <Home>/bin is searched.
	Home string
	// UserHome is the user's home directory; <UserHome>/.cargo/bin is searched last.
	UserHome string
	// LookPath searches PATH (exec.LookPath when nil).
	LookPath func(string) (string, error)
}

// EnvVar is the variable that overrides the location of sidecar name:
// "dendrite-core" -> DENDRITE_DENDRITE_CORE_BIN, "oxillama" -> DENDRITE_OXILLAMA_BIN.
func EnvVar(name string) string {
	return "DENDRITE_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name)) + "_BIN"
}

// Candidates lists, in resolution order, every path Locate checks for name
// (excluding PATH, which Locate searches between the Dendrite home and ~/.cargo/bin).
func (l Locator) Candidates(name string) []string {
	var out []string
	if v := os.Getenv(EnvVar(name)); v != "" {
		out = append(out, v)
	}
	for _, d := range l.MemberDirs {
		out = append(out, filepath.Join(d, "target", "release", name), filepath.Join(d, "bin", name))
	}
	if l.Home != "" {
		out = append(out, filepath.Join(l.Home, "bin", name))
	}
	return out
}

// Locate returns the first executable for name, searching $DENDRITE_<NAME>_BIN, the
// member checkouts' target/release and bin directories, $DENDRITE_HOME/bin, PATH and
// finally ~/.cargo/bin. An explicitly set environment variable that does not point at
// an executable is an error rather than silently skipped.
func (l Locator) Locate(name string) (string, error) {
	if v := os.Getenv(EnvVar(name)); v != "" && !isExecutable(v) {
		return "", fmt.Errorf("%s=%q is not an executable: %w", EnvVar(name), v, ErrNotFound)
	}
	for _, c := range l.Candidates(name) {
		if isExecutable(c) {
			return c, nil
		}
	}
	lookPath := l.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if p, err := lookPath(name); err == nil {
		return p, nil
	}
	if l.UserHome != "" {
		if c := filepath.Join(l.UserHome, ".cargo", "bin", name); isExecutable(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, ErrNotFound)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// Health is the sidecar's answer to the "health" handshake.
type Health struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

// RPCError is a JSON-RPC error object returned by a sidecar.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("sidecar error %d: %s", e.Code, e.Message) }

type request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

// Client is a running sidecar process.
type Client struct {
	Path   string
	Health Health

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID atomic.Int64
}

// Start launches the sidecar at path and performs the health handshake, requiring the
// sidecar's protocol major version to equal major. The process lives until Close.
func Start(ctx context.Context, path string, major int) (*Client, error) {
	cmd := exec.Command(path)
	cmd.Stderr = os.Stderr
	// The pipe constructors only fail when Stdin/Stdout are already set or the process
	// has started, neither of which holds for a fresh Cmd.
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting sidecar %s: %w", path, err)
	}
	c := &Client{Path: path, cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}

	hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Call(hctx, "health", nil, &c.Health); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("sidecar %s health handshake: %w", path, err)
	}
	if got := majorOf(c.Health.Protocol); got != major {
		_ = c.Close()
		return nil, fmt.Errorf("%s speaks protocol %q, need major %d: %w", path, c.Health.Protocol, major, ErrIncompatible)
	}
	return c, nil
}

// Call sends one request and decodes the result into result (which may be nil).
// Calls are serialised; a context deadline kills the process, since a reply can no
// longer be matched to its request.
func (c *Client) Call(ctx context.Context, method string, params, result interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID.Add(1)
	data, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	type reply struct {
		resp response
		err  error
	}
	done := make(chan reply, 1)
	go func() {
		if _, err := c.stdin.Write(append(data, '\n')); err != nil {
			done <- reply{err: fmt.Errorf("writing request: %w", err)}
			return
		}
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			done <- reply{err: fmt.Errorf("reading response: %w", err)}
			return
		}
		var r response
		if err := json.Unmarshal(line, &r); err != nil {
			done <- reply{err: fmt.Errorf("decoding response %q: %w", strings.TrimSpace(string(line)), err)}
			return
		}
		done <- reply{resp: r}
	}()

	select {
	case <-ctx.Done():
		_ = c.cmd.Process.Kill()
		return ctx.Err()
	case r := <-done:
		if r.err != nil {
			return r.err
		}
		if r.resp.ID != id {
			return fmt.Errorf("response id %d does not match request id %d", r.resp.ID, id)
		}
		if r.resp.Error != nil {
			return r.resp.Error
		}
		if result == nil || len(r.resp.Result) == 0 {
			return nil
		}
		return json.Unmarshal(r.resp.Result, result)
	}
}

// closeGrace is how long Close waits for a sidecar to exit after its stdin closes.
var closeGrace = 5 * time.Second

// Close ends the sidecar by closing its stdin and waits for it to exit, killing it
// after closeGrace.
func (c *Client) Close() error {
	_ = c.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(closeGrace):
		_ = c.cmd.Process.Kill()
		return <-done
	}
}

func majorOf(semver string) int {
	head, _, _ := strings.Cut(strings.TrimPrefix(semver, "v"), ".")
	n, err := strconv.Atoi(head)
	if err != nil {
		return -1
	}
	return n
}

// Origin records which implementation served a Call.
type Origin string

const (
	OriginSidecar  Origin = "sidecar"
	OriginFallback Origin = "fallback"
)

// Outcome describes how a Call was served; Reason explains a fallback.
type Outcome struct {
	Origin Origin `json:"origin"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Spec names a sidecar method and the protocol major version the caller speaks.
type Spec struct {
	Binary string
	Major  int
	Method string
}

// Call runs spec.Method on the sidecar located by loc and decodes its result, or runs
// fallback when the sidecar is missing, incompatible or fails. The returned Outcome
// says which one answered. Each call starts and stops the sidecar process.
func Call[T any](ctx context.Context, loc Locator, spec Spec, params interface{}, fallback func() (T, error)) (T, Outcome, error) {
	path, err := loc.Locate(spec.Binary)
	if err != nil {
		v, ferr := fallback()
		return v, Outcome{Origin: OriginFallback, Reason: err.Error()}, ferr
	}
	c, err := Start(ctx, path, spec.Major)
	if err != nil {
		v, ferr := fallback()
		return v, Outcome{Origin: OriginFallback, Path: path, Reason: err.Error()}, ferr
	}
	defer c.Close()
	var out T
	if err := c.Call(ctx, spec.Method, params, &out); err != nil {
		v, ferr := fallback()
		return v, Outcome{Origin: OriginFallback, Path: path, Reason: err.Error()}, ferr
	}
	return out, Outcome{Origin: OriginSidecar, Path: path}, nil
}

// DefaultLocator searches $DENDRITE_HOME, the user's home and the local checkouts of
// every member of every synapse in the default store, so a sidecar built in a sibling
// repository (e.g. ../dendrite-core-rs/target/release) is found without installing it.
// Members switched to remote source and unresolved members are skipped.
func DefaultLocator() Locator {
	loc := Locator{}
	loc.Home, _ = projectid.HomeDir()
	loc.UserHome, _ = os.UserHomeDir()
	st, err := synapse.DefaultStore()
	if err != nil {
		return loc
	}
	syns, _ := st.List()
	seen := map[string]bool{}
	for _, s := range syns {
		for _, m := range s.Members {
			if m.IsRemote() {
				continue
			}
			if dir, err := synapse.Resolve(m); err == nil && !seen[dir] {
				seen[dir] = true
				loc.MemberDirs = append(loc.MemberDirs, dir)
			}
		}
	}
	return loc
}
