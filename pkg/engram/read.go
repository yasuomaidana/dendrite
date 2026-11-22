package engram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// readTimeout bounds one read of Engram; a check must never hang on a stuck store.
const readTimeout = 30 * time.Second

// Observation is one stored memory, as `engram export` writes it.
type Observation struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// Created is when the observation was stored; the zero time when Engram's stamp cannot be read.
func (o Observation) Created() time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", o.CreatedAt)
	return t
}

// Export returns every memory of the project that dir belongs to (Engram resolves the project from the
// directory), through `engram export` into a temporary file.
func Export(ctx context.Context, bin, dir string) ([]Observation, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	file := filepath.Join(os.TempDir(), fmt.Sprintf("engram-export-%d-%d.json", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(file)
	cmd := exec.CommandContext(ctx, bin, "export", file)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("engram export: %w: %s", err, out)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("engram export: %w", err)
	}
	var dump struct {
		Observations []Observation `json:"observations"`
	}
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("engram export: unreadable file: %w", err)
	}
	return dump.Observations, nil
}

// hitLine is a result line of `engram search`: "[1] #7 (feedback) — title".
var hitLine = regexp.MustCompile(`(?m)^\[\d+\] #\d+ \([^)]*\) — (.+)$`)

// Search returns the titles of the top limit memories matching any word of query in the project of dir.
func Search(ctx context.Context, bin, dir, query string, limit int) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "search", query, "--match", "any", "--limit", strconv.Itoa(limit))
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("engram search: %w", err)
	}
	var titles []string
	for _, m := range hitLine.FindAllStringSubmatch(string(out), -1) {
		titles = append(titles, m[1])
	}
	return titles, nil
}
