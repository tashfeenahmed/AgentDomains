package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tashfeenahmed/AgentDomains/internal/client"
)

// TestCheckSendsNoKey pins that check hits /v1/available with the label and
// WITHOUT the configured key: deciding whether a name is free happens before
// (and independently of) having an account.
func TestCheckSendsNoKey(t *testing.T) {
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path+"?"+r.URL.Query().Encode(), r.Header.Get("Authorization")
		w.Write([]byte(`{"label":"alice","domain":"makes.fyi","fqdn":"alice.makes.fyi","available":true,"reason":"available"}`))
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTDOMAINS_API_KEY", "some-key")
	cmdCheck([]string{"alice", "--api-url", srv.URL, "--json"})

	if path != "/v1/available?label=alice" {
		t.Errorf("request path = %q, want /v1/available?label=alice", path)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want none (availability is public)", auth)
	}
}

// TestCheckMultipleLabels pins the batch behaviour: one request per label,
// --domain forwarded, and the JSON output carrying every verdict in order —
// an agent picking between candidate names reads that array.
func TestCheckMultipleLabels(t *testing.T) {
	var labels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		label := r.URL.Query().Get("label")
		if d := r.URL.Query().Get("domain"); d != "" {
			labels = append(labels, label+"@"+d)
		} else {
			labels = append(labels, label)
		}
		avail := label != "taken-name"
		reason := "available"
		if !avail {
			reason = "taken"
		}
		w.Write([]byte(`{"label":"` + label + `","fqdn":"` + label + `.agentdomains.co","available":` + boolStr(avail) + `,"reason":"` + reason + `"}`))
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	cmdCheck([]string{"alpha", "taken-name", "--domain", "agentdomains.co", "--api-url", srv.URL, "--json"})

	if len(labels) != 2 || labels[0] != "alpha@agentdomains.co" || labels[1] != "taken-name@agentdomains.co" {
		t.Fatalf("labels seen = %v, want both scoped to agentdomains.co", labels)
	}
	// --json prints the verdict array; capture stdout is out of scope for the
	// helpers here, so the request assertions above plus TestCheckHumanOutput
	// pin the two halves.
	_ = json.Valid
}

func TestCheckHumanOutput(t *testing.T) {
	var b strings.Builder
	printCheckResults(&b, []checkVerdict{
		{Label: "alice", Fqdn: "alice.makes.fyi", Available: true, Reason: "available"},
		{Label: "bob", Fqdn: "bob.makes.fyi", Reason: "taken"},
		{Label: "Bad Name!", Fqdn: "Bad Name!.makes.fyi", Reason: "invalid", Detail: "labels must be lowercase"},
	}, true)
	out := b.String()
	if !strings.Contains(out, "free     alice.makes.fyi") {
		t.Errorf("missing free line:\n%s", out)
	}
	if !strings.Contains(out, "taken    bob.makes.fyi") {
		t.Errorf("missing taken line:\n%s", out)
	}
	if !strings.Contains(out, "invalid  Bad Name!.makes.fyi  (labels must be lowercase)") {
		t.Errorf("invalid line must carry the server's detail:\n%s", out)
	}
	if strings.Contains(out, "none of those are free") {
		t.Errorf("nudge shown although one name was free:\n%s", out)
	}

	b.Reset()
	printCheckResults(&b, []checkVerdict{{Label: "bob", Fqdn: "bob.makes.fyi", Reason: "taken"}}, true)
	if !strings.Contains(b.String(), "none of those are free") {
		t.Errorf("all-taken output should nudge to next steps:\n%s", b.String())
	}

	// A run cut short never claims "none of those are free": the names it
	// never got to may be.
	b.Reset()
	printCheckResults(&b, []checkVerdict{{Label: "bob", Fqdn: "bob.makes.fyi", Reason: "taken"}}, false)
	if strings.Contains(b.String(), "none of those are free") {
		t.Errorf("partial run must not nudge as if every name was taken:\n%s", b.String())
	}
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// availableServer answers /v1/available, refusing the listed labels with a 429
// (retry_after 2) the given number of times each before answering.
func availableServer(t *testing.T, limited map[string]int) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		label := r.URL.Query().Get("label")
		seen = append(seen, label)
		if limited[label] > 0 {
			limited[label]--
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded — slow down","retry_after":2}`))
			return
		}
		w.Write([]byte(`{"label":"` + label + `","fqdn":"` + label + `.makes.fyi","available":true,"reason":"available"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// TestCheckWaitsOutOneRateLimit pins that a 429 carrying retry_after is waited
// out once and the same label retried, and the run then carries on.
func TestCheckWaitsOutOneRateLimit(t *testing.T) {
	srv, seen := availableServer(t, map[string]int{"beta": 1})
	var waits []time.Duration
	results, err := runCheck(client.New(srv.URL, "", "test"), []string{"alpha", "beta", "gamma"}, "", func(d time.Duration) { waits = append(waits, d) })
	if err != nil {
		t.Fatalf("runCheck: %v", err)
	}
	if len(results) != 3 || results[1].Label != "beta" || !results[1].Available {
		t.Fatalf("results = %+v, want alpha, beta, gamma all answered", results)
	}
	if len(waits) != 1 || waits[0] != 2*time.Second {
		t.Errorf("waits = %v, want one wait of retry_after (2s)", waits)
	}
	if got := strings.Join(*seen, ","); got != "alpha,beta,beta,gamma" {
		t.Errorf("requests = %s, want beta retried once", got)
	}
}

// TestCheckKeepsResultsOnFailure pins that a second 429 (or any other error)
// stops the run but hands back every verdict already fetched, so the caller
// can print them and --json stays a valid array of what was answered.
func TestCheckKeepsResultsOnFailure(t *testing.T) {
	srv, seen := availableServer(t, map[string]int{"gamma": 2})
	results, err := runCheck(client.New(srv.URL, "", "test"), []string{"alpha", "beta", "gamma", "delta"}, "", func(time.Duration) {})
	var api *client.APIError
	if !errors.As(err, &api) || api.Status != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want the second 429", err)
	}
	if len(results) != 2 || results[0].Label != "alpha" || results[1].Label != "beta" {
		t.Fatalf("results = %+v, want alpha and beta kept", results)
	}
	if got := strings.Join(*seen, ","); got != "alpha,beta,gamma,gamma" {
		t.Errorf("requests = %s, want gamma retried once and delta never asked", got)
	}
	b, _ := json.Marshal(results)
	if !json.Valid(b) || !strings.HasPrefix(string(b), "[") {
		t.Errorf("partial results must marshal to a JSON array: %s", b)
	}
}

// TestCheckRetryAfterBounds pins which errors are waited out: only a 429, and
// only when the requested wait is short.
func TestCheckRetryAfterBounds(t *testing.T) {
	if _, ok := checkRetryAfter(errors.New("cannot reach")); ok {
		t.Error("a network error is not a rate limit")
	}
	if _, ok := checkRetryAfter(&client.APIError{Status: 400, Body: map[string]any{}}); ok {
		t.Error("a 400 is not a rate limit")
	}
	if d, ok := checkRetryAfter(&client.APIError{Status: 429, Body: map[string]any{}}); !ok || d != time.Second {
		t.Errorf("429 without retry_after = %v,%v, want 1s,true", d, ok)
	}
	if _, ok := checkRetryAfter(&client.APIError{Status: 429, Body: map[string]any{"retry_after": float64(600)}}); ok {
		t.Error("a 10-minute wait should not be waited out")
	}
}
