package engram

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// saveTimeout bounds one `engram save`; a stuck memory must never hold a session end back.
const saveTimeout = 10 * time.Second

// Save stores one memory through `engram save`. Engram dedupes an identical title and body in a project,
// so saving the same memory again is harmless. typ (decision, plan, note, outcome) and project may be empty.
func Save(ctx context.Context, bin, project, typ, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, saveTimeout)
	defer cancel()
	args := []string{"save", title, body}
	if typ != "" {
		args = append(args, "--type", typ)
	}
	if project != "" {
		args = append(args, "--project", project)
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("engram save: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
