package sidecar

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

// testBinary is the real testsidecar executable built once for the package tests.
var testBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sidecar-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testBinary = filepath.Join(dir, "testsidecar")
	if out, err := exec.Command("go", "build", "-o", testBinary, "./testdata/testsidecar").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building testsidecar: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func install(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func noPath(string) (string, error) { return "", exec.ErrNotFound }

func TestEnvVar(t *testing.T) {
	for in, want := range map[string]string{
		"dendrite-core": "DENDRITE_DENDRITE_CORE_BIN",
		"oxillama":      "DENDRITE_OXILLAMA_BIN",
		"a.b":           "DENDRITE_A_B_BIN",
	} {
		if got := EnvVar(in); got != want {
			t.Errorf("EnvVar(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocateResolutionOrder(t *testing.T) {
	root := t.TempDir()
	member := filepath.Join(root, "dendrite-core-rs")
	home := filepath.Join(root, "dhome")
	user := filepath.Join(root, "user")
	name := "dendrite-core"
	t.Setenv(EnvVar(name), "")

	loc := Locator{MemberDirs: []string{member}, Home: home, UserHome: user, LookPath: noPath}
	if _, err := loc.Locate(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty layout err = %v", err)
	}

	cargo := install(t, filepath.Join(user, ".cargo", "bin"), name)
	if got, _ := loc.Locate(name); got != cargo {
		t.Fatalf("cargo bin = %q", got)
	}
	onPath := filepath.Join(root, "path-bin", name)
	loc.LookPath = func(string) (string, error) { return onPath, nil }
	if got, _ := loc.Locate(name); got != onPath {
		t.Fatalf("PATH = %q", got)
	}
	homeBin := install(t, filepath.Join(home, "bin"), name)
	if got, _ := loc.Locate(name); got != homeBin {
		t.Fatalf("home bin = %q", got)
	}
	memberBin := install(t, filepath.Join(member, "bin"), name)
	if got, _ := loc.Locate(name); got != memberBin {
		t.Fatalf("member bin = %q", got)
	}
	release := install(t, filepath.Join(member, "target", "release"), name)
	if got, _ := loc.Locate(name); got != release {
		t.Fatalf("target/release = %q", got)
	}
	override := install(t, filepath.Join(root, "custom"), "anything")
	t.Setenv(EnvVar(name), override)
	if got, _ := loc.Locate(name); got != override {
		t.Fatalf("env override = %q", got)
	}
	if c := loc.Candidates(name); c[0] != override || len(c) != 4 {
		t.Fatalf("candidates = %v", c)
	}

	// A non-executable or missing override is an error, not skipped.
	plain := filepath.Join(root, "plain")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar(name), plain)
	if _, err := loc.Locate(name); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), EnvVar(name)) {
		t.Fatalf("bad override err = %v", err)
	}
	// Directories are not executables.
	if isExecutable(root) {
		t.Fatal("directory reported executable")
	}
	// The default LookPath is exec.LookPath.
	t.Setenv(EnvVar("sh"), "")
	if got, err := (Locator{}).Locate("sh"); err != nil || got == "" {
		t.Fatalf("Locate(sh) = %q, %v", got, err)
	}
}

func TestClientHandshakeAndCalls(t *testing.T) {
	ctx := context.Background()
	c, err := Start(ctx, testBinary, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Health.Name != "testsidecar" || c.Health.Protocol != "1.0.0" {
		t.Fatalf("health = %+v", c.Health)
	}
	var echoed map[string]int
	if err := c.Call(ctx, "echo", map[string]int{"n": 7}, &echoed); err != nil || echoed["n"] != 7 {
		t.Fatalf("echo = %v, %v", echoed, err)
	}
	if err := c.Call(ctx, "noresult", nil, nil); err != nil {
		t.Fatalf("noresult: %v", err)
	}
	var rpcErr *RPCError
	if err := c.Call(ctx, "nope", nil, nil); !errors.As(err, &rpcErr) || rpcErr.Code != -32601 || !strings.Contains(err.Error(), "-32601") {
		t.Fatalf("unknown method err = %v", err)
	}
	if err := c.Call(ctx, "echo", func() {}, nil); err == nil {
		t.Fatal("expected marshal error for unencodable params")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestStartFailures(t *testing.T) {
	ctx := context.Background()
	if _, err := Start(ctx, filepath.Join(t.TempDir(), "missing"), 1); err == nil {
		t.Fatal("expected start error for missing binary")
	}
	if _, err := Start(ctx, testBinary, 2); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("major mismatch err = %v", err)
	}
	t.Setenv("TESTSIDECAR_PROTOCOL", "garbage")
	if _, err := Start(ctx, testBinary, 1); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("unparsable protocol err = %v", err)
	}
	// A binary that is not a sidecar fails the handshake.
	if _, err := Start(ctx, "/usr/bin/true", 1); err == nil || !strings.Contains(err.Error(), "handshake") {
		t.Fatalf("non-sidecar err = %v", err)
	}
}

func TestClientMisbehavingSidecar(t *testing.T) {
	ctx := context.Background()
	for mode, want := range map[string]string{"badjson": "decoding response", "wrongid": "does not match", "exit": "reading response"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TESTSIDECAR_MODE", mode)
			c, err := Start(ctx, testBinary, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if err := c.Call(ctx, "echo", 1, nil); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
	t.Run("hang", func(t *testing.T) {
		t.Setenv("TESTSIDECAR_MODE", "hang")
		c, err := Start(ctx, testBinary, 1)
		if err != nil {
			t.Fatal(err)
		}
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if err := c.Call(cctx, "echo", 1, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
		_ = c.Close()
		// Writing to a dead sidecar fails.
		if err := c.Call(ctx, "echo", 1, nil); err == nil {
			t.Fatal("expected error calling a killed sidecar")
		}
	})
}

func TestCallFallsBackOrUsesSidecar(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EnvVar("testsidecar"), "")
	root := t.TempDir()
	fallback := func() (map[string]int, error) { return map[string]int{"n": -1}, nil }
	spec := Spec{Binary: "testsidecar", Major: 1, Method: "echo"}

	// Missing binary.
	loc := Locator{Home: root, LookPath: noPath}
	v, out, err := Call(ctx, loc, spec, map[string]int{"n": 5}, fallback)
	if err != nil || v["n"] != -1 || out.Origin != OriginFallback || out.Reason == "" {
		t.Fatalf("missing = %v %+v %v", v, out, err)
	}

	// Installed and compatible.
	path := install(t, filepath.Join(root, "bin"), "testsidecar")
	v, out, err = Call(ctx, loc, spec, map[string]int{"n": 5}, fallback)
	if err != nil || v["n"] != 5 || out.Origin != OriginSidecar || out.Path != path {
		t.Fatalf("sidecar = %v %+v %v", v, out, err)
	}

	// Incompatible protocol.
	v, out, _ = Call(ctx, loc, Spec{Binary: "testsidecar", Major: 9, Method: "echo"}, nil, fallback)
	if out.Origin != OriginFallback || !strings.Contains(out.Reason, "incompatible") || v["n"] != -1 {
		t.Fatalf("incompatible = %v %+v", v, out)
	}

	// Method failure.
	_, out, _ = Call(ctx, loc, Spec{Binary: "testsidecar", Major: 1, Method: "nope"}, nil, fallback)
	if out.Origin != OriginFallback || out.Path != path {
		t.Fatalf("method failure = %+v", out)
	}
}

func TestMajorOf(t *testing.T) {
	for in, want := range map[string]int{"1.2.3": 1, "v2.0.0": 2, "3": 3, "": -1, "x.1": -1} {
		if got := majorOf(in); got != want {
			t.Errorf("majorOf(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestCloseKillsLingeringSidecar(t *testing.T) {
	t.Setenv("TESTSIDECAR_MODE", "linger")
	orig := closeGrace
	closeGrace = 100 * time.Millisecond
	defer func() { closeGrace = orig }()
	c, err := Start(context.Background(), testBinary, 1)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := c.Close(); err == nil {
		t.Fatal("expected a killed-process error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("Close did not kill the lingering sidecar")
	}
}

func TestDefaultLocatorUsesLocalSynapseMembers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	root := t.TempDir()
	local, remote, gone := filepath.Join(root, "core"), filepath.Join(root, "rag"), filepath.Join(root, "gone")
	ids := map[string]string{}
	for _, d := range []string{local, remote, gone} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		id, err := projectid.Ensure(d)
		if err != nil {
			t.Fatal(err)
		}
		ids[d] = id
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []*synapse.Synapse{
		{ID: "a", State: synapse.StateActive, Members: []synapse.Member{
			{ProjectID: ids[local], Alias: "core", Role: synapse.RoleProducer, PathHint: local},
			{ProjectID: ids[remote], Alias: "rag", Role: synapse.RoleProducer, PathHint: remote, Source: synapse.SourceRemote, Remote: "u"},
		}},
		{ID: "b", State: synapse.StateActive, Members: []synapse.Member{
			{ProjectID: ids[local], Alias: "core", Role: synapse.RolePeer, PathHint: local},
			{ProjectID: ids[gone], Alias: "gone", Role: synapse.RolePeer, PathHint: gone},
		}},
	} {
		if err := st.Save(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	loc := DefaultLocator()
	if loc.Home != home || len(loc.MemberDirs) != 1 || loc.MemberDirs[0] != local {
		t.Fatalf("locator = %+v", loc)
	}
	built := install(t, filepath.Join(local, "target", "release"), "dendrite-core")
	t.Setenv(EnvVar("dendrite-core"), "")
	if got, err := loc.Locate("dendrite-core"); err != nil || got != built {
		t.Fatalf("Locate = %q, %v", got, err)
	}

	// Without any home directory there is no store; the locator still works.
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if loc := DefaultLocator(); loc.MemberDirs != nil || loc.Home != "" {
		t.Fatalf("homeless locator = %+v", loc)
	}
}
