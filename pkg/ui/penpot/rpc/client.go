// Package rpc is a small client of Penpot's RPC API (POST /api/rpc/command/<command>), authenticated
// with an access token. Penpot must run with the enable-access-tokens flag (DEN-835).
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the local backend; the frontend (:9000) proxies the same path.
	DefaultBaseURL = "http://127.0.0.1:9001"
	// TokenEnv and BaseURLEnv name the environment the client is built from.
	TokenEnv   = "PENPOT_ACCESS_TOKEN"
	BaseURLEnv = "PENPOT_BACKEND_URL"
	maxBody    = 8 << 20
)

// ErrNoToken means no access token is configured.
var ErrNoToken = errors.New("penpot: " + TokenEnv + " is empty (create one in Penpot: Profile > Access tokens)")

// Error is a Penpot RPC failure. It never carries the token.
type Error struct {
	Command string
	Status  int
	Type    string
	Hint    string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("penpot rpc %s: HTTP %d", e.Command, e.Status)
	if e.Type != "" {
		msg += " " + e.Type
	}
	if e.Hint != "" {
		msg += ": " + e.Hint
	}
	return msg
}

// Client calls Penpot commands.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client; an empty base URL means DefaultBaseURL and a nil httpClient a 15 s one.
func New(baseURL, token string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrNoToken
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: httpClient}, nil
}

// NewFromEnv builds a client from PENPOT_ACCESS_TOKEN and PENPOT_BACKEND_URL.
func NewFromEnv() (*Client, error) {
	return New(os.Getenv(BaseURLEnv), os.Getenv(TokenEnv), nil)
}

// Call runs one command with the given params (nil for none) and decodes the JSON answer into out (nil to discard it).
func (c *Client) Call(ctx context.Context, command string, params, out any) error {
	body, err := json.Marshal(orEmpty(params))
	if err != nil {
		return fmt.Errorf("penpot rpc %s: %w", command, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/rpc/command/"+command, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("penpot rpc %s: %w", command, err)
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("penpot rpc %s: %w", command, redact(err, c.token))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("penpot rpc %s: reading answer: %w", command, err)
	}
	if resp.StatusCode/100 != 2 {
		return failure(command, resp.StatusCode, raw)
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("penpot rpc %s: decoding answer: %w", command, err)
	}
	return nil
}

func orEmpty(params any) any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

// failure reads Penpot's {"type": ..., "hint": ...} error body when there is one.
func failure(command string, status int, raw []byte) *Error {
	e := &Error{Command: command, Status: status}
	var body struct {
		Type string `json:"type"`
		Hint string `json:"hint"`
	}
	if json.Unmarshal(raw, &body) == nil {
		e.Type, e.Hint = body.Type, body.Hint
	}
	return e
}

// redact keeps the token out of transport errors.
func redact(err error, token string) error {
	return errors.New(strings.ReplaceAll(err.Error(), token, "[token]"))
}
