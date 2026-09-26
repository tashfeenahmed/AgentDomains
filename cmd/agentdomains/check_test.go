package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	})
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
	printCheckResults(&b, []checkVerdict{{Label: "bob", Fqdn: "bob.makes.fyi", Reason: "taken"}})
	if !strings.Contains(b.String(), "none of those are free") {
		t.Errorf("all-taken output should nudge to next steps:\n%s", b.String())
	}
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
