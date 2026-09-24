package main

import (
	"strings"
	"testing"

	"github.com/tashfeenahmed/AgentDomains/internal/client"
)

// TestAPIHint pins the recovery line attached to each status an agent hits
// most. The point of these hints is that they name a command or a wait, not
// just the status — an agent reading "request failed (401)" retries forever,
// one reading the hint recovers by itself.
func TestAPIHint(t *testing.T) {
	tests := []struct {
		name       string
		api        *client.APIError
		wantSubstr string // "" means: no hint expected
		wantEmpty  bool
	}{
		{
			name:       "401 names signup as the recovery",
			api:        &client.APIError{Status: 401, Message: "invalid key", Body: map[string]any{}},
			wantSubstr: "agentdomains signup",
		},
		{
			name:       "404 suggests list and --domain",
			api:        &client.APIError{Status: 404, Message: "not found", Body: map[string]any{}},
			wantSubstr: "agentdomains list",
		},
		{
			name: "429 with retry_after counts in seconds",
			api: &client.APIError{Status: 429, Message: "rate limit exceeded — slow down",
				Body: map[string]any{"retry_after": float64(17)}},
			wantSubstr: "17 second",
		},
		{
			name: "retry flag true still wins over the status table",
			api: &client.APIError{Status: 502, Message: "upstream down",
				Body: map[string]any{"retry": true}},
			wantSubstr: "worth retrying",
		},
		{
			name: "retry flag false says not retryable",
			api: &client.APIError{Status: 502, Message: "upstream down",
				Body: map[string]any{"retry": false}},
			wantSubstr: "not retryable",
		},
		{
			name:      "500 without hints stays quiet",
			api:       &client.APIError{Status: 500, Message: "boom", Body: map[string]any{}},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiHint(tt.api)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("apiHint() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("apiHint() = %q, want substring %q", got, tt.wantSubstr)
			}
		})
	}
}
