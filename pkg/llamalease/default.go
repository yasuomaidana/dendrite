//go:build !windows

package llamalease

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"Dendrite/pkg/memfit"
	"Dendrite/pkg/projectid"
)

// Model is one GGUF installed under ModelsDir().
type Model struct {
	ID        string // the id the server serves it under
	Path      string
	SizeBytes int64
}

// ModelsDir is where managed GGUF models live: DENDRITE_GGUF_MODELS_DIR, else $DENDRITE_HOME/models.
func ModelsDir() string {
	if d := os.Getenv("DENDRITE_GGUF_MODELS_DIR"); d != "" {
		return d
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "models")
}

// Installed lists the installed models, largest first. A model's id is the first line of an `id`
// file next to it when present, otherwise its directory name.
func Installed() []Model {
	dir := ModelsDir()
	if dir == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.gguf"))
	var out []Model
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		id := filepath.Base(filepath.Dir(f))
		if raw, err := os.ReadFile(filepath.Join(filepath.Dir(f), "id")); err == nil {
			if line := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0]); line != "" {
				id = line
			}
		}
		out = append(out, Model{ID: id, Path: f, SizeBytes: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SizeBytes > out[j].SizeBytes })
	return out
}

// BestFitting is the largest installed model that may start now. A larger model is the smarter
// one; slower is acceptable, starving the machine is not.
func BestFitting(ctx context.Context) (Model, error) {
	models := Installed()
	if len(models) == 0 {
		return Model{}, fmt.Errorf("no GGUF model installed under %s", ModelsDir())
	}
	var last error
	for _, m := range models {
		if err := memfit.CheckStart(ctx, "llamacpp", m.SizeBytes); err != nil {
			last = err
			continue
		}
		return m, nil
	}
	return Model{}, last
}

// BinaryPath finds the llama-server executable: DENDRITE_LLAMA_SERVER_BIN, then PATH.
func BinaryPath() (string, error) {
	if p := os.Getenv("DENDRITE_LLAMA_SERVER_BIN"); p != "" {
		return p, nil
	}
	return exec.LookPath("llama-server")
}

var (
	defaultOnce sync.Once
	defaultMgr  *Manager
)

// Default is the process-wide manager for the real server on the default port.
func Default() *Manager {
	defaultOnce.Do(func() {
		var m *Manager
		m = New(Options{
			Fits: func(r Request) error {
				info, err := os.Stat(r.ModelPath)
				if err != nil {
					return err
				}
				return memfit.CheckStart(context.Background(), "llamacpp", info.Size())
			},
			ChildCmd: func(r Request) (*exec.Cmd, error) { return serveCmd(r, m.o.Port, m.o.Dir) },
			Spawn:    func(r Request) (int, error) { return spawnSupervisor(r, m.o.Port, m.o.Dir) },
		})
		defaultMgr = m
	})
	return defaultMgr
}

// NewManager is a manager for a custom port and directory with the real start commands.
func NewManager(port int, dir string) *Manager {
	var m *Manager
	m = New(Options{
		Port: port,
		Dir:  dir,
		Fits: func(r Request) error {
			info, err := os.Stat(r.ModelPath)
			if err != nil {
				return err
			}
			return memfit.CheckStart(context.Background(), "llamacpp", info.Size())
		},
		ChildCmd: func(r Request) (*exec.Cmd, error) { return serveCmd(r, m.o.Port, m.o.Dir) },
		Spawn:    func(r Request) (int, error) { return spawnSupervisor(r, m.o.Port, m.o.Dir) },
	})
	return m
}

func serveCmd(r Request, port int, dir string) (*exec.Cmd, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("llama-server binary not found (set DENDRITE_LLAMA_SERVER_BIN or install llama.cpp): %w", err)
	}
	cmd := exec.Command(bin, "-m", r.ModelPath, "--alias", r.ModelID,
		"--host", "127.0.0.1", "--port", strconv.Itoa(port), "-c", "8192",
		"-t", strconv.Itoa(min(runtime.NumCPU(), 8)), "--timeout", "1800")
	logf, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	return cmd, nil
}

// spawnSupervisor starts `dendrite spike supervise` detached from the caller, so it outlives the
// session that started it and can stop the server after the last session ends.
func spawnSupervisor(r Request, port int, dir string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if strings.HasSuffix(self, ".test") { // never re-exec a test binary as a supervisor
		return 0, errors.New("refusing to start a supervisor from a test binary")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	cmd := exec.Command(self, "spike", "supervise", "--model", r.ModelPath, "--model-id", r.ModelID,
		"--port", strconv.Itoa(port), "--dir", dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	logf, err := os.OpenFile(filepath.Join(dir, "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}
