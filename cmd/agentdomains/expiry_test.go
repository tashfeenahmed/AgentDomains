package main

import (
	"strings"
	"testing"
	"time"
)

// TestExpiryLine pins the countdown `whoami` shows a provisional account. The
// server sends a bare epoch number; the whole point of rendering it is that an
// agent reading the line NOW knows it has to run `email`, and one reading it
// with hours left gets the most urgent wording. No expires_at (verified or Pro
// account) must stay completely silent.
func TestExpiryLine(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	in := func(d time.Duration) float64 { return float64(now.Add(d).Unix()) }

	tests := []struct {
		name      string
		v         any
		wantEmpty bool
		wantSubst []string
	}{
		{"verified account (field absent) is silent", nil, true, nil},
		{"string is not a time", "1790000000", true, nil},
		{"30 days out, calm wording", in(29 * 24 * time.Hour), false,
			[]string{"29 days", "agentdomains email", "deleted"}},
		{"10 days out", in(10*24*time.Hour + time.Hour), false,
			[]string{"10 days", "agentdomains email"}},
		{"3 days left gets the warning mark", in(3 * 24 * time.Hour), false,
			[]string{"⚠", "3 day(s)", "agentdomains email"}},
		{"under a day left names the escape hatch", in(4 * time.Hour), false,
			[]string{"⚠", "less than 24 hours", "agentdomains upgrade"}},
		{"already past the deadline still says validate now", in(-2 * time.Hour), false,
			[]string{"past its validation deadline", "agentdomains email"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expiryLine(now, tt.v)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expiryLine = %q, want empty", got)
				}
				return
			}
			for _, s := range tt.wantSubst {
				if !strings.Contains(got, s) {
					t.Errorf("expiryLine = %q, want it to contain %q", got, s)
				}
			}
			if !strings.Contains(got, "validate") {
				t.Errorf("every warning must name the action, got %q", got)
			}
		})
	}
}
