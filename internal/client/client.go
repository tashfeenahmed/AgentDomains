// Package client is a thin HTTP client for the AgentDomains API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// APIError is a refusal the API explained. The server answers every error as
// JSON, and some of those answers carry more than a message: a claim of a name
// you already hold says owned:true, and an upstream failure says whether it is
// worth retrying. Keeping the whole body lets a command say something better
// than the raw message.
type APIError struct {
	Status  int
	Message string
	Body    map[string]any
}

func (e *APIError) Error() string { return e.Message }

// Flag reports a boolean field of the error body (owned, retry, ...). The
// second return says whether the field was there at all, which "retry":false
// and "no retry field" have to be told apart.
func (e *APIError) Flag(name string) (value, present bool) {
	v, ok := e.Body[name].(bool)
	return v, ok
}

// Do performs a request and decodes the JSON response into out (if non-nil).
// On a non-2xx status it returns an *APIError carrying the server's message
// and the rest of the body.
func (c *Client) Do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach AgentDomains API at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode, Body: map[string]any{}}
		_ = json.Unmarshal(raw, &apiErr.Body)
		if msg, ok := apiErr.Body["error"].(string); ok && msg != "" {
			apiErr.Message = msg
		} else {
			apiErr.Message = fmt.Sprintf("request failed (%d)", resp.StatusCode)
		}
		return apiErr
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
