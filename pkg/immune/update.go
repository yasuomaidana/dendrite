package immune

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Package is the name SkillSpector is published under; UVEnv overrides where the uv executable
// that installs it is found.
const (
	Package = "skillspector"
	UVEnv   = "DENDRITE_UV_BIN"
)

// ErrPinMismatch means the installed scanner is not the version the project pins.
var ErrPinMismatch = errors.New("the installed scanner is not the pinned version: run `dendrite immune update`")

// ErrUVMissing means no uv executable was found to install the scanner with.
var ErrUVMissing = errors.New("uv not found: install it (https://docs.astral.sh/uv/) or set " + UVEnv)

// Version reduces what the scanner prints for --version ("SkillSpector v2.12.0") to the version
// itself ("2.12.0"), the form a pin is written in.
func Version(output string) string {
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[len(fields)-1], "v")
}

// checkPin compares what the scanner says about itself with the pinned version; no pin accepts any.
func checkPin(pin, versionOutput string) error {
	if pin == "" || Version(versionOutput) == pin {
		return nil
	}
	return fmt.Errorf("%w (installed %q, pinned %q)", ErrPinMismatch, versionOutput, pin)
}

// Update is what `immune update` did: the scanner version before and after, and the pin it aimed for.
type Update struct {
	Before  string `json:"before"`
	After   string `json:"after"`
	Pin     string `json:"pin"`
	Changed bool   `json:"changed"`
}

// Refresh brings the installed scanner to the pinned version, or to the latest when pin is empty.
// A scanner that is already at the pin is left alone, so running it again changes nothing.
func Refresh(ctx context.Context, pin string) (Update, error) {
	u := Update{Pin: pin}
	before, err := installedVersion(ctx)
	if err != nil {
		return u, err
	}
	u.Before, u.After = before, before
	if pin != "" && before == pin {
		return u, nil
	}
	uv, err := locateUV()
	if err != nil {
		return u, err
	}
	if out, err := exec.CommandContext(ctx, uv, installArgs(pin, before)...).CombinedOutput(); err != nil {
		return u, fmt.Errorf("uv %s: %w: %s", strings.Join(installArgs(pin, before), " "), err, strings.TrimSpace(string(out)))
	}
	if u.After, err = installedVersion(ctx); err != nil {
		return u, err
	}
	u.Changed = u.After != u.Before
	if pin != "" && u.After != pin {
		return u, fmt.Errorf("%w (installed %q, pinned %q)", ErrPinMismatch, u.After, pin)
	}
	return u, nil
}

// installArgs are the uv arguments that install the pin, or upgrade to (or first install) the latest.
func installArgs(pin, installed string) []string {
	switch {
	case pin != "":
		return []string{"tool", "install", "--force", Package + "==" + pin}
	case installed == "":
		return []string{"tool", "install", Package}
	}
	return []string{"tool", "upgrade", Package}
}

// installedVersion is the version of the scanner Locate finds; "" when none is installed.
func installedVersion(ctx context.Context) (string, error) {
	bin, err := Locate()
	if errors.Is(err, ErrScannerMissing) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return Version(Scanner{}.version(ctx, bin)), nil
}

// locateUV returns the uv executable: $DENDRITE_UV_BIN (which must work if set), then PATH.
func locateUV() (string, error) {
	if p := os.Getenv(UVEnv); p != "" {
		if err := executable(p); err != nil {
			return "", fmt.Errorf("%s=%s: %w", UVEnv, p, err)
		}
		return p, nil
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p, nil
	}
	return "", ErrUVMissing
}
