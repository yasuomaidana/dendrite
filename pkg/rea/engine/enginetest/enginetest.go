// Package enginetest gives the engine tests stand-ins for docker, rizin, radare2 and objdump without any of them
// installed: one small program (standin) copied under each name.
package enginetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	once sync.Once
	dir  string
	err  error
)

// Install builds the stand-in once and returns the directory holding it as docker, rizin, radare2, r2, objdump and
// llvm-objdump.
func Install(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		if dir, err = os.MkdirTemp("", "enginetest"); err != nil {
			return
		}
		src := filepath.Join(dir, "standin")
		if out, buildErr := exec.Command("go", "build", "-o", src, "Dendrite/pkg/rea/engine/enginetest/standin").CombinedOutput(); buildErr != nil {
			err = &buildError{buildErr, out}
			return
		}
		data, _ := os.ReadFile(src)
		for _, name := range []string{"docker", "rizin", "radare2", "r2", "objdump", "llvm-objdump"} {
			if err = os.WriteFile(filepath.Join(dir, name), data, 0o755); err != nil {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type buildError struct {
	err error
	out []byte
}

func (b *buildError) Error() string {
	return "building the engine stand-in: " + b.err.Error() + "\n" + string(b.out)
}

// UsePath puts the stand-ins first on PATH for the test and names the images `docker image inspect` finds.
func UsePath(t testing.TB, images ...string) string {
	t.Helper()
	bin := Install(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STANDIN_IMAGES", strings.Join(images, ","))
	return bin
}
