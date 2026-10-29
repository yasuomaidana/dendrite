package ticketlock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/llmchat/llmchattest"
)

func TestGatewayReviewerAsksTheModelAndReadsTheVerdicts(t *testing.T) {
	gw := llmchattest.New(t, "Here you go:\n```json\n[{\"ac\":\"AC-1\",\"pass\":true,\"reason\":\"seen\"},{\"ac\":\"AC-2\",\"pass\":false,\"reason\":\"no test\"}]\n```")
	got, err := GatewayReviewer{Chat: gw.Client}.Review(context.Background(), ReviewInput{Ticket: "DEN-1", Description: "goal", Criteria: []string{"[AC-1] a", "[AC-2] b"}, Diff: "diff --git"})
	if err != nil || len(got) != 2 || !got[0].Pass || got[1].Pass || got[1].Reason != "no test" {
		t.Fatalf("%+v, %v", got, err)
	}
	if last := gw.Last(); !strings.Contains(last.System, "acceptance criteria") || !strings.Contains(last.User, "[AC-2] b") || !strings.Contains(last.User, "diff --git") {
		t.Errorf("prompt = %+v", last)
	}
}

func TestGatewayReviewerFailures(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		chat llmchat.Completer
		want string
	}{
		"the model is unreachable": {llmchat.Client{URL: "http://127.0.0.1:1"}, "connect"},
		"the gateway errs":         {llmchattest.Failing(t, 503).Client, "HTTP 503"},
		"no json array":            {llmchattest.New(t, "looks fine to me").Client, "JSON array"},
		"broken json":              {llmchattest.New(t, "[{oops]").Client, "JSON array"},
		"wrong shape":              {llmchattest.New(t, "[1,2]").Client, "not valid JSON"},
	} {
		if _, err := (GatewayReviewer{Chat: tc.chat}).Review(ctx, ReviewInput{Ticket: "DEN-1"}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

func TestGatewayReviewerAsksOnceMoreWhenTheFirstAnswerCannotBeRead(t *testing.T) {
	gw := llmchattest.Sequence(t, "Let me think: [AC-1] holds and [AC-2] does not.", `[{"ac":"AC-1","pass":true},{"ac":"AC-2","pass":false,"reason":"no test"}]`)
	got, err := GatewayReviewer{Chat: gw.Client}.Review(context.Background(), ReviewInput{Ticket: "DEN-1", Criteria: []string{"[AC-1] a", "[AC-2] b"}})
	if err != nil || len(got) != 2 || got[1].Pass {
		t.Fatalf("%+v, %v", got, err)
	}
	if gw.Requests() != 2 || !strings.Contains(gw.Last().User, "could not be read") || !strings.Contains(gw.Last().User, "ONLY the JSON array") {
		t.Errorf("the second request quotes why the first failed: %d requests, %q", gw.Requests(), gw.Last().User)
	}

	twice := llmchattest.Sequence(t, "no idea", "still prose")
	if _, err := (GatewayReviewer{Chat: twice.Client}).Review(context.Background(), ReviewInput{Ticket: "DEN-1"}); err == nil || !strings.Contains(err.Error(), "JSON array") || twice.Requests() != 2 {
		t.Errorf("a second unusable answer is the error: %v after %d requests", err, twice.Requests())
	}

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"prose"}}]}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := (GatewayReviewer{Chat: llmchat.Client{URL: srv.URL}}).Review(context.Background(), ReviewInput{Ticket: "DEN-1"}); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("a failure of the retry is reported: %v", err)
	}
}

func TestGatewayReviewerDoesNotAcceptAnEmptyVerdictList(t *testing.T) {
	gw := llmchattest.Sequence(t, "[]", `[{"ac":"AC-1","pass":true}]`)
	got, err := GatewayReviewer{Chat: gw.Client}.Review(context.Background(), ReviewInput{Ticket: "DEN-1", Criteria: []string{"[AC-1] a"}})
	if err != nil || len(got) != 1 || gw.Requests() != 2 || !strings.Contains(gw.Last().User, "empty list for 1 criteria") {
		t.Fatalf("an empty list is asked again: %+v, %v, %d requests", got, err, gw.Requests())
	}
	none := llmchattest.New(t, "[]")
	if _, err := (GatewayReviewer{Chat: none.Client}).Review(context.Background(), ReviewInput{Ticket: "DEN-1", Criteria: []string{"[AC-1] a"}}); err == nil || !strings.Contains(err.Error(), "empty list") {
		t.Errorf("still empty after the retry: %v", err)
	}
	if got, err := (GatewayReviewer{Chat: none.Client}).Review(context.Background(), ReviewInput{Ticket: "DEN-1"}); err != nil || len(got) != 0 {
		t.Errorf("with no criteria an empty list is right: %+v, %v", got, err)
	}
}
