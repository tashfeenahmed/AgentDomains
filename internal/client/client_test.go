package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUserAgent pins the header the API groups its callers by. Go's default is
// "Go-http-client/1.1", which is indistinguishable from any other Go program, so
// the point of the test is that we never fall back to it.
func TestUserAgent(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"release build", "v0.1.3", "agentdomains-cli/v0.1.3"},
		{"source build", "dev", "agentdomains-cli/dev"},
		{"no version at all", "", "agentdomains-cli/dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("User-Agent")
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			c := New(srv.URL, "adom_test", tt.version)
			if err := c.Do("GET", "/v1/whoami", nil, nil); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if got != tt.want {
				t.Errorf("User-Agent = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUserAgentOnRequestWithBody covers the other branch of Do — a request that
// carries JSON — because the header is set once for both and a refactor could
// easily move it under the bodyless path.
func TestUserAgentOnRequestWithBody(t *testing.T) {
	var got, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "adom_test", "v0.1.3")
	if err := c.Do("POST", "/v1/subdomains", map[string]any{"label": "x"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := "agentdomains-cli/v0.1.3"; got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
	if want := "Bearer adom_test"; auth != want {
		t.Errorf("Authorization = %q, want %q", auth, want)
	}
}
