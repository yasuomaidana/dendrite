package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// callSpec says how one Plane call reports an unsuccessful answer.
type callSpec struct {
	failure  string // names the call: "<failure>: status N"
	notFound error  // returned for 404; nil makes a 404 an ordinary failure
	withBody bool   // append Plane's answer: "<failure>: status N, err: <body>"
}

// callRaw sends one request (body is JSON-encoded when not nil) and returns the answer's bytes.
// 401 and 403 are ErrUnauthorized, 404 is spec.notFound when set, other statuses from 400 up are
// spec.failure with the status.
func (c *Client) callRaw(ctx context.Context, method, endpoint string, body interface{}, spec callSpec) ([]byte, error) {
	req, err := c.newRequest(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound && spec.notFound != nil:
		return nil, spec.notFound
	case resp.StatusCode >= 400 && spec.withBody:
		return nil, fmt.Errorf("%s: status %d, err: %s", spec.failure, resp.StatusCode, data)
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("%s: status %d", spec.failure, resp.StatusCode)
	}
	return data, err
}

// call is callRaw that decodes a JSON answer into out (nil ignores the answer).
func (c *Client) call(ctx context.Context, method, endpoint string, body, out interface{}, spec callSpec) error {
	data, err := c.callRaw(ctx, method, endpoint, body, spec)
	if err != nil || out == nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(data)).Decode(out)
}

// decodeList reads a list that Plane sends either bare or wrapped as {"results": [...]}. Anything
// else yields an empty list.
func decodeList[T any](data []byte) []T {
	var wrapped struct {
		Results []T `json:"results"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Results) > 0 {
		return wrapped.Results
	}
	var bare []T
	if err := json.Unmarshal(data, &bare); err == nil {
		return bare
	}
	return wrapped.Results
}
