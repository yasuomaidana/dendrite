package ticketlock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/llmchat"
)

// GatewayReviewer asks a chat model to judge a diff against acceptance criteria.
type GatewayReviewer struct {
	Chat llmchat.Completer
}

const reviewPrompt = "You review a code change against the acceptance criteria of its ticket. For EACH criterion decide whether the diff " +
	"fulfils it. Answer with ONLY a JSON array, one object per criterion: " +
	`[{"ac":"AC-1","pass":true,"reason":"one short sentence"}]. ` + "Mark pass false when the diff does not show the criterion is met."

// Review returns one verdict per criterion the model answered for.
func (g GatewayReviewer) Review(ctx context.Context, in ReviewInput) ([]ACVerdict, error) {
	user := fmt.Sprintf("Ticket %s\n\n%s\n\nCriteria:\n%s\n\nDiff:\n%s", in.Ticket, in.Description, strings.Join(in.Criteria, "\n"), in.Diff)
	answer, err := g.Chat.Complete(ctx, reviewPrompt, user)
	if err != nil {
		return nil, err
	}
	return parseVerdicts(answer)
}

// parseVerdicts reads the JSON array out of a model's answer, which may wrap it in prose or a code fence.
func parseVerdicts(content string) ([]ACVerdict, error) {
	raw, err := llmchat.JSON(content, '[', ']')
	if err != nil {
		return nil, err
	}
	var out []ACVerdict
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("the review answer is not valid JSON: %w", err)
	}
	return out, nil
}
