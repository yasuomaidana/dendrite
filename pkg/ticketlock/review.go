package ticketlock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GatewayReviewer asks a chat model behind an OpenAI-compatible gateway (LiteLLM) to judge a diff against
// acceptance criteria.
type GatewayReviewer struct {
	// URL is the chat completions endpoint, Key its bearer token and Model the model alias.
	URL, Key, Model string
	// HTTP is the client to use; nil means http.DefaultClient.
	HTTP *http.Client
}

const reviewPrompt = "You review a code change against the acceptance criteria of its ticket. For EACH criterion decide whether the diff " +
	"fulfils it. Answer with ONLY a JSON array, one object per criterion: " +
	`[{"ac":"AC-1","pass":true,"reason":"one short sentence"}]. ` + "Mark pass false when the diff does not show the criterion is met."

// Review returns one verdict per criterion the model answered for.
func (g GatewayReviewer) Review(ctx context.Context, in ReviewInput) ([]ACVerdict, error) {
	user := fmt.Sprintf("Ticket %s\n\n%s\n\nCriteria:\n%s\n\nDiff:\n%s", in.Ticket, in.Description, strings.Join(in.Criteria, "\n"), in.Diff)
	body, _ := json.Marshal(map[string]any{"model": g.Model, "temperature": 0, "messages": []map[string]string{
		{"role": "system", "content": reviewPrompt}, {"role": "user", "content": user}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.Key != "" {
		req.Header.Set("Authorization", "Bearer "+g.Key)
	}
	client := g.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the gateway answered HTTP %d", resp.StatusCode)
	}
	var chat struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &chat) != nil || len(chat.Choices) == 0 {
		return nil, errors.New("the gateway sent no answer")
	}
	return parseVerdicts(chat.Choices[0].Message.Content)
}

// parseVerdicts reads the JSON array out of a model's answer, which may wrap it in prose or a code fence.
func parseVerdicts(content string) ([]ACVerdict, error) {
	start, end := strings.Index(content, "["), strings.LastIndex(content, "]")
	if start < 0 || end < start {
		return nil, errors.New("the review did not answer with a JSON array")
	}
	var out []ACVerdict
	if err := json.Unmarshal([]byte(content[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("the review answer is not valid JSON: %w", err)
	}
	return out, nil
}
