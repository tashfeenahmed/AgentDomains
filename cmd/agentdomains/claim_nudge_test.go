package main

import (
	"strings"
	"testing"
)

// claimNudge is the CLI's one line telling an agent that the name it just
// claimed answers no DNS query. It renders the server's next_step verbatim so
// the wording has exactly one owner, and falls back to a local command when
// talking to a server old enough not to send the field.
func TestClaimNudge(t *testing.T) {
	tests := []struct {
		name      string
		resp      map[string]any
		want      string
		wantSubst []string
		wantEmpty bool
	}{
		{
			name:      "no serving field (old server) stays silent",
			resp:      map[string]any{"fqdn": "x.makes.fyi"},
			wantEmpty: true,
		},
		{
			name:      "claim with its record serves, no nudge",
			resp:      map[string]any{"fqdn": "x.makes.fyi", "serving": true},
			wantEmpty: true,
		},
		{
			name: "recordless claim prints the server's next_step verbatim",
			resp: map[string]any{
				"label": "x", "fqdn": "x.makes.fyi", "serving": false,
				"next_step": "x.makes.fyi has no record yet and points at nothing.",
			},
			want: "x.makes.fyi has no record yet and points at nothing.",
		},
		{
			name:      "recordless claim against an old server falls back to a command",
			resp:      map[string]any{"label": "x", "fqdn": "x.makes.fyi", "serving": false},
			wantSubst: []string{"agentdomains record x", "--type A"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := claimNudge(tc.resp)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("claimNudge = %q, want empty", got)
				}
				return
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("claimNudge = %q, want %q", got, tc.want)
			}
			for _, s := range tc.wantSubst {
				if !strings.Contains(got, s) {
					t.Errorf("claimNudge %q missing %q", got, s)
				}
			}
		})
	}
}
