package ticketlock

import (
	"context"
	"strings"
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
		"invalid json":             {llmchattest.New(t, "[{oops]").Client, "not valid JSON"},
	} {
		if _, err := (GatewayReviewer{Chat: tc.chat}).Review(ctx, ReviewInput{Ticket: "DEN-1"}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}
