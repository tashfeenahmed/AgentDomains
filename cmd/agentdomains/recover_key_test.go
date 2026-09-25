package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRecoverKeySendsNoKey pins that recover-key hits the recovery endpoint
// with the email and WITHOUT the configured key: the key is the lost/stale one.
func TestRecoverKeySendsNoKey(t *testing.T) {
	var method, path, auth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"ok":true,"note":"link on its way"}`))
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTDOMAINS_API_KEY", "stale-key")
	cmdRecoverKey([]string{"owner@example.com", "--api-url", srv.URL, "--json"})

	if method != "POST" || path != "/v1/account/key/recover" {
		t.Errorf("request = %s %s, want POST /v1/account/key/recover", method, path)
	}
	if body["email"] != "owner@example.com" {
		t.Errorf("body email = %v, want owner@example.com", body["email"])
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want none (the configured key is the lost one)", auth)
	}
}

func TestPrintRecoverKey(t *testing.T) {
	var b bytes.Buffer
	printRecoverKey(&b, map[string]any{"note": "Open it and confirm to get a new API key."})
	if got := strings.Count(b.String(), "confirm"); got != 1 {
		t.Errorf("confirm instruction appears %d times, want once:\n%s", got, b.String())
	}
	b.Reset()
	printRecoverKey(&b, map[string]any{})
	if !strings.Contains(b.String(), "confirm") || !strings.Contains(b.String(), "AGENTDOMAINS_API_KEY") {
		t.Errorf("fallback output missing instructions:\n%s", b.String())
	}
}
