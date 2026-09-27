package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// upgrade asks for a checkout with the account's key and the chosen interval;
// billing asks for the portal. Under `go test` stdout is not a terminal, so
// neither tries to open a browser.
func TestUpgradeAndBillingHitTheBillingEndpoints(t *testing.T) {
	type call struct {
		method, path, auth, interval string
	}
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		iv, _ := body["interval"].(string)
		calls = append(calls, call{r.Method, r.URL.Path, r.Header.Get("Authorization"), iv})
		w.Write([]byte(`{"kind":"checkout","url":"https://checkout.stripe.test/x"}`))
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTDOMAINS_API_KEY", "adom_key")
	cmdUpgrade([]string{"--api-url", srv.URL, "--json"})
	cmdUpgrade([]string{"--yearly", "--api-url", srv.URL, "--json"})
	cmdBilling([]string{"--api-url", srv.URL, "--json"})

	want := []call{
		{"POST", "/v1/billing/checkout", "Bearer adom_key", "month"},
		{"POST", "/v1/billing/checkout", "Bearer adom_key", "year"},
		{"POST", "/v1/billing/portal", "Bearer adom_key", ""},
	}
	if len(calls) != len(want) {
		t.Fatalf("made %d calls, want %d: %+v", len(calls), len(want), calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, calls[i], want[i])
		}
	}
	if isTerminal() {
		t.Log("stdout is a terminal in this run; browser opening was not exercised")
	}
}
