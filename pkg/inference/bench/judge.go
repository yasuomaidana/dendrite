package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GatewayJudge scores answers with a strong model behind the LiteLLM gateway.
type GatewayJudge struct {
	URL    string // gateway base URL, e.g. http://localhost:4000
	Key    string
	Model  string
	Client *http.Client
}

var scoreJSON = regexp.MustCompile(`"score"\s*:\s*([1-5])`)

// Score implements Judge. The judge sees the same context the candidate saw, so it can check that the
// answer is grounded in it rather than merely fluent.
func (j GatewayJudge) Score(ctx context.Context, p Prompt, answer string) (int, string, error) {
	if strings.TrimSpace(answer) == "" {
		return 1, "empty answer", nil
	}
	if j.Client == nil {
		j.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	prompt := "You grade an AI assistant's answer to a question about a software project. The assistant saw only the " +
		"retrieved context below.\n\nCONTEXT AND QUESTION:\n" + p.User + "\n\nASSISTANT'S ANSWER:\n" + answer +
		"\n\nScore 1-5: 5 = correct, complete and grounded in the context; 3 = partly right or vague; " +
		"1 = wrong, invented, or ignores the context. Reply with only JSON: {\"score\": N, \"reason\": \"one short sentence\"}"
	body, _ := json.Marshal(map[string]any{
		"model": j.Model, "temperature": 0, "max_tokens": 200,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(j.URL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if j.Key != "" {
		req.Header.Set("Authorization", "Bearer "+j.Key)
	}
	resp, err := j.Client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(data[:min(len(data), 200)])))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return 0, "", fmt.Errorf("unreadable judge reply")
	}
	text := out.Choices[0].Message.Content
	m := scoreJSON.FindStringSubmatch(text)
	if m == nil {
		return 0, "", fmt.Errorf("judge gave no score: %q", text[:min(len(text), 120)])
	}
	score, _ := strconv.Atoi(m[1])
	var reason struct {
		Reason string `json:"reason"`
	}
	if i := strings.Index(text, "{"); i >= 0 {
		_ = json.Unmarshal([]byte(text[i:]), &reason)
	}
	return score, reason.Reason, nil
}
