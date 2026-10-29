// Package llmchat is the one place Dendrite talks to a chat model through the LiteLLM gateway for small,
// structured jobs (reviewing a diff against criteria, reading a rule into directive changes).
package llmchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/rag"
)

// Completer answers one system-plus-user prompt. Client is the real one; tests use fakes.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Client is an OpenAI-compatible chat completions endpoint.
type Client struct {
	// URL is the chat completions endpoint, Key its bearer token and Model the model alias.
	URL, Key, Model string
	// HTTP is the client to use; nil means http.DefaultClient.
	HTTP *http.Client
}

// Complete asks the model at temperature 0 and returns its answer.
func (c Client) Complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{"model": c.Model, "temperature": 0, "messages": []map[string]string{
		{"role": "system", "content": system}, {"role": "user", "content": user}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the gateway answered HTTP %d", resp.StatusCode)
	}
	var chat struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &chat) != nil || len(chat.Choices) == 0 {
		return "", errors.New("the gateway sent no answer")
	}
	return chat.Choices[0].Message.Content, nil
}

// preferLocalModel is a seam for the choice of the model.
var preferLocalModel = rag.PreferLocalModel

// Gateway is the client for the project at dir: the LiteLLM gateway of the environment, its master key from
// the project's or the global .env, and the gateway's preferred local model.
func Gateway(dir string) Client {
	base := os.Getenv("LITELLM_PROXY_URL")
	if strings.TrimSpace(base) == "" {
		base = "http://localhost:4000"
	}
	base = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	key := dotenv.Lookup(dir, "LITELLM_MASTER_KEY")
	model := rag.DefaultLocalModel
	if choice, ok := preferLocalModel(context.Background(), key); ok {
		model = choice.Model
	}
	return Client{URL: base + "/v1/chat/completions", Key: key, Model: model}
}

var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// JSON cuts the JSON value out of a model's answer, which may wrap it in prose, a code fence or a <think>
// block, and may mention brackets around it ("[AC-1] holds", "returns []"). open and closeCh are '[' and ']'
// for an array, '{' and '}' for an object. It returns the longest span that starts at an open bracket, ends
// at a close bracket and is valid JSON, so the answer wins over a stray empty pair before it.
func JSON(content string, open, closeCh byte) (string, error) {
	content = thinkBlock.ReplaceAllString(content, "")
	best := ""
	for from := 0; ; {
		i := strings.IndexByte(content[from:], open)
		if i < 0 {
			break
		}
		start := from + i
		for end := strings.LastIndexByte(content, closeCh); end > start && end+1-start > len(best); end = strings.LastIndexByte(content[:end], closeCh) {
			if candidate := content[start : end+1]; json.Valid([]byte(candidate)) {
				best = candidate
				break
			}
		}
		from = start + 1
	}
	if best == "" {
		return "", fmt.Errorf("the answer holds no JSON %s (it starts: %q)", map[byte]string{'[': "array", '{': "object"}[open], head(content))
	}
	return best, nil
}

func head(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
