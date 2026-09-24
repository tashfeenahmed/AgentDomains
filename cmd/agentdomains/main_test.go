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
	lookup := nameNotFoundHint("") // what get/delete pass with no --domain
	tests := []struct {
		name       string
		api        *client.APIError
		notFound   string
		wantSubstr []string
		notSubstr  []string
		wantEmpty  bool
	}{
		{
			name:       "401 points at the key and recovery, not signup",
			api:        &client.APIError{Status: 401, Message: "invalid key", Body: map[string]any{}},
			wantSubstr: []string{"AGENTDOMAINS_API_KEY", "/v1/account/key/recover"},
			notSubstr:  []string{"run `agentdomains signup`"},
		},
		{
			name:       "404 on a name lookup suggests list",
			api:        &client.APIError{Status: 404, Message: "domain not found (or not yours)", Body: map[string]any{}},
			notFound:   lookup,
			wantSubstr: []string{"agentdomains list"},
			notSubstr:  []string{"--domain"},
		},
		{
			name:      "404 outside a name lookup gets no hint",
			api:       &client.APIError{Status: 404, Message: "no forward on shop.makes.fyi", Body: map[string]any{}},
			wantEmpty: true,
		},
		{
			name:      "404 for an unknown endpoint gets no hint even on a lookup",
			api:       &client.APIError{Status: 404, Message: "no such endpoint: GET /v1/nope", Body: map[string]any{}},
			notFound:  lookup,
			wantEmpty: true,
		},
		{
			name: "429 with retry_after counts in seconds",
			api: &client.APIError{Status: 429, Message: "rate limit exceeded — slow down",
				Body: map[string]any{"retry_after": float64(17)}},
			wantSubstr: []string{"17 second"},
		},
		{
			name:       "429 without retry_after says wait",
			api:        &client.APIError{Status: 429, Message: "slow down", Body: map[string]any{}},
			wantSubstr: []string{"wait a minute"},
		},
		{
			name: "retry flag true still wins over the status table",
			api: &client.APIError{Status: 502, Message: "upstream down",
				Body: map[string]any{"retry": true}},
			wantSubstr: []string{"worth retrying"},
		},
		{
			name: "retry flag false says not retryable",
			api: &client.APIError{Status: 502, Message: "upstream down",
				Body: map[string]any{"retry": false}},
			wantSubstr: []string{"not retryable"},
		},
		{
			name:      "500 without hints stays quiet",
			api:       &client.APIError{Status: 500, Message: "boom", Body: map[string]any{}},
			wantEmpty: true,
		},
		{
			name:      "409 owned carries no extra hint",
			api:       &client.APIError{Status: 409, Message: "taken", Body: map[string]any{"owned": true}},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiHint(tt.api, tt.notFound)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("apiHint() = %q, want empty", got)
				}
				return
			}
			for _, s := range tt.wantSubstr {
				if !strings.Contains(got, s) {
					t.Errorf("apiHint() = %q, want substring %q", got, s)
				}
			}
			for _, s := range tt.notSubstr {
				if strings.Contains(got, s) {
					t.Errorf("apiHint() = %q, must not contain %q", got, s)
				}
			}
		})
	}
}

// TestNameNotFoundHint: without --domain the server searched every domain, so
// no domain is suggested; with one, the hint names the OTHER domain.
func TestNameNotFoundHint(t *testing.T) {
	tests := []struct {
		domain, want, notWant string
	}{
		{"", "agentdomains list", "--domain"},
		{"makes.fyi", "--domain agentdomains.co", "--domain makes.fyi"},
		{"agentdomains.co", "--domain makes.fyi", "--domain agentdomains.co"},
		{"example.com", "agentdomains list", "--domain"},
	}
	for _, tt := range tests {
		got := nameNotFoundHint(tt.domain)
		if !strings.Contains(got, tt.want) {
			t.Errorf("nameNotFoundHint(%q) = %q, want %q", tt.domain, got, tt.want)
		}
		if strings.Contains(got, tt.notWant) {
			t.Errorf("nameNotFoundHint(%q) = %q, must not contain %q", tt.domain, got, tt.notWant)
		}
	}
}
