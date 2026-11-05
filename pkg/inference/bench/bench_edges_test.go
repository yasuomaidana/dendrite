package bench

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMediansOfNothingAreZero(t *testing.T) {
	if medianInt(nil) != 0 || medianFloat(nil) != 0 {
		t.Error("the median of no values must be 0")
	}
}

func TestSampleMachineReadsSwapPressureAndThrottling(t *testing.T) {
	outputs := map[string]string{
		"sysctl":          "total = 22528.00M  used = 21778.50M  free = 749.50M  (encrypted)",
		"memory_pressure": "System-wide memory free percentage: 12%",
		"pmset":           "CPU_Scheduler_Limit = 100\nCPU_Speed_Limit = 80",
	}
	got := sampleMachine(func(name string, _ ...string) (string, error) { return outputs[name], nil })
	if got.SwapUsedMB != 21778.5 || got.FreePct != 12 || !got.Throttled {
		t.Errorf("sampled %+v", got)
	}
	outputs["pmset"] = "CPU_Speed_Limit = 100"
	if sampleMachine(func(name string, _ ...string) (string, error) { return outputs[name], nil }).Throttled {
		t.Error("a full-speed CPU is not throttled")
	}
	none := sampleMachine(func(string, ...string) (string, error) { return "", errors.New("not found") })
	if none != (Machine{}) {
		t.Errorf("without the commands everything reads zero, got %+v", none)
	}
	_ = SampleMachine() // the real commands: present on macOS, absent elsewhere
}

// scripted is a server whose /api/ps, /api/chat and /api/generate answers a test chooses.
func scripted(t *testing.T, ps, chat string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_, _ = w.Write([]byte(ps))
		case "/api/chat":
			_, _ = w.Write([]byte(chat))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func oneRun(url string) Config {
	return Config{OllamaURL: url, Models: []string{"m"}, Prompts: []Prompt{{Name: "p", User: "q"}}, NumPredict: 8}
}

func TestExecuteReportsAnUnreachableOrUnreadableOllama(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if _, err := Execute(context.Background(), oneRun(dead.URL)); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("unreachable: %v", err)
	}
	if _, err := Execute(context.Background(), oneRun(scripted(t, "not json", "").URL)); err == nil {
		t.Error("an unreadable /api/ps must fail the benchmark")
	}
	if _, err := Execute(context.Background(), oneRun("http://bad\x7fhost")); err == nil {
		t.Error("an invalid URL must fail")
	}
}

func TestExecuteSkipsUnreadableChunksAndFailsOnAnOversizedLine(t *testing.T) {
	chat := "not a chunk\n" + `{"message":{"content":"ok"},"done":true,"eval_count":1,"eval_duration":1000000}` + "\n"
	runs, err := Execute(context.Background(), oneRun(scripted(t, `{"models":[]}`, chat).URL))
	if err != nil || len(runs) != 1 || runs[0].Answer != "ok" {
		t.Fatalf("%+v %v", runs, err)
	}
	huge := strings.Repeat("x", 5<<20) // longer than the scanner's 4 MB line limit, without a newline
	runs, err = Execute(context.Background(), oneRun(scripted(t, `{"models":[]}`, huge).URL))
	if err != nil || len(runs) != 1 || runs[0].Err == "" {
		t.Errorf("an oversized line is recorded as the run's error: %+v %v", runs, err)
	}
}

type failingJudge struct{}

func (failingJudge) Score(context.Context, Prompt, string) (int, string, error) {
	return 0, "", errors.New("judge down")
}

func TestExecuteNotesAFailingJudgeAndStopsWhenCancelled(t *testing.T) {
	chat := `{"message":{"content":"ok"},"done":true}` + "\n"
	srv := scripted(t, `{"models":[]}`, chat)
	cfg := oneRun(srv.URL)
	cfg.Judge = failingJudge{}
	runs, err := Execute(context.Background(), cfg)
	if err != nil || len(runs) != 1 || !strings.Contains(runs[0].ScoreNote, "judge failed: judge down") {
		t.Errorf("%+v %v", runs, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cfg = oneRun(srv.URL)
	cfg.Runs = 3
	cfg.Progress = func(string) { cancel() } // cancel after the first finished run
	runs, err = Execute(ctx, cfg)
	if !errors.Is(err, context.Canceled) || len(runs) != 1 {
		t.Errorf("%d runs, %v", len(runs), err)
	}
}

func TestGatewayJudgeEdges(t *testing.T) {
	j := GatewayJudge{URL: "http://127.0.0.1:1", Model: "m"}
	if s, note, err := j.Score(context.Background(), Prompt{}, "  "); s != 1 || note != "empty answer" || err != nil {
		t.Errorf("empty answer: %d %q %v", s, note, err)
	}
	if _, _, err := j.Score(context.Background(), Prompt{}, "answer"); err == nil {
		t.Error("an unreachable gateway must fail")
	}
	if _, _, err := (GatewayJudge{URL: "http://bad\x7fhost"}).Score(context.Background(), Prompt{}, "answer"); err == nil {
		t.Error("an invalid URL must fail")
	}
	slow := GatewayJudge{URL: "http://127.0.0.1:1", Client: &http.Client{Timeout: time.Millisecond}}
	if _, _, err := slow.Score(context.Background(), Prompt{}, "answer"); err == nil {
		t.Error("a client timeout must fail")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) }))
	defer bad.Close()
	if _, _, err := (GatewayJudge{URL: bad.URL}).Score(context.Background(), Prompt{}, "answer"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("a reply without choices: %v", err)
	}
}

func TestGenerateReportsATransportFailure(t *testing.T) {
	// /api/ps answers, then the chat connection is cut: the run fails, the benchmark goes on.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ps" {
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()
	runs, err := Execute(context.Background(), oneRun(srv.URL))
	if err != nil || len(runs) != 1 || runs[0].Err == "" {
		t.Errorf("%+v %v", runs, err)
	}
	if _, err := postJSON(context.Background(), Config{OllamaURL: "http://bad\x7fhost"}, "/x", nil); err == nil {
		t.Error("an invalid URL must fail")
	}
}

func TestTheModelUnderTestIsNotUnloaded(t *testing.T) {
	srv := scripted(t, `{"models":[{"name":"m"}]}`, `{"message":{"content":"ok"},"done":true}`+"\n")
	if runs, err := Execute(context.Background(), oneRun(srv.URL)); err != nil || len(runs) != 1 || runs[0].Err != "" {
		t.Errorf("%+v %v", runs, err)
	}
}
