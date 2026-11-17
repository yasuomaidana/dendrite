package penpotstack

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/env"
)

var (
	serviceName   = regexp.MustCompile(`(?m)^  (penpot-[a-z]+):\s*$`)
	flagsLine     = regexp.MustCompile(`(?m)^\s*PENPOT_FLAGS: "([^"]+)"`)
	secretKeyLine = regexp.MustCompile(`(?m)^\s+PENPOT_SECRET_KEY:`)
	healthcheck   = regexp.MustCompile(`(?m)^    healthcheck:`)
)

func shippedPenpotCompose(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "stacks", "compose.penpot.yaml"))
	if err != nil {
		t.Fatalf("stacks/compose.penpot.yaml: %v", err)
	}
	return string(data)
}

func TestPenpotComposeCopiesAgreeOnServicesFlagsAndHealthchecks(t *testing.T) {
	shipped, generated := shippedPenpotCompose(t), env.GeneratePenpotCompose(env.DefaultPenpotConfig())
	for name, find := range map[string]func(string) []string{
		"services": func(s string) []string { return firstGroups(serviceName, s) },
		"flags":    func(s string) []string { return firstGroups(flagsLine, s) },
	} {
		if a, b := strings.Join(find(shipped), ","), strings.Join(find(generated), ","); a != b {
			t.Errorf("%s differ: stacks file %q, generator %q", name, a, b)
		}
	}
	if a, b := len(healthcheck.FindAllString(shipped, -1)), len(healthcheck.FindAllString(generated, -1)); a != b {
		t.Errorf("healthchecks differ: stacks file %d, generator %d", a, b)
	}
}

func firstGroups(re *regexp.Regexp, s string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestPenpotComposeEnablesAccessTokensAndRunsAnExporter(t *testing.T) {
	for name, compose := range map[string]string{"stacks file": shippedPenpotCompose(t), "generator": env.GeneratePenpotCompose(env.DefaultPenpotConfig())} {
		for _, flags := range firstGroups(flagsLine, compose) {
			if !strings.Contains(flags, "enable-access-tokens") {
				t.Errorf("%s: PENPOT_FLAGS lacks enable-access-tokens: %q", name, flags)
			}
		}
		if n := len(secretKeyLine.FindAllString(compose, -1)); n != 2 {
			t.Errorf("%s: the backend and the exporter both need PENPOT_SECRET_KEY, found %d", name, n)
		}
		for _, want := range []string{"penpot-exporter:", "penpotapp/exporter", `PENPOT_EXPORTER_URI: "http://penpot-exporter:6061"`} {
			if !strings.Contains(compose, want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
	}
}

func TestGeneratedPenpotComposeHonoursTheExporterPortAndFillsDefaults(t *testing.T) {
	if got := env.GeneratePenpotCompose(env.PenpotConfig{ExporterPort: 9123}); !strings.Contains(got, `"9123:6061"`) {
		t.Errorf("exporter port not used:\n%s", got)
	}
	if got := env.GeneratePenpotCompose(env.PenpotConfig{}); !strings.Contains(got, `"9002:6061"`) || strings.Count(got, env.DefaultPenpotConfig().SecretKey) != 2 {
		t.Errorf("defaults not filled:\n%s", got)
	}
}

func TestPenpotAccessTokenSlotIsEmptyAndNotGenerated(t *testing.T) {
	for _, k := range env.ManagedSecrets {
		if k == "PENPOT_ACCESS_TOKEN" {
			t.Fatal("Penpot issues the token; 'secrets ensure --generate' must not invent one")
		}
	}
	for _, path := range []string{filepath.Join("..", "..", "..", "stacks", ".env.example"), filepath.Join("..", "defaults.env")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`(?m)^PENPOT_ACCESS_TOKEN=$`).Match(data) {
			t.Errorf("%s needs an empty PENPOT_ACCESS_TOKEN= line", path)
		}
	}
}

// listen returns a TCP listener's port; the listener closes with the test.
func listen(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

func TestCheckReportsEachServiceAndHealthNeedsAllThree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(srv.Close)
	frontend, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	backend, exporter := listen(t), listen(t)
	cfg := env.PenpotConfig{FrontendPort: frontend, BackendPort: backend, ExporterPort: exporter}

	probes := Check(context.Background(), "127.0.0.1", cfg, time.Second)
	if !Healthy(probes) || len(probes) != 3 {
		t.Fatalf("all three should be up: %+v", probes)
	}
	if out := Summary(probes); strings.Contains(out, "DOWN") || !strings.Contains(out, "exporter") {
		t.Errorf("summary:\n%s", out)
	}

	cfg.ExporterPort = closedPort(t)
	probes = Check(context.Background(), "127.0.0.1", cfg, time.Second)
	if Healthy(probes) {
		t.Fatalf("a missing exporter must make the stack unhealthy: %+v", probes)
	}
	if out := Summary(probes); !strings.Contains(out, "DOWN") {
		t.Errorf("summary should name the DOWN service:\n%s", out)
	}
	if Healthy(nil) {
		t.Error("no probes is not healthy")
	}
}

func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}
